// SPDX-Licence-Identifier: EUPL-1.2

// Package bridge provides the retained local MCP HTTP compatibility server.
// The default desktop composition now uses Wails' build-tagged MCP service and
// does not register this package. Explicit fallback consumers can still wire it
// with RegisterService without reviving a second server by default. It was
// ported down from core/ide's
// mcp_bridge.go to the minimum-viable surface lthn/desktop needs in
// compatibility mode: console + error capture, webview_eval with fetch-back,
// and a handful of pre-shaped JS one-liners that build on eval.
//
// The bridge is dev-mode-focused — it binds 127.0.0.1:9879 so it's
// never exposed beyond the local machine. Production builds can skip
// registration entirely; lthn's default composition does so.
//
// Why a separate HTTP server (not a /api/ route on the gin engine):
// the bridge is intentionally OUT-OF-PROCESS-shaped so a stand-alone
// MCP client can reach it without going through Wails routing.
// Console + error capture from the bundled WebView lands via the
// Wails Events bus (post-A5) — the JS shim in frontend/index.html
// emits "lthn:console" / "lthn:error" events and the bridge
// subscribes via core/gui's CustomEventBinder surface at startup.
//
// Endpoints:
//   GET  /mcp/info             — bridge metadata + version
//   GET  /mcp/tools            — tool catalogue
//   POST /mcp/call             — { tool, params } → { ok, value, error }
//   GET  /health               — liveness probe
//
// (The whole /internal/* HTTP surface — console, error, eval-reply
// — retired in favour of core/gui's TaskEvalJS event-bus path. The
// JS shim emits via window.wails.Events.Emit("lthn:console"/"lthn:
// error") and the bridge subscribes via windowSvc.SubscribeEvent in
// OnStartup. One transport, no CORS, no DNS-rebind defence, no
// originAllowed gate on those paths.)
//
// Usage example (Core registration):
//
//	c := core.New(
//	    core.WithName("bridge", bridge.RegisterService(bridge.Options{Port: 9879})),
//	)

package bridge

import (
	core "dappco.re/go"
	guiwindow "dappco.re/go/render/display/webkit/pkg/window"
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

// Event names the WebView JS shim emits on. The bridge subscribes
// via windowSvc.SubscribeEvent in OnStartup; each event lands as
// map[string]any carrying the same field shape the retired
// /internal/console + /internal/error HTTP handlers used to parse.
const (
	eventConsoleName        = "lthn:console"
	eventErrorName          = "lthn:error"
	eventWebMCPToolsChanged = "lthn:webmcp:tools-changed"
)

// maxEntryMessageBytes is the post-decode clamp on the per-entry
// Message field. Pre-A5 the HTTP handlers had an additional
// http.MaxBytesReader gate before this clamp; A5 dropped the
// HTTP surface so this clamp is the only size guard now. 16 KiB
// covers any plausible real console line / error stack while
// still capping a hostile/looping emitter.
const maxEntryMessageBytes = 16 * 1024

// Options configures the bridge HTTP server.
//
// Usage example:
//
//	core.WithName("bridge", bridge.RegisterService(bridge.Options{Port: 9879}))
type Options struct {
	// Port is the HTTP port the bridge binds. Default: DefaultPort (9879).
	Port int
}

// ConsoleEntry mirrors the JSON shape the JS shim POSTs to
// /internal/console.
type ConsoleEntry struct {
	Level   string    `json:"level"`
	Message string    `json:"message"`
	Source  string    `json:"source,omitempty"`
	At      core.Time `json:"at"`
}

// ErrorEntry mirrors the JSON shape the JS shim POSTs to
// /internal/error.
type ErrorEntry struct {
	Message string    `json:"message"`
	Source  string    `json:"source,omitempty"`
	Line    int       `json:"line,omitempty"`
	Col     int       `json:"col,omitempty"`
	Stack   string    `json:"stack,omitempty"`
	At      core.Time `json:"at"`
}

// Service is the bridge runtime. Holds the HTTP server + ring
// buffers populated from "lthn:console" + "lthn:error" Wails
// events. The earlier pendingEvals/evalReply state retired in A5
// when the eval-reply path migrated fully to core/gui's
// TaskEvalJS event bus.
type Service struct {
	*core.ServiceRuntime[Options]

	mu      core.Mutex
	httpSrv *core.HTTPServer
	port    int

	consoleMu  core.Mutex
	consoleBuf []ConsoleEntry

	errorMu  core.Mutex
	errorBuf []ErrorEntry

	// Cerberus #1423 / Mantis 2026-05-16 — bearer-token auth.
	// Token is loaded or generated in OnStartup; held in memory so
	// requireAuth doesn't hit disk per-request.
	tokenMu core.Mutex
	token   string

	// webMCP mirrors Angular's document.modelContext tool registry onto the
	// native MCP server. Tool definitions arrive over the existing Wails event
	// bus; calls return through the existing window.eval_js transport.
	webMCP     *webMCPState
	webMCPCall webMCPCallFunc

	// subscribeTimeout / subscribePoll override subscribeToWebViewEvents'
	// default 10s deadline / 50ms poll cadence when non-zero. Test-only
	// seam (smallest-safe testability knob, no public API change) so
	// the "window" service registration wait — and its timeout branch —
	// can be exercised in milliseconds instead of full wall-clock
	// seconds. Zero value (the production default) keeps prior
	// behaviour identical.
	subscribeTimeout core.Duration
	subscribePoll    core.Duration
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
		}
		return core.Ok(s)
	}
}

// OnStartup starts the bridge HTTP server.
func (s *Service) OnStartup(_ core.Context) core.Result {
	// Cerberus #1423 — load/generate the bearer-token before
	// installing routes so requireAuth has the expected value
	// available on the first request.
	tokR := loadOrGenerateToken()
	if !tokR.OK {
		return core.Fail(core.E("bridge.OnStartup", "auth token init failed", tokR.Value.(error)))
	}
	tok, ok := tokR.Value.(string)
	if !ok || tok == "" {
		// Cast guard: if loadOrGenerateToken ever returns a non-string
		// Value on OK (contract drift) or an empty string, fail startup
		// loud rather than silently leaving s.token = "" and turning
		// every /mcp/* request into a 401 (which the empty-token check
		// in requireAuth would then enforce). Without this branch the
		// bridge "starts" but is permanently unreachable — symptom
		// would be \"the bridge worked yesterday, not today\" with no
		// log to correlate.
		return core.Fail(core.E("bridge.OnStartup",
			"auth token init returned non-string or empty value", nil))
	}
	s.tokenMu.Lock()
	s.token = tok
	s.tokenMu.Unlock()

	mux := core.NewServeMux()
	// Auth tiers (post-A5):
	//   - /health  : liveness probe (no auth — must work without the
	//     token so external uptime checks can poll).
	//   - /mcp/*   : requireAuth (bearer + Origin). The privilege-
	//     escalation surface (webview_eval lives here).
	//
	// The whole /internal/* HTTP surface retired in A5 (Mantis
	// `roll console + errors onto same postMessage channel`). Console
	// + error capture rides the Wails Events bus now — see the
	// SubscribeEvent block below.
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/mcp/info", s.requireAuth(s.handleInfo))
	mux.HandleFunc("/mcp/tools", s.requireAuth(s.handleTools))
	mux.HandleFunc("/mcp/call", s.requireAuth(s.handleCall))

	// A5 — subscribe to "lthn:console" + "lthn:error" Wails custom
	// events. The WebView's JS shim (frontend/index.html) emits via
	// window.wails.Events.Emit with the same {level,message,…} /
	// {message,source,line,col,stack} shapes the HTTP handlers used
	// to receive. SubscribeEvent returns false on platforms that
	// don't support custom-event binding (test MockPlatform); the
	// bridge is dev-mode-only so degraded operation is acceptable.
	//
	// The "window" service registers later than bridge (it's
	// instantiated by pkg/desktop.registerCoreGUI as a side effect
	// of Wails app startup, well after the WithName-list services
	// have all called OnStartup). Poll in a background goroutine
	// until it appears or 10s elapses — first iteration usually
	// succeeds within tens of ms once Wails finishes booting.
	s.Core().Go(s.subscribeToWebViewEvents)

	s.mu.Lock()
	s.httpSrv = &core.HTTPServer{
		Addr:              core.Sprintf("127.0.0.1:%d", s.port),
		Handler:           mux,
		ReadHeaderTimeout: 5 * core.Second,
	}
	srv := s.httpSrv
	s.mu.Unlock()

	s.Core().Go(func() {
		if err := srv.ListenAndServe(); err != nil && err != core.ErrHTTPServerClosed {
			core.Print(core.Stderr(), "bridge: http listen failed: %v\n", err)
		}
	})
	return core.Ok(nil)
}

// OnShutdown stops the bridge HTTP server gracefully.
func (s *Service) OnShutdown(ctx core.Context) core.Result {
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

// subscribeToWebViewEvents polls for the "window" service registration
// and, once it's up, subscribes to "lthn:console" + "lthn:error" via
// the CustomEventBinder surface added in core/gui A5. The polling
// loop runs at 50ms cadence for up to 10s — first iteration usually
// succeeds within tens of ms once Wails finishes booting.
//
// Designed to run inside s.Core().Go. Returns silently on success or
// timeout; the bridge is dev-only and a missing window service means
// console/error capture is degraded but the rest of the bridge
// (eval, navigate, query, mcp/*) keeps working.
// subscribeToWebViewEvents wires the WebView's posted console + error
// events directly into bridge.Service via window.Service.SubscribeEvent.
//
// Action-bus exception: this is the one place lthn/desktop holds a
// direct *guiwindow.Service pointer rather than dispatching via
// c.Action(...). The reason — SubscribeEvent is callback-shaped, and
// the callbacks (handleConsoleEvent / handleErrorEvent) need access to
// bridge.Service receiver state (the ring buffer for /mcp/console). An
// action-bus indirection would require: a "window.subscribe_event"
// task carrying a handler_action_id, the service emitting a
// per-event-name Message{Data}, and bridge.Service registering an
// action handler that re-routes to the right method. Three extra hops
// for no real decoupling — the bridge already has a hard dependency on
// the window service for /mcp/eval / /mcp/console parity.
//
// GUI-8 audit (2026-05-27): kept as-is.
func (s *Service) subscribeToWebViewEvents() {
	timeout := s.subscribeTimeout
	if timeout == 0 {
		timeout = 10 * core.Second
	}
	poll := s.subscribePoll
	if poll == 0 {
		poll = 50 * core.Millisecond
	}
	deadline := core.Now().Add(timeout)
	attempts := 0
	for core.Now().Before(deadline) {
		attempts++
		windowSvc, ok := core.ServiceFor[*guiwindow.Service](s.Core(), "window")
		if ok && windowSvc != nil {
			okC := windowSvc.SubscribeEvent(eventConsoleName, s.handleConsoleEvent)
			okE := windowSvc.SubscribeEvent(eventErrorName, s.handleErrorEvent)
			okW := windowSvc.SubscribeEvent(
				eventWebMCPToolsChanged,
				s.handleWebMCPToolsChanged,
			)
			core.Print(core.Stderr(),
				"bridge: SubscribeEvent wired after %d attempts (console=%v error=%v webmcp=%v)\n",
				attempts, okC, okE, okW)
			if okW {
				// Registrations can race bridge startup. Ask the already-loaded
				// shim for its full current snapshot after subscribing so no
				// early tools-changed event is lost.
				s.refreshWebMCPTools()
			}
			return
		}
		core.Sleep(poll)
	}
	core.Print(core.Stderr(),
		"bridge: SubscribeEvent timed out after %d attempts — console/error capture degraded\n",
		attempts)
}
