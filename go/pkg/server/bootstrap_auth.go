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
	"crypto/subtle"
	"net/http"

	core "dappco.re/go"
	coreapi "dappco.re/go/api"
	"dappco.re/lthn/desktop/pkg/auth"
	"dappco.re/lthn/desktop/pkg/serverkey"
	"github.com/gin-gonic/gin"
)

// constantTimeStringEqual compares two strings in constant time relative
// to their length. Wraps crypto/subtle.ConstantTimeCompare so the
// caller's intent ("close the timing oracle") reads at the call site.
//
// Cerberus #57 F-1 / Mantis #1699 — the previous `a != b` bytewise
// compares at bootstrap_auth.go:392 + :531 short-circuited on the first
// mismatched byte, leaking the matched-prefix length as a wall-clock
// timing signal. A localhost attacker can recover an N-byte LocalKey
// with ~256·N samples; ~10⁵ requests is enough at typical loopback
// jitter. ConstantTimeCompare normalises the comparison cost so the
// timing channel collapses.
//
// crypto/subtle.ConstantTimeCompare returns 1 iff len(a)==len(b) AND
// the byte slices match; otherwise 0. This wrapper lifts that to a
// `bool` and accepts string inputs so it drops in for the existing
// `token != bearerToken` shape. CoreGO has no ConstantTimeCompare
// wrapper yet — surfaced as a CoreGO export gap so a future stack-wide
// adoption can fix N+1 sites at once (sibling case for upstream
// core/api/middleware.go:58 / Mantis #1451).
func constantTimeStringEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

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
			// B4 / RFC §4.6.1 + §4.6 table — bootstrap-token path
			// stamps TierOperator with empty Subject (bootstrap is
			// pre-identity per RFC §4.6 line 248). Source breadcrumb
			// "http-bootstrap" so audit grep can attribute the call.
			// The gin keys are read by WithCallerBridge (Option A
			// promotes to *core.Core SetCaller) and by handler-level
			// auth.FromGin fallbacks (gin-only handlers per §4.6.1).
			c.Set(auth.GinKeyTier, string(auth.TierOperator))
			c.Set(auth.GinKeySubject, "")
			c.Set(auth.GinKeySource, "http-bootstrap")
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
			// B4 / RFC §4.6.1 + §4.6 table — session-token path
			// stamps TierOperator with Subject=account_id (the
			// VerifySessionToken-recovered claim). Source breadcrumb
			// "http-session" so audit grep can distinguish session
			// from bootstrap or static-bearer call paths.
			c.Set(auth.GinKeyTier, string(auth.TierOperator))
			c.Set(auth.GinKeySubject, out.AccountID)
			c.Set(auth.GinKeySource, "http-session")
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
		// Cerberus #57 F-1 / Mantis #1699 — constant-time compare
		// closes the localhost timing-oracle that would otherwise
		// leak LocalKey one byte at a time via early-exit on first
		// mismatched byte.
		if !constantTimeStringEqual(token, bearerToken) {
			c.AbortWithStatusJSON(http.StatusUnauthorized,
				coreapi.Fail("unauthorised", "invalid bearer token"))
			return
		}
		c.Set("auth_via", "local")
		// B4 / RFC §4.6.1 + §4.6 table — LocalKey static-bearer path
		// stamps TierOperator with empty Subject (static key has no
		// account_id claim per RFC §4.6 line 249). Source breadcrumb
		// "http-local" so audit grep can distinguish from session.
		c.Set(auth.GinKeyTier, string(auth.TierOperator))
		c.Set(auth.GinKeySubject, "")
		c.Set(auth.GinKeySource, "http-local")
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
			// B4 / RFC §4.6.1 — mirror the BootstrapAndSession path
			// so the legacy WithBootstrapAuth callers (pre-Stage-E
			// surfaces) ALSO populate the gin keys the bridge reads.
			// Without this, a WithBootstrapAuth-wired server would
			// emit no Caller on bootstrap-path requests even with
			// WithCallerBridge installed downstream.
			c.Set(auth.GinKeyTier, string(auth.TierOperator))
			c.Set(auth.GinKeySubject, "")
			c.Set(auth.GinKeySource, "http-bootstrap")
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
		// Cerberus #57 F-1 / Mantis #1699 — constant-time compare on
		// the bearer-secret arm of BootstrapAuthMiddleware (mirrors
		// the WithBootstrapAuth path above). The scheme+length checks
		// stay early-exit; only the secret compare needs the timing
		// defence.
		if len(parts) != 2 || core.Lower(parts[0]) != "bearer" || !constantTimeStringEqual(parts[1], bearerToken) {
			c.AbortWithStatusJSON(http.StatusUnauthorized,
				coreapi.Fail("unauthorised", "invalid bearer token"))
			return
		}
		// B4 / RFC §4.6.1 — legacy WithBootstrapAuth bearer arm
		// stamps TierOperator/empty/http-local for parity with the
		// Stage E.B middleware. Without this, downstream Caller
		// reads would default to TierInternal even for an
		// authenticated request, breaking gates that require
		// TierOperator at the substrate.
		c.Set("auth_via", "local")
		c.Set(auth.GinKeyTier, string(auth.TierOperator))
		c.Set(auth.GinKeySubject, "")
		c.Set(auth.GinKeySource, "http-local")
		c.Next()
	}
}

// WithCallerBridge returns a coreapi.Option that installs the §4.6.1
// Option A gin → *core.Core CallerIdentity bridge (Mantis #1735,
// RFC.tier-auth-substrate.md §4.6.1).
//
// Wiring: registered AFTER WithBootstrapAuth / WithBootstrapAndSessionAuth
// in the coreapi.Option chain. Per request, the bridge:
//
//  1. Reads the auth.GinKey* values stamped by the auth middleware.
//  2. If a Tier is present (auth resolved), calls auth.SetCaller(c, ident)
//     so that downstream service-method dispatches via auth.Caller(c) see
//     the resolved identity.
//  3. Defers auth.ClearCaller(c) so the stamp does not leak across
//     requests when the same *core.Core is reused (the common case for
//     pkg/server callers — opts.Core is one shared instance).
//  4. Calls gctx.Next() to continue the handler chain.
//
// When c is nil this Option installs no middleware — the gin-key stamps
// are still set by the auth middleware, so handler-level
// auth.FromGin(gctx, coreCtx) fallbacks still work for gin-only handlers
// (per RFC §4.6.1 last paragraph). The bridge ONLY engages when the
// caller wires a *core.Core (the production wire at service.go does
// this when opts.Core is non-nil).
//
// Cerberus #1735 / RFC §4.6.1 Option A — the bridge is single-site
// (this middleware) so new handlers do not need to remember a per-call
// prelude. The substrate is additive: handlers that never reach a
// substrate-gated service method see no behaviour change.
//
// Usage example:
//
//	opts := []coreapi.Option{
//	    server.WithBootstrapAndSessionAuth(verifier, key,
//	        server.BootstrapPathScopes, server.RouteTiers, server.RouteTierPrefixes),
//	    server.WithCallerBridge(opts.Core),
//	}
func WithCallerBridge(c *core.Core) coreapi.Option {
	if c == nil {
		return func(_ *coreapi.Engine) {}
	}
	mw := CallerBridgeMiddleware(c)
	if mw == nil {
		return func(_ *coreapi.Engine) {}
	}
	return coreapi.WithMiddleware(mw)
}

// CallerBridgeMiddleware returns the gin handler that promotes the
// auth.GinKey* stamps onto the supplied *core.Core via auth.SetCaller.
// Returns nil when c is nil so the caller can chain it safely.
//
// Behaviour:
//
//   - No gin-key stamp present (auth middleware did not run, or
//     skip-list path bypass) → no SetCaller, c.Next() unchanged. Caller
//     reads return the TierInternal floor — deny-by-default at gates.
//   - Gin-key stamp present → SetCaller(c, {Tier,Subject,Source}) +
//     defer ClearCaller(c) + c.Next().
//
// The defer-clear pattern matches the RFC §4.6.1 wiring shape and the
// auth.SetCaller doc-string guidance (HTTP middleware MUST call
// ClearCaller when reusing *core.Core across requests).
//
//	mw := server.CallerBridgeMiddleware(coreInstance)
//	engine.Use(mw)
//
// Forward-arc (Mantis #1756 partial / RFC.tier-auth-substrate v3.1
// §4.6.2 Option 3 — soft-deprecate): the SetCaller/ClearCaller pair
// this bridge uses is the documented sidecar-shaped path; the
// Context-shaped primitive (auth.WithCaller) is race-safe by
// construction (immutable derived ctx) and is the preferred substrate
// for new handler-chain wiring. The bridge keeps the sidecar shape
// until the Phase 1.5.1 follow-on ticket migrates the handler chain to
// thread a per-request core.Context through pkg/server →
// pkg/tasks → pkg/queue. Until then, the sidecar's pointer-identity
// scope across concurrent requests sharing one *core.Core is the
// surfaced lottery (H#256 surface, F-2 cascade) — the bridge's
// defer-ClearCaller closes the inter-request leak but does not address
// the in-flight overlap between two concurrent requests on the same
// *core.Core (mitigated today by per-request derived Core where wired).
func CallerBridgeMiddleware(c *core.Core) gin.HandlerFunc {
	if c == nil {
		return nil
	}
	return func(gctx *gin.Context) {
		tier, _ := gctx.Get(auth.GinKeyTier)
		tierStr, _ := tier.(string)
		if tierStr == "" {
			// Auth middleware did not stamp (skip-list, static asset,
			// or an unauthenticated request that the auth middleware
			// already aborted). No SetCaller; downstream Caller(c)
			// returns TierInternal floor per RFC §4.1.
			gctx.Next()
			return
		}
		subject, _ := gctx.Get(auth.GinKeySubject)
		source, _ := gctx.Get(auth.GinKeySource)
		subjectStr, _ := subject.(string)
		sourceStr, _ := source.(string)
		auth.SetCaller(c, auth.CallerIdentity{
			Tier:    auth.CallerTier(tierStr),
			Subject: subjectStr,
			Source:  sourceStr,
		})
		defer auth.ClearCaller(c)
		gctx.Next()
	}
}
