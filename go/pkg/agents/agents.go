// SPDX-Licence-Identifier: EUPL-1.2

// Package agents is the desktop's client to CoreAgent (lthn-agent) — the
// harness-agnostic orchestration layer. The human / GUI lane drives the
// agentic pipeline by shelling out to the lthn-agent CLI (`<verb> --json`,
// see cli.go) — the same shape pkg/calibrate uses for lthn-mlx. The plugin
// lane (/v1/tools + /mcp on a running `serve`) is for the Claude/Codex harness
// (Cladius), not the GUI. On top of the CLI ops, a live channel listener
// (channels.go) consumes the serve's claude/channel push stream so the
// Activity panel updates the instant a run changes rather than only on poll.
//
// Wails service "Agents" — the Agents view's backend.
//
//	core.New(core.WithName("agents", agents.Register))
package agents

import (
	"sync"

	core "dappco.re/go"
	gui "dappco.re/go/gui"
)

// DefaultServeURL is the loopback the crew brings lthn-agent serve up on. Used
// only by the channel listener (the push stream); CLI ops shell out to the
// binary and don't touch it.
const DefaultServeURL = "http://127.0.0.1:9101"

// Config configures the Service. Zero-value uses DefaultServeURL.
type Config struct {
	// ServeURL is the lthn-agent serve loopback base for the channel listener;
	// empty → DefaultServeURL.
	ServeURL string
}

// Service is the CoreAgent client. Agentic ops shell out to the lthn-agent CLI
// (cli.go) — stateless per call; on top, a channel listener relays the serve's
// push events to the UI (channels.go).
type Service struct {
	serveURL string
	// core is captured at Register (and StartChannels) so CLI ops can reach the
	// process service and the listener can emit desktop events; nil when built
	// via New (unit tests) → CLI ops + listener stay off.
	core      *core.Core
	listener  *channelListener
	startOnce sync.Once
}

// New constructs a Service. Zero-value Config talks to the loopback serve for
// the channel listener; CLI ops resolve the lthn-agent binary at call time.
//
//	svc := agents.New(agents.Config{})
func New(cfg Config) *Service {
	base := core.Trim(cfg.ServeURL)
	if base == "" {
		base = DefaultServeURL
	}
	return &Service{serveURL: base}
}

// Register builds the Agents service for core registration. Construction can't
// fail, so this always Ok's.
//
//	core.New(core.WithName("agents", agents.Register))
func Register(c *core.Core) core.Result {
	svc := New(Config{})
	svc.core = c
	return core.Ok(svc)
}

// ServiceName is the Wails binding name → frontend @desktop/agents/service.
func (s *Service) ServiceName() string { return "Agents" }

// ServiceStartup is a no-op — the live channel listener is started explicitly
// via StartChannels from the desktop startup path (the GUI launch doesn't run
// service ServiceStartup); CLI ops need no boot work.
func (s *Service) ServiceStartup(_ core.Context, _ any) core.Result { return core.Ok(nil) }

// StartChannels brings up the live channel listener: a consume-only MCP session
// to the crew's lthn-agent serve that relays CoreAgent push events
// (agent.blocked, agent.complete, agent.status, …) to the Agents view via
// gui.EmitEvent("lthn:agents:channel"). Idempotent — call it once the crew is
// supervising the serve. No-op with a nil Core.
//
//	agentsSvc.StartChannels(core)
func (s *Service) StartChannels(c *core.Core) {
	if c == nil {
		return
	}
	s.startOnce.Do(func() {
		s.core = c
		relay := func(channel string, data any) {
			_ = gui.EmitEvent(c, channelEventName, map[string]any{
				"channel": channel,
				"data":    data,
			})
		}
		s.listener = newChannelListener(s.serveURL+"/mcp", relay)
		core.Info("agents: starting channel listener", "mcp", s.serveURL+"/mcp")
		s.listener.start()
	})
}

// ServiceShutdown stops the channel listener.
func (s *Service) ServiceShutdown() core.Result {
	if s.listener != nil {
		s.listener.stop()
	}
	return core.Ok(nil)
}
