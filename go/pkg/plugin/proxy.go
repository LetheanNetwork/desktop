// SPDX-Licence-Identifier: EUPL-1.2

// Reverse-proxy mount — a single coreapi.RouteGroup registered
// once at boot. Internally it holds a code → targetURL table
// that mutates as plugins Start / Stop. coreapi.Engine has no
// Unregister API today (Phase 1 design note), so the indirection
// here keeps the host's lifecycle simple.
//
// Auth: routes land inside the engine-global bearer-auth middleware
// (coreapi.WithBearerAuth, applied in pkg/server via Mantis #1430).
// Unauthenticated requests are rejected at the middleware layer before
// dispatch() is reached.
//
// X-Lthn-User: the forwarded header carries a stable per-session
// fingerprint derived from the Authorization header (SHA-256, first
// 16 hex chars). Plugins use this to keyspace state per caller without
// the host leaking the raw bearer credential upstream.

package plugin

import (
	"net/http/httputil"
	"net/url"

	core "dappco.re/go"
	"github.com/gin-gonic/gin"
)

// ProxyGroup implements coreapi.RouteGroup. Registered exactly
// once on the coreapi.Engine; the targets map mutates at runtime.
type ProxyGroup struct {
	mu      core.RWMutex
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
	// httputil's default Director rewrites the target host while
	// preserving the request path. Path rewriting (strip /v1/api/plugin/
	// prefix, prepend code namespace) happens in dispatch() at call
	// time so the Director stays unmodified.
	rp := httputil.NewSingleHostReverseProxy(u)
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
	code := core.TrimCutset(c.Param("code"), "/ ")
	g.mu.RLock()
	rp, ok := g.targets[code]
	g.mu.RUnlock()
	if !ok {
		c.JSON(core.StatusNotFound, gin.H{
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

	// Inject X-Lthn-User so plugins can keyspace per-caller state.
	// The bearer gate (coreapi.WithBearerAuth) already validated the
	// Authorization header before this handler runs, so the header is
	// guaranteed non-empty for authenticated requests. We forward a
	// stable fingerprint — SHA-256(Authorization)[0:16] — so plugins
	// get a stable, opaque, per-session identity without the host
	// leaking the raw bearer credential upstream.
	// Cerberus Mantis #1437.
	authHeader := c.GetHeader("Authorization")
	userID := core.SHA256HexString(authHeader)[:16]
	c.Request.Header.Set("X-Lthn-User", userID)

	rp.ServeHTTP(c.Writer, c.Request)
}
