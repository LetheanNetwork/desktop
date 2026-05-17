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

// sessionTokenHeaderPrefix identifies a session token inside a
// standard `Authorization: Bearer <token>` header. The bearer
// middleware dispatches on this prefix before doing a bytewise
// LocalKey comparison so a session token never falls through to
// the static-bearer path (Cerberus DREAD H2 fail-closed posture).
const sessionTokenHeaderPrefix = "LTHN-SESS-1."

// RouteTier classifies an HTTP path's auth requirement per
// RFC.stage-e.md v2 §4. A route's tier dictates which token types
// the bearer middleware accepts on the route's behalf.
//
// Per Cerberus DREAD H1 — deny-by-default is necessary but not
// sufficient: pkg/server's CI test
// TestService_AllRoutesTiered_Good walks engine.Routes() post-
// registration and asserts every path is either in the routeTiers
// map OR in the bootstrap-auth skip-list.
//
// Per Cerberus DREAD C2 — the data-tier slot is RESERVED. Today's
// implementation treats it as a session-tier alias; Phase 2 (Mantis
// #1487) fills the at-rest-encryption decrypt layer.
type RouteTier string

const (
	// TierLocal accepts the static LocalKey bearer-equality match OR
	// a valid LTHN-SESS-1.* session token. Use for endpoints that
	// don't access user-data behind the unlock gate (e.g. /health,
	// /v1/server/info, /v1/models stub).
	TierLocal RouteTier = "local"

	// TierSession accepts ONLY a valid LTHN-SESS-1.* session token.
	// Static LocalKey requests reject with 401 — the static key has
	// no `account_id` claim, so it can't scope per-account
	// reads/writes that user-data endpoints require.
	TierSession RouteTier = "session"

	// TierData is the RESERVED slot for at-rest-encrypted data
	// endpoints per Cerberus DREAD C2 + Mantis #1487. Behaves as
	// TierSession today; the decrypt-on-read precondition fills in
	// when #1487 lands. Defining the slot now locks the policy so
	// future contributors classify into it instead of widening
	// TierSession.
	TierData RouteTier = "data"
)

// WithBootstrapAuth is the pre-Stage-E entry that wires only the
// bootstrap+bearer responsibilities. Stage E.B introduces the
// session-tier layer via WithBootstrapAndSessionAuth; this entry
// stays for callers that haven't migrated yet (it routes every
// non-bootstrap path through the static-bearer comparison). New
// callers SHOULD use WithBootstrapAndSessionAuth.
//
// Path matching is exact-equality (same isPublicPath semantics
// external/api uses for the bearer skip list).
//
// Cerberus #1489 (HIGH): when the middleware IS installed (verifier !=
// nil AND pathScopes non-empty) but bearerToken is empty, non-bootstrap
// requests fail-closed with 503 "server_misconfigured".
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
func WithBootstrapAuth(verifier serverkey.Verifier, bearerToken string, pathScopes map[string]string) coreapi.Option {
	mw := BootstrapAuthMiddleware(verifier, bearerToken, pathScopes)
	if mw == nil {
		return func(_ *coreapi.Engine) {}
	}
	return coreapi.WithMiddleware(mw)
}

// resolveRouteTier classifies the supplied request path by
// consulting (in order):
//
//  1. routeTiers exact-match
//  2. routeTierPrefixes longest-prefix-match (uses pathStartsWithPrefix)
//
// Returns (tier, true) on a classified path; ("", false) on the
// deny-by-default branch. Longest-prefix discipline matters when
// future additions stack sub-trees — the most specific wins so an
// override at /v1/api/opencode/secret beats a wider /v1/api/opencode.
//
// Centralised here (not inline in the middleware) so the CI test
// can call the same resolver and stay in lockstep with the live
// classification logic.
//
// Usage example:
//
//	tier, ok := resolveRouteTier(server.RouteTiers, server.RouteTierPrefixes, "/v1/api/opencode/sandbox/abc")
//	if ok { _ = tier } // TierLocal via prefix match
func resolveRouteTier(routeTiers map[string]RouteTier, routeTierPrefixes map[string]RouteTier, path string) (RouteTier, bool) {
	if tier, ok := routeTiers[path]; ok {
		return tier, true
	}
	var (
		bestTier  RouteTier
		bestLen   int
		bestFound bool
	)
	for prefix, tier := range routeTierPrefixes {
		if !pathStartsWithPrefix(path, prefix) {
			continue
		}
		if len(prefix) > bestLen {
			bestLen = len(prefix)
			bestTier = tier
			bestFound = true
		}
	}
	return bestTier, bestFound
}

// pathStartsWithPrefix reports whether path == prefix OR path
// starts with prefix + "/". Matches the same semantics service.go's
// pathHasPrefix helper uses for the webview-engine composite — kept
// local here so bootstrap_auth.go stays self-contained.
func pathStartsWithPrefix(path, prefix string) bool {
	if !core.HasPrefix(path, prefix) {
		return false
	}
	if len(path) == len(prefix) {
		return true
	}
	return path[len(prefix)] == '/'
}

// WithBootstrapAndSessionAuth returns a coreapi.Option that installs
// the full Stage E.B auth-chain middleware: bootstrap-token paths,
// session-token paths (per the routeTiers classification), and the
// static-bearer fallback for local-tier paths.
//
// Auth-chain decision tree (RFC.stage-e.md v2 §4):
//
//  1. skip-list path (e.g. /health, non-/v1/* static assets) → c.Next()
//  2. path in pathScopes → require Bootstrap header + matching scope
//     (fail-closed: 401 if missing/invalid; NEVER falls through)
//  3. resolveRouteTier(path) returns TierSession or TierData → require
//     Bearer header parseable as LTHN-SESS-1.* and VerifySessionToken
//     OK (fail-closed: 401; NEVER falls through to static-bearer)
//  4. resolveRouteTier(path) returns TierLocal → require Bearer header
//     matching bearerToken OR a valid LTHN-SESS-1.* session token
//     (either OK)
//  5. resolveRouteTier(path) returns false → DENY-BY-DEFAULT, 401 with
//     "route_not_tiered" (the CI test should catch this at build,
//     this is the runtime fallback)
//
// routeTierPrefixes catches gin parameterised routes (e.g.
// /v1/api/opencode/sandbox/:id) that the exact-match routeTiers
// can't classify against concrete incoming paths.
//
// Per Cerberus DREAD H2 — five fail-closed cases on the session-
// token branch MUST short-circuit with 401, NEVER fall through to
// LocalKey bytewise comparison: malformed prefix, signature
// invalid, expired, wrong scope, signed by a different server-key.
//
// Per Cerberus DREAD H3 — verification happens ONCE at middleware
// entry. The handler completes even if the token expires mid-
// handler (long-running work uses the task-queue substrate, not
// mid-flight re-verification).
//
// Usage example:
//
//	opts := []coreapi.Option{
//	    coreapi.WithRequestID(),
//	    coreapi.WithResponseMeta(),
//	    coreapi.WithMiddleware(cspMiddleware()),
//	    server.WithBootstrapAndSessionAuth(serverkeySvc, bearerToken,
//	        server.BootstrapPathScopes, server.RouteTiers, server.RouteTierPrefixes),
//	}
func WithBootstrapAndSessionAuth(
	verifier serverkey.Verifier,
	bearerToken string,
	pathScopes map[string]string,
	routeTiers map[string]RouteTier,
	routeTierPrefixes map[string]RouteTier,
) coreapi.Option {
	mw := BootstrapAndSessionAuthMiddleware(verifier, bearerToken, pathScopes, routeTiers, routeTierPrefixes)
	if mw == nil {
		return func(_ *coreapi.Engine) {}
	}
	return coreapi.WithMiddleware(mw)
}

// BootstrapAndSessionAuthMiddleware returns the Stage E.B gin
// handler for the full bootstrap + session + bearer chain.
// Returns nil when verifier is nil OR pathScopes is empty so the
// caller can chain it safely.
//
// Implementation MUST hold the auth chain shape verbatim:
//
//   - skip-list / non-/v1 → c.Next()
//   - bootstrap path → require Bootstrap header + matching scope,
//     fail-closed on miss
//   - session-tier or data-tier route → require Bearer LTHN-SESS-1.*
//     token, VerifySessionToken OK, fail-closed on miss (NEVER
//     bytewise-compares against LocalKey)
//   - local-tier route → Bearer either matches LocalKey OR is a
//     valid session token
//   - unclassified route → 401 deny-by-default (the CI test
//     TestService_AllRoutesTiered_Good catches at build time)
func BootstrapAndSessionAuthMiddleware(
	verifier serverkey.Verifier,
	bearerToken string,
	pathScopes map[string]string,
	routeTiers map[string]RouteTier,
	routeTierPrefixes map[string]RouteTier,
) gin.HandlerFunc {
	if verifier == nil || len(pathScopes) == 0 {
		return nil
	}
	pathMap := make(map[string]string, len(pathScopes))
	for k, v := range pathScopes {
		pathMap[k] = v
	}
	tierMap := make(map[string]RouteTier, len(routeTiers))
	for k, v := range routeTiers {
		tierMap[k] = v
	}
	prefixMap := make(map[string]RouteTier, len(routeTierPrefixes))
	for k, v := range routeTierPrefixes {
		prefixMap[k] = v
	}
	skipList := []string{"/health"}
	return func(c *gin.Context) {
		path := c.Request.URL.Path

		// Static-asset / SPA-route bypass (mirrors the legacy
		// BootstrapAuthMiddleware — the Gin engine in lthn-desktop
		// fronts both /v1/* API and the embedded SPA via NoRoute).
		if !core.HasPrefix(path, "/v1/") && path != "/v1" {
			c.Next()
			return
		}

		// Skip-list paths bypass both auth types — matches the
		// external/api bearer-auth behaviour for /health.
		for _, p := range skipList {
			if path == p {
				c.Next()
				return
			}
		}

		// Bootstrap-token path: require Bootstrap scheme + matching
		// scope. NEVER fall through to bearer / session — a
		// missing/invalid bootstrap token on an allowlisted path
		// MUST 401, not slip through on a normal bearer.
		//
		// Mantis #1626 (Cerberus #25 ADD-HIGH-1) — lookup uses
		// c.FullPath() (gin's registered route pattern, e.g.
		// "/v1/account/:id/seal") NOT c.Request.URL.Path (the concrete
		// request URL with parameters interpolated). Today's
		// BootstrapPathScopes entries are all fixed strings so literal
		// match works, but Stage E.A's /v1/account/:id/seal is the
		// first parametrised entry — literal lookup against
		// "/v1/account/abc123/seal" would silently miss and the
		// endpoint would be dead-on-arrival.
		fullPath := c.FullPath()
		if wantScope, ok := pathMap[fullPath]; ok && fullPath != "" {
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

		// Route-tier classification (RFC §4 H1 — deny-by-default).
		// Consult exact-match tierMap first, then longest-prefix
		// match in prefixMap. Unclassified routes 401 with a
		// route_not_tiered code so the CI failure surfaces obviously
		// vs a normal auth reject.
		tier, classified := resolveRouteTier(tierMap, prefixMap, path)
		if !classified {
			c.AbortWithStatusJSON(http.StatusUnauthorized,
				coreapi.Fail("route_not_tiered",
					"route is not in routeTiers — add an entry to pkg/server.RouteTiers or RouteTierPrefixes (RFC §4 H1)"))
			return
		}

		header := c.GetHeader("Authorization")
		if header == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized,
				coreapi.Fail("unauthorised", "missing authorization header"))
			return
		}
		parts := core.SplitN(header, " ", 2)
		if len(parts) != 2 || core.Lower(parts[0]) != "bearer" {
			c.AbortWithStatusJSON(http.StatusUnauthorized,
				coreapi.Fail("unauthorised", "invalid bearer scheme"))
			return
		}
		token := parts[1]

		// Session-token branch — recognised by the LTHN-SESS-1.
		// prefix. MUST short-circuit on every failure path; NEVER
		// fall through to LocalKey bytewise comparison per Cerberus
		// DREAD H2.
		if core.HasPrefix(token, sessionTokenHeaderPrefix) {
			r := verifier.VerifySessionToken(token)
			if !r.OK {
				c.AbortWithStatusJSON(http.StatusUnauthorized,
					coreapi.Fail("unauthorised", "session token rejected"))
				return
			}
			out, _ := r.Value.(serverkey.SessionVerifyOutput)
			c.Set("auth_via", "session")
			c.Set("account_id", out.AccountID)
			c.Next()
			return
		}

		// Non-session token: only local-tier routes accept the
		// static LocalKey. Session-tier or data-tier routes REJECT
		// here — the static key has no account_id, so it can't
		// scope per-account reads/writes.
		if tier == TierSession || tier == TierData {
			c.AbortWithStatusJSON(http.StatusUnauthorized,
				coreapi.Fail("unauthorised",
					"this endpoint requires a session token — unlock your account first"))
			return
		}

		// Local-tier static-bearer comparison. Cerberus #1489
		// fail-closed: when bearerToken is empty AND the middleware
		// is installed, abort 503 server_misconfigured instead of
		// permitting unauthenticated access.
		if bearerToken == "" {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable,
				coreapi.Fail("server_misconfigured", "no bearer source configured"))
			return
		}
		if token != bearerToken {
			c.AbortWithStatusJSON(http.StatusUnauthorized,
				coreapi.Fail("unauthorised", "invalid bearer token"))
			return
		}
		c.Set("auth_via", "local")
		c.Next()
	}
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

		// Static-asset / SPA-route bypass — the Gin engine in lthn-desktop
		// fronts both the JSON API (under /v1/*) AND the embedded SPA
		// (everything else, served via the NoRoute fallback at
		// desktop.go:attachSPA). Without this skip the WebView's first
		// request for "/?surface=tray" (or any /src/*, /assets/*, etc.)
		// gets a 401 JSON instead of index.html, so the entire frontend
		// never loads and the user sees raw `{"error":"unauthorised",...}`
		// in the WebView (Snider's 2026-05-16 screenshot).
		//
		// Policy: only paths under /v1/ go through auth. Everything else
		// is a static asset and reaches NoRoute → file server. The /v1
		// prefix is also the convention coreapi.WithBearerAuth implicitly
		// assumed; making it explicit closes the engine-wide-middleware
		// footgun.
		if !core.HasPrefix(path, "/v1/") && path != "/v1" {
			c.Next()
			return
		}

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
		//
		// Mantis #1626 (Cerberus #25 ADD-HIGH-1) — lookup uses
		// c.FullPath() (gin's registered route pattern, e.g.
		// "/v1/account/:id/seal") NOT c.Request.URL.Path (the concrete
		// request URL with parameters interpolated). Today's
		// BootstrapPathScopes entries are all fixed strings so literal
		// match works, but Stage E.A's /v1/account/:id/seal is the
		// first parametrised entry — literal lookup against
		// "/v1/account/abc123/seal" would silently miss and the
		// endpoint would be dead-on-arrival.
		fullPath := c.FullPath()
		if wantScope, ok := pathMap[fullPath]; ok && fullPath != "" {
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
