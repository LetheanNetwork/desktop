// SPDX-Licence-Identifier: EUPL-1.2

package desktop

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"

	core "dappco.re/go"
	coreapi "dappco.re/go/api"
	mcpsvc "dappco.re/go/mcp/pkg/mcp"
	"github.com/gin-gonic/gin"

	"dappco.re/lthn/desktop/pkg/opencode"
	"dappco.re/lthn/desktop/pkg/plugin"
	"dappco.re/lthn/desktop/pkg/runner"
)

func newSubsystemsEngine(t *core.T) *coreapi.Engine {
	t.Helper()
	engine, err := coreapi.New()
	core.RequireNoError(t, err)
	return engine
}

func TestSubsystems_MountSubsystems_Bad_NilCore(t *core.T) {
	result := mountSubsystems(nil, newSubsystemsEngine(t), nil)
	core.AssertFalse(t, result.OK)
	core.AssertContains(t, result.Error(), "core is nil")
}

func TestSubsystems_MountSubsystems_Bad_NilEngine(t *core.T) {
	result := mountSubsystems(core.New(), nil, nil)
	core.AssertFalse(t, result.OK)
	core.AssertContains(t, result.Error(), "engine is nil")
}

func TestSubsystems_MountSubsystems_Good_NoOptionalServicesSkipsCleanly(t *core.T) {
	result := mountSubsystems(core.New(), newSubsystemsEngine(t), nil)
	core.AssertTrue(t, result.OK, result.Error())
}

func TestSubsystems_MountSubsystems_Good_RunnerRegistersWithoutAPIService(t *core.T) {
	c := core.New()
	r := runner.NewService(runner.Options{})
	result := mountSubsystems(c, newSubsystemsEngine(t), r)
	core.AssertTrue(t, result.OK, result.Error())
}

// TestSubsystems_MountSubsystems_Good_MountsEveryRegisteredSubsystem drives
// every branch that hinges on an optional Core service being present: the
// dappco.re/go/api sub-engine mount, the MCP tool bridge, the plugin proxy,
// and the opencode sandbox proxy. Each subsystem's own Register factory is
// deliberately zero-config — none of them open a socket at construction, so
// this stays hermetic per the pkg/desktop port-seam contract.
func TestSubsystems_MountSubsystems_Good_MountsEveryRegisteredSubsystem(t *core.T) {
	c := core.New(
		core.WithName("api", coreapi.Register),
		core.WithName("mcp", mcpsvc.Register),
		core.WithName("plugin", plugin.Register),
		core.WithName("opencode", opencode.Register),
	)
	core.RequireTrue(t, c.ServiceStartup(core.Background(), nil).OK)
	t.Cleanup(func() { _ = c.ServiceShutdown(core.Background()) })

	engine := newSubsystemsEngine(t)
	r := runner.NewService(runner.Options{})

	result := mountSubsystems(c, engine, r)
	core.RequireTrue(t, result.OK, result.Error())

	names := map[string]bool{}
	for _, group := range engine.Groups() {
		names[group.Name()] = true
	}
	core.AssertTrue(t, names["subapi"], "api sub-engine group must be mounted")
	core.AssertTrue(t, names["plugin"], "plugin proxy group must be mounted")
	core.AssertTrue(t, names["sandbox"], "opencode proxy group must be mounted")
}

func TestSubsystems_AdaptMCPRest_Bad_NilHandlerReportsUnbound(t *core.T) {
	handler := adaptMCPRest(nil)

	router := gin.New()
	router.POST("/mcp/tool", handler)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/mcp/tool", nil))

	core.AssertEqual(t, core.StatusInternalServerError, response.Code)
	core.AssertTrue(t, core.Contains(response.Body.String(), "tool_unbound"))
}

type subsystemsErrReader struct{}

func (subsystemsErrReader) Read([]byte) (int, error) {
	return 0, errors.New("subsystems: simulated body read failure")
}

func TestSubsystems_AdaptMCPRest_Bad_BodyReadFailureReportsInvalidBody(t *core.T) {
	handler := adaptMCPRest(func(_ context.Context, _ []byte) (any, error) {
		return "unreachable", nil
	})

	router := gin.New()
	router.POST("/mcp/tool", handler)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/mcp/tool", subsystemsErrReader{}))

	core.AssertEqual(t, core.StatusBadRequest, response.Code)
	core.AssertTrue(t, core.Contains(response.Body.String(), "invalid_body"))
}

func TestSubsystems_AdaptMCPRest_Bad_HandlerErrorReportsToolError(t *core.T) {
	handler := adaptMCPRest(func(_ context.Context, _ []byte) (any, error) {
		return nil, errors.New("tool blew up")
	})

	router := gin.New()
	router.POST("/mcp/tool", handler)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/mcp/tool", nil))

	core.AssertEqual(t, core.StatusInternalServerError, response.Code)
	core.AssertTrue(t, core.Contains(response.Body.String(), "tool_error"))
}

func TestSubsystems_AdaptMCPRest_Good_HandlerSuccessReportsResult(t *core.T) {
	handler := adaptMCPRest(func(_ context.Context, body []byte) (any, error) {
		return map[string]string{"echo": string(body)}, nil
	})

	router := gin.New()
	router.POST("/mcp/tool", handler)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/mcp/tool", strings.NewReader("ping"))
	router.ServeHTTP(response, request)

	core.AssertEqual(t, core.StatusOK, response.Code)
	core.AssertTrue(t, core.Contains(response.Body.String(), "ping"))
}

func TestSubsystems_SubEngineGroup_Good_ProxiesToInnerHandler(t *core.T) {
	var seenPath string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.Path
		w.WriteHeader(http.StatusTeapot)
	})
	group := &subEngineGroup{name: "subapi", basePath: "/api", handler: inner}

	core.AssertEqual(t, "subapi", group.Name())
	core.AssertEqual(t, "/api", group.BasePath())

	router := gin.New()
	rg := router.Group(group.BasePath())
	group.RegisterRoutes(rg)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/status", nil))

	core.AssertEqual(t, http.StatusTeapot, response.Code)
	core.AssertEqual(t, "/api/status", seenPath)
}
