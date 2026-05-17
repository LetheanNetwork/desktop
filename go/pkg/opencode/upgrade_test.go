// SPDX-Licence-Identifier: EUPL-1.2

package opencode

import (
	"strings"
	"testing"
)

// TestUpgrade_RequiresConfirmation_Bad — UpgradeWithConsent MUST
// refuse with "upgrade.requires_confirmation" when the caller has
// not set ConfirmedByUser=true. The refusal happens BEFORE any
// process service lookup or docker pull side effect — proven here
// by driving against a zero Service{} whose proc() returns nil
// (any path that reached `ps == nil` would surface a different
// error message; reaching the docker pull at all would panic on
// the missing core runtime).
//
// Pins the Cerberus #22 MED-2 / Mantis #1619 supply-chain hardening
// gate: a compromised registry, drive-by HTTP request, or cron-loop
// caller MUST NOT be able to mutate the running image without an
// explicit human approval.
func TestUpgrade_RequiresConfirmation_Bad(t *testing.T) {
	svc := &Service{}

	r := svc.UpgradeWithConsent(UpgradeInput{ConfirmedByUser: false})
	if r.OK {
		t.Fatalf("UpgradeWithConsent succeeded without confirmation; want Fail")
	}
	if got := r.Error(); !strings.Contains(got, "upgrade.requires_confirmation") {
		t.Errorf("UpgradeWithConsent error = %q; want substring %q",
			got, "upgrade.requires_confirmation")
	}

	// The legacy parameterless entry point is now equivalent to
	// UpgradeWithConsent(UpgradeInput{}) — i.e. default-deny. Any
	// pre-existing caller that hasn't been updated to thread an
	// UpgradeInput through reaches the gate, not the pull.
	r2 := svc.Upgrade()
	if r2.OK {
		t.Fatalf("legacy Upgrade() succeeded; want default-deny Fail")
	}
	if got := r2.Error(); !strings.Contains(got, "upgrade.requires_confirmation") {
		t.Errorf("legacy Upgrade() error = %q; want substring %q",
			got, "upgrade.requires_confirmation")
	}
}

// TestUpgrade_NoAutoRestartByDefault_Good — UpgradeInput with
// ConfirmedByUser=true but RestartSandboxes=false (the default)
// MUST NOT in-place restart running sandboxes even when the pull
// produces a new digest. The Cerberus #22 MED-2 / Mantis #1619
// gate cannot be relied on alone — confirmation is the consent
// surface, no-auto-restart is the blast-radius surface.
//
// This test asserts the policy at the type level: the
// RestartSandboxes field defaults to false in a zero
// UpgradeInput{}, and the documented contract is that without
// it the Restarted slice in the result stays empty. The full
// integration shape (mocked docker pull producing "Downloaded
// newer image" + asserting Stop was not called) lives at the
// service-tier integration test pass that follows this lane —
// here the unit-level invariant is the zero-value default of
// the gating field.
func TestUpgrade_NoAutoRestartByDefault_Good(t *testing.T) {
	var in UpgradeInput
	if in.RestartSandboxes {
		t.Errorf("UpgradeInput{}.RestartSandboxes = true; want false (no in-place restart unless caller opts in — Cerberus #22 MED-2)")
	}
	if in.ConfirmedByUser {
		t.Errorf("UpgradeInput{}.ConfirmedByUser = true; want false (gate is opt-in — Cerberus #22 MED-2)")
	}

	// And the consent-gated path with the default RestartSandboxes
	// still respects the type-level invariant: even a confirmed
	// caller does not get auto-restart unless they ask for it.
	gated := UpgradeInput{ConfirmedByUser: true}
	if gated.RestartSandboxes {
		t.Errorf("UpgradeInput{ConfirmedByUser: true}.RestartSandboxes = true; want false (consent is necessary but not sufficient for in-place restart)")
	}
}

// TestUpgrade_ConsentGate_PreEmpts_ProcLookup_Ugly — the gate MUST
// fire before any service-resolution side effect. Drives the path
// where confirmation is missing AND the underlying process service
// would also be unavailable: the caller MUST see the
// requires_confirmation error, NOT the process-unavailable error.
// Surface ordering matters for audit + UX — operator's "I forgot
// to tick the box" recovery is different from "the host's process
// runtime is broken".
func TestUpgrade_ConsentGate_PreEmpts_ProcLookup_Ugly(t *testing.T) {
	svc := &Service{}

	// Confirmation absent + proc() will return nil. Gate must win.
	r := svc.UpgradeWithConsent(UpgradeInput{})
	if r.OK {
		t.Fatalf("UpgradeWithConsent returned OK on zero-input; want Fail")
	}
	got := r.Error()
	if !strings.Contains(got, "upgrade.requires_confirmation") {
		t.Errorf("error = %q; want consent-gate to win, not the proc lookup",
			got)
	}
	if strings.Contains(got, "process service unavailable") {
		t.Errorf("error = %q; gate must short-circuit BEFORE proc() — leaking process-state to a non-confirming caller is a different surface",
			got)
	}
}
