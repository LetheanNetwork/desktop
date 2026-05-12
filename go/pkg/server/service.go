// SPDX-Licence-Identifier: EUPL-1.2

// Package server exposes lthn's HTTP API surface. OpenAI-compatible
// endpoints (/v1/chat/completions, /v1/completions, /v1/models) plus
// a liveness probe (/health). The Service wraps core.HTTPServer +
// core.ServeMux behind the canonical Mantis #1336 shape, so consumers
// can wire it into a Core container or run it standalone via Start.
//
// The runner reference is optional. When unset, completion endpoints
// echo the prompt and /v1/models reports the static stub list. When
// set, requests are routed through the runner subsystem for real
// inference. Decoupling lets `lthn serve` ship before go-mlx wiring
// lands.
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
	core "dappco.re/go"
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
	opts Options
	mux  *core.ServeMux
	http *core.HTTPServer
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
	mux := &core.ServeMux{}
	s := &Service{opts: opts, mux: mux}
	s.routes()
	s.http = &core.HTTPServer{Addr: opts.Addr, Handler: mux}
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

// Start begins serving HTTP on the configured Addr. Blocks until the
// listener errors or Stop is called. Returns core.Ok(nil) on graceful
// shutdown, core.Fail(err) otherwise.
//
// Usage example:
//
//	go func() { _ = s.Start(core.Background()) }()
func (s *Service) Start(ctx core.Context) core.Result {
	core.Print(core.Stdout(), "lthn serve: listening on %s\n", s.opts.Addr)
	err := s.http.ListenAndServe()
	if err == nil {
		return core.Ok(nil)
	}
	// http.ErrServerClosed is the expected outcome of a graceful Stop.
	if err.Error() == "http: Server closed" {
		return core.Ok(nil)
	}
	return core.Fail(err)
}

// Stop gracefully shuts the server down within the context deadline.
//
// Usage example:
//
//	ctx, cancel := core.WithTimeout(core.Background(), 5*core.Second)
//	defer cancel()
//	_ = s.Stop(ctx)
func (s *Service) Stop(ctx core.Context) core.Result {
	err := s.http.Shutdown(ctx)
	if err != nil {
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
