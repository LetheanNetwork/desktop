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
	"encoding/json"

	core "dappco.re/go"
	mcpsvc "dappco.re/go/mcp/pkg/mcp"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// ToolView is the lean shape the WebView consumes. Mirrors the
// fields the tools-window renders. InputSchema is pre-marshalled
// to a JSON string here — the upstream ToolRecord carries it as a
// map[string]any that Wails' binding generator can't type-check
// cleanly, so the boundary converts to a string the WebView's
// schema viewer can render verbatim.
type ToolView struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Group       string `json:"group"`
	// InputSchema is the JSON-marshalled tool input schema. Empty
	// string when the upstream tool has no declared schema. Pretty-
	// printed (2-space indent) so the WebView can drop it straight
	// into a <pre> block.
	InputSchema string `json:"input_schema"`
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
			InputSchema: marshalSchema(t.InputSchema),
		})
	}
	return out
}

// marshalSchema converts the upstream map[string]any input schema
// into a pretty-printed JSON string the WebView can render in a
// <pre> block. Returns the empty string when the schema is nil or
// the marshal fails — both cases let the WebView fall back to a
// "no schema declared" placeholder.
func marshalSchema(schema map[string]any) string {
	if len(schema) == 0 {
		return ""
	}
	b, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return ""
	}
	return string(b)
}

// Register constructs and returns the tools service for Core
// registration. Mirrors the Service.go shape other lthn packages
// use; the *core.Core is captured so List() can look up mcp later.
func Register(c *core.Core) core.Result {
	return core.Ok(NewWailsService(c))
}
