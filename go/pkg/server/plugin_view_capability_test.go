// SPDX-Licence-Identifier: EUPL-1.2

// Tests for the /v1/plugin-view/capability-grant receiver per
// Mantis #1523. Covers the four request shapes the frontend shim
// can present:
//
//   - Good — valid plugin_id + capability + origin → 200, audit
//     row materialised with {plugin_id, capability, origin}
//   - Bad — body missing required field → 400
//   - Bad — plugin_id not installed → 404 (PluginInstalledChecker
//     returns false) and NO audit row materialised
//   - Bad — audit emit failed → 500 (failing recorder fixture)

package server_test

import (
	"net/http/httptest"
	"testing"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/audit"
	"dappco.re/lthn/desktop/pkg/server"
)

// recordingRecorder captures every Event it sees. Used as the audit
// fixture so tests can assert on what the handler emitted without
// touching ~/Lethean/audit/.
type recordingRecorder struct {
	events []audit.Event
}

func (r *recordingRecorder) Record(ev audit.Event) core.Result {
	r.events = append(r.events, ev)
	return core.Ok(nil)
}

// failingRecorder simulates an audit-substrate outage. Drives the
// 500 branch in handlePluginViewCapabilityGrant.
type failingRecorder struct{}

func (failingRecorder) Record(audit.Event) core.Result {
	return core.Fail(core.E("audit.test", "disk full", nil))
}

func TestServer_PluginViewCapabilityGrant_Good(t *testing.T) {
	rec := &recordingRecorder{}
	audit.SetDefault(rec)
	t.Cleanup(func() { audit.SetDefault(nil) })

	s := server.NewService(server.Options{
		PluginInstalledChecker: func(code string) bool {
			return code == "opencode"
		},
	})

	body := `{"plugin_id":"opencode","capability":"session-token","origin":"http://127.0.0.1:4096"}`
	req := httptest.NewRequest(core.MethodPost,
		server.PluginViewCapabilityGrantPath, core.NewBufferReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	core.AssertEqual(t, core.StatusOK, w.Code)
	core.AssertTrue(t, core.Contains(w.Body.String(), `"success":true`))
	core.AssertEqual(t, 1, len(rec.events))
	ev := rec.events[0]
	core.AssertEqual(t, audit.EventPluginViewCapabilityGranted, ev.Event)
	core.AssertEqual(t, audit.OutcomeOK, ev.Outcome)
	core.AssertEqual(t, "opencode", ev.Meta["plugin_id"])
	core.AssertEqual(t, "session-token", ev.Meta["capability"])
	core.AssertEqual(t, "http://127.0.0.1:4096", ev.Meta["origin"])
}

func TestServer_PluginViewCapabilityGrant_Good_NoCheckerWiredAcceptsAnyID(t *testing.T) {
	// Non-desktop builds + tests don't wire PluginInstalledChecker;
	// the audit row still emits — only the plugin_id gate is skipped.
	rec := &recordingRecorder{}
	audit.SetDefault(rec)
	t.Cleanup(func() { audit.SetDefault(nil) })

	s := server.NewService(server.Options{}) // no checker

	body := `{"plugin_id":"any-id","capability":"session-token","origin":"http://127.0.0.1:9999"}`
	req := httptest.NewRequest(core.MethodPost,
		server.PluginViewCapabilityGrantPath, core.NewBufferReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	core.AssertEqual(t, core.StatusOK, w.Code)
	core.AssertEqual(t, 1, len(rec.events))
}

func TestServer_PluginViewCapabilityGrant_Bad_MissingField(t *testing.T) {
	rec := &recordingRecorder{}
	audit.SetDefault(rec)
	t.Cleanup(func() { audit.SetDefault(nil) })

	s := server.NewService(server.Options{})

	// origin missing — handler rejects at the required-fields gate.
	body := `{"plugin_id":"opencode","capability":"session-token"}`
	req := httptest.NewRequest(core.MethodPost,
		server.PluginViewCapabilityGrantPath, core.NewBufferReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	core.AssertEqual(t, core.StatusBadRequest, w.Code)
	core.AssertTrue(t, core.Contains(w.Body.String(), "plugin_view.grant.invalid"))
	core.AssertEqual(t, 0, len(rec.events))
}

func TestServer_PluginViewCapabilityGrant_Bad_UnknownCapability(t *testing.T) {
	rec := &recordingRecorder{}
	audit.SetDefault(rec)
	t.Cleanup(func() { audit.SetDefault(nil) })

	s := server.NewService(server.Options{})

	// vi-events is a forward-contract slot the broker does not
	// honour today; mirror the shim's DENY_REASON_UNKNOWN_SCOPE.
	body := `{"plugin_id":"opencode","capability":"vi-events","origin":"http://127.0.0.1:4096"}`
	req := httptest.NewRequest(core.MethodPost,
		server.PluginViewCapabilityGrantPath, core.NewBufferReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	core.AssertEqual(t, core.StatusBadRequest, w.Code)
	core.AssertTrue(t, core.Contains(w.Body.String(), "plugin_view.grant.invalid"))
	core.AssertEqual(t, 0, len(rec.events))
}

func TestServer_PluginViewCapabilityGrant_Bad_UnknownPlugin(t *testing.T) {
	rec := &recordingRecorder{}
	audit.SetDefault(rec)
	t.Cleanup(func() { audit.SetDefault(nil) })

	s := server.NewService(server.Options{
		PluginInstalledChecker: func(code string) bool {
			return code == "opencode"
		},
	})

	body := `{"plugin_id":"ghost","capability":"session-token","origin":"http://127.0.0.1:4096"}`
	req := httptest.NewRequest(core.MethodPost,
		server.PluginViewCapabilityGrantPath, core.NewBufferReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	core.AssertEqual(t, core.StatusNotFound, w.Code)
	core.AssertTrue(t, core.Contains(w.Body.String(), "plugin_view.grant.unknown_plugin"))
	core.AssertEqual(t, 0, len(rec.events))
}

func TestServer_PluginViewCapabilityGrant_Bad_AuditFailure(t *testing.T) {
	audit.SetDefault(failingRecorder{})
	t.Cleanup(func() { audit.SetDefault(nil) })

	s := server.NewService(server.Options{})

	body := `{"plugin_id":"opencode","capability":"session-token","origin":"http://127.0.0.1:4096"}`
	req := httptest.NewRequest(core.MethodPost,
		server.PluginViewCapabilityGrantPath, core.NewBufferReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	core.AssertEqual(t, core.StatusInternalServerError, w.Code)
	core.AssertTrue(t, core.Contains(w.Body.String(), "plugin_view.grant.audit_failed"))
}

func TestServer_PluginViewCapabilityGrant_Bad_MalformedJSON(t *testing.T) {
	rec := &recordingRecorder{}
	audit.SetDefault(rec)
	t.Cleanup(func() { audit.SetDefault(nil) })

	s := server.NewService(server.Options{})

	req := httptest.NewRequest(core.MethodPost,
		server.PluginViewCapabilityGrantPath, core.NewBufferReader([]byte("{not-json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	core.AssertEqual(t, core.StatusBadRequest, w.Code)
	core.AssertEqual(t, 0, len(rec.events))
}
