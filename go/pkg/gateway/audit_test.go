// SPDX-Licence-Identifier: EUPL-1.2

// Tests for the Cerberus #57 F-2 (Mantis #1700) Repudiation gap close:
// pkg/gateway.Service.Handle now emits typed audit.EventGateway*
// rows via audit.Default() so the auditor sees plugin → host dispatch
// decisions instead of forensically silent gateway calls. Mirror of
// pkg/marketplace/audit_test.go (H#208 Cerberus #54 C4) — the
// recordingRecorder shape is duplicated rather than shared because
// each test package's SetDefault is process-global and the fixture
// must not leak across packages running in parallel.
//
// Test matrix:
//
//   - TestGateway_Dispatch_EmitsAuditTrail_Good — happy path drives
//     a registered scope to OK and asserts the Requested + Succeeded
//     pair lands with the expected Meta shape (no Rejected, no Failed).
//
//   - TestGateway_AuditMeta_NoRawSecrets_Bad — smuggle-surface canary.
//     Sends a request carrying X-Bundle-Token + a JSON body and asserts
//     the audit row carries ONLY the safe Meta keys (bundle_id /
//     route_pattern / method); the token bytes and body content never
//     reach the substrate per Cerberus #1465 closure-only-scope.
//
//   - TestGateway_DispatchFailed_EmitsRejected_Bad — failure-side cover
//     across the four rejection gates (invalid token, missing bundle,
//     unknown scope, permission denied) PLUS the handler-Failed path.
//     Asserts each gate emits the correct reason literal and outcome.

package gateway_test

import (
	"net/http/httptest"
	"strings"
	"sync"

	core "dappco.re/go"
	"github.com/gin-gonic/gin"

	"dappco.re/lthn/desktop/pkg/audit"
	"dappco.re/lthn/desktop/pkg/gateway"
	"dappco.re/lthn/desktop/pkg/marketplace"
)

// gatewayScopeLiteral mirrors the package-internal const stamped on
// every typed audit row from pkg/gateway. Kept here as a literal so the
// black-box test can assert on it without re-exporting the const.
const gatewayScopeLiteral = "gateway"

// recordingRecorder captures every audit.Event the gateway service
// emits during a test. Mirror of the same-named fixture in
// pkg/marketplace/audit_test.go (H#208) and pkg/process/audit_test.go
// (H#193).
type recordingRecorder struct {
	mu     sync.Mutex
	events []audit.Event
}

func (r *recordingRecorder) Record(ev audit.Event) core.Result {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, ev)
	return core.Ok(nil)
}

func (r *recordingRecorder) snapshot() []audit.Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]audit.Event, len(r.events))
	copy(out, r.events)
	return out
}

// filterByName returns only events whose Event field matches name.
func (r *recordingRecorder) filterByName(name string) []audit.Event {
	all := r.snapshot()
	out := make([]audit.Event, 0, len(all))
	for _, ev := range all {
		if ev.Event == name {
			out = append(out, ev)
		}
	}
	return out
}

// installAuditRecorder wires a recordingRecorder as audit.Default and
// restores the prior default at test end.
func installAuditRecorder(t *core.T) *recordingRecorder {
	t.Helper()
	rec := &recordingRecorder{}
	audit.SetDefault(rec)
	t.Cleanup(func() { audit.SetDefault(nil) })
	return rec
}

// staticAuditResolver implements gateway.BundleCodeResolver against a fixed
// token → code map so the test can drive both the happy resolver path
// (token resolves to expected bundle) and the rejection path (token
// unknown).
type staticAuditResolver struct{ table map[string]string }

func (s *staticAuditResolver) LookupBundleCode(secret string) (string, bool) {
	code, ok := s.table[secret]
	return code, ok
}

// TestGateway_Dispatch_EmitsAuditTrail_Good drives Handle against a
// fully-wired (bundle installed, permission declared, scope registered)
// route and asserts the Requested + Succeeded pair lands on the audit
// substrate with the expected Meta shape. No Rejected row, no Failed
// row — the happy path is exactly the two-event trio shape.
func TestGateway_Dispatch_EmitsAuditTrail_Good(t *core.T) {
	rec := installAuditRecorder(t)
	c := newGatewayCore(t)
	seedBundle(t, c, "opencode", []marketplace.Permission{
		{Scope: "project.metadata", Mode: "read"},
	})
	_, r := newTestRouter(t, c)

	req := httptest.NewRequest("POST", "/v1/api/gateway/project.metadata/read", nil)
	req.Header.Set("Bundle-ID", "opencode")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	core.AssertEqual(t, core.StatusOK, w.Code, "happy path must return 200")

	requested := rec.filterByName("gateway.dispatch.requested")
	core.AssertEqual(t, 1, len(requested), "happy path must emit one Requested")
	core.AssertEqual(t, gatewayScopeLiteral, requested[0].Scope)
	core.AssertEqual(t, audit.OutcomeOK, requested[0].Outcome)
	core.AssertEqual(t, "opencode", requested[0].Meta["bundle_id"])
	core.AssertEqual(t, "project.metadata:read", requested[0].Meta["route_pattern"])
	core.AssertEqual(t, "POST", requested[0].Meta["method"])

	succeeded := rec.filterByName("gateway.dispatch.succeeded")
	core.AssertEqual(t, 1, len(succeeded), "OK handler must emit one Succeeded")
	core.AssertEqual(t, audit.OutcomeOK, succeeded[0].Outcome)
	core.AssertEqual(t, "opencode", succeeded[0].Meta["bundle_id"])
	core.AssertEqual(t, "project.metadata:read", succeeded[0].Meta["route_pattern"])

	// Defensive: zero Failed + zero Rejected rows on the happy path.
	core.AssertEqual(t, 0, len(rec.filterByName("gateway.dispatch.failed")),
		"happy path must not emit Failed")
	core.AssertEqual(t, 0, len(rec.filterByName("gateway.dispatch.rejected")),
		"happy path must not emit Rejected")
}

// TestGateway_AuditMeta_NoRawSecrets_Bad sends a request carrying
// X-Bundle-Token + a JSON body containing a Bearer-shaped string and
// asserts the audit row carries ONLY the safe Meta keys. Token bytes
// from the X-Bundle-Token header MUST NOT reach the substrate per
// Cerberus #1465; request body content MUST NOT reach the substrate
// per the same closure-only-scope discipline. The recordingRecorder
// captures the pre-redactor Event so this test directly verifies the
// emit-site shape (not just the fileSink redactor backstop).
func TestGateway_AuditMeta_NoRawSecrets_Bad(t *core.T) {
	rec := installAuditRecorder(t)
	c := newGatewayCore(t)
	seedBundle(t, c, "opencode", []marketplace.Permission{
		{Scope: "project.metadata", Mode: "read"},
	})

	// Wire a resolver that maps a known token to the seeded bundle so
	// the token path is exercised end-to-end (the strip-header step in
	// Handle is downstream of the audit emit so a regression that
	// recorded the token would land here).
	gin.SetMode(gin.TestMode)
	svc := gateway.NewService(c)
	gateway.SetBundleCodeResolver(svc, &staticAuditResolver{
		table: map[string]string{"sekrit-token-bytes-DO-NOT-LEAK": "opencode"},
	})
	router := gin.New()
	router.POST("/v1/api/gateway/:scope/:mode", svc.Handle)

	// Body shape includes a Bearer-prefixed pseudo-secret; the gateway
	// emit MUST NOT thread the body bytes through Meta even when the
	// caller crafts the body to look like a secret.
	body := strings.NewReader(`{"hint":"Bearer sk-pretend-credential-1234567890"}`)
	req := httptest.NewRequest("POST", "/v1/api/gateway/project.metadata/read", body)
	req.Header.Set("X-Bundle-Token", "sekrit-token-bytes-DO-NOT-LEAK")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	core.AssertEqual(t, core.StatusOK, w.Code, "token-resolved happy path must return 200")

	// Combine ALL emitted rows from this test and walk Meta — the
	// closed-set assertion is enforced per row.
	allowedKeys := map[string]bool{
		"bundle_id": true, "route_pattern": true, "method": true,
		"reason": true, "error_code": true,
	}
	for _, ev := range rec.snapshot() {
		for k, v := range ev.Meta {
			core.AssertTrue(t, allowedKeys[k],
				"audit Meta key not in closed set: "+k)
			s, ok := v.(string)
			if !ok {
				continue
			}
			core.AssertFalse(t,
				strings.Contains(s, "sekrit-token-bytes-DO-NOT-LEAK"),
				"X-Bundle-Token bytes leaked into audit Meta")
			core.AssertFalse(t,
				strings.Contains(s, "sk-pretend-credential"),
				"request body bytes leaked into audit Meta")
			core.AssertFalse(t,
				strings.Contains(strings.ToLower(s), "bearer "),
				"Bearer-shaped string leaked into audit Meta")
		}
	}

	// Sanity: at least the Requested + Succeeded pair landed.
	core.AssertEqual(t, 1, len(rec.filterByName("gateway.dispatch.requested")))
	core.AssertEqual(t, 1, len(rec.filterByName("gateway.dispatch.succeeded")))
}

// TestGateway_DispatchFailed_EmitsRejected_Bad drives each rejection
// gate AND the handler-Failed path. Each gate must emit exactly one
// Rejected row with the correct reason literal; the handler-Failed
// path must emit the Requested → Failed pair instead.
func TestGateway_DispatchFailed_EmitsRejected_Bad(t *core.T) {
	// Sub-case 1: invalid X-Bundle-Token → reasonInvalidBundleToken.
	t.Run("invalid_bundle_token", func(t *core.T) {
		rec := installAuditRecorder(t)
		c := newGatewayCore(t)
		gin.SetMode(gin.TestMode)
		svc := gateway.NewService(c)
		gateway.SetBundleCodeResolver(svc, &staticAuditResolver{table: map[string]string{}})
		router := gin.New()
		router.POST("/v1/api/gateway/:scope/:mode", svc.Handle)

		req := httptest.NewRequest("POST", "/v1/api/gateway/project.metadata/read", nil)
		req.Header.Set("X-Bundle-Token", "unknown-token")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		core.AssertEqual(t, core.StatusUnauthorized, w.Code)
		rejected := rec.filterByName("gateway.dispatch.rejected")
		core.AssertEqual(t, 1, len(rejected))
		core.AssertEqual(t, audit.OutcomeDenied, rejected[0].Outcome)
		core.AssertEqual(t, "invalid_bundle_token", rejected[0].Meta["reason"])
		core.AssertEqual(t, "", rejected[0].Meta["bundle_id"],
			"bundle_id MUST be empty when rejection happens before identity resolution")
		core.AssertEqual(t, "project.metadata:read", rejected[0].Meta["route_pattern"])
		core.AssertEqual(t, 0, len(rec.filterByName("gateway.dispatch.requested")),
			"rejection before Requested gate must not emit Requested")
	})

	// Sub-case 2: missing Bundle-ID header → reasonMissingBundleID.
	t.Run("missing_bundle_id", func(t *core.T) {
		rec := installAuditRecorder(t)
		c := newGatewayCore(t)
		_, r := newTestRouter(t, c)

		req := httptest.NewRequest("POST", "/v1/api/gateway/project.metadata/read", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		core.AssertEqual(t, core.StatusUnauthorized, w.Code)
		rejected := rec.filterByName("gateway.dispatch.rejected")
		core.AssertEqual(t, 1, len(rejected))
		core.AssertEqual(t, audit.OutcomeDenied, rejected[0].Outcome)
		core.AssertEqual(t, "missing_bundle_id", rejected[0].Meta["reason"])
		core.AssertEqual(t, "", rejected[0].Meta["bundle_id"])
	})

	// Sub-case 3: scope not in handler registry → reasonScopeUnavailable.
	t.Run("scope_unavailable", func(t *core.T) {
		rec := installAuditRecorder(t)
		c := newGatewayCore(t)
		seedBundle(t, c, "opencode", []marketplace.Permission{
			{Scope: "nope.unknown", Mode: "read"},
		})
		_, r := newTestRouter(t, c)

		req := httptest.NewRequest("POST", "/v1/api/gateway/nope.unknown/read", nil)
		req.Header.Set("Bundle-ID", "opencode")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		core.AssertEqual(t, core.StatusNotImplemented, w.Code)
		rejected := rec.filterByName("gateway.dispatch.rejected")
		core.AssertEqual(t, 1, len(rejected))
		core.AssertEqual(t, audit.OutcomeDenied, rejected[0].Outcome)
		core.AssertEqual(t, "scope_unavailable", rejected[0].Meta["reason"])
		core.AssertEqual(t, "opencode", rejected[0].Meta["bundle_id"],
			"bundle_id MUST be populated when rejection happens after identity resolution")
	})

	// Sub-case 4: permission not declared → reasonPermissionDenied.
	t.Run("permission_denied", func(t *core.T) {
		rec := installAuditRecorder(t)
		c := newGatewayCore(t)
		seedBundle(t, c, "opencode", []marketplace.Permission{
			{Scope: "service.notify", Mode: "invoke"},
		})
		_, r := newTestRouter(t, c)

		req := httptest.NewRequest("POST", "/v1/api/gateway/project.metadata/read", nil)
		req.Header.Set("Bundle-ID", "opencode")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		core.AssertEqual(t, core.StatusForbidden, w.Code)
		rejected := rec.filterByName("gateway.dispatch.rejected")
		core.AssertEqual(t, 1, len(rejected))
		core.AssertEqual(t, audit.OutcomeDenied, rejected[0].Outcome)
		core.AssertEqual(t, "permission_denied", rejected[0].Meta["reason"])
		core.AssertEqual(t, "opencode", rejected[0].Meta["bundle_id"])
	})

	// Sub-case 5: handler returns Fail → Requested + Failed pair.
	t.Run("handler_failed", func(t *core.T) {
		rec := installAuditRecorder(t)
		c := newGatewayCore(t)
		// service.notify is the v1 stub that returns Fail (pkg/notifications
		// hasn't shipped); bundle declares the permission so the gates pass
		// and the handler-Fail path drives the Requested + Failed pair.
		seedBundle(t, c, "opencode", []marketplace.Permission{
			{Scope: "service.notify", Mode: "invoke"},
		})
		_, r := newTestRouter(t, c)

		req := httptest.NewRequest("POST", "/v1/api/gateway/service.notify/invoke", nil)
		req.Header.Set("Bundle-ID", "opencode")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		core.AssertEqual(t, core.StatusBadRequest, w.Code)
		requested := rec.filterByName("gateway.dispatch.requested")
		core.AssertEqual(t, 1, len(requested),
			"handler-fail path must still emit Requested BEFORE the handler runs")
		failed := rec.filterByName("gateway.dispatch.failed")
		core.AssertEqual(t, 1, len(failed))
		core.AssertEqual(t, audit.OutcomeFailed, failed[0].Outcome)
		core.AssertEqual(t, "opencode", failed[0].Meta["bundle_id"])
		core.AssertEqual(t, "service.notify:invoke", failed[0].Meta["route_pattern"])
		_, hasErrCode := failed[0].Meta["error_code"]
		core.AssertTrue(t, hasErrCode, "Failed Meta must carry error_code key")
		// Defensive: no Rejected on the handler-fail path.
		core.AssertEqual(t, 0, len(rec.filterByName("gateway.dispatch.rejected")))
		core.AssertEqual(t, 0, len(rec.filterByName("gateway.dispatch.succeeded")))
	})
}

// TestGateway_AuditMeta_ErrorCodeBoundedKeyspace_Ugly — STRIDE-I
// prose-leak canary per RFC.error-code-cascade.md §3 (W6) /
// Mantis #1717 / Cerberus #61 C-5. Drives the handler-Failed path
// (service.notify stub returns Fail wrapping core.E("gateway.
// serviceNotifyInvoke", "pkg/notifications not yet implemented…")) and
// asserts the emitted Failed row's Meta["error_code"] is the BOUNDED
// Operation literal "gateway.serviceNotifyInvoke", NOT the raw
// r.Error() prose containing the "pkg/notifications not yet
// implemented" message body.
//
// The pre-W6 shape (emitDispatchFailed(..., r.Error())) would have
// landed the full "gateway.serviceNotifyInvoke: pkg/notifications not
// yet implemented — file as a separate ticket if needed" string in
// Meta["error_code"] — and at plugin scope dispatch, that prose
// routinely embeds upstream-provider error bodies, URLs, file paths,
// and caller-controlled bytes. This test pins the type-system contract
// at the gateway helper boundary so any regression that reverts the
// helper signature to a raw string falls loud rather than silently
// re-leaking handler-scope prose.
func TestGateway_AuditMeta_ErrorCodeBoundedKeyspace_Ugly(t *core.T) {
	rec := installAuditRecorder(t)
	c := newGatewayCore(t)
	seedBundle(t, c, "opencode", []marketplace.Permission{
		{Scope: "service.notify", Mode: "invoke"},
	})
	_, r := newTestRouter(t, c)

	req := httptest.NewRequest("POST", "/v1/api/gateway/service.notify/invoke", nil)
	req.Header.Set("Bundle-ID", "opencode")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	core.AssertEqual(t, core.StatusBadRequest, w.Code,
		"handler-fail path must surface 400 to caller")

	failed := rec.filterByName("gateway.dispatch.failed")
	core.AssertEqual(t, 1, len(failed),
		"exactly one Failed row on handler-fail path")

	// Bounded-keyspace contract: error_code is the *core.Err.Operation
	// canon per audit.ErrorCode(r) resolution order, NOT the raw
	// r.Error() prose. serviceNotifyInvoke wraps Operation =
	// "gateway.serviceNotifyInvoke".
	code, ok := failed[0].Meta["error_code"].(string)
	core.AssertTrue(t, ok, "error_code Meta value must be a string")
	core.AssertEqual(t, "gateway.serviceNotifyInvoke", code,
		"error_code must be the bounded Operation literal (audit.ErrorCode), not raw prose")

	// Belt-and-braces: NONE of the handler's prose body bytes may
	// appear in Meta["error_code"]. Defends against future regressions
	// that re-introduce a string parameter or fold raw r.Error() into
	// a renamed key.
	core.AssertFalse(t, strings.Contains(code, "pkg/notifications"),
		"error_code must not echo handler prose body: got "+code)
	core.AssertFalse(t, strings.Contains(code, "not yet implemented"),
		"error_code must not echo handler prose body: got "+code)
	core.AssertFalse(t, strings.Contains(code, "file as a separate ticket"),
		"error_code must not echo handler prose body: got "+code)
	core.AssertFalse(t, strings.Contains(code, ":"),
		"bounded Operation literal must be the codepath alone, not ‘op: message’: got "+code)
}
