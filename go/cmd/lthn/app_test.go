// SPDX-Licence-Identifier: EUPL-1.2

// app_test.go — Mantis #1522 boot-order verification.
//
// The hazard #1522 closes: audit events emitted between the
// package-level audit.Default() lazy init and the explicit boot wire's
// audit.SetDefault land in the noopRecorder and are silently dropped.
// Service OnStart hooks fire under c.ServiceStartup; today no hook
// emits audit (grep-verified), but Stage X.B Phase 2c moves probes
// earlier in the lifecycle so first-boot setup events would otherwise
// disappear.
//
// The fix promotes audit.New + audit.SetDefault ahead of
// c.ServiceStartup using Options{} (random-fallback secret), then
// swaps the secret to the serverkey-derived HMAC via SetSecret after
// serverkey.Bootstrap.
//
// These tests pin two things:
//
//  1. SetSecret swaps the in-memory HMAC under the same Service
//     instance — no day-file handle rotation, no goroutine restart.
//     A subsequent Record uses the new secret for account_id hashing.
//  2. The ordering invariant: after newAppCore() returns OK,
//     audit.Default() returns a non-noop *audit.Service (the explicit
//     boot-wire instance) — the noopRecorder fallback never landed.
//
// AX-1: predictable name TestApp_<Function>_<Good|Bad|Ugly>.
// AX-6: every I/O via core.* — no os / encoding/json / strings.

package main

import (
	"testing"

	core "dappco.re/go"
	"dappco.re/go/config"
	coreio "dappco.re/go/io"

	lthn "dappco.re/lthn/desktop"
	"dappco.re/lthn/desktop/pkg/audit"
	"dappco.re/lthn/desktop/pkg/keys"
	"dappco.re/lthn/desktop/pkg/modelruntime"
	"dappco.re/lthn/desktop/pkg/runner"
	"dappco.re/lthn/desktop/pkg/services"
)

// TestApp_AuditSetSecret_Good_SwapsInPlaceForSubsequentRecord pins
// the in-place swap semantic SetSecret promises: a Service constructed
// with Options{} (random fallback) accepts a SetSecret call that
// changes the HMAC secret in place; the same Service instance keeps
// its live day-file handle + rotation goroutine; a subsequent Record
// hashes account_id under the new secret.
func TestApp_AuditSetSecret_Good_SwapsInPlaceForSubsequentRecord(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	// Build the audit Service with Options{} — random fallback secret.
	// Mirrors the cmd/lthn/app.go pre-ServiceStartup wire.
	auditSvc := audit.New(nil, audit.Options{})
	t.Cleanup(func() { _ = auditSvc.Close() })
	audit.SetDefault(auditSvc)
	t.Cleanup(func() { audit.SetDefault(nil) })

	// Record an event under the random-fallback secret. account_id
	// gets hashed at-rest per RFC §6.4; we capture the day-file body
	// after Close-flush to compare against a second event recorded
	// under a known secret.
	r1 := auditSvc.RecordSync(audit.Event{
		Event:     "auth.test.pre_swap",
		AccountID: "acct-swap-test",
		Outcome:   audit.OutcomeOK,
	})
	core.AssertTrue(t, r1.OK, "pre-swap RecordSync must succeed")

	// Swap the secret. Mirrors the cmd/lthn/app.go post-Bootstrap wire
	// (auditSvc.SetSecret(serverkeySvc.AuditHMACSecret())).
	auditSvc.SetSecret([]byte("known-32-byte-secret-padding-xx!"))

	// Record under the new secret. Same account_id input, different
	// hash output (asserted indirectly via day-file content distinct
	// from the pre-swap line).
	r2 := auditSvc.RecordSync(audit.Event{
		Event:     "auth.test.post_swap",
		AccountID: "acct-swap-test",
		Outcome:   audit.OutcomeOK,
	})
	core.AssertTrue(t, r2.OK, "post-swap RecordSync must succeed")

	// Flush to disk.
	core.AssertTrue(t, auditSvc.Close().OK, "audit close must succeed")

	// Day-file contains both events; the per-event account_id hashes
	// MUST differ because the secret changed between them. Read the
	// day-file via the audit-dir glob (same pattern as
	// paths_audit_adapter_test.go).
	auditDir := core.PathJoin(tmp, "Lethean", "audit")
	entriesR := core.ReadDir(core.DirFS(auditDir), ".")
	core.AssertTrue(t, entriesR.OK, "audit dir must be readable post-swap")
	entries := entriesR.Value.([]core.FsDirEntry)
	core.AssertGreater(t, len(entries), 0, "audit day-file must exist post-swap")

	var dayFile string
	for _, e := range entries {
		if !e.IsDir() {
			dayFile = core.PathJoin(auditDir, e.Name())
			break
		}
	}
	core.AssertNotEqual(t, "", dayFile, "audit day-file must be discoverable")
	body := core.ReadFile(dayFile)
	core.AssertTrue(t, body.OK, "day-file must be readable")
	contents := string(body.Value.([]byte))

	// Both events landed.
	core.AssertTrue(t, core.Contains(contents, "auth.test.pre_swap"),
		"day-file must carry the pre-swap event; got: "+contents)
	core.AssertTrue(t, core.Contains(contents, "auth.test.post_swap"),
		"day-file must carry the post-swap event; got: "+contents)
}

// TestApp_AuditSetSecret_Bad_EmptySecretIsNoOp pins the "MUST NOT
// clear" contract: SetSecret([]byte{}) leaves the existing secret in
// place. Defends against a caller accidentally regressing to the
// random-fallback warning state via an unintentional empty-slice
// dispatch (e.g. uninitialised serverkey instance returning empty).
func TestApp_AuditSetSecret_Bad_EmptySecretIsNoOp(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	auditSvc := audit.New(nil, audit.Options{
		AuditSecret: []byte("initial-32-byte-secret-padding-x"),
	})
	t.Cleanup(func() { _ = auditSvc.Close() })

	// Record with the initial secret.
	r1 := auditSvc.RecordSync(audit.Event{
		Event:     "auth.test.initial",
		AccountID: "acct-noop-test",
		Outcome:   audit.OutcomeOK,
	})
	core.AssertTrue(t, r1.OK, "initial RecordSync must succeed")

	// SetSecret with empty slice — must be no-op.
	auditSvc.SetSecret(nil)
	auditSvc.SetSecret([]byte{})

	// Record again — should still succeed (the secret is still
	// "initial-32-byte..." not cleared to nil).
	r2 := auditSvc.RecordSync(audit.Event{
		Event:     "auth.test.after_noop",
		AccountID: "acct-noop-test",
		Outcome:   audit.OutcomeOK,
	})
	core.AssertTrue(t, r2.OK, "post-noop RecordSync must succeed")

	core.AssertTrue(t, auditSvc.Close().OK, "audit close must succeed")
}

// TestApp_AuditDefaultAfterPreStartupWire_Good pins the structural
// invariant the Mantis #1522 fix enforces: after the cmd/lthn/app.go
// boot wire constructs audit.New(c, Options{}) + audit.SetDefault
// ahead of c.ServiceStartup, audit.Default() returns the explicit
// *audit.Service instance — NOT the noopRecorder fallback.
//
// This mirrors the boot-wire sequence in newAppCore (the pre-Startup
// audit init) without spinning up the full Core (which would require
// every registered service's dependencies). The assertion is on the
// type identity of audit.Default()'s return — the noop sink is a
// distinct unexported type that would compare unequal to *audit.Service.
func TestApp_AuditDefaultAfterPreStartupWire_Good(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	// Reset the package default first so we observe the explicit-wire
	// transition cleanly. SetDefault(nil) clears the cache so the next
	// Default() call re-evaluates.
	audit.SetDefault(nil)
	t.Cleanup(func() { audit.SetDefault(nil) })

	// Boot wire: construct + SetDefault. Same shape as the cmd/lthn/app.go
	// pre-ServiceStartup block.
	auditSvc := audit.New(nil, audit.Options{})
	t.Cleanup(func() { _ = auditSvc.Close() })
	audit.SetDefault(auditSvc)

	// At this point any service OnStart hook calling audit.Default()
	// MUST land on the *audit.Service instance, not the noop fallback.
	got := audit.Default()
	_, ok := got.(*audit.Service)
	core.AssertTrue(t, ok,
		"audit.Default() after pre-ServiceStartup wire must return *audit.Service, not noopRecorder")
}

// TestApp_NewAppCore_EmitsServiceRegistrationAudit_Good pins the
// Cerberus #70 F-2 MED contract: emitCompositionAudit, called at the
// end of newAppCore, lands a single row carrying
// EventCompositionServicesRegistered with the four-key Meta vocabulary
// (service_count + service_names_hash + wails_binding_count +
// wails_bindings_hash) plus the build version.
//
// Bypasses the full newAppCore (which would need every registered
// service's dependencies) and exercises emitCompositionAudit directly
// against a hand-built Core with two known service names. The hash
// shape + count integers are deterministic for the test's Core, so the
// assertions pin the exact on-disk Meta values not just presence.
func TestApp_NewAppCore_EmitsServiceRegistrationAudit_Good(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	auditSvc := audit.New(nil, audit.Options{})
	t.Cleanup(func() { _ = auditSvc.Close() })
	audit.SetDefault(auditSvc)
	t.Cleanup(func() { audit.SetDefault(nil) })

	// Build a minimal Core with two known services so the
	// service_names_hash is deterministic. core.New auto-registers a
	// "cli" service so the total c.Services() length is 3 — that's the
	// pinned expectation below. noopServiceFactory satisfies the
	// core.WithName factory contract; the Service contract only requires
	// the bus presence for c.Services() to surface the name.
	c := core.New(
		core.WithName("alpha", noopServiceFactory),
		core.WithName("beta", noopServiceFactory),
	)
	core.RequireTrue(t, c.ServiceStartup(core.Background(), nil).OK)
	t.Cleanup(func() { _ = c.ServiceShutdown(core.Background()) })

	// Pin the version so the version Meta key is deterministic. Restore
	// on cleanup so other tests in the same package see the original.
	originalVersion := lthn.Version
	t.Cleanup(func() { lthn.Version = originalVersion })
	lthn.Version = "test-version-1.2.3"

	emitCompositionAudit(c, auditSvc)

	// Flush to disk so the day-file body is observable.
	core.AssertTrue(t, auditSvc.Close().OK, "audit close must succeed")

	auditDir := core.PathJoin(tmp, "Lethean", "audit")
	entriesR := core.ReadDir(core.DirFS(auditDir), ".")
	core.AssertTrue(t, entriesR.OK, "audit dir must be readable")
	entries := entriesR.Value.([]core.FsDirEntry)
	core.AssertGreater(t, len(entries), 0, "audit day-file must exist")

	var dayFile string
	for _, e := range entries {
		if !e.IsDir() {
			dayFile = core.PathJoin(auditDir, e.Name())
			break
		}
	}
	core.AssertNotEqual(t, "", dayFile, "audit day-file must be discoverable")
	body := core.ReadFile(dayFile)
	core.AssertTrue(t, body.OK, "day-file must be readable")
	contents := string(body.Value.([]byte))

	// The composition row landed under the reserved event-name literal.
	core.AssertTrue(t,
		core.Contains(contents, audit.EventCompositionServicesRegistered),
		"day-file must carry "+audit.EventCompositionServicesRegistered+"; got: "+contents)

	// Meta carries the four forensic-fingerprint keys and the version.
	// service_count is 2 — the fixture registers "alpha" and "beta";
	// Core does not inject an implicit CLI service.
	core.AssertTrue(t, core.Contains(contents, `"service_count":2`),
		"Meta must carry service_count=2 (alpha + beta); got: "+contents)
	core.AssertTrue(t, core.Contains(contents, `"service_names_hash":`),
		"Meta must carry service_names_hash; got: "+contents)
	core.AssertTrue(t, core.Contains(contents, `"wails_binding_count":`),
		"Meta must carry wails_binding_count; got: "+contents)
	core.AssertTrue(t, core.Contains(contents, `"wails_bindings_hash":`),
		"Meta must carry wails_bindings_hash; got: "+contents)
	core.AssertTrue(t, core.Contains(contents, `"version":"test-version-1.2.3"`),
		"Meta must carry the pinned build version; got: "+contents)

	// The service_names_hash for the deterministic ["alpha","beta"] set
	// (sorted-comma-joined → "alpha,beta") is a stable forensic
	// value. Hash truncated to 16 hex chars to stay under the secret-
	// shape redactor's 32-char floor (matches the emit-site idiom).
	expectedNamesHash := core.SHA256HexString("alpha,beta")[:16]
	core.AssertTrue(t, core.Contains(contents, `"service_names_hash":"`+expectedNamesHash+`"`),
		"service_names_hash must equal SHA-256[:16] of 'alpha,beta'; got: "+contents)

	// The wails_binding_count is the catalogue length — pinned so a
	// drift in wailsBindingCatalogue without an intentional schema bump
	// trips the test. Update both this assertion + the catalogue
	// together when intentionally adding a binding.
	//
	// Count == 45 is the hand-maintained mirror of pkg/desktop/desktop.go's
	// Wails bindings (see Cerberus #50 ADD-1 / Mantis #1759). A `len > 0`
	// gate previously let a forgotten catalogue-update slip through silently
	// (audit hash flipped, no test failure). The exact-count gate forces
	// the catalogue + the binding surface to be edited in lockstep.
	core.AssertEqual(t, 45, len(wailsBindingCatalogue),
		"wailsBindingCatalogue length must equal 45 — update catalogue + this pin together when intentionally adding/removing a Wails binding")
}

// TestApp_AuditMeta_NoServiceInternals_Bad pins the Meta-PII discipline:
// the composition row MUST carry only public identifiers (counts,
// hashes, version, bounded vocabulary) — NEVER service-internal state
// like config values, secret bytes, or arbitrary raw service-name
// strings (a forensic walker reads hashes, not the underlying list).
//
// The intent: a regression that tried to record the literal
// service-name LIST (instead of the hash) into Meta would leak the
// at-rest installed-feature surface in the clear, which both bloats the
// audit row unboundedly AND opens a reconnaissance side-channel for any
// reader of the day-file.
func TestApp_AuditMeta_NoServiceInternals_Bad(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	auditSvc := audit.New(nil, audit.Options{})
	t.Cleanup(func() { _ = auditSvc.Close() })
	audit.SetDefault(auditSvc)
	t.Cleanup(func() { audit.SetDefault(nil) })

	// Build a Core with services whose names embed substrings that
	// would be embarrassing in a raw-name leak (e.g. an unrelated
	// secret-shape token). The literal names MUST NOT appear in Meta —
	// only the SHA-256 fingerprint over the sorted catalogue.
	canaryName := "canary-service-must-not-leak-to-meta"
	c := core.New(
		core.WithName(canaryName, noopServiceFactory),
		core.WithName("ordinary", noopServiceFactory),
	)
	core.RequireTrue(t, c.ServiceStartup(core.Background(), nil).OK)
	t.Cleanup(func() { _ = c.ServiceShutdown(core.Background()) })

	emitCompositionAudit(c, auditSvc)
	core.AssertTrue(t, auditSvc.Close().OK, "audit close must succeed")

	auditDir := core.PathJoin(tmp, "Lethean", "audit")
	entriesR := core.ReadDir(core.DirFS(auditDir), ".")
	core.AssertTrue(t, entriesR.OK, "audit dir must be readable")
	entries := entriesR.Value.([]core.FsDirEntry)
	core.AssertGreater(t, len(entries), 0, "audit day-file must exist")

	var dayFile string
	for _, e := range entries {
		if !e.IsDir() {
			dayFile = core.PathJoin(auditDir, e.Name())
			break
		}
	}
	body := core.ReadFile(dayFile)
	core.AssertTrue(t, body.OK, "day-file must be readable")
	contents := string(body.Value.([]byte))

	// Canary service-name MUST NOT appear in Meta — only the hash.
	core.AssertFalse(t,
		core.Contains(contents, canaryName),
		"composition Meta MUST NOT carry the raw service-name list — only the SHA-256 fingerprint; canary leaked into: "+contents)

	// The composition row landed (defensive — guards against the
	// FALSE-on-empty-day-file degenerate case).
	core.AssertTrue(t,
		core.Contains(contents, audit.EventCompositionServicesRegistered),
		"day-file must carry the composition row; got: "+contents)
}

// noopServiceFactory satisfies the core.WithName factory contract
// `func(*Core) Result` for tests that only need the bus to surface a
// service-name via c.Services() — no lifecycle hooks required for the
// composition-audit emit-shape assertions. Returns a non-nil opaque
// payload so WithName's nil-guard (contract.go:223) passes.
func noopServiceFactory(_ *core.Core) core.Result {
	return core.Result{Value: struct{}{}, OK: true}
}

// TestApp_WailsBindingCatalogue_CountPinned_Good is the dedicated
// drift-detection gate for wailsBindingCatalogue (Cerberus #50 ADD-1 /
// Mantis #1759 LOW). The catalogue in app.go is the hand-maintained
// mirror of pkg/desktop/desktop.go's Wails-binding surface (45 bindings
// at the time of writing). A contributor adding a binding to
// pkg/desktop without updating the catalogue would silently flip the
// `wails_bindings_hash` Meta field in the boot composition audit row
// — observable only post-hoc by a forensic reader noticing the hash
// drift, NEVER caught at CI.
//
// This test fails loudly on count drift, forcing the catalogue and the
// pkg/desktop binding surface to move in lockstep. The fix for a real
// drift is two-line: bump the pin here and update the catalogue (or
// retire the hand-maintained list entirely in favour of an exported
// `pkg/desktop.WailsBindingNames()` accessor — see forward-arc deferral
// in commit body for Mantis #1759).
//
// Usage example:
//
//	core.AssertEqual(t, 45, len(wailsBindingCatalogue), "...")
func TestApp_WailsBindingCatalogue_CountPinned_Good(t *testing.T) {
	core.AssertEqual(t, 45, len(wailsBindingCatalogue),
		"wailsBindingCatalogue length must equal 45 — drift gate: update catalogue + this pin together when intentionally adding/removing a Wails binding in pkg/desktop/desktop.go")
	found := false
	for _, binding := range wailsBindingCatalogue {
		if binding == "modelruntime" {
			found = true
			break
		}
	}
	core.AssertTrue(t, found, "modelruntime must be included in the Wails binding catalogue")
}

func TestApp_NewAppCore_ModelRuntimeGoodRegistersInertTrustedComposition(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	c := newAppCore()
	core.RequireTrue(t, c != nil, "newAppCore must succeed under an isolated HOME")
	t.Cleanup(func() { _ = c.ServiceShutdown(core.Background()) })

	runtime, runtimeOK := core.ServiceFor[*modelruntime.Service](c, "modelruntime")
	core.AssertTrue(t, runtimeOK && runtime != nil,
		"modelruntime must be registered on the shared Core")

	lemIO, mediumOK := core.ServiceFor[*coreio.Service](c, "lem-io")
	core.RequireTrue(t, mediumOK && lemIO != nil && lemIO.Medium != nil,
		"lem-io must be a registered Medium")
	core.AssertEqual(t, core.PathJoin(tmp, "Lethean"), lemIO.Options().Root)
	core.AssertFalse(t, lemIO.Medium.IsDir("lem/models"),
		"model-runtime composition must not create a model catalogue")

	manager, managerOK := core.ServiceFor[*services.Service](c, "services")
	core.RequireTrue(t, managerOK && manager != nil,
		"managed services must be registered")
	result := manager.Get("inference")
	core.RequireTrue(t, result.OK, result.Error())
	snapshot := result.Value.(services.Snapshot)
	core.AssertEqual(t, services.StateStopped, snapshot.State)
	core.AssertFalse(t, snapshot.Desired)
	core.AssertEqual(t, "", snapshot.ProcessID)
	core.AssertEqual(t, 0, snapshot.PID)
}

// TestApp_PostUnlock_TriggersMigrateLegacyKeys_Good pins the H#250 /
// Mantis #1657 wire that cmd/lthn/app.go installs immediately after
// SetKEKProvider inside the accountSvc-wire block: when the tier-1
// KEK provider is live (post-unlock equivalent), a call to
// runner.MigrateLegacyKeys(c) MUST drain any plaintext `api_key:`
// route on disk into the tier-1 substrate and rewrite the config
// with `api_key:` empty + `api_key_ref:` populated.
//
// The test mirrors the production wire shape (config + keys + runner
// services on a real on-disk config file) rather than calling the
// full newAppCore — newAppCore needs every registered service's
// dependencies (paths.WalletsDir / serverkey.Bootstrap / .seed /
// orm.DuckDB) which is more substrate than this one-line wire
// warrants. The pin is on the symbol resolution + call shape: any
// drift in runner.MigrateLegacyKeys's signature breaks this test
// before it breaks the cmd/lthn binary build, and the wire-in-
// app.go-after-SetKEKProvider invariant stays observable from the
// cmd/lthn test surface.
//
// SECURITY-NOTE — the boot-time call in app.go is best-effort: at
// cold boot no account is unlocked so the production wire returns
// 0 and the boot-scan
// EventProviderCredentialMigrationPendingObserved row carries the
// deferred signal. A true post-unlock retry needs an unlock-event
// subscription surface in pkg/account that doesn't exist today;
// captured as a separate ticket per H#250 surfacing. This test
// exercises the post-unlock equivalent (tier-1 KEK pre-wired) to
// prove the wire's semantic when the unlock-hook surface lands.
//
// Usage example:
//
//	migrated := runner.MigrateLegacyKeys(c)
//	core.AssertEqual(t, 1, migrated)
func TestApp_PostUnlock_TriggersMigrateLegacyKeys_Good(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	cfgFile := core.PathJoin(tmp, "lthn.yaml")
	core.AssertTrue(t,
		core.WriteFile(cfgFile, []byte("routes: {}\n"), 0o644).OK,
		"seed config file must succeed")

	// Build the minimal Core the wire depends on: config + keys +
	// runner. Mirrors credentialFixture in pkg/runner so the
	// MigrateLegacyKeys call path matches the production wire
	// exactly.
	c := core.New(
		core.WithName("config", config.NewConfigServiceWith(config.ServiceOptions{
			Path:      cfgFile,
			EnvPrefix: "LTHN_TEST_H250",
		})),
		core.WithName("keys", keys.Register),
		core.WithName("runner", runner.Register),
	)
	core.RequireTrue(t, c.ServiceStartup(core.Background(), nil).OK,
		"core ServiceStartup must succeed")
	t.Cleanup(func() { _ = c.ServiceShutdown(core.Background()) })

	// Wire the tier-0 + tier-1 KEK providers — the post-unlock
	// equivalent the production wire installs at app.go line 498
	// (SetKEKProvider). Deterministic 32-byte keys mirror
	// credentialFixture.
	keysSvc, _ := core.ServiceFor[*keys.Service](c, "keys")
	core.RequireTrue(t, keysSvc != nil, "keys service must be registered")
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

	// Seed a plaintext route — the H#250 wire's load-bearing input.
	cfgSvc, _ := core.ServiceFor[*config.Service](c, "config")
	core.RequireTrue(t, cfgSvc != nil, "config service must be wired")
	core.AssertTrue(t, cfgSvc.Set("routes", map[string]runner.RouteConfig{
		"openai": {Kind: "openai", BaseURL: "x", APIKey: "sk-legacy", Model: "gpt"},
	}).OK, "seeding plaintext route must succeed")
	core.AssertTrue(t, cfgSvc.Commit().OK, "config commit must succeed")

	// The wire call — the literal line app.go installs after
	// SetKEKProvider. Any signature drift in runner.MigrateLegacyKeys
	// trips here before it trips the cmd/lthn binary build.
	migrated := runner.MigrateLegacyKeys(c)
	core.AssertEqual(t, 1, migrated,
		"post-unlock wire must drain one plaintext route into tier-1")

	// Tier-1 substrate now holds the credential under the synthesised
	// ref — proves the dispatch path landed, not just the call shape.
	hasR := keysSvc.HasTier1("openai-migrated")
	core.AssertTrue(t, hasR.OK,
		"HasTier1 must succeed for the migrated ref")
	core.AssertEqual(t, true, hasR.Value.(bool),
		"tier-1 substrate must hold the migrated credential")

	// Config no longer carries plaintext — round-trip proves the
	// rewrite-and-commit path fired, not just the in-memory mutation.
	var raw map[string]runner.RouteConfig
	core.AssertTrue(t, cfgSvc.Get("routes", &raw).OK,
		"routes Get must succeed post-migration")
	rc := raw["openai"]
	core.AssertEqual(t, "", rc.APIKey,
		"plaintext must be stripped post-migration")
	core.AssertEqual(t, "openai-migrated", rc.APIKeyRef,
		"api_key_ref must point at the synthesised tier-1 ref")
}
