// SPDX-Licence-Identifier: EUPL-1.2

// Service-level tests — primary one today is the route-tier
// completeness check per RFC.stage-e.md v2 §4 H1 (Cerberus DREAD
// HIGH). Deny-by-default is the runtime defence; this CI test is
// the build-time defence.

package server_test

import (
	"testing"

	core "dappco.re/go"
	coreapi "dappco.re/go/api"
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
func TestService_AllRoutesTiered_Good(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := server.NewService(server.Options{
		Addr:     ":0",
		LocalKey: "test-key",
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

// silence the coreapi import — referenced indirectly via
// server.Options (which embeds coreapi types), but Go requires the
// import marker to keep the file compileable in isolation.
var _ = coreapi.WithRequestID
