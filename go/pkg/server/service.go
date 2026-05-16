// SPDX-Licence-Identifier: EUPL-1.2

// Package server exposes lthn's HTTP API surface as a thin wrapper
// around *coreapi.Engine — every byte that enters or leaves the
// lthn binary passes through the same middleware chain core/api
// applies upstream: bearer-auth, SSRF guard, sunset, cache, tracing,
// codegen.
//
// Two ways the same engine is used:
//
//	Start(ctx)      → standalone HTTP listener on opts.Addr (lthn serve)
//	Handler()       → core.Handler for Wails Asset.Handler (lthn gui)
//
// The runner reference is optional. When unset, completion endpoints
// echo the prompt and /v1/models reports the static stub list. When
// set, requests are routed through the runner subsystem for real
// inference.
//
// Auth: when Options.LocalKey is non-empty, every request must
// present `Authorization: Bearer <localKey>`. Health and Swagger
// remain public (core/api's WithBearerAuth handles the skip list).
//
// Usage example:
//
//	c := core.New()
//	s := server.NewService(server.Options{
//	    Addr:     ":8000",
//	    Runner:   r,
//	    LocalKey: "sk-lthn-…",  // set after pkg/apikey resolves
//	})
//	if r := s.Register(c); !r.OK { return r }
//	if r := s.Start(core.Background()); !r.OK { return r }
package server

import (
	"net/http"

	core "dappco.re/go"
	coreapi "dappco.re/go/api"
	"dappco.re/lthn/desktop/pkg/serverkey"
	"github.com/gin-gonic/gin"
)

// Runner is the optional inference surface the server consumes for
// completion endpoints. Decoupled so server ships without go-mlx
// wiring being live.
type Runner interface {
	// Generate returns the assistant reply for prompt. Implementations
	// may stream; the server today consumes the full reply.
	Generate(prompt string) core.Result

	// Models returns the list of model identifiers available to the
	// runner. Each entry maps to a directory under ~/Lethean/conf/models/.
	Models() core.Result
}

// Brand describes the API surface for Swagger / OpenAPI consumers.
// Used by Options.Brand to populate the spec metadata block.
type Brand struct {
	Title       string // e.g. "lthn API"
	Description string // e.g. "Local Lethean surface — chat, MCP tools, integrations"
	Version     string // e.g. "v0.2.0-rc1" — typically desktop.Version
}

// Options configures the server at construction time.
type Options struct {
	// Addr is the bind address passed to ListenAndServe. Defaults to
	// ":8000" when empty.
	Addr string

	// Runner is the inference surface. When nil, completion endpoints
	// fall back to the echo stub so the binary still serves something
	// useful before go-mlx is wired.
	Runner Runner

	// LocalKey is the per-Mac bearer token clients must present on
	// every request. Empty means auth is disabled (open server —
	// only safe in dev or behind a trusted reverse proxy).
	LocalKey string

	// SPAHandler is invoked when no registered route matches the
	// incoming request. Typically rewrites unknown GETs to the
	// embedded SPA index.html. Nil means gin's default 404.
	SPAHandler gin.HandlerFunc

	// Brand seeds the Swagger / OpenAPI metadata block. Empty fields
	// fall back to "lthn API" / "Local Lethean surface" / "0.0.0".
	Brand Brand

	// DisableSpec disables the /openapi.json endpoint. On by default
	// because spec exposure is the prerequisite for SDK clients to
	// auto-discover the surface.
	DisableSpec bool

	// DisableSDKGen disables the /sdk/* codegen endpoints. On by
	// default — coreapi.WithSDKGen mounts the multi-language codegen
	// surface so SDK clients can be generated from the live spec.
	DisableSDKGen bool

	// DisableSwagger disables the /swagger/* UI. On by default
	// because the human-discoverable surface is one of the v0.8
	// release's headline wins.
	DisableSwagger bool

	// RateLimit caps requests per second per client when > 0. 0 (the
	// default) leaves rate limiting off — fine for desktop where
	// only the user's clients connect; web-host builds set this.
	RateLimit int

	// TracingName enables OpenTelemetry tracing under the given
	// service name when non-empty. "" means tracing is off.
	TracingName string

	// ExtraGroups are coreapi.RouteGroup implementations to register
	// onto the engine before its Handler is materialised. Required
	// for late registrations (e.g. plugin proxy, opencode proxy +
	// control) since Engine.Handler() snapshots the route tree at
	// construction time — appending groups after NewService has
	// no effect on the live core.HTTPServer.
	ExtraGroups []coreapi.RouteGroup

	// Core, when non-nil, lets the server auto-discover RouteGroups
	// from every registered Core service that implements RoutesProvider
	// (i.e. exposes a `RouteGroups() []coreapi.RouteGroup` method). The
	// auto-discovered groups are mounted alongside ExtraGroups before
	// Engine.Handler() snapshots — the canonical path for service-
	// owned API surfaces without accumulating in cmd/lthn.
	Core *core.Core

	// WebViewOnlyGroups names ADDITIONAL RouteGroups (by
	// RouteGroup.Name()) that must NOT mount on the public HTTP engine.
	// Auto-discovered groups whose Name() is in this list — or in the
	// built-in defaultWebViewOnlyGroups set — mount on a separate
	// engine reachable only through Handler() (the surface exposed to
	// the Wails Asset.Handler). The standalone `lthn serve` HTTP
	// listener does not host these routes.
	//
	// Mantis #1449 Path 3 — the lthn-process RouteGroup is always
	// filtered (built-in default). process.run / process.start /
	// process.kill expose arbitrary command execution; combined with
	// any bearer leak (XSS, apikey-file read, plugin proxy, timing
	// oracle) the surface becomes RCE as the user. The WebView is the
	// only legitimate HTTP consumer (in-process Go and MCP callers
	// dispatch via Core actions, not REST); restricting the mount to
	// the Wails Asset.Handler keeps the WebView functional while
	// removing every external HTTP-client reach path.
	//
	// Use this field when future RouteGroups need the same protection
	// without bumping the built-in defaults (e.g. an early-access
	// admin surface, a debug RouteGroup that should never be public):
	//
	//	server.Options{
	//	    Core:              c,
	//	    WebViewOnlyGroups: []string{"lthn-admin-debug"},
	//	}
	//
	// Tests that need to OPT OUT of the built-in lthn-process filter
	// (to verify pre-#1449 behaviour) set DisableDefaultWebViewOnly.
	WebViewOnlyGroups []string

	// DisableDefaultWebViewOnly skips the built-in WebView-only filter
	// set. Tests use this to verify the legacy single-engine mount
	// path. Production callers should NEVER set this — Mantis #1449
	// Path 3 reach reduction depends on the default filter being
	// active.
	DisableDefaultWebViewOnly bool

	// ServerKey is the bootstrap-token verifier produced by
	// pkg/serverkey.Service. When non-nil, NewService installs the
	// combined bootstrap-plus-bearer middleware (WithBootstrapAuth)
	// instead of the standalone bearer middleware, gating the paths
	// in BootstrapPathScopes with verifier.VerifyBootstrapToken. When
	// nil, the bearer-only middleware is installed (pre-Stage-B
	// behaviour) and any /v1/account/create request 401s on the
	// missing bootstrap header.
	//
	// Stage B' (Mantis #1474 successor) wires this from cmd/lthn/app.go
	// via core.ServiceFor[*serverkey.Service](c, "serverkey") so the
	// same verifier the Wails surface uses (ServerKey.IssueBootstrapToken)
	// is the one the HTTP middleware accepts tokens against.
	ServerKey serverkey.Verifier
}

// BootstrapPathScopes names every HTTP path the bootstrap-auth
// middleware gates, mapped to its required token scope. Adding an
// entry here is a security-policy decision per Cerberus #1467 —
// each path is bound to exactly one scope, so a token minted for
// scope X cannot satisfy a different path's scope, even if both
// paths are in the allowlist. New entries REQUIRE a new Mantis
// ticket + new Cerberus DREAD review.
//
// Today's single entry: /v1/account/create requires a token minted
// with scope=account.create. pkg/account.APIBasePath + "/create"
// is the canonical path and MUST match this map's key verbatim.
//
// Usage example (inside NewService):
//
//	apiOpts = append(apiOpts,
//	    WithBootstrapAuth(opts.ServerKey, opts.LocalKey, BootstrapPathScopes))
var BootstrapPathScopes = map[string]string{
	"/v1/account/create": "account.create",
}

// RoutesProvider is the contract a Core-registered service implements to
// publish its HTTP RouteGroups to the lthn server. Service.NewService
// iterates every registered service at construction time and mounts the
// groups returned by anything that satisfies this interface.
//
// Usage example:
//
//	func (s *Service) RouteGroups() []coreapi.RouteGroup {
//	    return []coreapi.RouteGroup{NewAPIProvider(s.coreproc)}
//	}
type RoutesProvider interface {
	RouteGroups() []coreapi.RouteGroup
}

// Service is the HTTP API subsystem. Wraps a *coreapi.Engine so the
// canonical middleware chain (auth, SSRF, sunset, cache, tracing,
// codegen) applies to every route lthn registers.
//
// Two engines today (Mantis #1449 Path 3):
//
//   - engine — the public engine. Hosts every RouteGroup except those
//     whose Name() appears in opts.WebViewOnlyGroups. Bound by Start()
//     to the standalone HTTP listener for `lthn serve`.
//   - webviewEngine — the WebView-only engine. Hosts the filtered-out
//     groups. Reachable solely through Handler(), which composes both
//     so the Wails Asset.Handler sees the union and the standalone
//     listener sees only the public surface.
//
// When opts.WebViewOnlyGroups is empty (default for non-desktop
// callers), webviewEngine is nil and Handler() returns the public
// engine's handler verbatim — pre-#1449 behaviour preserved.
type Service struct {
	opts          Options
	engine        *coreapi.Engine
	webviewEngine *coreapi.Engine
	http          *core.HTTPServer
	// listening tracks whether Start() has an active ListenAndServe
	// running. Read by the Wails surface (WListening()) so the WebView
	// can show the "HTTP server" toggle in its real state — false in
	// desktop mode (the gin engine is mounted on Wails' AssetServer
	// instead), true after `lthn serve` is invoked.
	listening bool
}

// NewService constructs the server with the canonical shape.
//
// Usage example:
//
//	s := server.NewService(server.Options{Addr: ":8000", LocalKey: key})
//	s.Register(c)
func NewService(opts Options) *Service {
	if opts.Addr == "" {
		opts.Addr = ":8000"
	}
	gin.SetMode(gin.ReleaseMode)

	brand := opts.Brand
	if brand.Title == "" {
		brand.Title = "lthn API"
	}
	if brand.Description == "" {
		brand.Description = "Local Lethean surface — chat, MCP tools, integrations"
	}
	if brand.Version == "" {
		brand.Version = "0.0.0"
	}

	apiOpts := []coreapi.Option{
		coreapi.WithAddr(opts.Addr),
		coreapi.WithRequestID(),
		coreapi.WithResponseMeta(),
		// Cerberus Mantis #1422 (2026-05-16) — CSP defence-in-depth.
		//
		// Primary defence: Lit auto-escapes all interpolated content in
		// html`...` templates — a raw <script> string becomes the literal
		// six-character sequence, never an executable element.
		//
		// This CSP is the second layer: if an unexpected sink (unsafeHTML
		// misuse, a new component bypassing Lit's escape, or a future
		// innerHTML write carrying external content) ever reaches the
		// browser, the policy refuses inline-script execution and eval().
		//
		// style-src 'unsafe-inline': Lit emits inline <style> inside shadow
		// roots. The standards-compliant alternative is nonce-based CSP for
		// styles, which requires coordinating a per-request nonce between
		// the Go server and the Vite build. The complexity cost is not
		// justified today — 'unsafe-inline' on styles only allows injected
		// styles, not injected scripts. Revisit if a nonce pipeline lands.
		//
		// connect-src: 127.0.0.1:9879 is the bridge (tools), 9876 is the
		// mdns / service-discovery port. huggingface.co + hf.co are for the
		// model browser's HF API calls. wss: is included for future WebSocket
		// transports on the same localhost origins.
		//
		// The Wails runtime injects its JS via the native WebView runtime,
		// not via <script> tags — webview_eval bypass is unaffected by CSP.
		coreapi.WithMiddleware(cspMiddleware()),
	}
	// Cerberus Mantis #1430 (2026-05-16) — bearer auth re-enabled. The
	// WebView fetch interceptor lives at frontend/src/lit/api-fetch.ts;
	// it loads the token via apikey.Reveal() on first call + injects
	// Authorization: Bearer on every same-origin API request. Without
	// this, every local process can reach every API endpoint without
	// authentication.
	//
	// Stage B' wiring (Mantis #1474 successor): when ServerKey is
	// supplied, the combined bootstrap-plus-bearer middleware replaces
	// the standalone bearer — the same chain entry handles both auth
	// schemes so /v1/account/create can be gated by a bootstrap-token
	// while every other route keeps its normal bearer requirement.
	// When ServerKey is nil, fall back to plain bearer auth (pre-
	// Stage-B behaviour) so non-desktop callers (CLI, tests) still
	// authenticate correctly.
	if opts.ServerKey != nil {
		apiOpts = append(apiOpts,
			WithBootstrapAuth(opts.ServerKey, opts.LocalKey, BootstrapPathScopes))
	} else if opts.LocalKey != "" {
		apiOpts = append(apiOpts, coreapi.WithBearerAuth(opts.LocalKey))
	}
	if opts.SPAHandler != nil {
		apiOpts = append(apiOpts, coreapi.WithNoRoute(opts.SPAHandler))
	}
	if !opts.DisableSpec {
		apiOpts = append(apiOpts, coreapi.WithOpenAPISpec())
	}
	if !opts.DisableSDKGen {
		apiOpts = append(apiOpts, coreapi.WithSDKGen())
	}
	if !opts.DisableSwagger {
		apiOpts = append(apiOpts,
			coreapi.WithSwagger(brand.Title, brand.Description, brand.Version),
		)
	}
	if opts.RateLimit > 0 {
		apiOpts = append(apiOpts, coreapi.WithRateLimit(opts.RateLimit))
	}
	if opts.TracingName != "" {
		apiOpts = append(apiOpts, coreapi.WithTracing(opts.TracingName))
	}

	engine, _ := coreapi.New(apiOpts...) // current New always returns nil err
	s := &Service{opts: opts, engine: engine}
	engine.Register(newLthnRoutes(s))
	for _, g := range opts.ExtraGroups {
		engine.Register(g)
	}
	// Build the WebView-only engine when the resolved filter has any
	// entries (either from opts.WebViewOnlyGroups or the built-in
	// defaultWebViewOnlyGroups list — Mantis #1449 Path 3). Runs with
	// the same middleware chain (bearer-auth, CSP, etc.) so a stolen
	// bearer still authenticates, but the routes simply aren't
	// reachable from outside the Wails Asset.Handler.
	webviewOnly := resolveWebViewOnly(opts)
	if len(webviewOnly) > 0 {
		s.webviewEngine, _ = coreapi.New(apiOpts...)
	}
	// Auto-discover RouteGroups from registered Core services. Each
	// service that owns an API surface implements RoutesProvider so
	// the route declaration lives next to the service instead of
	// accumulating in cmd/lthn.
	//
	// Groups whose Name() appears in opts.WebViewOnlyGroups route to
	// s.webviewEngine instead of s.engine — Mantis #1449 Path 3 reach
	// reduction. Groups not in the set keep their pre-#1449 mount on
	// the public engine.
	if opts.Core != nil {
		for _, name := range opts.Core.Services() {
			r := opts.Core.Service(name)
			if !r.OK || r.Value == nil {
				continue
			}
			provider, ok := r.Value.(RoutesProvider)
			if !ok {
				continue
			}
			for _, g := range provider.RouteGroups() {
				if g == nil {
					continue
				}
				if webviewOnly[g.Name()] && s.webviewEngine != nil {
					s.webviewEngine.Register(g)
					continue
				}
				engine.Register(g)
			}
		}
	}
	s.http = &core.HTTPServer{Addr: opts.Addr, Handler: engine.Handler()}
	return s
}

// defaultWebViewOnlyGroups names the RouteGroups that ship with
// always-on WebView-only mounting. The lthn-process group's identifier
// is duplicated here (rather than imported from pkg/process) to keep
// pkg/server free of a back-edge to the consumer packages it auto-
// discovers — the same reason RoutesProvider is satisfied by interface
// shape rather than concrete type.
//
// Adding to this list is a security-policy decision: every name here
// becomes unreachable from `lthn serve` HTTP listeners. New entries
// should land via Mantis ticket + Athena routing.
//
// Mantis #1449 Path 3 — "lthn-process" is the load-bearing entry. It
// matches the constant pkg/process exports as GroupName.
var defaultWebViewOnlyGroups = []string{
	"lthn-process",
}

// resolveWebViewOnly merges opts.WebViewOnlyGroups with
// defaultWebViewOnlyGroups (unless opts.DisableDefaultWebViewOnly is
// set) and returns a set-lookup map. Returns nil when both inputs are
// empty so callers can branch on `len(map) == 0`.
//
// Usage example:
//
//	if resolveWebViewOnly(opts)[g.Name()] { /* webview-only mount */ }
func resolveWebViewOnly(opts Options) map[string]bool {
	if opts.DisableDefaultWebViewOnly && len(opts.WebViewOnlyGroups) == 0 {
		return nil
	}
	out := make(map[string]bool, len(opts.WebViewOnlyGroups)+len(defaultWebViewOnlyGroups))
	if !opts.DisableDefaultWebViewOnly {
		for _, n := range defaultWebViewOnlyGroups {
			if n != "" {
				out[n] = true
			}
		}
	}
	for _, n := range opts.WebViewOnlyGroups {
		if n == "" {
			continue
		}
		out[n] = true
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// Register wires the server service into the Core container. Mantis
// #1336 canonical shape. Today this is a no-op — the server is
// driven by Start/Stop rather than the Core action bus — but the
// canonical entry stays so future action wiring (e.g. exposing
// server.status as a core action) has a home.
//
// Usage example:
//
//	if r := s.Register(c); !r.OK {
//		return r
//	}
func (s *Service) Register(c *core.Core) core.Result {
	return core.Ok(nil)
}

// Handler returns the core.Handler the Wails Asset.Handler mounts.
// When opts.WebViewOnlyGroups is empty this is the public engine's
// handler verbatim (pre-#1449 behaviour). When at least one group is
// flagged WebView-only, Handler returns a composite handler that
// dispatches requests whose path matches a WebView-only group's
// BasePath to the webview engine and routes everything else to the
// public engine — so the WebView keeps seeing the union of routes
// while the standalone HTTP listener (Start()) only sees the public
// surface.
//
// Mantis #1449 Path 3 — process REST mounts here so the WebView's
// same-origin /v1/api/process/* fetches still resolve, but an external
// curl against `lthn serve`'s listener returns 404.
//
// Usage example:
//
//	s := server.NewService(server.Options{
//	    Runner:            r,
//	    WebViewOnlyGroups: []string{process.GroupName},
//	})
//	app := application.New(application.Options{
//	    Assets: application.AssetOptions{Handler: s.Handler()},
//	})
func (s *Service) Handler() core.Handler {
	public := s.engine.Handler()
	if s.webviewEngine == nil {
		return public
	}
	return newWebViewComposite(public, s.webviewEngine.Handler(), s.webviewOnlyPrefixes())
}

// Engine returns the public *coreapi.Engine for callers that need to
// register additional RouteGroups, StreamGroups, or attach a fallback.
// pkg/desktop uses this to mount subsystem routes onto the same
// canonical public engine.
//
// Groups registered here mount on the standalone HTTP listener too —
// use WebViewEngine() for WebView-only mounts (Mantis #1449).
//
// Usage example:
//
//	s := server.NewService(server.Options{})
//	s.Engine().Register(myRouteGroup{})
func (s *Service) Engine() *coreapi.Engine {
	return s.engine
}

// WebViewEngine returns the WebView-only *coreapi.Engine, or nil when
// opts.WebViewOnlyGroups is empty. Routes registered on this engine
// are reachable only through Handler() — the standalone HTTP listener
// (Start()) ignores them. Mantis #1449 Path 3.
//
// Usage example:
//
//	if eng := s.WebViewEngine(); eng != nil {
//	    eng.Register(mySensitiveGroup{})
//	}
func (s *Service) WebViewEngine() *coreapi.Engine {
	return s.webviewEngine
}

// webviewOnlyPrefixes returns the BasePaths of every RouteGroup
// currently mounted on the webview engine. Used by Handler()'s
// composite to decide which incoming request paths go to the webview
// engine instead of the public engine. Each prefix is taken verbatim
// from RouteGroup.BasePath() — gin route trees match by exact prefix
// so the same comparison applies here.
func (s *Service) webviewOnlyPrefixes() []string {
	if s.webviewEngine == nil {
		return nil
	}
	groups := s.webviewEngine.Groups()
	out := make([]string, 0, len(groups))
	for _, g := range groups {
		if g == nil {
			continue
		}
		bp := g.BasePath()
		if bp == "" {
			continue
		}
		out = append(out, bp)
	}
	return out
}

// webViewComposite is the http.Handler returned by Handler() when at
// least one RouteGroup mounted on the webview engine. Dispatches
// requests whose path starts with any of the webview-only prefixes to
// the webview handler; routes everything else (including unknown
// paths) to the public handler.
//
// The fallthrough means the public handler still owns the SPA fallback
// and 404 path — the webview composite only diverts traffic it knows
// belongs to a webview-only group.
type webViewComposite struct {
	public          http.Handler
	webview         http.Handler
	webviewPrefixes []string
}

// newWebViewComposite returns a composite handler that diverts
// webview-only paths to webview and falls through to public for
// everything else. prefixes are the BasePath() values of the webview
// engine's RouteGroups; a path matches a prefix when it equals the
// prefix or starts with prefix + "/".
func newWebViewComposite(public, webview http.Handler, prefixes []string) http.Handler {
	return &webViewComposite{
		public:          public,
		webview:         webview,
		webviewPrefixes: prefixes,
	}
}

// ServeHTTP routes the request to the webview handler when the path
// matches any of the webview-only BasePath prefixes, otherwise to the
// public handler.
func (c *webViewComposite) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	for _, p := range c.webviewPrefixes {
		if pathHasPrefix(r.URL.Path, p) {
			c.webview.ServeHTTP(w, r)
			return
		}
	}
	c.public.ServeHTTP(w, r)
}

// pathHasPrefix reports whether path equals prefix or starts with
// prefix + "/". Avoids the trailing-slash mismatch that bites when
// BasePath is "/v1/api/process" and the incoming request is
// "/v1/api/process/processes/list".
//
// Usage example:
//
//	if pathHasPrefix("/v1/api/process/list", "/v1/api/process") { ... }
func pathHasPrefix(path, prefix string) bool {
	if !core.HasPrefix(path, prefix) {
		return false
	}
	if len(path) == len(prefix) {
		return true
	}
	return path[len(prefix)] == '/'
}

// Start begins serving HTTP on the configured Addr. Blocks until the
// listener errors or Stop is called. Returns core.Ok(nil) on graceful
// shutdown, core.Fail(err) otherwise.
//
// Usage example:
//
//	go func() { _ = s.Start(core.Background()) }()
func (s *Service) Start(_ core.Context) core.Result {
	core.Print(core.Stdout(), "lthn serve: listening on %s\n", s.opts.Addr)
	s.listening = true
	err := s.http.ListenAndServe()
	s.listening = false
	if err == nil || err == core.ErrHTTPServerClosed {
		return core.Ok(nil)
	}
	return core.Fail(err)
}

// Stop gracefully shuts the server down. Uses a background context
// today — graceful-shutdown deadlines wire later via Options.
//
// Usage example:
//
//	_ = s.Stop(core.Background())
func (s *Service) Stop(_ core.Context) core.Result {
	if err := s.http.Shutdown(core.Background()); err != nil {
		return core.Fail(err)
	}
	return core.Ok(nil)
}

// Register is the zero-option core.WithName-compatible factory. Builds
// the server with default Options and the supplied Core so route-group
// auto-discovery can resolve registered RoutesProvider services.
//
// Use RegisterService(opts) when the caller needs to bind a Runner /
// LocalKey / Brand / Addr / ExtraGroups at registration time — those
// can't be set after construction because Engine.Handler() snapshots
// the route tree on first call.
//
// Usage example:
//
//	core.New(core.WithName("server", server.Register))
func Register(c *core.Core) core.Result {
	return core.Ok(NewService(Options{Core: c}))
}

// RegisterService is the options-binding factory used by core.WithName
// when the caller needs to pre-configure the server (Runner / LocalKey
// / Addr / Brand / ExtraGroups). Closes over opts so options reach the
// service before Engine.Handler() snapshots routes.
//
// Usage example:
//
//	core.New(core.WithName("server", server.RegisterService(server.Options{
//	    Runner: r, LocalKey: key, Brand: server.Brand{Version: v},
//	})))
func RegisterService(opts Options) func(*core.Core) core.Result {
	return func(c *core.Core) core.Result {
		opts.Core = c
		return core.Ok(NewService(opts))
	}
}
