// SPDX-Licence-Identifier: EUPL-1.2

// Mantis #1449 Path 3 — tests proving the WebView-only mount split.
//
// The lthn-process RouteGroup must NOT be reachable from the standalone
// HTTP listener (the surface `lthn serve` binds). It MUST stay reachable
// from the Wails Asset.Handler so the WebView's same-origin
// /v1/api/process/* fetches keep working unchanged.
//
// Assertions go via:
//
//  1. Engine() Groups() / WebViewEngine() Groups() — structural — the
//     filter places the right group on the right engine.
//  2. Handler() response body — behavioural — the composite dispatches
//     /v1/api/process/* to the webview engine, so the probe route's
//     "hit":"lthn-process" body lands.
//  3. Engine().Handler() response body — behavioural — the public
//     engine returns 404 for /v1/api/process/probe because the
//     webViewOnlyRejectMiddleware aborts the request before the
//     coreapi.responseMetaMiddleware can rewrite the empty 404 to
//     a 200+empty body (Mantis #1458 — OPSEC-clean signal that the
//     public surface genuinely does not host process REST).

package server_test

import (
	"net/http/httptest"

	core "dappco.re/go"
	coreapi "dappco.re/go/api"
	"github.com/gin-gonic/gin"

	"dappco.re/lthn/desktop/pkg/server"
	"dappco.re/lthn/desktop/pkg/serverkey"
)

// stubGroup is a minimal coreapi.RouteGroup carrying one probe GET so
// tests can assert reachability. name + base let each test pick a
// distinct identity for the WebViewOnlyGroups filter. The probe body
// carries the group name verbatim so a body-content assertion proves
// which engine resolved the request.
type stubGroup struct {
	name string
	base string
}

func (g *stubGroup) Name() string     { return g.name }
func (g *stubGroup) BasePath() string { return g.base }
func (g *stubGroup) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/probe", func(c *gin.Context) {
		c.JSON(core.StatusOK, gin.H{"hit": g.name})
	})
}

// stubProvider implements pkg/server.RoutesProvider so NewService's
// auto-discovery loop picks up the embedded groups.
type stubProvider struct {
	groups []coreapi.RouteGroup
}

func (p *stubProvider) RouteGroups() []coreapi.RouteGroup { return p.groups }

// processProbePath is the URL the WebView (and the test) hits on the
// lthn-process group. Mirrors the real
// /v1/api/process + /<route> shape RouteGroup.RegisterRoutes emits.
const processProbePath = "/v1/api/process/probe"

// processGroupName mirrors the constant pkg/process exports as
// GroupName. Inlined here so this test stays decoupled from the
// pkg/process import (which would create a server → process compile
// edge — the whole point of the Options.WebViewOnlyGroups string slice
// is that the filter is caller-driven, not server-knows-process).
const processGroupName = "lthn-process"

// processBasePath mirrors pkg/process.APIBasePath. Inlined per the
// processGroupName rationale above.
const processBasePath = "/v1/api/process"

// otherGroupName is a non-flagged group used to verify the filter only
// diverts the names the caller explicitly listed.
const otherGroupName = "lthn-other"

// otherBasePath is a non-flagged group's BasePath.
const otherBasePath = "/v1/api/other"

// otherProbePath is the request path the test hits to confirm the
// non-flagged group stayed on the public engine.
const otherProbePath = "/v1/api/other/probe"

// probeBodyHit is the substring the probe handler emits in its JSON
// body for the lthn-process group. Presence proves the request reached
// the registered handler; absence proves it didn't.
const probeBodyHit = `"hit":"lthn-process"`

// newCoreWithProvider builds a Core, registers a service named
// "stub-routes" that exposes the given groups via RoutesProvider, and
// returns the Core for NewService to auto-discover.
func newCoreWithProvider(groups ...coreapi.RouteGroup) *core.Core {
	c := core.New()
	provider := &stubProvider{groups: groups}
	c.RegisterService("stub-routes", provider)
	return c
}

// groupNames returns the Name() of every group registered on engine.
func groupNames(engine *coreapi.Engine) []string {
	if engine == nil {
		return nil
	}
	out := make([]string, 0, len(engine.Groups()))
	for _, g := range engine.Groups() {
		if g == nil {
			continue
		}
		out = append(out, g.Name())
	}
	return out
}

// containsString reports whether haystack holds needle.
func containsString(haystack []string, needle string) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}
	return false
}

// --- Mantis #1449 Path 3 — public engine MUST NOT mount lthn-process ---

// TestServerWebViewSplit_Process_Good_PublicEngineDoesNotMountGroup
// is the structural assertion: the lthn-process RouteGroup is not in
// the public engine's Groups() list. This is the load-bearing
// reach-reduction guarantee for #1449 — every consumer of the public
// engine (the HTTP listener, the SPA fallback, the SDK gen surface)
// inherits this absence.
func TestServerWebViewSplit_Process_Good_PublicEngineDoesNotMountGroup(t *core.T) {
	c := newCoreWithProvider(&stubGroup{name: processGroupName, base: processBasePath})
	s := server.NewService(server.Options{
		Core:              c,
		WebViewOnlyGroups: []string{processGroupName},
	})

	names := groupNames(s.Engine())
	if containsString(names, processGroupName) {
		t.Fatalf("public engine must NOT mount %q (Mantis #1449 Path 3) — registered groups: %v",
			processGroupName, names)
	}
}

// TestServerWebViewSplit_Process_Good_WebViewEngineMountsGroup is the
// counterpart structural assertion: the lthn-process group lands on
// the webview engine where Handler() can route to it for in-WebView
// fetches.
func TestServerWebViewSplit_Process_Good_WebViewEngineMountsGroup(t *core.T) {
	c := newCoreWithProvider(&stubGroup{name: processGroupName, base: processBasePath})
	s := server.NewService(server.Options{
		Core:              c,
		WebViewOnlyGroups: []string{processGroupName},
	})

	names := groupNames(s.WebViewEngine())
	if !containsString(names, processGroupName) {
		t.Fatalf("webview engine must mount %q — registered groups: %v",
			processGroupName, names)
	}
}

// TestServerWebViewSplit_Process_Good_WebViewHandlerServesProbeBody is
// the behavioural counterpart: the composite Handler() dispatches the
// process probe path to the webview engine, the handler runs, and the
// "hit":"lthn-process" payload appears in the response body. Confirms
// the WebView (Wails Asset.Handler) keeps working unchanged.
func TestServerWebViewSplit_Process_Good_WebViewHandlerServesProbeBody(t *core.T) {
	c := newCoreWithProvider(&stubGroup{name: processGroupName, base: processBasePath})
	s := server.NewService(server.Options{
		Core:              c,
		WebViewOnlyGroups: []string{processGroupName},
	})

	req := httptest.NewRequest(core.MethodGet, processProbePath, nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	core.AssertEqual(t, core.StatusOK, w.Code,
		"WebView Handler() must serve the process probe")
	core.AssertTrue(t, core.Contains(w.Body.String(), probeBodyHit),
		"composite must dispatch /v1/api/process/* to webview engine: body=%q",
		w.Body.String())
}

// TestServerWebViewSplit_Process_Bad_PublicHandlerOmitsProbeBody is
// the behavioural reach-reduction assertion: hitting the public engine
// directly (the surface Start() binds for `lthn serve`) for the same
// process probe path returns a response that does NOT carry the probe
// handler's "hit" payload — because the route handler is not
// registered there. Combined with the structural assertion above this
// is the load-bearing #1449 guarantee.
//
// Mantis #1458 — also asserts the response is a clean 404 (not the
// pre-fix 200+empty default coreapi.responseMetaMiddleware would
// otherwise emit). The webViewOnlyRejectMiddleware on the public
// engine aborts the request with 404 before responseMetaMiddleware
// can rewrite the status.
func TestServerWebViewSplit_Process_Bad_PublicHandlerOmitsProbeBody(t *core.T) {
	c := newCoreWithProvider(&stubGroup{name: processGroupName, base: processBasePath})
	s := server.NewService(server.Options{
		Core:              c,
		WebViewOnlyGroups: []string{processGroupName},
	})

	req := httptest.NewRequest(core.MethodGet, processProbePath, nil)
	w := httptest.NewRecorder()
	s.Engine().Handler().ServeHTTP(w, req)

	if core.Contains(w.Body.String(), probeBodyHit) {
		t.Fatalf("public engine must NOT serve /v1/api/process/* (Mantis #1449 Path 3) — body=%q",
			w.Body.String())
	}
	if w.Code != core.StatusNotFound {
		t.Fatalf("public engine must return 404 for /v1/api/process/* (Mantis #1458) — got status=%d body=%q",
			w.Code, w.Body.String())
	}
}

// TestServerWebViewSplit_Process_Good_ServeListener_Returns404OnProcessPrefix
// is the load-bearing #1458 assertion against the surface a real
// caller hits — the http.Handler bound by Start() for `lthn serve`.
// Reaches into Engine().Handler() (the same handler s.http.Handler
// wraps) and confirms every flavour of /v1/api/process/* path
// returns a clean 404, not the pre-fix 200+empty.
//
// Three flavours exercised: bare prefix, deep nested path, exact
// prefix match (no trailing). All three must 404 because pathHasPrefix
// matches the same set the webview composite diverts on.
func TestServerWebViewSplit_Process_Good_ServeListener_Returns404OnProcessPrefix(t *core.T) {
	c := newCoreWithProvider(&stubGroup{name: processGroupName, base: processBasePath})
	s := server.NewService(server.Options{
		Core:              c,
		WebViewOnlyGroups: []string{processGroupName},
	})

	paths := []string{
		"/v1/api/process",                       // exact prefix
		"/v1/api/process/probe",                 // shallow
		"/v1/api/process/processes/list/active", // deep
	}
	for _, p := range paths {
		req := httptest.NewRequest(core.MethodGet, p, nil)
		w := httptest.NewRecorder()
		s.Engine().Handler().ServeHTTP(w, req)

		if w.Code != core.StatusNotFound {
			t.Fatalf("serve listener must 404 webview-only prefix path %q (Mantis #1458) — got status=%d body=%q",
				p, w.Code, w.Body.String())
		}
	}
}

// TestServerWebViewSplit_Process_Good_WebViewComposite_StillServesProbe
// is the symmetric assertion: the 404 reject middleware on the public
// engine does NOT affect the WebView composite handler. A request
// matching the webview-only prefix on Handler() is diverted to the
// webview engine BEFORE the public engine's middleware chain runs,
// so the probe body still lands. Guards against a regression where
// the #1458 fix accidentally breaks the WebView side.
func TestServerWebViewSplit_Process_Good_WebViewComposite_StillServesProbe(t *core.T) {
	c := newCoreWithProvider(&stubGroup{name: processGroupName, base: processBasePath})
	s := server.NewService(server.Options{
		Core:              c,
		WebViewOnlyGroups: []string{processGroupName},
	})

	req := httptest.NewRequest(core.MethodGet, processProbePath, nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	core.AssertEqual(t, core.StatusOK, w.Code,
		"WebView composite must still serve /v1/api/process/* (Mantis #1458 reject must not bleed across)")
	core.AssertTrue(t, core.Contains(w.Body.String(), probeBodyHit),
		"WebView composite must dispatch probe to webview engine: body=%q",
		w.Body.String())
}

// TestServerWebViewSplit_Process_Good_LegacyShape_ServeListener_NoRejectWhenSplitDisabled
// asserts the pre-#1449 legacy code path (DisableDefaultWebViewOnly:
// true, empty WebViewOnlyGroups) does NOT install the reject
// middleware. The slice is empty, the middleware is a no-op, and the
// existing 200+empty default for unmatched routes stays in place.
// Belt-and-braces — production callers never reach this state but
// the legacy shape regression test (already in this file) needs the
// reject middleware out of the way.
func TestServerWebViewSplit_Process_Good_LegacyShape_ServeListener_NoRejectWhenSplitDisabled(t *core.T) {
	c := newCoreWithProvider(&stubGroup{name: processGroupName, base: processBasePath})
	s := server.NewService(server.Options{
		Core:                      c,
		DisableDefaultWebViewOnly: true,
	})

	// The probe IS registered on the public engine under legacy shape,
	// so a hit should return the probe body — not 404.
	req := httptest.NewRequest(core.MethodGet, processProbePath, nil)
	w := httptest.NewRecorder()
	s.Engine().Handler().ServeHTTP(w, req)

	core.AssertEqual(t, core.StatusOK, w.Code,
		"legacy shape must keep serving /v1/api/process/* from public engine — reject middleware must stay dormant")
	core.AssertTrue(t, core.Contains(w.Body.String(), probeBodyHit),
		"legacy shape public engine must serve probe body: body=%q",
		w.Body.String())
}

// TestServerWebViewSplit_Process_Ugly_BearerLeakViaPublicListenerStillBlocked
// is the threat-model assertion: even an attacker presenting the
// correct bearer token cannot reach process REST through the public
// HTTP surface. Path 3's reach reduction is structural (route not
// mounted), so the bearer is irrelevant — Wails auth-bypass and any
// future timing-oracle attack on the bearer simply have no surface to
// hit.
func TestServerWebViewSplit_Process_Ugly_BearerLeakViaPublicListenerStillBlocked(t *core.T) {
	c := newCoreWithProvider(&stubGroup{name: processGroupName, base: processBasePath})
	s := server.NewService(server.Options{
		Core:              c,
		LocalKey:          "sk-stolen-bearer-1234567890abcdef",
		WebViewOnlyGroups: []string{processGroupName},
	})

	req := httptest.NewRequest(core.MethodGet, processProbePath, nil)
	req.Header.Set("Authorization", "Bearer sk-stolen-bearer-1234567890abcdef")
	w := httptest.NewRecorder()
	s.Engine().Handler().ServeHTTP(w, req)

	if core.Contains(w.Body.String(), probeBodyHit) {
		t.Fatalf("bearer presence must not unlock /v1/api/process/* — body=%q", w.Body.String())
	}
}

// --- WebView accessor surface ---

// TestServerWebViewSplit_WebViewEngine_Good_NonNilWhenGroupsFlagged
// confirms the new WebViewEngine() accessor returns the engine when
// the caller flagged at least one group for WebView-only mounting.
func TestServerWebViewSplit_WebViewEngine_Good_NonNilWhenGroupsFlagged(t *core.T) {
	c := newCoreWithProvider(&stubGroup{name: processGroupName, base: processBasePath})
	s := server.NewService(server.Options{
		Core:              c,
		WebViewOnlyGroups: []string{processGroupName},
	})

	core.AssertNotNil(t, s.WebViewEngine(),
		"WebViewEngine must be available when WebViewOnlyGroups is non-empty")
}

// TestServerWebViewSplit_WebViewEngine_Bad_NilWhenAllOptOut confirms
// the WebView engine is not constructed when both the caller's
// WebViewOnlyGroups slice AND the built-in defaults are disabled —
// e.g. a unit test exercising the pre-#1449 single-engine code path.
// Production callers cannot reach this state because they don't set
// DisableDefaultWebViewOnly.
func TestServerWebViewSplit_WebViewEngine_Bad_NilWhenAllOptOut(t *core.T) {
	c := newCoreWithProvider(&stubGroup{name: processGroupName, base: processBasePath})
	s := server.NewService(server.Options{
		Core:                      c,
		DisableDefaultWebViewOnly: true,
	})

	if s.WebViewEngine() != nil {
		t.Fatalf("WebViewEngine must be nil when defaults are disabled and no custom groups are set")
	}
}

// TestServerWebViewSplit_WebViewEngine_Good_NonNilByDefaultUnderProductionShape
// confirms that the production default — no explicit
// WebViewOnlyGroups + DisableDefaultWebViewOnly unset — still
// constructs the WebView engine because lthn-process is filtered by
// the built-in default. Guards against a regression where the default
// list gets emptied and silently drops the #1449 reach reduction.
func TestServerWebViewSplit_WebViewEngine_Good_NonNilByDefaultUnderProductionShape(t *core.T) {
	c := newCoreWithProvider(&stubGroup{name: processGroupName, base: processBasePath})
	s := server.NewService(server.Options{Core: c})

	core.AssertNotNil(t, s.WebViewEngine(),
		"production default (no options) must build the WebView engine — lthn-process is always filtered")
}

// --- Opt-in default — no split, no behaviour change ---

// TestServerWebViewSplit_Default_Good_LegacyShape_GroupVisibleOnPublic
// confirms the legacy single-engine code path still works for callers
// that explicitly opt out of the built-in filter
// (DisableDefaultWebViewOnly: true). Useful for pre-#1449 regression
// testing and for hypothetical future binaries that want the legacy
// behaviour.
func TestServerWebViewSplit_Default_Good_LegacyShape_GroupVisibleOnPublic(t *core.T) {
	c := newCoreWithProvider(&stubGroup{name: processGroupName, base: processBasePath})
	s := server.NewService(server.Options{
		Core:                      c,
		DisableDefaultWebViewOnly: true,
	})

	names := groupNames(s.Engine())
	if !containsString(names, processGroupName) {
		t.Fatalf("legacy shape (DisableDefaultWebViewOnly=true) must keep %q on the public engine — groups: %v",
			processGroupName, names)
	}
}

// TestServerWebViewSplit_Default_Good_LegacyShape_HandlerStillServesGroupBody
// confirms the composite degrades to the public handler verbatim when
// the split is fully opted out (webviewEngine == nil), and the public
// engine continues serving the route.
func TestServerWebViewSplit_Default_Good_LegacyShape_HandlerStillServesGroupBody(t *core.T) {
	c := newCoreWithProvider(&stubGroup{name: processGroupName, base: processBasePath})
	s := server.NewService(server.Options{
		Core:                      c,
		DisableDefaultWebViewOnly: true,
	})

	req := httptest.NewRequest(core.MethodGet, processProbePath, nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	core.AssertEqual(t, core.StatusOK, w.Code)
	core.AssertTrue(t, core.Contains(w.Body.String(), probeBodyHit),
		"legacy shape Handler() must keep serving the group: body=%q",
		w.Body.String())
}

// TestServerWebViewSplit_Default_Good_ProductionShape_GroupNotOnPublic
// is the symmetric production-default assertion: under no explicit
// options the lthn-process group is automatically WebView-only. Guards
// against a regression that silently empties the built-in default
// list.
func TestServerWebViewSplit_Default_Good_ProductionShape_GroupNotOnPublic(t *core.T) {
	c := newCoreWithProvider(&stubGroup{name: processGroupName, base: processBasePath})
	s := server.NewService(server.Options{Core: c})

	names := groupNames(s.Engine())
	if containsString(names, processGroupName) {
		t.Fatalf("production default must filter %q off the public engine — groups: %v",
			processGroupName, names)
	}
}

// --- Filter precision — only flagged groups divert ---

// TestServerWebViewSplit_NonFlagged_Good_PublicEngineKeepsServingOtherGroup
// confirms the WebViewOnlyGroups filter is precise — a group whose
// Name() is NOT in the list stays on the public engine even when the
// split is active. Guards against a regression where the filter
// accidentally captures every auto-discovered group.
func TestServerWebViewSplit_NonFlagged_Good_PublicEngineKeepsServingOtherGroup(t *core.T) {
	c := newCoreWithProvider(
		&stubGroup{name: processGroupName, base: processBasePath},
		&stubGroup{name: otherGroupName, base: otherBasePath},
	)
	s := server.NewService(server.Options{
		Core:              c,
		WebViewOnlyGroups: []string{processGroupName},
	})

	// Structural: other group lives on public engine.
	publicNames := groupNames(s.Engine())
	if !containsString(publicNames, otherGroupName) {
		t.Fatalf("non-flagged group %q must stay on public engine — groups: %v",
			otherGroupName, publicNames)
	}

	// Behavioural: request resolves through the public engine.
	req := httptest.NewRequest(core.MethodGet, otherProbePath, nil)
	w := httptest.NewRecorder()
	s.Engine().Handler().ServeHTTP(w, req)
	core.AssertEqual(t, core.StatusOK, w.Code)
	core.AssertTrue(t, core.Contains(w.Body.String(), `"hit":"lthn-other"`),
		"non-flagged group must keep its public mount: body=%q", w.Body.String())
}

// TestServerWebViewSplit_NonFlagged_Good_WebViewEngineDoesNotMountOtherGroup
// is the symmetric assertion — the WebView engine carries ONLY the
// flagged groups, not every auto-discovered group.
func TestServerWebViewSplit_NonFlagged_Good_WebViewEngineDoesNotMountOtherGroup(t *core.T) {
	c := newCoreWithProvider(
		&stubGroup{name: processGroupName, base: processBasePath},
		&stubGroup{name: otherGroupName, base: otherBasePath},
	)
	s := server.NewService(server.Options{
		Core:              c,
		WebViewOnlyGroups: []string{processGroupName},
	})

	wvNames := groupNames(s.WebViewEngine())
	if containsString(wvNames, otherGroupName) {
		t.Fatalf("webview engine must not host non-flagged group %q — groups: %v",
			otherGroupName, wvNames)
	}
}

// TestServerWebViewSplit_Health_Good_StillServedByPublicEngine
// confirms the split didn't accidentally move /health off the public
// engine. /health is the cheapest stable canary — if it survives, the
// middleware chain on the public engine is intact.
func TestServerWebViewSplit_Health_Good_StillServedByPublicEngine(t *core.T) {
	c := newCoreWithProvider(&stubGroup{name: processGroupName, base: processBasePath})
	s := server.NewService(server.Options{
		Core:              c,
		WebViewOnlyGroups: []string{processGroupName},
	})

	req := httptest.NewRequest(core.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	s.Engine().Handler().ServeHTTP(w, req)

	core.AssertEqual(t, core.StatusOK, w.Code,
		"/health must keep responding from the public engine — split must not move it")
	core.AssertTrue(t, core.Contains(w.Body.String(), `"data":"healthy"`),
		"/health body must be intact: body=%q", w.Body.String())
}

// TestServerWebViewSplit_Health_Good_StillServedByCompositeHandler is
// the symmetric assertion — the composite must fall through to the
// public engine for non-process paths so /health keeps working from
// the WebView side too.
func TestServerWebViewSplit_Health_Good_StillServedByCompositeHandler(t *core.T) {
	c := newCoreWithProvider(&stubGroup{name: processGroupName, base: processBasePath})
	s := server.NewService(server.Options{
		Core:              c,
		WebViewOnlyGroups: []string{processGroupName},
	})

	req := httptest.NewRequest(core.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	core.AssertEqual(t, core.StatusOK, w.Code,
		"composite must fall through to the public engine for non-process paths")
	core.AssertTrue(t, core.Contains(w.Body.String(), `"data":"healthy"`),
		"/health body via composite must be intact: body=%q", w.Body.String())
}

// --- Cerberus C#17 F-1 — middleware ordering regression ---

// TestServeListener_ProcessRoute_404NotAuth_Bad asserts the load-bearing
// ordering invariant the Cerberus C#17 F-1 finding caught: under the
// PRODUCTION wire shape (ServerKey present → WithBootstrapAndSessionAuth
// installs the bootstrap-plus-bearer middleware), an unmatched webview-
// only /v1/api/process/* path on the public engine must still return
// 404 — NOT 401.
//
// Pre-fix history: webViewOnlyRejectMiddleware was appended to
// publicOpts AFTER auth in service.go, so gin ran auth first. With
// ServerKey wired (the production shape), the request 401'd before
// reject ever ran. That regressed Mantis #1458 OPSEC intent — a 401
// signals "there is a resource behind the auth gate" while a 404 says
// "the public surface genuinely does not host this route". The existing
// #1458 tests (TestServerWebViewSplit_Process_Good_ServeListener_*) all
// ran WITHOUT ServerKey so auth was never installed; the ordering bug
// was masked.
//
// Fix shape: publicOpts now appends reject BEFORE auth, and the
// webview engine continues to receive auth without reject (the webview
// engine must serve those routes, not 404 them). This test is the
// guard rail: any future restructure that puts auth before reject on
// publicOpts will fail here.
func TestServeListener_ProcessRoute_404NotAuth_Bad(t *core.T) {
	_ = homeFixture(t)
	svc := serverkey.NewService(nil)
	core.AssertTrue(t, svc.Bootstrap().OK, "serverkey.Bootstrap must succeed")

	c := newCoreWithProvider(&stubGroup{name: processGroupName, base: processBasePath})
	s := server.NewService(server.Options{
		Core:              c,
		ServerKey:         svc,
		LocalKey:          "bearer-secret-1234567890abcdef",
		WebViewOnlyGroups: []string{processGroupName},
	})

	paths := []string{
		"/v1/api/process",                       // exact prefix
		"/v1/api/process/foo",                   // shallow
		"/v1/api/process/processes/list/active", // deep
	}
	for _, p := range paths {
		req := httptest.NewRequest(core.MethodGet, p, nil)
		// Deliberately no Authorization header — proves reject fires
		// before auth even though auth would have rejected this too.
		w := httptest.NewRecorder()
		s.Engine().Handler().ServeHTTP(w, req)

		if w.Code == core.StatusUnauthorized {
			t.Fatalf("path %q: reject middleware must fire BEFORE auth (Cerberus C#17 F-1) — got 401 leaks resource existence, expected 404 OPSEC-clean per Mantis #1458; body=%q",
				p, w.Body.String())
		}
		if w.Code != core.StatusNotFound {
			t.Fatalf("path %q: production-shape (ServerKey wired) public engine must 404 webview-only prefix — got status=%d body=%q",
				p, w.Code, w.Body.String())
		}
	}
}

// TestServeListener_AuthStillProtectsNonWebviewPaths_Good is the
// counterpart assertion to F-1: the ordering fix moved reject before
// auth, but auth still must run for paths reject lets through. A
// /v1/api/chat request with no auth header must 401 (the path is not
// webview-only, so reject is a no-op, and auth then rejects).
//
// Guards against an over-correction where reject swallows EVERY
// request and auth never runs.
func TestServeListener_AuthStillProtectsNonWebviewPaths_Good(t *core.T) {
	_ = homeFixture(t)
	svc := serverkey.NewService(nil)
	core.AssertTrue(t, svc.Bootstrap().OK, "serverkey.Bootstrap must succeed")

	c := newCoreWithProvider(&stubGroup{name: processGroupName, base: processBasePath})
	s := server.NewService(server.Options{
		Core:              c,
		ServerKey:         svc,
		LocalKey:          "bearer-secret-1234567890abcdef",
		WebViewOnlyGroups: []string{processGroupName},
	})

	// /v1/api/chat is NOT in WebViewOnlyGroups, so reject is a no-op
	// and auth must run. No Authorization header → 401.
	req := httptest.NewRequest(core.MethodGet, "/v1/api/chat", nil)
	w := httptest.NewRecorder()
	s.Engine().Handler().ServeHTTP(w, req)

	core.AssertEqual(t, core.StatusUnauthorized, w.Code,
		"non-webview path with no auth must still 401 — reject must not swallow every request")
}
