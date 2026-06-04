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
	"net/http"
	"net/http/httputil"
	"net/url"

	core "dappco.re/go"
	"github.com/gin-gonic/gin"
)

// pluginProxyCSPHeader is the restrictive Content-Security-Policy
// the loopback reverse-proxy STRIPS from the plugin's own response
// + INJECTS as the authoritative policy per
// plans/code/lthn/desktop/views/RFC.plugin-views.md §3.4 (HIGH-1)
// and Cerberus round-2 LOW-R2-1 floor extension (Mantis #1712).
//
// Defence shape:
//   - default-src 'self'             — same-origin baseline
//   - script-src 'self'              — explicit; never rely on default
//   - style-src 'self' 'unsafe-inline' — opencode (today's first tier-2
//     consumer) ships inline style; drop 'unsafe-inline' if a future
//     plugin can confirm it doesn't need it
//   - connect-src 'self' wails://wails — outbound fetches restricted
//     to the plugin's own loopback origin + the host shell origin
//     (postMessage / parent-window navigation). Forbids exfiltration
//     of a granted session token to any external endpoint.
//   - frame-ancestors wails://wails  — only the host shell may frame
//     the plugin
//   - object-src 'none'              — block <object>/<embed> plugins
//   - base-uri 'self'                — prevent <base> href injection
//   - form-action 'self'             — restrict form submissions to
//     the plugin's own origin
//
// The plugin author's own CSP header is dropped before injection so
// a permissive policy from the plugin process can't widen the
// effective policy the WebView observes.
const pluginProxyCSPHeader = "default-src 'self'; " +
	"script-src 'self'; " +
	"style-src 'self' 'unsafe-inline'; " +
	"connect-src 'self' wails://wails; " +
	"frame-ancestors wails://wails; " +
	"object-src 'none'; " +
	"base-uri 'self'; " +
	"form-action 'self'"

// PluginProxyCSPHeader returns the policy value the reverse-proxy
// injects on every response from a plugin's loopback origin.
// Exported for tests + audit log + spec-grep parity. The literal
// is the contract; renaming this const without a spec bump breaks
// the Cerberus DREAD §3.4 + LOW-R2-1 audit trail.
func PluginProxyCSPHeader() string { return pluginProxyCSPHeader }

// stripAndInjectCSP is the http.ReverseProxy.ModifyResponse hook
// per §3.4. Drops ANY CSP header the plugin process set and writes
// the restrictive policy in its place. Fires on every response —
// 2xx, 3xx, 4xx, 5xx — so an error response from the plugin webapp
// cannot smuggle a permissive policy past the gate (Cerberus round-2
// LOW-R2-1 error-response coverage).
//
// Returns nil error always — the rewrite is unconditional. Callers
// wire it as the proxy's ModifyResponse field.
//
// Usage example:
//
//	rp := httputil.NewSingleHostReverseProxy(u)
//	rp.ModifyResponse = stripAndInjectCSP
func stripAndInjectCSP(resp *http.Response) error {
	if resp == nil || resp.Header == nil {
		return nil
	}
	// Drop both header-name spellings — RFC 7231 says headers are
	// case-insensitive but some upstreams send "content-security-policy"
	// while http.Header normalises on canonical case. Del covers
	// canonical; the variant-spellings are dropped via raw iteration
	// over the underlying map.
	resp.Header.Del("Content-Security-Policy")
	resp.Header.Del("Content-Security-Policy-Report-Only")
	for k := range resp.Header {
		if core.Lower(k) == "content-security-policy" ||
			core.Lower(k) == "content-security-policy-report-only" {
			resp.Header.Del(k)
		}
	}
	resp.Header.Set("Content-Security-Policy", pluginProxyCSPHeader)
	// The injected CSP's `frame-ancestors wails://wails` is the
	// authoritative framing policy. Drop any upstream X-Frame-Options so a
	// plugin/bundle webapp that sends "DENY" (e.g. Odysseus's
	// SecurityHeadersMiddleware) can't override it and block the host shell
	// from framing the proxied view. The CSP spec says frame-ancestors
	// supersedes X-Frame-Options, but removing it explicitly avoids any
	// WebView ambiguity. Both canonical + variant spellings, every response
	// code — same belt-and-suspenders shape as the CSP strip above.
	resp.Header.Del("X-Frame-Options")
	for k := range resp.Header {
		if core.Lower(k) == "x-frame-options" {
			resp.Header.Del(k)
		}
	}
	return nil
}

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
	// CSP strip-and-inject per RFC.plugin-views §3.4 (Cerberus HIGH-1
	// + round-2 LOW-R2-1). The plugin's own CSP is dropped before
	// injection so a permissive plugin policy cannot widen the
	// effective WebView posture. Fires on every response code so
	// 4xx/5xx responses cannot slip a permissive policy past.
	rp.ModifyResponse = stripAndInjectCSP
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
