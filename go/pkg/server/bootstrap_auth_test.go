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

// silence the testing import.
var _ = testing.Short
