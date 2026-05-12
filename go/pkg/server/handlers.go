// SPDX-Licence-Identifier: EUPL-1.2

// HTTP handlers for the lthn API server. OpenAI-compatible shape for
// /v1/models, /v1/chat/completions, /v1/completions. The handlers are
// thin: parse request JSON, dispatch to the runner (or stub fallback),
// marshal the response, write it. All Result-handling stays inside the
// handler — the runner contract returns Result, not (T, error).
//
// Usage example:
//
//	s := server.NewService(server.Options{Runner: r})
//	mux := &core.ServeMux{}
//	s.routes() // wires the OpenAI surface onto s.mux
package server

import (
	core "dappco.re/go"
)

// healthResponse is the /health endpoint payload.
type healthResponse struct {
	Status  string `json:"status"`
	Service string `json:"service"`
}

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

// routes wires the OpenAI-compatible surface onto s.mux. Called from
// NewService.
func (s *Service) routes() {
	s.mux.HandleFunc("/health", s.handleHealth)
	s.mux.HandleFunc("/v1/models", s.handleModels)
	s.mux.HandleFunc("/v1/chat/completions", s.handleChat)
	s.mux.HandleFunc("/v1/completions", s.handleCompletion)
}

// handleHealth returns the liveness probe response. Always 200 OK.
func (s *Service) handleHealth(w core.ResponseWriter, r *core.Request) {
	w.Header().Set("Content-Type", "application/json")
	writeJSON(w, healthResponse{Status: "ok", Service: "lthn"})
}

// handleModels returns the OpenAI /v1/models list. Routes through the
// runner when set; falls back to the stub list otherwise.
func (s *Service) handleModels(w core.ResponseWriter, r *core.Request) {
	w.Header().Set("Content-Type", "application/json")
	var ids []string
	if s.opts.Runner != nil {
		mr := s.opts.Runner.Models()
		if !mr.OK {
			writeError(w, 500, mr.Error(), "runner_error")
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
	writeJSON(w, resp)
}

// handleChat implements POST /v1/chat/completions. Non-streaming today;
// streaming SSE responses land in the next pass.
func (s *Service) handleChat(w core.ResponseWriter, r *core.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != "POST" {
		writeError(w, 405, "method not allowed", "invalid_request_error")
		return
	}
	body := core.ReadAll(r.Body)
	if !body.OK {
		writeError(w, 400, body.Error(), "invalid_request_error")
		return
	}
	var req chatRequest
	if pr := core.JSONUnmarshalString(body.Value.(string), &req); !pr.OK {
		writeError(w, 400, pr.Error(), "invalid_request_error")
		return
	}
	prompt := lastUserMessage(req.Messages)
	reply := s.generate(prompt)
	writeJSON(w, chatResponse{
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
func (s *Service) handleCompletion(w core.ResponseWriter, r *core.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != "POST" {
		writeError(w, 405, "method not allowed", "invalid_request_error")
		return
	}
	body := core.ReadAll(r.Body)
	if !body.OK {
		writeError(w, 400, body.Error(), "invalid_request_error")
		return
	}
	var req completionRequest
	if pr := core.JSONUnmarshalString(body.Value.(string), &req); !pr.OK {
		writeError(w, 400, pr.Error(), "invalid_request_error")
		return
	}
	reply := s.generate(req.Prompt)
	writeJSON(w, completionResponse{
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

// writeJSON marshals v and writes it as the response body. On marshal
// failure, writes a JSON error envelope at 500.
func writeJSON(w core.ResponseWriter, v any) {
	r := core.JSONMarshal(v)
	if !r.OK {
		writeError(w, 500, r.Error(), "encoding_error")
		return
	}
	if b, ok := r.Value.([]byte); ok {
		_, _ = w.Write(b)
	}
}

// writeError writes an OpenAI-style error envelope at the given status.
func writeError(w core.ResponseWriter, status int, msg, kind string) {
	w.WriteHeader(status)
	r := core.JSONMarshal(errorResponse{Error: errorBody{Message: msg, Type: kind}})
	if r.OK {
		if b, ok := r.Value.([]byte); ok {
			_, _ = w.Write(b)
		}
	}
}
