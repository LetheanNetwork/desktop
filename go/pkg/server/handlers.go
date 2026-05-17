// SPDX-Licence-Identifier: EUPL-1.2

// HTTP handlers for the lthn API server. Gin-based per the Lethean
// design canon — same handler shape that core/api uses, so this
// surface composes into core/api.Engine when the swagger / openapi
// / authentik wrapping is needed later.
//
// OpenAI-compatible shape: /health, /v1/models, /v1/chat/completions,
// /v1/completions. The handlers are thin: bind JSON, dispatch to the
// runner (or stub fallback), respond.
//
// Usage example:
//
//	s := server.NewService(server.Options{Runner: r})
//	s.routes() // wires the OpenAI surface onto s.engine
package server

import (

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/marketplace"
	"github.com/gin-gonic/gin"
)

// modelEntry mirrors the OpenAI /v1/models entry shape.
type modelEntry struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

// modelsResponse is the OpenAI /v1/models envelope.
type modelsResponse struct {
	Object string       `json:"object"`
	Data   []modelEntry `json:"data"`
}

// chatMessage is a single OpenAI chat-completion message.
type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatRequest is the OpenAI /v1/chat/completions request body.
type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
}

// chatChoice is one assistant reply in the chat-completion response.
type chatChoice struct {
	Index        int         `json:"index"`
	Message      chatMessage `json:"message"`
	FinishReason string      `json:"finish_reason"`
}

// chatResponse is the OpenAI /v1/chat/completions response body.
type chatResponse struct {
	ID      string       `json:"id"`
	Object  string       `json:"object"`
	Created int64        `json:"created"`
	Model   string       `json:"model"`
	Choices []chatChoice `json:"choices"`
}

// completionRequest is the OpenAI /v1/completions request body.
type completionRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

// completionChoice is one completion in the response.
type completionChoice struct {
	Text         string `json:"text"`
	Index        int    `json:"index"`
	FinishReason string `json:"finish_reason"`
}

// completionResponse is the OpenAI /v1/completions response body.
type completionResponse struct {
	ID      string             `json:"id"`
	Object  string             `json:"object"`
	Created int64              `json:"created"`
	Model   string             `json:"model"`
	Choices []completionChoice `json:"choices"`
}

// errorResponse is the OpenAI error envelope.
type errorResponse struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Message string `json:"message"`
	Type    string `json:"type"`
}

// lthnRoutes implements coreapi.RouteGroup for the OpenAI-compatible
// endpoints lthn ships. Registered on the *coreapi.Engine at
// construction time so the routes pick up the canonical middleware
// chain (auth, SSRF, sunset, cache, tracing) instead of bypassing it.
type lthnRoutes struct {
	server *Service
}

func newLthnRoutes(s *Service) *lthnRoutes { return &lthnRoutes{server: s} }

// Name reports the human-readable group identifier.
func (g *lthnRoutes) Name() string { return "lthn" }

// BasePath returns the URL prefix every route in this group shares.
// Empty string mounts at the engine root; the routes carry their own
// /v1/* prefix so the OpenAI-compat client paths match exactly.
func (g *lthnRoutes) BasePath() string { return "" }

// RegisterRoutes wires the OpenAI-compatible surface onto rg.
// core/api's WithBearerAuth + WithCacheControl + WithSunset etc.
// already attached when this fires, so every handler below inherits
// the middleware chain. /health is provided by coreapi itself —
// no need to register it here.
func (g *lthnRoutes) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/v1/models", g.server.handleModels)
	rg.POST("/v1/chat/completions", g.server.handleChat)
	rg.POST("/v1/completions", g.server.handleCompletion)
	// Mantis #1523 — plugin-view shim audit-grant receiver. POST'd
	// by the frontend shim BEFORE postMessage'ing token bytes to the
	// iframe so the audit row commits first; failure blocks delivery.
	// Mounted on lthnRoutes (not the /v1/api/plugin/:code/*proxyPath
	// wildcard) so the literal path resolves cleanly.
	rg.POST(PluginViewCapabilityGrantPath, g.server.handlePluginViewCapabilityGrant)
}

// handleModels returns the OpenAI /v1/models list. Routes through the
// runner when set; falls back to the stub list otherwise.
func (s *Service) handleModels(c *gin.Context) {
	var ids []string
	if s.opts.Runner != nil {
		mr := s.opts.Runner.Models()
		if !mr.OK {
			writeGinError(c, core.StatusInternalServerError, mr.Error(), "runner_error")
			return
		}
		if v, ok := mr.Value.([]string); ok {
			ids = v
		}
	}
	if len(ids) == 0 {
		ids = []string{"lthn-stub"}
	}
	resp := modelsResponse{Object: "list"}
	now := core.UnixNow()
	for _, id := range ids {
		resp.Data = append(resp.Data, modelEntry{
			ID:      id,
			Object:  "model",
			Created: now,
			OwnedBy: "lthn",
		})
	}
	c.JSON(core.StatusOK, resp)
}

// handleChat implements POST /v1/chat/completions. Non-streaming
// today; SSE responses land in the next pass.
func (s *Service) handleChat(c *gin.Context) {
	var req chatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeGinError(c, core.StatusBadRequest, err.Error(), "invalid_request_error")
		return
	}
	prompt := lastUserMessage(req.Messages)
	reply := s.generate(prompt)
	c.JSON(core.StatusOK, chatResponse{
		ID:      core.Concat("chatcmpl-", randID()),
		Object:  "chat.completion",
		Created: core.UnixNow(),
		Model:   pickModel(req.Model),
		Choices: []chatChoice{{
			Index:        0,
			Message:      chatMessage{Role: "assistant", Content: reply},
			FinishReason: "stop",
		}},
	})
}

// handleCompletion implements POST /v1/completions.
func (s *Service) handleCompletion(c *gin.Context) {
	var req completionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeGinError(c, core.StatusBadRequest, err.Error(), "invalid_request_error")
		return
	}
	reply := s.generate(req.Prompt)
	c.JSON(core.StatusOK, completionResponse{
		ID:      core.Concat("cmpl-", randID()),
		Object:  "text_completion",
		Created: core.UnixNow(),
		Model:   pickModel(req.Model),
		Choices: []completionChoice{{
			Text:         reply,
			Index:        0,
			FinishReason: "stop",
		}},
	})
}

// generate routes a prompt through the runner when set; falls back to
// the echo stub otherwise so the binary still serves something useful
// before go-mlx wiring lands.
func (s *Service) generate(prompt string) string {
	if s.opts.Runner == nil {
		return core.Concat("[lthn stub] received: ", prompt)
	}
	r := s.opts.Runner.Generate(prompt)
	if !r.OK {
		return core.Concat("[lthn error] ", r.Error())
	}
	if str, ok := r.Value.(string); ok {
		return str
	}
	return ""
}

// lastUserMessage returns the most recent user-role content from a
// chat-completion request, or "" when none.
func lastUserMessage(msgs []chatMessage) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "user" {
			return msgs[i].Content
		}
	}
	return ""
}

// pickModel falls back to "lthn-stub" when the caller omits a model.
func pickModel(m string) string {
	if m == "" {
		return "lthn-stub"
	}
	return m
}

// randID returns an 8-char random suffix for OpenAI-style ID fields.
// Falls back to "stub" when the random source errors.
func randID() string {
	r := core.RandomString(8)
	if !r.OK {
		return "stub"
	}
	if s, ok := r.Value.(string); ok {
		return s
	}
	return "stub"
}

// writeGinError writes an OpenAI-style error envelope at the given
// HTTP status.
func writeGinError(c *gin.Context, status int, msg, kind string) {
	c.AbortWithStatusJSON(status, errorResponse{
		Error: errorBody{Message: msg, Type: kind},
	})
}

// cspPolicyBase is the static portion of the Content-Security-Policy
// value applied to every response the lthn server emits. It forms
// the second defence layer behind Lit's built-in auto-escape (the
// primary defence). The frame-src directive is appended per-response
// by cspMiddleware so the live plugin-port registry (Cerberus CRIT-1
// per plans/code/lthn/desktop/views/RFC.plugin-views.md §3.3) is
// the source of truth at the moment each response renders.
//
// Directive rationale:
//   - default-src 'self'       — deny everything not explicitly listed.
//   - script-src 'self'        — no inline scripts, no eval, no remote JS.
//   - style-src 'self' 'unsafe-inline' — inline <style> allowed for Lit
//     shadow-root styles; nonce-based alternative deferred (see service.go
//     cspMiddleware comment for the tradeoff).
//   - img-src 'self' data: blob: — Lit image bindings + base64 data URIs.
//   - connect-src ... — XHR/fetch targets: same-origin + bridge ports +
//     HF API (model browser) + wss for future WebSocket lanes.
//   - frame-ancestors 'none'   — prevents the WebView being embedded in
//     an attacker-controlled frame (belt-and-suspenders with X-Frame-Options).
//   - frame-src appended at request time — per-port loopback allowlist
//     regenerated from marketplace.ViewRegistry.IframePorts(). NEVER
//     wildcards (Cerberus CRIT-1 — frame-src 'none' http://127.0.0.1:<p1>
//     http://127.0.0.1:<p2>). Empty registry → frame-src 'none'.
const cspPolicyBase = "default-src 'self'; " +
	"script-src 'self'; " +
	"style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' data: blob:; " +
	"connect-src 'self' " +
	"http://127.0.0.1:9879 http://127.0.0.1:9876 " +
	"wss://127.0.0.1:9879 wss://127.0.0.1:9876 " +
	"https://huggingface.co https://hf.co; " +
	"frame-ancestors 'none'"

// cspHeader is the HTTP header name for the Content-Security-Policy.
const cspHeader = "Content-Security-Policy"

// PluginPortSource is the contract cspMiddleware uses to fetch the
// live plugin-port allowlist on every response. The default impl
// reads marketplace.ViewRegistry; tests inject a stub.
type PluginPortSource func() []int

// defaultPluginPortSource returns the live ports the marketplace
// ViewRegistry holds. Read on every response so install / uninstall
// mutations propagate without restart.
var defaultPluginPortSource PluginPortSource = func() []int {
	return marketplace.ViewRegistry.IframePorts()
}

// cspMiddleware returns a gin.HandlerFunc that injects the composite
// Content-Security-Policy on every response. The frame-src directive
// enumerates exactly the loopback ports the live ViewRegistry
// currently holds for iframe-kind plugins. Empty registry produces
// `frame-src 'none'` per RFC §3.3.
//
// Mounted before any route so SPA shell, API responses, and error
// pages all carry the policy.
//
// Usage example:
//
//	coreapi.WithMiddleware(cspMiddleware())
//
// Usage example (with a custom port source — tests):
//
//	mw := cspMiddlewareWithSource(func() []int { return []int{4096} })
func cspMiddleware() gin.HandlerFunc {
	return cspMiddlewareWithSource(defaultPluginPortSource)
}

// cspMiddlewareWithSource builds the CSP middleware against an
// explicit port source. Used by tests to assert frame-src
// enumeration without touching the package-global ViewRegistry.
func cspMiddlewareWithSource(source PluginPortSource) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header(cspHeader, BuildCSPPolicy(source))
		c.Next()
	}
}

// BuildCSPPolicy composes the full CSP value: the static
// cspPolicyBase plus a frame-src directive enumerating the live
// iframe loopback ports. Exported so tests can assert the exact
// rendered policy string without spinning a gin engine.
//
// Behaviour per RFC §3.3:
//   - empty port list → `frame-src 'none'`
//   - non-empty → `frame-src 'none' http://127.0.0.1:<p1> http://127.0.0.1:<p2>` …
//   - NEVER wildcards (no `frame-src *`, no `http://127.0.0.1:*`)
//   - port 0 entries are silently dropped (defensive — zero is the
//     lit-kind sentinel; should never reach this path)
func BuildCSPPolicy(source PluginPortSource) string {
	var ports []int
	if source != nil {
		ports = source()
	}
	b := core.NewBuilder()
	b.WriteString(cspPolicyBase)
	b.WriteString("; frame-src 'none'")
	seen := map[int]bool{}
	for _, p := range ports {
		if p <= 0 || seen[p] {
			continue
		}
		seen[p] = true
		b.WriteString(core.Sprintf(" http://127.0.0.1:%d", p))
	}
	return b.String()
}
