// SPDX-Licence-Identifier: EUPL-1.2

// Cerberus Mantis #1435 — body-cap tests. End-to-end via newTestRouter:
// a registered scope that reads the request body is hit with a POST
// that exceeds the 4 MiB cap. The MaxBytesReader wrap in Handle causes
// the scope handler's body read to error; the dispatcher returns
// "handler-failed" 400. Small bodies still succeed.

package gateway_test

import (
	"bytes"
	"io"
	"net/http/httptest"
	"strings"

	core "dappco.re/go"
	"github.com/gin-gonic/gin"

	"dappco.re/lthn/desktop/pkg/gateway"
	"dappco.re/lthn/desktop/pkg/marketplace"
)

// echoBodyHandler is a test scope that ReadAll's the request body
// and echoes a count. If MaxBytesReader fires, ReadAll returns an
// error and we Fail.
func echoBodyHandler(_ *core.Core, _ string, ctx *gin.Context) core.Result {
	buf, err := io.ReadAll(ctx.Request.Body)
	if err != nil {
		return core.Fail(core.E("test.echoBody", "body read failed", err))
	}
	return core.Ok(map[string]any{"bytes": len(buf)})
}

// newBodyCapRouter wires the gateway with an extra "test.echo" scope
// that exercises the body-read path.
func newBodyCapRouter(t *core.T, c *core.Core) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	svc := gateway.NewService(c)
	core.RequireTrue(t, gateway.RegisterScope(svc, "test.echo", "write", echoBodyHandler).OK)
	r := gin.New()
	r.POST("/v1/api/gateway/:scope/:mode", svc.Handle)
	return r
}

func TestGateway_Handle_Bad_BodyExceedsCap(t *core.T) {
	c := newGatewayCore(t)
	seedBundle(t, c, "opencode", []marketplace.Permission{
		{Scope: "test.echo", Mode: "write"},
	})
	r := newBodyCapRouter(t, c)

	// 5 MiB body — exceeds the 4 MiB cap.
	huge := strings.Repeat("x", 5<<20)
	req := httptest.NewRequest("POST", "/v1/api/gateway/test.echo/write",
		bytes.NewReader([]byte(huge)))
	req.Header.Set("Bundle-ID", "opencode")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	core.AssertEqual(t, core.StatusBadRequest, w.Code,
		"body over cap must surface as handler-failed (the scope's ReadAll errors past the cap)")
	core.AssertContains(t, w.Body.String(), "handler-failed")
}

func TestGateway_Handle_Good_BodyUnderCap(t *core.T) {
	c := newGatewayCore(t)
	seedBundle(t, c, "opencode", []marketplace.Permission{
		{Scope: "test.echo", Mode: "write"},
	})
	r := newBodyCapRouter(t, c)

	// 1 MiB body — well under the 4 MiB cap.
	small := strings.Repeat("y", 1<<20)
	req := httptest.NewRequest("POST", "/v1/api/gateway/test.echo/write",
		bytes.NewReader([]byte(small)))
	req.Header.Set("Bundle-ID", "opencode")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	core.AssertEqual(t, core.StatusOK, w.Code,
		"under-cap body must succeed")
	core.AssertContains(t, w.Body.String(), "1048576")
}

func TestGateway_Handle_Good_EmptyBody(t *core.T) {
	c := newGatewayCore(t)
	seedBundle(t, c, "opencode", []marketplace.Permission{
		{Scope: "test.echo", Mode: "write"},
	})
	r := newBodyCapRouter(t, c)

	// Empty body — the wrap is a no-op for zero bytes.
	req := httptest.NewRequest("POST", "/v1/api/gateway/test.echo/write", nil)
	req.Header.Set("Bundle-ID", "opencode")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	core.AssertEqual(t, core.StatusOK, w.Code,
		"empty body must succeed")
	core.AssertContains(t, w.Body.String(), `"bytes":0`)
}
