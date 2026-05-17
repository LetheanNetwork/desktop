// SPDX-Licence-Identifier: EUPL-1.2

// Package api wires lthn's HTTP surface onto core/api's Gin Engine
// as RouteGroup implementations. The same engine is mounted at /api/*
// inside the Wails WebView by pkg/desktop.mountSubsystems, and would
// also light up under `lthn serve` if the desktop binary is ever
// asked to expose the gateway in headless mode.
//
// Wails3 bindings cover in-process Service access from the WebView
// directly — no HTTP round trip. The HTTP routes registered here are
// for OUT-OF-PROCESS consumers:
//
//   - External clients (Claude Code, Codex, OpenCode) using the
//     OpenAI-compatible /v1/* surface served by pkg/server.
//   - Plugins running in their own process that talk to lthn over
//     localhost (the "plugin market" surface, e.g. Raycast / VS Code
//     extensions / Pi integrations).
//   - Other Lethean nodes in the fleet querying state across the wire.
//
// The OpenAPI spec generated from these groups is the source of truth
// for the @lthn/api TypeScript SDK published to npm.
//
// Usage example:
//
//	apiSvc, _ := core.ServiceFor[*coreapi.Service](c, "api")
//	apiSvc.Engine.Register(api.NewRunnerGroup(r))
package api

import (
	"github.com/gin-gonic/gin"

	core "dappco.re/go"
	coreapi "dappco.re/go/api"
	"dappco.re/go/inference"
	"dappco.re/lthn/desktop/pkg/runner"
)

// RunnerGroup exposes runner.Service over HTTP — the same Generate /
// Chat / Models surface used by the WebView via Wails bindings, but
// reachable from any OpenAPI client. Lives under /v1/runner/* on the
// gateway (so once mounted at /api/, the full path is /api/v1/runner/*).
type RunnerGroup struct {
	runner *runner.Service
}

// NewRunnerGroup builds a RunnerGroup bound to r. r MUST be non-nil —
// otherwise handlers would panic on first call. The caller (typically
// pkg/desktop.mountSubsystems) is responsible for ensuring r was
// constructed before group registration.
//
// Usage example:
//
//	g := api.NewRunnerGroup(myRunner)
//	engine.Register(g)
func NewRunnerGroup(r *runner.Service) *RunnerGroup {
	return &RunnerGroup{runner: r}
}

// Name satisfies coreapi.RouteGroup. Surfaces in the generated
// OpenAPI tag list and in admin diagnostics.
func (g *RunnerGroup) Name() string { return "runner" }

// BasePath satisfies coreapi.RouteGroup. Versioned under /v1 so
// breaking changes can be staged behind /v2 without disrupting
// in-flight clients.
func (g *RunnerGroup) BasePath() string { return "/v1" }

// RegisterRoutes mounts handlers onto rg. Called once at app boot
// by coreapi.Engine when the group is registered.
func (g *RunnerGroup) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/runner/models", g.handleModels)
	rg.POST("/runner/generate", g.handleGenerate)
	rg.POST("/runner/chat", g.handleChat)
}

// Describe satisfies coreapi.DescribableGroup. Returns metadata for
// every route the group registers so the OpenAPI spec builder can
// emit a complete description without scraping gin internals.
func (g *RunnerGroup) Describe() []coreapi.RouteDescription {
	return []coreapi.RouteDescription{
		{
			Method:     "GET",
			Path:       "/runner/models",
			Summary:    "List configured runner model routes",
			Tags:       []string{"runner"},
			StatusCode: 200,
			Response: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"models": map[string]any{
						"type":  "array",
						"items": map[string]any{"type": "string"},
					},
				},
			},
		},
		{
			Method:     "POST",
			Path:       "/runner/generate",
			Summary:    "Single-prompt generation",
			Tags:       []string{"runner"},
			StatusCode: 200,
			RequestBody: map[string]any{
				"type":     "object",
				"required": []string{"prompt"},
				"properties": map[string]any{
					"prompt": map[string]any{"type": "string"},
				},
			},
			Response: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"reply": map[string]any{"type": "string"},
				},
			},
		},
		{
			Method:     "POST",
			Path:       "/runner/chat",
			Summary:    "Multi-turn chat completion",
			Tags:       []string{"runner"},
			StatusCode: 200,
			RequestBody: map[string]any{
				"type":     "object",
				"required": []string{"messages"},
				"properties": map[string]any{
					"messages": map[string]any{
						"type": "array",
						"items": map[string]any{
							"type":     "object",
							"required": []string{"role", "content"},
							"properties": map[string]any{
								"role":    map[string]any{"type": "string"},
								"content": map[string]any{"type": "string"},
							},
						},
					},
				},
			},
			Response: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"reply": map[string]any{"type": "string"},
				},
			},
		},
	}
}

// handleModels serves GET /v1/runner/models.
func (g *RunnerGroup) handleModels(c *gin.Context) {
	r := g.runner.Models()
	if !r.OK {
		c.JSON(500, gin.H{"error": r.Error()})
		return
	}
	ids, _ := r.Value.([]string)
	if ids == nil {
		ids = []string{}
	}
	c.JSON(200, gin.H{"models": ids})
}

// generateRequest is the POST /v1/runner/generate body shape.
type generateRequest struct {
	Prompt string `json:"prompt" binding:"required"`
}

// handleGenerate serves POST /v1/runner/generate.
//
// Cerberus #60 F-2 / Mantis #1711 (H#220 cluster close): threads
// c.Request.Context() into runner.GenerateCtx so a client disconnect
// (browser tab close, fetch abort) cancels the upstream provider RTT
// instead of letting it run to completion and burn quota against the
// Requested audit row. Mirrors the discipline pkg/server/handlers.go
// landed for the OpenAI-compatible /v1/chat/completions + /v1/completions
// surface; g.runner is a concrete *runner.Service which already exposes
// GenerateCtx, so no type-assert dance is needed here.
func (g *RunnerGroup) handleGenerate(c *gin.Context) {
	var req generateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	r := g.runner.GenerateCtx(c.Request.Context(), req.Prompt)
	if !r.OK {
		c.JSON(500, gin.H{"error": r.Error()})
		return
	}
	reply, _ := r.Value.(string)
	c.JSON(200, gin.H{"reply": reply})
}

// chatRequest is the POST /v1/runner/chat body shape.
type chatRequest struct {
	Messages []inference.Message `json:"messages" binding:"required"`
}

// handleChat serves POST /v1/runner/chat.
//
// Cerberus #60 F-2 / Mantis #1711 (H#220 cluster close): same
// client-disconnect cancellation discipline as handleGenerate — see
// that handler's doc-comment. Calls runner.ChatCtx with the gin
// request context so an abandoned request terminates the upstream
// provider call instead of running to completion.
//
// Mantis #1661 / Cerberus #45 — per-message structural caps mirror
// the Wails surface (runner.MaxChatMessages / MaxPromptBytes /
// MaxChatTotalBytes). The HTTP path previously inherited only the
// outer engine-wide body cap (64 KiB) which admitted ~30k empty-
// content message structs in one allocation. Cap rejection fires at
// 400 so the gin envelope distinguishes operator-error from upstream
// failure. Same ordering as WChat: count first (cheap), per-message
// next, cumulative total last.
func (g *RunnerGroup) handleChat(c *gin.Context) {
	var req chatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if msg, ok := violatesChatCaps(req.Messages); !ok {
		c.JSON(400, gin.H{"error": msg})
		return
	}
	r := g.runner.ChatCtx(c.Request.Context(), req.Messages)
	if !r.OK {
		c.JSON(500, gin.H{"error": r.Error()})
		return
	}
	reply, _ := r.Value.(string)
	c.JSON(200, gin.H{"reply": reply})
}

// violatesChatCaps applies the Wails-side structural caps from
// pkg/runner (MaxChatMessages / MaxPromptBytes / MaxChatTotalBytes)
// to an inference.Message slice. Returns (diagnostic, true) on
// success and (diagnostic, false) on the first cap violation. Ordering
// matches the WChat surface for parity-test symmetry.
//
// Usage example (in handleChat):
//
//	if msg, ok := violatesChatCaps(req.Messages); !ok {
//	    c.JSON(400, gin.H{"error": msg})
//	    return
//	}
func violatesChatCaps(messages []inference.Message) (string, bool) {
	if len(messages) > runner.MaxChatMessages {
		return core.Sprintf("message count exceeds %d (got %d)",
			runner.MaxChatMessages, len(messages)), false
	}
	var total int
	for i, m := range messages {
		size := len(m.Role) + len(m.Content)
		if size > runner.MaxPromptBytes {
			return core.Sprintf("message[%d] size %d exceeds %d byte cap",
				i, size, runner.MaxPromptBytes), false
		}
		total += size
		if total > runner.MaxChatTotalBytes {
			return core.Sprintf("cumulative message size at index %d exceeds %d byte cap",
				i, runner.MaxChatTotalBytes), false
		}
	}
	return "", true
}
