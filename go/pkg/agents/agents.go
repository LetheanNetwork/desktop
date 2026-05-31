// SPDX-Licence-Identifier: EUPL-1.2

// Package agents is the desktop's client to CoreAgent (lthn-agent hub) — the
// harness-agnostic orchestration layer. The human / GUI lane drives the
// agentic pipeline by shelling out to the lthn-agent CLI (`<verb> --json`,
// see cli.go) — the same shape pkg/calibrate uses for lthn-mlx. On top of
// the CLI ops, a live channel listener (channels.go) connects to the hub's
// MCP HTTP+SSE plane (:9202) and relays every notifications/claude/channel
// event so the Activity panel updates the instant a run changes rather than
// only on poll. The MCP plane is fail-closed (bearer required) — the listener
// sends MCP_AUTH_TOKEN as Authorization: Bearer on every request.
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

// DefaultMCPURL is the hub's MCP HTTP+SSE plane that the crew brings
// lthn-agent hub up on (Mantis #1807 Unit D). The channel listener
// (channels.go) connects here; CLI ops shell out to the binary directly
// and don't use this URL.
const DefaultMCPURL = "http://127.0.0.1:9202"

// Config configures the Service. Zero-value uses DefaultMCPURL with no auth
// (unauthenticated — only valid for test fixtures; production wires MCPToken).
type Config struct {
	// MCPURL is the hub MCP HTTP+SSE loopback for the channel listener;
	// empty → DefaultMCPURL.
	MCPURL string
	// MCPToken is the bearer token the channel listener sends on every request
	// to the hub MCP plane (MCP_AUTH_TOKEN from pkg/keys tier-0). Empty string
	// disables bearer injection — tests and unit contexts only; production
	// always supplies the token so the hub's fail-closed auth passes.
	MCPToken string
}

// Service is the CoreAgent client. Agentic ops shell out to the lthn-agent CLI
// (cli.go) — stateless per call; on top, a channel listener relays the hub's
// push events to the UI (channels.go).
type Service struct {
	mcpURL   string
	mcpToken string
	// core is captured at Register (and StartChannels) so CLI ops can reach the
	// process service and the listener can emit desktop events; nil when built
	// via New (unit tests) → CLI ops + listener stay off.
	core      *core.Core
	listener  *channelListener
	startOnce sync.Once
}

// New constructs a Service. Zero-value Config talks to the hub MCP plane for
// the channel listener; CLI ops resolve the lthn-agent binary at call time.
//
//	svc := agents.New(agents.Config{MCPToken: token})
func New(cfg Config) *Service {
	base := core.Trim(cfg.MCPURL)
	if base == "" {
		base = DefaultMCPURL
	}
	return &Service{mcpURL: base, mcpToken: cfg.MCPToken}
}

// Register builds the Agents service for core registration. Construction can't
// fail, so this always Ok's. MCPToken is wired later via StartChannels once
// the desktop startup has resolved the hub token from pkg/keys tier-0.
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

// StartChannels brings up the live channel listener: a consume-only MCP
// session to the hub's MCP plane that relays CoreAgent push events
// (agent.blocked, agent.complete, agent.status, …) to the Agents view via
// gui.EmitEvent("lthn:agents:channel"). Idempotent — call it once the crew
// is supervising the hub. mcpToken is the MCP_AUTH_TOKEN bearer credential
// the listener sends on every request to the hub's fail-closed MCP plane;
// empty disables bearer injection (unit tests only). No-op with a nil Core.
//
//	agentsSvc.StartChannels(core, mcpToken)
func (s *Service) StartChannels(c *core.Core, mcpToken string) {
	if c == nil {
		return
	}
	s.startOnce.Do(func() {
		s.core = c
		if mcpToken != "" {
			s.mcpToken = mcpToken
		}
		relay := func(channel string, data any) {
			_ = gui.EmitEvent(c, channelEventName, map[string]any{
				"channel": channel,
				"data":    data,
			})
		}
		s.listener = newChannelListener(s.mcpURL, s.mcpToken, relay)
		core.Info("agents: starting channel listener", "mcp", s.mcpURL)
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
