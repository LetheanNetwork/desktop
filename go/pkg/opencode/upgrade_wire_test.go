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

// validUpgradeDigest is a canonical sha256:<64 lowercase hex> digest
// used across the _Good tests so the digest gate (Mantis #1621 / wired
// by #1630) passes and the call reaches the substrate. Any
// well-formed digest works — the proof is that we get past
// validSHA256Digest, not that the digest resolves to a real image.
const validUpgradeDigest = "sha256:ca59eb28d5ea6a1f50c45a1f1df5c1a9286343e41b389fe89fb4ffac96dbeb84"

// TestUpgradeHTTP_DigestRequired_Bad — POST `{"confirmed_by_user":
// true}` with NO image_digest field MUST surface as 400 Bad Request
// with audit outcome=denied + code "upgrade.digest_required". Pins
// Mantis #1630: the HTTP handler maps the digest-required gate
// refusal to the same 400 surface as requires_confirmation so the
// frontend can render "pick a release digest" rather than "the
// upgrade substrate broke".
//
// Order matters: consent gate fires first (#1619), so a body missing
// BOTH confirmation AND digest would surface as requires_confirmation
// — this test threads confirmed_by_user=true to drive the digest
// gate specifically.
func TestUpgradeHTTP_DigestRequired_Bad(t *testing.T) {
	w, events := runUpgradeHTTP(t, []byte(`{"confirmed_by_user": true}`))
	if w.Code != core.StatusBadRequest {
		t.Fatalf("status = %d; want 400 (digest-gate refusal is a 4xx — body=%q)",
			w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "upgrade.digest_required") {
		t.Errorf("body = %q; want substring %q", w.Body.String(), "upgrade.digest_required")
	}
	ev := assertOneEvent(t, events, EventOpencodeUpgrade, "opencode.upgrade", audit.OutcomeDenied)
	if got, _ := ev.Meta["error_code"].(string); got != "upgrade.digest_required" {
		t.Errorf("Meta.error_code = %v; want %q", ev.Meta["error_code"], "upgrade.digest_required")
	}
}

// TestUpgradeHTTP_DigestInvalid_Bad — POST with a malformed digest
// (missing sha256: prefix, wrong length, uppercase, etc.) MUST
// surface as 400 + audit outcome=denied + code
// "upgrade.digest_invalid". Distinct from digest_required so the
// frontend can route "you forgot to pick one" vs "what you sent is
// not a valid manifest digest" to different dialog branches. Pins
// Mantis #1630.
func TestUpgradeHTTP_DigestInvalid_Bad(t *testing.T) {
	w, events := runUpgradeHTTP(t, []byte(`{"confirmed_by_user": true, "image_digest": "deadbeef"}`))
	if w.Code != core.StatusBadRequest {
		t.Fatalf("status = %d; want 400 (digest-invalid refusal is a 4xx — body=%q)",
			w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "upgrade.digest_invalid") {
		t.Errorf("body = %q; want substring %q", w.Body.String(), "upgrade.digest_invalid")
	}
	ev := assertOneEvent(t, events, EventOpencodeUpgrade, "opencode.upgrade", audit.OutcomeDenied)
	if got, _ := ev.Meta["error_code"].(string); got != "upgrade.digest_invalid" {
		t.Errorf("Meta.error_code = %v; want %q", ev.Meta["error_code"], "upgrade.digest_invalid")
	}
}

// TestUpgradeHTTP_DigestValid_PassesGate_Good — POST with both
// confirmed_by_user=true AND a well-formed image_digest MUST decode
// the body into UpgradeInput and thread it through to
// Service.UpgradeWithConsent. Proof-of-wiring: BOTH gates pass — the
// body MUST NOT carry "upgrade.requires_confirmation" /
// "upgrade.digest_required" / "upgrade.digest_invalid"; the substrate
// error that surfaces is "process service unavailable" from
// proc()==nil (i.e. the gates were passed and the call reached the
// substrate). The full pull integration is exercised by the
// service-tier test.
//
// Pins Mantis #1623 + #1630: any regression that reverted the
// handler to call legacy Upgrade(), or stripped image_digest off the
// JSON decode, or stopped routing valid digests through the gate
// would surface here as a 400 + gate-code in the body.
func TestUpgradeHTTP_DigestValid_PassesGate_Good(t *testing.T) {
	body := []byte(`{"confirmed_by_user": true, "image_digest": "` + validUpgradeDigest + `"}`)
	w, events := runUpgradeHTTP(t, body)
	// Both gates MUST have passed — body must NOT carry any of the
	// gate refusal strings. The substrate-unavailable error is the
	// expected 500 surface for a Service{} with no proc backing.
	if w.Code == core.StatusBadRequest {
		t.Fatalf("status = 400 (a gate fired) — body MUST have been "+
			"decoded + ConfirmedByUser=true + ImageDigest=<valid> threaded "+
			"through; body=%q", w.Body.String())
	}
	for _, gate := range []string{
		"upgrade.requires_confirmation",
		"upgrade.digest_required",
		"upgrade.digest_invalid",
	} {
		if strings.Contains(w.Body.String(), gate) {
			t.Fatalf("body carries gate refusal %q — handler did not thread "+
				"the JSON body through to UpgradeWithConsent. body=%q",
				gate, w.Body.String())
		}
	}
	// One audit row, outcome MUST be error (substrate path), not denied
	// (gate path) — distinguishes "wiring landed" from "wiring missing".
	if len(events) != 1 {
		t.Fatalf("want exactly 1 audit event; got %d: %+v", len(events), events)
	}
	if events[0].Outcome == audit.OutcomeDenied {
		t.Errorf("audit outcome = denied; want error (a gate fired — "+
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

// TestUpgradeWails_DigestRequired_Bad —
// WUpgradeWithConsent(UpgradeInput{ConfirmedByUser: true}) with no
// ImageDigest MUST return Fail with "upgrade.digest_required". Pins
// Mantis #1630: the Wails binding threads ImageDigest through (or
// doesn't, when empty) to Service.UpgradeWithConsent, and the
// digest_required gate surfaces in the Result envelope so the
// frontend can render "pick a release digest" rather than swallow
// the failure as a generic substrate error.
//
// Distinct from TestUpgradeWails_RequiresConsentParam_Bad: there
// ConfirmedByUser=false fires the consent gate; here
// ConfirmedByUser=true passes consent and reaches the digest gate.
func TestUpgradeWails_DigestRequired_Bad(t *testing.T) {
	w := NewWailsService(&Service{})
	r := w.WUpgradeWithConsent(UpgradeInput{ConfirmedByUser: true})
	if r.OK {
		t.Fatalf("WUpgradeWithConsent(UpgradeInput{ConfirmedByUser:true}) returned OK; " +
			"want Fail (digest gate must refuse an empty ImageDigest — Mantis #1621/#1630)")
	}
	if got := r.Error(); !strings.Contains(got, "upgrade.digest_required") {
		t.Errorf("WUpgradeWithConsent(no digest) error = %q; want substring %q",
			got, "upgrade.digest_required")
	}
}

// TestUpgradeWails_DigestInvalid_Bad —
// WUpgradeWithConsent(UpgradeInput{ConfirmedByUser: true, ImageDigest:
// "deadbeef"}) MUST return Fail with "upgrade.digest_invalid".
// Distinct error code from digest_required so the frontend can route
// "you forgot to pick" vs "what you sent is malformed" to different
// dialog branches. Pins Mantis #1630.
func TestUpgradeWails_DigestInvalid_Bad(t *testing.T) {
	w := NewWailsService(&Service{})
	r := w.WUpgradeWithConsent(UpgradeInput{
		ConfirmedByUser: true,
		ImageDigest:     "deadbeef",
	})
	if r.OK {
		t.Fatalf("WUpgradeWithConsent(invalid digest) returned OK; want Fail " +
			"(digest gate must reject non-sha256:<64hex> — Mantis #1621/#1630)")
	}
	if got := r.Error(); !strings.Contains(got, "upgrade.digest_invalid") {
		t.Errorf("WUpgradeWithConsent(invalid digest) error = %q; want substring %q",
			got, "upgrade.digest_invalid")
	}
}

// TestUpgradeWails_DigestValid_PassesGate_Good — WUpgradeWithConsent
// with both ConfirmedByUser=true AND a well-formed ImageDigest MUST
// thread the input through to Service.UpgradeWithConsent. Proof-of-
// wiring: BOTH gates pass — error string MUST NOT contain
// "upgrade.requires_confirmation" / "upgrade.digest_required" /
// "upgrade.digest_invalid"; the substrate error that surfaces is
// "process service unavailable" from proc()==nil (gate was passed and
// the call reached the substrate). Full pull integration lives in the
// service-tier test.
//
// Pins Mantis #1623 + #1630: a regression that dropped the
// ImageDigest field, or skipped passing it to UpgradeWithConsent,
// would re-fail with digest_required here.
func TestUpgradeWails_DigestValid_PassesGate_Good(t *testing.T) {
	w := NewWailsService(&Service{})
	r := w.WUpgradeWithConsent(UpgradeInput{
		ConfirmedByUser: true,
		ImageDigest:     validUpgradeDigest,
	})
	if r.OK {
		// A &Service{} cannot produce a successful pull (no proc
		// backing) — if we somehow saw OK here something else is
		// wrong. Treat as test-environment hazard, not a wiring
		// regression.
		t.Fatalf("WUpgradeWithConsent(valid digest) returned OK against a stub " +
			"Service — expected proc-unavailable failure")
	}
	got := r.Error()
	for _, gate := range []string{
		"upgrade.requires_confirmation",
		"upgrade.digest_required",
		"upgrade.digest_invalid",
	} {
		if strings.Contains(got, gate) {
			t.Fatalf("WUpgradeWithConsent(valid digest) error = %q; "+
				"carries gate refusal %q — ImageDigest was NOT threaded "+
				"through to Service.UpgradeWithConsent. Mantis #1630 regression.",
				got, gate)
		}
	}
}
