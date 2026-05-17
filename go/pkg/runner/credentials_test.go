// SPDX-Licence-Identifier: EUPL-1.2

// Provider-credential tier-1 substrate adoption tests per Mantis
// #1657 / RFC.provider-creds-tier1.md v1.1 §9 done-criteria. Covers:
//
//   - Cache invalidation on keys.Tier1KeyChanged broadcast (ADD-1)
//   - Auto-migration via MigrateLegacyKeys (§6)
//   - WForceDeleteTier1 ack-token enforcement (ADD-3)
//   - WSetDynamicRoutes plaintext auto-import-and-strip (ADD-2)
//   - Cluster-canon audit-emit shape for all 5 event names (§7 table)
//   - Boot-scan EventProviderCredentialMigrationPendingObserved (ADD-4)

package runner_test

import (

	core "dappco.re/go"
	"dappco.re/go/config"

	"dappco.re/lthn/desktop/pkg/audit"
	"dappco.re/lthn/desktop/pkg/keys"
	"dappco.re/lthn/desktop/pkg/runner"
)

// credentialFixture wires the minimal substrate the provider-
// credential tests need: a Core container with config + keys + runner
// services, the keys service pre-wired with deterministic tier-0 +
// tier-1 KEK providers, and HOME rerouted to t.TempDir() so the on-
// disk tier-1 partition is per-test isolated.
//
// The config service is backed by a real disk file under t.TempDir()
// so the Set/Commit/Get cycle exercises the same code paths the
// production wiring in cmd/lthn/app.go takes.
//
// Returns the Core, the keys.Service, and the runner.Service already
// connected.
func credentialFixture(t *core.T) (*core.Core, *keys.Service, *runner.Service) {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	cfgFile := core.PathJoin(tmp, "lthn.yaml")
	core.AssertTrue(t, core.WriteFile(cfgFile, []byte("routes: {}\n"), 0o644).OK,
		"seed config file")

	c := core.New(
		core.WithName("config", config.NewConfigServiceWith(config.ServiceOptions{
			Path:      cfgFile,
			EnvPrefix: "LTHN_TEST_CRED",
		})),
		core.WithName("keys", keys.Register),
	)
	core.AssertTrue(t, c.ServiceStartup(core.Background(), nil).OK,
		"core ServiceStartup must succeed")

	keysSvc, _ := core.ServiceFor[*keys.Service](c, "keys")
	core.AssertTrue(t, keysSvc != nil, "keys service must be registered")
	keysSvc.SetKEKProviderTier0(func() ([]byte, bool) {
		k := make([]byte, 32)
		for i := range k {
			k[i] = 0x42
		}
		return k, true
	})
	keysSvc.SetKEKProvider(func() ([]byte, bool) {
		k := make([]byte, 32)
		for i := range k {
			k[i] = 0x77
		}
		return k, true
	})
	keysSvc.SetCore(c)

	runnerSvc := runner.NewServiceFromCore(c)
	// Mirror the production wire: ServiceStartup subscribes the
	// tier-1 cache-invalidation handler + emits the boot-scan
	// migration-pending row.
	core.AssertTrue(t, runnerSvc.ServiceStartup(core.Background(), nil).OK,
		"runner ServiceStartup must succeed")
	return c, keysSvc, runnerSvc
}

// lockedFixture mirrors credentialFixture but DOES NOT wire the tier-1
// KEK provider — the post-construct keys.Service is in the "account
// locked" state. Use to exercise the deferred-route + deferred-migration
// paths.
func lockedFixture(t *core.T) (*core.Core, *keys.Service, *runner.Service) {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	cfgFile := core.PathJoin(tmp, "lthn.yaml")
	core.AssertTrue(t, core.WriteFile(cfgFile, []byte("routes: {}\n"), 0o644).OK)

	c := core.New(
		core.WithName("config", config.NewConfigServiceWith(config.ServiceOptions{
			Path:      cfgFile,
			EnvPrefix: "LTHN_TEST_CRED_LOCKED",
		})),
		core.WithName("keys", keys.Register),
	)
	core.AssertTrue(t, c.ServiceStartup(core.Background(), nil).OK)

	keysSvc, _ := core.ServiceFor[*keys.Service](c, "keys")
	core.AssertTrue(t, keysSvc != nil)
	// Tier-0 wired (.seed bootstrap is always live) but tier-1 left
	// nil to mirror account-locked.
	keysSvc.SetKEKProviderTier0(func() ([]byte, bool) {
		k := make([]byte, 32)
		for i := range k {
			k[i] = 0x42
		}
		return k, true
	})
	keysSvc.SetCore(c)

	runnerSvc := runner.NewServiceFromCore(c)
	return c, keysSvc, runnerSvc
}

// TestCredentials_WSetDynamicRoutes_PlaintextAutoStripped_Good — the
// load-bearing payload-contract behaviour per RFC v1.1 §5: a route
// dispatched via WSetDynamicRoutes carrying APIKey + no APIKeyRef is
// auto-imported into the tier-1 substrate, the persisted config has
// APIKey empty + APIKeyRef populated, and the EventProvider
// CredentialStored audit row carries source="wails_input".
func TestCredentials_WSetDynamicRoutes_PlaintextAutoStripped_Good(t *core.T) {
	rec := installRecorder(t)
	_, keysSvc, runnerSvc := credentialFixture(t)

	r := runnerSvc.WSetDynamicRoutes([]runner.RouteInput{
		{
			Name:    "openai",
			Kind:    "openai",
			BaseURL: "https://api.openai.com/v1",
			APIKey:  "sk-secret-do-not-persist",
			Model:   "gpt-4o-mini",
		},
	})
	core.AssertTrue(t, r.OK, "WSetDynamicRoutes must succeed with plaintext auto-import")

	// Tier-1 substrate must hold the imported credential under the
	// synthesised ref.
	hasR := keysSvc.HasTier1("openai-migrated")
	core.AssertTrue(t, hasR.OK)
	core.AssertEqual(t, true, hasR.Value.(bool),
		"openai-migrated ref must exist post-import")

	// Persisted config must NOT carry plaintext. MigrationStatus
	// scans the saved on-disk shape.
	statusR := runnerSvc.WCredentialMigrationStatus()
	core.AssertTrue(t, statusR.OK)
	status := statusR.Value.(runner.MigrationStatus)
	core.AssertEqual(t, 0, status.PendingMigrationCount,
		"plaintext must be stripped from the saved routes")
	core.AssertFalse(t, status.PlaintextOnDisk,
		"PlaintextOnDisk flag must mirror the zero count")

	// Audit row shape per RFC §7 — Stored with source="wails_input".
	events := rec.snapshot()
	var stored *audit.Event
	for i := range events {
		if events[i].Event == audit.EventProviderCredentialStored {
			stored = &events[i]
			break
		}
	}
	core.AssertTrue(t, stored != nil,
		"EventProviderCredentialStored must fire from WSetDynamicRoutes")
	core.AssertEqual(t, "openai", stored.Meta["provider"])
	core.AssertEqual(t, "openai-migrated", stored.Meta["ref"])
	core.AssertEqual(t, "wails_input", stored.Meta["source"])
	// Defence-in-depth: secret bytes must not leak into Meta string
	// values. The redactor enforces too — emit-site enforces first.
	for _, ev := range events {
		for k, v := range ev.Meta {
			if vs, ok := v.(string); ok {
				core.AssertFalse(t, core.Contains(vs, "sk-secret-do-not-persist"),
					"Meta["+k+"] must not echo the plaintext credential")
			}
		}
	}
}

// TestCredentials_WForceDeleteTier1_AckTokenEnforced_Bad — the literal-
// token gate per RFC v1.1 §5 ADD-3: any value other than the canonical
// ForceDeleteAckToken MUST be rejected before any tier-1 mutation.
func TestCredentials_WForceDeleteTier1_AckTokenEnforced_Bad(t *core.T) {
	_ = installRecorder(t)
	_, keysSvc, runnerSvc := credentialFixture(t)
	core.AssertTrue(t, keysSvc.PutTier1("openai-default", []byte("sk-x")).OK)

	cases := []string{
		"",
		"yes",
		"please",
		"OPERATOR_ACKNOWLEDGED_ORPHAN", // case-sensitive
		"operator-acknowledged-orphan", // hyphenated
		"orphan",
	}
	for _, ack := range cases {
		r := runnerSvc.WForceDeleteTier1("openai-default", ack)
		core.AssertFalse(t, r.OK,
			"WForceDeleteTier1 must reject any token other than the canonical literal; got OK for ack="+ack)
	}

	// Ref must still exist after every rejected ack.
	hasR := keysSvc.HasTier1("openai-default")
	core.AssertTrue(t, hasR.OK)
	core.AssertEqual(t, true, hasR.Value.(bool),
		"rejected ack must not mutate the tier-1 substrate")
}

// TestCredentials_WForceDeleteTier1_AckTokenAccepted_Good — the happy
// path. With the canonical literal token, the delete proceeds + the
// audit row fires with the referenced_by Meta populated.
func TestCredentials_WForceDeleteTier1_AckTokenAccepted_Good(t *core.T) {
	rec := installRecorder(t)
	_, keysSvc, runnerSvc := credentialFixture(t)

	// Seed a route + matching tier-1 ref so the audit row's
	// referenced_by Meta carries a value.
	core.AssertTrue(t, runnerSvc.WSetDynamicRoutes([]runner.RouteInput{
		{Name: "openai", Kind: "openai", BaseURL: "x", APIKeyRef: "openai-default", Model: "m"},
	}).OK)
	core.AssertTrue(t, keysSvc.PutTier1("openai-default", []byte("sk-y")).OK)

	r := runnerSvc.WForceDeleteTier1("openai-default", runner.ForceDeleteAckToken)
	core.AssertTrue(t, r.OK, "canonical ack token must be accepted")

	// Ref must be gone post-force-delete.
	hasR := keysSvc.HasTier1("openai-default")
	core.AssertTrue(t, hasR.OK)
	core.AssertEqual(t, false, hasR.Value.(bool),
		"force-delete must remove the tier-1 ciphertext")

	events := rec.snapshot()
	var forced *audit.Event
	for i := range events {
		if events[i].Event == audit.EventProviderCredentialForceDeleted {
			forced = &events[i]
			break
		}
	}
	core.AssertTrue(t, forced != nil,
		"EventProviderCredentialForceDeleted must fire post-force-delete")
	core.AssertEqual(t, "openai-default", forced.Meta["ref"])
	core.AssertEqual(t, runner.ForceDeleteAckToken, forced.Meta["operator_confirmation"])
	ref, ok := forced.Meta["referenced_by"].(string)
	core.AssertTrue(t, ok, "referenced_by must be a string")
	core.AssertTrue(t, core.Contains(ref, "openai"),
		"referenced_by must list the dependent routes")
}

// TestCredentials_CacheInvalidatedOnTier1Delete_Good — the load-
// bearing ADD-1 behaviour: deleting a referenced tier-1 ref fires
// EventProviderCredentialCacheInvalidated via the runner's
// SubscribeToKeysEvents subscriber.
func TestCredentials_CacheInvalidatedOnTier1Delete_Good(t *core.T) {
	rec := installRecorder(t)
	_, keysSvc, runnerSvc := credentialFixture(t)

	core.AssertTrue(t, runnerSvc.WSetDynamicRoutes([]runner.RouteInput{
		{Name: "openai", Kind: "openai", BaseURL: "x", APIKeyRef: "openai-default", Model: "m"},
	}).OK)
	core.AssertTrue(t, keysSvc.PutTier1("openai-default", []byte("sk-z")).OK)

	// Snapshot pre-delete event count so we can isolate the
	// invalidation-row that fires from the subscriber.
	preCount := len(rec.snapshot())

	core.AssertTrue(t, keysSvc.DeleteTier1("openai-default").OK)

	events := rec.snapshot()[preCount:]
	var invalidated *audit.Event
	for i := range events {
		if events[i].Event == audit.EventProviderCredentialCacheInvalidated {
			invalidated = &events[i]
			break
		}
	}
	core.AssertTrue(t, invalidated != nil,
		"EventProviderCredentialCacheInvalidated must fire post-DeleteTier1")
	core.AssertEqual(t, "openai-default", invalidated.Meta["ref"])
	core.AssertEqual(t, keys.Tier1KeyDeleted, invalidated.Meta["reason"])
}

// TestCredentials_CacheInvalidatedOnTier1Replace_Good — the
// PutTier1-replace half of the ADD-1 contract. Replacing an existing
// ref fires the cache_invalidated row with reason="replaced".
func TestCredentials_CacheInvalidatedOnTier1Replace_Good(t *core.T) {
	rec := installRecorder(t)
	_, keysSvc, runnerSvc := credentialFixture(t)

	core.AssertTrue(t, runnerSvc.WSetDynamicRoutes([]runner.RouteInput{
		{Name: "openai", Kind: "openai", BaseURL: "x", APIKeyRef: "openai-default", Model: "m"},
	}).OK)
	core.AssertTrue(t, keysSvc.PutTier1("openai-default", []byte("sk-original")).OK)
	preCount := len(rec.snapshot())

	// Replace — same ref, new bytes.
	core.AssertTrue(t, keysSvc.PutTier1("openai-default", []byte("sk-rotated")).OK)

	events := rec.snapshot()[preCount:]
	var invalidated *audit.Event
	for i := range events {
		if events[i].Event == audit.EventProviderCredentialCacheInvalidated {
			invalidated = &events[i]
			break
		}
	}
	core.AssertTrue(t, invalidated != nil,
		"EventProviderCredentialCacheInvalidated must fire post-replace")
	core.AssertEqual(t, "openai-default", invalidated.Meta["ref"])
	core.AssertEqual(t, keys.Tier1KeyReplaced, invalidated.Meta["reason"])
}

// TestCredentials_RouteSkippedWhenAccountLocked_Ugly — the lazy /
// deferred-resolution behaviour per RFC v1.1 §4: a route referencing
// a tier-1 ref that cannot be resolved (no KEK provider) is SKIPPED
// at build time rather than failing the whole load. Boot-friendly
// behaviour — the runner serves whatever can be resolved.
func TestCredentials_RouteSkippedWhenAccountLocked_Ugly(t *core.T) {
	c, _, _ := lockedFixture(t)

	cfgSvc, _ := core.ServiceFor[*config.Service](c, "config")
	core.AssertTrue(t, cfgSvc != nil)
	core.AssertTrue(t, cfgSvc.Set("routes", map[string]runner.RouteConfig{
		"openai": {
			Kind:      "openai",
			BaseURL:   "https://api.openai.com/v1",
			APIKeyRef: "ghost-ref-that-cannot-resolve",
			Model:     "gpt-4o-mini",
		},
	}).OK)
	core.AssertTrue(t, cfgSvc.Commit().OK)

	routes := runner.LoadRoutesFromCore(c)
	core.AssertEqual(t, 0, len(routes),
		"unresolvable APIKeyRef must defer the route, not panic / fail load")
}

// TestCredentials_MigrationStatus_PendingObservedAtBoot_Good — the
// boot-scan emit per RFC v1.1 §6 ADD-4: when plaintext credentials
// exist on disk at ServiceStartup, the migration-pending row fires
// with the observed count.
func TestCredentials_MigrationStatus_PendingObservedAtBoot_Good(t *core.T) {
	c, _, _ := lockedFixture(t)
	cfgSvc, _ := core.ServiceFor[*config.Service](c, "config")
	core.AssertTrue(t, cfgSvc != nil)

	core.AssertTrue(t, cfgSvc.Set("routes", map[string]runner.RouteConfig{
		"openai":    {Kind: "openai", BaseURL: "x", APIKey: "sk-1", Model: "gpt"},
		"anthropic": {Kind: "openai", BaseURL: "y", APIKey: "sk-2", Model: "claude"},
	}).OK)
	core.AssertTrue(t, cfgSvc.Commit().OK)

	rec := installRecorder(t)
	runnerSvc := runner.NewServiceFromCore(c)
	core.AssertTrue(t, runnerSvc.ServiceStartup(core.Background(), nil).OK)

	events := rec.snapshot()
	var observed *audit.Event
	for i := range events {
		if events[i].Event == audit.EventProviderCredentialMigrationPendingObserved {
			observed = &events[i]
			break
		}
	}
	core.AssertTrue(t, observed != nil,
		"EventProviderCredentialMigrationPendingObserved must fire when plaintext on disk at boot")
	core.AssertEqual(t, 2, observed.Meta["count"],
		"count Meta must reflect the observed plaintext-route total")
}

// TestCredentials_MigrateLegacyKeys_StripsPlaintext_Good — the
// MigrateLegacyKeys helper (intended to fire post-SetKEKProvider in
// cmd/lthn/app.go) must dispatch every plaintext route into tier-1
// and rewrite the config with APIKey empty + APIKeyRef populated.
func TestCredentials_MigrateLegacyKeys_StripsPlaintext_Good(t *core.T) {
	rec := installRecorder(t)
	c, keysSvc, _ := credentialFixture(t)

	cfgSvc, _ := core.ServiceFor[*config.Service](c, "config")
	core.AssertTrue(t, cfgSvc != nil, "config service must be wired by the fixture")
	core.AssertTrue(t, cfgSvc.Set("routes", map[string]runner.RouteConfig{
		"openai": {Kind: "openai", BaseURL: "x", APIKey: "sk-legacy", Model: "gpt"},
	}).OK)
	core.AssertTrue(t, cfgSvc.Commit().OK)

	migrated := runner.MigrateLegacyKeys(c)
	core.AssertEqual(t, 1, migrated, "one plaintext route must migrate")

	// Tier-1 substrate now holds the credential.
	hasR := keysSvc.HasTier1("openai-migrated")
	core.AssertTrue(t, hasR.OK)
	core.AssertEqual(t, true, hasR.Value.(bool))

	// Config no longer carries plaintext.
	var raw map[string]runner.RouteConfig
	core.AssertTrue(t, cfgSvc.Get("routes", &raw).OK)
	rc := raw["openai"]
	core.AssertEqual(t, "", rc.APIKey, "plaintext must be stripped post-migration")
	core.AssertEqual(t, "openai-migrated", rc.APIKeyRef)

	// Migration audit row with source="config".
	events := rec.snapshot()
	var migratedEv *audit.Event
	for i := range events {
		if events[i].Event == audit.EventProviderCredentialMigrated {
			migratedEv = &events[i]
			break
		}
	}
	core.AssertTrue(t, migratedEv != nil,
		"EventProviderCredentialMigrated must fire from MigrateLegacyKeys")
	core.AssertEqual(t, "config", migratedEv.Meta["source"])
	core.AssertEqual(t, "openai", migratedEv.Meta["provider"])
}
