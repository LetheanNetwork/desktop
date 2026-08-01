// SPDX-Licence-Identifier: EUPL-1.2

// Connect Claude Code to this machine's CoreAgent hub. ClaudeConnectRecipe is
// the read-only recipe the Agents view's Connect pane renders;
// LaunchClaudeConnected spawns Claude Code with the hub bearer already on its
// env (the launcher). Lethean only READS its own pkg/keys tier-0 key — it
// never edits the user's ~/.claude config; the bearer rides on the spawn env,
// never the command line.

package agents

import (
	core "dappco.re/go"
	"dappco.re/go/crypt/keys"
	"dappco.re/go/process"
	"dappco.re/lthn/desktop/pkg/terminal"
)

// connectAuthTokenRef is the pkg/keys tier-0 ref the desktop startup
// (hubSandboxEnv) resolves into MCP_AUTH_TOKEN for the hub's fail-closed MCP
// plane. Read-only here — the crew is the writer; MUST match pkg/desktop.
const connectAuthTokenRef = "hub-mcp-auth-token"

// ConnectRecipe is the data the Connect pane renders: the bearer the hub
// expects plus the pieces of the attach-Claude-Code flow. Presentation is
// composed frontend-side from Token — Lethean only READS its tier-0 key and
// renders text, it never writes the user's ~/.claude config.
type ConnectRecipe struct {
	// Token is the MCP_AUTH_TOKEN bearer the hub's MCP plane checks.
	Token string `json:"token"`
	// MCPURL is the hub MCP HTTP+SSE endpoint the plugin's .mcp.json targets.
	MCPURL string `json:"mcp_url"`
	// MarketplaceURL is the core/agent plugin marketplace (forge mirror).
	MarketplaceURL string `json:"marketplace_url"`
	// InstallCommands are the Claude Code slash-commands to add the
	// marketplace, install the plugin, and reload without a restart.
	InstallCommands []string `json:"install_commands"`
}

// hubBearerToken reads MCP_AUTH_TOKEN from pkg/keys tier-0 (the same key the
// crew handed the hub at spawn; GetOrCreateTier0 is idempotent so this agrees
// with whatever the hub got).
func (s *Service) hubBearerToken() (string, core.Result) {
	if s.core == nil {
		return "", core.Fail(core.E("agents.hubBearerToken", "no core — desktop runtime not wired", nil))
	}
	ks, ok := core.ServiceFor[*keys.Service](s.core, "keys")
	if !ok || ks == nil {
		return "", core.Fail(core.E("agents.hubBearerToken", "keys service unavailable — cannot read the hub bearer", nil))
	}
	r := ks.GetOrCreateTier0(connectAuthTokenRef, func() ([]byte, error) {
		rr := core.RandomBytes(32)
		if !rr.OK {
			return nil, rr.Value.(error)
		}
		return []byte(core.HexEncode(rr.Value.([]byte))), nil
	})
	if !r.OK {
		return "", r
	}
	raw, _ := r.Value.([]byte)
	token := core.Trim(string(raw))
	if token == "" {
		return "", core.Fail(core.E("agents.hubBearerToken", "hub bearer is empty — has the hub started?", nil))
	}
	return token, core.Ok(nil)
}

// ClaudeConnectRecipe returns the attach-Claude-Code recipe for this machine's
// hub. A nil Core or unavailable keys service fails with a clear reason.
//
//	r := agentsSvc.ClaudeConnectRecipe()
//	recipe := r.Value.(ConnectRecipe)
func (s *Service) ClaudeConnectRecipe() core.Result {
	token, r := s.hubBearerToken()
	if !r.OK {
		return r
	}
	return core.Ok(ConnectRecipe{
		Token:          token,
		MCPURL:         DefaultMCPURL + "/mcp",
		MarketplaceURL: "https://forge.lthn.ai/core/agent.git",
		InstallCommands: []string{
			"/plugin marketplace add https://forge.lthn.ai/core/agent.git",
			"/plugin install core@core-agent",
			"/reload-plugins",
		},
	})
}

// resolveClaudePath finds the claude CLI's absolute path — the standard
// ~/.claude/local install + the common bin dirs first (fast, no subshell),
// then the user's interactive-login shell PATH. Falls back to "claude" (the
// spawn env's PATH must then carry it).
func (s *Service) resolveClaudePath() string {
	home := core.Getenv("HOME")
	for _, p := range []string{
		core.JoinPath(home, ".claude", "local", "claude"),
		"/opt/homebrew/bin/claude",
		"/usr/local/bin/claude",
	} {
		if s.core != nil && s.core.Fs().IsFile(p).OK {
			return p
		}
	}
	if proc, ok := core.ServiceFor[*process.Service](s.core, "process"); ok && proc != nil {
		shell := core.Getenv("SHELL")
		if shell == "" {
			shell = "/bin/zsh"
		}
		if r := proc.Run(core.Background(), shell, "-ilc", "command -v claude"); r.OK {
			if out, ok := r.Value.(string); ok {
				if p := lastAbsPathLine(out); p != "" {
					return p
				}
			}
		}
	}
	return "claude"
}

// lastAbsPathLine returns the last line of out that starts with "/", trimming
// the login-shell init noise proc.Run merges in from stderr.
func lastAbsPathLine(out string) string {
	lines := core.Split(out, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if l := core.Trim(lines[i]); core.HasPrefix(l, "/") {
			return l
		}
	}
	return ""
}

// LaunchClaudeConnected spawns Claude Code with MCP_AUTH_TOKEN set so the
// plugin's `core` MCP connects to this machine's hub. target "tab" runs it in
// an in-app PTY terminal (watchable/drivable in the Terminal pane); "window"
// opens a macOS Terminal.app window. The bearer rides on Env, never argv.
//
//	r := agentsSvc.LaunchClaudeConnected("window")
func (s *Service) LaunchClaudeConnected(target string) core.Result {
	token, r := s.hubBearerToken()
	if !r.OK {
		return r
	}
	claudePath := s.resolveClaudePath()
	env := []string{"MCP_AUTH_TOKEN=" + token}

	switch core.Trim(target) {
	case "tab":
		sid, err := terminal.Spawn(terminal.SpawnInput{
			Command: []string{claudePath},
			Env:     env,
			Label:   "Claude Code",
			Cols:    220,
			Rows:    50,
		})
		if err != nil {
			return core.Fail(core.E("agents.LaunchClaudeConnected", "spawn in-app terminal failed", err))
		}
		return core.Ok(map[string]any{"target": "tab", "session_id": sid})

	case "window":
		proc, ok := core.ServiceFor[*process.Service](s.core, "process")
		if !ok || proc == nil {
			return core.Fail(core.E("agents.LaunchClaudeConnected", "process service unavailable", nil))
		}
		// A 0700 launcher script keeps the bearer off the command line; Terminal
		// runs it via `open -a Terminal`. (token is hex, path has no quotes — safe
		// inside single quotes without further escaping.)
		script := "#!/bin/sh\nexport MCP_AUTH_TOKEN='" + token + "'\nexec '" + claudePath + "'\n"
		tmp := core.Getenv("TMPDIR")
		if core.Trim(tmp) == "" {
			tmp = "/tmp"
		}
		path := core.JoinPath(tmp, "lthn-launch-claude.command")
		if w := s.core.Fs().WriteMode(path, script, 0o700); !w.OK {
			return w
		}
		if rr := proc.Run(core.Background(), "open", "-a", "Terminal", path); !rr.OK {
			return core.Fail(core.E("agents.LaunchClaudeConnected", "open Terminal failed: "+rr.Error(), nil))
		}
		return core.Ok(map[string]any{"target": "window"})

	default:
		return core.Fail(core.E("agents.LaunchClaudeConnected", "unknown target (want tab|window): "+target, nil))
	}
}
