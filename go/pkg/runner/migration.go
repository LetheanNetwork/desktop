// SPDX-Licence-Identifier: EUPL-1.2

// Provider-credential migration + cache-invalidation orchestration
// per Mantis #1657 / RFC.provider-creds-tier1.md v1.1 §4 + §6.
//
// Two distinct lifecycles live here:
//
//  1. Auto-import-and-strip (§6 Option A) — on first boot post-deploy,
//     every route with `api_key:` set + `api_key_ref:` empty gets its
//     plaintext lifted into the tier-1 substrate under a synthesised
//     ref ("<name>-migrated"), then the config is rewritten with
//     `api_key:` empty and `api_key_ref:` populated. Deferred when the
//     tier-1 KEK provider is not yet live; retries on the next call
//     to MigrateLegacyKeys (intended wire from cmd/lthn/app.go after
//     SetKEKProvider fires post-unlock).
//
//  2. Cache invalidation on tier-1 change (§4 ADD-1, Cerberus #1742)
//     — the runner subscribes to keys.Tier1KeyChanged event-bus
//     broadcasts. On each event the affected route is REBUILT against
//     the now-changed substrate state so the resolved credential held
//     in the *openai.Backend.cfg.APIKey field is replaced with the
//     fresh value (delete → backend removed; replace → backend re-
//     resolved). Audit emit fires on every cache-invalidation so a
//     forensic walker can correlate "I deleted X" with "backend Y
//     stopped serving requests at T".
//
// The "lazy resolve at first request" shape from RFC §4 Option A1 is
// pending an upstream APIKeyResolver field on dappco.re/go/ai/
// providers/openai.Config. Until that lands, the substrate-port
// here resolves credentials EAGERLY at route-build time and rebuilds
// on every keys.Tier1KeyChanged event — equivalent observable
// behaviour at the cache-flush boundary, with the trade-off that the
// resolved plaintext lives in *openai.Backend.cfg.APIKey for the
// route's lifetime rather than being scoped per-request. SECURITY-
// NOTE: log as a CoreGO export gap for the lazy-resolver field per
// project_corego_export_gaps.

package runner

import (

	core "dappco.re/go"
	"dappco.re/go/ai/ai"

	"dappco.re/lthn/desktop/pkg/audit"
	"dappco.re/lthn/desktop/pkg/keys"
)

// migrationRefSuffix is the synthesised-ref tail appended to the route
// name when the auto-import-and-strip latch lifts plaintext into the
// tier-1 substrate (RFC v1.1 §6). E.g. a route named "openai" with
// plaintext on disk becomes ref "openai-migrated" post-import. Single
// declaration so the read-side (RoutesReferencing, MigrationStatus)
// stays in lockstep with the write-side.
const migrationRefSuffix = "-migrated"

// migrationSource enumerates the audit-emit `source` Meta discriminator
// for the two distinct provider-credential.{stored,migrated} entry
// paths. Exported as named consts so the emit-site reads literally
// rather than as magic strings and the parity-grep test can join 1:1
// against the audit-constants.ts mirror.
const (
	migrationSourceConfig     = "config"
	migrationSourceWailsInput = "wails_input"
)

// MigrateLegacyKeys scans the config for routes carrying plaintext
// `api_key:` fields, dispatches each plaintext into the tier-1
// substrate under a synthesised ref ("<name>-migrated"), and rewrites
// the config with the plaintext stripped + the ref populated. Returns
// the count of routes successfully migrated. Idempotent — already-
// migrated routes (empty APIKey, populated APIKeyRef) are skipped.
//
// Deferred-migration path: when the tier-1 KEK provider is not yet
// live (account locked), PutTier1 returns no_tier1_provider and the
// call short-circuits with the migration latch unmoved. The boot-time
// emit of EventProviderCredentialMigrationPendingObserved gives the
// banner UX the signal it needs while the deferral persists.
//
// Mantis #1657 / RFC v1.1 §6 — Option A auto-import-and-strip.
//
// Usage example (in cmd/lthn/app.go after SetKEKProvider fires):
//
//	migrated := runner.MigrateLegacyKeys(c)
//	core.Print(core.Stdout(), "migrated %d legacy provider credentials\n", migrated)
func MigrateLegacyKeys(c *core.Core) int {
	if c == nil {
		return 0
	}
	keysSvc, _ := core.ServiceFor[*keys.Service](c, "keys")
	if keysSvc == nil {
		return 0
	}
	routes := loadRouteConfigsFromCore(c)
	if len(routes) == 0 {
		return 0
	}
	migrated := 0
	changed := false
	for name, rc := range routes {
		if rc.APIKey == "" {
			continue
		}
		// If the operator hand-edited a ref AND left plaintext in
		// place, the ref takes precedence (loader contract per
		// RFC §3.1 — APIKeyRef wins when both populated). Strip
		// the now-redundant plaintext as a defence-in-depth
		// hygiene step even though no PutTier1 dispatch is needed.
		if rc.APIKeyRef != "" {
			rc.APIKey = ""
			routes[name] = rc
			changed = true
			continue
		}
		ref := name + migrationRefSuffix
		if r := keysSvc.PutTier1(ref, []byte(rc.APIKey)); !r.OK {
			// Tier-1 KEK provider not live (account locked) is
			// the dominant failure mode here. Bail without
			// mutating further entries so the next retry sees
			// the full set still queued.
			break
		}
		rc.APIKey = ""
		rc.APIKeyRef = ref
		routes[name] = rc
		changed = true
		migrated++
		_ = audit.Default().Record(audit.Event{
			Event:   audit.EventProviderCredentialMigrated,
			TS:      core.Now().Unix(),
			Scope:   "runner",
			Outcome: audit.OutcomeOK,
			Meta: map[string]any{
				"provider": name,
				"ref":      ref,
				"source":   migrationSourceConfig,
			},
		})
	}
	if changed {
		_ = saveRouteConfigsToCore(c, routes)
	}
	return migrated
}

// ScanMigrationStatus walks the on-disk routes map and returns the
// MigrationStatus shape the persistent banner consumes (RFC v1.1 §6
// ADD-4). The boot-time wire ALSO emits
// EventProviderCredentialMigrationPendingObserved when the count is
// positive — gives external observability the same signal the
// operator sees in the banner.
//
// Pure scan — does not touch keys.Service or attempt resolution.
// Distinct from MigrateLegacyKeys which performs the actual at-rest
// dispatch.
//
// Usage example (boot-time emit):
//
//	st := runner.ScanMigrationStatus(c)
//	if st.PendingMigrationCount > 0 {
//	    runner.EmitMigrationPendingObserved(st)
//	}
func ScanMigrationStatus(c *core.Core) MigrationStatus {
	routes := loadRouteConfigsFromCore(c)
	out := MigrationStatus{LockedRoutes: []string{}}
	for name, rc := range routes {
		if rc.APIKey != "" && rc.APIKeyRef == "" {
			out.PendingMigrationCount++
			out.LockedRoutes = append(out.LockedRoutes, name)
		}
	}
	out.PlaintextOnDisk = out.PendingMigrationCount > 0
	return out
}

// EmitMigrationPendingObserved fires the per-boot
// EventProviderCredentialMigrationPendingObserved audit row when the
// status reports a non-zero pending count. No-op for the zero-pending
// path — the row is reserved for the deferred-migration signal, not
// the steady-state.
//
// Usage example:
//
//	runner.EmitMigrationPendingObserved(runner.ScanMigrationStatus(c))
func EmitMigrationPendingObserved(st MigrationStatus) {
	if st.PendingMigrationCount <= 0 {
		return
	}
	_ = audit.Default().Record(audit.Event{
		Event:   audit.EventProviderCredentialMigrationPendingObserved,
		TS:      core.Now().Unix(),
		Scope:   "runner",
		Outcome: audit.OutcomeOK,
		Meta: map[string]any{
			"count": st.PendingMigrationCount,
		},
	})
}

// RoutesReferencing returns the route names whose APIKeyRef equals
// the given ref. Used by Service.WForceDeleteTier1 to populate the
// `referenced_by` audit Meta + by the default WDeleteTier1 path to
// REJECT delete-when-referenced (RFC v1.1 §5 ADD-3).
//
// Usage example:
//
//	names := runner.RoutesReferencing(c, "openai-default")
//	if len(names) > 0 { /* reject delete or surface to UI */ }
func RoutesReferencing(c *core.Core, ref string) []string {
	if ref == "" {
		return nil
	}
	routes := loadRouteConfigsFromCore(c)
	var out []string
	for name, rc := range routes {
		if rc.APIKeyRef == ref {
			out = append(out, name)
		}
	}
	return out
}

// SubscribeToKeysEvents wires the runner-side subscriber for
// keys.Tier1KeyChanged broadcasts. Called from Service.ServiceStartup
// (Wails lifecycle hook) so the subscription is alive for the full
// runtime. On each event the runner re-resolves the affected route
// via RebuildStaticRoutes — credential delete drops the route from
// the router; credential replace re-resolves the plaintext into a
// fresh *openai.Backend.
//
// Audit emit on every event (EventProviderCredentialCacheInvalidated)
// — the audit row makes the cache-flush decision visible to a
// forensic walker regardless of whether the subsequent rebuild
// surfaces or skips the route.
//
// Mantis #1657 / RFC v1.1 §4 ADD-1 — Cerberus #1742 STRIDE-T
// survival-past-delete.
func SubscribeToKeysEvents(c *core.Core, s *Service) {
	if c == nil || s == nil {
		return
	}
	c.RegisterAction(func(c *core.Core, msg core.Message) core.Result {
		ev, ok := msg.(keys.Tier1KeyChanged)
		if !ok {
			return core.Ok(nil)
		}
		affected := RoutesReferencing(c, ev.Ref)
		// Always emit the audit row — even when no route currently
		// references the ref, the event is observable substrate
		// state ("operator deleted X; nothing pointed at it").
		// Per-affected-route emit so the join-key against the
		// inference cluster (provider/model) stays clean.
		if len(affected) == 0 {
			_ = audit.Default().Record(audit.Event{
				Event:   audit.EventProviderCredentialCacheInvalidated,
				TS:      core.Now().Unix(),
				Scope:   "runner",
				Outcome: audit.OutcomeOK,
				Meta: map[string]any{
					"provider": "",
					"ref":      ev.Ref,
					"reason":   ev.Kind,
				},
			})
		}
		for _, name := range affected {
			_ = audit.Default().Record(audit.Event{
				Event:   audit.EventProviderCredentialCacheInvalidated,
				TS:      core.Now().Unix(),
				Scope:   "runner",
				Outcome: audit.OutcomeOK,
				Meta: map[string]any{
					"provider": name,
					"ref":      ev.Ref,
					"reason":   ev.Kind,
				},
			})
		}
		// Rebuild static routes against the now-changed substrate
		// state. Delete drops the route from the router; replace
		// re-resolves the plaintext into a fresh backend. Skipped
		// when no route references the ref (cheap fast-path).
		if len(affected) > 0 {
			RebuildStaticRoutes(c, s)
		}
		return core.Ok(nil)
	})
}

// RebuildStaticRoutes re-reads the on-disk routes map, re-resolves
// every credential ref against the current keys.Service state, and
// installs the resulting ai.ProviderRoute slice as the service's
// static set. The dynamic set (ApplyDynamicRoutes — opencode sandbox
// routes) is preserved and merged on top, matching the construction-
// time shape.
//
// Used by:
//
//   - SubscribeToKeysEvents on keys.Tier1KeyChanged (cache-flush)
//   - cmd/lthn/app.go SetKEKProvider post-unlock hook (deferred-
//     migration drain — re-runs route build now that tier-1 can
//     resolve)
//
// Returns the count of resolved routes. Nil-safe.
func RebuildStaticRoutes(c *core.Core, s *Service) int {
	if c == nil || s == nil {
		return 0
	}
	routes := LoadRoutesFromCore(c)
	s.replaceStaticRoutes(routes)
	return len(routes)
}

// replaceStaticRoutes swaps the static route set on the runner and
// re-builds the ai.ProviderRouter against (static + dynamic) under the
// existing routerMu discipline (Cerberus #45 / Mantis #1656 — package-
// level function so Wails3's method-set reflection never binds it onto
// the renderer surface).
func (s *Service) replaceStaticRoutes(routes []ai.ProviderRoute) {
	if s == nil {
		return
	}
	s.routerMu.Lock()
	defer s.routerMu.Unlock()
	s.opts.Routes = routes
	merged := make([]ai.ProviderRoute, 0, len(s.opts.Routes)+len(s.dynamic))
	merged = append(merged, s.opts.Routes...)
	merged = append(merged, s.dynamic...)
	if len(merged) == 0 {
		s.router = nil
		return
	}
	built := ai.NewProviderRouter(merged...)
	if !built.OK {
		return
	}
	if router, ok := built.Value.(*ai.ProviderRouter); ok {
		s.router = router
	}
}
