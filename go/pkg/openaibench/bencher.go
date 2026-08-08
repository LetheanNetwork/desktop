// SPDX-Licence-Identifier: EUPL-1.2

// Package openaibench implements benchmark.Bencher against any
// endpoint that speaks the OpenAI Chat Completions wire format —
// /v1/chat/completions with messages[] + usage{} response.
//
// One adapter covers a wide ecosystem:
//   - llama-server (llama.cpp's HTTP front; the modern way to use llama.cpp)
//   - NVIDIA NIM (microservices catalogue)
//   - OpenRouter (aggregator routing to many providers)
//   - vLLM (high-performance Python inference server)
//   - TGI (Text Generation Inference, HuggingFace's server)
//   - LM Studio (desktop app with local server mode)
//   - Anthropic, Google, Together, Mistral via their OpenAI-compat endpoints
//   - Any custom endpoint adhering to the OpenAI Chat Completions shape
//
// Design lineage: Snider 2026-05-18 — "people like using what they
// know; maybe they have local inference sorted and just want to point
// to their openai api's". Each registered Bencher binds to one
// endpoint; multiple registrations cover multiple providers in
// parallel.
//
// Measurement caveats — the non-streaming Chat Completions response
// gives us total wall-clock + usage token counts but NOT the prompt-
// processing / generation split. We report pp_tok_sec and tg_tok_sec
// as approximations against total wall-clock; this overestimates
// pp_tok_sec (includes some generation overhead) and underestimates
// tg_tok_sec (includes prompt processing). A streaming variant that
// measures TTFT for clean separation is a follow-up. For relative
// benchmarking across endpoints + models the approximation is fine
// — the relationship between numbers is what matters, not the absolute
// per-phase accuracy.
//
// Usage example:
//
//	b := openaibench.NewBencher(openaibench.Options{
//	    Name:     "openai-compat:nim-prod",
//	    Endpoint: "https://api.example.nim/v1",
//	    Bearer:   "sk-...",
//	})
//	svc.RegisterBencher(b)
package openaibench

import (
	core "dappco.re/go"

	"dappco.re/lthn/desktop/pkg/benchmark"
)

// DefaultTimeout caps each Bench call so a wedged endpoint cannot
// pin a Bench goroutine indefinitely. Generous enough for large-ctx
// requests on slow endpoints; explicit so tests can shorten it.
const DefaultTimeout = core.Minute * 5

// Options configures a Bencher. Required: Endpoint + Name. All other
// fields optional.
type Options struct {
	// Name is the substrate-side identity registered with
	// Service.RegisterBencher. Required — each Bencher must be
	// uniquely addressable so Run.Bencher attribution is stable.
	// Convention: "openai-compat:<short-tag>" e.g. "openai-compat:nim".
	Name string

	// Endpoint is the base URL up to and including /v1. The adapter
	// appends "/chat/completions" + "/models" as needed. Example:
	// "https://api.openrouter.ai/api/v1".
	Endpoint string

	// Description is the Describe() string returned for ListBenchers.
	// Empty → derived from Endpoint.
	Description string

	// Bearer is the API token sent as Authorization: Bearer <Bearer>.
	// Empty → header omitted (some local endpoints like llama-server
	// and LM Studio don't require auth).
	Bearer string

	// Org is the optional OpenAI-Organization header. Empty → omitted.
	Org string

	// Timeout caps each Bench call. Zero → DefaultTimeout.
	Timeout core.Duration

	// HTTPClient overrides the default *core.HTTPClient. Tests inject
	// httptest-bound clients; production leaves zero so the package
	// builds its own with the configured Timeout.
	HTTPClient *core.HTTPClient
}

// Bencher implements benchmark.Bencher against an OpenAI-compatible
// endpoint.
type Bencher struct {
	opts   Options
	client *core.HTTPClient
}

// NewBencher constructs an unregistered Bencher. Returns nil when
// required fields are missing so the caller fails-fast at boot
// instead of registering a half-configured bencher.
//
// Usage example:
//
//	b := openaibench.NewBencher(openaibench.Options{
//	    Name: "openai-compat:llama-server",
//	    Endpoint: "http://localhost:8080/v1",
//	})
//	if b == nil { return core.E("config", "openaibench misconfigured", nil) }
//	svc.RegisterBencher(b)
func NewBencher(opts Options) *Bencher {
	if opts.Name == "" || opts.Endpoint == "" {
		return nil
	}
	if opts.Timeout == 0 {
		opts.Timeout = DefaultTimeout
	}
	client := opts.HTTPClient
	if client == nil {
		client = &core.HTTPClient{Timeout: opts.Timeout}
	}
	return &Bencher{opts: opts, client: client}
}

// Name implements benchmark.Bencher.
func (b *Bencher) Name() string { return b.opts.Name }

// Kind reports KindRemoteHTTP — every endpoint this adapter speaks
// to is remote HTTP from our perspective, even if it happens to be
// loopback (llama-server / LM Studio bound to 127.0.0.1).
func (b *Bencher) Kind() benchmark.Kind { return benchmark.KindRemoteHTTP }

// Describe implements the optional Describer interface so
// ListBenchers shows an informative subtitle in the picker.
func (b *Bencher) Describe() string {
	if b.opts.Description != "" {
		return b.opts.Description
	}
	return core.Concat("OpenAI-compatible endpoint at ", b.opts.Endpoint)
}

// CanBench reports whether the endpoint advertises the requested
// model via /v1/models. Soft-fails to true when /v1/models is
// unreachable or returns a non-OpenAI shape (some endpoints don't
// implement it) so the Bench call gets to surface the real error.
//
// Usage example:
//
//	if b.CanBench(benchmark.Bench{Model: "meta-llama/Llama-3-8b"}) { ... }
func (b *Bencher) CanBench(req benchmark.Bench) bool {
	if req.Model == "" {
		return false
	}
	url := core.Concat(b.opts.Endpoint, "/models")
	rr := core.NewHTTPRequestContext(core.Background(), "GET", url, nil)
	if !rr.OK {
		return true
	}
	httpReq := rr.Value.(*core.Request)
	if b.opts.Bearer != "" {
		httpReq.Header.Set("Authorization", core.Concat("Bearer ", b.opts.Bearer))
	}
	if b.opts.Org != "" {
		httpReq.Header.Set("OpenAI-Organization", b.opts.Org)
	}
	resp, err := b.client.Do(httpReq)
	if err != nil {
		return true
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		return true
	}
	br := core.ReadAll(resp.Body)
	if !br.OK {
		return true
	}
	bodyStr, _ := br.Value.(string)
	var ml modelsResponse
	if r := core.JSONUnmarshalString(bodyStr, &ml); !r.OK {
		return true
	}
	if len(ml.Data) == 0 {
		// Endpoint returned an unexpected shape — soft-fail so Bench
		// surfaces the real failure mode.
		return true
	}
	for _, m := range ml.Data {
		if m.ID == req.Model {
			return true
		}
	}
	return false
}

// Models returns the model IDs the endpoint advertises via
// /v1/models. Frontend uses this to populate the model picker once
// the operator has chosen this bencher. Unlike CanBench (which
// soft-fails to true so Bench surfaces the real error), Models returns
// Fail when the endpoint is unreachable + returns an empty slice
// (with OK=true) when the endpoint responds but advertises nothing
// — some endpoints (llama-server) intentionally serve a single
// implicit model without listing it.
//
// Usage example:
//
//	r := b.Models(c.Context())
//	if r.OK { ids := r.Value.([]string); _ = ids }
func (b *Bencher) Models(ctx core.Context) core.Result {
	url := core.Concat(b.opts.Endpoint, "/models")
	rr := core.NewHTTPRequestContext(ctx, "GET", url, nil)
	if !rr.OK {
		return rr
	}
	httpReq := rr.Value.(*core.Request)
	if b.opts.Bearer != "" {
		httpReq.Header.Set("Authorization", core.Concat("Bearer ", b.opts.Bearer))
	}
	if b.opts.Org != "" {
		httpReq.Header.Set("OpenAI-Organization", b.opts.Org)
	}
	resp, err := b.client.Do(httpReq)
	if err != nil {
		return core.Fail(core.Errorf("openaibench.Models HTTP: %w", err))
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		return core.Fail(core.E("openaibench.Models", core.Concat("status=", core.Sprintf("%d", resp.StatusCode)), nil))
	}
	br := core.ReadAll(resp.Body)
	if !br.OK {
		return br
	}
	bodyStr, _ := br.Value.(string)
	var ml modelsResponse
	if r := core.JSONUnmarshalString(bodyStr, &ml); !r.OK {
		return r
	}
	out := make([]string, 0, len(ml.Data))
	for _, m := range ml.Data {
		if m.ID != "" {
			out = append(out, m.ID)
		}
	}
	return core.Ok(out)
}

// Bench executes one inference round through /v1/chat/completions,
// measures wall-clock total, and returns a benchmark.Run with
// pp_tok_sec / tg_tok_sec / prompt_len / output_len populated from
// the usage object.
//
// See package doc for the measurement caveat — the non-streaming
// response cannot cleanly split prompt-processing time from generation
// time. Numbers are approximate against wall-clock; useful for
// relative benchmarking across endpoints + models.
//
// Usage example:
//
//	r := b.Bench(c.Context(), benchmark.Bench{Model: "gpt-4", Prompt: text, Ctx: 4096})
//	if r.OK { run := r.Value.(benchmark.Run); _ = run }
func (b *Bencher) Bench(ctx core.Context, req benchmark.Bench) core.Result {
	if req.Model == "" {
		return core.Fail(core.E("openaibench.Bench", "model is required", nil))
	}
	maxOut := req.MaxOutput
	if maxOut <= 0 {
		maxOut = 256
	}
	payload := chatRequest{
		Model: req.Model,
		Messages: []chatMessage{
			{Role: "user", Content: req.Prompt},
		},
		MaxTokens: maxOut,
		Stream:    false,
	}
	bodyBytes := core.JSONMarshal(payload)
	if !bodyBytes.OK {
		return bodyBytes
	}
	url := core.Concat(b.opts.Endpoint, "/chat/completions")
	rr := core.NewHTTPRequestContext(ctx, "POST", url, core.NewBuffer(bodyBytes.Value.([]byte)))
	if !rr.OK {
		return rr
	}
	httpReq := rr.Value.(*core.Request)
	httpReq.Header.Set("Content-Type", "application/json")
	if b.opts.Bearer != "" {
		httpReq.Header.Set("Authorization", core.Concat("Bearer ", b.opts.Bearer))
	}
	if b.opts.Org != "" {
		httpReq.Header.Set("OpenAI-Organization", b.opts.Org)
	}
	started := core.Now()
	resp, err := b.client.Do(httpReq)
	if err != nil {
		return core.Fail(core.Errorf("openaibench.Bench HTTP: %w", err))
	}
	defer func() { _ = resp.Body.Close() }()
	totalNs := core.Since(started).Nanoseconds()
	if resp.StatusCode != 200 {
		raw := core.ReadAll(resp.Body)
		bodyStr, _ := raw.Value.(string)
		return core.Fail(core.E("openaibench.Bench", core.Concat("status=", core.Sprintf("%d", resp.StatusCode), " body=", bodyStr), nil))
	}
	br := core.ReadAll(resp.Body)
	if !br.OK {
		return br
	}
	bodyStr, _ := br.Value.(string)
	var cr chatResponse
	if r := core.JSONUnmarshalString(bodyStr, &cr); !r.OK {
		return r
	}
	run := benchmark.Run{
		Bencher:   b.Name(), // substrate stamps over this anyway
		Model:     firstNonEmpty(cr.Model, req.Model),
		Ctx:       req.Ctx,
		PromptLen: cr.Usage.PromptTokens,
		OutputLen: cr.Usage.CompletionTokens,
		PpTokSec:  tokSec(cr.Usage.PromptTokens, totalNs),
		TgTokSec:  tokSec(cr.Usage.CompletionTokens, totalNs),
		Endpoint:  b.opts.Endpoint,
		Extra: map[string]any{
			"response_id":    cr.ID,
			"response_model": cr.Model,
			"total_tokens":   cr.Usage.TotalTokens,
			"wall_clock_ms":  totalNs / 1e6,
			"measurement":    "approx-from-wall-clock",
		},
	}
	if len(cr.Choices) > 0 && cr.Choices[0].FinishReason != "" {
		run.Extra["finish_reason"] = cr.Choices[0].FinishReason
	}
	return core.Ok(run)
}

// tokSec computes tokens/sec from a token count + duration in
// nanoseconds. Zero duration / zero tokens returns 0 (avoids NaN +
// honest about "no measurement").
func tokSec(tokens int, durNs int64) float64 {
	if tokens <= 0 || durNs <= 0 {
		return 0
	}
	return float64(tokens) / (float64(durNs) / 1e9)
}

// firstNonEmpty returns a when non-empty, else b. Used to prefer
// the response-reported model over the request-stated model.
func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// chatRequest is the POST body shape for /v1/chat/completions.
// stream=false because we want the single non-streamed response that
// carries the usage{} block.
type chatRequest struct {
	Model     string        `json:"model"`
	Messages  []chatMessage `json:"messages"`
	MaxTokens int           `json:"max_tokens"`
	Stream    bool          `json:"stream"`
}

// chatMessage is one entry in chatRequest.Messages. We only ever
// send a single user message for benchmarking.
type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatResponse is the parsed /v1/chat/completions response.
type chatResponse struct {
	ID      string       `json:"id"`
	Model   string       `json:"model"`
	Choices []chatChoice `json:"choices"`
	Usage   chatUsage    `json:"usage"`
}

// chatChoice carries the model-generated message + termination
// reason. We only need FinishReason for the Extra annotation today.
type chatChoice struct {
	FinishReason string `json:"finish_reason"`
}

// chatUsage is the standard OpenAI usage block.
type chatUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// modelsResponse is the parsed /v1/models response. We only need
// the ID per entry for CanBench gating.
type modelsResponse struct {
	Data []modelEntry `json:"data"`
}

// modelEntry is one model in modelsResponse.Data.
type modelEntry struct {
	ID string `json:"id"`
}
