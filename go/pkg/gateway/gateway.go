// SPDX-Licence-Identifier: EUPL-1.2

// Package gateway is the runtime data firewall (RFC.marketplace.md §7a).
//
// Every request from a community plugin to the host's data + services
// flows through this package. Two responsibilities:
//
//  1. **Permission enforcement** — the calling bundle is identified by
//     the Bundle-ID header; CheckPermission verifies the bundle's
//     manifest declared the requested scope+mode pair at install time.
//     A plugin that asks for project.metadata:read without declaring
//     it in manifest.permissions gets HTTP 403 + a typed error.
//
//  2. **Schema validation at the boundary** — every incoming JSON body
//     is decoded against the registered scope's expected shape before
//     ANY handler logic runs. Reject-not-cleanse: malformed bodies
//     never reach the data layer.
//
// Two-layer security model (RFC §7a):
//
//   - **Layer 1 (already exists)** — pkg/sandbox + borg run sit at the
//     OS-process boundary. Plugins can't escape the container.
//   - **Layer 2 (this package)** — pkg/gateway sits at the API boundary.
//     Plugins can call out of the container, but only through this
//     gate; the gate enforces declared-permissions + schema.
//
// Localhost session encryption is a v2 forward-arc; for v1 we derive a
// session key from a go-store secret read at gateway boot (per spec
// "v1: derive from a go-store secret"). LetheanAccount-derived session
// keys are TARGETED for the account substrate Stage B work (Mantis
// #1440) — TODAY the v1 session key is the only thing in play; the
// account substrate is not yet wired to derive anything. Updating
// the gateway side will be mechanical once the keys-pkg PGP unlock
// lands.
package gateway

import (
	"net/http"

	"github.com/gin-gonic/gin"

	core "dappco.re/go"
	coreapi "dappco.re/go/api"
	"dappco.re/go/orm"

	"dappco.re/lthn/desktop/pkg/marketplace"
	"dappco.re/lthn/desktop/pkg/opencode"
)

// headerBundleID is the client-supplied bundle identity claim. Kept for
// backward compatibility but treated as untrusted when X-Bundle-Token is
// present (Cerberus Mantis #1443). When the token resolves, the resolved
// code supersedes whatever the client asserted in this header.
const headerBundleID = "Bundle-ID"

// headerBundleToken carries the per-launch secret issued by the plugin
// host at Start time (Cerberus Mantis #1443). Plugins pass this on every
// outbound gateway request. The gateway resolves it to the authoritative
// bundle code via BundleCodeResolver — so a plugin cannot spoof another
// plugin's identity by forging the Bundle-ID header.
const headerBundleToken = "X-Bundle-Token"

// BundleCodeResolver is the narrow interface the gateway uses to resolve
// a per-launch bundle secret to the plugin code that was issued that
// secret. Implemented by pkg/plugin.Service; held as an interface here
// to avoid a circular import (gateway → plugin → marketplace → gateway).
//
// Usage example (in tests):
//
//	svc := gateway.NewService(c)
//	gateway.SetBundleCodeResolver(svc, myResolver)
type BundleCodeResolver interface {
	LookupBundleCode(secret string) (string, bool)
}

// MaxBodyBytes is the upper bound on a single incoming plugin
// request body. Cerberus Mantis #1435 — without this, a plugin
// could submit a 10 GiB POST body, ReadAll'd by a scope handler,
// to OOM the host. 4 MiB is generous (manifest-sized data + room
// for batched RAG payloads); scope handlers that need bigger bodies
// declare their cap via a future per-scope override.
//
// Exported per RFC.body-cap-middleware.md Amendment A5 (Cerberus #16
// C-6) — single source of truth, eliminates the duplicate-constant
// drift risk that v1 of the body-cap RFC nearly shipped. pkg/server's
// DefaultBodyCapOverrides table sources the gateway cap from this
// constant directly, so a bump here propagates automatically.
//
// The net/http import here is permitted (covered by AX phase 5/5b
// migrations — gateway is the API boundary, http.MaxBytesReader is
// the canonical primitive). core.MaxBytesReader would be the
// preferred CoreGO wrap; tracking as project_corego_export_gaps.md
// Gap 6 if it becomes a pattern.
const MaxBodyBytes = 4 << 20 // 4 MiB

// scopeKey composes Scope:Mode for the in-memory handler registry.
// The same handler can serve multiple modes (e.g. project.metadata:
// read AND project.metadata:write) by registering twice with different
// modes; the registry treats them as independent entries.
func scopeKey(scope, mode string) string {
	return scope + ":" + mode
}

// Handler is the per-scope worker. Receives the calling bundle id +
// the gin context (already validated against schema at the gateway
// boundary). Returns a core.Result; the dispatcher serialises Value as
// JSON for OK, Error as a typed-error envelope for Fail.
//
// Usage example:
//
//	gateway.RegisterScope("project.metadata", "read",
//	    func(c *core.Core, bundleID string, ctx *gin.Context) core.Result {
//	        return core.Ok(map[string]any{"projects": []string{}})
//	    })
type Handler func(c *core.Core, bundleID string, ctx *gin.Context) core.Result

// Service owns the gateway's registered scope handlers + the per-call
// request lifecycle. Held on the Core service registry under the name
// "gateway"; pkg/server mounts the route group at /v1/api/gateway/*.
type Service struct {
	core     *core.Core
	handlers map[string]Handler
	// resolver is the optional per-bundle token verifier (Cerberus #1443).
	// When non-nil, Handle reads X-Bundle-Token and resolves it to the
	// authoritative bundle code. Nil until SetBundleCodeResolver is called;
	// tests and non-plugin callers run without it (header claim accepted
	// as-is for those paths).
	resolver BundleCodeResolver
}

// NewService constructs the gateway against a Core container. Wired
// via core.WithName("gateway", gateway.Register) in cmd/lthn/app.go.
//
// Usage example:
//
//	svc := gateway.NewService(c)
func NewService(c *core.Core) *Service {
	s := &Service{
		core:     c,
		handlers: map[string]Handler{},
	}
	registerBuiltinScopes(s)
	return s
}

// SetBundleCodeResolver wires the per-bundle token verifier into a
// running Service (Cerberus Mantis #1443). Called at boot after both
// the "gateway" and "plugin" services are registered on Core. Once set,
// Handle uses the resolver to derive the authoritative bundle code from
// X-Bundle-Token rather than trusting the caller-asserted Bundle-ID.
//
// Usage example:
//
//	if pluginSvc, ok := core.ServiceFor[*plugin.Service](c, "plugin"); ok {
//	    gateway.SetBundleCodeResolver(gatewaySvc, pluginSvc)
//	}
func SetBundleCodeResolver(s *Service, r BundleCodeResolver) {
	if s == nil {
		return
	}
	s.resolver = r
}

// Register constructs the gateway service for Core registration.
// Mirrors the marketplace.Register / queue.Register convention.
//
// Usage example:
//
//	core.New(core.WithName("gateway", gateway.Register))
func Register(c *core.Core) core.Result {
	return core.Ok(NewService(c))
}

// RegisterScope installs a handler for one (scope, mode) pair. Last-
// write-wins on the same key (matches Core's c.Action semantics).
// Returns a Result for symmetry with the rest of the package; today
// the only failure path is an empty scope or nil handler.
//
// Usage example:
//
//	r := gateway.RegisterScope(svc, "project.metadata", "read",
//	    func(c *core.Core, bundleID string, ctx *gin.Context) core.Result {
//	        return core.Ok(map[string]any{"projects": []string{}})
//	    })
func RegisterScope(s *Service, scope, mode string, handler Handler) core.Result {
	if s == nil {
		return core.Fail(core.E("gateway.RegisterScope", "service is nil", nil))
	}
	if scope == "" {
		return core.Fail(core.E("gateway.RegisterScope", "scope is required", nil))
	}
	if mode == "" {
		return core.Fail(core.E("gateway.RegisterScope", "mode is required", nil))
	}
	if handler == nil {
		return core.Fail(core.E("gateway.RegisterScope", "handler is required", nil))
	}
	s.handlers[scopeKey(scope, mode)] = handler
	return core.Ok(scopeKey(scope, mode))
}

// CheckPermission verifies the named bundle declared (scope, mode) in
// its manifest.permissions block at install time. Returns Ok on a
// match, Fail on mismatch or unknown bundle. The gateway's HTTP
// handler calls this before dispatching to the scope's Handler;
// callers outside the HTTP path can use it directly to gate non-HTTP
// access (e.g. an MCP tool that wraps the same scope catalogue).
//
// Usage example:
//
//	r := gateway.CheckPermission(c, "opencode", "project.metadata", "read")
//	if !r.OK { return core.Fail(...) }
func CheckPermission(c *core.Core, bundleID, scope, mode string) core.Result {
	if c == nil {
		return core.Fail(core.E("gateway.CheckPermission", "core is nil", nil))
	}
	if bundleID == "" {
		return core.Fail(core.E("gateway.CheckPermission", "bundle id is required", nil))
	}
	r := orm.Of[marketplace.InstalledBundle](c).Find(bundleID)
	if !r.OK {
		return core.Fail(core.E("gateway.CheckPermission",
			"bundle not installed: "+bundleID, nil))
	}
	rec, _, ok := orm.Detail[marketplace.InstalledBundle](r)
	if !ok {
		return core.Fail(core.E("gateway.CheckPermission", "cast bundle record", nil))
	}
	for _, p := range marketplace.DecodePermissions(rec.Permissions) {
		if p.Scope == scope && p.Mode == mode {
			return core.Ok(nil)
		}
	}
	return core.Fail(core.E("gateway.CheckPermission",
		"permission not declared: "+scope+":"+mode, nil))
}

// gatewayRoutes implements coreapi.RouteGroup so the gateway HTTP
// surface mounts via api.Engine().Register alongside opencode +
// plugin proxy + the OpenAI-compat handlers. Pattern mirrors
// pkg/server/handlers.go:lthnRoutes.
type gatewayRoutes struct{ s *Service }

// NewRoutes returns a coreapi.RouteGroup exposing the gateway over
// /v1/api/gateway/<scope>/<mode>. Mount via:
//
//	s.Engine().Register(gateway.NewRoutes(svc))
func NewRoutes(s *Service) coreapi.RouteGroup { return &gatewayRoutes{s: s} }

// Name labels the route group in coreapi diagnostics.
func (g *gatewayRoutes) Name() string { return "gateway" }

// BasePath leaves prefixing to the routes themselves so the URL shape
// is /v1/api/gateway/<scope>/<mode>. Matches the lthnRoutes shape.
func (g *gatewayRoutes) BasePath() string { return "" }

// RegisterRoutes wires the gateway endpoint. POST so the body carries
// the per-scope payload; the URL path encodes scope + mode.
//
// Usage example (calling plugin):
//
//	curl -H "Bundle-ID: opencode" \
//	     -d '{"limit":10}' \
//	     http://localhost:8000/v1/api/gateway/project.metadata/read
func (g *gatewayRoutes) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/v1/api/gateway/:scope/:mode", g.s.Handle)
}

// Handle is the gin.HandlerFunc dispatch. Order:
//  1. Body-cap (Cerberus #1435) — MaxBytesReader before any read.
//  2. Resolve authoritative bundle identity (Cerberus #1443):
//     a. If X-Bundle-Token present and a BundleCodeResolver is wired,
//        resolve token → code. Spoofed Bundle-ID header is ignored.
//     b. If no token (backward compat / non-plugin callers), fall back
//        to the Bundle-ID header claim.
//     c. 401 when neither produces a non-empty code.
//  3. Look up scope handler — 501 if no handler registered.
//  4. CheckPermission against the bundle's declared manifest scope —
//     403 if not declared.
//  5. Dispatch to the registered Handler.
//  6. Serialise Result.Value as JSON on success; typed error envelope
//     on Fail.
//
// Schema validation lives inside each scope's Handler so per-scope
// shapes don't bleed up here; the gateway just routes.
func (s *Service) Handle(ctx *gin.Context) {
	// Cerberus #1435 — body-cap before scope handlers read. Wraps
	// the request body in MaxBytesReader; any scope handler that
	// does ShouldBindJSON / ReadAll / etc. on this body will get
	// an error past the cap. Stops OOM-by-large-payload attacks
	// at the boundary, regardless of which handler is dispatched.
	ctx.Request.Body = http.MaxBytesReader(ctx.Writer, ctx.Request.Body, MaxBodyBytes)

	// Cerberus #1443 — derive authoritative bundle identity.
	// Priority: X-Bundle-Token (verified) > Bundle-ID (claimed).
	// When a resolver is wired and the token header is present, the
	// resolved code is the truth; the Bundle-ID header is ignored so
	// Plugin A cannot impersonate Plugin B by forging Bundle-ID: B.
	bundleID := ""
	if s.resolver != nil {
		if token := ctx.GetHeader(headerBundleToken); token != "" {
			code, ok := s.resolver.LookupBundleCode(token)
			if !ok || code == "" {
				writeError(ctx, core.StatusUnauthorized, "invalid-bundle-token",
					"X-Bundle-Token is unknown or expired")
				return
			}
			bundleID = code
			// Strip the token from the forwarded request so scope
			// handlers never see the raw secret.
			ctx.Request.Header.Del(headerBundleToken)
		}
	}
	// Fall back to the caller-asserted Bundle-ID when no token was
	// resolved (backward compat: non-plugin callers such as the
	// WebView or integration tests supply only the Bundle-ID header).
	if bundleID == "" {
		bundleID = ctx.GetHeader(headerBundleID)
	}
	if bundleID == "" {
		writeError(ctx, core.StatusUnauthorized, "missing-bundle-id",
			"Bundle-ID header is required (or X-Bundle-Token for verified plugin calls)")
		return
	}
	scope := ctx.Param("scope")
	mode := ctx.Param("mode")
	handler, ok := s.handlers[scopeKey(scope, mode)]
	if !ok {
		writeError(ctx, core.StatusNotImplemented, "scope-unavailable",
			"scope not implemented: "+scopeKey(scope, mode))
		return
	}
	if r := CheckPermission(s.core, bundleID, scope, mode); !r.OK {
		writeError(ctx, core.StatusForbidden, "permission-denied", r.Error())
		return
	}
	r := handler(s.core, bundleID, ctx)
	if !r.OK {
		writeError(ctx, core.StatusBadRequest, "handler-failed", r.Error())
		return
	}
	ctx.JSON(core.StatusOK, r.Value)
}

// writeError formats a typed-error envelope so plugin clients can
// branch on `code` without parsing message strings.
func writeError(ctx *gin.Context, status int, code, message string) {
	ctx.JSON(status, map[string]any{
		"error": map[string]any{
			"code":    code,
			"message": message,
		},
	})
}

// registerBuiltinScopes wires the v1 scope catalogue. Handlers live in
// scopes.go split out so the dispatch table stays readable.
func registerBuiltinScopes(s *Service) {
	RegisterScope(s, "project.metadata", "read", projectMetadataRead)
	RegisterScope(s, "service.notify", "invoke", serviceNotifyInvoke)
}

// projectMetadataRead returns the list of imported projects without
// any file content. Sourced from orm.Of[opencode.ImportedProject]
// directly — already the source-of-truth in this binary; pkg/repos
// is a UI aggregator on top + doesn't expose a clean List().
func projectMetadataRead(c *core.Core, _ string, ctx *gin.Context) core.Result {
	r := orm.Of[opencode.ImportedProject](c).Get()
	if !r.OK {
		// Empty catalogue is a valid state, not a failure — return
		// zero-row response cleanly so plugins can iterate without a
		// special-case branch.
		return core.Ok(map[string]any{
			"projects": []map[string]string{},
			"count":    0,
		})
	}
	rows, ok := r.Value.([]opencode.ImportedProject)
	if !ok {
		return core.Ok(map[string]any{
			"projects": []map[string]string{},
			"count":    0,
		})
	}
	out := make([]map[string]string, 0, len(rows))
	for _, p := range rows {
		out = append(out, map[string]string{
			"name":     p.Name,
			"worktree": p.Worktree,
		})
	}
	return core.Ok(map[string]any{
		"projects": out,
		"count":    len(out),
	})
}

// serviceNotifyInvoke is the v1 stub for desktop-notification dispatch.
// Returns 501-shaped error per spec — pkg/notifications hasn't shipped
// yet (see Mantis #1405 pre-claim note 1647). Filed as a separate
// concern; the gateway slot is wired so when notifications lands the
// handler is a one-line replacement.
func serviceNotifyInvoke(_ *core.Core, _ string, _ *gin.Context) core.Result {
	return core.Fail(core.E("gateway.serviceNotifyInvoke",
		"pkg/notifications not yet implemented — file as a separate ticket if needed", nil))
}
