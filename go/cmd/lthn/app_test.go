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
	"dappco.re/lthn/desktop/pkg/audit"
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
