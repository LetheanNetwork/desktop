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
	core "dappco.re/go"
	coreapi "dappco.re/go/api"
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
type Service struct {
	opts   Options
	engine *coreapi.Engine
	http   *core.HTTPServer
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
	}
	// Cerberus Mantis #1430 (2026-05-16) — bearer auth re-enabled. The
	// WebView fetch interceptor lives at frontend/src/lit/api-fetch.ts;
	// it loads the token via apikey.Reveal() on first call + injects
	// Authorization: Bearer on every same-origin API request. Without
	// this, every local process can reach every API endpoint without
	// authentication.
	if opts.LocalKey != "" {
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
	// Auto-discover RouteGroups from registered Core services. Each
	// service that owns an API surface implements RoutesProvider so
	// the route declaration lives next to the service instead of
	// accumulating in cmd/lthn.
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
				if g != nil {
					engine.Register(g)
				}
			}
		}
	}
	s.http = &core.HTTPServer{Addr: opts.Addr, Handler: engine.Handler()}
	return s
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

// Handler returns the core.Handler that serves the OpenAI-compatible
// surface. Exposed so consumers (pkg/desktop's Wails Asset.Handler)
// can mount the same routes the standalone `lthn serve` exposes
// inside the WebView origin — no CORS, no port hunting.
//
// Usage example:
//
//	s := server.NewService(server.Options{Runner: r})
//	app := application.New(application.Options{
//	    Assets: application.AssetOptions{Handler: s.Handler()},
//	})
func (s *Service) Handler() core.Handler {
	return s.engine.Handler()
}

// Engine returns the underlying *coreapi.Engine for callers that need
// to register additional RouteGroups, StreamGroups, or attach a
// fallback. pkg/desktop uses this to mount subsystem routes onto the
// same canonical engine.
//
// Usage example:
//
//	s := server.NewService(server.Options{})
//	s.Engine().Register(myRouteGroup{})
func (s *Service) Engine() *coreapi.Engine {
	return s.engine
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
