// SPDX-Licence-Identifier: EUPL-1.2

// Tests for the runner RouteGroup. Drives the gin engine via
// httptest.NewRecorder so the test exercises the actual handler chain
// — gin binding, json decode, runner.Service stub responses, and the
// gin.H envelope on the way back out — without binding a TCP port.

package api_test

import (
	"net/http"
	"net/http/httptest"

	core "dappco.re/go"
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
	req, _ := http.NewRequest(core.MethodGet, "/v1/runner/models", nil)
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
	req, _ := http.NewRequest(core.MethodPost, "/v1/runner/generate",
		core.NewReader(`{"prompt":"hello"}`))
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
	req, _ := http.NewRequest(core.MethodPost, "/v1/runner/generate",
		core.NewReader(`{}`))
	req.Header.Set(contentTypeHeader, applicationJSON)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	core.AssertEqual(t, core.StatusBadRequest, w.Code)
}

func TestRunnerGroup_Chat_RoundTripsLastUserMessage(t *core.T) {
	engine := newTestEngine(t)
	req, _ := http.NewRequest(core.MethodPost, "/v1/runner/chat",
		core.NewReader(`{"messages":[{"role":"user","content":"ping"}]}`))
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
