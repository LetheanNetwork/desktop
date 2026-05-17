// SPDX-Licence-Identifier: EUPL-1.2

// Tests for the global body-cap middleware (Mantis #1568 Unit C,
// Cerberus #16). Per RFC.body-cap-middleware.md §4.2 — Good × 4,
// Bad × 1, Ugly × 2, Sentinel × 1, DefaultOverrides-pin × 1.
//
// The middleware mounts on a bare gin.Engine for unit-test isolation;
// integration coverage (middleware actually wired by NewService onto
// both engines) lives in service_test.go's
// TestService_BodyCap_Default_Bad_AllRoutesCappedDefault.

package server_test

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/gateway"
	"dappco.re/lthn/desktop/pkg/server"
	"github.com/gin-gonic/gin"
)

// readBodyHandler is the canonical downstream-handler fixture for these
// tests. Reads the entire request body via io.ReadAll so MaxBytesReader
// errors surface; renders 413 + body.too_large per the Amendment A1
// envelope on *http.MaxBytesError, 200 with the byte count otherwise.
func readBodyHandler(c *gin.Context) {
	n, err := io.Copy(io.Discard, c.Request.Body)
	if err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{
				"success": false,
				"error":   gin.H{"code": "body.too_large"},
			})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"code": "body.read_failed"}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"bytes": n}})
}

// newCapEngine constructs a bare gin.Engine with the body-cap middleware
// installed and a single catch-all POST handler that reads the body.
// Per-test fixture so the route shape stays minimal and the cap
// behaviour is the only thing under test.
func newCapEngine(t *testing.T, defaultCap int64, overrides []server.BodyCapOverride) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	eng := gin.New()
	eng.Use(server.BodyCapMiddleware(defaultCap, overrides))
	eng.POST("/*any", readBodyHandler)
	return eng
}

// postBytes drives n bytes of body at the engine and returns the
// recorder. Body content is mechanical ('x' repeated); the tests are
// about the cap, not the content.
func postBytes(t *testing.T, eng *gin.Engine, path string, n int) *httptest.ResponseRecorder {
	t.Helper()
	body := bytes.NewReader([]byte(strings.Repeat("x", n)))
	req := httptest.NewRequest(http.MethodPost, path, body)
	req.Header.Set("Content-Type", "application/octet-stream")
	w := httptest.NewRecorder()
	eng.ServeHTTP(w, req)
	return w
}

// TestBodyCap_DefaultCap_413_Bad — body over MaxBodyBytesDefault on a
// non-overridden path returns 413 + body.too_large per Amendment A1.
func TestBodyCap_DefaultCap_413_Bad(t *testing.T) {
	eng := newCapEngine(t, server.MaxBodyBytesDefault, nil)
	w := postBytes(t, eng, "/v1/unknown/route", int(server.MaxBodyBytesDefault)+1)
	core.AssertEqual(t, http.StatusRequestEntityTooLarge, w.Code)
	core.AssertTrue(t, core.Contains(w.Body.String(), "body.too_large"),
		"expected body.too_large envelope on default-cap overflow")
}

// TestBodyCap_GatewayOverride_413_Bad — body over gateway.MaxBodyBytes
// on /v1/api/gateway/ rejects at 413. Pins that the override is wired
// through to MaxBytesReader correctly. We use cap+1 with a smaller cap
// for test speed by using the default override table's gateway entry
// but driving exactly past the boundary.
func TestBodyCap_GatewayOverride_413_Bad(t *testing.T) {
	// Use a synthetic small override so the test doesn't have to
	// allocate 4 MiB+1 bytes. The behaviour under test is "override
	// fires on prefix match"; the byte values are independent.
	overrides := []server.BodyCapOverride{
		{Prefix: "/v1/api/gateway/", Bytes: 1 << 10}, // 1 KiB
	}
	eng := newCapEngine(t, server.MaxBodyBytesDefault, overrides)
	w := postBytes(t, eng, "/v1/api/gateway/project.metadata/read", (1<<10)+1)
	core.AssertEqual(t, http.StatusRequestEntityTooLarge, w.Code)
	core.AssertTrue(t, core.Contains(w.Body.String(), "body.too_large"),
		"expected body.too_large envelope on gateway-override overflow")
}

// TestBodyCap_HealthOverride_413_Bad — body over MaxBodyBytesHealth on
// /health rejects at 413. Pins the 4 KiB floor.
func TestBodyCap_HealthOverride_413_Bad(t *testing.T) {
	overrides := []server.BodyCapOverride{
		{Prefix: "/health", Bytes: server.MaxBodyBytesHealth},
	}
	eng := newCapEngine(t, server.MaxBodyBytesDefault, overrides)
	w := postBytes(t, eng, "/health", int(server.MaxBodyBytesHealth)+1)
	core.AssertEqual(t, http.StatusRequestEntityTooLarge, w.Code)
	core.AssertTrue(t, core.Contains(w.Body.String(), "body.too_large"),
		"expected body.too_large envelope on health-override overflow")
}

// TestBodyCap_DefaultOverrides_Good — pins DefaultBodyCapOverrides
// table (constants + prefixes) so future drift is test-caught.
// Sources gateway cap from pkg/gateway.MaxBodyBytes directly (single-
// source per Amendment A5) — this test fails loudly if the gateway
// constant moves and the override table isn't re-sourced.
//
// Per RFC §3.2.1 — /v1/plugin-view/capability-grant deliberately is
// NOT in this table (inner-wrap discipline). Asserting its absence
// makes the discipline explicit.
//
// Cerberus #57 F-3 (Mantis #1701) — /mcp/ and /v1/mcp/ are
// deliberately NOT in this table. The MCP transport surface lives on
// pkg/bridge's own core.HTTPServer + core.NewServeMux, not on the
// coreapi.Engine this middleware wraps. The previous entries here
// were signal-leak dead config; asserting their absence pins the
// honesty.
func TestBodyCap_DefaultOverrides_Good(t *testing.T) {
	expect := map[string]int64{
		"/v1/api/gateway/": gateway.MaxBodyBytes,
		"/health":          server.MaxBodyBytesHealth,
	}
	got := map[string]int64{}
	for _, o := range server.DefaultBodyCapOverrides {
		got[o.Prefix] = o.Bytes
	}
	core.AssertEqual(t, len(expect), len(got))
	for prefix, bytes := range expect {
		v, ok := got[prefix]
		core.AssertTrue(t, ok, "DefaultBodyCapOverrides missing prefix: "+prefix)
		core.AssertEqual(t, bytes, v)
	}
	// Per RFC §3.2.1 — capability-grant stays out of the registry.
	_, present := got["/v1/plugin-view/capability-grant"]
	core.AssertTrue(t, !present,
		"capability-grant MUST NOT be in DefaultBodyCapOverrides (RFC §3.2.1 inner-wrap discipline)")
	// Per Cerberus #57 F-3 — bridge-only prefixes stay out of the registry.
	_, mcpPresent := got["/mcp/"]
	core.AssertTrue(t, !mcpPresent,
		"/mcp/ MUST NOT be in DefaultBodyCapOverrides (bridge-only path, Cerberus #57 F-3)")
	_, mcpV1Present := got["/v1/mcp/"]
	core.AssertTrue(t, !mcpV1Present,
		"/v1/mcp/ MUST NOT be in DefaultBodyCapOverrides (bridge-only path, Cerberus #57 F-3)")
}

// TestBodyCapMiddleware_TwoWraps_Good_InnerWins — when a handler adds
// its own tighter MaxBytesReader wrap on top of the global middleware,
// the inner (tighter) cap fires first. The pattern lives on
// capability-grant today; this test pins the contract independently
// so a future inner-wrap-removal would surface as a test failure here
// even if capability-grant's own tests stay green.
func TestBodyCapMiddleware_TwoWraps_Good_InnerWins(t *testing.T) {
	gin.SetMode(gin.TestMode)
	eng := gin.New()
	// Outer: 16 KiB default, no overrides.
	eng.Use(server.BodyCapMiddleware(16<<10, nil))
	// Inner: 1 KiB at the handler. Tighter — should fire first.
	eng.POST("/inner-wrap", func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1<<10)
		readBodyHandler(c)
	})
	// Drive 2 KiB — under outer (16 KiB) but over inner (1 KiB).
	w := postBytes(t, eng, "/inner-wrap", 2<<10)
	core.AssertEqual(t, http.StatusRequestEntityTooLarge, w.Code)
	core.AssertTrue(t, core.Contains(w.Body.String(), "body.too_large"),
		"inner (tighter) wrap should fire first")
}

// TestBodyCapMiddleware_TwoWraps_Ugly_OuterWinsWhenTighter — Amendment
// A8 inverted-stack invariant. If a handler accidentally adds an
// inner wrap LARGER than the outer global cap, the outer (tighter)
// cap still wins because MaxBytesReader counts against the cap it was
// constructed with; the outer reader has already constrained the
// total bytes available from the underlying connection. Belt + braces
// against a future regression where someone "raises" a per-endpoint
// cap above the global and assumes it actually raises the effective
// cap.
func TestBodyCapMiddleware_TwoWraps_Ugly_OuterWinsWhenTighter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	eng := gin.New()
	// Outer: 1 KiB default — the tighter cap.
	eng.Use(server.BodyCapMiddleware(1<<10, nil))
	// Inner: 16 KiB at the handler — accidentally LOOSER.
	eng.POST("/wider-inner", func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
		readBodyHandler(c)
	})
	// Drive 4 KiB — over outer (1 KiB), under inner (16 KiB).
	// Outer should fire first because the outer reader counted the
	// bytes against its 1 KiB budget before they reached the inner.
	w := postBytes(t, eng, "/wider-inner", 4<<10)
	core.AssertEqual(t, http.StatusRequestEntityTooLarge, w.Code)
	core.AssertTrue(t, core.Contains(w.Body.String(), "body.too_large"),
		"outer (tighter) wrap should still win when inner is accidentally larger")
}

// TestBodyCapMiddleware_SentinelZero_Bypasses_Good — Amendment A4 /
// Cerberus #16 F-NEW-1. Override with Bytes: 0 short-circuits the
// middleware wrap entirely, so a 1 MiB POST reaches the handler
// unblocked (the streaming endpoint owns its own budget). Without the
// sentinel branch, cap == 0 passed to MaxBytesReader would reject
// every body byte — the exact opposite of the intent.
func TestBodyCapMiddleware_SentinelZero_Bypasses_Good(t *testing.T) {
	overrides := []server.BodyCapOverride{
		{Prefix: "/v1/api/stream/", Bytes: 0}, // streaming opt-out
	}
	eng := newCapEngine(t, 1<<10, overrides) // tight default — would otherwise reject
	// Drive 1 MiB — well over the default. Sentinel must let it through.
	w := postBytes(t, eng, "/v1/api/stream/upload", 1<<20)
	core.AssertEqual(t, http.StatusOK, w.Code)
	core.AssertTrue(t, core.Contains(w.Body.String(), `"success":true`),
		"sentinel Bytes:0 must bypass the wrap entirely")
}

// TestBodyCap_LongestPrefixWins_Good — overlapping prefixes resolve
// to the longer (more specific) match regardless of declaration order.
// Pins the longest-prefix-first sort the middleware performs at
// construction time.
func TestBodyCap_LongestPrefixWins_Good(t *testing.T) {
	overrides := []server.BodyCapOverride{
		// Declare in "wrong" order — shorter prefix first. The
		// middleware must still pick the longer prefix when a request
		// matches both.
		{Prefix: "/v1/api/", Bytes: 1 << 20},          // 1 MiB
		{Prefix: "/v1/api/gateway/", Bytes: 1 << 10},  // 1 KiB — more specific
	}
	eng := newCapEngine(t, server.MaxBodyBytesDefault, overrides)
	// 2 KiB to /v1/api/gateway/foo/bar — under the looser /v1/api/ cap
	// (1 MiB) but over the tighter /v1/api/gateway/ cap (1 KiB). Must
	// reject because the longer prefix wins.
	w := postBytes(t, eng, "/v1/api/gateway/foo/bar", 2<<10)
	core.AssertEqual(t, http.StatusRequestEntityTooLarge, w.Code)
	core.AssertTrue(t, core.Contains(w.Body.String(), "body.too_large"),
		"longer prefix /v1/api/gateway/ should win over shorter /v1/api/")
}
