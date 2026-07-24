// SPDX-Licence-Identifier: EUPL-1.2

// Tests for the per-port CSP frame-src allowlist middleware per
// plans/code/lthn/desktop/views/RFC.plugin-views.md §3.3
// (Cerberus CRIT-1). The frame-src directive is composed per
// response from the live PluginPortSource so plugin install /
// uninstall mutations are reflected on the next request.

package server

import (
	"net/http/httptest"
	"testing"

	core "dappco.re/go"
	"github.com/gin-gonic/gin"
)

// newTestEngineWithPorts returns a gin engine wired with the CSP
// middleware backed by a fixed port slice. /probe responds 204 so
// the test can inspect the rendered CSP header.
func newTestEngineWithPorts(ports []int) *gin.Engine {
	gin.SetMode(gin.TestMode)
	e := gin.New()
	e.Use(cspMiddlewareWithSource(func() []int { return ports }))
	e.GET("/probe", func(c *gin.Context) { c.Status(204) })
	return e
}

func TestServer_CSPFrameSrc_EnumeratesInstalledPlugins_Good(t *testing.T) {
	e := newTestEngineWithPorts([]int{4096, 4097})
	req := httptest.NewRequest(core.MethodGet, "/probe", nil)
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)

	csp := w.Header().Get(cspHeader)
	if csp == "" {
		t.Fatal("CSP header missing")
	}
	if !core.Contains(csp, "frame-src 'none' http://127.0.0.1:4096 http://127.0.0.1:4097") {
		t.Fatalf("CSP must enumerate installed plugin ports as loopback origins; got: %s", csp)
	}
}

func TestServer_CSPFrameSrc_DropsUninstalled_Good(t *testing.T) {
	// First response sees both ports — second response, with the
	// uninstalled port removed, sees only the surviving port. Same
	// middleware instance; source mutates between requests.
	ports := []int{4096, 4097}
	e := gin.New()
	gin.SetMode(gin.TestMode)
	e.Use(cspMiddlewareWithSource(func() []int { return ports }))
	e.GET("/probe", func(c *gin.Context) { c.Status(204) })

	req1 := httptest.NewRequest(core.MethodGet, "/probe", nil)
	w1 := httptest.NewRecorder()
	e.ServeHTTP(w1, req1)
	if !core.Contains(w1.Header().Get(cspHeader), "http://127.0.0.1:4097") {
		t.Fatalf("first response must include port 4097; got: %s",
			w1.Header().Get(cspHeader))
	}

	// Simulate uninstall: drop port 4097 from the source.
	ports = []int{4096}

	req2 := httptest.NewRequest(core.MethodGet, "/probe", nil)
	w2 := httptest.NewRecorder()
	e.ServeHTTP(w2, req2)
	csp2 := w2.Header().Get(cspHeader)
	if core.Contains(csp2, "http://127.0.0.1:4097") {
		t.Fatalf("second response must NOT include uninstalled port 4097; got: %s", csp2)
	}
	if !core.Contains(csp2, "http://127.0.0.1:4096") {
		t.Fatalf("second response must still include surviving port 4096; got: %s", csp2)
	}
}

func TestServer_CSPFrameSrc_EmptyWhenNoPlugins_Good(t *testing.T) {
	e := newTestEngineWithPorts(nil)
	req := httptest.NewRequest(core.MethodGet, "/probe", nil)
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)

	csp := w.Header().Get(cspHeader)
	if !core.Contains(csp, "frame-src 'none'") {
		t.Fatalf("empty plugin registry must emit `frame-src 'none'`; got: %s", csp)
	}
	// Inspect the frame-src directive in isolation — connect-src
	// legitimately carries bridge loopback origins (9879 / 9876) that
	// would otherwise false-positive a flat 127.0.0.1 search.
	idx := core.Index(csp, "frame-src")
	if idx < 0 {
		t.Fatalf("frame-src directive missing; got: %s", csp)
	}
	frameSrc := csp[idx:]
	if core.Contains(frameSrc, "127.0.0.1") {
		t.Fatalf("no-plugin frame-src must NOT mention any loopback origin; got frame-src: %s",
			frameSrc)
	}
}

func TestServer_CSPFrameSrc_RejectsWildcardPort_Bad(t *testing.T) {
	// BuildCSPPolicy MUST never emit a wildcard form, even given a
	// hostile or accidental source supplying a port 0 sentinel or
	// duplicate entries. Validates the silent-drop + literal-only
	// posture from §3.3 (NEVER `frame-src *`, NEVER
	// `http://127.0.0.1:*`).
	csp := BuildCSPPolicy(func() []int { return []int{0, -1, 4096, 4096} }, false)
	if core.Contains(csp, "frame-src *") || core.Contains(csp, "127.0.0.1:*") {
		t.Fatalf("CSP must never emit a wildcard frame-src; got: %s", csp)
	}
	if !core.Contains(csp, "http://127.0.0.1:4096") {
		t.Fatalf("CSP must include the valid port; got: %s", csp)
	}
	// Zero / negative entries silently dropped — exactly one literal port.
	count := 0
	for i := 0; i+len("http://127.0.0.1:4096") <= len(csp); i++ {
		if csp[i:i+len("http://127.0.0.1:4096")] == "http://127.0.0.1:4096" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("CSP must deduplicate ports; expected 1 literal, got %d in: %s", count, csp)
	}
}

func TestBuildCSPPolicy_DevGatesUnsafeEval(t *testing.T) {
	// The security invariant: the relaxed script-src keyword is dev-only.
	// Production must stay hardened; dev gets Wails' MCP drive-surface.
	// Passing dev explicitly keeps this deterministic regardless of the
	// LTHN_DEV env the test process happens to run under.
	noPorts := func() []int { return nil }
	const relaxed = "unsafe-eval"
	const wailsMCPOrigin = "http://127.0.0.1:9099"

	prod := BuildCSPPolicy(noPorts, false)
	if !core.Contains(prod, "script-src 'self'") {
		t.Fatalf("prod CSP must restrict scripts to 'self'; got: %s", prod)
	}
	if core.Contains(prod, relaxed) {
		t.Fatalf("prod CSP must NOT permit %s; got: %s", relaxed, prod)
	}
	if core.Contains(prod, wailsMCPOrigin) {
		t.Fatalf("prod CSP must NOT permit the Wails MCP callback; got: %s", prod)
	}

	dev := BuildCSPPolicy(noPorts, true)
	if !core.Contains(dev, "script-src 'self' '"+relaxed+"'") {
		t.Fatalf("dev CSP must relax script-src for Wails MCP; got: %s", dev)
	}
	if !core.Contains(dev, wailsMCPOrigin) {
		t.Fatalf("dev CSP must permit the Wails MCP callback; got: %s", dev)
	}
	// The relaxed keyword must appear exactly once — inside script-src,
	// nowhere else (no leak into connect-src / style-src / frame-src).
	count := 0
	for i := 0; i+len(relaxed) <= len(dev); i++ {
		if dev[i:i+len(relaxed)] == relaxed {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("%s must appear exactly once (script-src only); got %d in: %s", relaxed, count, dev)
	}
}
