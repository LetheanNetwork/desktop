// SPDX-Licence-Identifier: EUPL-1.2

// HTTP + Wails thread-through tests for the UpgradeInput consent gate
// (Mantis #1623, follow-on to Cerberus #22 MED-2 / Mantis #1619).
//
// Before #1623 the HTTP handler at /v1/api/opencode/upgrade and the
// Wails binding WUpgrade both called the parameterless Service.Upgrade
// which, post-#1619, fail-closes with "upgrade.requires_confirmation"
// — i.e. every upgrade attempt failed. These tests pin the body /
// parameter wiring so:
//
//   1. The HTTP handler decodes the JSON body into UpgradeInput, threads
//      it through to Service.UpgradeWithConsent, and a missing /
//      ConfirmedByUser=false body surfaces as 400 Bad Request with
//      audit outcome=denied (caller-supplied request rejected, distinct
//      from substrate failure which stays outcome=error / 500).
//   2. The Wails WUpgrade(in UpgradeInput) binding threads the input
//      through to Service.UpgradeWithConsent verbatim — a zero
//      UpgradeInput{} reaches the consent gate (matching the legacy
//      Upgrade() fail-closed contract), and an UpgradeInput with
//      ConfirmedByUser=true passes the gate.
//
// "Good" success-path tests prove gate-passed rather than full
// docker-pull integration — the Service requires a process service
// + container runtime that we don't stand up here. The proof is that
// the substrate error surfaced is "process service unavailable"
// (i.e. the gate let the call through to the proc lookup) rather than
// "upgrade.requires_confirmation" (gate-blocked). The service-tier
// integration test that exercises a real pull lives elsewhere.

package opencode

import (
	"bytes"
	"net/http/httptest"
	"strings"
	"testing"

	core "dappco.re/go"
	"github.com/gin-gonic/gin"

	"dappco.re/lthn/desktop/pkg/audit"
)

// runUpgradeHTTP wires the REAL ControlGroup.upgrade handler against a
// stub Service (&Service{} — proc() returns nil), captures audit events,
// and returns the response + captured slice.
//
// Body is the raw HTTP body bytes; pass nil for "no body" (the
// gate-fires case).
//
// Usage example:
//
//	w, events := runUpgradeHTTP(t, []byte(`{"confirmed_by_user":true}`))
//	if w.Code != core.StatusInternalServerError { … }
func runUpgradeHTTP(t *testing.T, body []byte) (*httptest.ResponseRecorder, []audit.Event) {
	t.Helper()
	fake := &inMemoryRecorder{}
	audit.SetDefault(fake)
	t.Cleanup(func() { audit.SetDefault(nil) })

	g := NewControlGroup(&Service{})
	gin.SetMode(gin.TestMode)
	e := gin.New()
	e.POST("/upgrade", g.upgrade)

	var req = httptest.NewRequest(core.MethodPost, "/upgrade", nil)
	if body != nil {
		req = httptest.NewRequest(core.MethodPost, "/upgrade", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)
	return w, fake.snapshot()
}

// TestUpgradeHTTP_RequiresConsentBody_Bad — POST with no body MUST
// surface as 400 Bad Request with audit outcome=denied + error_code
// "upgrade.requires_confirmation". The empty-body case decodes to
// UpgradeInput{ConfirmedByUser: false} which the consent gate inside
// Service.UpgradeWithConsent refuses without any side effect. Before
// Mantis #1623 the handler called the legacy Upgrade() which produced
// the same refusal but as a 500 (substrate error) rather than a 400
// (caller-supplied request rejected) — this test pins the distinction
// so the frontend can render a "please confirm" dialog instead of a
// "something is broken" error.
func TestUpgradeHTTP_RequiresConsentBody_Bad(t *testing.T) {
	w, events := runUpgradeHTTP(t, nil)
	if w.Code != core.StatusBadRequest {
		t.Fatalf("status = %d; want 400 (consent-gate refusal is a 4xx, not a 5xx — body=%q)",
			w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "upgrade.requires_confirmation") {
		t.Errorf("body = %q; want substring %q", w.Body.String(), "upgrade.requires_confirmation")
	}
	ev := assertOneEvent(t, events, EventOpencodeUpgrade, "opencode.upgrade", audit.OutcomeDenied)
	if got, _ := ev.Meta["error_code"].(string); got != "upgrade.requires_confirmation" {
		t.Errorf("Meta.error_code = %v; want %q", ev.Meta["error_code"], "upgrade.requires_confirmation")
	}
}

// TestUpgradeHTTP_RequiresConsentBody_FalseFlag_Bad — POST with an
// explicit `{"confirmed_by_user": false}` body MUST also surface as
// 400 + outcome=denied. The shape proves the JSON decoder is wired
// (not just that the empty-body path works), and that an explicit
// "no" is treated identically to an absent confirmation.
func TestUpgradeHTTP_RequiresConsentBody_FalseFlag_Bad(t *testing.T) {
	w, events := runUpgradeHTTP(t, []byte(`{"confirmed_by_user": false}`))
	if w.Code != core.StatusBadRequest {
		t.Fatalf("status = %d; want 400 — body=%q", w.Code, w.Body.String())
	}
	assertOneEvent(t, events, EventOpencodeUpgrade, "opencode.upgrade", audit.OutcomeDenied)
}

// TestUpgradeHTTP_WithConsent_Good — POST `{"confirmed_by_user":
// true}` MUST decode the body into UpgradeInput and thread it through
// to Service.UpgradeWithConsent. Proof-of-wiring: the consent gate
// does NOT fire (the 400 / outcome=denied path is NOT taken), and the
// substrate error that does surface is the "process service
// unavailable" one from proc()==nil (i.e. the gate was passed and the
// call reached the substrate). The full pull integration is exercised
// by the service-tier test that stands up a fake process runtime; here
// we only pin the wiring.
//
// Pins Mantis #1623: any regression that reverted the handler to call
// the legacy parameterless Upgrade() would re-fail with
// "upgrade.requires_confirmation" + 400 here, surfacing immediately.
func TestUpgradeHTTP_WithConsent_Good(t *testing.T) {
	w, events := runUpgradeHTTP(t, []byte(`{"confirmed_by_user": true}`))
	// Consent gate MUST have passed — body must NOT carry the gate
	// refusal string. The substrate-unavailable error is the expected
	// 500 surface for a Service{} with no proc backing.
	if w.Code == core.StatusBadRequest {
		t.Fatalf("status = 400 (consent gate fired) — body MUST have been "+
			"decoded + ConfirmedByUser=true threaded through; body=%q", w.Body.String())
	}
	if strings.Contains(w.Body.String(), "upgrade.requires_confirmation") {
		t.Fatalf("body carries consent-gate refusal — handler did not "+
			"thread the JSON body through to UpgradeWithConsent. body=%q",
			w.Body.String())
	}
	// One audit row, outcome MUST be error (substrate path), not denied
	// (gate path) — distinguishes "wiring landed" from "wiring missing".
	if len(events) != 1 {
		t.Fatalf("want exactly 1 audit event; got %d: %+v", len(events), events)
	}
	if events[0].Outcome == audit.OutcomeDenied {
		t.Errorf("audit outcome = denied; want error (consent gate fired — "+
			"body decode did not reach Service.UpgradeWithConsent). meta=%+v",
			events[0].Meta)
	}
}

// TestUpgradeWails_LegacyWUpgrade_FailClosed_Bad — the zero-arg
// WUpgrade() legacy entry point MUST fail-close with
// "upgrade.requires_confirmation". Preserved post-Mantis #1623 so the
// pre-#1623 frontend caller still compiles, but the underlying
// Service.Upgrade now equals UpgradeWithConsent(UpgradeInput{}) which
// the consent gate refuses without side effects.
//
// Documents the shim contract: the legacy method is a compile-only
// preservation. The frontend dialog lane is expected to migrate to
// WUpgradeWithConsent; once that lands this entry point becomes a
// removal candidate.
func TestUpgradeWails_LegacyWUpgrade_FailClosed_Bad(t *testing.T) {
	w := NewWailsService(&Service{})
	r := w.WUpgrade()
	if r.OK {
		t.Fatalf("legacy WUpgrade() returned OK; want fail-closed Fail " +
			"(Cerberus #22 MED-2 / Mantis #1619 + #1623)")
	}
	if got := r.Error(); !strings.Contains(got, "upgrade.requires_confirmation") {
		t.Errorf("legacy WUpgrade() error = %q; want substring %q",
			got, "upgrade.requires_confirmation")
	}
}

// TestUpgradeWails_RequiresConsentParam_Bad —
// WUpgradeWithConsent(UpgradeInput{}) MUST return Fail with
// "upgrade.requires_confirmation". The zero UpgradeInput defaults to
// ConfirmedByUser=false which the underlying Service.UpgradeWithConsent
// refuses at the gate without any side effect. Pins the new Wails
// surface added in Mantis #1623 + its thread-through to the gate.
func TestUpgradeWails_RequiresConsentParam_Bad(t *testing.T) {
	w := NewWailsService(&Service{})
	r := w.WUpgradeWithConsent(UpgradeInput{})
	if r.OK {
		t.Fatalf("WUpgradeWithConsent(UpgradeInput{}) returned OK; want Fail " +
			"(gate must refuse a non-confirming caller — Cerberus #22 MED-2)")
	}
	if got := r.Error(); !strings.Contains(got, "upgrade.requires_confirmation") {
		t.Errorf("WUpgradeWithConsent(UpgradeInput{}) error = %q; want substring %q",
			got, "upgrade.requires_confirmation")
	}
}

// TestUpgradeWails_WithConsent_Good — WUpgradeWithConsent(UpgradeInput{
// ConfirmedByUser: true}) MUST thread the input through to
// Service.UpgradeWithConsent. Proof-of-wiring: the consent gate
// does NOT fire (error string does NOT contain
// "upgrade.requires_confirmation"); the substrate error that does
// surface is "process service unavailable" from proc()==nil — i.e.
// the gate was passed and the call reached the substrate. The full
// pull integration is exercised by the service-tier test.
//
// Pins Mantis #1623: any regression that dropped the WUpgradeWithConsent
// thread-through — or dropped the ConfirmedByUser field on the input —
// would re-fail with the gate refusal here, surfacing the wiring
// regression at PR time.
func TestUpgradeWails_WithConsent_Good(t *testing.T) {
	w := NewWailsService(&Service{})
	r := w.WUpgradeWithConsent(UpgradeInput{ConfirmedByUser: true})
	if r.OK {
		// A &Service{} cannot produce a successful pull (no proc
		// backing) — if we somehow saw OK here something else is
		// wrong. Treat as test-environment hazard, not a wiring
		// regression.
		t.Fatalf("WUpgradeWithConsent(UpgradeInput{ConfirmedByUser: true}) returned " +
			"OK against a stub Service — expected proc-unavailable failure")
	}
	got := r.Error()
	if strings.Contains(got, "upgrade.requires_confirmation") {
		t.Fatalf("WUpgradeWithConsent(UpgradeInput{ConfirmedByUser: true}) error "+
			"= %q; want gate-PASS (any substrate error) — the consent flag was "+
			"NOT threaded through to Service.UpgradeWithConsent. Mantis #1623 "+
			"regression.", got)
	}
}
