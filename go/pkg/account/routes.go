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

	core "dappco.re/go"
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

// RegisterRoutes satisfies coreapi.RouteGroup. Stage E.B adds POST
// /unlock + POST /lock alongside the existing /create surface. Stage
// E.A adds PUT /:id/seal — the marker-replacement endpoint that closes
// the seal-step gap left dangling by the legacy create+seal flow
// (RFC.stage-e-seal.md §1). Each new path lands a corresponding
// pathScopes entry in pkg/server (every new path is a Mantis ticket +
// Cerberus DREAD review per #1467).
//
// Usage example (inside server.NewService):
//
//	for _, g := range provider.RouteGroups() {
//	    engine.Register(g)
//	}
func (g *routesProvider) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/create", g.handleCreate)
	rg.POST("/provision", g.handleProvision)
	rg.POST("/unlock", g.handleUnlock)
	rg.POST("/lock", g.handleLock)
	// Stage E.A per plans/code/lthn/desktop/auth-gate/RFC.stage-e-seal.md.
	// PUT (not POST) per REST idempotency convention — repeated identical
	// body MUST produce the same on-disk state per §2.1 / §3.3.
	// Bootstrap-tier, scope account.seal (RFC §2.2). The route pattern
	// "/:id/seal" is the first parametrised entry in BootstrapPathScopes;
	// the c.FullPath() lookup substrate landed at Mantis #1626 / commit
	// d4fdcca on dev (Cerberus #25 ADD-HIGH-1 closure).
	rg.PUT("/:id/seal", g.handleSeal)
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

	// Cerberus #1524 — server generates the audit RequestID. Any
	// caller-supplied input.RequestID is dropped to prevent forensic
	// deniability (an attacker forging arbitrary id values to muddy
	// the audit JOIN). Mirrors the same discipline handleUnlock +
	// handleProvision apply per Cerberus #1511. The server-side id is
	// echoed in the response X-Request-Id header so the legitimate
	// caller can still correlate their request to the audit log.
	// Create has no audit emission today; the override still lands so
	// the contract is consistent across every bootstrap-auth endpoint
	// and stays correct the moment audit-on-create ships.
	srvReqID := serverRequestID()
	input.RequestID = srvReqID
	c.Header("X-Request-Id", srvReqID)

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

// handleProvision is the gin handler for POST /v1/account/provision.
// Reads ProvisionInput, defers keygen + encrypt + atomic-write to
// Service.Provision per RFC.stage-x.md §3, and shapes the response.
//
// Auth: bootstrap-auth middleware verifies LTHN-BOOT-1.* token with
// scope=account.provision BEFORE this handler runs — by the time we
// land the nonce is already burned (replay defence holds on partial
// failure per Cerberus #1460 (d) discipline applied here).
//
// HTTP status mapping per RFC.stage-x.md §3 "Failure codes":
//
//   - account.invalid_body                   → 400 Bad Request
//   - account.provision.passphrase.required  → 400 Bad Request
//   - account.exists                         → 409 Conflict
//   - account.provision.keygen_failed        → 500 Internal Server Error
//   - account.provision.encrypt_failed       → 500 Internal Server Error
//   - account.provision.session_mint_failed  → 500 Internal Server Error
//   - account.provision.server_misconfigured → 500 Internal Server Error
//   - account.write_failed                   → 500 Internal Server Error
func (g *routesProvider) handleProvision(c *gin.Context) {
	var input ProvisionInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, coreapi.Fail(codeAccountInvalidBody, err.Error()))
		return
	}

	// Cerberus #1511 — server generates the audit RequestID. Any
	// caller-supplied input.RequestID is dropped to prevent forensic
	// deniability (an attacker forging arbitrary id values to muddy
	// the audit JOIN). The server-side id is echoed in the response
	// X-Request-Id header so the legitimate caller can still correlate
	// their request to the audit log. Phase 2c idempotency dedup (if
	// it ever lands) MUST design a separate ClientIdempotencyKey field;
	// the audit RequestID stays server-authoritative.
	srvReqID := serverRequestID()
	input.RequestID = srvReqID
	c.Header("X-Request-Id", srvReqID)

	r := g.svc.Provision(input)
	if r.OK {
		out, _ := r.Value.(ProvisionOutput)
		c.JSON(http.StatusOK, coreapi.OK(out))
		return
	}

	code := r.Code()
	if code == "" {
		code = codeAccountWriteFailed
	}
	c.JSON(statusForCode(code), coreapi.Fail(code, r.Error()))
}

// handleUnlock is the gin handler for POST /v1/account/unlock.
// Reads UnlockInput from the request body, defers the actual
// passphrase verification + session-token mint to Service.Unlock,
// and shapes the HTTP response.
//
// Auth: bootstrap-auth middleware (pkg/server/bootstrap_auth.go)
// runs BEFORE this handler — it verifies the LTHN-BOOT-1.* token
// with scope=account.unlock, consumes the nonce, then dispatches.
// By the time this handler runs:
//
//   - the token signature has been verified
//   - the nonce has been added to consumed-set (Cerberus #1463)
//   - c.Get("auth_via") == "bootstrap"
//
// HTTP status mapping per RFC §2.2 "Failure codes":
//
//   - account.invalid_body                 → 400 Bad Request
//   - account.id.required                  → 400 Bad Request
//   - account.unlock.passphrase.required   → 400 Bad Request
//   - account.unlock.bad_passphrase        → 401 Unauthorized
//   - account.unlock.locked_out            → 429 Too Many Requests
//   - account.unlock.corrupted_key         → 500 Internal Server Error
//   - account.unlock.server_misconfigured  → 500 Internal Server Error
func (g *routesProvider) handleUnlock(c *gin.Context) {
	var input UnlockInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, coreapi.Fail(codeAccountInvalidBody, err.Error()))
		return
	}

	// Cerberus #1511 — server generates the audit RequestID. Both
	// the caller's body field AND the X-Request-Id header are dropped
	// (both are attacker-controlled in a hostile-plugin scenario;
	// trusting either lets an attacker forge an arbitrary audit
	// JOIN key). The server's UUID v4 is echoed in the response
	// X-Request-Id header so the legitimate caller can correlate.
	// Supersedes the original Cerberus #1711 Phase 2.5 wiring (which
	// took caller's request_id verbatim).
	srvReqID := serverRequestID()
	input.RequestID = srvReqID
	c.Header("X-Request-Id", srvReqID)

	r := g.svc.Unlock(input)
	if r.OK {
		out, _ := r.Value.(UnlockOutput)
		c.JSON(http.StatusOK, coreapi.OK(out))
		return
	}

	code := r.Code()
	if code == "" {
		// Defensive — Unlock wraps every domain error with NewCode.
		// An empty code means an unwrapped internal failure leaked
		// out — surface as a 500.
		code = codeUnlockCorruptedKey
	}
	c.JSON(statusForCode(code), coreapi.Fail(code, r.Error()))
}

// handleLock is the gin handler for POST /v1/account/lock. Forced
// lock — clears the in-process unlocked private key for the
// supplied account_id so the next REST request without a valid
// session token re-arms the auth gate.
//
// Auth: per RFC §6 + §4 routeTiers — this endpoint is session-tier
// (a user signing themselves out must already be unlocked). The
// bearer middleware verifies the LTHN-SESS-1.* token and writes
// the verified account_id to c.Set("account_id", …) via
// pkg/server/bootstrap_auth.go ~line 356. This handler MUST bind
// account_id from the session context — NOT from the request body
// — so an attacker holding a session token for account A cannot
// force-lock account B by submitting {"account_id":"B"}.
//
// Cerberus #18 — session-tier handlers bind authoritative identity
// fields from session context, never body. Body account_id is still
// accepted for backwards-compat but MUST match the session-bound id
// (mismatch → 400 account_id.mismatch); when omitted, the session
// id is used as the canonical source. See Mantis #1587.
//
// HTTP status mapping:
//
//   - session.unbound       → 401 Unauthorized (no session in context)
//   - account.invalid_body  → 400 Bad Request
//   - account_id.mismatch   → 400 Bad Request (body ≠ session)
//   - account.id.required   → 400 Bad Request
//   - success               → 200 OK
func (g *routesProvider) handleLock(c *gin.Context) {
	// Cerberus #18 / Mantis #1587 — authoritative account_id comes
	// from the verified session, not the request body. The bearer
	// middleware (pkg/server/bootstrap_auth.go) writes account_id
	// after VerifySessionToken returns OK; if it's empty here, the
	// middleware was not applied or the session-tier classification
	// in pkg/server is wrong. Fail-closed with 401 rather than
	// silently locking an attacker-chosen body field.
	sessionAccountID := c.GetString("account_id")
	if sessionAccountID == "" {
		c.JSON(http.StatusUnauthorized,
			coreapi.Fail("session.unbound", "session missing account_id"))
		return
	}

	var input LockInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, coreapi.Fail(codeAccountInvalidBody, err.Error()))
		return
	}

	// Body account_id is permitted (legacy clients) but MUST match
	// the session-bound id when present. A non-matching body field
	// is an attempt to force-lock a different account on the same
	// connection — reject with 400 so the caller sees the contract
	// violation rather than a silent body-ignored success.
	if input.AccountID != "" && input.AccountID != sessionAccountID {
		c.JSON(http.StatusBadRequest,
			coreapi.Fail("account_id.mismatch",
				"body account_id does not match session"))
		return
	}
	// Canonical source — overwrite the (possibly empty, possibly
	// matching) body field with the session-bound id before handing
	// off to Service.Lock.
	input.AccountID = sessionAccountID

	r := g.svc.Lock(input)
	if r.OK {
		out, _ := r.Value.(LockOutput)
		c.JSON(http.StatusOK, coreapi.OK(out))
		return
	}

	code := r.Code()
	if code == "" {
		code = codeAccountWriteFailed
	}
	c.JSON(statusForCode(code), coreapi.Fail(code, r.Error()))
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
		codeAccountIDMismatch,
		codeUnlockPassphraseRequired,
		codeProvisionPassphraseRequired:
		return http.StatusBadRequest
	case codeAccountExists:
		return http.StatusConflict
	case codeUnlockBadPassphrase:
		return http.StatusUnauthorized
	case codeUnlockLockedOut:
		return http.StatusTooManyRequests
	}
	return http.StatusInternalServerError
}

// serverRequestID generates a server-side UUID v4 used as the
// authoritative audit RequestID for /v1/account/{unlock,provision}
// handlers per Cerberus #1511. The caller's body field +
// X-Request-Id header are dropped at the handler boundary to prevent
// forensic deniability (an attacker forging arbitrary values to
// confuse the audit JOIN). The server's UUID is echoed in the
// response X-Request-Id header so legitimate callers can still
// correlate their request to the audit log.
//
// On RandomBytes failure returns the empty string; downstream
// IssueSessionTokenWithRequest is tolerant of empty (just omits the
// audit field). NOT a panic-worthy condition.
//
// CoreGO gap: core.UUIDv4 doesn't exist yet (logged at
// project_corego_export_gaps). This helper is the local stand-in
// until the export lands; the call-site swap is a single import +
// rename when CoreGO ships it.
//
// Usage example:
//
//	srvReqID := serverRequestID()
//	input.RequestID = srvReqID
//	c.Header("X-Request-Id", srvReqID)
func serverRequestID() string {
	r := core.RandomBytes(16)
	if !r.OK {
		return ""
	}
	b, ok := r.Value.([]byte)
	if !ok || len(b) != 16 {
		return ""
	}
	// RFC 4122 §4.4 — version 4 (random) UUID. Top nibble of
	// time_hi_and_version (byte 6) = 0100 = 4. Top two bits of
	// clock_seq_hi_and_reserved (byte 8) = 10 (variant 1).
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return core.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
