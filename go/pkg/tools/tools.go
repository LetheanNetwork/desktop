// SPDX-Licence-Identifier: EUPL-1.2

// Package tools — Wails-bindable view over the MCP server's
// registered tool catalogue. Wraps dappco.re/go/mcp.Service.Tools()
// in a minimal struct shape Wails can marshal cleanly (the upstream
// ToolRecord carries InputSchema / OutputSchema as map[string]any +
// a RESTHandler closure that the binding generator can't serialise).
//
// Usage example:
//
//	core.WithName("tools", tools.Register)

package tools

import (
	"context"

	core "dappco.re/go"
	mcpsvc "dappco.re/go/mcp/pkg/mcp"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// ToolView is the lean shape the WebView consumes. Mirrors the
// fields the tools-window actually renders today; richer fields
// (schemas, REST handler URL) can layer on later via a Detail
// method when the UI needs them.
type ToolView struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Group       string `json:"group"`
}

// WailsService is the bindable service. Bound by
// application.NewService(tools.NewWailsService(core)) in
// pkg/desktop/desktop.go.
type WailsService struct {
	core *core.Core
}

func NewWailsService(c *core.Core) *WailsService { return &WailsService{core: c} }

func (s *WailsService) ServiceName() string { return "Tools" }
func (s *WailsService) ServiceStartup(_ context.Context, _ application.ServiceOptions) error {
	return nil
}
func (s *WailsService) ServiceShutdown() error { return nil }

// List returns one ToolView per tool the MCP service has registered.
// Returns an empty slice (not nil) when the MCP service isn't on
// the Core — UI renders the empty state.
func (s *WailsService) List() []ToolView {
	out := []ToolView{}
	if s == nil || s.core == nil {
		return out
	}
	mcp, ok := core.ServiceFor[*mcpsvc.Service](s.core, "mcp")
	if !ok || mcp == nil {
		return out
	}
	for _, t := range mcp.Tools() {
		out = append(out, ToolView{
			Name:        t.Name,
			Description: t.Description,
			Group:       t.Group,
		})
	}
	return out
}

// Register constructs and returns the tools service for Core
// registration. Mirrors the Service.go shape other lthn packages
// use; the *core.Core is captured so List() can look up mcp later.
func Register(c *core.Core) core.Result {
	return core.Ok(NewWailsService(c))
}
