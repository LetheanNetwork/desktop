// SPDX-Licence-Identifier: EUPL-1.2

package bridge

import (
	"context"

	core "dappco.re/go"
	coremcp "dappco.re/go/mcp/pkg/mcp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type webMCPHarness struct {
	service       *Service
	mcpService    *coremcp.Service
	clientSession *mcp.ClientSession
	serverSession *mcp.ServerSession
}

func newWebMCPHarness(t *core.T) webMCPHarness {
	t.Helper()
	mcpService, err := coremcp.New(coremcp.Options{WorkspaceRoot: t.TempDir()})
	core.AssertNoError(t, err)
	service := &Service{}
	r := RegisterWebMCPTools(mcpService, service)
	core.AssertTrue(t, r.OK, "WebMCP registration must accept live services")

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := mcpService.Server().Connect(
		core.Background(), serverTransport, nil,
	)
	core.AssertNoError(t, err)
	client := mcp.NewClient(&mcp.Implementation{
		Name: "bridge-webmcp-test", Version: "1",
	}, nil)
	clientSession, err := client.Connect(core.Background(), clientTransport, nil)
	core.AssertNoError(t, err)
	t.Cleanup(func() {
		_ = clientSession.Close()
		_ = serverSession.Close()
	})
	return webMCPHarness{
		service:       service,
		mcpService:    mcpService,
		clientSession: clientSession,
		serverSession: serverSession,
	}
}

func webMCPEvent(tools ...map[string]any) map[string]any {
	items := make([]any, 0, len(tools))
	for _, tool := range tools {
		items = append(items, tool)
	}
	return map[string]any{"tools": items}
}

func listedTool(result *mcp.ListToolsResult, name string) *mcp.Tool {
	if result == nil {
		return nil
	}
	for _, tool := range result.Tools {
		if tool.Name == name {
			return tool
		}
	}
	return nil
}

func TestBridge_RegisterWebMCPTools_Good_DynamicListAndCall(t *core.T) {
	h := newWebMCPHarness(t)
	called := false
	h.service.webMCPCall = func(
		_ context.Context,
		name string,
		args map[string]any,
	) core.Result {
		called = true
		core.AssertEqual(t, "chat_read_thread", name)
		core.AssertEqual(t, "full", args["detail"])
		return core.Ok(map[string]any{
			"content": []any{
				map[string]any{"type": "text", "text": "live thread"},
			},
		})
	}

	before, err := h.clientSession.ListTools(core.Background(), nil)
	core.AssertNoError(t, err)
	core.AssertNil(t, listedTool(before, "chat_read_thread"))

	h.service.handleWebMCPToolsChanged(webMCPEvent(map[string]any{
		"name":        "chat_read_thread",
		"description": "Reads the live Angular chat thread.",
		"inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"detail": map[string]any{"type": "string"},
			},
		},
	}))

	after, err := h.clientSession.ListTools(core.Background(), nil)
	core.AssertNoError(t, err)
	tool := listedTool(after, "chat_read_thread")
	core.AssertNotNil(t, tool, "tools/list must include the Angular-registered tool")
	core.AssertEqual(t, "Reads the live Angular chat thread.", tool.Description)

	call, err := h.clientSession.CallTool(core.Background(), &mcp.CallToolParams{
		Name:      "chat_read_thread",
		Arguments: map[string]any{"detail": "full"},
	})
	core.AssertNoError(t, err)
	core.AssertTrue(t, called, "tools/call must cross the bridge into the WebView")
	core.AssertEqual(t, 1, len(call.Content))
	text, ok := call.Content[0].(*mcp.TextContent)
	core.AssertTrue(t, ok)
	core.AssertEqual(t, "live thread", text.Text)
}

func TestBridge_RegisterWebMCPTools_Bad_NilAndStaticCollision(t *core.T) {
	core.AssertTrue(t, RegisterWebMCPTools(nil, nil).OK)

	h := newWebMCPHarness(t)
	static := h.mcpService.Tools()[0]
	before, err := h.clientSession.ListTools(core.Background(), nil)
	core.AssertNoError(t, err)
	original := listedTool(before, static.Name)
	core.AssertNotNil(t, original)

	h.service.handleWebMCPToolsChanged(webMCPEvent(map[string]any{
		"name":        static.Name,
		"description": "Renderer attempted to replace a native tool.",
		"inputSchema": map[string]any{"type": "object"},
	}))

	after, err := h.clientSession.ListTools(core.Background(), nil)
	core.AssertNoError(t, err)
	stillNative := listedTool(after, static.Name)
	core.AssertNotNil(t, stillNative)
	core.AssertEqual(t, original.Description, stillNative.Description,
		"WebMCP tools must not replace native Go MCP tools")
}

func TestBridge_RegisterWebMCPTools_Ugly_RemovesUnregisteredTools(t *core.T) {
	h := newWebMCPHarness(t)
	h.service.webMCPCall = func(
		context.Context,
		string,
		map[string]any,
	) core.Result {
		return core.Ok(map[string]any{"content": []any{}})
	}
	h.service.handleWebMCPToolsChanged(webMCPEvent(map[string]any{
		"name":        "desktop_read_state",
		"description": "Reads live desktop state.",
		"inputSchema": map[string]any{"type": "object"},
	}))
	listed, err := h.clientSession.ListTools(core.Background(), nil)
	core.AssertNoError(t, err)
	core.AssertNotNil(t, listedTool(listed, "desktop_read_state"))

	h.service.handleWebMCPToolsChanged(webMCPEvent())

	listed, err = h.clientSession.ListTools(core.Background(), nil)
	core.AssertNoError(t, err)
	core.AssertNil(t, listedTool(listed, "desktop_read_state"),
		"aborted Angular registrations must disappear from tools/list")
}
