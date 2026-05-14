// SPDX-Licence-Identifier: EUPL-1.2

// Package bridge — local MCP HTTP server that lets an external agent
// (Cladius / Codex / any MCP-speaking client) drive and observe the
// running lthn/desktop WebView. Ported down from core/ide's
// mcp_bridge.go to the minimum-viable surface lthn/desktop needs in
// dev mode: console + error capture, webview_eval with fetch-back,
// and a handful of pre-shaped JS one-liners that build on eval.
//
// The bridge is dev-mode-focused — it binds 127.0.0.1:9876 so it's
// never exposed beyond the local machine. Production builds can skip
// registration entirely by branching in pkg/desktop/desktop.go's
// wailsServices list.
//
// Why a separate HTTP server (not a /api/ route on the gin engine):
// the bridge is intentionally OUT-OF-PROCESS-shaped so a stand-alone
// MCP client can reach it without going through Wails routing.
// /internal/console + /internal/error are POSTed by the JS shim
// loaded in frontend/index.html on every window load.
//
// Endpoints:
//   GET  /mcp/info             — bridge metadata + version
//   GET  /mcp/tools            — tool catalogue
//   POST /mcp/call             — { tool, params } → { ok, value, error }
//   GET  /health               — liveness probe
//   POST /internal/console     — JS shim → ring buffer
//   POST /internal/error       — JS shim → ring buffer
//   POST /internal/eval-reply  — JS fetch-back from a wrapped eval
//
// Usage example (Core registration):
//
//	c := core.New(
//	    core.WithName("bridge", bridge.RegisterService(bridge.Options{Port: 9876})),
//	)

package bridge

import (
	"context"
	"net/http"
	"sync"
	"time"

	core "dappco.re/go"
)

// DefaultPort is the bridge HTTP port. Picked to differ from
// core/ide's 9877 (and JetBrains products bind 9876 by convention)
// so all three can coexist on the same dev machine.
const DefaultPort = 9879

// DefaultWindow is the Wails window the bridge ExecJS's into when
// no window param is supplied. "tray" is the only always-present
// surface in lthn/desktop today.
const DefaultWindow = "tray"

// consoleBufLimit caps the ring buffer for console + error events.
const consoleBufLimit = 1000

// Options configures the bridge HTTP server.
//
// Usage example:
//
//	core.WithName("bridge", bridge.RegisterService(bridge.Options{Port: 9876}))
type Options struct {
	// Port is the HTTP port the bridge binds. Default: 9876.
	Port int
}

// ConsoleEntry mirrors the JSON shape the JS shim POSTs to
// /internal/console.
type ConsoleEntry struct {
	Level   string    `json:"level"`
	Message string    `json:"message"`
	Source  string    `json:"source,omitempty"`
	At      time.Time `json:"at"`
}

// ErrorEntry mirrors the JSON shape the JS shim POSTs to
// /internal/error.
type ErrorEntry struct {
	Message string    `json:"message"`
	Source  string    `json:"source,omitempty"`
	Line    int       `json:"line,omitempty"`
	Col     int       `json:"col,omitempty"`
	Stack   string    `json:"stack,omitempty"`
	At      time.Time `json:"at"`
}

// evalReply pairs a request id with the JS execution result (or
// error). The JS shim's fetch-back wrapper posts one of these per
// eval call to /internal/eval-reply.
type evalReply struct {
	ReqID  string `json:"reqId"`
	Result any    `json:"result,omitempty"`
	Error  string `json:"error,omitempty"`
}

// Service is the bridge runtime. Holds the HTTP server, ring buffers,
// and the pending-eval map.
type Service struct {
	*core.ServiceRuntime[Options]

	mu      sync.Mutex
	httpSrv *http.Server
	port    int

	consoleMu  sync.Mutex
	consoleBuf []ConsoleEntry

	errorMu  sync.Mutex
	errorBuf []ErrorEntry

	evalMu       sync.Mutex
	evalCounter  uint64
	pendingEvals map[string]chan evalReply
}

// RegisterService returns a Core service factory for the bridge. The
// factory shape matches every other Core service so registration
// fits the standard `core.WithName(...)` pattern.
func RegisterService(opts Options) func(*core.Core) core.Result {
	return func(c *core.Core) core.Result {
		if opts.Port == 0 {
			opts.Port = DefaultPort
		}
		s := &Service{
			ServiceRuntime: core.NewServiceRuntime[Options](c, opts),
			port:           opts.Port,
			pendingEvals:   make(map[string]chan evalReply),
		}
		return core.Ok(s)
	}
}

// OnStartup starts the bridge HTTP server.
func (s *Service) OnStartup(_ context.Context) core.Result {
	mux := http.NewServeMux()
	mux.HandleFunc("/mcp/info", s.handleInfo)
	mux.HandleFunc("/mcp/tools", s.handleTools)
	mux.HandleFunc("/mcp/call", s.handleCall)
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/internal/console", s.handleInternalConsole)
	mux.HandleFunc("/internal/error", s.handleInternalError)
	mux.HandleFunc("/internal/eval-reply", s.handleInternalEvalReply)

	s.mu.Lock()
	s.httpSrv = &http.Server{
		Addr:              core.Sprintf("127.0.0.1:%d", s.port),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	srv := s.httpSrv
	s.mu.Unlock()

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			core.Print(core.Stderr(), "bridge: http listen failed: %v\n", err)
		}
	}()
	return core.Ok(nil)
}

// OnShutdown stops the bridge HTTP server gracefully.
func (s *Service) OnShutdown(ctx context.Context) core.Result {
	s.mu.Lock()
	srv := s.httpSrv
	s.mu.Unlock()
	if srv == nil {
		return core.Ok(nil)
	}
	if err := srv.Shutdown(ctx); err != nil {
		return core.Fail(core.E("bridge.OnShutdown", "shutdown failed", err))
	}
	return core.Ok(nil)
}

// Port returns the bound bridge port — useful for the JS shim
// template + health probes.
func (s *Service) Port() int { return s.port }
