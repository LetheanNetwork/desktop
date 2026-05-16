// SPDX-Licence-Identifier: EUPL-1.2

// bootstrap_auth.go — Stage B of the first-run auth-gate sequence
// (Mantis #1474, plans RFC v2 at code/lthn/desktop/auth-gate/RFC.md
// §4). Adds a Gin middleware that gates the account-creation endpoint
// family with the short-lived PGP-signed bootstrap-token issued by
// serverkey.Service.IssueBootstrapToken.
//
// The middleware composes WITH the standard bearer-auth: for any path
// listed in pathScopes it REPLACES the bearer requirement with a
// bootstrap-token requirement; for every other path it delegates to
// bearer-auth verbatim. The existing skip-list (health, swagger,
// openapi) is preserved.
//
// IMPORTANT — pkg/server.WithBootstrapAuth is the ONLY supported entry
// point. We do NOT edit external/api/middleware.go; the bearer skip
// mechanism we'd want to reach is hardcoded inside it, so we wrap the
// combined responsibility in a single middleware here. When this
// option is supplied, callers MUST NOT also append coreapi.WithBearerAuth
// — the combined middleware handles both sides.
//
// Cerberus #1467 path/scope lockstep — pathScopes is map[path]scope,
// NOT []path + single scope. A bootstrap token minted with scope=X
// cannot satisfy a different path's scope, even if both are in the
// allowlist. Adding a new path here REQUIRES a new Mantis ticket +
// new Cerberus DREAD review.

package server

import (
	"net/http"

	core "dappco.re/go"
	coreapi "dappco.re/go/api"
	"dappco.re/lthn/desktop/pkg/serverkey"
	"github.com/gin-gonic/gin"
)

// bootstrapAuthHeaderPrefix names the Authorization-header scheme the
// frontend uses to deliver a bootstrap token. Mirrors the "Bearer "
// convention so middleware-chain ordering keeps the parse code
// uniform.
const bootstrapAuthHeaderPrefix = "Bootstrap "

// WithBootstrapAuth returns a coreapi.Option that installs the
// combined bearer-plus-bootstrap middleware. Paths in pathScopes are
// gated by VerifyBootstrapToken(verifier, scope); every other path
// falls through to bearer-token auth against bearerToken (mirroring
// the existing coreapi.WithBearerAuth semantics, including the skip
// list for /health + swagger + openapi).
//
// This option REPLACES coreapi.WithBearerAuth — callers MUST NOT use
// both. The combined middleware handles both responsibilities so the
// bootstrap path can short-circuit the bearer requirement cleanly
// without editing external/api/middleware.go.
//
// When verifier is nil OR pathScopes is empty, the option returns a
// no-op so callers can apply it unconditionally; the appropriate
// fallback is plain coreapi.WithBearerAuth(bearerToken) registered
// alongside (callers branch on serverkey availability at server
// construction time).
//
// Cerberus #1489 (HIGH): when the middleware IS installed (verifier !=
// nil AND pathScopes non-empty) but bearerToken is empty, non-bootstrap
// requests fail-closed with 503 "server_misconfigured". An empty
// LocalKey with an active ServerKey is no longer treated as "open
// server" — that posture was the fail-open footgun.
//
// Path matching is exact-equality (same isPublicPath semantics
// external/api uses for the bearer skip list).
//
// Usage example:
//
//	opts := []coreapi.Option{
//	    coreapi.WithRequestID(),
//	    coreapi.WithResponseMeta(),
//	    coreapi.WithMiddleware(cspMiddleware()),
//	    server.WithBootstrapAuth(serverkeySvc, bearerToken, map[string]string{
//	        "/v1/account/create": "account.create",
//	    }),
//	}
//
// Cerberus #1467: paths added here are security-policy decisions. New
// entries require Mantis ticket + new Cerberus DREAD review. Codify
// the rule on the caller side too (constant declaration in
// cmd/lthn/app.go or pkg/server/service.go).
func WithBootstrapAuth(verifier serverkey.Verifier, bearerToken string, pathScopes map[string]string) coreapi.Option {
	mw := BootstrapAuthMiddleware(verifier, bearerToken, pathScopes)
	if mw == nil {
		// No-op option — caller is responsible for falling back to
		// coreapi.WithBearerAuth(bearerToken) when bootstrap auth is
		// unavailable.
		return func(_ *coreapi.Engine) {}
	}
	return coreapi.WithMiddleware(mw)
}

// BootstrapAuthMiddleware returns the combined middleware Gin handler
// for direct registration via coreapi.WithMiddleware. Returns nil when
// verifier is nil OR pathScopes is empty so the caller can chain it
// safely:
//
//	if mw := server.BootstrapAuthMiddleware(verifier, key, paths); mw != nil {
//	    apiOpts = append(apiOpts, coreapi.WithMiddleware(mw))
//	}
//
// The middleware behaves:
//   - request path in skipList → c.Next() (no auth)
//   - request path in pathScopes → require Bootstrap token w/ matching scope
//   - any other path → require Bearer token == bearerToken
//
// Cerberus #1467 — pathScopes is map[path]scope, not []path. A
// bootstrap token's embedded scope claim MUST literal-match the
// pathScopes[req.URL.Path] entry; scope-laundering across paths is
// rejected.
//
// Cerberus #1465 — clients MUST NOT persist bootstrap tokens. The
// 60s issuer TTL plus per-mint nonce is the defence; this middleware
// is the verifier half.
func BootstrapAuthMiddleware(verifier serverkey.Verifier, bearerToken string, pathScopes map[string]string) gin.HandlerFunc {
	if verifier == nil || len(pathScopes) == 0 {
		return nil
	}
	// Build a defensive copy so caller mutations after construction
	// can't change live policy.
	pathMap := make(map[string]string, len(pathScopes))
	for k, v := range pathScopes {
		pathMap[k] = v
	}
	skipList := []string{"/health"}
	return func(c *gin.Context) {
		path := c.Request.URL.Path

		// Skip-list paths bypass both auth types — matches the
		// external/api bearer-auth behaviour for /health / swagger /
		// openapi. We only carry /health here because the engine
		// reports its own swagger + openapi skips via coreapi
		// internals; this middleware fronts everything else.
		for _, p := range skipList {
			if path == p {
				c.Next()
				return
			}
		}

		// Bootstrap-token path: require Bootstrap scheme + matching
		// scope. NEVER fall through to bearer — a missing/invalid
		// bootstrap token on an allowlisted path MUST 401, not slip
		// through on a normal bearer.
		if wantScope, ok := pathMap[path]; ok {
			header := c.GetHeader("Authorization")
			if !core.HasPrefix(header, bootstrapAuthHeaderPrefix) {
				c.AbortWithStatusJSON(http.StatusUnauthorized,
					coreapi.Fail("unauthorised", "bootstrap token required"))
				return
			}
			token := header[len(bootstrapAuthHeaderPrefix):]
			r := verifier.VerifyBootstrapToken(token, wantScope)
			if !r.OK {
				c.AbortWithStatusJSON(http.StatusUnauthorized,
					coreapi.Fail("unauthorised", "bootstrap token rejected"))
				return
			}
			c.Set("auth_via", "bootstrap")
			c.Next()
			return
		}

		// Standard bearer-auth path.
		//
		// Cerberus #1489 (HIGH — fail-closed posture): when bearerToken
		// is empty BUT this middleware was installed (i.e. verifier !=
		// nil AND pathScopes non-empty, gated above), the server has a
		// bootstrap-auth surface active. Permitting unauthenticated
		// access to ALL other paths in that posture is the exact
		// fail-open footgun Cerberus flagged — the lthn-mlx split-binary
		// plans run subsystems where ServerKey is the only auth source
		// and LocalKey is empty by construction.
		//
		// Reject every non-bootstrap request with 503 to signal
		// "server misconfigured: no bearer source". The caller is
		// responsible for installing coreapi.WithBearerAuth (or a
		// bootstrap-aware equivalent) alongside this middleware when
		// LocalKey is empty by design.
		if bearerToken == "" {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable,
				coreapi.Fail("server_misconfigured", "no bearer source configured"))
			return
		}
		header := c.GetHeader("Authorization")
		if header == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized,
				coreapi.Fail("unauthorised", "missing authorization header"))
			return
		}
		parts := core.SplitN(header, " ", 2)
		if len(parts) != 2 || core.Lower(parts[0]) != "bearer" || parts[1] != bearerToken {
			c.AbortWithStatusJSON(http.StatusUnauthorized,
				coreapi.Fail("unauthorised", "invalid bearer token"))
			return
		}
		c.Next()
	}
}
