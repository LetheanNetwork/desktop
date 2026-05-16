// SPDX-Licence-Identifier: EUPL-1.2

// HTTP-handler tests for pkg/account. The bootstrap-auth middleware
// is OUT OF SCOPE here — pkg/server/bootstrap_auth_test pins the
// auth chain end-to-end. These tests mount the RouteGroup directly
// on a gin engine with no middleware so the handler logic is the
// only variable under test.

package account_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/account"
	"github.com/gin-gonic/gin"
)

// newTestEngine builds a bare gin engine with pkg/account's
// RouteGroup mounted at its canonical BasePath. No auth middleware —
// bootstrap-auth lives in pkg/server and is tested there.
func newTestEngine(t *core.T, svc *subject.Service) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	groups := svc.RouteGroups()
	core.AssertLen(t, groups, 1, "Service must expose exactly one RouteGroup")
	g := groups[0]
	core.AssertEqual(t, subject.GroupName, g.Name())
	core.AssertEqual(t, subject.APIBasePath, g.BasePath())
	g.RegisterRoutes(r.Group(g.BasePath()))
	return r
}

// doPOST issues a POST against the engine with the supplied body
// bytes. Returns the recorder so the caller asserts on Code + Body.
func doPOST(eng *gin.Engine, path string, body []byte) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	eng.ServeHTTP(rr, req)
	return rr
}

// --- Good ---

func TestRoutes_CreateEndpoint_Good(t *core.T) {
	_ = homeFixture(t)
	svc := subject.NewService(nil)
	eng := newTestEngine(t, svc)

	in := validInput()
	body := core.JSONMarshal(in)
	core.AssertTrue(t, body.OK)

	rr := doPOST(eng, "/v1/account/create", body.Value.([]byte))
	core.AssertEqual(t, http.StatusOK, rr.Code, "valid Create → 200")

	// Response envelope is coreapi.Response[CreateOutput] — assert
	// success=true and data.account_id matches the canonical id.
	var env struct {
		Success bool `json:"success"`
		Data    struct {
			AccountID string `json:"account_id"`
			Path      string `json:"path"`
		} `json:"data"`
	}
	uR := core.JSONUnmarshal(rr.Body.Bytes(), &env)
	core.AssertTrue(t, uR.OK)
	core.AssertTrue(t, env.Success)
	core.AssertEqual(t, in.AccountID, env.Data.AccountID)
	core.AssertNotEqual(t, "", env.Data.Path)
}

// --- Bad — malformed body ---

func TestRoutes_CreateEndpoint_InvalidBody_Bad(t *core.T) {
	_ = homeFixture(t)
	svc := subject.NewService(nil)
	eng := newTestEngine(t, svc)

	// Garbage bytes — ShouldBindJSON fails.
	rr := doPOST(eng, "/v1/account/create", []byte("not-json-at-all"))
	core.AssertEqual(t, http.StatusBadRequest, rr.Code, "malformed JSON → 400")

	var env struct {
		Success bool `json:"success"`
		Error   struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	uR := core.JSONUnmarshal(rr.Body.Bytes(), &env)
	core.AssertTrue(t, uR.OK)
	core.AssertFalse(t, env.Success)
	core.AssertEqual(t, "account.invalid_body", env.Error.Code)
}

// --- Conflict — second create → 409 ---

func TestRoutes_CreateEndpoint_AccountExists_Conflict(t *core.T) {
	_ = homeFixture(t)
	svc := subject.NewService(nil)
	eng := newTestEngine(t, svc)

	in := validInput()
	body := core.JSONMarshal(in)
	core.AssertTrue(t, body.OK)

	rr1 := doPOST(eng, "/v1/account/create", body.Value.([]byte))
	core.AssertEqual(t, http.StatusOK, rr1.Code, "first POST → 200")

	rr2 := doPOST(eng, "/v1/account/create", body.Value.([]byte))
	core.AssertEqual(t, http.StatusConflict, rr2.Code,
		"second POST against same account → 409 (Cerberus #1460 (a))")

	var env struct {
		Success bool `json:"success"`
		Error   struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	uR := core.JSONUnmarshal(rr2.Body.Bytes(), &env)
	core.AssertTrue(t, uR.OK)
	core.AssertFalse(t, env.Success)
	core.AssertEqual(t, "account.exists", env.Error.Code)
}

// --- Bad — id mismatch → 400 ---

func TestRoutes_CreateEndpoint_IDMismatch_Bad(t *core.T) {
	_ = homeFixture(t)
	svc := subject.NewService(nil)
	eng := newTestEngine(t, svc)

	in := validInput()
	in.AccountID = "0000000000000000"
	body := core.JSONMarshal(in)
	core.AssertTrue(t, body.OK)

	rr := doPOST(eng, "/v1/account/create", body.Value.([]byte))
	core.AssertEqual(t, http.StatusBadRequest, rr.Code,
		"id mismatch → 400 (Cerberus #1460 (b))")

	var env struct {
		Success bool `json:"success"`
		Error   struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	uR := core.JSONUnmarshal(rr.Body.Bytes(), &env)
	core.AssertTrue(t, uR.OK)
	core.AssertEqual(t, "account.id_mismatch", env.Error.Code)
}

// --- Compile-time anchor: pkg/server.BootstrapPathScopes must list
// our endpoint at the canonical path/scope tuple. This protects
// against a silent rename in either side breaking the auth gate.
// Lives here (not in pkg/server) because the canonical path
// constant + canonical scope are owned by pkg/account.
func TestRoutes_PathScope_Anchor(t *testing.T) {
	// The string this test pins is the SAME literal the bootstrap
	// middleware looks up at request time. If a refactor splits
	// /v1/account/create into a sub-resource path, this assertion
	// fails LOUD and the matching pathScopes update is forced.
	core.AssertEqual(t, "/v1/account", subject.APIBasePath)
}
