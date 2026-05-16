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
		if _, inTiers := server.RouteTiers[r.Path]; inTiers {
			continue
		}
		missing = append(missing, r.Method+" "+r.Path)
	}
	if len(missing) > 0 {
		buf := "routes missing from RouteTiers / BootstrapPathScopes / skip-list (RFC §4 H1):"
		for _, m := range missing {
			buf += "\n  - " + m
		}
		buf += "\n\nAdd entries to pkg/server.RouteTiers (or RouteTierSkipList for genuine bypass paths)."
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

// silence the coreapi import — referenced indirectly via
// server.Options (which embeds coreapi types), but Go requires the
// import marker to keep the file compileable in isolation.
var _ = coreapi.WithRequestID
