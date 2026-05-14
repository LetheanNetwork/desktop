// SPDX-Licence-Identifier: EUPL-1.2

// Reverse-proxy mount — a single coreapi.RouteGroup registered
// once at boot. Internally it holds a sandbox-id → ReverseProxy
// table that mutates as opencode sandboxes Start / Stop. Mirrors
// pkg/plugin's ProxyGroup shape; differs in path semantics — we
// strip the /v1/api/sandbox/<id>/ prefix entirely so the upstream
// (opencode-serve) sees clean paths like /global/health, /session.

package opencode

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"

	core "dappco.re/go"
	"github.com/gin-gonic/gin"
)

// SandboxProxyGroup implements coreapi.RouteGroup. Registered exactly
// once on the coreapi.Engine; the targets map mutates at runtime as
// opencode sandboxes Start / Stop.
type SandboxProxyGroup struct {
	mu      sync.RWMutex
	targets map[string]*httputil.ReverseProxy // keyed by sandbox id
}

// NewSandboxProxyGroup constructs an empty proxy group.
//
// Usage example:
//
//	g := opencode.NewSandboxProxyGroup()
//	engine.Register(g)              // mount /v1/api/sandbox/* once at boot
//	g.Set("oc-7f3a2b1c", "http://127.0.0.1:51823")
func NewSandboxProxyGroup() *SandboxProxyGroup {
	return &SandboxProxyGroup{targets: map[string]*httputil.ReverseProxy{}}
}

// Name satisfies coreapi.RouteGroup. Surfaces in /v1/openapi.
func (g *SandboxProxyGroup) Name() string { return "sandbox" }

// BasePath satisfies coreapi.RouteGroup. All sandbox routes mount
// under /v1/api/sandbox/.
func (g *SandboxProxyGroup) BasePath() string { return "/v1/api/sandbox" }

// RegisterRoutes satisfies coreapi.RouteGroup. The wildcard pattern
// captures `:id/*proxyPath` so the dispatcher can look the target
// up and forward.
//
// Path semantics differ from pkg/plugin: opencode-serve is content
// with clean paths, so we strip /v1/api/sandbox/<id>/ entirely
// before forwarding. The container sees /global/health, /session,
// /provider — never the sandbox-id namespace.
func (g *SandboxProxyGroup) RegisterRoutes(rg *gin.RouterGroup) {
	rg.Any("/:id/*proxyPath", g.dispatch)
}

// Set installs a forwarding target for one sandbox id. Called from
// Service.Start() once the container is healthy. targetURL is
// `http://127.0.0.1:<host-port>` where host-port is the dynamic
// port allocated for this sandbox.
//
// Usage example:
//
//	g.Set("oc-7f3a2b1c", "http://127.0.0.1:51823")
func (g *SandboxProxyGroup) Set(id, targetURL string) {
	u, err := url.Parse(targetURL)
	if err != nil {
		return
	}
	rp := httputil.NewSingleHostReverseProxy(u)
	// SSE-friendly: httputil.ReverseProxy's default ServeHTTP
	// flushes streaming responses (no Buffered field — flush happens
	// when downstream Writer implements http.Flusher, which gin's
	// ResponseWriter does). No customisation needed for SSE today.
	g.mu.Lock()
	g.targets[id] = rp
	g.mu.Unlock()
}

// Delete drops a sandbox's forwarding entry. Subsequent requests
// to /v1/api/sandbox/<id>/* return 404 with a helpful hint.
//
// Usage example:
//
//	g.Delete("oc-7f3a2b1c")
func (g *SandboxProxyGroup) Delete(id string) {
	g.mu.Lock()
	delete(g.targets, id)
	g.mu.Unlock()
}

// Has reports whether a sandbox is currently mounted.
//
// Usage example:
//
//	if g.Has("oc-7f3a2b1c") { ... }
func (g *SandboxProxyGroup) Has(id string) bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	_, ok := g.targets[id]
	return ok
}

// dispatch looks the target up by URL param and forwards. The path
// passed to the proxy is *proxyPath (the part after /v1/api/sandbox/<id>),
// so the upstream container sees /global/health, /session/<id>, etc.
func (g *SandboxProxyGroup) dispatch(c *gin.Context) {
	id := core.TrimCutset(c.Param("id"), "/ ")
	g.mu.RLock()
	rp, ok := g.targets[id]
	g.mu.RUnlock()
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "sandbox not running: " + id,
			"hint":  "start a sandbox via `lthn opencode start` or the Integrations panel",
		})
		return
	}
	// gin's "*proxyPath" wildcard includes the leading slash, e.g.
	// "/global/health". Rewriting Request.URL.Path strips the
	// /v1/api/sandbox/<id> prefix entirely.
	c.Request.URL.Path = c.Param("proxyPath")
	rp.ServeHTTP(c.Writer, c.Request)
}
