// SPDX-Licence-Identifier: EUPL-1.2

// ClaudeConnectRecipe — the read-only "attach your Claude Code to this
// machine's CoreAgent hub" recipe rendered by the Agents view's Connect
// pane. Lethean NEVER edits the user's Claude Code config (~/.claude is not
// ours); this returns the bearer + the exact copy-paste steps for the
// operator to apply by hand.

package agents

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/keys"
)

// connectAuthTokenRef is the pkg/keys tier-0 ref the desktop startup
// (hubSandboxEnv) resolves into MCP_AUTH_TOKEN for the hub's fail-closed MCP
// plane. Read-only here — the crew is the writer; this MUST match the ref in
// pkg/desktop hubSandboxEnv.
const connectAuthTokenRef = "hub-mcp-auth-token"

// ConnectRecipe is the data the Connect pane renders: the bearer the hub
// expects plus the pieces of the attach-Claude-Code flow. Presentation (the
// settings.json snippet + export line) is composed frontend-side from Token —
// Lethean only ever READS its own tier-0 key and renders text, it never
// writes the user's ~/.claude config.
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

// ClaudeConnectRecipe returns the attach-Claude-Code recipe for this
// machine's hub. The bearer is read from pkg/keys tier-0 (the same key the
// crew handed the hub at spawn) — GetOrCreateTier0 is idempotent, so this
// agrees with whatever the hub got. A nil Core or unavailable keys service
// fails with a clear reason.
//
//	r := agentsSvc.ClaudeConnectRecipe()
//	recipe := r.Value.(ConnectRecipe)
func (s *Service) ClaudeConnectRecipe() core.Result {
	if s.core == nil {
		return core.Fail(core.E("agents.ClaudeConnectRecipe", "no core — desktop runtime not wired", nil))
	}
	ks, ok := core.ServiceFor[*keys.Service](s.core, "keys")
	if !ok || ks == nil {
		return core.Fail(core.E("agents.ClaudeConnectRecipe", "keys service unavailable — cannot read the hub bearer", nil))
	}
	r := ks.GetOrCreateTier0(connectAuthTokenRef, func() ([]byte, error) {
		rr := core.RandomBytes(32)
		if !rr.OK {
			return nil, rr.Value.(error)
		}
		return []byte(core.HexEncode(rr.Value.([]byte))), nil
	})
	if !r.OK {
		return r
	}
	raw, _ := r.Value.([]byte)
	token := core.Trim(string(raw))
	if token == "" {
		return core.Fail(core.E("agents.ClaudeConnectRecipe", "hub bearer is empty — has the hub started?", nil))
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
