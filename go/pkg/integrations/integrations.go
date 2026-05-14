// SPDX-Licence-Identifier: EUPL-1.2

// Package integrations enumerates external AI clients that can be
// pointed at lthn's local OpenAI-compatible endpoint. Each client
// has a known config-file location; the service expands ~/ to the
// real home and reports whether the file exists so the UI knows
// whether the integration is wired or just available-to-wire.
//
// Today's surface is read-only — no install/configure-for-me path
// yet. That lands when each client has a vetted config template
// the service can write into place on demand.
//
// Usage example:
//
//	core.WithName("integrations", integrations.Register)

package integrations

import (
	"context"

	core "dappco.re/go"
)

// ClientStatus is the per-integration view the WebView renders. Fields
// follow snake_case so the generated TS binding feels native.
type ClientStatus struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	// ConfigPath is the absolute path (after ~ expansion) where the
	// client expects its lthn-compatible config to live.
	ConfigPath string `json:"config_path"`
	// ConfigPathRaw is the un-expanded "~/.config/foo/…" form for
	// display in compact UI rows.
	ConfigPathRaw string `json:"config_path_raw"`
	// Exists is true when ConfigPath resolves to a real file on disk.
	Exists bool `json:"exists"`
	// State is a small enum the UI maps to colour:
	//   "connected"    — config file present + lthn URL referenced
	//                    (we only check presence today; URL match
	//                    is a future probe).
	//   "configured"   — config file present, lthn URL not asserted.
	//   "available"    — config file missing; user can opt in.
	//   "n/a"          — client doesn't need a config file (e.g. Pi).
	State string `json:"state"`
}

// WailsService is the bindable surface. Wails generates TS at
// frontend/bindings/dappco.re/lthn/desktop/pkg/integrations/.
type WailsService struct{}

// NewWailsService constructs the integrations service.
func NewWailsService() *WailsService { return &WailsService{} }

func (s *WailsService) ServiceName() string { return "Integrations" }
func (s *WailsService) ServiceStartup(_ context.Context, _ any) core.Result {
	return core.Ok(nil)
}
func (s *WailsService) ServiceShutdown() core.Result { return core.Ok(nil) }

// catalogue lists every known client. Adding a new integration is a
// one-entry addition here — the rest of the wiring is mechanical.
type clientDef struct {
	ID            string
	Name          string
	Description   string
	ConfigPathRaw string
	NeedsConfig   bool
}

var catalogue = []clientDef{
	{ID: "claude-code", Name: "Claude Code", Description: "Anthropic CLI · OpenAI-compatible endpoint mode",
		ConfigPathRaw: "~/.config/claude/config.json", NeedsConfig: true},
	{ID: "opencode", Name: "OpenCode", Description: "Open-source coding agent · sandbox-managed via Lethean Desktop",
		ConfigPathRaw: "~/.config/opencode/opencode.json", NeedsConfig: true},
	{ID: "codex", Name: "Codex CLI", Description: "OpenAI CLI",
		ConfigPathRaw: "~/.codex/config.yaml", NeedsConfig: true},
	{ID: "copilot", Name: "GitHub Copilot", Description: "VS Code extension proxy mode",
		ConfigPathRaw: "~/Library/Application Support/Code/copilot/config.json", NeedsConfig: true},
	{ID: "pi", Name: "Pi (raycast)", Description: "Raycast extension talks to lthn directly",
		ConfigPathRaw: "(no config needed)", NeedsConfig: false},
}

// List returns the full catalogue with each entry's filesystem state
// resolved against $HOME.
func (s *WailsService) List() []ClientStatus {
	homeR := core.UserHomeDir()
	home, _ := homeR.Value.(string)
	out := make([]ClientStatus, 0, len(catalogue))
	for _, c := range catalogue {
		entry := ClientStatus{
			ID:            c.ID,
			Name:          c.Name,
			Description:   c.Description,
			ConfigPathRaw: c.ConfigPathRaw,
		}
		if !c.NeedsConfig {
			entry.State = "n/a"
			out = append(out, entry)
			continue
		}
		// Expand a leading "~" only when $HOME resolves. If err is
		// non-nil we leave ConfigPath unset; the UI displays the
		// raw form and shows the integration as "available".
		if homeR.OK && len(c.ConfigPathRaw) > 0 && c.ConfigPathRaw[0] == '~' {
			entry.ConfigPath = core.PathJoin(home, c.ConfigPathRaw[1:])
		} else {
			entry.ConfigPath = c.ConfigPathRaw
		}
		if stat := core.Stat(entry.ConfigPath); stat.OK {
			entry.Exists = true
			entry.State = "configured"
		} else {
			entry.Exists = false
			entry.State = "available"
		}
		out = append(out, entry)
	}
	return out
}

// Register constructs and returns the integrations service for
// Core registration. Mirror of the canonical Service.go shape
// other lthn packages use.
func Register(c *core.Core) core.Result {
	_ = c
	return core.Ok(NewWailsService())
}
