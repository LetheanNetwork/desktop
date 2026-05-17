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

// TestCapabilityGrant_OversizeBody_Bad_413 — body that exceeds
// MaxPluginViewGrantBodyBytes surfaces as HTTP 413 + the canonical
// coreapi.Fail("body.too_large", ...) envelope (RFC.body-cap-middleware.md
// Amendment A1 / Cerberus #1568 F1). Pins the wrap-before-bind ordering:
// MaxBytesReader fires before ShouldBindJSON, errors.As walks the
// wrap-chain to recognise *http.MaxBytesError, audit substrate stays
// untouched.
func TestCapabilityGrant_OversizeBody_Bad_413(t *testing.T) {
	rec := &recordingRecorder{}
	audit.SetDefault(rec)
	t.Cleanup(func() { audit.SetDefault(nil) })

	s := server.NewService(server.Options{})

	// Build a well-formed JSON body one byte over the cap. JSON shape
	// matters: a raw "xxxxx..." body trips the decoder on the first
	// non-JSON byte BEFORE MaxBytesReader fires, surfacing as
	// *json.SyntaxError not *http.MaxBytesError. Stuffing the value
	// inside a valid string field lets the decoder consume bytes from
	// the wrapped reader until the cap fires mid-string.
	envelope := `{"plugin_id":"opencode","capability":"session-token","origin":"`
	closer := `"}`
	// Pad plus envelope/closer pushes the total over the cap.
	padLen := server.MaxPluginViewGrantBodyBytes - len(envelope) - len(closer) + 1
	pad := make([]byte, padLen)
	for i := range pad {
		pad[i] = 'x'
	}
	body := envelope + string(pad) + closer
	req := httptest.NewRequest(core.MethodPost,
		server.PluginViewCapabilityGrantPath, core.NewBufferReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	// 413 RequestEntityTooLarge — distinguishes "body too big" from
	// "shape broken" (400) so log-tailers can pattern-match abuse.
	core.AssertEqual(t, 413, w.Code)
	core.AssertTrue(t, core.Contains(w.Body.String(), "body.too_large"))
	// Audit substrate must be untouched — the abusive payload never
	// reaches Record(). Ugly invariant: oversized requests leak zero
	// bytes into the append-only audit log.
	core.AssertEqual(t, 0, len(rec.events))
}

// TestCapabilityGrant_FieldLengthCaps_Bad_400 — table-driven: each
// oversized field independently rejects with 400 + the matching
// codePluginViewGrantInvalid envelope. Belt-and-braces against a 64 KiB
// body packing one 60 KiB field. Pins per-field caps fire BEFORE audit
// emission (rec.events stays at zero).
func TestCapabilityGrant_FieldLengthCaps_Bad_400(t *testing.T) {
	// Build oversize-by-one strings for each capped field.
	oversizePluginID := make([]byte, server.MaxPluginIDBytes+1)
	for i := range oversizePluginID {
		oversizePluginID[i] = 'p'
	}
	oversizeCapability := make([]byte, server.MaxCapabilityBytes+1)
	for i := range oversizeCapability {
		oversizeCapability[i] = 'c'
	}
	oversizeOriginHost := make([]byte, server.MaxOriginBytes+1)
	for i := range oversizeOriginHost {
		oversizeOriginHost[i] = 'o'
	}
	cases := []struct {
		name        string
		body        string
		wantSubstr  string
	}{
		{
			name: "plugin_id over MaxPluginIDBytes",
			body: `{"plugin_id":"` + string(oversizePluginID) +
				`","capability":"session-token","origin":"http://127.0.0.1:4096"}`,
			wantSubstr: "plugin_id exceeds",
		},
		{
			name: "capability over MaxCapabilityBytes",
			body: `{"plugin_id":"opencode","capability":"` + string(oversizeCapability) +
				`","origin":"http://127.0.0.1:4096"}`,
			wantSubstr: "capability exceeds",
		},
		{
			name: "origin over MaxOriginBytes",
			body: `{"plugin_id":"opencode","capability":"session-token","origin":"http://` +
				string(oversizeOriginHost) + `"}`,
			wantSubstr: "origin exceeds",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := &recordingRecorder{}
			audit.SetDefault(rec)
			t.Cleanup(func() { audit.SetDefault(nil) })

			s := server.NewService(server.Options{})

			req := httptest.NewRequest(core.MethodPost,
				server.PluginViewCapabilityGrantPath, core.NewBufferReader([]byte(tc.body)))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			s.Handler().ServeHTTP(w, req)

			core.AssertEqual(t, core.StatusBadRequest, w.Code)
			core.AssertTrue(t, core.Contains(w.Body.String(), "plugin_view.grant.invalid"))
			core.AssertTrue(t, core.Contains(w.Body.String(), tc.wantSubstr))
			// Per-field cap fires BEFORE audit emission.
			core.AssertEqual(t, 0, len(rec.events))
		})
	}
}

// TestCapabilityGrant_OriginValidation_Ugly — table-driven: every
// origin shape that isn't http(s)://host[:port] rejects with 400.
// Refuses javascript: + data: + file: schemes (XSS vectors if the
// audit row surfaces in a Lit view), refuses paths / queries /
// fragments on otherwise-valid origins, refuses empty host. Pins
// isValidPostMessageOrigin against the postMessage grammar.
func TestCapabilityGrant_OriginValidation_Ugly(t *testing.T) {
	cases := []struct {
		name   string
		origin string
	}{
		{"javascript scheme", "javascript:alert(1)"},
		{"data scheme", "data:text/html,foo"},
		{"file scheme", "file:///etc/passwd"},
		{"path on http origin", "http://127.0.0.1:4096/foo"},
		{"query on http origin", "http://127.0.0.1:4096?x=1"},
		{"fragment on http origin", "http://127.0.0.1:4096#x"},
		{"empty host on http scheme", "http://"},
		{"relative path", "../etc/passwd"},
		{"bare hostname", "example.com"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := &recordingRecorder{}
			audit.SetDefault(rec)
			t.Cleanup(func() { audit.SetDefault(nil) })

			s := server.NewService(server.Options{})

			body := `{"plugin_id":"opencode","capability":"session-token","origin":"` +
				tc.origin + `"}`
			req := httptest.NewRequest(core.MethodPost,
				server.PluginViewCapabilityGrantPath, core.NewBufferReader([]byte(body)))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			s.Handler().ServeHTTP(w, req)

			core.AssertEqual(t, core.StatusBadRequest, w.Code)
			core.AssertTrue(t, core.Contains(w.Body.String(), "plugin_view.grant.invalid"))
			core.AssertTrue(t, core.Contains(w.Body.String(), "origin must be http"))
			core.AssertEqual(t, 0, len(rec.events))
		})
	}
}

// TestCapabilityGrant_ValidOrigin_Good — table-driven: every origin
// shape that matches http(s)://host[:port] (with or without trailing
// slash) passes validation and the audit row commits with the origin
// captured verbatim.
func TestCapabilityGrant_ValidOrigin_Good(t *testing.T) {
	cases := []struct {
		name   string
		origin string
	}{
		{"http loopback with port", "http://127.0.0.1:4096"},
		{"http loopback trailing slash", "http://127.0.0.1:4096/"},
		{"https public host", "https://legitimate.example"},
		{"https public host with port", "https://legitimate.example:8443"},
		{"http localhost name", "http://localhost:9876"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := &recordingRecorder{}
			audit.SetDefault(rec)
			t.Cleanup(func() { audit.SetDefault(nil) })

			s := server.NewService(server.Options{})

			body := `{"plugin_id":"opencode","capability":"session-token","origin":"` +
				tc.origin + `"}`
			req := httptest.NewRequest(core.MethodPost,
				server.PluginViewCapabilityGrantPath, core.NewBufferReader([]byte(body)))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			s.Handler().ServeHTTP(w, req)

			core.AssertEqual(t, core.StatusOK, w.Code)
			core.AssertTrue(t, core.Contains(w.Body.String(), `"success":true`))
			core.AssertEqual(t, 1, len(rec.events))
			core.AssertEqual(t, tc.origin, rec.events[0].Meta["origin"])
		})
	}
}
