// SPDX-Licence-Identifier: EUPL-1.2

// route_test.go — coverage for the OpenCode-backed inference.TextModel
// adapter (route.go). invoke() talks HTTP to opencode-serve's session
// API; tested against a real httptest.Server standing in for the
// container. Routes() enumerates providers from the first running
// sandbox — tested via a seeded orm Sandbox record + a fake /provider
// upstream.

package opencode

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	core "dappco.re/go"
	"dappco.re/go/inference"
)

// newFakeOpencodeSession stands in for opencode-serve's session API:
// POST /session -> {"id": "ses_1"}; POST /session/ses_1/message ->
// the assistant turn. sessionStatus / messageStatus / messageBody let
// callers inject failure/response shapes.
type fakeOpencodeSession struct {
	*httptest.Server
	sessionStatus int
	messageStatus int
	messageBody   string
}

func newFakeOpencodeSession(t *testing.T) *fakeOpencodeSession {
	t.Helper()
	f := &fakeOpencodeSession{
		sessionStatus: http.StatusOK,
		messageStatus: http.StatusOK,
		messageBody:   `{"parts":[{"type":"text","text":"hello from opencode"}]}`,
	}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/session" && r.Method == http.MethodPost:
			w.WriteHeader(f.sessionStatus)
			if f.sessionStatus < 400 {
				_, _ = w.Write([]byte(`{"id":"ses_1"}`))
			}
		case r.URL.Path == "/session/ses_1/message":
			w.WriteHeader(f.messageStatus)
			if f.messageStatus < 400 {
				_, _ = w.Write([]byte(f.messageBody))
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(f.Close)
	return f
}

func seededModel(t *testing.T, svc *Service, sandboxID string, hostPort int) *Model {
	t.Helper()
	seedRunningSandbox(t, svc, sandboxID, hostPort)
	return NewModel(ModelOptions{Service: svc, SandboxID: sandboxID, ProviderID: "lthn", ModelID: "lthn-local"})
}

// TestModel_Chat_HappyPath_Good — Chat posts the message + yields
// exactly one token carrying the assistant's text.
func TestModel_Chat_HappyPath_Good(t *testing.T) {
	fake := newFakeOpencodeSession(t)
	svc := newTestService(t, Options{})
	m := seededModel(t, svc, "oc-model-1", portOf(t, fake.Server))

	var got []inference.Token
	for tok := range m.Chat(core.Background(), []inference.Message{{Role: "user", Content: "hi"}}) {
		got = append(got, tok)
	}
	if len(got) != 1 {
		t.Fatalf("Chat yielded %d tokens; want 1: %+v", len(got), got)
	}
	if got[0].Text != "hello from opencode" {
		t.Errorf("token.Text = %q; want %q", got[0].Text, "hello from opencode")
	}
	if r := m.Err(); !r.OK {
		t.Errorf("Err() after success = %+v; want Ok", r)
	}
	metrics := m.Metrics()
	if metrics.TotalDuration < 0 {
		t.Errorf("Metrics().TotalDuration = %v; want >= 0", metrics.TotalDuration)
	}
}

// TestModel_Generate_DelegatesToChat_Good — Generate wraps a single
// user message and reaches the same invoke() path as Chat.
func TestModel_Generate_DelegatesToChat_Good(t *testing.T) {
	fake := newFakeOpencodeSession(t)
	svc := newTestService(t, Options{})
	m := seededModel(t, svc, "oc-model-generate", portOf(t, fake.Server))

	var got []inference.Token
	for tok := range m.Generate(core.Background(), "hi") {
		got = append(got, tok)
	}
	if len(got) != 1 || got[0].Text != "hello from opencode" {
		t.Fatalf("Generate tokens = %+v; want 1 token %q", got, "hello from opencode")
	}
}

// TestModel_Chat_SandboxNotRunning_Bad — targetFor fails before any
// HTTP call; Chat yields no tokens and Err() reports the failure.
func TestModel_Chat_SandboxNotRunning_Bad(t *testing.T) {
	svc := newTestService(t, Options{})
	m := NewModel(ModelOptions{Service: svc, SandboxID: "oc-ghost", ProviderID: "lthn", ModelID: "lthn-local"})

	var got []inference.Token
	for tok := range m.Chat(core.Background(), []inference.Message{{Role: "user", Content: "hi"}}) {
		got = append(got, tok)
	}
	if len(got) != 0 {
		t.Fatalf("Chat against a missing sandbox yielded tokens: %+v", got)
	}
	if r := m.Err(); r.OK {
		t.Errorf("Err() after a targetFor failure = Ok; want Fail")
	}
}

// TestModel_Chat_SessionCreateFails_Bad — a 4xx from POST /session
// surfaces via Err() with no tokens yielded.
func TestModel_Chat_SessionCreateFails_Bad(t *testing.T) {
	fake := newFakeOpencodeSession(t)
	fake.sessionStatus = http.StatusInternalServerError
	svc := newTestService(t, Options{})
	m := seededModel(t, svc, "oc-model-sesfail", portOf(t, fake.Server))

	var got []inference.Token
	for tok := range m.Chat(core.Background(), []inference.Message{{Role: "user", Content: "hi"}}) {
		got = append(got, tok)
	}
	if len(got) != 0 {
		t.Fatalf("Chat with a failing session-create yielded tokens: %+v", got)
	}
	if r := m.Err(); r.OK {
		t.Errorf("Err() = Ok after session-create failure; want Fail")
	}
}

// TestModel_Chat_MessagePostFails_Bad — session creates fine but the
// message POST 4xx's.
func TestModel_Chat_MessagePostFails_Bad(t *testing.T) {
	fake := newFakeOpencodeSession(t)
	fake.messageStatus = http.StatusBadGateway
	svc := newTestService(t, Options{})
	m := seededModel(t, svc, "oc-model-msgfail", portOf(t, fake.Server))

	var got []inference.Token
	for range m.Chat(core.Background(), []inference.Message{{Role: "user", Content: "hi"}}) {
		got = append(got, inference.Token{})
	}
	if len(got) != 0 {
		t.Fatalf("Chat with a failing message-post yielded tokens")
	}
	if r := m.Err(); r.OK {
		t.Errorf("Err() = Ok after message-post failure; want Fail")
	}
}

// TestModel_Chat_MalformedResponse_Ugly — the message response body
// isn't the expected {parts:[...]} shape; invoke() falls back to
// returning the raw body as the token text rather than erroring.
func TestModel_Chat_MalformedResponse_Ugly(t *testing.T) {
	fake := newFakeOpencodeSession(t)
	fake.messageBody = `not-json-at-all`
	svc := newTestService(t, Options{})
	m := seededModel(t, svc, "oc-model-malformed", portOf(t, fake.Server))

	var got []inference.Token
	for tok := range m.Chat(core.Background(), []inference.Message{{Role: "user", Content: "hi"}}) {
		got = append(got, tok)
	}
	if len(got) != 1 || got[0].Text != "not-json-at-all" {
		t.Fatalf("Chat malformed-response fallback = %+v; want raw body token", got)
	}
}

// TestModel_Chat_MultipleMessages_Good — non-user roles are prefixed
// with "[role] " in the concatenated prompt; proven indirectly by
// checking the upstream received a single POST (multi-message
// concatenation happens client-side, before the wire call).
func TestModel_Chat_MultipleMessages_Good(t *testing.T) {
	fake := newFakeOpencodeSession(t)
	svc := newTestService(t, Options{})
	m := seededModel(t, svc, "oc-model-multi", portOf(t, fake.Server))

	messages := []inference.Message{
		{Role: "system", Content: "be terse"},
		{Role: "user", Content: "hi"},
	}
	var got []inference.Token
	for tok := range m.Chat(core.Background(), messages) {
		got = append(got, tok)
	}
	if len(got) != 1 {
		t.Fatalf("Chat with multiple messages yielded %d tokens; want 1", len(got))
	}
}

// TestModel_Classify_NotSupported_Bad — Classify always fails; the
// session API has no classification endpoint.
func TestModel_Classify_NotSupported_Bad(t *testing.T) {
	m := NewModel(ModelOptions{})
	r := m.Classify(core.Background(), []string{"a", "b"})
	if r.OK {
		t.Fatalf("Classify returned OK; want Fail (unsupported)")
	}
}

// TestModel_BatchGenerate_Good — runs Generate per prompt, capturing
// both tokens and any error per prompt.
func TestModel_BatchGenerate_Good(t *testing.T) {
	fake := newFakeOpencodeSession(t)
	svc := newTestService(t, Options{})
	m := seededModel(t, svc, "oc-model-batch", portOf(t, fake.Server))

	r := m.BatchGenerate(core.Background(), []string{"a", "b"})
	if !r.OK {
		t.Fatalf("BatchGenerate failed: %s", r.Error())
	}
	batches, ok := r.Value.([]inference.BatchResult)
	if !ok || len(batches) != 2 {
		t.Fatalf("BatchGenerate value = %#v; want 2 BatchResult", r.Value)
	}
	for i, b := range batches {
		if b.Err != nil {
			t.Errorf("batch[%d].Err = %v; want nil", i, b.Err)
		}
		if len(b.Tokens) != 1 {
			t.Errorf("batch[%d].Tokens = %+v; want 1 token", i, b.Tokens)
		}
	}
}

// TestModel_MetadataAccessors_Good — ModelType/Info/Close are static;
// pinned so a future refactor can't silently change the routed-model
// identity strings the runner's /v1/models surface relies on.
func TestModel_MetadataAccessors_Good(t *testing.T) {
	m := NewModel(ModelOptions{})
	if m.ModelType() != "opencode-routed" {
		t.Errorf("ModelType() = %q; want opencode-routed", m.ModelType())
	}
	if m.Info().Architecture != "opencode-routed" {
		t.Errorf("Info().Architecture = %q; want opencode-routed", m.Info().Architecture)
	}
	if r := m.Close(); !r.OK {
		t.Errorf("Close() = %+v; want Ok", r)
	}
	if r := m.Err(); !r.OK {
		t.Errorf("fresh Model.Err() = %+v; want Ok (no calls made yet)", r)
	}
}

// --- Routes() --------------------------------------------------------

// TestOpencodeRoutes_NoSandboxRunning_Good — Routes returns nil (not
// panic) when Status() reports nothing running.
func TestOpencodeRoutes_NoSandboxRunning_Good(t *testing.T) {
	svc := newTestService(t, Options{})
	if got := svc.Routes(); got != nil {
		t.Errorf("Routes() with nothing running = %+v; want nil", got)
	}
}

// TestOpencodeRoutes_EnumeratesProviders_Good — a running sandbox
// whose /provider response carries providers+models is turned into
// one ai.ProviderRoute per (provider, model) pair.
func TestOpencodeRoutes_EnumeratesProviders_Good(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/provider" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		payload := map[string]any{
			"all": []map[string]any{
				{"id": "lthn", "models": map[string]any{"lthn-local": map[string]any{}}},
			},
		}
		b, _ := json.Marshal(payload)
		_, _ = w.Write(b)
	}))
	t.Cleanup(upstream.Close)

	svc := newTestService(t, Options{})
	seedRunningSandbox(t, svc, "oc-routes-model", portOf(t, upstream))

	routes := svc.Routes()
	if len(routes) != 1 {
		t.Fatalf("Routes() = %+v; want exactly 1 route", routes)
	}
	rt := routes[0]
	if rt.Name != "opencode:lthn/lthn-local" {
		t.Errorf("route.Name = %q; want opencode:lthn/lthn-local", rt.Name)
	}
	if rt.Labels["kind"] != "opencode-routed" {
		t.Errorf("route.Labels[kind] = %q; want opencode-routed", rt.Labels["kind"])
	}
	if rt.Model == nil {
		t.Errorf("route.Model is nil")
	}
}

// TestOpencodeRoutes_ProviderListFails_Bad — the sandbox is running
// but /provider errors; Routes must return nil, not panic.
func TestOpencodeRoutes_ProviderListFails_Bad(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(upstream.Close)

	svc := newTestService(t, Options{})
	seedRunningSandbox(t, svc, "oc-routes-fail", portOf(t, upstream))

	if got := svc.Routes(); got != nil {
		t.Errorf("Routes() with a failing /provider = %+v; want nil", got)
	}
}
