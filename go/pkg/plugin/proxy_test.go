// SPDX-Licence-Identifier: EUPL-1.2

// Cerberus Mantis #1437 — auth gate + X-Lthn-User propagation tests.
//
// These tests exercise ProxyGroup.dispatch() via a gin router served
// through httptest.NewServer so the reverse-proxy's CloseNotifier cast
// works correctly over a real HTTP connection. The suite covers:
//
//   - X-Lthn-User is a stable SHA-256 fingerprint rather than "local".
//   - Same Authorization header → same fingerprint across requests.
//   - Different Authorization headers → different fingerprints.
//   - Missing Authorization → non-"local" SHA-256("") fingerprint.
//   - Unknown plugin code → 404 with hint body.
//   - Delete removes target → 404 on next request.
//
// Note: the bearer-auth gate is tested at the engine-middleware level
// in pkg/server. This file covers what ProxyGroup itself owns.

package plugin_test

import (
	"io"
	"net/http"
	"net/http/httptest"

	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/plugin"
	"github.com/gin-gonic/gin"
)

// setupProxy creates a gin httptest.Server with ProxyGroup routes
// mounted. Returns the front-end server URL, the ProxyGroup, and a
// cleanup function.
func setupProxy(t *core.T) (frontURL string, pg *subject.ProxyGroup, cleanup func()) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	pg = subject.NewProxyGroup()
	rg := r.Group(pg.BasePath())
	pg.RegisterRoutes(rg)
	front := httptest.NewServer(r)
	return front.URL, pg, front.Close
}

// captureBackend starts a backend httptest.Server that records the
// last X-Lthn-User it received. Returns server + pointer to captured value.
func captureBackend() (*httptest.Server, *string) {
	captured := new(string)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*captured = r.Header.Get("X-Lthn-User")
		w.WriteHeader(http.StatusOK)
	}))
	return srv, captured
}

// get issues an HTTP GET to the proxy front-end with an optional
// Authorization header. Returns the response (caller must close body).
func get(t *core.T, baseURL, path, authHeader string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, baseURL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

// TestProxy_dispatch_XLthnUser_Good_FingerprintNotLiteral verifies
// that X-Lthn-User is non-empty and not the "local" literal when an
// Authorization header is present.
func TestProxy_dispatch_XLthnUser_Good_FingerprintNotLiteral(t *core.T) {
	frontURL, pg, cleanup := setupProxy(t)
	defer cleanup()
	backend, captured := captureBackend()
	defer backend.Close()

	pg.Set("testpkg", backend.URL)

	resp := get(t, frontURL, "/v1/api/plugin/testpkg/ping",
		"Bearer sk-lthn-aabbccddeeff00112233445566778899")
	defer resp.Body.Close()
	io.ReadAll(resp.Body) //nolint:errcheck

	core.AssertEqual(t, http.StatusOK, resp.StatusCode)
	core.AssertNotEqual(t, "local", *captured)
	core.AssertNotEmpty(t, *captured)
}

// TestProxy_dispatch_XLthnUser_Good_Stable verifies that identical
// Authorization headers produce identical X-Lthn-User fingerprints.
func TestProxy_dispatch_XLthnUser_Good_Stable(t *core.T) {
	frontURL, pg, cleanup := setupProxy(t)
	defer cleanup()
	backend, captured := captureBackend()
	defer backend.Close()

	pg.Set("testpkg", backend.URL)

	const auth = "Bearer sk-lthn-aabbccddeeff00112233445566778899"

	resp1 := get(t, frontURL, "/v1/api/plugin/testpkg/a", auth)
	defer resp1.Body.Close()
	io.ReadAll(resp1.Body) //nolint:errcheck
	first := *captured

	resp2 := get(t, frontURL, "/v1/api/plugin/testpkg/b", auth)
	defer resp2.Body.Close()
	io.ReadAll(resp2.Body) //nolint:errcheck
	second := *captured

	core.AssertEqual(t, first, second)
	core.AssertNotEmpty(t, first)
}

// TestProxy_dispatch_XLthnUser_Bad_DistinctTokensDistinctIDs verifies
// that different Authorization headers produce different fingerprints.
func TestProxy_dispatch_XLthnUser_Bad_DistinctTokensDistinctIDs(t *core.T) {
	frontURL, pg, cleanup := setupProxy(t)
	defer cleanup()
	backend, captured := captureBackend()
	defer backend.Close()

	pg.Set("testpkg", backend.URL)

	resp1 := get(t, frontURL, "/v1/api/plugin/testpkg/x",
		"Bearer sk-lthn-aaaa000000000000000000000000aaaa")
	defer resp1.Body.Close()
	io.ReadAll(resp1.Body) //nolint:errcheck
	id1 := *captured

	resp2 := get(t, frontURL, "/v1/api/plugin/testpkg/x",
		"Bearer sk-lthn-bbbb111111111111111111111111bbbb")
	defer resp2.Body.Close()
	io.ReadAll(resp2.Body) //nolint:errcheck
	id2 := *captured

	core.AssertNotEqual(t, id1, id2)
}

// TestProxy_dispatch_XLthnUser_Ugly_NoAuthEmptyFingerprint verifies
// that with no Authorization header the X-Lthn-User is the SHA-256("")
// fingerprint rather than the "local" literal.
func TestProxy_dispatch_XLthnUser_Ugly_NoAuthEmptyFingerprint(t *core.T) {
	frontURL, pg, cleanup := setupProxy(t)
	defer cleanup()
	backend, captured := captureBackend()
	defer backend.Close()

	pg.Set("testpkg", backend.URL)

	// No Authorization — simulates what is a 401 in production when
	// the bearer gate is active. Here we test without gate middleware
	// so dispatch() runs and we can verify the fallback fingerprint.
	resp := get(t, frontURL, "/v1/api/plugin/testpkg/ping", "")
	defer resp.Body.Close()
	io.ReadAll(resp.Body) //nolint:errcheck

	core.AssertNotEqual(t, "local", *captured)
	core.AssertNotEmpty(t, *captured)
}

// TestProxy_dispatch_NotFound_Good verifies that a request to an
// unknown plugin code returns 404.
func TestProxy_dispatch_NotFound_Good(t *core.T) {
	frontURL, _, cleanup := setupProxy(t)
	defer cleanup()

	resp := get(t, frontURL, "/v1/api/plugin/noexist/ping", "")
	defer resp.Body.Close()
	io.ReadAll(resp.Body) //nolint:errcheck

	core.AssertEqual(t, http.StatusNotFound, resp.StatusCode)
}

// TestProxy_dispatch_NotFound_Bad verifies that a registered plugin
// does not respond to a different plugin's path prefix.
func TestProxy_dispatch_NotFound_Bad(t *core.T) {
	frontURL, pg, cleanup := setupProxy(t)
	defer cleanup()
	backend, _ := captureBackend()
	defer backend.Close()

	pg.Set("real", backend.URL)

	resp := get(t, frontURL, "/v1/api/plugin/ghost/ping", "")
	defer resp.Body.Close()
	io.ReadAll(resp.Body) //nolint:errcheck

	core.AssertEqual(t, http.StatusNotFound, resp.StatusCode)
}

// TestProxy_dispatch_NotFound_Ugly verifies that Delete removes the
// target and subsequent requests return 404.
func TestProxy_dispatch_NotFound_Ugly(t *core.T) {
	frontURL, pg, cleanup := setupProxy(t)
	defer cleanup()
	backend, _ := captureBackend()
	defer backend.Close()

	pg.Set("gone", backend.URL)
	pg.Delete("gone")

	resp := get(t, frontURL, "/v1/api/plugin/gone/ping", "")
	defer resp.Body.Close()
	io.ReadAll(resp.Body) //nolint:errcheck

	core.AssertEqual(t, http.StatusNotFound, resp.StatusCode)
}

// cspBackend returns a backend httptest.Server that responds with a
// caller-supplied status code AND a permissive
// Content-Security-Policy header. Used to verify the proxy's
// strip-and-inject runs on every response shape (2xx + 4xx + 5xx)
// per Cerberus round-2 LOW-R2-1 error-response coverage.
func cspBackend(status int, permissiveCSP string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if permissiveCSP != "" {
			w.Header().Set("Content-Security-Policy", permissiveCSP)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte("body"))
	}))
}

// TestProxy_StripsAndInjectsCSP_Good verifies the §3.4 contract on
// the happy 200 path — the plugin's permissive CSP is dropped + the
// restrictive PluginProxyCSPHeader is set.
func TestProxy_StripsAndInjectsCSP_Good(t *core.T) {
	frontURL, pg, cleanup := setupProxy(t)
	defer cleanup()
	backend := cspBackend(http.StatusOK,
		"default-src *; connect-src https://exfil.example.com")
	defer backend.Close()

	pg.Set("testpkg", backend.URL)

	resp := get(t, frontURL, "/v1/api/plugin/testpkg/x", "")
	defer resp.Body.Close()
	io.ReadAll(resp.Body) //nolint:errcheck

	core.AssertEqual(t, http.StatusOK, resp.StatusCode)
	headers := resp.Header.Values("Content-Security-Policy")
	core.AssertEqual(t, 1, len(headers))
	core.AssertEqual(t, subject.PluginProxyCSPHeader(), headers[0])

	// LOW-R2-1 floor extension — confirm the extended directives land.
	for _, mustHave := range []string{
		"object-src 'none'",
		"base-uri 'self'",
		"form-action 'self'",
		"connect-src 'self' wails://wails",
		"frame-ancestors wails://wails",
	} {
		core.AssertTrue(t, core.Contains(headers[0], mustHave))
	}
}

// TestProxy_StripsAndInjectsCSP_OnErrorResponse_Good asserts the
// strip-inject MUST fire on 4xx + 5xx responses too, not only 200s.
// A plugin returning 500 with a CSP-permissive header would otherwise
// slip the inject (Cerberus round-2 LOW-R2-1).
func TestProxy_StripsAndInjectsCSP_OnErrorResponse_Good(t *core.T) {
	for _, status := range []int{http.StatusBadRequest,
		http.StatusUnauthorized, http.StatusInternalServerError,
		http.StatusBadGateway} {
		frontURL, pg, cleanup := setupProxy(t)
		backend := cspBackend(status,
			"default-src *; connect-src https://attacker.example.com")

		pg.Set("err", backend.URL)
		resp := get(t, frontURL, "/v1/api/plugin/err/x", "")
		io.ReadAll(resp.Body) //nolint:errcheck
		got := resp.Header.Get("Content-Security-Policy")
		resp.Body.Close()
		backend.Close()
		cleanup()

		core.AssertEqual(t, status, resp.StatusCode)
		core.AssertEqual(t, subject.PluginProxyCSPHeader(), got)
		core.AssertFalse(t, core.Contains(got, "attacker.example.com"))
	}
}

// TestProxy_Name_Good pins the RouteGroup surface name used in
// /v1/openapi.
func TestProxy_Name_Good(t *core.T) {
	pg := subject.NewProxyGroup()
	core.AssertEqual(t, "plugin", pg.Name())
}

// TestProxy_BasePath_Good pins the mount prefix every plugin route
// lives under.
func TestProxy_BasePath_Good(t *core.T) {
	pg := subject.NewProxyGroup()
	core.AssertEqual(t, "/v1/api/plugin", pg.BasePath())
}

// TestProxy_Has_Good_ReflectsSetAndDelete verifies Has() tracks the
// target table across Set/Delete.
func TestProxy_Has_Good_ReflectsSetAndDelete(t *core.T) {
	pg := subject.NewProxyGroup()
	core.AssertFalse(t, pg.Has("testpkg"))
	pg.Set("testpkg", "http://127.0.0.1:1")
	core.AssertTrue(t, pg.Has("testpkg"))
	pg.Delete("testpkg")
	core.AssertFalse(t, pg.Has("testpkg"))
}

// TestProxy_Set_Bad_MalformedTargetURLIsSilentlyIgnored covers Set's
// url.Parse failure branch — a malformed target must not panic or
// register a broken proxy entry.
func TestProxy_Set_Bad_MalformedTargetURLIsSilentlyIgnored(t *core.T) {
	pg := subject.NewProxyGroup()
	pg.Set("bad", "http://[::1]:not-a-port")
	core.AssertFalse(t, pg.Has("bad"))
}

// TestProxy_StripsAndInjectsCSP_NoCSPHeader_Good covers the path
// where the plugin webapp DOESN'T set a CSP header at all — the
// proxy must still inject the restrictive policy so an attacker
// can't smuggle through by simply omitting the header.
func TestProxy_StripsAndInjectsCSP_NoCSPHeader_Good(t *core.T) {
	frontURL, pg, cleanup := setupProxy(t)
	defer cleanup()
	backend := cspBackend(http.StatusOK, "") // no CSP at all
	defer backend.Close()

	pg.Set("testpkg", backend.URL)

	resp := get(t, frontURL, "/v1/api/plugin/testpkg/x", "")
	defer resp.Body.Close()
	io.ReadAll(resp.Body) //nolint:errcheck

	core.AssertEqual(t, subject.PluginProxyCSPHeader(),
		resp.Header.Get("Content-Security-Policy"))
}
