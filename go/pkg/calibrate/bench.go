// SPDX-Licence-Identifier: EUPL-1.2

package calibrate

import (
	core "dappco.re/go"
	"dappco.re/go/process"

	"dappco.re/lthn/desktop/pkg/benchmark"
)

// benchName is the substrate-stamped identity of the lthn-mlx bencher
// (becomes Run.Bencher on every recorded row).
const benchName = "lthn-mlx"

// Bencher is the benchmark.Bencher that shells `lthn-mlx bench --json`
// for a local model and maps the report into a benchmark.Run. It's the
// data source behind the admin Benchmark table — registered into the
// benchmark substrate from pkg/desktop so the table shows real local
// rows instead of fixtures.
//
// Power: bench --json carries no watts (the window's "peak_watts: 0 =
// not measurable" case), so PeakWatts stays 0. Energy/joules is a
// separate driver-profile concern, deferred while the power path is
// spotty.
//
// Usage example:
//
//	benchmarkSvc.RegisterBencher(calibrate.NewBencher(c))
type Bencher struct {
	core *core.Core
}

// NewBencher constructs the lthn-mlx bencher against a Core container.
func NewBencher(c *core.Core) *Bencher { return &Bencher{core: c} }

// Name returns the substrate identity stamped onto recorded runs.
func (b *Bencher) Name() string { return benchName }

// Kind reports KindSubprocess — the bencher spawns the lthn-mlx binary.
func (b *Bencher) Kind() benchmark.Kind { return benchmark.KindSubprocess }

// CanBench accepts any request that names a model (a local gguf /
// safetensors path); the binary validates the path itself.
func (b *Bencher) CanBench(req benchmark.Bench) bool {
	return core.Trim(req.Model) != ""
}

// Models returns an empty list — the lthn-mlx bencher benchmarks an
// explicit model path supplied by the caller (the model-browser is the
// inventory surface), so it advertises no fixed catalogue. Empty + OK
// per the Bencher contract.
func (b *Bencher) Models(_ core.Context) core.Result {
	return core.Ok([]string{})
}

// Bench runs `lthn-mlx bench --json <model>` (optionally pinning
// -context / -max-tokens) and maps the report into a benchmark.Run:
// prefill→PpTokSec, decode→TgTokSec, peak memory→PeakMemMB, token
// counts, ctx. ID / Timestamp / Bencher are left for the substrate to
// fill. Synchronous — a benchmark inherently blocks for the run.
//
// Usage example:
//
//	r := b.Bench(ctx, benchmark.Bench{Model: "/path/to/model", Ctx: 2048, MaxOutput: 256})
func (b *Bencher) Bench(c core.Context, req benchmark.Bench) core.Result {
	const op = "calibrate.Bencher.Bench"
	if b == nil || b.core == nil {
		return core.Fail(core.E(op, "core not bound", nil))
	}
	if core.Trim(req.Model) == "" {
		return core.Fail(core.E(op, "model path required", nil))
	}
	proc, ok := core.ServiceFor[*process.Service](b.core, "process")
	if !ok || proc == nil {
		return core.Fail(core.E(op, "process service unavailable", nil))
	}

	args := []string{"bench", "--json"}
	if req.Ctx > 0 {
		args = append(args, "-context", core.Sprintf("%d", req.Ctx))
	}
	if req.MaxOutput > 0 {
		args = append(args, "-max-tokens", core.Sprintf("%d", req.MaxOutput))
	}
	args = append(args, req.Model)

	r := proc.Run(c, resolveBinary(), args...)
	out, _ := r.Value.(string)
	if !r.OK {
		return core.Fail(core.E(op, r.Error(), nil))
	}
	return parseBenchReport(out, req)
}

// benchReport is the subset of lthn-mlx's `bench --json` report
// (dappco.re/go/inference/bench.Report) this adapter maps. Only the
// fields that flow into benchmark.Run are declared.
type benchReport struct {
	Model     string `json:"model"`
	ModelPath string `json:"model_path"`
	ModelInfo struct {
		ContextLength int `json:"context_length"`
	} `json:"model_info"`
	Generation struct {
		PromptTokens        int     `json:"prompt_tokens"`
		GeneratedTokens     int     `json:"generated_tokens"`
		PrefillTokensPerSec float64 `json:"prefill_tokens_per_sec"`
		DecodeTokensPerSec  float64 `json:"decode_tokens_per_sec"`
		PeakMemoryBytes     uint64  `json:"peak_memory_bytes"`
	} `json:"generation"`
}

// bytesPerMB converts the report's byte counts to the MB the Run + UI use.
const bytesPerMB = 1024 * 1024

// parseBenchReport maps a `bench --json` report onto a benchmark.Run.
// Split out for unit-testability (no binary spawn) and so a report
// contract drift breaks a test, not the live flow. PeakWatts is left 0
// — bench carries no power metric.
func parseBenchReport(stdout string, req benchmark.Bench) core.Result {
	const op = "calibrate.parseBenchReport"
	if core.Trim(stdout) == "" {
		return core.Fail(core.E(op, "empty bench report", nil))
	}
	var rep benchReport
	if pr := core.JSONUnmarshalString(stdout, &rep); !pr.OK {
		return core.Fail(core.E(op, pr.Error(), nil))
	}

	ctx := rep.ModelInfo.ContextLength
	if ctx == 0 {
		ctx = req.Ctx
	}
	model := rep.Model
	if model == "" {
		model = req.Model
	}
	return core.Ok(benchmark.Run{
		Model:     model,
		Ctx:       ctx,
		PpTokSec:  rep.Generation.PrefillTokensPerSec,
		TgTokSec:  rep.Generation.DecodeTokensPerSec,
		PromptLen: rep.Generation.PromptTokens,
		OutputLen: rep.Generation.GeneratedTokens,
		PeakMemMB: float64(rep.Generation.PeakMemoryBytes) / bytesPerMB,
	})
}
