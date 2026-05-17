// SPDX-Licence-Identifier: EUPL-1.2

// Tests for the /v1/plugin-view/capability-grant receiver per
// Mantis #1523 + Mantis #1576 (RFC.plugin-view-audit-atomicity v2,
// target_version v1.0.0-beta.1). Covers the request shapes the
// frontend broker (api-fetch.ts grantTokenToFrame) can present:
//
//   - Good — valid {plugin_id, capabilities:[...], origin, outcome:"granted"}
//     → 200, ONE audit row materialised with
//     {plugin_id, capabilities, origin, outcome, correlation_id};
//     response body carries correlation_id (UUIDv4-shaped).
//   - Bad — body missing required field → 400
//   - Bad — plugin_id not installed → 404 + NO audit row
//   - Bad — audit emit failed → 500
//   - Bad — legacy {capability:<scalar>} shape → 400 (RFC v2 §5(a)
//     hard cutover; broker is sole producer and ships new shape in lockstep)
//   - Bad — outcome literal not "granted" → 400 + NO audit row
//   - Bad — capability outside allowlist → 400 + NO audit row
//   - Bad — request-body correlation_id assertion → 400 (handler-only authority)

package server_test

import (
	"encoding/json"
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

// uuidV4Shape reports whether s matches the 36-char 8-4-4-4-12 hex
// pattern with the version-4 nibble + variant-1 top bits per RFC 4122.
// Mirrors the shape assertion in pkg/account/routes_test.go so future
// drift surfaces in both places at once.
func uuidV4Shape(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, ch := range s {
		switch i {
		case 8, 13, 18, 23:
			if ch != '-' {
				return false
			}
		default:
			isHex := (ch >= '0' && ch <= '9') ||
				(ch >= 'a' && ch <= 'f') ||
				(ch >= 'A' && ch <= 'F')
			if !isHex {
				return false
			}
		}
	}
	// Version nibble — byte 6 (chars 14-15) top nibble == '4'.
	if s[14] != '4' {
		return false
	}
	// Variant top two bits 10 — byte 8 (chars 19-20) top nibble in {8,9,a,b}.
	switch s[19] {
	case '8', '9', 'a', 'b', 'A', 'B':
	default:
		return false
	}
	return true
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

	body := `{"plugin_id":"opencode","capabilities":["session-token"],"origin":"http://127.0.0.1:4096","outcome":"granted"}`
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
	core.AssertEqual(t, "http://127.0.0.1:4096", ev.Meta["origin"])
	core.AssertEqual(t, "granted", ev.Meta["outcome"])
	// capabilities lands as a defensive-copied []string in Meta.
	caps, ok := ev.Meta["capabilities"].([]string)
	core.AssertTrue(t, ok)
	core.AssertEqual(t, 1, len(caps))
	core.AssertEqual(t, "session-token", caps[0])
	// correlation_id MUST be present + UUIDv4-shaped + match the response body.
	cid, _ := ev.Meta["correlation_id"].(string)
	core.AssertTrue(t, uuidV4Shape(cid))
}

// TestServer_PluginViewCapabilityGrant_ArrayShape_Good (RFC §4.3) —
// POST {plugin_id, origin, capabilities:[...], outcome:"granted"} → 200;
// audit row Meta.capabilities == ["session-token"]; response body carries
// correlation_id (UUIDv4-shaped); audit row Meta.correlation_id equals
// response value.
func TestServer_PluginViewCapabilityGrant_ArrayShape_Good(t *testing.T) {
	rec := &recordingRecorder{}
	audit.SetDefault(rec)
	t.Cleanup(func() { audit.SetDefault(nil) })

	s := server.NewService(server.Options{})

	body := `{"plugin_id":"any","capabilities":["session-token"],"origin":"http://127.0.0.1:4096","outcome":"granted"}`
	req := httptest.NewRequest(core.MethodPost,
		server.PluginViewCapabilityGrantPath, core.NewBufferReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	core.AssertEqual(t, core.StatusOK, w.Code)
	core.AssertEqual(t, 1, len(rec.events))
	// Parse the response body to lift correlation_id.
	type respShape struct {
		Success bool `json:"success"`
		Data    struct {
			CorrelationID string `json:"correlation_id"`
		} `json:"data"`
	}
	var resp respShape
	core.AssertNil(t, json.Unmarshal(w.Body.Bytes(), &resp))
	core.AssertTrue(t, resp.Success)
	core.AssertTrue(t, uuidV4Shape(resp.Data.CorrelationID))
	// Audit row correlation_id MUST equal response correlation_id.
	core.AssertEqual(t, resp.Data.CorrelationID, rec.events[0].Meta["correlation_id"])
}

func TestServer_PluginViewCapabilityGrant_Good_NoCheckerWiredAcceptsAnyID(t *testing.T) {
	// Non-desktop builds + tests don't wire PluginInstalledChecker;
	// the audit row still emits — only the plugin_id gate is skipped.
	rec := &recordingRecorder{}
	audit.SetDefault(rec)
	t.Cleanup(func() { audit.SetDefault(nil) })

	s := server.NewService(server.Options{}) // no checker

	body := `{"plugin_id":"any-id","capabilities":["session-token"],"origin":"http://127.0.0.1:9999","outcome":"granted"}`
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
	body := `{"plugin_id":"opencode","capabilities":["session-token"],"outcome":"granted"}`
	req := httptest.NewRequest(core.MethodPost,
		server.PluginViewCapabilityGrantPath, core.NewBufferReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	core.AssertEqual(t, core.StatusBadRequest, w.Code)
	core.AssertTrue(t, core.Contains(w.Body.String(), "plugin_view.grant.invalid"))
	core.AssertEqual(t, 0, len(rec.events))
}

// TestServer_PluginViewCapabilityGrant_LegacyScalarRejected_Bad (RFC §4.3) —
// POST {plugin_id, capability:"session-token", origin, ...} (legacy
// scalar shape, no `capabilities` array, no `outcome`) → 400 +
// codePluginViewGrantInvalid + "`capability` field deprecated".
// ZERO audit rows emitted. Pairs with §5(a) hard cutover.
func TestServer_PluginViewCapabilityGrant_LegacyScalarRejected_Bad(t *testing.T) {
	rec := &recordingRecorder{}
	audit.SetDefault(rec)
	t.Cleanup(func() { audit.SetDefault(nil) })

	s := server.NewService(server.Options{})

	body := `{"plugin_id":"opencode","capability":"session-token","origin":"http://127.0.0.1:4096"}`
	req := httptest.NewRequest(core.MethodPost,
		server.PluginViewCapabilityGrantPath, core.NewBufferReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	core.AssertEqual(t, core.StatusBadRequest, w.Code)
	core.AssertTrue(t, core.Contains(w.Body.String(), "plugin_view.grant.invalid"))
	core.AssertTrue(t, core.Contains(w.Body.String(), "capability"))
	core.AssertTrue(t, core.Contains(w.Body.String(), "deprecated"))
	core.AssertEqual(t, 0, len(rec.events))
}

// TestServer_PluginViewCapabilityGrant_UnknownCapabilityInArray_Bad
// (RFC §4.3) — POST with an unknown capability literal in the array
// → 400 + codePluginViewGrantInvalid + ZERO audit rows emitted.
func TestServer_PluginViewCapabilityGrant_UnknownCapabilityInArray_Bad(t *testing.T) {
	rec := &recordingRecorder{}
	audit.SetDefault(rec)
	t.Cleanup(func() { audit.SetDefault(nil) })

	s := server.NewService(server.Options{})

	// vi-events is a forward-contract slot the broker does not
	// honour today; mirror the shim's DENY_REASON_UNKNOWN_SCOPE.
	body := `{"plugin_id":"opencode","capabilities":["vi-events"],"origin":"http://127.0.0.1:4096","outcome":"granted"}`
	req := httptest.NewRequest(core.MethodPost,
		server.PluginViewCapabilityGrantPath, core.NewBufferReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	core.AssertEqual(t, core.StatusBadRequest, w.Code)
	core.AssertTrue(t, core.Contains(w.Body.String(), "plugin_view.grant.invalid"))
	core.AssertEqual(t, 0, len(rec.events))
}

// TestServer_PluginViewCapabilityGrant_UnlistedCapability_Bad (RFC §4.3 /
// §10 + dispatch brief Step 4) — Mixed-array shape with one allowlisted
// + one not. Whole request rejects with 400 + ZERO audit rows.
// Load-bearing for §10 — handler's capability allowlist IS the
// authoritative gate.
func TestServer_PluginViewCapabilityGrant_UnlistedCapability_Bad(t *testing.T) {
	rec := &recordingRecorder{}
	audit.SetDefault(rec)
	t.Cleanup(func() { audit.SetDefault(nil) })

	s := server.NewService(server.Options{})

	body := `{"plugin_id":"opencode","capabilities":["session-token","synthetic-not-in-allowlist"],"origin":"http://127.0.0.1:4096","outcome":"granted"}`
	req := httptest.NewRequest(core.MethodPost,
		server.PluginViewCapabilityGrantPath, core.NewBufferReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	core.AssertEqual(t, core.StatusBadRequest, w.Code)
	core.AssertTrue(t, core.Contains(w.Body.String(), "plugin_view.grant.invalid"))
	core.AssertTrue(t, core.Contains(w.Body.String(), "allowlist"))
	core.AssertEqual(t, 0, len(rec.events))
}

// TestServer_PluginViewCapabilityGrant_EmptyArray_Ugly (RFC §4.3) —
// POST with capabilities:[] → 400 + ZERO audit rows.
func TestServer_PluginViewCapabilityGrant_EmptyArray_Ugly(t *testing.T) {
	rec := &recordingRecorder{}
	audit.SetDefault(rec)
	t.Cleanup(func() { audit.SetDefault(nil) })

	s := server.NewService(server.Options{})

	body := `{"plugin_id":"opencode","capabilities":[],"origin":"http://127.0.0.1:4096","outcome":"granted"}`
	req := httptest.NewRequest(core.MethodPost,
		server.PluginViewCapabilityGrantPath, core.NewBufferReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	core.AssertEqual(t, core.StatusBadRequest, w.Code)
	core.AssertTrue(t, core.Contains(w.Body.String(), "plugin_view.grant.invalid"))
	core.AssertTrue(t, core.Contains(w.Body.String(), "at least one"))
	core.AssertEqual(t, 0, len(rec.events))
}

// TestServer_PluginViewCapabilityGrant_OutcomeDenied_Ugly (RFC §4.3 /
// dispatch brief Step 4) — POST with outcome:"denied" → 400 + ZERO
// rows. Hardens against a compromised broker emitting fabricated
// "denied" outcome rows.
func TestServer_PluginViewCapabilityGrant_OutcomeDenied_Ugly(t *testing.T) {
	rec := &recordingRecorder{}
	audit.SetDefault(rec)
	t.Cleanup(func() { audit.SetDefault(nil) })

	s := server.NewService(server.Options{})

	body := `{"plugin_id":"opencode","capabilities":["session-token"],"origin":"http://127.0.0.1:4096","outcome":"denied"}`
	req := httptest.NewRequest(core.MethodPost,
		server.PluginViewCapabilityGrantPath, core.NewBufferReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	core.AssertEqual(t, core.StatusBadRequest, w.Code)
	core.AssertTrue(t, core.Contains(w.Body.String(), "plugin_view.grant.invalid"))
	core.AssertTrue(t, core.Contains(w.Body.String(), "outcome must be 'granted'"))
	core.AssertEqual(t, 0, len(rec.events))
}

// TestServer_PluginViewCapabilityGrant_OutcomeMissing_Ugly (RFC §4.3 /
// dispatch brief Step 4) — POST with no outcome field → 400 + ZERO rows.
func TestServer_PluginViewCapabilityGrant_OutcomeMissing_Ugly(t *testing.T) {
	rec := &recordingRecorder{}
	audit.SetDefault(rec)
	t.Cleanup(func() { audit.SetDefault(nil) })

	s := server.NewService(server.Options{})

	body := `{"plugin_id":"opencode","capabilities":["session-token"],"origin":"http://127.0.0.1:4096"}`
	req := httptest.NewRequest(core.MethodPost,
		server.PluginViewCapabilityGrantPath, core.NewBufferReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	core.AssertEqual(t, core.StatusBadRequest, w.Code)
	core.AssertTrue(t, core.Contains(w.Body.String(), "plugin_view.grant.invalid"))
	core.AssertEqual(t, 0, len(rec.events))
}

// TestServer_PluginViewCapabilityGrant_CorrelationIDInRequest_Bad
// (RFC §3.1 + §4.2) — POST with caller-asserted correlation_id → 400 +
// ZERO rows. Handler is the sole authority; caller assertion would let
// a compromised broker fabricate audit-JOIN keys.
func TestServer_PluginViewCapabilityGrant_CorrelationIDInRequest_Bad(t *testing.T) {
	rec := &recordingRecorder{}
	audit.SetDefault(rec)
	t.Cleanup(func() { audit.SetDefault(nil) })

	s := server.NewService(server.Options{})

	body := `{"plugin_id":"opencode","capabilities":["session-token"],"origin":"http://127.0.0.1:4096","outcome":"granted","correlation_id":"00000000-0000-4000-8000-000000000000"}`
	req := httptest.NewRequest(core.MethodPost,
		server.PluginViewCapabilityGrantPath, core.NewBufferReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	core.AssertEqual(t, core.StatusBadRequest, w.Code)
	core.AssertTrue(t, core.Contains(w.Body.String(), "plugin_view.grant.invalid"))
	core.AssertTrue(t, core.Contains(w.Body.String(), "correlation_id"))
	core.AssertEqual(t, 0, len(rec.events))
}

// TestServer_PluginViewCapabilityGrant_NoCorrelationIdInRequest_Good
// (dispatch brief Step 4) — request omits correlation_id; handler
// generates a fresh UUIDv4 and echoes it in the response.
func TestServer_PluginViewCapabilityGrant_NoCorrelationIdInRequest_Good(t *testing.T) {
	rec := &recordingRecorder{}
	audit.SetDefault(rec)
	t.Cleanup(func() { audit.SetDefault(nil) })

	s := server.NewService(server.Options{})

	body := `{"plugin_id":"opencode","capabilities":["session-token"],"origin":"http://127.0.0.1:4096","outcome":"granted"}`
	req := httptest.NewRequest(core.MethodPost,
		server.PluginViewCapabilityGrantPath, core.NewBufferReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	core.AssertEqual(t, core.StatusOK, w.Code)
	core.AssertEqual(t, 1, len(rec.events))
	// Pull the generated correlation_id from the audit row + response;
	// both must be UUIDv4-shaped + equal.
	type respShape struct {
		Success bool `json:"success"`
		Data    struct {
			CorrelationID string `json:"correlation_id"`
		} `json:"data"`
	}
	var resp respShape
	core.AssertNil(t, json.Unmarshal(w.Body.Bytes(), &resp))
	core.AssertTrue(t, uuidV4Shape(resp.Data.CorrelationID))
	core.AssertEqual(t, resp.Data.CorrelationID, rec.events[0].Meta["correlation_id"])
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

	body := `{"plugin_id":"ghost","capabilities":["session-token"],"origin":"http://127.0.0.1:4096","outcome":"granted"}`
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

	body := `{"plugin_id":"opencode","capabilities":["session-token"],"origin":"http://127.0.0.1:4096","outcome":"granted"}`
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
	envelope := `{"plugin_id":"opencode","capabilities":["session-token"],"outcome":"granted","origin":"`
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
		name       string
		body       string
		wantSubstr string
	}{
		{
			name: "plugin_id over MaxPluginIDBytes",
			body: `{"plugin_id":"` + string(oversizePluginID) +
				`","capabilities":["session-token"],"origin":"http://127.0.0.1:4096","outcome":"granted"}`,
			wantSubstr: "plugin_id exceeds",
		},
		{
			name: "capability literal over MaxCapabilityBytes",
			body: `{"plugin_id":"opencode","capabilities":["` + string(oversizeCapability) +
				`"],"origin":"http://127.0.0.1:4096","outcome":"granted"}`,
			wantSubstr: "capability literal exceeds",
		},
		{
			name: "origin over MaxOriginBytes",
			body: `{"plugin_id":"opencode","capabilities":["session-token"],"origin":"http://` +
				string(oversizeOriginHost) + `","outcome":"granted"}`,
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

			body := `{"plugin_id":"opencode","capabilities":["session-token"],"origin":"` +
				tc.origin + `","outcome":"granted"}`
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

			body := `{"plugin_id":"opencode","capabilities":["session-token"],"origin":"` +
				tc.origin + `","outcome":"granted"}`
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
