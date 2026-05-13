// SPDX-Licence-Identifier: EUPL-1.2

// Package server exposes lthn's HTTP API surface. OpenAI-compatible
// endpoints (/v1/chat/completions, /v1/completions, /v1/models) plus
// a liveness probe (/health), built on Gin per the Lethean design
// canon — core/api is Gin-based, lthn services follow.
//
// The Service holds a *gin.Engine. Same engine is exposed two ways:
//
//	Start(ctx)      → standalone HTTP listener on opts.Addr (lthn serve)
//	Handler()       → http.Handler for Wails Asset.Handler (lthn gui)
//
// The runner reference is optional. When unset, completion endpoints
// echo the prompt and /v1/models reports the static stub list. When
// set, requests are routed through the runner subsystem for real
// inference.
//
// Usage example:
//
//	c := core.New()
//	s := server.NewService(server.Options{Addr: ":8000"})
//	if r := s.Register(c); !r.OK {
//		return r
//	}
//	if r := s.Start(core.Background()); !r.OK {
//		return r
//	}
package server

import (
	"context"
	"net/http"

	core "dappco.re/go"
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

// Options configures the server at construction time.
type Options struct {
	// Addr is the bind address passed to ListenAndServe. Defaults to
	// ":8000" when empty.
	Addr string

	// Runner is the inference surface. When nil, completion endpoints
	// fall back to the echo stub so the binary still serves something
	// useful before go-mlx is wired.
	Runner Runner
}

// Service is the HTTP API subsystem.
type Service struct {
	opts   Options
	engine *gin.Engine
	http   *http.Server
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
//	s := server.NewService(server.Options{Addr: ":8000"})
//	s.Register(c)
func NewService(opts Options) *Service {
	if opts.Addr == "" {
		opts.Addr = ":8000"
	}
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	engine.HandleMethodNotAllowed = true // return 405 (not 404) on POST/GET mismatch
	engine.Use(gin.Recovery())
	s := &Service{opts: opts, engine: engine}
	s.routes()
	s.http = &http.Server{Addr: opts.Addr, Handler: engine}
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

// Handler returns the http.Handler that serves the OpenAI-compatible
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
func (s *Service) Handler() http.Handler {
	return s.engine
}

// Engine returns the underlying *gin.Engine for callers that need to
// register additional routes (frontend SPA fallback, custom verbs).
// pkg/desktop uses this to attach the SPA static handler.
//
// Usage example:
//
//	s := server.NewService(server.Options{})
//	s.Engine().NoRoute(spaHandler)
func (s *Service) Engine() *gin.Engine {
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
	if err == nil || err == http.ErrServerClosed {
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
	if err := s.http.Shutdown(context.Background()); err != nil {
		return core.Fail(err)
	}
	return core.Ok(nil)
}

// Register constructs a default server Service and wires it into the
// Core container. Mantis #1336 one-shot canonical entry.
//
// Usage example:
//
//	if r := server.Register(c); !r.OK {
//		return r
//	}
func Register(c *core.Core) core.Result {
	return NewService(Options{}).Register(c)
}
