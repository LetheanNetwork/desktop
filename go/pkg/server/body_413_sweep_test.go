// SPDX-Licence-Identifier: EUPL-1.2

// Mantis #1584 — canonical 413 envelope sweep across pkg/server
// ShouldBindJSON callsites. The Layer 2 body-cap middleware (H#43/
// H#44, middleware_body_cap.go) wraps every inbound body in
// http.MaxBytesReader and surfaces *http.MaxBytesError verbatim;
// each handler that binds JSON must recognise that error and render
// the canonical coreapi.Fail("body.too_large", ...) envelope at HTTP
// 413, instead of leaking the gin-default 400 + stdlib "http: request
// body too large" string to consumers.
//
// One test per modified handler, asserting:
//
//   - HTTP 413 (not 400, not 500) — distinguishes body-too-big from
//     shape-broken so log-tailers can pattern-match abuse
//   - JSON body contains "body.too_large" — pins the canonical code
//   - JSON body contains "success":false — pins the coreapi envelope
//     shape (Response[any] with Success=false, Error.Code="body.too_large")
//
// Test shape matches plugin_view_capability_test.go's
// TestCapabilityGrant_OversizeBody_Bad_413 so future maintainers
// adopting this pattern in a new handler have one fixture to copy.

package server_test

import (
	"net/http/httptest"
	"testing"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/server"
)

// overflowSweepBody builds a JSON body one byte over the engine-wide
// MaxBodyBytesDefault cap. The padding lives inside a valid string
// field so the JSON decoder consumes bytes from the wrapped reader
// until MaxBytesReader fires mid-string — a raw "xxxx..." body would
// trip *json.SyntaxError on the first non-JSON byte BEFORE the cap
// fires, surfacing the wrong error type.
//
// fieldName is the request field that will carry the padding. For
// chatRequest the field is "messages[0].content"; for completionRequest
// it's "prompt".
func overflowSweepBody(envelope, closer string) string {
	padLen := int(server.MaxBodyBytesDefault) - len(envelope) - len(closer) + 1
	pad := make([]byte, padLen)
	for i := range pad {
		pad[i] = 'x'
	}
	return envelope + string(pad) + closer
}

// TestHandleChat_BodyTooLarge_Returns413_Bad — POST /v1/chat/completions
// with a body that exceeds MaxBodyBytesDefault must surface as HTTP 413
// + coreapi.Fail("body.too_large", ...) envelope, not as gin-default 400
// with leaked stdlib text. Per Mantis #1584 / Amendment A1.
func TestHandleChat_BodyTooLarge_Returns413_Bad(t *testing.T) {
	s := server.NewService(server.Options{})

	envelope := `{"model":"lthn-stub","messages":[{"role":"user","content":"`
	closer := `"}]}`
	body := overflowSweepBody(envelope, closer)

	req := httptest.NewRequest(core.MethodPost,
		"/v1/chat/completions", core.NewBufferReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	core.AssertEqual(t, 413, w.Code)
	core.AssertTrue(t, core.Contains(w.Body.String(), "body.too_large"))
	core.AssertTrue(t, core.Contains(w.Body.String(), `"success":false`))
}

// TestHandleCompletion_BodyTooLarge_Returns413_Bad — POST /v1/completions
// with a body that exceeds MaxBodyBytesDefault must surface as HTTP 413
// + coreapi.Fail("body.too_large", ...) envelope. Sibling of the chat
// test; both handlers share the same Amendment A1 wrap shape.
func TestHandleCompletion_BodyTooLarge_Returns413_Bad(t *testing.T) {
	s := server.NewService(server.Options{})

	envelope := `{"model":"lthn-stub","prompt":"`
	closer := `"}`
	body := overflowSweepBody(envelope, closer)

	req := httptest.NewRequest(core.MethodPost,
		"/v1/completions", core.NewBufferReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	core.AssertEqual(t, 413, w.Code)
	core.AssertTrue(t, core.Contains(w.Body.String(), "body.too_large"))
	core.AssertTrue(t, core.Contains(w.Body.String(), `"success":false`))
}

// TestHandleChat_BadJSON_Returns400_Bad — shape-broken bodies must
// still return 400 with the existing invalid_request_error envelope,
// proving the 413 fast-path only catches *http.MaxBytesError and
// doesn't reshape the legacy 400 path. Pins the Mantis #1584 contract
// from the other side.
func TestHandleChat_BadJSON_Returns400_Bad(t *testing.T) {
	s := server.NewService(server.Options{})

	req := httptest.NewRequest(core.MethodPost,
		"/v1/chat/completions", core.NewBufferReader([]byte("{not-json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	core.AssertEqual(t, 400, w.Code)
	core.AssertTrue(t, core.Contains(w.Body.String(), "invalid_request_error"))
}

// TestHandleCompletion_BadJSON_Returns400_Bad — sibling 400-preserved
// assertion for /v1/completions.
func TestHandleCompletion_BadJSON_Returns400_Bad(t *testing.T) {
	s := server.NewService(server.Options{})

	req := httptest.NewRequest(core.MethodPost,
		"/v1/completions", core.NewBufferReader([]byte("{not-json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	core.AssertEqual(t, 400, w.Code)
	core.AssertTrue(t, core.Contains(w.Body.String(), "invalid_request_error"))
}
