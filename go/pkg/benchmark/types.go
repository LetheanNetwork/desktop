// SPDX-Licence-Identifier: EUPL-1.2

// Package benchmark is the runner-agnostic results substrate for
// inference benchmarks. It defines the Bencher interface (anything
// that can execute one benchmark run) and the canonical Run / Bench
// / Filter / Diff value types that flow through the registry and
// the DuckDB-backed history store.
//
// Why a substrate rather than an executor: many different runners
// produce comparable benchmarks — our own lthn-mlx runner, llama.cpp
// subprocesses, remote OpenAI-compatible endpoints (ollama, NVIDIA
// NIM, opencode), and future go-ai endpoints. The substrate organises
// + persists + queries the results without dictating how a run
// executes. Adapters live in their owner packages (AX-8: lib never
// imports consumer) and self-register via Service.RegisterBencher.
//
// Storage is DuckDB via dappco.re/go/orm so cross-bencher + cross-model
// queries are indexed rather than scan-the-jsonl.
//
// Usage example (overall shape):
//
//	svc := benchmark.NewService(benchmark.Options{}).MustRegister(c)
//	svc.RegisterBencher(myBencher)
//	r := svc.History(benchmark.Filter{Model: "gemma-4-e2b", Limit: 20})
//	if r.OK { runs := r.Value.([]benchmark.Run); _ = runs }
package benchmark

import (
	core "dappco.re/go"
)

// Kind classifies how a Bencher executes. Drives UI grouping (the
// admin Benchmark window labels local vs remote sources differently)
// and informs which optional Run fields a bencher can populate —
// e.g. PeakWatts is only meaningful for local benchers.
type Kind string

// Canonical Kind values.
const (
	// KindLocal — bencher runs inference in-process or in the
	// same machine's GPU. Can populate PeakWatts + PeakMemMB.
	KindLocal Kind = "local"

	// KindSubprocess — bencher spawns a local binary (e.g. llama.cpp
	// llama-bench) and parses output. Can populate PeakMemMB (RSS
	// of the subprocess) but PeakWatts only if the host wraps it.
	KindSubprocess Kind = "subprocess"

	// KindRemoteHTTP — bencher POSTs to a remote OpenAI-compatible
	// endpoint (ollama, NIM, opencode, custom). Populates Endpoint
	// and may add region / queue / cost metadata into Extra; cannot
	// populate PeakWatts / PeakMemMB (no local sampling possible).
	KindRemoteHTTP Kind = "remote-http"
)

// validKinds is the closed set of acceptable Kind values. Service
// validation rejects rows / registrations that carry anything else
// so the UI never has to render an unknown badge.
var validKinds = map[Kind]struct{}{
	KindLocal:      {},
	KindSubprocess: {},
	KindRemoteHTTP: {},
}

// IsValidKind reports whether k is one of the canonical Kind values.
//
// Usage example:
//
//	if !benchmark.IsValidKind(b.Kind()) { return reject }
func IsValidKind(k Kind) bool {
	_, ok := validKinds[k]
	return ok
}

// Bench is the runner-agnostic request a caller hands to a Bencher.
// Fields a Bencher MAY interpret loosely (Model semantics differ
// between gguf-path-based local benchers and model-ID-based remote
// ones); the substrate does not validate Model — that's the
// Bencher's territory via CanBench.
//
// Usage example:
//
//	req := benchmark.Bench{Model: "gemma-4-e2b", Prompt: stdPrompt, Ctx: 2048, MaxOutput: 256}
type Bench struct {
	// Model the bencher will benchmark — gguf basename, remote
	// model ID, or "auto" depending on the bencher. The substrate
	// treats this as an opaque string; bencher interprets it.
	Model string

	// Prompt is the input text the bencher will feed to the model.
	// Callers SHOULD pass a deterministic prompt sized to fill Ctx
	// for comparable benchmarks; the substrate does not validate.
	Prompt string

	// Ctx is the target context size in tokens. Bencher MAY clamp
	// to the model's actual max and report the effective Ctx on the
	// returned Run.
	Ctx int

	// MaxOutput caps generated tokens. Zero means use the bencher's
	// default (typically 256, matching common benchmark tools).
	MaxOutput int
}

// Run is the runner-agnostic result a Bencher returns and the
// substrate persists. Fields fall in three tiers:
//
//   - Identity: ID + Timestamp + Bencher + Model + Ctx — always set.
//   - Measurement: PpTokSec + TgTokSec + PromptLen + OutputLen — always
//     set; the whole point of a benchmark.
//   - Optional / source-dependent: PeakWatts + PeakMemMB (local only),
//     Endpoint (remote only), Extra (bencher-specific metadata).
//
// Zero values mean "not measurable" not "zero" — the UI distinguishes
// missing data from real zeroes by checking which Bencher.Kind() the
// Run came from.
//
// Usage example:
//
//	run := benchmark.Run{
//	    ID: "01ABC...", Timestamp: core.Now().UTC(),
//	    Bencher: "lthn-mlx", Model: "gemma-4-e2b", Ctx: 2048,
//	    PpTokSec: 4820, TgTokSec: 47.2,
//	    PromptLen: 1900, OutputLen: 256,
//	    PeakWatts: 8.4, PeakMemMB: 2400,
//	}
type Run struct {
	// Identity fields.
	ID        string    `json:"id"`
	Timestamp core.Time `json:"timestamp"`
	Bencher   string    `json:"bencher"`
	Model     string    `json:"model"`
	Ctx       int       `json:"ctx"`

	// Measurement fields.
	PpTokSec  float64 `json:"pp_tok_sec"`
	TgTokSec  float64 `json:"tg_tok_sec"`
	PromptLen int     `json:"prompt_len"`
	OutputLen int     `json:"output_len"`

	// Optional / source-dependent.
	PeakWatts float64        `json:"peak_watts,omitempty"`
	PeakMemMB float64        `json:"peak_mem_mb,omitempty"`
	Endpoint  string         `json:"endpoint,omitempty"`
	Extra     map[string]any `json:"extra,omitempty"`
}

// BencherInfo is the substrate-side metadata view of a registered
// Bencher. Returned by Service.ListBenchers so the frontend can
// render a bencher picker without holding a Bencher reference.
//
// Usage example:
//
//	r := svc.ListBenchers()
//	if r.OK { infos := r.Value.([]benchmark.BencherInfo); _ = infos }
type BencherInfo struct {
	Name        string `json:"name"`
	Kind        Kind   `json:"kind"`
	Description string `json:"description,omitempty"`
}

// Filter narrows a History query. Empty / zero fields mean "no
// constraint on this dimension". Ordering is always newest-first
// (timestamp DESC). Pagination via Limit + Offset.
//
// Usage example:
//
//	f := benchmark.Filter{Bencher: "lthn-mlx", Model: "gemma-4-e2b", Limit: 50}
type Filter struct {
	Bencher string    // exact match on Run.Bencher
	Model   string    // exact match on Run.Model
	MinCtx  int       // inclusive lower bound on Run.Ctx (0 = no constraint)
	MaxCtx  int       // inclusive upper bound on Run.Ctx (0 = no constraint)
	Since   core.Time // only runs at-or-after this timestamp (zero = no constraint)
	Limit   int       // page size; 0 = no limit (substrate may cap defensively)
	Offset  int       // page offset
}

// Diff is the result of comparing two Runs. Deltas are computed as
// (B - A); positive means B is larger. The full Run records are
// included so callers can render the comparison without a second
// fetch.
//
// Usage example:
//
//	r := svc.Compare("01A...", "01B...")
//	if r.OK { d := r.Value.(benchmark.Diff); _ = d.TgDelta }
type Diff struct {
	IDA            string  `json:"id_a"`
	IDB            string  `json:"id_b"`
	RunA           Run     `json:"run_a"`
	RunB           Run     `json:"run_b"`
	PpDelta        float64 `json:"pp_delta"`
	TgDelta        float64 `json:"tg_delta"`
	PeakWattsDelta float64 `json:"peak_watts_delta"`
	PeakMemMBDelta float64 `json:"peak_mem_mb_delta"`
}

// Bencher is anything that can execute one benchmark run. Implementations
// live in their owner package (AX-8) and self-register against the
// substrate via Service.RegisterBencher.
//
// The substrate guarantees:
//   - Name() is stable across the bencher's lifetime and used as the
//     Run.Bencher value when results are recorded.
//   - Kind() classifies the bencher for UI grouping + optional-field
//     interpretation.
//   - CanBench(req) is a fast pre-flight check; the substrate calls it
//     before Bench() to fail-fast on unsupported requests (e.g. a
//     remote bencher with no provisioning for the requested model).
//   - Bench(c, req) does the actual measurement and returns either a
//     core.Ok(Run{...}) or a core.Fail wrapping the underlying error.
//     Bencher SHOULD populate Run.ID (substrate will fill if empty),
//     Timestamp (substrate fills if zero), and Bencher (substrate
//     overwrites with Name()).
//
// Usage example:
//
//	type myBencher struct{ ... }
//	func (b *myBencher) Name() string             { return "ollama-localhost" }
//	func (b *myBencher) Kind() benchmark.Kind     { return benchmark.KindRemoteHTTP }
//	func (b *myBencher) CanBench(_ benchmark.Bench) bool { return true }
//	func (b *myBencher) Bench(c core.Context, req benchmark.Bench) core.Result {
//	    // ...POST to localhost:11434/api/generate, time it, build Run
//	}
type Bencher interface {
	Name() string
	Kind() Kind
	CanBench(req Bench) bool
	Bench(c core.Context, req Bench) core.Result
}
