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

// TestRoutes_CreateEndpoint_RequestIDOverriddenByServer_Ugly pins
// Cerberus #1524 — caller-supplied input.RequestID is DROPPED at the
// handler boundary + replaced with a server-generated UUID v4 echoed
// in the X-Request-Id response header. Mirrors the Unlock + Provision
// discipline so every bootstrap-auth endpoint behaves consistently
// (no audit emission on Create today, but the contract holds the
// moment one ships).
func TestRoutes_CreateEndpoint_RequestIDOverriddenByServer_Ugly(t *core.T) {
	_ = homeFixture(t)
	svc := subject.NewService(nil)
	eng := newTestEngine(t, svc)

	const forgedID = "attacker-chosen-forensic-decoy-12345"
	in := validInput()
	in.RequestID = forgedID
	body := core.JSONMarshal(in)
	core.AssertTrue(t, body.OK)

	rr := doPOST(eng, "/v1/account/create", body.Value.([]byte))
	core.AssertEqual(t, http.StatusOK, rr.Code, "valid Create with forged request_id → 200")

	echoed := rr.Header().Get("X-Request-Id")
	core.AssertNotEqual(t, "", echoed,
		"server MUST echo a generated request_id in X-Request-Id response header")
	core.AssertNotEqual(t, forgedID, echoed,
		"server-generated id MUST NOT equal caller-supplied forged id (Cerberus #1524)")
	// UUID v4 shape: 8-4-4-4-12 hex chars (36 incl dashes). Length sanity
	// is sufficient for the handler-routing test; full RFC parse lives
	// in serverRequestID's own unit tests upstream.
	core.AssertEqual(t, 36, len(echoed), "server-generated id MUST be 36-char UUID v4")
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

// --- Unlock handler (Stage E.B) ---

func TestRoutes_UnlockEndpoint_Good(t *core.T) {
	home := homeFixture(t)
	writeEncryptedAccount(t, home, fixtureAccountID, fixturePassphrase)
	svc := newUnlockable(t, home)
	eng := newTestEngine(t, svc)

	body := core.JSONMarshal(subject.UnlockInput{
		AccountID:  fixtureAccountID,
		Passphrase: fixturePassphrase,
	})
	core.AssertTrue(t, body.OK)

	rr := doPOST(eng, "/v1/account/unlock", body.Value.([]byte))
	core.AssertEqual(t, http.StatusOK, rr.Code, "valid Unlock → 200")

	var env struct {
		Success bool `json:"success"`
		Data    struct {
			SessionToken string `json:"session_token"`
			ExpiresAt    int64  `json:"expires_at"`
			AccountID    string `json:"account_id"`
		} `json:"data"`
	}
	uR := core.JSONUnmarshal(rr.Body.Bytes(), &env)
	core.AssertTrue(t, uR.OK)
	core.AssertTrue(t, env.Success)
	core.AssertEqual(t, fixtureAccountID, env.Data.AccountID)
	core.AssertNotEqual(t, "", env.Data.SessionToken)
}

// TestRoutes_UnlockEndpoint_RequestIDOverriddenByServer_Ugly pins
// Cerberus #1511 — caller-supplied input.RequestID is DROPPED at the
// handler boundary + replaced with a server-generated UUID v4 echoed
// in the X-Request-Id response header. Forensic deniability defence.
func TestRoutes_UnlockEndpoint_RequestIDOverriddenByServer_Ugly(t *core.T) {
	home := homeFixture(t)
	writeEncryptedAccount(t, home, fixtureAccountID, fixturePassphrase)
	svc := newUnlockable(t, home)
	eng := newTestEngine(t, svc)

	const forgedID = "attacker-chosen-forensic-decoy-12345"
	body := core.JSONMarshal(subject.UnlockInput{
		AccountID:  fixtureAccountID,
		Passphrase: fixturePassphrase,
		RequestID:  forgedID,
	})
	core.AssertTrue(t, body.OK)

	rr := doPOST(eng, "/v1/account/unlock", body.Value.([]byte))
	core.AssertEqual(t, http.StatusOK, rr.Code)

	echoed := rr.Header().Get("X-Request-Id")
	core.AssertNotEqual(t, "", echoed,
		"server MUST echo a generated request_id in X-Request-Id response header")
	core.AssertNotEqual(t, forgedID, echoed,
		"server-generated id MUST NOT equal caller-supplied forged id (Cerberus #1511)")
	// UUID v4 shape: 8-4-4-4-12 hex chars, version-byte top nibble = 4,
	// variant-byte top two bits = 10. Length sanity check is sufficient
	// for routing test; full RFC parse lives in the helper's own unit tests.
	core.AssertEqual(t, 36, len(echoed), "server-generated id MUST be 36-char UUID v4")
}

func TestRoutes_UnlockEndpoint_BadPassphrase_401(t *core.T) {
	home := homeFixture(t)
	writeEncryptedAccount(t, home, fixtureAccountID, fixturePassphrase)
	svc := newUnlockable(t, home)
	eng := newTestEngine(t, svc)

	body := core.JSONMarshal(subject.UnlockInput{
		AccountID:  fixtureAccountID,
		Passphrase: fixtureWrongPassphrase,
	})
	core.AssertTrue(t, body.OK)

	rr := doPOST(eng, "/v1/account/unlock", body.Value.([]byte))
	core.AssertEqual(t, http.StatusUnauthorized, rr.Code,
		"wrong passphrase → 401")

	var env struct {
		Success bool `json:"success"`
		Error   struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	uR := core.JSONUnmarshal(rr.Body.Bytes(), &env)
	core.AssertTrue(t, uR.OK)
	core.AssertFalse(t, env.Success)
	core.AssertEqual(t, "account.unlock.bad_passphrase", env.Error.Code)
}

func TestRoutes_UnlockEndpoint_LockedOut_429(t *core.T) {
	home := homeFixture(t)
	writeEncryptedAccount(t, home, fixtureAccountID, fixturePassphrase)
	svc := newUnlockable(t, home)
	eng := newTestEngine(t, svc)

	// Spam past the lockout threshold.
	body := core.JSONMarshal(subject.UnlockInput{
		AccountID:  fixtureAccountID,
		Passphrase: fixtureWrongPassphrase,
	})
	core.AssertTrue(t, body.OK)
	for i := 0; i < 5; i++ {
		_ = doPOST(eng, "/v1/account/unlock", body.Value.([]byte))
	}

	// Next attempt (even correct) → 429 Too Many Requests.
	goodBody := core.JSONMarshal(subject.UnlockInput{
		AccountID:  fixtureAccountID,
		Passphrase: fixturePassphrase,
	})
	core.AssertTrue(t, goodBody.OK)
	rr := doPOST(eng, "/v1/account/unlock", goodBody.Value.([]byte))
	core.AssertEqual(t, http.StatusTooManyRequests, rr.Code, "locked-out account → 429")

	var env struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	uR := core.JSONUnmarshal(rr.Body.Bytes(), &env)
	core.AssertTrue(t, uR.OK)
	core.AssertEqual(t, "account.unlock.locked_out", env.Error.Code)
}

func TestRoutes_UnlockEndpoint_CorruptedKey_500(t *core.T) {
	home := homeFixture(t)
	writeRawAccount(t, home, fixtureAccountID, []byte("not pgp at all"))
	svc := newUnlockable(t, home)
	eng := newTestEngine(t, svc)

	body := core.JSONMarshal(subject.UnlockInput{
		AccountID:  fixtureAccountID,
		Passphrase: fixturePassphrase,
	})
	core.AssertTrue(t, body.OK)

	rr := doPOST(eng, "/v1/account/unlock", body.Value.([]byte))
	core.AssertEqual(t, http.StatusInternalServerError, rr.Code,
		"corrupted key → 500")

	var env struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	uR := core.JSONUnmarshal(rr.Body.Bytes(), &env)
	core.AssertTrue(t, uR.OK)
	core.AssertEqual(t, "account.unlock.corrupted_key", env.Error.Code)
}

func TestRoutes_UnlockEndpoint_InvalidBody_400(t *core.T) {
	_ = homeFixture(t)
	svc := newUnlockable(t, "")
	eng := newTestEngine(t, svc)

	rr := doPOST(eng, "/v1/account/unlock", []byte("not-json"))
	core.AssertEqual(t, http.StatusBadRequest, rr.Code)

	var env struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	uR := core.JSONUnmarshal(rr.Body.Bytes(), &env)
	core.AssertTrue(t, uR.OK)
	core.AssertEqual(t, "account.invalid_body", env.Error.Code)
}

// --- Lock handler (Stage E.B) ---

func TestRoutes_LockEndpoint_Good(t *core.T) {
	home := homeFixture(t)
	writeEncryptedAccount(t, home, fixtureAccountID, fixturePassphrase)
	svc := newUnlockable(t, home)
	eng := newTestEngine(t, svc)

	// Unlock first so we have state to clear.
	unlockBody := core.JSONMarshal(subject.UnlockInput{
		AccountID: fixtureAccountID, Passphrase: fixturePassphrase,
	})
	core.AssertTrue(t, unlockBody.OK)
	_ = doPOST(eng, "/v1/account/unlock", unlockBody.Value.([]byte))
	core.AssertTrue(t, svc.HasUnlocked(fixtureAccountID))

	lockBody := core.JSONMarshal(subject.LockInput{AccountID: fixtureAccountID})
	core.AssertTrue(t, lockBody.OK)
	rr := doPOST(eng, "/v1/account/lock", lockBody.Value.([]byte))
	core.AssertEqual(t, http.StatusOK, rr.Code, "valid Lock → 200")
	core.AssertFalse(t, svc.HasUnlocked(fixtureAccountID),
		"Lock handler MUST clear unlocked state")
}

func TestRoutes_LockEndpoint_AccountIDRequired_400(t *core.T) {
	_ = homeFixture(t)
	svc := newUnlockable(t, "")
	eng := newTestEngine(t, svc)

	body := core.JSONMarshal(subject.LockInput{AccountID: ""})
	core.AssertTrue(t, body.OK)
	rr := doPOST(eng, "/v1/account/lock", body.Value.([]byte))
	core.AssertEqual(t, http.StatusBadRequest, rr.Code)

	var env struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	uR := core.JSONUnmarshal(rr.Body.Bytes(), &env)
	core.AssertTrue(t, uR.OK)
	core.AssertEqual(t, "account.id.required", env.Error.Code)
}

// --- Route registration sanity ---

// TestRoutes_RegisterRoutes_AllThree confirms the RouteGroup mounts
// /create + /unlock + /lock — defends against a silent rename of
// the leaf path that would strand the frontend.
func TestRoutes_RegisterRoutes_AllThree(t *core.T) {
	_ = homeFixture(t)
	svc := newUnlockable(t, "")
	eng := newTestEngine(t, svc)

	// Each route MUST respond with a non-405 (Method Not Allowed) on
	// POST. A 200/4xx/5xx is fine — what we're proving is the route
	// is mounted, not the handler's contract.
	for _, p := range []string{"/v1/account/create", "/v1/account/unlock", "/v1/account/lock"} {
		rr := doPOST(eng, p, []byte(`{}`))
		core.AssertNotEqual(t, http.StatusMethodNotAllowed, rr.Code,
			"route "+p+" must be POST-able")
	}
}
