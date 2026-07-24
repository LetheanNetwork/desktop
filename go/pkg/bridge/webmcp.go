// SPDX-Licence-Identifier: EUPL-1.2

package bridge

import (
	"context"

	core "dappco.re/go"
	coremcp "dappco.re/go/mcp/pkg/mcp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	webMCPWindow            = "app"
	maxWebMCPTools          = 256
	maxWebMCPToolNameBytes  = 128
	maxWebMCPDescriptionLen = 8 * 1024
)

type webMCPCallFunc func(
	context.Context,
	string,
	map[string]any,
) core.Result

type webMCPState struct {
	mu      core.Mutex
	server  *mcp.Server
	static  map[string]bool
	dynamic map[string]bool
}

type webMCPTool struct {
	name        string
	description string
	inputSchema map[string]any
}

// RegisterWebMCPTools connects the Angular WebMCP registry to an existing
// dappco.re/go MCP server. It is safe when either service is absent. Native Go
// tool names are reserved; renderer registrations can neither replace nor
// remove them.
//
// Usage example:
//
//	bridge.RegisterWebMCPTools(mcpSvc, bridgeSvc)
func RegisterWebMCPTools(mcpSvc *coremcp.Service, bridgeSvc *Service) core.Result {
	if mcpSvc == nil || bridgeSvc == nil || mcpSvc.Server() == nil {
		return core.Ok(nil)
	}

	static := map[string]bool{}
	for _, tool := range mcpSvc.Tools() {
		if tool.Name != "" {
			static[tool.Name] = true
		}
	}
	bridgeSvc.webMCP = &webMCPState{
		server:  mcpSvc.Server(),
		static:  static,
		dynamic: map[string]bool{},
	}
	if bridgeSvc.webMCPCall == nil {
		bridgeSvc.webMCPCall = bridgeSvc.callWebMCPTool
	}
	return core.Ok(nil)
}

// handleWebMCPToolsChanged replaces the dynamic subset of the native MCP
// server's registry with the webview's latest full snapshot.
func (s *Service) handleWebMCPToolsChanged(data any) {
	if s == nil || s.webMCP == nil {
		return
	}

	tools := parseWebMCPTools(data)
	next := make(map[string]webMCPTool, len(tools))
	for _, tool := range tools {
		if s.webMCP.static[tool.name] {
			continue
		}
		next[tool.name] = tool
	}

	state := s.webMCP
	state.mu.Lock()
	defer state.mu.Unlock()

	remove := make([]string, 0, len(state.dynamic))
	for name := range state.dynamic {
		if _, remains := next[name]; !remains {
			remove = append(remove, name)
		}
	}
	if len(remove) > 0 {
		state.server.RemoveTools(remove...)
	}

	dynamic := make(map[string]bool, len(next))
	for name, tool := range next {
		toolName := name
		state.server.AddTool(&mcp.Tool{
			Name:        toolName,
			Description: tool.description,
			InputSchema: tool.inputSchema,
		}, func(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return s.callDynamicWebMCPTool(ctx, toolName, request)
		})
		dynamic[name] = true
	}
	state.dynamic = dynamic
}

func parseWebMCPTools(data any) []webMCPTool {
	payload, ok := data.(map[string]any)
	if !ok {
		return nil
	}
	rawTools, ok := payload["tools"].([]any)
	if !ok {
		return nil
	}
	if len(rawTools) > maxWebMCPTools {
		rawTools = rawTools[:maxWebMCPTools]
	}

	out := make([]webMCPTool, 0, len(rawTools))
	for _, raw := range rawTools {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		name, _ := entry["name"].(string)
		name = core.Trim(name)
		if !validWebMCPToolName(name) {
			continue
		}
		description, _ := entry["description"].(string)
		if len(description) > maxWebMCPDescriptionLen {
			description = description[:maxWebMCPDescriptionLen]
		}
		schema, _ := entry["inputSchema"].(map[string]any)
		if schema == nil {
			schema = map[string]any{"type": "object"}
		}
		if schemaType, _ := schema["type"].(string); schemaType != "object" {
			continue
		}
		out = append(out, webMCPTool{
			name:        name,
			description: description,
			inputSchema: schema,
		})
	}
	return out
}

func validWebMCPToolName(name string) bool {
	if name == "" || len(name) > maxWebMCPToolNameBytes {
		return false
	}
	for _, char := range name {
		valid := char >= 'a' && char <= 'z' ||
			char >= 'A' && char <= 'Z' ||
			char >= '0' && char <= '9' ||
			char == '_' || char == '-' || char == '.'
		if !valid {
			return false
		}
	}
	return true
}

func (s *Service) callDynamicWebMCPTool(
	ctx context.Context,
	name string,
	request *mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	args := map[string]any{}
	if request != nil && request.Params != nil && len(request.Params.Arguments) > 0 {
		if r := core.JSONUnmarshal(request.Params.Arguments, &args); !r.OK {
			return webMCPErrorResult("invalid tool arguments: " + r.Error()), nil
		}
	}
	if s.webMCPCall == nil {
		return webMCPErrorResult("WebMCP call transport is unavailable"), nil
	}
	call := s.webMCPCall(ctx, name, args)
	if !call.OK {
		return webMCPErrorResult(call.Error()), nil
	}
	return webMCPResult(call.Value), nil
}

func (s *Service) callWebMCPTool(
	ctx context.Context,
	name string,
	args map[string]any,
) core.Result {
	if s == nil || s.ServiceRuntime == nil || s.Core() == nil {
		return core.Fail(core.E("bridge.callWebMCPTool",
			"bridge has no Core window transport", nil))
	}
	encodedArgs := core.JSONMarshal(args)
	if !encodedArgs.OK {
		cause, _ := encodedArgs.Value.(error)
		return core.Fail(core.E("bridge.callWebMCPTool",
			"encode WebMCP arguments", cause))
	}
	script := core.Sprintf(
		`(async()=>{if(!window.__lthnWebMcp)throw new Error("WebMCP bridge unavailable");return await window.__lthnWebMcp.callTool(%s,%s);})()`,
		core.JSONMarshalString(name),
		string(encodedArgs.Value.([]byte)),
	)
	result := s.eval(ctx, webMCPWindow, script)
	ok, _ := result["ok"].(bool)
	if !ok {
		message, _ := result["error"].(string)
		return core.Fail(core.E("bridge.callWebMCPTool",
			core.FirstNonBlank(message, "webview call failed"), nil))
	}
	return core.Ok(result["value"])
}

func (s *Service) refreshWebMCPTools() {
	if s == nil || s.webMCP == nil {
		return
	}
	result := s.eval(
		core.Background(),
		webMCPWindow,
		`window.__lthnWebMcp?{tools:window.__lthnWebMcp.listTools()}:{tools:[]}`,
	)
	ok, _ := result["ok"].(bool)
	if !ok {
		return
	}
	s.handleWebMCPToolsChanged(result["value"])
}

func webMCPResult(value any) *mcp.CallToolResult {
	if envelope, ok := value.(map[string]any); ok {
		content := webMCPContent(envelope["content"])
		result := &mcp.CallToolResult{
			Content:           content,
			StructuredContent: envelope["structuredContent"],
		}
		result.IsError, _ = envelope["isError"].(bool)
		if len(result.Content) > 0 {
			return result
		}
	}

	text := webMCPText(value)
	result := &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: text}},
	}
	if structured, ok := value.(map[string]any); ok {
		result.StructuredContent = structured
	}
	return result
}

func webMCPContent(value any) []mcp.Content {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]mcp.Content, 0, len(items))
	for _, item := range items {
		content, ok := item.(map[string]any)
		if !ok {
			continue
		}
		kind, _ := content["type"].(string)
		text, _ := content["text"].(string)
		if kind == "text" {
			out = append(out, &mcp.TextContent{Text: text})
		}
	}
	return out
}

func webMCPText(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	encoded := core.JSONMarshal(value)
	if !encoded.OK {
		return core.Sprintf("%v", value)
	}
	return string(encoded.Value.([]byte))
}

func webMCPErrorResult(message string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: message}},
		IsError: true,
	}
}
