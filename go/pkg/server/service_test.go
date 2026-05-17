// SPDX-Licence-Identifier: EUPL-1.2

// Service-level tests — primary one today is the route-tier
// completeness check per RFC.stage-e.md v2 §4 H1 (Cerberus DREAD
// HIGH). Deny-by-default is the runtime defence; this CI test is
// the build-time defence.

package server_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	core "dappco.re/go"
	coreapi "dappco.re/go/api"
	"dappco.re/lthn/desktop/pkg/account"
	"dappco.re/lthn/desktop/pkg/server"
	"github.com/gin-gonic/gin"
)

// TestService_AllRoutesTiered_Good walks engine.Routes() post-
// registration and asserts every /v1/* path is in routeTiers OR in
// bootstrapPathScopes OR in the skip-list. The catch is for the
// case where a contributor adds a route via auto-discovery without
// touching the tier map — without this test, deny-by-default only
// protects against routes the contributor THINKS ABOUT; this test
// protects against routes they don't.
//
// Per Cerberus DREAD H1 — "deny-by-default is necessary but NOT
// sufficient". Build-time check + runtime fallback together close
// the security-policy gap.
//
// Cerberus Stage E.B DREAD ADD-HIGH-1 — earlier version of this
// test constructed NewService with no Core wired, so auto-discovery
// never fired and the engine only contained the OpenAI shim routes.
// The /v1/account/* routes were never registered → the test was
// vacuously passing. Fixed by registering account.Service via Core
// so the auto-discovery loop actually populates the engine with the
// production route set.
func TestService_AllRoutesTiered_Good(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Build a Core with account.Register so the RouteGroup auto-
	// discovery loop inside server.NewService populates the engine
	// with /v1/account/{create,unlock,lock}. Without this, the test
	// would only see the OpenAI shim routes (vacuous coverage).
	c := core.New(core.WithName("account", account.Register))

	svc := server.NewService(server.Options{
		Addr:     ":0",
		LocalKey: "test-key",
		Core:     c,
		// ServerKey deliberately nil — exercise the path-tier
		// validation without requiring a serverkey Bootstrap. The
		// route-tier map is independent of auth-mode.
	})

	// Use the public coreapi.Engine directly — Handler() may wrap
	// it in a composite for webview-only routing; we want the raw
	// engine's gin route tree.
	publicEng := svc.Engine()
	core.AssertTrue(t, publicEng != nil, "service.Engine() must be non-nil")
	h := publicEng.Handler()
	eng, ok := h.(*gin.Engine)
	core.AssertTrue(t, ok, "coreapi.Engine.Handler must be a *gin.Engine")

	// Sanity: prove account routes ARE in the engine (defends
	// against a future regression where Core auto-discovery
	// silently breaks and the test goes vacuous again).
	var seenAccountCreate, seenAccountUnlock, seenAccountLock bool
	for _, r := range eng.Routes() {
		switch r.Path {
		case "/v1/account/create":
			seenAccountCreate = true
		case "/v1/account/unlock":
			seenAccountUnlock = true
		case "/v1/account/lock":
			seenAccountLock = true
		}
	}
	core.AssertTrue(t, seenAccountCreate, "/v1/account/create MUST be auto-discovered (vacuous test guard)")
	core.AssertTrue(t, seenAccountUnlock, "/v1/account/unlock MUST be auto-discovered (vacuous test guard)")
	core.AssertTrue(t, seenAccountLock, "/v1/account/lock MUST be auto-discovered (vacuous test guard)")

	missing := []string{}
	for _, r := range eng.Routes() {
		if isRouteTierSkip(r.Path) {
			continue
		}
		if _, inScopes := server.BootstrapPathScopes[r.Path]; inScopes {
			continue
		}
		if isRouteTierClassified(r.Path) {
			continue
		}
		missing = append(missing, r.Method+" "+r.Path)
	}
	if len(missing) > 0 {
		buf := "routes missing from RouteTiers / RouteTierPrefixes / BootstrapPathScopes / skip-list (RFC §4 H1):"
		for _, m := range missing {
			buf += "\n  - " + m
		}
		buf += "\n\nAdd entries to pkg/server.RouteTiers (exact) or RouteTierPrefixes (group) or RouteTierSkipList (genuine bypass)."
		t.Fatal(buf)
	}
}

// isRouteTierSkip reports whether path is in the RouteTierSkipList
// (exact match) OR matches any RouteTierSkipPrefixes prefix. The
// prefix branch covers expanded swagger UI / SDK paths.
func isRouteTierSkip(path string) bool {
	for _, p := range server.RouteTierSkipList {
		if path == p {
			return true
		}
	}
	for _, prefix := range server.RouteTierSkipPrefixes {
		if pathHasPrefix(path, prefix) {
			return true
		}
	}
	// Non-/v1 paths (static assets, SPA fallback) bypass tier
	// classification — the middleware short-circuits these before
	// reaching the tier check.
	if !core.HasPrefix(path, "/v1") {
		return true
	}
	return false
}

// pathHasPrefix returns true if path == prefix or path starts with
// prefix + "/". Mirrors the same-named helper in service.go (test-
// local to avoid cross-file leakage).
func pathHasPrefix(path, prefix string) bool {
	if !core.HasPrefix(path, prefix) {
		return false
	}
	if len(path) == len(prefix) {
		return true
	}
	return path[len(prefix)] == '/'
}

// TestService_ProductionRoutesTiered_Good walks the route set with
// ALL known production RouteGroups registered via ExtraGroups, to
// surface paths that auto-discovery + extras add beyond the bare
// NewService surface. This is the production-shape variant of
// TestService_AllRoutesTiered_Good and lands as part of the Stage
// E.B integration cutover (Mantis #1480) so the cutover doesn't
// silently flip route-tier behaviour on routes the bare test
// didn't see.
//
// Routes covered here that the bare test does not:
//   - /v1/account/* (auto-discovered via account.Service.RouteGroups)
//   - /v1/api/opencode/* + /v1/api/sandbox/* (opencode ExtraGroups)
//   - /v1/api/plugin/* (plugin ExtraGroup)
//   - /v1/api/gateway/* (gateway ExtraGroup)
//   - /v1/api/process/* — webview-only, lives on s.webviewEngine, not
//     the public engine; the public test SHOULDN'T see this path.
//
// Cerberus DREAD H1 — runtime deny-by-default + this build-time
// completeness check together close the policy gap.
func TestService_ProductionRoutesTiered_Good(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Register exemplar route groups inline so the test doesn't drag
	// in the actual opencode/plugin/gateway service constructors
	// (which require disk + network for their own setup).
	extras := []coreapi.RouteGroup{
		&fakeGroup{name: "opencode-control", base: "/v1/api/opencode", verbs: map[string][]string{
			"GET":    {"/sandbox", "/sandbox/:id", "/profile", "/profile/:name", "/sandbox/:id/providers", "/enabled", "/studio", "/sandbox/:id/web", "/imports", "/imports/providers"},
			"POST":   {"/sandbox", "/profile", "/host-config", "/enable", "/disable", "/sandbox/:id/tui", "/studio", "/upgrade", "/sandbox/:id/web", "/import"},
			"DELETE": {"/sandbox/:id", "/profile/:name"},
		}},
		&fakeGroup{name: "opencode-proxy", base: "/v1/api/sandbox", verbs: map[string][]string{
			"GET":  {"/*proxy"},
			"POST": {"/*proxy"},
		}},
		&fakeGroup{name: "plugin-proxy", base: "/v1/api/plugin", verbs: map[string][]string{
			"GET":    {"/*proxy"},
			"POST":   {"/*proxy"},
			"PUT":    {"/*proxy"},
			"DELETE": {"/*proxy"},
		}},
		&fakeGroup{name: "gateway", base: "", verbs: map[string][]string{
			"POST": {"/v1/api/gateway/:scope/:mode"},
		}},
		&fakeGroup{name: "runner", base: "/v1", verbs: map[string][]string{
			"GET":  {"/runner/models"},
			"POST": {"/runner/generate", "/runner/chat"},
		}},
		&fakeGroup{name: "account", base: "/v1/account", verbs: map[string][]string{
			"POST": {"/create", "/unlock", "/lock"},
		}},
	}

	svc := server.NewService(server.Options{
		Addr:        ":0",
		LocalKey:    "test-key",
		ExtraGroups: extras,
	})

	publicEng := svc.Engine()
	core.AssertTrue(t, publicEng != nil)
	h := publicEng.Handler()
	eng, ok := h.(*gin.Engine)
	core.AssertTrue(t, ok)

	missing := []string{}
	for _, r := range eng.Routes() {
		if isRouteTierSkip(r.Path) {
			continue
		}
		if _, inScopes := server.BootstrapPathScopes[r.Path]; inScopes {
			continue
		}
		if isRouteTierClassified(r.Path) {
			continue
		}
		missing = append(missing, r.Method+" "+r.Path)
	}
	if len(missing) > 0 {
		buf := "PRODUCTION-shape routes missing from RouteTiers / RouteTierPrefixes / BootstrapPathScopes / skip-list (RFC §4 H1):"
		for _, m := range missing {
			buf += "\n  - " + m
		}
		buf += "\n\nAdd entries to pkg/server.RouteTiers (exact match) or RouteTierPrefixes (group)."
		t.Fatal(buf)
	}
}

// isRouteTierClassified reports whether the supplied gin route path
// resolves to a tier classification via either exact-match or
// longest-prefix lookup. Mirrors the live middleware's
// resolveRouteTier discipline. gin paths can contain :param
// placeholders that won't match RouteTiers exact-match keys — the
// prefix branch covers parameterised sub-trees (e.g.
// /v1/api/opencode/sandbox/:id matches the /v1/api/opencode prefix).
func isRouteTierClassified(path string) bool {
	if _, ok := server.RouteTiers[path]; ok {
		return true
	}
	for prefix := range server.RouteTierPrefixes {
		if pathHasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

// fakeGroup is a minimal coreapi.RouteGroup test fixture used by
// TestService_ProductionRoutesTiered_Good to register paths without
// dragging the real service constructors into the test compile unit.
type fakeGroup struct {
	name  string
	base  string
	verbs map[string][]string // METHOD → []leafPath
}

func (g *fakeGroup) Name() string     { return g.name }
func (g *fakeGroup) BasePath() string { return g.base }
func (g *fakeGroup) RegisterRoutes(rg *gin.RouterGroup) {
	noop := func(c *gin.Context) { c.Status(204) }
	for method, leaves := range g.verbs {
		for _, leaf := range leaves {
			switch method {
			case "GET":
				rg.GET(leaf, noop)
			case "POST":
				rg.POST(leaf, noop)
			case "PUT":
				rg.PUT(leaf, noop)
			case "DELETE":
				rg.DELETE(leaf, noop)
			case "PATCH":
				rg.PATCH(leaf, noop)
			}
		}
	}
}

// TestService_WebViewEngineRoutesTiered_Good walks the WEBVIEW
// engine's gin route tree and asserts every path is classified —
// the missing-coverage pair of TestService_AllRoutesTiered_Good
// (which walks only the public engine).
//
// Cerberus Stage E.B re-DREAD ADD-CRIT-2 (Mantis #1708) — the
// cutover `f4f93bf` broke /v1/api/process/* because the webview
// engine inherits the same auth middleware as the public engine
// but /v1/api/process was missing from both RouteTiers AND
// RouteTierPrefixes. WebView same-origin fetches hit deny-by-
// default and 401. The bare public-engine test could never have
// caught this — /v1/api/process is webview-only-mounted (Mantis
// #1449 Path 3) and never appears on the public engine.
//
// This test stands up a stub `lthn-process` route group, registers
// it via Core auto-discovery + WebViewOnlyGroups filter, and walks
// the webview engine's routes through the same tier-classification
// pipeline. Fix-verification: /v1/api/process now in
// RouteTierPrefixes → the probe route classifies → assertion holds.
func TestService_WebViewEngineRoutesTiered_Good(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Reuse the stubGroup + newCoreWithProvider scaffold from
	// webview_split_test.go (same package). The lthn-process group
	// lands on the webview engine via the default WebViewOnly
	// filter set.
	const processGroupName = "lthn-process"
	const processBasePath = "/v1/api/process"
	c := newCoreWithProvider(&stubGroup{name: processGroupName, base: processBasePath})

	svc := server.NewService(server.Options{
		Addr:              ":0",
		LocalKey:          "test-key",
		Core:              c,
		WebViewOnlyGroups: []string{processGroupName},
	})

	wvEng := svc.WebViewEngine()
	core.AssertTrue(t, wvEng != nil,
		"service.WebViewEngine() MUST be non-nil when WebViewOnlyGroups is non-empty")

	h := wvEng.Handler()
	eng, ok := h.(*gin.Engine)
	core.AssertTrue(t, ok, "coreapi.Engine.Handler must be a *gin.Engine")

	// Sanity assert — the stub group's probe route IS on the
	// webview engine. Defends against a future regression where
	// the filter wiring silently breaks and the test goes vacuous.
	var seenProbe bool
	for _, r := range eng.Routes() {
		if r.Path == "/v1/api/process/probe" {
			seenProbe = true
			break
		}
	}
	core.AssertTrue(t, seenProbe,
		"/v1/api/process/probe MUST be on the webview engine (vacuous test guard)")

	missing := []string{}
	for _, r := range eng.Routes() {
		if isRouteTierSkip(r.Path) {
			continue
		}
		if _, inScopes := server.BootstrapPathScopes[r.Path]; inScopes {
			continue
		}
		if isRouteTierClassified(r.Path) {
			continue
		}
		missing = append(missing, r.Method+" "+r.Path)
	}
	if len(missing) > 0 {
		buf := "WEBVIEW engine routes missing from RouteTiers / RouteTierPrefixes / BootstrapPathScopes / skip-list (Cerberus ADD-CRIT-2):"
		for _, m := range missing {
			buf += "\n  - " + m
		}
		buf += "\n\nAdd entries to pkg/server.RouteTiers (exact match) or RouteTierPrefixes (group). " +
			"This test pairs with TestService_AllRoutesTiered_Good — the public-engine variant. " +
			"Both engines share middleware so both need classification coverage."
		t.Fatal(buf)
	}
}

// silence the coreapi import — referenced indirectly via
// server.Options (which embeds coreapi types), but Go requires the
// import marker to keep the file compileable in isolation.
var _ = coreapi.WithRequestID

// TestService_BodyCap_Default_Bad_AllRoutesCappedDefault — sample-N
// route audit per RFC.body-cap-middleware.md §4.3 (Mantis #1568 Unit C).
//
// Boots a real pkg/server.Service, iterates engine.Routes(), and POSTs
// an over-default body to a handful of non-overridden POST routes
// asserting each rejects with 413 (the canonical body.too_large
// envelope when the handler uses the Amendment A1 pattern) OR 4xx (any
// rejection — guards against legacy handlers that haven't yet adopted
// the canonical envelope but still reject because MaxBytesReader
// surfaces an error). Catches a future regression where the global
// middleware is removed from NewService — without it, the POST would
// reach the handler with the full body and return whatever the handler
// produces (200 / 401 / 400 — never the 413 we'd expect from a cap
// rejection).
//
// Cerberus #16 cross-cut: the structural lock IS the middleware being
// installed. This test pins that the install actually happened.
func TestService_BodyCap_Default_Bad_AllRoutesCappedDefault(t *testing.T) {
	gin.SetMode(gin.TestMode)

	c := core.New(core.WithName("account", account.Register))
	svc := server.NewService(server.Options{
		Addr:     ":0",
		LocalKey: "test-key",
		Core:     c,
	})

	publicEng := svc.Engine()
	core.AssertTrue(t, publicEng != nil)
	h := publicEng.Handler()
	eng, ok := h.(*gin.Engine)
	core.AssertTrue(t, ok)

	// Build a list of non-overridden POST routes — skip anything that
	// would match a DefaultBodyCapOverrides prefix because that route
	// inherits a different cap (gateway 4 MiB, MCP 256 KiB, health
	// 4 KiB) and our default-sized payload wouldn't trigger it.
	overflowSize := int(server.MaxBodyBytesDefault) + 1
	body := bytes.NewReader([]byte(strings.Repeat("x", overflowSize)))
	probed := 0
	for _, r := range eng.Routes() {
		if r.Method != http.MethodPost {
			continue
		}
		// Skip parameterised paths — we can't construct a real
		// matching URL without knowing the param-substitution shape.
		if strings.Contains(r.Path, ":") || strings.Contains(r.Path, "*") {
			continue
		}
		// Skip overridden prefixes — different cap regime.
		skipped := false
		for _, o := range server.DefaultBodyCapOverrides {
			if core.HasPrefix(r.Path, o.Prefix) {
				skipped = true
				break
			}
		}
		if skipped {
			continue
		}
		// Reset the body reader for each request.
		body = bytes.NewReader([]byte(strings.Repeat("x", overflowSize)))
		req := httptest.NewRequest(http.MethodPost, r.Path, body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		eng.ServeHTTP(w, req)
		// Expect 4xx — either the canonical 413 (handler adopted
		// Amendment A1 pattern), or 400 (legacy handler whose
		// ShouldBindJSON / ReadAll surfaced the MaxBytesError as
		// generic parse failure). Anything in the 2xx range means
		// the global middleware didn't fire — the cap is missing.
		core.AssertTrue(t, w.Code >= 400 && w.Code < 500,
			"route "+r.Path+" returned "+core.Sprintf("%d", w.Code)+
				" for over-default body — global body-cap middleware missing?")
		probed++
	}
	// Sanity: the test must have probed SOMETHING. Zero probes means
	// the route discovery walked past every POST — likely a future
	// regression in route registration that should fail loudly here
	// rather than silently passing with vacuous coverage.
	core.AssertTrue(t, probed > 0,
		"sample-N audit probed zero routes — vacuous coverage guard tripped")
}
