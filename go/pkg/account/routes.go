// SPDX-Licence-Identifier: EUPL-1.2

// routes.go — REST surface for pkg/account. Implements the
// coreapi.RouteGroup interface so pkg/server's auto-discovery loop
// picks it up via the RoutesProvider contract (server.Service.NewService
// iterates registered Core services and mounts every group their
// RouteGroups() returns).
//
// Stage B' wiring (Mantis #1474 successor): the POST /v1/account/create
// endpoint is the consumer of bootstrap-tokens minted by pkg/serverkey.
// pkg/server.BootstrapAuthMiddleware (Stage B) gates the path with
// scope=account.create — by the time the handler below runs, the
// nonce has been burned and the signature has been verified. The
// handler ONLY enforces the four #1460 MUST-NOTs (Service.Create
// owns those) and shapes the HTTP response.

package account

import (
	"net/http"

	coreapi "dappco.re/go/api"
	"github.com/gin-gonic/gin"
)

// APIBasePath is the canonical lthn mount for the account REST
// surface. Lives under /v1/account/ (NOT /v1/api/account) so the
// path the bootstrap-auth middleware sees matches the RFC §4
// pathScopes map verbatim:
//
//	map[string]string{"/v1/account/create": "account.create"}
//
// Any change here MUST be made in lockstep with the pathScopes map
// in pkg/server.NewService — Cerberus #1467 path/scope lockstep
// rejects a token minted for a different path's scope.
const APIBasePath = "/v1/account"

// GroupName is the RouteGroup identifier the routes provider
// publishes via RouteGroup.Name(). Stable + grep-able so future
// auto-discovery filters (e.g. WebView-only gating) can identify
// the group unambiguously.
//
// Usage example:
//
//	for _, g := range provider.RouteGroups() {
//	    if g.Name() == account.GroupName { /* … */ }
//	}
const GroupName = "lthn-account"

// routesProvider satisfies coreapi.RouteGroup so pkg/server's
// auto-discovery mounts the handler. Held private — callers consume
// it through Service.RouteGroups() which is the
// pkg/server.RoutesProvider entry point.
type routesProvider struct {
	svc *Service
}

// Name satisfies coreapi.RouteGroup.
func (g *routesProvider) Name() string { return GroupName }

// BasePath satisfies coreapi.RouteGroup. All account routes mount
// under /v1/account/.
func (g *routesProvider) BasePath() string { return APIBasePath }

// RegisterRoutes satisfies coreapi.RouteGroup. Today's surface is
// a single POST /create — future endpoints (PUT /<id>/seal,
// DELETE /<id>) land here, each with a corresponding pathScopes
// entry in pkg/server (every new path is a Mantis ticket + Cerberus
// DREAD review per #1467).
//
// Usage example (inside server.NewService):
//
//	for _, g := range provider.RouteGroups() {
//	    engine.Register(g)
//	}
func (g *routesProvider) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/create", g.handleCreate)
}

// RouteGroups satisfies pkg/server.RoutesProvider — returns the
// single REST group this service publishes. pkg/server's
// auto-discovery loop calls this for every Core-registered service
// that implements the interface, then mounts each returned group
// on the public coreapi.Engine.
//
// Usage example:
//
//	// pkg/server iterates Core services + calls this automatically.
//	for _, g := range svc.RouteGroups() {
//	    engine.Register(g)
//	}
func (s *Service) RouteGroups() []coreapi.RouteGroup {
	return []coreapi.RouteGroup{&routesProvider{svc: s}}
}

// handleCreate is the gin handler for POST /v1/account/create.
// Reads CreateInput from the request body, defers the four #1460
// MUST-NOTs to Service.Create, and shapes the HTTP response.
//
// Auth: the bootstrap-auth middleware (pkg/server/bootstrap_auth.go)
// runs BEFORE this handler — it verifies the LTHN-BOOT-1.* token in
// the Authorization header, checks scope == "account.create", and
// consumes the nonce on success. By the time this handler runs:
//
//   - the token signature has been verified
//   - the nonce has been added to the consumed-set (Cerberus #1463)
//   - c.Get("auth_via") == "bootstrap" (middleware sets this)
//
// Cerberus #1460 (d) — nonce-consumption-before-write holds even on
// failure: if Service.Create returns an error after the middleware
// burned the nonce, the nonce is still consumed (replay defence is
// independent of write success). Do NOT add a duplicate burn here.
//
// Response shape mirrors coreapi.Response[T] envelope — on success
// the Data field carries CreateOutput; on failure Error carries the
// code from the "account.*" namespace + a human-readable message.
//
// HTTP status mapping:
//
//   - account.invalid_body       → 400 Bad Request
//   - account.public_key.required → 400 Bad Request
//   - account.id.required        → 400 Bad Request
//   - account.id_mismatch        → 400 Bad Request
//   - account.exists             → 409 Conflict
//   - account.write_failed       → 500 Internal Server Error
//   - any other (unexpected)     → 500 Internal Server Error
func (g *routesProvider) handleCreate(c *gin.Context) {
	var input CreateInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, coreapi.Fail(codeAccountInvalidBody, err.Error()))
		return
	}

	r := g.svc.Create(input)
	if r.OK {
		out, _ := r.Value.(CreateOutput)
		c.JSON(http.StatusOK, coreapi.OK(out))
		return
	}

	// Map the core error code → HTTP status. The code/message are
	// surfaced verbatim so the frontend can branch on a stable
	// machine-readable code (per RFC §1 "trigger source" — the lit
	// auth-gate keys "account exists" UI off the code).
	code := r.Code()
	if code == "" {
		// Service.Create wraps every account-domain error with NewCode;
		// an empty code means an unexpected internal failure (e.g.
		// paths.Root) flowed through unwrapped — surface as a 500.
		code = codeAccountWriteFailed
	}
	msg := r.Error()
	status := statusForCode(code)
	c.JSON(status, coreapi.Fail(code, msg))
}

// statusForCode maps an "account.*" error code to its HTTP status.
// Kept as a small switch so additions are obvious + LSP-discoverable;
// codes outside the known set default to 500 so an unmapped failure
// surfaces as a server error rather than masquerading as a client
// error.
//
// Usage example:
//
//	c.JSON(statusForCode("account.exists"), coreapi.Fail("account.exists", "…"))
func statusForCode(code string) int {
	switch code {
	case codeAccountInvalidBody,
		codePublicKeyRequired,
		codeAccountIDRequired,
		codeAccountIDMismatch:
		return http.StatusBadRequest
	case codeAccountExists:
		return http.StatusConflict
	}
	return http.StatusInternalServerError
}
