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
	"net/http"

	core "dappco.re/go"
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
}

// handleModels returns the OpenAI /v1/models list. Routes through the
// runner when set; falls back to the stub list otherwise.
func (s *Service) handleModels(c *gin.Context) {
	var ids []string
	if s.opts.Runner != nil {
		mr := s.opts.Runner.Models()
		if !mr.OK {
			writeGinError(c, http.StatusInternalServerError, mr.Error(), "runner_error")
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
	c.JSON(http.StatusOK, resp)
}

// handleChat implements POST /v1/chat/completions. Non-streaming
// today; SSE responses land in the next pass.
func (s *Service) handleChat(c *gin.Context) {
	var req chatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeGinError(c, http.StatusBadRequest, err.Error(), "invalid_request_error")
		return
	}
	prompt := lastUserMessage(req.Messages)
	reply := s.generate(prompt)
	c.JSON(http.StatusOK, chatResponse{
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
		writeGinError(c, http.StatusBadRequest, err.Error(), "invalid_request_error")
		return
	}
	reply := s.generate(req.Prompt)
	c.JSON(http.StatusOK, completionResponse{
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
