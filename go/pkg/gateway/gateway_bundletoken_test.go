// SPDX-Licence-Identifier: EUPL-1.2

// Cerberus Mantis #1443 — per-bundle token binding tests.
//
// The threat: Plugin A holds the host's global bearer token and knows
// Bundle B's name. Before the fix it could set Bundle-ID: B in the
// header and exercise B's declared permissions. After the fix, when a
// BundleCodeResolver is wired, the gateway derives the authoritative
// bundle code from X-Bundle-Token (verified) and ignores Bundle-ID.
//
// Test matrix:
//   - Plugin calls with a valid X-Bundle-Token → code resolved, request
//     permitted against that bundle's own permissions.
//   - Plugin A tries to claim Plugin B's identity via Bundle-ID: B while
//     carrying A's bundle token → gateway uses A's code (from token),
//     A's permissions are checked — not B's. If A lacks the scope, 403.
//   - Unknown / expired X-Bundle-Token → 401 invalid-bundle-token.
//   - No resolver wired (backward compat) → falls back to Bundle-ID header.

package gateway_test

import (
	"net/http/httptest"

	core "dappco.re/go"
	"github.com/gin-gonic/gin"

	"dappco.re/lthn/desktop/pkg/gateway"
	"dappco.re/lthn/desktop/pkg/marketplace"
)

// staticResolver is a test-only BundleCodeResolver backed by a fixed map.
// Mirrors pkg/plugin.Service.LookupBundleCode without importing that package.
type staticResolver struct {
	m map[string]string // secret → code
}

func (r *staticResolver) LookupBundleCode(secret string) (string, bool) {
	if secret == "" {
		return "", false
	}
	code, ok := r.m[secret]
	return code, ok
}

// newTokenRouter wires a gateway.Service with the supplied resolver and a
// single "test.token" scope so tests can drive Handle end-to-end.
func newTokenRouter(t *core.T, c *core.Core, resolver gateway.BundleCodeResolver) (*gateway.Service, *gin.Engine) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	svc := gateway.NewService(c)
	if resolver != nil {
		gateway.SetBundleCodeResolver(svc, resolver)
	}
	// Register a scope that echoes the bundle code it received, so tests
	// can confirm which identity the gateway chose.
	echoBundleID := func(_ *core.Core, bundleID string, _ *gin.Context) core.Result {
		return core.Ok(map[string]any{"bundle": bundleID})
	}
	core.RequireTrue(t, gateway.RegisterScope(svc, "test.token", "read", echoBundleID).OK)

	eng := gin.New()
	eng.POST("/v1/api/gateway/:scope/:mode", svc.Handle)
	return svc, eng
}

// ─── Good: valid X-Bundle-Token ────────────────────────────────────────

func TestGateway_BundleToken_Good_TokenResolvesToOwnCode(t *core.T) {
	c := newGatewayCore(t)
	// Plugin A is installed with the test.token scope.
	seedBundle(t, c, "plugin-a", []marketplace.Permission{
		{Scope: "test.token", Mode: "read"},
	})
	resolver := &staticResolver{m: map[string]string{"secret-a": "plugin-a"}}
	_, eng := newTokenRouter(t, c, resolver)

	req := httptest.NewRequest("POST", "/v1/api/gateway/test.token/read", nil)
	req.Header.Set("X-Bundle-Token", "secret-a")
	// Bundle-ID is also set — when token is resolved, header is ignored.
	req.Header.Set("Bundle-ID", "plugin-a")
	w := httptest.NewRecorder()
	eng.ServeHTTP(w, req)

	core.AssertEqual(t, core.StatusOK, w.Code)
	core.AssertContains(t, w.Body.String(), "plugin-a")
}

// ─── Bad: cross-plugin identity spoof (the Cerberus #1443 attack) ──────

func TestGateway_BundleToken_Bad_CrossPluginSpoof(t *core.T) {
	c := newGatewayCore(t)
	// Plugin A has NO test.token permission.
	seedBundle(t, c, "plugin-a", []marketplace.Permission{
		{Scope: "service.notify", Mode: "invoke"},
	})
	// Plugin B HAS test.token permission.
	seedBundle(t, c, "plugin-b", []marketplace.Permission{
		{Scope: "test.token", Mode: "read"},
	})
	resolver := &staticResolver{m: map[string]string{
		"secret-a": "plugin-a",
		"secret-b": "plugin-b",
	}}
	_, eng := newTokenRouter(t, c, resolver)

	// Plugin A carries its own bundle token but asserts Bundle-ID: plugin-b.
	// Pre-fix: gateway would use "plugin-b", find its permission, and allow.
	// Post-fix: gateway resolves secret-a → plugin-a; plugin-a lacks
	// test.token:read → 403 permission-denied.
	req := httptest.NewRequest("POST", "/v1/api/gateway/test.token/read", nil)
	req.Header.Set("X-Bundle-Token", "secret-a")
	req.Header.Set("Bundle-ID", "plugin-b") // the spoof attempt
	w := httptest.NewRecorder()
	eng.ServeHTTP(w, req)

	core.AssertEqual(t, core.StatusForbidden, w.Code,
		"plugin A must not gain plugin B's permissions by forging Bundle-ID: plugin-b")
	core.AssertContains(t, w.Body.String(), "permission-denied")
}

func TestGateway_BundleToken_Bad_UnknownToken(t *core.T) {
	c := newGatewayCore(t)
	seedBundle(t, c, "plugin-a", []marketplace.Permission{
		{Scope: "test.token", Mode: "read"},
	})
	resolver := &staticResolver{m: map[string]string{"secret-a": "plugin-a"}}
	_, eng := newTokenRouter(t, c, resolver)

	// Present a token that is not in the registry (e.g. plugin was stopped).
	req := httptest.NewRequest("POST", "/v1/api/gateway/test.token/read", nil)
	req.Header.Set("X-Bundle-Token", "stale-or-forged-token")
	w := httptest.NewRecorder()
	eng.ServeHTTP(w, req)

	core.AssertEqual(t, core.StatusUnauthorized, w.Code,
		"an unknown token must be rejected before permission check")
	core.AssertContains(t, w.Body.String(), "invalid-bundle-token")
}

// ─── Ugly: resolver absent → backward-compat fallback ──────────────────

func TestGateway_BundleToken_Ugly_NoResolverFallsBackToHeader(t *core.T) {
	c := newGatewayCore(t)
	seedBundle(t, c, "opencode", []marketplace.Permission{
		{Scope: "test.token", Mode: "read"},
	})
	// No resolver wired — simulates the WebView / integration-test caller
	// that authenticates with the host's global bearer and uses Bundle-ID.
	_, eng := newTokenRouter(t, c, nil)

	req := httptest.NewRequest("POST", "/v1/api/gateway/test.token/read", nil)
	req.Header.Set("Bundle-ID", "opencode")
	// No X-Bundle-Token — header claim should be accepted.
	w := httptest.NewRecorder()
	eng.ServeHTTP(w, req)

	core.AssertEqual(t, core.StatusOK, w.Code,
		"no resolver: Bundle-ID header is the authoritative source (backward compat)")
	core.AssertContains(t, w.Body.String(), "opencode")
}

func TestGateway_BundleToken_Ugly_ResolverPresentNoTokenFallsBackToHeader(t *core.T) {
	c := newGatewayCore(t)
	seedBundle(t, c, "opencode", []marketplace.Permission{
		{Scope: "test.token", Mode: "read"},
	})
	resolver := &staticResolver{m: map[string]string{"some-secret": "other-plugin"}}
	_, eng := newTokenRouter(t, c, resolver)

	// Resolver is wired but the caller sends no X-Bundle-Token (e.g. a
	// direct curl from a developer). Falls back to Bundle-ID header.
	req := httptest.NewRequest("POST", "/v1/api/gateway/test.token/read", nil)
	req.Header.Set("Bundle-ID", "opencode")
	w := httptest.NewRecorder()
	eng.ServeHTTP(w, req)

	core.AssertEqual(t, core.StatusOK, w.Code,
		"resolver wired but no token header: Bundle-ID fallback still works")
	core.AssertContains(t, w.Body.String(), "opencode")
}

// ─── Regression: token header is stripped before scope handler sees it ─

func TestGateway_BundleToken_Good_TokenStrippedFromForwardedRequest(t *core.T) {
	c := newGatewayCore(t)
	seedBundle(t, c, "plugin-a", []marketplace.Permission{
		{Scope: "test.token", Mode: "read"},
	})
	resolver := &staticResolver{m: map[string]string{"secret-a": "plugin-a"}}

	// Register a scope that checks whether X-Bundle-Token is still
	// present in the forwarded request headers.
	tokenLeakDetect := func(_ *core.Core, _ string, ctx *gin.Context) core.Result {
		leaked := ctx.GetHeader("X-Bundle-Token")
		return core.Ok(map[string]any{"leaked": leaked})
	}

	svc := gateway.NewService(c)
	gateway.SetBundleCodeResolver(svc, resolver)
	core.RequireTrue(t, gateway.RegisterScope(svc, "test.token", "read", tokenLeakDetect).OK)
	gin.SetMode(gin.TestMode)
	eng := gin.New()
	eng.POST("/v1/api/gateway/:scope/:mode", svc.Handle)

	req := httptest.NewRequest("POST", "/v1/api/gateway/test.token/read", nil)
	req.Header.Set("X-Bundle-Token", "secret-a")
	w := httptest.NewRecorder()
	eng.ServeHTTP(w, req)

	core.AssertEqual(t, core.StatusOK, w.Code)
	// The raw secret must not leak into the scope handler.
	core.AssertContains(t, w.Body.String(), `"leaked":""`)
}
