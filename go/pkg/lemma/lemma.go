// SPDX-Licence-Identifier: EUPL-1.2

// Package lemma is lthn/desktop's client-side handle on the local Lemma
// model runtime. The model itself lives in the lthn-mlx binary (Metal +
// cgo, runs as a subprocess via `lthn-mlx serve --model <path> --addr
// :11434`). This package is the package-side wire that talks to that
// binary over the standard OpenAI HTTP API.
//
// Boundary rule (per Snider 2026-05-25): the actual model is the
// binary, everything else is a package. lthn-mlx is the only process
// boundary; routing / RAG / context assembly all ride in-process Go.
//
// Wire:
//
//	lthn/desktop (Go, this package)
//	     │  HTTP POST /v1/chat/completions
//	     ▼
//	lthn-mlx serve  (binary boundary)
//	     │
//	     ▼
//	go-mlx → metal → lemer-lite
//
// MVP scope — non-streaming chat completion. Streaming SSE arrives in
// a follow-up alongside subprocess lifecycle management of the lthn-mlx
// binary itself. The go-ai providers/openai package is deliberately
// NOT used yet — local external/ai is pinned against an unreleased
// inference contract and won't compile against the workspace-resolved
// dappco.re/go/inference. When the upstream cascade lands a compatible
// tag, this package collapses into a thin facade over providers/openai.
//
// Usage example:
//
//	r := lemma.New(lemma.Config{})
//	if !r.OK { return r }
//	svc := r.Value.(*lemma.Service)
//	defer svc.Close()
//
//	reply := svc.Chat(ctx, []inference.Message{
//	    {Role: "user", Content: "What is two plus two?"},
//	})
//	if reply.OK { core.Print(stdout, "%s", reply.Value) }
package lemma

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"time"

	core "dappco.re/go"
	"dappco.re/go/inference"
)

const (
	// DefaultBaseURL matches the lthn-mlx serve default port. Override
	// via Config.BaseURL when running the binary on a different addr.
	DefaultBaseURL = "http://127.0.0.1:11434/v1"

	// DefaultModelID is the wire model name. lthn-mlx serve lazily
	// loads whatever --model directory it was started with; the ID here
	// is the label that goes on the wire.
	DefaultModelID = "lemer-lite"

	// DefaultTimeout caps per-request wall-clock. Generous since cold
	// generations on bigger models can run minutes; tighten via Config.
	DefaultTimeout = 5 * time.Minute
)

// Config configures the connection to a local lthn-mlx serve instance.
// Zero-value Config uses DefaultBaseURL + DefaultModelID + DefaultTimeout.
type Config struct {
	BaseURL string
	ModelID string
	Timeout time.Duration
	Client  *http.Client
}

// Service holds the resolved config + an HTTP client. Goroutine-safe;
// the http.Client uses shared connection pooling.
type Service struct {
	cfg Config
}

// New builds a Service from cfg. Defaults are applied where fields are
// zero-valued. Does NOT probe the endpoint — failures surface on the
// first Chat call.
//
// Usage example:
//
//	r := lemma.New(lemma.Config{})
//	if r.OK { svc := r.Value.(*lemma.Service); _ = svc }
func New(cfg Config) core.Result {
	cfg = cfg.applyDefaults()
	return core.Ok(&Service{cfg: cfg})
}

// Register is the core.WithName-compatible factory. Uses zero-value
// Config (all defaults).
//
// Usage example:
//
//	core.New(core.WithName("lemma", lemma.Register))
func Register(_ *core.Core) core.Result {
	return New(Config{})
}

// Chat sends a non-streaming chat completion request to lthn-mlx serve
// and returns the assistant's reply text on success. Multi-turn is
// supported by passing the full message history.
//
// Usage example:
//
//	reply := svc.Chat(ctx, []inference.Message{
//	    {Role: "user", Content: "hello"},
//	})
//	if reply.OK { fmt.Println(reply.Value) }
func (s *Service) Chat(ctx context.Context, messages []inference.Message) core.Result {
	if len(messages) == 0 {
		return core.Fail(core.E("lemma.Chat", "messages slice must be non-empty", nil))
	}

	body := chatRequest{
		Model:    s.cfg.ModelID,
		Messages: messages,
		Stream:   false,
	}
	encoded := core.JSONMarshal(body)
	if !encoded.OK {
		return encoded
	}

	reqCtx, cancel := context.WithTimeout(ctx, s.cfg.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, s.cfg.BaseURL+"/chat/completions", bytes.NewReader(encoded.Value.([]byte)))
	if err != nil {
		return core.Fail(core.E("lemma.Chat", "build request", err))
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("accept", "application/json")

	resp, err := s.cfg.Client.Do(req)
	if err != nil {
		return core.Fail(core.E("lemma.Chat", "lthn-mlx unreachable at "+s.cfg.BaseURL, err))
	}
	defer resp.Body.Close()

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return core.Fail(core.E("lemma.Chat", "read response", err))
	}
	if resp.StatusCode/100 != 2 {
		return core.Fail(core.E("lemma.Chat", "lthn-mlx returned status "+resp.Status+": "+string(rawBody), nil))
	}

	var decoded chatResponse
	if r := core.JSONUnmarshal(rawBody, &decoded); !r.OK {
		return r
	}
	if len(decoded.Choices) == 0 {
		return core.Fail(core.E("lemma.Chat", "response had no choices", nil))
	}
	return core.Ok(decoded.Choices[0].Message.Content)
}

// BaseURL returns the configured endpoint.
func (s *Service) BaseURL() string { return s.cfg.BaseURL }

// ModelID returns the configured wire model name.
func (s *Service) ModelID() string { return s.cfg.ModelID }

// Close is a no-op today — retained for symmetry with services that
// may grow lifecycle (subprocess management) later.
func (s *Service) Close() core.Result { return core.Ok(nil) }

// ---- internal: OpenAI-shape DTOs ----

type chatRequest struct {
	Model    string              `json:"model"`
	Messages []inference.Message `json:"messages"`
	Stream   bool                `json:"stream"`
}

type chatResponseChoice struct {
	Index        int               `json:"index"`
	Message      inference.Message `json:"message"`
	FinishReason string            `json:"finish_reason,omitempty"`
}

type chatResponse struct {
	ID      string               `json:"id,omitempty"`
	Object  string               `json:"object,omitempty"`
	Model   string               `json:"model,omitempty"`
	Choices []chatResponseChoice `json:"choices"`
	Usage   *chatResponseUsage   `json:"usage,omitempty"`
}

type chatResponseUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
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
