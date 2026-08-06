// SPDX-Licence-Identifier: EUPL-1.2

// proxy_test.go — coverage for SandboxProxyGroup (proxy.go), fully
// self-contained: no Service, no orm, no kv, no exec. dispatch is
// proven against a real httptest.Server standing in for the sandbox
// container, forwarded to via a real httputil.ReverseProxy.
//
// dispatch tests drive the gin engine through a REAL httptest.Server
// (net.Conn-backed ResponseWriter) rather than httptest.NewRecorder —
// httputil.ReverseProxy.ServeHTTP calls the ResponseWriter's
// CloseNotify() when it satisfies http.CloseNotifier, and gin's
// responseWriter.CloseNotify() unconditionally type-asserts its
// underlying writer to http.CloseNotifier, which panics against a bare
// httptest.ResponseRecorder. A real server round-trip sidesteps the
// gotcha entirely and is arguably the more honest test anyway (proves
// the full HTTP stack, not just the handler function).

package opencode

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSandboxProxyGroup_NameAndBasePath_Good(t *testing.T) {
	g := NewSandboxProxyGroup()
	if g.Name() != "sandbox" {
		t.Errorf("Name() = %q; want sandbox", g.Name())
	}
	if g.BasePath() != "/v1/api/sandbox" {
		t.Errorf("BasePath() = %q; want /v1/api/sandbox", g.BasePath())
	}
}

// newDispatchServer wires g.RegisterRoutes onto a real gin engine
// served by a real httptest.Server, returning the server for the
// caller to issue requests against.
func newDispatchServer(t *testing.T, g *SandboxProxyGroup) *httptest.Server {
	t.Helper()
	gin.SetMode(gin.TestMode)
	e := gin.New()
	rg := e.Group(g.BasePath())
	g.RegisterRoutes(rg)
	srv := httptest.NewServer(e)
	t.Cleanup(srv.Close)
	return srv
}

func TestSandboxProxyGroup_RegisterRoutes_Good(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/global/health" {
			_, _ = w.Write([]byte("ok"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(upstream.Close)

	g := NewSandboxProxyGroup()
	g.Set("oc-routes-1", upstream.URL, "")
	front := newDispatchServer(t, g)

	resp, err := http.Get(front.URL + "/v1/api/sandbox/oc-routes-1/global/health")
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d; want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "ok" {
		t.Errorf("body = %q; want ok", string(body))
	}
}

func TestSandboxProxyGroup_SetHasDelete_Good(t *testing.T) {
	g := NewSandboxProxyGroup()
	if g.Has("oc-1") {
		t.Fatalf("Has(oc-1) = true before Set")
	}
	g.Set("oc-1", "http://127.0.0.1:1", "")
	if !g.Has("oc-1") {
		t.Fatalf("Has(oc-1) = false after Set")
	}
	g.Delete("oc-1")
	if g.Has("oc-1") {
		t.Fatalf("Has(oc-1) = true after Delete")
	}
}

func TestSandboxProxyGroup_Set_InvalidURL_Bad(t *testing.T) {
	g := NewSandboxProxyGroup()
	// A URL containing a raw control byte fails url.Parse — Set must
	// silently no-op rather than panic or register a broken proxy.
	g.Set("oc-bad", "http://\x7f", "")
	if g.Has("oc-bad") {
		t.Fatalf("Has(oc-bad) = true; Set on an invalid URL must be a no-op")
	}
}

// TestSandboxProxyGroup_Dispatch_InjectsAuthHeader_Good — when Set is
// given a non-empty authHeader, the forwarded request must carry an
// Authorization header the upstream can see (proves the Director
// wrap-and-inject path, not just the default rewrite).
func TestSandboxProxyGroup_Dispatch_InjectsAuthHeader_Good(t *testing.T) {
	var gotAuth string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(upstream.Close)

	g := NewSandboxProxyGroup()
	g.Set("oc-auth-1", upstream.URL, "Basic dGVzdDp0ZXN0")
	front := newDispatchServer(t, g)

	resp, err := http.Get(front.URL + "/v1/api/sandbox/oc-auth-1/session")
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d; want 200", resp.StatusCode)
	}
	if gotAuth != "Basic dGVzdDp0ZXN0" {
		t.Errorf("upstream saw Authorization = %q; want injected Basic header", gotAuth)
	}
}

// TestSandboxProxyGroup_Dispatch_UnknownID_Bad — no target registered
// for the id → 404 with a helpful hint, not a panic / 500.
func TestSandboxProxyGroup_Dispatch_UnknownID_Bad(t *testing.T) {
	g := NewSandboxProxyGroup()
	front := newDispatchServer(t, g)

	resp, err := http.Get(front.URL + "/v1/api/sandbox/oc-ghost/global/health")
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d; want 404", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "sandbox not running") {
		t.Errorf("body = %s; want hint about sandbox not running", string(body))
	}
}
