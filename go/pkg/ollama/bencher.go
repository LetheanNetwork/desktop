// SPDX-Licence-Identifier: EUPL-1.2

// Package ollama implements the benchmark.Bencher interface against
// a local (or remote) ollama daemon. Ollama is opinionated enough
// that it deserves its own adapter rather than sharing the
// pkg/openaibench /v1/chat/completions shape — ollama's /api/generate
// returns prompt_eval_count + prompt_eval_duration + eval_count +
// eval_duration directly, so pp/tg measurement is single-call cheap.
//
// Design lineage: Snider 2026-05-18 — "people like using what they
// know; maybe they have local inference sorted and just want to point
// to their openai api's". This adapter is the friendly path for the
// "I already run ollama serve" cohort. Discoverable models come from
// /api/tags; CanBench gates against that snapshot.
//
// Usage example:
//
//	b := ollama.NewBencher(ollama.Options{Endpoint: "http://localhost:11434"})
//	if r := bench.Service.RegisterBencher(b); !r.OK { return r }
package ollama

import (
	core "dappco.re/go"

	"dappco.re/lthn/desktop/pkg/benchmark"
)

// DefaultEndpoint is the loopback ollama daemon address. Override
// via Options.Endpoint to point at a remote host.
const DefaultEndpoint = "http://localhost:11434"

// DefaultName is the substrate-side identity this bencher registers
// under. Multiple ollama daemons can register with custom Name fields
// (e.g. "ollama-laptop", "ollama-workstation") to distinguish their
// Run rows in History.
const DefaultName = "ollama"

// DefaultTimeout caps how long a single Bench call may run before the
// HTTP client gives up. Generous enough for 8k-ctx prompts on slow
// hardware; explicit so tests can lower it.
const DefaultTimeout = core.Minute * 5

// Options configures a Bencher. All fields optional — Defaults kick
// in when zero.
type Options struct {
	// Endpoint is the base URL of the ollama daemon. Empty →
	// DefaultEndpoint (loopback).
	Endpoint string

	// Name is the substrate-side identity registered with
	// Service.RegisterBencher. Empty → DefaultName.
	Name string

	// Description is the Describe() string returned for ListBenchers.
	// Empty → derived from Endpoint.
	Description string

	// Timeout caps each Bench call. Zero → DefaultTimeout.
	Timeout core.Duration

	// HTTPClient overrides the default *core.HTTPClient. Tests inject
	// a fixture client; production leaves zero so the package builds
	// its own with sensible TLS + redirect policy.
	HTTPClient *core.HTTPClient
}

// Bencher is the runner-agnostic adapter for ollama. Implements
// benchmark.Bencher; instances register via Service.RegisterBencher
// during desktop boot.
type Bencher struct {
	opts   Options
	client *core.HTTPClient
}

// NewBencher constructs an unregistered Bencher. Caller registers
// via benchmark.Service.RegisterBencher.
//
// Usage example:
//
//	b := ollama.NewBencher(ollama.Options{})
//	svc.RegisterBencher(b)
func NewBencher(opts Options) *Bencher {
	if opts.Endpoint == "" {
		opts.Endpoint = DefaultEndpoint
	}
	if opts.Name == "" {
		opts.Name = DefaultName
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

// Kind reports KindRemoteHTTP — ollama is loopback HTTP from our
// perspective. Even when localhost, peak watts / peak memory are not
// directly measurable through the HTTP boundary (the ollama daemon
// owns the process whose RSS we'd need), so KindRemoteHTTP is the
// honest classification.
func (b *Bencher) Kind() benchmark.Kind { return benchmark.KindRemoteHTTP }

// Describe implements the optional Describer interface so
// ListBenchers shows a useful subtitle in the picker.
func (b *Bencher) Describe() string {
	if b.opts.Description != "" {
		return b.opts.Description
	}
	return core.Concat("Ollama daemon at ", b.opts.Endpoint)
}

// CanBench reports whether the ollama daemon advertises the
// requested model. Soft-fail: when /api/tags is unreachable (daemon
// down, network partition), returns true so the eventual Bench call
// surfaces the real error rather than swallowing it in CanBench.
//
// Usage example:
//
//	if b.CanBench(benchmark.Bench{Model: "llama3"}) { ... }
func (b *Bencher) CanBench(req benchmark.Bench) bool {
	if req.Model == "" {
		return false
	}
	tagsURL := core.Concat(b.opts.Endpoint, "/api/tags")
	rr := core.NewHTTPRequestContext(core.Background(), "GET", tagsURL, nil)
	if !rr.OK {
		return true // soft-fail
	}
	req2 := rr.Value.(*core.Request)
	resp, err := b.client.Do(req2)
	if err != nil {
		return true // soft-fail
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		return true // soft-fail
	}
	br := core.ReadAll(resp.Body)
	if !br.OK {
		return true
	}
	bodyStr, _ := br.Value.(string)
	var tags tagsResponse
	if r := core.JSONUnmarshalString(bodyStr, &tags); !r.OK {
		return true
	}
	for _, m := range tags.Models {
		if m.Name == req.Model {
			return true
		}
	}
	return false
}

// Bench executes one inference round through ollama's /api/generate,
// parses the timing markers from the response, and returns a
// benchmark.Run with pp_tok_sec / tg_tok_sec / prompt_len / output_len
// populated. PeakWatts + PeakMemMB stay zero (not measurable across
// the HTTP boundary). Endpoint carries the daemon URL; Extra carries
// the ollama-reported model string (which may include digest suffix).
//
// Usage example:
//
//	r := b.Bench(c.Context(), benchmark.Bench{Model: "llama3", Prompt: text, Ctx: 2048})
//	if r.OK { run := r.Value.(benchmark.Run); _ = run }
func (b *Bencher) Bench(ctx core.Context, req benchmark.Bench) core.Result {
	if req.Model == "" {
		return core.Fail(core.E("ollama.Bench", "model is required", nil))
	}
	maxOut := req.MaxOutput
	if maxOut <= 0 {
		maxOut = 256
	}
	payload := generateRequest{
		Model:  req.Model,
		Prompt: req.Prompt,
		Stream: false,
		Options: generateOptions{
			NumPredict: maxOut,
			NumCtx:     req.Ctx,
		},
	}
	bodyBytes := core.JSONMarshal(payload)
	if !bodyBytes.OK {
		return bodyBytes
	}
	url := core.Concat(b.opts.Endpoint, "/api/generate")
	rr := core.NewHTTPRequestContext(ctx, "POST", url, core.NewBuffer(bodyBytes.Value.([]byte)))
	if !rr.OK {
		return rr
	}
	req2 := rr.Value.(*core.Request)
	req2.Header.Set("Content-Type", "application/json")
	resp, err := b.client.Do(req2)
	if err != nil {
		return core.Fail(core.Errorf("ollama.Bench HTTP: %w", err))
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		raw := core.ReadAll(resp.Body)
		bodyStr, _ := raw.Value.(string)
		return core.Fail(core.E("ollama.Bench", core.Concat("status=", core.Sprintf("%d", resp.StatusCode), " body=", bodyStr), nil))
	}
	br := core.ReadAll(resp.Body)
	if !br.OK {
		return br
	}
	bodyStr, _ := br.Value.(string)
	var gr generateResponse
	if r := core.JSONUnmarshalString(bodyStr, &gr); !r.OK {
		return r
	}
	run := benchmark.Run{
		Bencher:   b.Name(), // substrate stamps over this anyway; set for direct callers
		Model:     gr.Model,
		Ctx:       req.Ctx,
		PromptLen: gr.PromptEvalCount,
		OutputLen: gr.EvalCount,
		PpTokSec:  tokSec(gr.PromptEvalCount, gr.PromptEvalDurationNs),
		TgTokSec:  tokSec(gr.EvalCount, gr.EvalDurationNs),
		Endpoint:  b.opts.Endpoint,
		Extra: map[string]any{
			"ollama_model":           gr.Model,
			"ollama_total_duration":  gr.TotalDurationNs,
			"ollama_load_duration":   gr.LoadDurationNs,
		},
	}
	if run.Model == "" {
		run.Model = req.Model
	}
	return core.Ok(run)
}

// tokSec computes tokens/sec from a token count + duration in
// nanoseconds. Zero duration returns 0 (avoids divide-by-zero); zero
// tokens also returns 0 (a Bench that produced no output is honestly
// zero throughput, not infinity).
func tokSec(tokens int, durNs int64) float64 {
	if tokens <= 0 || durNs <= 0 {
		return 0
	}
	return float64(tokens) / (float64(durNs) / 1e9)
}

// generateRequest is the POST body shape for ollama /api/generate.
// stream=false so we get one JSON response instead of NDJSON chunks.
type generateRequest struct {
	Model   string          `json:"model"`
	Prompt  string          `json:"prompt"`
	Stream  bool            `json:"stream"`
	Options generateOptions `json:"options"`
}

// generateOptions mirrors the subset of ollama's GenerateOptions we
// care about for benchmarking. num_predict caps output length;
// num_ctx pins the context window.
type generateOptions struct {
	NumPredict int `json:"num_predict,omitempty"`
	NumCtx     int `json:"num_ctx,omitempty"`
}

// generateResponse is the parsed /api/generate response. Durations
// arrive as nanosecond integers per ollama's wire format.
type generateResponse struct {
	Model                string `json:"model"`
	Response             string `json:"response"`
	TotalDurationNs      int64  `json:"total_duration"`
	LoadDurationNs       int64  `json:"load_duration"`
	PromptEvalCount      int    `json:"prompt_eval_count"`
	PromptEvalDurationNs int64  `json:"prompt_eval_duration"`
	EvalCount            int    `json:"eval_count"`
	EvalDurationNs       int64  `json:"eval_duration"`
}

// tagsResponse is the parsed /api/tags response — used by CanBench
// to gate model availability.
type tagsResponse struct {
	Models []tagsModel `json:"models"`
}

// tagsModel is a single entry in /api/tags. We only need Name; the
// full tag entry carries size + digest + modified_at + details.
type tagsModel struct {
	Name string `json:"name"`
}
