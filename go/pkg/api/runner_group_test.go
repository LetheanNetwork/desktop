// SPDX-Licence-Identifier: EUPL-1.2

// Tests for the runner RouteGroup. Drives the gin engine via
// httptest.NewRecorder so the test exercises the actual handler chain
// — gin binding, json decode, runner.Service stub responses, and the
// gin.H envelope on the way back out — without binding a TCP port.

package api_test

import (
	"context"
	"iter"
	"net/http/httptest"

	core "dappco.re/go"
	"dappco.re/go/ai/ai"
	"dappco.re/go/inference"
	"github.com/gin-gonic/gin"

	lthnapi "dappco.re/lthn/desktop/pkg/api"
	"dappco.re/lthn/desktop/pkg/runner"
)

const (
	contentTypeHeader = "Content-Type"
	applicationJSON   = "application/json"
)

// newTestEngine constructs a bare gin engine and registers the
// RunnerGroup on its root RouterGroup. Mirrors what core/api's
// Engine does internally for full coverage.
func newTestEngine(t *core.T) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := runner.NewService(runner.Options{})
	g := lthnapi.NewRunnerGroup(r)
	core.AssertEqual(t, "runner", g.Name())
	core.AssertEqual(t, "/v1", g.BasePath())

	engine := gin.New()
	rg := engine.Group(g.BasePath())
	g.RegisterRoutes(rg)
	return engine
}

func TestRunnerGroup_Models_ReturnsStubEmptyList(t *core.T) {
	engine := newTestEngine(t)
	req := core.NewHTTPRequest(core.MethodGet, "/v1/runner/models", nil).Value.(*core.Request)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	core.AssertEqual(t, core.StatusOK, w.Code)
	body := w.Body.String()
	if !core.Contains(body, `"models"`) {
		t.Fatalf("expected models key in body, got %q", body)
	}
}

func TestRunnerGroup_Generate_EchoesPromptViaStub(t *core.T) {
	engine := newTestEngine(t)
	req := core.NewHTTPRequest(core.MethodPost, "/v1/runner/generate",
		core.NewReader(`{"prompt":"hello"}`)).Value.(*core.Request)
	req.Header.Set(contentTypeHeader, applicationJSON)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	core.AssertEqual(t, core.StatusOK, w.Code)
	body := w.Body.String()
	if !core.Contains(body, "[lthn stub] received: hello") {
		t.Fatalf("expected stub reply in body, got %q", body)
	}
}

func TestRunnerGroup_Generate_400OnMissingPrompt(t *core.T) {
	engine := newTestEngine(t)
	req := core.NewHTTPRequest(core.MethodPost, "/v1/runner/generate",
		core.NewReader(`{}`)).Value.(*core.Request)
	req.Header.Set(contentTypeHeader, applicationJSON)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	core.AssertEqual(t, core.StatusBadRequest, w.Code)
}

func TestRunnerGroup_Chat_RoundTripsLastUserMessage(t *core.T) {
	engine := newTestEngine(t)
	req := core.NewHTTPRequest(core.MethodPost, "/v1/runner/chat",
		core.NewReader(`{"messages":[{"role":"user","content":"ping"}]}`)).Value.(*core.Request)
	req.Header.Set(contentTypeHeader, applicationJSON)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	core.AssertEqual(t, core.StatusOK, w.Code)
	body := w.Body.String()
	if !core.Contains(body, "[lthn stub] received: ping") {
		t.Fatalf("expected stub reply in body, got %q", body)
	}
}

func TestRunnerGroup_Describe_CoversEveryRegisteredRoute(t *core.T) {
	g := lthnapi.NewRunnerGroup(runner.NewService(runner.Options{}))
	descs := g.Describe()
	core.AssertEqual(t, 3, len(descs))

	wantPaths := map[string]bool{
		"/runner/models":   false,
		"/runner/generate": false,
		"/runner/chat":     false,
	}
	for _, d := range descs {
		if _, ok := wantPaths[d.Path]; ok {
			wantPaths[d.Path] = true
		}
	}
	for p, found := range wantPaths {
		if !found {
			t.Fatalf("Describe() missing RouteDescription for %s", p)
		}
	}
}

func TestRunnerGroup_Register_NilRunnerFails(t *core.T) {
	c := core.New()
	r := lthnapi.Register(c, nil)
	core.AssertEqual(t, false, r.OK)
}

func TestRunnerGroup_Register_NilCoreFails(t *core.T) {
	r := lthnapi.Register(nil, runner.NewService(runner.Options{}))
	core.AssertEqual(t, false, r.OK)
	core.AssertNotEmpty(t, r.Error())
}

func TestRunnerGroup_Register_NoAPIServiceIsOk(t *core.T) {
	c := core.New()
	r := lthnapi.Register(c, runner.NewService(runner.Options{}))
	core.AssertTrue(t, r.OK)
}

// runnerStubModel is a minimal inference.TextModel that yields a fixed
// reply token — enough surface area for ai.ProviderRouter.Chat to round-
// trip without network egress. Mirror of pkg/runner/audit_test.go's
// runnerStubModel; replicated here because the pkg/runner test helpers
// live in `package runner_test` and are not importable from pkg/api_test.
type runnerStubModel struct{ output string }

func (m *runnerStubModel) seq() iter.Seq[inference.Token] {
	out := m.output
	return func(yield func(inference.Token) bool) {
		if out == "" {
			return
		}
		yield(inference.Token{Text: out})
	}
}

func (m *runnerStubModel) Generate(ctx context.Context, prompt string, opts ...inference.GenerateOption) iter.Seq[inference.Token] {
	return m.seq()
}

func (m *runnerStubModel) Chat(ctx context.Context, messages []inference.Message, opts ...inference.GenerateOption) iter.Seq[inference.Token] {
	return m.seq()
}

func (m *runnerStubModel) Classify(context.Context, []string, ...inference.GenerateOption) ([]inference.ClassifyResult, error) {
	return nil, nil
}

func (m *runnerStubModel) BatchGenerate(context.Context, []string, ...inference.GenerateOption) ([]inference.BatchResult, error) {
	return nil, nil
}

func (m *runnerStubModel) ModelType() string                  { return "stub" }
func (m *runnerStubModel) Info() inference.ModelInfo          { return inference.ModelInfo{} }
func (m *runnerStubModel) Metrics() inference.GenerateMetrics { return inference.GenerateMetrics{} }
func (m *runnerStubModel) Err() error                         { return nil }
func (m *runnerStubModel) Close() error                       { return nil }

// newRouterBackedEngine wires a runner.Service with a real
// ai.ProviderRouter (backed by runnerStubModel) into the RunnerGroup.
// Distinct from newTestEngine because the latter constructs the runner
// with empty Routes, which short-circuits the GenerateCtx / ChatCtx
// stub path BEFORE ctx is consulted — cancellation cannot be observed
// against that surface. The router-backed path runs ai.ProviderRouter.Chat
// which checks ctx.Err() before each route attempt and returns
// "request cancelled" when set.
func newRouterBackedEngine(t *core.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := runner.NewService(runner.Options{
		Routes: []ai.ProviderRoute{
			{
				Name:    "stub-provider",
				ModelID: "stub-model",
				Model:   &runnerStubModel{output: "would-be-reply"},
			},
		},
	})
	g := lthnapi.NewRunnerGroup(r)
	engine := gin.New()
	rg := engine.Group(g.BasePath())
	g.RegisterRoutes(rg)
	return engine
}

// TestRunnerGroup_Generate_ClientDisconnect_Bad — Cerberus #60 F-2 /
// Mantis #1711 (H#220 cluster close): when a client disconnects mid-
// call (gin c.Request.Context() cancelled by the http server because
// the TCP peer went away), handleGenerate must propagate the cancelled
// ctx into runner.GenerateCtx → ai.ProviderRouter.Chat so the in-flight
// provider RTT terminates instead of running to completion and burning
// quota against the Requested audit row.
//
// Before H#223 the handler called runner.Generate(prompt) which routes
// through Background() and ignores the gin request context entirely —
// cancellation was silently dropped at the pkg/api boundary even though
// pkg/server/handlers.go had landed the fix for the OpenAI-compat
// surface. This test pins the discipline: a pre-cancelled request MUST
// surface as a non-200 envelope, proving ctx is plumbed through.
func TestRunnerGroup_Generate_ClientDisconnect_Bad(t *core.T) {
	engine := newRouterBackedEngine(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel — simulates client disconnect BEFORE inference completes

	req := core.NewHTTPRequestContext(ctx, core.MethodPost, "/v1/runner/generate",
		core.NewReader(`{"prompt":"hello"}`)).Value.(*core.Request)
	req.Header.Set(contentTypeHeader, applicationJSON)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code == core.StatusOK {
		t.Fatalf("cancelled ctx should propagate into router.Chat → 500 envelope, got 200 body=%q (ctx was dropped)", w.Body.String())
	}
	core.AssertEqual(t, core.StatusInternalServerError, w.Code)
}

// TestRunnerGroup_Chat_ClientDisconnect_Bad — Chat-path mirror of the
// Generate variant. Both /v1/runner/* HTTP egress sites must honour
// client-disconnect cancellation uniformly per the H#220 cluster-close
// discipline. See TestRunnerGroup_Generate_ClientDisconnect_Bad for the
// full rationale.
func TestRunnerGroup_Chat_ClientDisconnect_Bad(t *core.T) {
	engine := newRouterBackedEngine(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel — simulates client disconnect

	req := core.NewHTTPRequestContext(ctx, core.MethodPost, "/v1/runner/chat",
		core.NewReader(`{"messages":[{"role":"user","content":"ping"}]}`)).Value.(*core.Request)
	req.Header.Set(contentTypeHeader, applicationJSON)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code == core.StatusOK {
		t.Fatalf("cancelled ctx should propagate into router.Chat → 500 envelope, got 200 body=%q (ctx was dropped)", w.Body.String())
	}
	core.AssertEqual(t, core.StatusInternalServerError, w.Code)
}
