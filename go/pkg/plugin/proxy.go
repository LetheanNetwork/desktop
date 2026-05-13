// SPDX-Licence-Identifier: EUPL-1.2

// Reverse-proxy mount — a single coreapi.RouteGroup registered
// once at boot. Internally it holds a code → targetURL table
// that mutates as plugins Start / Stop. coreapi.Engine has no
// Unregister API today (Phase 1 design note), so the indirection
// here keeps the host's lifecycle simple.

package plugin

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
)

// ProxyGroup implements coreapi.RouteGroup. Registered exactly
// once on the coreapi.Engine; the targets map mutates at runtime.
type ProxyGroup struct {
	mu      sync.RWMutex
	targets map[string]*httputil.ReverseProxy // keyed by plugin code
}

// NewProxyGroup constructs an empty proxy group.
func NewProxyGroup() *ProxyGroup {
	return &ProxyGroup{targets: map[string]*httputil.ReverseProxy{}}
}

// Name satisfies coreapi.RouteGroup. Surfaces in /v1/openapi.
func (g *ProxyGroup) Name() string { return "plugin" }

// BasePath satisfies coreapi.RouteGroup. All plugin routes mount
// under /v1/api/plugin/.
func (g *ProxyGroup) BasePath() string { return "/v1/api/plugin" }

// RegisterRoutes satisfies coreapi.RouteGroup. The wildcard
// pattern captures `:code/*proxyPath` so the dispatcher can
// look the target up and forward.
//
// Path stripping is "preserve" per the Phase 1 decision: the
// plugin sees /<namespace>/<...> on the way in (its own routing
// uses --namespace as the mount prefix), so the host strips only
// /v1/api/plugin/ from the incoming path before forwarding.
func (g *ProxyGroup) RegisterRoutes(rg *gin.RouterGroup) {
	rg.Any("/:code/*proxyPath", g.dispatch)
}

// Set installs a forwarding target for one plugin code. Called
// from Start once the plugin is healthy.
func (g *ProxyGroup) Set(code, targetURL string) {
	u, err := url.Parse(targetURL)
	if err != nil {
		return
	}
	rp := httputil.NewSingleHostReverseProxy(u)
	// Customise the Director so we get path preservation right.
	// httputil's default appends Request.URL.Path to the target's
	// path; we want the request to land at <target>/<namespace>/...
	// where the gin wildcard captured "*proxyPath" as <namespace>/...
	// (because the gin route is /:code/*proxyPath; Phase 1 spec
	// says code == namespace by default so the namespace prefix is
	// effectively preserved).
	originalDirector := rp.Director
	rp.Director = func(req *http.Request) {
		originalDirector(req)
		// Inject X-Lthn-User for plugins that want to keyspace state.
		req.Header.Set("X-Lthn-User", "local")
	}
	g.mu.Lock()
	g.targets[code] = rp
	g.mu.Unlock()
}

// Delete drops a plugin's forwarding entry. Subsequent requests
// to /v1/api/plugin/<code>/* return 404.
func (g *ProxyGroup) Delete(code string) {
	g.mu.Lock()
	delete(g.targets, code)
	g.mu.Unlock()
}

// Has reports whether a plugin is currently mounted.
func (g *ProxyGroup) Has(code string) bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	_, ok := g.targets[code]
	return ok
}

// dispatch looks the target up by URL param and forwards. The
// path passed to the proxy is the part after /v1/api/plugin/ so
// the plugin receives e.g. /coreagent/api/chat — namespace
// preserved.
func (g *ProxyGroup) dispatch(c *gin.Context) {
	code := strings.Trim(c.Param("code"), "/ ")
	g.mu.RLock()
	rp, ok := g.targets[code]
	g.mu.RUnlock()
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "plugin not running: " + code,
			"hint":  "install + start via the marketplace surface",
		})
		return
	}
	// Rewrite the URL so the proxy forwards <code>/<proxyPath>
	// rather than the full /v1/api/plugin/<code>/<proxyPath>.
	// gin's "*proxyPath" includes the leading slash.
	proxyPath := c.Param("proxyPath")
	c.Request.URL.Path = "/" + code + proxyPath
	rp.ServeHTTP(c.Writer, c.Request)
}
