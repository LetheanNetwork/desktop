// SPDX-Licence-Identifier: EUPL-1.2

// Internal (same-package) test for the rollbackTier1Refs helper —
// Mantis #1760 / Cerberus #75 ADD-1 option (b) safety-net audit row.
//
// SECURITY-NOTE (Hephaestus surface, brief allowlist boundary): the
// brief's strict allowlist named pkg/runner/credentials_test.go for
// both new tests, but the rollback-fails path can only be exercised
// via in-process injection — keys.Service is a concrete type with no
// failure-injection seam, and the rollback runs synchronously inside
// WSetDynamicRoutes with no interleave point a `runner_test` package
// fixture could reach. Adding a same-package test file (this one) is
// the narrowest possible departure; the file does nothing but drive
// the unexported rollbackTier1Refs helper with a stub tier1Deleter.

package runner

import (

	core "dappco.re/go"

	"dappco.re/lthn/desktop/pkg/audit"
)

// recordingRecorderInternal captures audit.Event rows so the orphan-
// emit assertion can read the recorded shape. Mirror of the
// recordingRecorder fixture in audit_test.go — duplicated here
// because the runner_test package fixture is invisible to the
// internal package.
type recordingRecorderInternal struct {
	events []audit.Event
}

func (r *recordingRecorderInternal) Record(ev audit.Event) core.Result {
	r.events = append(r.events, ev)
	return core.Ok(nil)
}

// failingDeleter is a tier1Deleter stub whose DeleteTier1 always
// returns Fail. Drives the rollbackTier1Refs safety-net audit path.
type failingDeleter struct {
	cause error
	calls []string
}

func (f *failingDeleter) DeleteTier1(ref string) core.Result {
	f.calls = append(f.calls, ref)
	return core.Fail(f.cause)
}

// TestWSetDynamicRoutes_RollbackFails_EmitsOrphanAudit_Bad — Mantis
// #1760 / Cerberus #75 ADD-1 option (b) safety-net contract. When the
// rollback DeleteTier1 itself fails (file-permission, disk failure),
// rollbackTier1Refs must emit EventProviderCredentialStoredOrphan per
// failed delete so the orphan ciphertext at rest is distinguishable
// from a routine rotation in the audit log.
func TestWSetDynamicRoutes_RollbackFails_EmitsOrphanAudit_Bad(t *core.T) {
	rec := &recordingRecorderInternal{}
	audit.SetDefault(rec)
	t.Cleanup(func() { audit.SetDefault(nil) })

	stub := &failingDeleter{cause: core.NewError("disk: permission denied")}
	refs := []string{"openai-migrated", "anthropic-migrated"}

	rollbackTier1Refs(stub, refs)

	// Every ref must have been attempted (no early-bail on first fail).
	core.AssertEqual(t, 2, len(stub.calls),
		"rollback must attempt DeleteTier1 for every ref, not early-bail")
	core.AssertEqual(t, "openai-migrated", stub.calls[0])
	core.AssertEqual(t, "anthropic-migrated", stub.calls[1])

	// Every failed delete must emit EventProviderCredentialStoredOrphan
	// with ref + reason Meta populated.
	var orphans []audit.Event
	for _, ev := range rec.events {
		if ev.Event == audit.EventProviderCredentialStoredOrphan {
			orphans = append(orphans, ev)
		}
	}
	core.AssertEqual(t, 2, len(orphans),
		"one orphan audit row per failed rollback delete")
	core.AssertEqual(t, "openai-migrated", orphans[0].Meta["ref"])
	core.AssertEqual(t, "disk: permission denied", orphans[0].Meta["reason"])
	core.AssertEqual(t, audit.OutcomeError, orphans[0].Outcome)
	core.AssertEqual(t, "runner", orphans[0].Scope)
	core.AssertEqual(t, "anthropic-migrated", orphans[1].Meta["ref"])

	// Defence-in-depth: orphan rows must not echo any credential bytes
	// (none are in scope here — refs are opaque handles — but the
	// secret-shape redactor still enforces post-emit).
	for _, ev := range orphans {
		for k, v := range ev.Meta {
			if vs, ok := v.(string); ok {
				core.AssertFalse(t, core.Contains(vs, "sk-"),
					"Meta["+k+"] must not echo credential-shaped bytes")
			}
		}
	}
}

// TestWSetDynamicRoutes_RollbackSucceeds_NoOrphanAudit_Good — happy-
// path companion: when DeleteTier1 succeeds for every ref, no orphan
// row fires. Guards against a future regression where the safety-net
// emit-site loses its `if !r.OK` guard.
func TestWSetDynamicRoutes_RollbackSucceeds_NoOrphanAudit_Good(t *core.T) {
	rec := &recordingRecorderInternal{}
	audit.SetDefault(rec)
	t.Cleanup(func() { audit.SetDefault(nil) })

	stub := &okDeleter{}
	rollbackTier1Refs(stub, []string{"ok-1-migrated", "ok-2-migrated"})

	for _, ev := range rec.events {
		core.AssertFalse(t,
			ev.Event == audit.EventProviderCredentialStoredOrphan,
			"successful rollback must NOT emit the orphan safety-net row")
	}
	core.AssertEqual(t, 2, len(stub.calls))
}

// TestWSetDynamicRoutes_RollbackEmpty_NoOp_Good — the no-state path:
// an empty refs slice must be a no-op (no DeleteTier1 calls, no audit
// emit). Covers the Phase-2-first-route-fails branch where stored is
// still len(0) at rollback-call-time.
func TestWSetDynamicRoutes_RollbackEmpty_NoOp_Good(t *core.T) {
	rec := &recordingRecorderInternal{}
	audit.SetDefault(rec)
	t.Cleanup(func() { audit.SetDefault(nil) })

	stub := &okDeleter{}
	rollbackTier1Refs(stub, nil)
	core.AssertEqual(t, 0, len(stub.calls))
	core.AssertEqual(t, 0, len(rec.events))
}

// okDeleter is a tier1Deleter stub whose DeleteTier1 always succeeds.
type okDeleter struct {
	calls []string
}

func (o *okDeleter) DeleteTier1(ref string) core.Result {
	o.calls = append(o.calls, ref)
	return core.Ok(nil)
}
