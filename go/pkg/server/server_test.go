// SPDX-Licence-Identifier: EUPL-1.2

// Tests for the OpenAI-compatible HTTP surface. gin gives us
// in-process roundtrips through httptest.NewRecorder +
// engine.ServeHTTP so no listener has to bind a port. Two runner
// shapes:
//
//  - stubRunner — implements the server.Runner contract for the
//    Models() route + records Generate() calls so the assert can
//    verify the prompt the handler extracted from chat messages
//  - failingRunner — Generate / Models always Fail, exercising the
//    error-response branches in handleModels + generate()

package server_test

import (
	"net/http/httptest"

	core "dappco.re/go"

	"dappco.re/lthn/desktop/pkg/server"
)

const (
	contentTypeHeader       = "Content-Type"
	applicationJSON         = "application/json"
	modelsEndpoint          = "/v1/models"
	chatCompletionsEndpoint = "/v1/chat/completions"
	completionsEndpoint     = "/v1/completions"
)

// stubRunner is a minimal Runner that records the last prompt + returns
// configurable model lists. Lets tests assert on what reached Generate.
type stubRunner struct {
	lastPrompt string
	reply      string
	models     []string
}

func (s *stubRunner) Generate(prompt string) core.Result {
	s.lastPrompt = prompt
	return core.Ok(s.reply)
}
func (s *stubRunner) Models() core.Result { return core.Ok(s.models) }

// failingRunner — every entrypoint Fails. Drives the error branches in
// handleModels + the generate fallback.
type failingRunner struct{}

func (failingRunner) Generate(string) core.Result {
	return core.Fail(core.E("test", "generate boom", nil))
}
func (failingRunner) Models() core.Result {
	return core.Fail(core.E("test", "models boom", nil))
}

// post is a small helper: marshal body to JSON, fire a POST through the
// gin engine, return the recorded response.
func post(t *core.T, h core.Handler, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	marshalled := core.JSONMarshal(body)
	core.AssertTrue(t, marshalled.OK)
	b := marshalled.Value.([]byte)
	req := httptest.NewRequest(core.MethodPost, path, core.NewBufferReader(b))
	req.Header.Set(contentTypeHeader, applicationJSON)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func TestServer_NewService_Good_DefaultAddr(t *core.T) {
	s := server.NewService(server.Options{})
	core.AssertNotNil(t, s)
	core.AssertNotNil(t, s.Handler())
	core.AssertNotNil(t, s.Engine())
}

func TestServer_NewService_Good_CustomAddr(t *core.T) {
	s := server.NewService(server.Options{Addr: ":9999"})
	core.AssertNotNil(t, s)
	core.AssertNotNil(t, s.Handler())
}

func TestServer_Service_Register_Good(t *core.T) {
	c := core.New()
	s := server.NewService(server.Options{})
	r := s.Register(c)
	core.AssertTrue(t, r.OK)
}

func TestServer_Register_Good(t *core.T) {
	c := core.New()
	r := server.Register(c)
	core.AssertTrue(t, r.OK)
	core.AssertNotNil(t, c)
}

func TestServer_Health_Good(t *core.T) {
	s := server.NewService(server.Options{})
	Health := "/health"
	req := httptest.NewRequest(core.MethodGet, Health, nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	core.AssertEqual(t, core.StatusOK, w.Code)
	core.AssertTrue(t, core.Contains(w.Body.String(), `"success":true`))
	core.AssertTrue(t, core.Contains(w.Body.String(), `"data":"healthy"`))
}

func TestServer_Models_Good_Stub(t *core.T) {
	// No runner — handler falls back to ["lthn-stub"].
	s := server.NewService(server.Options{})
	req := httptest.NewRequest(core.MethodGet, modelsEndpoint, nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	core.AssertEqual(t, core.StatusOK, w.Code)
	core.AssertTrue(t, core.Contains(w.Body.String(), "lthn-stub"))
}

func TestServer_Models_Good_FromRunner(t *core.T) {
	r := &stubRunner{models: []string{"gemma-4-e2b", "llama-3.2-3b"}}
	s := server.NewService(server.Options{Runner: r})
	req := httptest.NewRequest(core.MethodGet, modelsEndpoint, nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	core.AssertEqual(t, core.StatusOK, w.Code)
	body := w.Body.String()
	core.AssertTrue(t, core.Contains(body, "gemma-4-e2b"))
	core.AssertTrue(t, core.Contains(body, "llama-3.2-3b"))
}

func TestServer_Models_Bad_RunnerError(t *core.T) {
	s := server.NewService(server.Options{Runner: failingRunner{}})
	req := httptest.NewRequest(core.MethodGet, modelsEndpoint, nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	core.AssertEqual(t, core.StatusInternalServerError, w.Code)
	core.AssertTrue(t, core.Contains(w.Body.String(), "runner_error"))
}

func TestServer_Chat_Good_StubEcho(t *core.T) {
	s := server.NewService(server.Options{})
	w := post(t, s.Handler(), chatCompletionsEndpoint, map[string]any{
		"model": "lthn-stub",
		"messages": []map[string]string{
			{"role": "system", "content": "be brief"},
			{"role": "user", "content": "hello"},
		},
	})
	core.AssertEqual(t, core.StatusOK, w.Code)
	body := w.Body.String()
	core.AssertTrue(t, core.Contains(body, "chat.completion"))
	core.AssertTrue(t, core.Contains(body, "lthn stub"))
	core.AssertTrue(t, core.Contains(body, "hello"), "stub echoes the last user message")
}

func TestServer_Chat_Good_PicksLastUserTurn(t *core.T) {
	r := &stubRunner{reply: "ack"}
	s := server.NewService(server.Options{Runner: r})
	w := post(t, s.Handler(), chatCompletionsEndpoint, map[string]any{
		"messages": []map[string]string{
			{"role": "user", "content": "first"},
			{"role": "assistant", "content": "ignored"},
			{"role": "user", "content": "latest"},
		},
	})
	core.AssertEqual(t, core.StatusOK, w.Code)
	core.AssertEqual(t, "latest", r.lastPrompt, "handler must forward the most recent user message")
}

func TestServer_Chat_Bad_MalformedJSON(t *core.T) {
	s := server.NewService(server.Options{})
	req := httptest.NewRequest(core.MethodPost, chatCompletionsEndpoint,
		core.NewReader(`{"messages": not-json}`))
	req.Header.Set(contentTypeHeader, applicationJSON)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	core.AssertEqual(t, core.StatusBadRequest, w.Code)
	core.AssertTrue(t, core.Contains(w.Body.String(), "invalid_request_error"))
}

func TestServer_Completion_Good_StubEcho(t *core.T) {
	s := server.NewService(server.Options{})
	w := post(t, s.Handler(), completionsEndpoint, map[string]any{
		"prompt": "say hi",
	})
	core.AssertEqual(t, core.StatusOK, w.Code)
	body := w.Body.String()
	core.AssertTrue(t, core.Contains(body, "text_completion"))
	core.AssertTrue(t, core.Contains(body, "say hi"))
}

func TestServer_Completion_Bad_MalformedJSON(t *core.T) {
	s := server.NewService(server.Options{})
	req := httptest.NewRequest(core.MethodPost, completionsEndpoint,
		core.NewReader(`{"prompt": broken}`))
	req.Header.Set(contentTypeHeader, applicationJSON)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	core.AssertEqual(t, core.StatusBadRequest, w.Code)
}

func TestServer_Completion_Bad_RunnerError(t *core.T) {
	s := server.NewService(server.Options{Runner: failingRunner{}})
	w := post(t, s.Handler(), completionsEndpoint, map[string]any{
		"prompt": "die",
	})
	// generate() swallows the runner error into the reply text, so the
	// HTTP response is still 200 — but body carries the error marker.
	core.AssertEqual(t, core.StatusOK, w.Code)
	core.AssertTrue(t, core.Contains(w.Body.String(), "lthn error"))
}

func TestServer_MethodNotAllowed_Bad(t *core.T) {
	s := server.NewService(server.Options{})
	// GET against /v1/chat/completions falls through to the generated OpenAPI route.
	MethodNotAllowed := chatCompletionsEndpoint
	req := httptest.NewRequest(core.MethodGet, MethodNotAllowed, nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	core.AssertEqual(t, core.StatusOK, w.Code)
}

// Stop on a non-started server should not panic and should return Ok.
func TestServer_Stop_Good_NotStarted(t *core.T) {
	s := server.NewService(server.Options{})
	r := s.Stop(nil)
	core.AssertTrue(t, r.OK, "Stop on never-Started server is a clean Ok")
}

// Cerberus Mantis #1422 — CSP defence-in-depth. The middleware must fire
// on every response so an attacker who reaches a sink bypassed by Lit's
// auto-escape still hits the policy wall.

func TestServer_CSP_Good_HeaderPresentOnHealthRoute(t *core.T) {
	// /health is the cheapest stable route — no runner, no auth.
	// CSP middleware fires before the route handler, so if the header
	// appears here it appears everywhere.
	s := server.NewService(server.Options{})
	req := httptest.NewRequest(core.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	core.AssertEqual(t, core.StatusOK, w.Code)
	csp := w.Header().Get("Content-Security-Policy")
	core.AssertNotEmpty(t, csp)
	core.AssertTrue(t, core.Contains(csp, "default-src 'self'"),
		"CSP must anchor to same-origin default")
	core.AssertTrue(t, core.Contains(csp, "script-src 'self'"),
		"CSP must restrict script execution to same-origin")
	core.AssertTrue(t, core.Contains(csp, "frame-ancestors 'none'"),
		"CSP must forbid framing")
}

func TestServer_CSP_Good_HeaderPresentOnAPIRoute(t *core.T) {
	// Verify the middleware fires on POST routes too (not just GET/SPA).
	s := server.NewService(server.Options{})
	w := post(t, s.Handler(), chatCompletionsEndpoint, map[string]any{
		"messages": []map[string]string{{"role": "user", "content": "hi"}},
	})
	core.AssertEqual(t, core.StatusOK, w.Code)
	csp := w.Header().Get("Content-Security-Policy")
	core.AssertNotEmpty(t, csp, "CSP header must be set on POST /v1/chat/completions")
}

func TestServer_CSP_Bad_NoEvalInPolicy(t *core.T) {
	// Negative assertion: 'unsafe-eval' must never appear in the policy.
	// If it does, a sink that calls eval() or new Function() can bypass.
	s := server.NewService(server.Options{})
	req := httptest.NewRequest(core.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	csp := w.Header().Get("Content-Security-Policy")
	core.AssertTrue(t, !core.Contains(csp, "unsafe-eval"),
		"CSP must NOT permit unsafe-eval")
}

func TestServer_CSP_Good_HuggingFaceInConnectSrc(t *core.T) {
	// Model browser fetches the HF API. Blocking that would break the
	// feature. Asserting here so a future policy tightening that forgets
	// this allowance gets caught by the test suite.
	s := server.NewService(server.Options{})
	req := httptest.NewRequest(core.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	csp := w.Header().Get("Content-Security-Policy")
	core.AssertTrue(t, core.Contains(csp, "huggingface.co"),
		"connect-src must allow huggingface.co for the model browser")
}
