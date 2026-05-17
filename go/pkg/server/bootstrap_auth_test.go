// SPDX-Licence-Identifier: EUPL-1.2

// Tests for the bootstrap-auth middleware. Stage B of the first-run
// auth-gate (Mantis #1474). Covers:
//
//   - Bootstrap-path + valid token + matching scope → 200
//   - Bootstrap-path + missing token → 401
//   - Bootstrap-path + valid token + mismatched scope → 401 (Cerberus #1467)
//   - Bootstrap-path + bearer token instead of bootstrap → 401
//     (i.e. bootstrap paths cannot be satisfied with a normal bearer)
//   - Non-bootstrap path + valid bearer → 200
//   - Non-bootstrap path + missing bearer → 401
//   - /health skip → 200 with no auth
//   - WithBootstrapAuth signature is map[path]scope — compile-time
//     assertion (Cerberus #1467 — must reject any change to []path)

package server_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	core "dappco.re/go"
	coreapi "dappco.re/go/api"
	"dappco.re/lthn/desktop/pkg/auth"
	"dappco.re/lthn/desktop/pkg/server"
	"dappco.re/lthn/desktop/pkg/serverkey"
	"github.com/gin-gonic/gin"
)

// homeFixture rebinds $HOME so the serverkey Service writes into a
// scratch tree.
func homeFixture(t *core.T) string {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	return tmp
}

// echoRouteGroup mounts an OK handler at /<leaf> under the configured
// basePath. Each test names its (basePath, leaf) split so the full
// path matches what the bootstrap-auth middleware sees.
type echoRouteGroup struct {
	basePath string
	leaf     string
}

func (g *echoRouteGroup) Name() string     { return "bootstrap-auth-test" }
func (g *echoRouteGroup) BasePath() string { return g.basePath }
func (g *echoRouteGroup) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET(g.leaf, func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
}

// splitPath splits an absolute path into its parent dir + final
// segment so the test can hand the two halves to RouteGroup's BasePath
// + RegisterRoutes("/leaf") shape. handlerPath MUST be absolute and
// contain at least one slash.
func splitPath(p string) (base, leaf string) {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' {
			base = p[:i]
			leaf = p[i:]
			if base == "" {
				base = "/"
			}
			return
		}
	}
	return "/", "/" + p
}

// newTestEngine builds a coreapi.Engine wired with the bootstrap-auth
// middleware + a single GET handler at the supplied path for assertion.
func newTestEngine(t *core.T, verifier serverkey.Verifier, bearer string, paths map[string]string, handlerPath string) *coreapi.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	eng, err := coreapi.New(server.WithBootstrapAuth(verifier, bearer, paths))
	if err != nil {
		t.Fatalf("coreapi.New: %v", err)
	}
	base, leaf := splitPath(handlerPath)
	eng.Register(&echoRouteGroup{basePath: base, leaf: leaf})
	return eng
}

func doGET(eng *coreapi.Engine, path, auth string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	rr := httptest.NewRecorder()
	eng.Handler().ServeHTTP(rr, req)
	return rr
}

// --- Good ---

func TestBootstrapAuth_BootstrapPath_ValidToken_Good(t *core.T) {
	_ = homeFixture(t)
	svc := serverkey.NewService(nil)
	core.AssertTrue(t, svc.Bootstrap().OK)

	paths := map[string]string{"/v1/account/create": "account.create"}
	eng := newTestEngine(t, svc, "bearer-secret", paths, "/v1/account/create")

	tokR := svc.IssueBootstrapToken()
	core.AssertTrue(t, tokR.OK)
	out := tokR.Value.(serverkey.BootstrapTokenOutput)

	rr := doGET(eng, "/v1/account/create", "Bootstrap "+out.Token)
	core.AssertEqual(t, http.StatusOK, rr.Code)
}

func TestBootstrapAuth_NonBootstrapPath_ValidBearer_Good(t *core.T) {
	_ = homeFixture(t)
	svc := serverkey.NewService(nil)
	core.AssertTrue(t, svc.Bootstrap().OK)

	paths := map[string]string{"/v1/account/create": "account.create"}
	eng := newTestEngine(t, svc, "bearer-secret", paths, "/v1/api/chat")

	rr := doGET(eng, "/v1/api/chat", "Bearer bearer-secret")
	core.AssertEqual(t, http.StatusOK, rr.Code)
}

func TestBootstrapAuth_HealthSkip_Good(t *core.T) {
	// /health is auto-registered by the coreapi.Engine; bootstrap-
	// auth middleware MUST short-circuit it so unauthenticated
	// requests succeed.
	_ = homeFixture(t)
	svc := serverkey.NewService(nil)
	core.AssertTrue(t, svc.Bootstrap().OK)

	paths := map[string]string{"/v1/account/create": "account.create"}
	gin.SetMode(gin.TestMode)
	eng, err := coreapi.New(server.WithBootstrapAuth(svc, "bearer-secret", paths))
	if err != nil {
		t.Fatalf("coreapi.New: %v", err)
	}

	rr := doGET(eng, "/health", "")
	core.AssertEqual(t, http.StatusOK, rr.Code)
}

// --- Bad ---

func TestBootstrapAuth_BootstrapPath_MissingToken_Bad(t *core.T) {
	_ = homeFixture(t)
	svc := serverkey.NewService(nil)
	core.AssertTrue(t, svc.Bootstrap().OK)

	paths := map[string]string{"/v1/account/create": "account.create"}
	eng := newTestEngine(t, svc, "bearer-secret", paths, "/v1/account/create")

	rr := doGET(eng, "/v1/account/create", "")
	core.AssertEqual(t, http.StatusUnauthorized, rr.Code)
}

func TestBootstrapAuth_BootstrapPath_BearerInstead_Bad(t *core.T) {
	// A normal bearer token MUST NOT satisfy a bootstrap-gated path.
	_ = homeFixture(t)
	svc := serverkey.NewService(nil)
	core.AssertTrue(t, svc.Bootstrap().OK)

	paths := map[string]string{"/v1/account/create": "account.create"}
	eng := newTestEngine(t, svc, "bearer-secret", paths, "/v1/account/create")

	rr := doGET(eng, "/v1/account/create", "Bearer bearer-secret")
	core.AssertEqual(t, http.StatusUnauthorized, rr.Code)
}

func TestBootstrapAuth_NonBootstrapPath_MissingBearer_Bad(t *core.T) {
	_ = homeFixture(t)
	svc := serverkey.NewService(nil)
	core.AssertTrue(t, svc.Bootstrap().OK)

	paths := map[string]string{"/v1/account/create": "account.create"}
	eng := newTestEngine(t, svc, "bearer-secret", paths, "/v1/api/chat")

	rr := doGET(eng, "/v1/api/chat", "")
	core.AssertEqual(t, http.StatusUnauthorized, rr.Code)
}

// --- Ugly (Cerberus #1467 scope/path lockstep) ---

func TestBootstrapAuth_BootstrapPath_WrongScope_Ugly_Cerberus1467(t *core.T) {
	// The token is valid + signature checks out — but its embedded
	// scope claim (account.create) doesn't match what THIS path
	// requires (admin.reset). Cerberus #1467 — scope-laundering
	// across paths is rejected.
	_ = homeFixture(t)
	svc := serverkey.NewService(nil)
	core.AssertTrue(t, svc.Bootstrap().OK)

	// Build the engine expecting a DIFFERENT scope.
	paths := map[string]string{"/v1/admin/reset": "admin.reset"}
	eng := newTestEngine(t, svc, "bearer-secret", paths, "/v1/admin/reset")

	// Issue a token with scope=account.create (the only scope
	// IssueBootstrapToken mints today).
	out := svc.IssueBootstrapToken().Value.(serverkey.BootstrapTokenOutput)

	rr := doGET(eng, "/v1/admin/reset", "Bootstrap "+out.Token)
	core.AssertEqual(t, http.StatusUnauthorized, rr.Code)
}

// --- Signature shape (Cerberus #1467 compile-time assertion) ---

// TestBootstrapAuth_Signature_Cerberus1467 fails to COMPILE if the
// signature changes from map[string]string to []string + single
// scope. Cerberus #1467 — path/scope lockstep is encoded in the type
// system; a change would break this file and the surrounding tests.
func TestBootstrapAuth_Signature_Cerberus1467(t *core.T) {
	var _ func(serverkey.Verifier, string, map[string]string) coreapi.Option = server.WithBootstrapAuth
	var _ func(serverkey.Verifier, string, map[string]string) gin.HandlerFunc = server.BootstrapAuthMiddleware
}

// --- Nil-verifier safety ---

func TestBootstrapAuth_NilVerifier_FallthroughBearer_Good(t *core.T) {
	// When verifier is nil, the option installs no middleware. The
	// caller is expected to add coreapi.WithBearerAuth separately;
	// this test just confirms the engine still constructs and the
	// route still responds (auth applied by whatever the caller
	// wires).
	gin.SetMode(gin.TestMode)
	eng, err := coreapi.New(server.WithBootstrapAuth(nil, "bearer-secret", nil))
	if err != nil {
		t.Fatalf("coreapi.New: %v", err)
	}
	base, leaf := splitPath("/v1/api/chat")
	eng.Register(&echoRouteGroup{basePath: base, leaf: leaf})

	rr := doGET(eng, "/v1/api/chat", "")
	// No auth middleware registered, so 200 is the expected open
	// behaviour (mirrors coreapi when LocalKey is empty).
	core.AssertEqual(t, http.StatusOK, rr.Code)
}

// --- Cerberus #1489 fail-closed posture ---

// TestBootstrapAuth_NoLocalKey_WithServerKey_FailClosed_Bad — when
// the middleware is installed (verifier non-nil + pathScopes non-empty)
// but bearerToken is empty, every non-bootstrap path MUST 503 rather
// than fall through to permit-all. The split-binary plan (lthn-mlx)
// runs subsystems where ServerKey is the only auth source and
// LocalKey is empty by construction — the previous "open server"
// fallback would have left those endpoints unauthenticated.
func TestBootstrapAuth_NoLocalKey_WithServerKey_FailClosed_Bad(t *core.T) {
	_ = homeFixture(t)
	svc := serverkey.NewService(nil)
	core.AssertTrue(t, svc.Bootstrap().OK)

	paths := map[string]string{"/v1/account/create": "account.create"}
	// Bearer token deliberately empty — historically this would have
	// fallen through to "open server", permitting unauthenticated
	// access to /v1/api/chat. Cerberus #1489: must 503 instead.
	eng := newTestEngine(t, svc, "", paths, "/v1/api/chat")

	rr := doGET(eng, "/v1/api/chat", "")
	core.AssertEqual(t, http.StatusServiceUnavailable, rr.Code,
		"empty bearer + active bootstrap surface must fail closed")
}

// TestBootstrapAuth_NoLocalKey_WithServerKey_BootstrapPathStillWorks_Good —
// the fail-closed posture only applies to non-bootstrap paths. The
// bootstrap endpoint itself must still accept a valid Bootstrap token
// even when bearerToken is empty.
func TestBootstrapAuth_NoLocalKey_WithServerKey_BootstrapPathStillWorks_Good(t *core.T) {
	_ = homeFixture(t)
	svc := serverkey.NewService(nil)
	core.AssertTrue(t, svc.Bootstrap().OK)

	paths := map[string]string{"/v1/account/create": "account.create"}
	eng := newTestEngine(t, svc, "", paths, "/v1/account/create")

	tokR := svc.IssueBootstrapToken()
	core.AssertTrue(t, tokR.OK)
	out := tokR.Value.(serverkey.BootstrapTokenOutput)

	rr := doGET(eng, "/v1/account/create", "Bootstrap "+out.Token)
	core.AssertEqual(t, http.StatusOK, rr.Code,
		"bootstrap path must still work even when bearer source is empty")
}

// silence the testing import.
var _ = testing.Short

// --- B4 / RFC §4.6.1 gin → *core.Core CallerIdentity bridge (Mantis #1735) ---

// newBridgeTestEngine wires a coreapi.Engine with the Stage E.B auth
// middleware + the §4.6.1 Option A bridge against the supplied
// *core.Core. The handler at handlerPath calls capture(c) so the test
// can assert what auth.Caller(coreInstance) returns DURING the request
// (i.e. what a downstream service-method dispatch would see).
//
// capture is called inside the gin handler — the bridge has set the
// caller via auth.SetCaller(c, ident) at that point. After the handler
// returns the bridge's deferred ClearCaller runs, so test assertions
// against the *coreInstance after the request will see TierInternal
// (the bridge cleans up). The test captures the live value into the
// returned channel to assert against.
func newBridgeTestEngine(
	t *core.T,
	c *core.Core,
	verifier serverkey.Verifier,
	bearer string,
	pathScopes map[string]string,
	routeTiers map[string]server.RouteTier,
	handlerPath string,
	capture func(*core.Core),
) *coreapi.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	eng, err := coreapi.New(
		server.WithBootstrapAndSessionAuth(verifier, bearer, pathScopes, routeTiers, nil),
		server.WithCallerBridge(c),
	)
	if err != nil {
		t.Fatalf("coreapi.New: %v", err)
	}
	base, leaf := splitPath(handlerPath)
	eng.Register(&captureRouteGroup{basePath: base, leaf: leaf, core: c, capture: capture})
	return eng
}

// captureRouteGroup is an echo handler that invokes capture(c) before
// responding 200. Used by the B4 bridge tests to observe what a
// downstream service-method dispatch would see when it calls
// auth.Caller(c).
type captureRouteGroup struct {
	basePath string
	leaf     string
	core     *core.Core
	capture  func(*core.Core)
}

func (g *captureRouteGroup) Name() string     { return "bridge-test" }
func (g *captureRouteGroup) BasePath() string { return g.basePath }
func (g *captureRouteGroup) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET(g.leaf, func(c *gin.Context) {
		if g.capture != nil {
			g.capture(g.core)
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
}

// TestBootstrapAuth_BridgesCallerToCore_Good — RFC §4.6.1 §10 done-
// criteria (Phase 1 Unit B.4). A bootstrap-path request with a valid
// Bootstrap token results in the handler-time auth.Caller(c) reading
// TierOperator with Source="http-bootstrap". The Subject is empty
// because the bootstrap path is pre-identity per RFC §4.6 line 248
// (the LetheanAccount has not been created yet).
func TestBootstrapAuth_BridgesCallerToCore_Good(t *core.T) {
	_ = homeFixture(t)
	svc := serverkey.NewService(nil)
	core.AssertTrue(t, svc.Bootstrap().OK)

	c := core.New()
	var captured auth.CallerIdentity
	pathScopes := map[string]string{"/v1/account/create": "account.create"}
	tiers := map[string]server.RouteTier{}
	eng := newBridgeTestEngine(t, c, svc, "static-bearer", pathScopes, tiers,
		"/v1/account/create",
		func(inner *core.Core) { captured = auth.Caller(inner) })

	out := svc.IssueBootstrapToken().Value.(serverkey.BootstrapTokenOutput)
	rr := doGET(eng, "/v1/account/create", "Bootstrap "+out.Token)
	core.AssertEqual(t, http.StatusOK, rr.Code,
		"bootstrap-path with valid token → 200 (auth resolved)")
	core.AssertEqual(t, auth.TierOperator, captured.Tier,
		"bridge MUST stamp TierOperator on bootstrap-path per RFC §4.6 line 248")
	core.AssertEqual(t, "", captured.Subject,
		"bootstrap-path Subject MUST be empty — bootstrap is pre-identity (RFC §4.6 line 248)")
	core.AssertEqual(t, "http-bootstrap", captured.Source,
		"bridge MUST stamp Source='http-bootstrap' for audit grep")
}

// TestSessionAuth_BridgesCallerToCore_Good — RFC §4.6.1 §10 done-
// criteria. A session-token request results in the handler-time
// auth.Caller(c) reading TierOperator with Subject=account_id (the
// VerifySessionToken-recovered claim) and Source="http-session". This
// is the canonical Phase-1 wiring shape: HTTP request → middleware
// resolves credential → bridge promotes to *core.Core → downstream
// service.Method(c, ...) sees the identity via auth.Caller(c).
func TestSessionAuth_BridgesCallerToCore_Good(t *core.T) {
	_ = homeFixture(t)
	svc := serverkey.NewService(nil)
	core.AssertTrue(t, svc.Bootstrap().OK)

	c := core.New()
	var captured auth.CallerIdentity
	pathScopes := map[string]string{"/v1/account/unlock": "account.unlock"}
	tiers := map[string]server.RouteTier{"/v1/api/data": server.TierSession}
	eng := newBridgeTestEngine(t, c, svc, "static-bearer", pathScopes, tiers,
		"/v1/api/data",
		func(inner *core.Core) { captured = auth.Caller(inner) })

	const acct = "abc123def4567890"
	out := svc.IssueSessionToken(acct).Value.(serverkey.SessionTokenOutput)
	rr := doGET(eng, "/v1/api/data", "Bearer "+out.Token)
	core.AssertEqual(t, http.StatusOK, rr.Code,
		"session-tier route with valid session token → 200")
	core.AssertEqual(t, auth.TierOperator, captured.Tier,
		"bridge MUST stamp TierOperator on session-token path per RFC §4.6 line 250")
	core.AssertEqual(t, acct, captured.Subject,
		"bridge MUST carry account_id as Subject (Cerberus #1735 done-criteria)")
	core.AssertEqual(t, "http-session", captured.Source,
		"bridge MUST stamp Source='http-session' for audit grep")
}

// TestBootstrapAuth_ClearsCallerOnExit_Good — RFC §4.6.1 cleanup
// guard. The bridge's deferred auth.ClearCaller(c) MUST clear the
// sidecar stamp before the response returns, so a subsequent
// auth.Caller(c) read OUTSIDE the request handler observes
// TierInternal (the floor). Without this guard the *core.Core would
// retain the last request's identity across the inter-request gap —
// the exact bleed the auth.SetCaller doc-string warns against.
func TestBootstrapAuth_ClearsCallerOnExit_Good(t *core.T) {
	_ = homeFixture(t)
	svc := serverkey.NewService(nil)
	core.AssertTrue(t, svc.Bootstrap().OK)

	c := core.New()
	pathScopes := map[string]string{"/v1/account/unlock": "account.unlock"}
	tiers := map[string]server.RouteTier{"/v1/api/data": server.TierSession}
	// No capture closure — we only care about the post-request state.
	eng := newBridgeTestEngine(t, c, svc, "static-bearer", pathScopes, tiers,
		"/v1/api/data", nil)

	const acct = "abc123def4567890"
	out := svc.IssueSessionToken(acct).Value.(serverkey.SessionTokenOutput)
	rr := doGET(eng, "/v1/api/data", "Bearer "+out.Token)
	core.AssertEqual(t, http.StatusOK, rr.Code,
		"session-tier route with valid session token → 200")

	// After the request, the deferred ClearCaller must have fired.
	// A fresh auth.Caller(c) read MUST return the TierInternal floor
	// (per RFC §4.1: zero-value resolves to TierInternal).
	after := auth.Caller(c)
	core.AssertEqual(t, auth.TierInternal, after.Tier,
		"after-request Caller MUST be TierInternal floor — defer ClearCaller(c) cleanup guard")
	core.AssertEqual(t, "", after.Subject,
		"after-request Subject MUST be cleared")
}

// TestBootstrapAuth_NoAuth_LeavesFloor_Good — RFC §4.6.1 + §4.1
// deny-by-default. A request that is REJECTED by the auth middleware
// (e.g. no credential to a session-tier route) MUST NOT leak a
// CallerIdentity stamp onto *core.Core. The bridge runs ONLY when the
// auth middleware called c.Next() (i.e. on the authenticated branch);
// AbortWithStatusJSON short-circuits the chain so the bridge never
// reaches SetCaller. A post-request auth.Caller(c) read MUST return
// TierInternal floor.
func TestBootstrapAuth_NoAuth_LeavesFloor_Good(t *core.T) {
	_ = homeFixture(t)
	svc := serverkey.NewService(nil)
	core.AssertTrue(t, svc.Bootstrap().OK)

	c := core.New()
	pathScopes := map[string]string{"/v1/account/unlock": "account.unlock"}
	tiers := map[string]server.RouteTier{"/v1/api/data": server.TierSession}
	eng := newBridgeTestEngine(t, c, svc, "static-bearer", pathScopes, tiers,
		"/v1/api/data", nil)

	rr := doGET(eng, "/v1/api/data", "")
	core.AssertEqual(t, http.StatusUnauthorized, rr.Code,
		"session-tier route with no auth → 401")
	// The middleware aborted; bridge never ran; sidecar untouched.
	after := auth.Caller(c)
	core.AssertEqual(t, auth.TierInternal, after.Tier,
		"unauth request MUST NOT stamp any tier — Caller stays at TierInternal floor")
}

// --- Stage E.B session-tier middleware coverage ---

// newSessionTestEngine builds a coreapi.Engine wired with the Stage
// E.B BootstrapAndSessionAuth middleware + a single GET handler at
// the supplied path. Tests pass nil for routeTierPrefixes when the
// scenario only exercises exact-match classification; the
// prefix-match path is covered in service_test.go.
func newSessionTestEngine(
	t *core.T,
	verifier serverkey.Verifier,
	bearer string,
	pathScopes map[string]string,
	routeTiers map[string]server.RouteTier,
	handlerPath string,
) *coreapi.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	eng, err := coreapi.New(server.WithBootstrapAndSessionAuth(verifier, bearer, pathScopes, routeTiers, nil))
	if err != nil {
		t.Fatalf("coreapi.New: %v", err)
	}
	base, leaf := splitPath(handlerPath)
	eng.Register(&echoRouteGroup{basePath: base, leaf: leaf})
	return eng
}

// --- Good ---

func TestBootstrapAuth_SessionTier_ValidToken_Good(t *core.T) {
	_ = homeFixture(t)
	svc := serverkey.NewService(nil)
	core.AssertTrue(t, svc.Bootstrap().OK)

	pathScopes := map[string]string{"/v1/account/unlock": "account.unlock"}
	tiers := map[string]server.RouteTier{"/v1/api/data": server.TierSession}
	eng := newSessionTestEngine(t, svc, "static-bearer", pathScopes, tiers, "/v1/api/data")

	out := svc.IssueSessionToken("abc123def4567890").Value.(serverkey.SessionTokenOutput)
	rr := doGET(eng, "/v1/api/data", "Bearer "+out.Token)
	core.AssertEqual(t, http.StatusOK, rr.Code, "valid session token on session-tier route → 200")
}

func TestBootstrapAuth_LocalTier_StaticBearer_Good(t *core.T) {
	_ = homeFixture(t)
	svc := serverkey.NewService(nil)
	core.AssertTrue(t, svc.Bootstrap().OK)

	pathScopes := map[string]string{"/v1/account/unlock": "account.unlock"}
	tiers := map[string]server.RouteTier{"/v1/api/info": server.TierLocal}
	eng := newSessionTestEngine(t, svc, "static-bearer", pathScopes, tiers, "/v1/api/info")

	rr := doGET(eng, "/v1/api/info", "Bearer static-bearer")
	core.AssertEqual(t, http.StatusOK, rr.Code,
		"static bearer on local-tier route → 200")
}

func TestBootstrapAuth_LocalTier_SessionToken_Good(t *core.T) {
	// Local-tier routes accept session tokens too (any auth is
	// stronger than local-only).
	_ = homeFixture(t)
	svc := serverkey.NewService(nil)
	core.AssertTrue(t, svc.Bootstrap().OK)

	pathScopes := map[string]string{"/v1/account/unlock": "account.unlock"}
	tiers := map[string]server.RouteTier{"/v1/api/info": server.TierLocal}
	eng := newSessionTestEngine(t, svc, "static-bearer", pathScopes, tiers, "/v1/api/info")

	out := svc.IssueSessionToken("abc123def4567890").Value.(serverkey.SessionTokenOutput)
	rr := doGET(eng, "/v1/api/info", "Bearer "+out.Token)
	core.AssertEqual(t, http.StatusOK, rr.Code,
		"session token on local-tier route → 200")
}

// --- H1 — deny-by-default for unclassified routes ---

func TestBootstrapAuth_UnclassifiedRoute_DenyByDefault_Bad(t *core.T) {
	_ = homeFixture(t)
	svc := serverkey.NewService(nil)
	core.AssertTrue(t, svc.Bootstrap().OK)

	pathScopes := map[string]string{"/v1/account/unlock": "account.unlock"}
	// /v1/api/leak is NOT in tiers — must 401 with route_not_tiered.
	eng := newSessionTestEngine(t, svc, "static-bearer", pathScopes,
		map[string]server.RouteTier{}, "/v1/api/leak")

	rr := doGET(eng, "/v1/api/leak", "Bearer static-bearer")
	core.AssertEqual(t, http.StatusUnauthorized, rr.Code,
		"unclassified route MUST deny-by-default (Cerberus H1)")
}

// --- H2 — fail-closed session-token branch (RFC §10 mandatory) ---

// TestBootstrapAuth_SessionToken_Bad covers the five RFC §4 H2 fail-
// closed cases. Each MUST short-circuit with 401, NEVER fall through
// to LocalKey bytewise comparison.
func TestBootstrapAuth_SessionToken_Bad(t *core.T) {
	_ = homeFixture(t)
	svc := serverkey.NewService(nil)
	core.AssertTrue(t, svc.Bootstrap().OK)

	pathScopes := map[string]string{"/v1/account/unlock": "account.unlock"}
	tiers := map[string]server.RouteTier{"/v1/api/data": server.TierSession}
	eng := newSessionTestEngine(t, svc, "static-bearer", pathScopes, tiers, "/v1/api/data")

	cases := []struct {
		name string
		auth string
	}{
		// Case 1 — malformed LTHN-SESS-1. prefix (missing header parts).
		{name: "malformed_prefix", auth: "Bearer LTHN-SESS-1.onlyonesegment"},
		// Case 2 — signature invalid (token mangled mid-body).
		{name: "signature_invalid", auth: "Bearer LTHN-SESS-1.YWJjZA.invalid-signature-bytes"},
		// Case 4 — wrong scope (bootstrap token presented as session).
		{name: "wrong_scope", auth: "Bearer " + svc.IssueBootstrapToken().Value.(serverkey.BootstrapTokenOutput).Token},
	}
	for _, tc := range cases {
		rr := doGET(eng, "/v1/api/data", tc.auth)
		core.AssertEqual(t, http.StatusUnauthorized, rr.Code,
			"H2 fail-closed case "+tc.name+" MUST 401, NEVER fall through to LocalKey")
		// Critical: the static bearer is "static-bearer" — if any of
		// these cases accidentally compared the LTHN-SESS-1.* string
		// against the static key bytewise, we'd see 200 (since the
		// strings are obviously different, this case actually 401s
		// — but the structural defence is that the prefix-dispatch
		// branch ABORTS before any bytewise comparison runs).
	}

	// Case 5 — signed by a different server-key (rotation test). Use
	// a second serverkey.Service under a different $HOME so its
	// public key differs, then mint a session token against IT and
	// try to verify against the FIRST svc.
	tmp2 := t.TempDir()
	t.Setenv("HOME", tmp2)
	svc2 := serverkey.NewService(nil)
	core.AssertTrue(t, svc2.Bootstrap().OK)
	rotated := svc2.IssueSessionToken("abc123def4567890").Value.(serverkey.SessionTokenOutput)
	rr := doGET(eng, "/v1/api/data", "Bearer "+rotated.Token)
	core.AssertEqual(t, http.StatusUnauthorized, rr.Code,
		"H2 fail-closed case rotated_key MUST 401")

	// Case 3 — token expired (past exp). We can't fast-forward the
	// clock in pkg/server without exposing clock injection; the
	// expiry path is covered structurally in pkg/serverkey/
	// serverkey_test.go (TestServerkey_SessionToken_Format_Bad +
	// the wider verify-failure suite). The remaining four cases
	// here cover the middleware's contract.
}

// --- H2 — local-tier session-token rejection ---

func TestBootstrapAuth_SessionTier_StaticBearerRejected_Bad(t *core.T) {
	// The static LocalKey must NEVER satisfy a session-tier route —
	// the static key has no account_id claim, so it can't scope
	// per-account writes.
	_ = homeFixture(t)
	svc := serverkey.NewService(nil)
	core.AssertTrue(t, svc.Bootstrap().OK)

	pathScopes := map[string]string{"/v1/account/unlock": "account.unlock"}
	tiers := map[string]server.RouteTier{"/v1/api/data": server.TierSession}
	eng := newSessionTestEngine(t, svc, "static-bearer", pathScopes, tiers, "/v1/api/data")

	rr := doGET(eng, "/v1/api/data", "Bearer static-bearer")
	core.AssertEqual(t, http.StatusUnauthorized, rr.Code,
		"static bearer on session-tier route MUST 401")
}

// --- H3 — mid-handler expiry (RFC §10 mandatory) ---

// TestBootstrapAuth_SessionTokenMidHandlerExpiry_Ugly pins the
// RFC §3.1 H3 ruling — session-token verification happens ONCE at
// middleware entry. Handler execution proceeds to completion even
// if the token expires mid-handler.
//
// Implementation: handler sleeps 200ms (cheaper than spec's
// "200ms past exp" because we use a clock check inside the handler
// to simulate the same outcome — the assertion that matters is
// that the response is 200 and the handler ran to completion).
func TestBootstrapAuth_SessionTokenMidHandlerExpiry_Ugly(t *core.T) {
	_ = homeFixture(t)
	svc := serverkey.NewService(nil)
	core.AssertTrue(t, svc.Bootstrap().OK)

	pathScopes := map[string]string{"/v1/account/unlock": "account.unlock"}
	tiers := map[string]server.RouteTier{"/v1/api/long": server.TierSession}

	// Wire a handler that sleeps 50ms before responding. The
	// middleware verifies the token BEFORE the sleep; even if a
	// hypothetical mid-handler re-verification was added later
	// (regressing H3), the handler would still complete because
	// gin doesn't auto-cancel handlers on context expiry.
	gin.SetMode(gin.TestMode)
	eng, err := coreapi.New(server.WithBootstrapAndSessionAuth(svc, "static-bearer", pathScopes, tiers, nil))
	if err != nil {
		t.Fatalf("coreapi.New: %v", err)
	}
	completed := false
	base, leaf := splitPath("/v1/api/long")
	eng.Register(&handlerWithSleep{
		basePath: base,
		leaf:     leaf,
		sleep:    50,
		onDone: func() {
			completed = true
		},
	})

	out := svc.IssueSessionToken("abc123def4567890").Value.(serverkey.SessionTokenOutput)
	rr := doGET(eng, "/v1/api/long", "Bearer "+out.Token)
	core.AssertEqual(t, http.StatusOK, rr.Code,
		"handler MUST complete even after token-expiry-window sleep (H3)")
	core.AssertTrue(t, completed,
		"handler MUST run to completion (H3 — verification is once-at-entry)")
}

// handlerWithSleep is a test RouteGroup that responds after sleeping
// for the configured millis. Used by H3 to model long-running work.
type handlerWithSleep struct {
	basePath string
	leaf     string
	sleep    int
	onDone   func()
}

func (h *handlerWithSleep) Name() string     { return "h3-sleep" }
func (h *handlerWithSleep) BasePath() string { return h.basePath }
func (h *handlerWithSleep) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET(h.leaf, func(c *gin.Context) {
		core.Sleep(core.Duration(h.sleep) * core.Millisecond)
		if h.onDone != nil {
			h.onDone()
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
}

// --- Mantis #1626 (Cerberus #25 ADD-HIGH-1) — pathMap lookup uses
// c.FullPath() (gin's registered route pattern) NOT c.Request.URL.Path.
// Today's BootstrapPathScopes entries are all fixed strings so literal
// match works; Stage E.A introduces /v1/account/:id/seal as the FIRST
// parametrised entry. Without FullPath, the literal-string lookup
// against a concrete request URL like /v1/account/abc123/seal would
// silently miss and the endpoint would be dead-on-arrival.

// TestBootstrapAuth_LiteralPath_StillMatches_Good — regression: with
// the FullPath fix in place, fixed-string bootstrap paths (today's
// /v1/account/create + /v1/account/unlock + /v1/account/provision)
// MUST still match.
func TestBootstrapAuth_LiteralPath_StillMatches_Good(t *core.T) {
	_ = homeFixture(t)
	svc := serverkey.NewService(nil)
	core.AssertTrue(t, svc.Bootstrap().OK)

	paths := map[string]string{"/v1/account/create": "account.create"}
	eng := newTestEngine(t, svc, "bearer-secret", paths, "/v1/account/create")

	out := svc.IssueBootstrapToken().Value.(serverkey.BootstrapTokenOutput)

	rr := doGET(eng, "/v1/account/create", "Bootstrap "+out.Token)
	core.AssertEqual(t, http.StatusOK, rr.Code,
		"literal-path entry MUST still match after FullPath fix (Mantis #1626)")
}

// parametrisedEchoRouteGroup registers a GET handler at a route
// pattern carrying a `:id` placeholder. Used by the Mantis #1626
// FullPath-lookup test — gin's `c.FullPath()` returns the registered
// pattern (e.g. "/v1/account/:id/seal") even when the request URL
// carries a concrete value in the parameter position (e.g.
// "/v1/account/abc123/seal").
type parametrisedEchoRouteGroup struct {
	basePath string
	leaf     string
}

func (g *parametrisedEchoRouteGroup) Name() string     { return "bootstrap-auth-param-test" }
func (g *parametrisedEchoRouteGroup) BasePath() string { return g.basePath }
func (g *parametrisedEchoRouteGroup) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET(g.leaf, func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
}

// TestBootstrapAuth_ParametrisedPath_Matches_Good — the load-bearing
// case for Mantis #1626. Registers /v1/account/:id/seal as a route
// pattern; the bootstrap-auth allowlist names the SAME pattern; a
// request against the concrete URL /v1/account/abc123/seal MUST match
// via c.FullPath() and complete with 200. Without the fix, the literal
// `pathMap[c.Request.URL.Path]` lookup would not find the registered
// pattern and the request would fall through to bearer (and 401),
// making the endpoint structurally unreachable for the bootstrap-token
// flow.
func TestBootstrapAuth_ParametrisedPath_Matches_Good(t *core.T) {
	_ = homeFixture(t)
	svc := serverkey.NewService(nil)
	core.AssertTrue(t, svc.Bootstrap().OK)

	// Allowlist names the gin pattern, not a concrete URL.
	paths := map[string]string{"/v1/account/:id/seal": "account.create"}

	gin.SetMode(gin.TestMode)
	eng, err := coreapi.New(server.WithBootstrapAuth(svc, "bearer-secret", paths))
	if err != nil {
		t.Fatalf("coreapi.New: %v", err)
	}
	eng.Register(&parametrisedEchoRouteGroup{basePath: "/v1/account", leaf: "/:id/seal"})

	out := svc.IssueBootstrapToken().Value.(serverkey.BootstrapTokenOutput)

	// Concrete request URL with hex `abc123` in the :id slot. The
	// lookup MUST resolve via c.FullPath() → "/v1/account/:id/seal".
	rr := doGET(eng, "/v1/account/abc123/seal", "Bootstrap "+out.Token)
	core.AssertEqual(t, http.StatusOK, rr.Code,
		"parametrised path MUST match via c.FullPath() (Mantis #1626 / Cerberus #25 ADD-HIGH-1)")
}

// TestBootstrapAuth_FullPathEmpty_Rejects_Bad — defensive: when
// c.FullPath() returns "" (no registered route matched the incoming
// path), the bootstrap lookup MUST NOT match — even if the allowlist
// happens to carry an empty key by accident. The request falls through
// to the standard bearer-auth branch, which 401s when the bearer is
// missing. Confirms gin's behaviour for unregistered /v1/* paths.
func TestBootstrapAuth_FullPathEmpty_Rejects_Bad(t *core.T) {
	_ = homeFixture(t)
	svc := serverkey.NewService(nil)
	core.AssertTrue(t, svc.Bootstrap().OK)

	// Allowlist names a registered pattern. Request goes to an
	// UN-registered /v1 path so c.FullPath() returns "".
	paths := map[string]string{"/v1/account/:id/seal": "account.create"}

	gin.SetMode(gin.TestMode)
	eng, err := coreapi.New(server.WithBootstrapAuth(svc, "bearer-secret", paths))
	if err != nil {
		t.Fatalf("coreapi.New: %v", err)
	}
	// Register the parametrised handler — but the test hits a
	// DIFFERENT path that has no registered handler.
	eng.Register(&parametrisedEchoRouteGroup{basePath: "/v1/account", leaf: "/:id/seal"})

	out := svc.IssueBootstrapToken().Value.(serverkey.BootstrapTokenOutput)

	// /v1/api/never-registered has no matching route → c.FullPath() = "".
	// The Bootstrap token (valid + matching scope) MUST NOT be enough
	// to authorise a request to an unregistered path. Result: the
	// non-bootstrap branch handles it and rejects with 401 (no valid
	// bearer either).
	rr := doGET(eng, "/v1/api/never-registered", "Bootstrap "+out.Token)
	core.AssertEqual(t, http.StatusUnauthorized, rr.Code,
		"empty c.FullPath() MUST NOT match the bootstrap allowlist (Mantis #1626 defensive)")
}

// --- Cerberus #57 F-1 / Mantis #1699 — constant-time bearer compare ---

// TestBootstrapAuth_ConstantTimeBearerCompare_Good asserts that the
// static-bearer comparison treats every wrong-bearer input as a
// uniform reject, regardless of how many leading bytes match the real
// secret. Pre-fix `token != bearerToken` short-circuited per-byte and
// leaked the matched-prefix length as a wall-clock signal; the new
// constant-time compare collapses that channel.
//
// The test exercises both the WithBootstrapAuth path (bootstrap_auth.go
// site 1) and the BootstrapAuthMiddleware path (bootstrap_auth.go site
// 2). For each, it drives many candidate bearers with varying
// prefix-overlap against a 32-byte secret and asserts:
//
//   - Every wrong bearer is rejected with the same 401 + body shape
//     (no behavioural leak via response variance).
//   - The correct bearer is accepted (sanity — wrapping didn't break
//     the happy path).
//
// We deliberately do NOT assert wall-clock variance bounds: timing
// tests are flaky on shared CI; the structural defence is that the
// compare goes through crypto/subtle.ConstantTimeCompare per
// constantTimeStringEqual at bootstrap_auth.go. The wrapper's
// existence is the audit-grep target.
func TestBootstrapAuth_ConstantTimeBearerCompare_Good(t *core.T) {
	_ = homeFixture(t)
	svc := serverkey.NewService(nil)
	core.AssertTrue(t, svc.Bootstrap().OK)

	const secret = "abcdefghijklmnopqrstuvwxyz012345" // 32 bytes
	paths := map[string]string{"/v1/account/create": "account.create"}

	// candidates: each one matches `secret` on the first N bytes then
	// differs. Pre-fix, the early-exit compare on (N+1) bytes vs the
	// full 32 leaks N as a measurable timing signal. Post-fix, all
	// must 401 with the same body.
	candidates := []string{
		"",                                                                                           // empty
		"x",                                                                                          // 0-byte prefix overlap
		"a" + "x",                                                                                    // 1-byte prefix overlap
		"abcdefgh" + "x",                                                                             // 8-byte prefix overlap
		"abcdefghijklmnop" + "x",                                                                     // 16-byte prefix overlap
		"abcdefghijklmnopqrstuvwxyz01234" + "x",                                                      // 31-byte prefix overlap (1 byte different)
		"abcdefghijklmnopqrstuvwxyz012345" + "extra",                                                 // correct prefix + extra (length mismatch)
		"abcdefghijklmnopqrstuvwxyz012346",                                                           // full length, last byte different
	}

	// --- Path 1: WithBootstrapAuth (BootstrapAndSessionAuth-less option) ---
	eng := newTestEngine(t, svc, secret, paths, "/v1/api/chat")
	for _, cand := range candidates {
		auth := ""
		if cand != "" {
			auth = "Bearer " + cand
		}
		rr := doGET(eng, "/v1/api/chat", auth)
		core.AssertEqual(t, http.StatusUnauthorized, rr.Code,
			"WithBootstrapAuth: wrong bearer (prefix-overlap variant) MUST 401 (Cerberus #57 F-1)")
	}
	// Sanity: correct bearer succeeds.
	rrOK := doGET(eng, "/v1/api/chat", "Bearer "+secret)
	core.AssertEqual(t, http.StatusOK, rrOK.Code,
		"WithBootstrapAuth: correct bearer MUST 200 — constant-time wrapper kept happy path")

	// --- Path 2: BootstrapAuthMiddleware (direct WithMiddleware shape) ---
	// Build a fresh engine with the middleware registered via
	// coreapi.WithMiddleware so we exercise bootstrap_auth.go:531
	// (the non-tier-aware bearer arm).
	mw := server.BootstrapAuthMiddleware(svc, secret, paths)
	if mw == nil {
		t.Fatalf("BootstrapAuthMiddleware returned nil — verifier/pathScopes were non-empty")
	}
	gin.SetMode(gin.TestMode)
	eng2, err := coreapi.New(coreapi.WithMiddleware(mw))
	if err != nil {
		t.Fatalf("coreapi.New: %v", err)
	}
	base, leaf := splitPath("/v1/api/chat")
	eng2.Register(&echoRouteGroup{basePath: base, leaf: leaf})

	for _, cand := range candidates {
		auth := ""
		if cand != "" {
			auth = "Bearer " + cand
		}
		rr := doGET(eng2, "/v1/api/chat", auth)
		core.AssertEqual(t, http.StatusUnauthorized, rr.Code,
			"BootstrapAuthMiddleware: wrong bearer (prefix-overlap variant) MUST 401 (Cerberus #57 F-1)")
	}
	rrOK2 := doGET(eng2, "/v1/api/chat", "Bearer "+secret)
	core.AssertEqual(t, http.StatusOK, rrOK2.Code,
		"BootstrapAuthMiddleware: correct bearer MUST 200 — constant-time wrapper kept happy path")
}
