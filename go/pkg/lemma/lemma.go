// SPDX-License-Identifier: EUPL-1.2

// Package lemma is lthn/desktop's client-side handle on the local
// Lemma model runtime (lthn-mlx serve). It is the user-chats-with-
// model lane, with auto-capture into pkg/chathistory so continuity-
// rights (project_chat_continuity_rights_normal_user_pattern) becomes
// real without per-call ceremony from Wails service / CLI consumer code.
//
// Every Send() call appends the user turn + assistant response into the
// caller's chathistory archive. Consumers don't have to remember to
// log; the integration is the surface.
//
// Boundary rule (per Snider 2026-05-25): the actual model is the
// binary, everything else is a package. lthn-mlx is the only process
// boundary; routing / context / chathistory all ride in-process Go.
//
// Wire:
//
//	lthn/desktop (Go, this package) ─┐
//	                                 │ HTTP POST /v1/chat/completions
//	                                 ▼
//	                       lthn-mlx serve  (binary boundary per
//	                                        feedback_binary_is_model_package_is_everything_else)
//	                                 │
//	                                 ▼
//	                   go-mlx → metal → loaded model
//
// Mirrors core/agent/go/pkg/lemma; per-binary copies for now, extract
// to shared module when drift proves shared need.
//
// Usage example:
//
//	hist, _ := chathistory.Open("snider", "/Users/snider/Lethean/data/users/snider/chats.duckdb")
//	defer hist.Close()
//
//	svc := lemma.New(lemma.Config{History: hist})
//	sess, _ := svc.StartSession("snider", lemma.SessionMeta{Title: "evening chat"})
//	reply, _ := sess.Send(ctx, "hey lemma")
//	core.Print(stdout, "%s", reply)
//	_ = sess.End()
package lemma

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"time"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/chathistory"
)

const (
	// DefaultBaseURL points at the lthn-ai host (the LEM Runtime host), which
	// gates capacity then proxies chat to the model driver it supervises.
	// lthn-ai owns :9100; the driver's own :11434 sits behind it.
	DefaultBaseURL = "http://127.0.0.1:9100/v1"

	// DefaultModelID is the wire model name. lthn-mlx serve lazily
	// loads whatever --model directory it was started with.
	DefaultModelID = "lemer-lite"

	// DefaultTimeout caps per-request wall-clock. Cold generations
	// on bigger models can run minutes; tighten via Config.
	DefaultTimeout = 5 * time.Minute
)

// Config configures the Service. Zero-value uses Defaults.
type Config struct {
	BaseURL string
	ModelID string
	Timeout time.Duration
	Client  *http.Client
	// History is the per-user chathistory archive. Required for
	// Send() — turns are captured automatically. Nil disables
	// session creation (the auto-capture contract is the surface).
	History *chathistory.History
}

// Service holds the resolved config and HTTP client. Goroutine-safe;
// connection pooling via the shared http.Client. One Service per
// process is usual; sessions are cheap.
type Service struct {
	cfg Config
}

// Session represents one ongoing conversation. Tracks the chathistory
// conversation_id so every Send() call appends turns in order. Caller
// owns lifecycle — End() marks the conversation closed in the archive.
type Session struct {
	svc            *Service
	userID         string
	conversationID string
	closed         bool
}

// SessionMeta captures the metadata persisted to chathistory when a
// session starts. Title is shown in UIs that list conversations;
// Tags / Metadata are caller-extensible curation hooks.
type SessionMeta struct {
	Title          string
	Tags           []string
	Metadata       []byte // JSON; caller-extensible
	ConsentVersion int    // 0 means use chathistory default
}

// New builds a Service. Required: Config.History for any StartSession
// call. Other fields default per the package constants.
//
//	svc := lemma.New(lemma.Config{History: hist})
func New(cfg Config) *Service {
	cfg = cfg.applyDefaults()
	return &Service{cfg: cfg}
}

// StartSession opens a fresh conversation in the user's history archive
// and returns a handle for Send() / End() calls.
//
//	sess, err := svc.StartSession("snider", lemma.SessionMeta{Title: "morning chat"})
func (s *Service) StartSession(userID string, meta SessionMeta) (*Session, error) {
	if s == nil {
		return nil, core.E("lemma.StartSession", "service nil", nil)
	}
	if core.Trim(userID) == "" {
		return nil, core.E("lemma.StartSession", "user id required", nil)
	}
	if s.cfg.History == nil {
		return nil, core.E("lemma.StartSession", "history nil — auto-capture requires chathistory", nil)
	}
	convID, err := s.cfg.History.StartConversation(chathistory.NewConversation{
		Title:          meta.Title,
		ModelID:        s.cfg.ModelID,
		Tags:           meta.Tags,
		Metadata:       meta.Metadata,
		ConsentVersion: meta.ConsentVersion,
	})
	if err != nil {
		return nil, core.E("lemma.StartSession", "open conversation", err)
	}
	return &Session{svc: s, userID: userID, conversationID: convID}, nil
}

// ConversationID returns the chathistory conversation_id this session
// is appending to. Useful for SetSignal calls + UI display.
func (sess *Session) ConversationID() string {
	if sess == nil {
		return ""
	}
	return sess.conversationID
}

// Send appends the user turn to history, calls the model, appends the
// assistant turn, and returns the assistant text. If the model call
// fails, the user turn is still recorded (so a retry shows the original
// prompt) but no assistant turn is recorded.
//
//	reply, err := sess.Send(ctx, "what's the weather metaphor for today")
func (sess *Session) Send(ctx context.Context, userContent string) (string, error) {
	if sess == nil || sess.closed {
		return "", core.E("lemma.Send", "session closed or nil", nil)
	}
	if sess.svc == nil || sess.svc.cfg.History == nil {
		return "", core.E("lemma.Send", "service has no history", nil)
	}
	if core.Trim(userContent) == "" {
		return "", core.E("lemma.Send", "user content required", nil)
	}

	// Persist user turn first — survives a failed model call so retry
	// preserves the prompt without operator gymnastics.
	if _, err := sess.svc.cfg.History.WriteTurn(sess.conversationID, chathistory.NewTurn{
		Role:    "user",
		Content: userContent,
	}); err != nil {
		return "", core.E("lemma.Send", "write user turn", err)
	}

	// Pull the full prior conversation back into the chat-completions
	// messages array — model needs context, history is the truth.
	priorTurns, err := sess.svc.cfg.History.LoadTurns(sess.conversationID)
	if err != nil {
		return "", core.E("lemma.Send", "load prior turns", err)
	}
	messages := make([]chatMessage, 0, len(priorTurns))
	for _, t := range priorTurns {
		if t.Role != "user" && t.Role != "assistant" && t.Role != "system" {
			continue
		}
		messages = append(messages, chatMessage{Role: t.Role, Content: t.Content})
	}

	assistant, tokensIn, tokensOut, err := sess.svc.callChatCompletions(ctx, messages)
	if err != nil {
		return "", core.E("lemma.Send", "model call", err)
	}

	if _, werr := sess.svc.cfg.History.WriteTurn(sess.conversationID, chathistory.NewTurn{
		Role:      "assistant",
		Content:   assistant,
		TokensIn:  tokensIn,
		TokensOut: tokensOut,
	}); werr != nil {
		return "", core.E("lemma.Send", "write assistant turn", werr)
	}
	return assistant, nil
}

// End marks the session's conversation as closed in the archive.
// Idempotent. Once called, further Send() calls fail.
func (sess *Session) End() error {
	if sess == nil || sess.closed {
		return nil
	}
	sess.closed = true
	if sess.svc == nil || sess.svc.cfg.History == nil {
		return nil
	}
	return sess.svc.cfg.History.EndConversation(sess.conversationID)
}

// BaseURL returns the configured endpoint.
func (s *Service) BaseURL() string { return s.cfg.BaseURL }

// ModelID returns the configured wire model name.
func (s *Service) ModelID() string { return s.cfg.ModelID }

// ---- internal: chat-completions wire ----

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
}

type chatResponseChoice struct {
	Index        int         `json:"index"`
	Message      chatMessage `json:"message"`
	FinishReason string      `json:"finish_reason,omitempty"`
}

type chatResponseUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type chatResponse struct {
	ID      string               `json:"id,omitempty"`
	Object  string               `json:"object,omitempty"`
	Model   string               `json:"model,omitempty"`
	Choices []chatResponseChoice `json:"choices"`
	Usage   *chatResponseUsage   `json:"usage,omitempty"`
}

// callChatCompletions sends the messages to lthn-mlx serve and returns
// the assistant text + token usage.
func (s *Service) callChatCompletions(ctx context.Context, messages []chatMessage) (string, int, int, error) {
	body := chatRequest{Model: s.cfg.ModelID, Messages: messages, Stream: false}
	encoded := core.JSONMarshal(body)
	if !encoded.OK {
		return "", 0, 0, encoded.Value.(error)
	}

	reqCtx, cancel := context.WithTimeout(ctx, s.cfg.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost,
		s.cfg.BaseURL+"/chat/completions",
		bytes.NewReader(encoded.Value.([]byte)),
	)
	if err != nil {
		return "", 0, 0, err
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("accept", "application/json")

	resp, err := s.cfg.Client.Do(req)
	if err != nil {
		return "", 0, 0, err
	}
	defer resp.Body.Close()

	// Cap the response body at 16 MiB so a misbehaving (or hostile)
	// lthn-mlx returning unbounded bytes can't OOM the chat process.
	// 16 MiB covers any plausible single chat completion (a 4096-token
	// reply at ~6 chars/token + metadata fits in <100 KiB); the cap
	// is intentionally generous so honest workloads never clip. The
	// admin client uses a tighter 1 MiB cap because admin endpoints
	// return small JSON status envelopes; chat replies need more room.
	rawBody, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return "", 0, 0, err
	}
	if resp.StatusCode/100 != 2 {
		return "", 0, 0, core.E("lemma.callChatCompletions",
			"lthn-mlx returned "+resp.Status+": "+string(rawBody), nil)
	}

	var decoded chatResponse
	if r := core.JSONUnmarshal(rawBody, &decoded); !r.OK {
		return "", 0, 0, r.Value.(error)
	}
	if len(decoded.Choices) == 0 {
		return "", 0, 0, core.E("lemma.callChatCompletions",
			"response had no choices", nil)
	}
	tokensIn, tokensOut := 0, 0
	if decoded.Usage != nil {
		tokensIn = decoded.Usage.PromptTokens
		tokensOut = decoded.Usage.CompletionTokens
	}
	return decoded.Choices[0].Message.Content, tokensIn, tokensOut, nil
}

func (c Config) applyDefaults() Config {
	if core.Trim(c.BaseURL) == "" {
		c.BaseURL = DefaultBaseURL
	}
	if core.Trim(c.ModelID) == "" {
		c.ModelID = DefaultModelID
	}
	if c.Timeout <= 0 {
		c.Timeout = DefaultTimeout
	}
	if c.Client == nil {
		c.Client = &http.Client{Timeout: c.Timeout + 30*time.Second}
	}
	return c
}
