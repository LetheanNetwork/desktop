// SPDX-Licence-Identifier: EUPL-1.2

package calibrate

import (
	"testing"

	"dappco.re/lthn/desktop/pkg/benchmark"
)

// Compile-time proof the adapter satisfies the substrate's Bencher.
var _ benchmark.Bencher = (*Bencher)(nil)

// benchReportFixture mirrors the subset of `lthn-mlx bench --json` this
// adapter maps. peak_memory_bytes = 2400 MB exactly (2400 * 1024 * 1024).
const benchReportFixture = `{
  "version": 1,
  "model": "lemer-lite",
  "model_path": "/Users/me/models/lemer-lite",
  "model_info": { "context_length": 8192, "architecture": "gemma4" },
  "generation": {
    "runs": 3,
    "prompt_tokens": 1900,
    "generated_tokens": 256,
    "prefill_tokens_per_sec": 4820.5,
    "decode_tokens_per_sec": 47.2,
    "peak_memory_bytes": 2516582400
  }
}`

// TestParseBenchReport_Good maps a real-shaped report onto a Run, with
// PeakWatts left 0 (bench carries no power).
func TestParseBenchReport_Good(t *testing.T) {
	r := parseBenchReport(benchReportFixture, benchmark.Bench{})
	if !r.OK {
		t.Fatalf("parseBenchReport: want OK, got %v", r.Error())
	}
	run, ok := r.Value.(benchmark.Run)
	if !ok {
		t.Fatalf("value is %T, want benchmark.Run", r.Value)
	}
	if run.Model != "lemer-lite" {
		t.Errorf("Model = %q, want lemer-lite", run.Model)
	}
	if run.Ctx != 8192 {
		t.Errorf("Ctx = %d, want 8192", run.Ctx)
	}
	if run.PpTokSec != 4820.5 {
		t.Errorf("PpTokSec = %v, want 4820.5", run.PpTokSec)
	}
	if run.TgTokSec != 47.2 {
		t.Errorf("TgTokSec = %v, want 47.2", run.TgTokSec)
	}
	if run.PromptLen != 1900 || run.OutputLen != 256 {
		t.Errorf("PromptLen/OutputLen = %d/%d, want 1900/256", run.PromptLen, run.OutputLen)
	}
	if run.PeakMemMB != 2400 {
		t.Errorf("PeakMemMB = %v, want 2400", run.PeakMemMB)
	}
	if run.PeakWatts != 0 {
		t.Errorf("PeakWatts = %v, want 0 (bench has no power metric)", run.PeakWatts)
	}
}

// TestParseBenchReport_CtxFallback uses the request Ctx when the report
// omits context_length.
func TestParseBenchReport_CtxFallback(t *testing.T) {
	noCtx := `{"model":"m","model_info":{},"generation":{"decode_tokens_per_sec":10}}`
	r := parseBenchReport(noCtx, benchmark.Bench{Ctx: 4096})
	if !r.OK {
		t.Fatalf("parseBenchReport: want OK, got %v", r.Error())
	}
	if run := r.Value.(benchmark.Run); run.Ctx != 4096 {
		t.Errorf("Ctx = %d, want 4096 (fell back to request)", run.Ctx)
	}
}

// TestParseBenchReport_Bad rejects an empty report.
func TestParseBenchReport_Bad(t *testing.T) {
	if r := parseBenchReport("", benchmark.Bench{}); r.OK {
		t.Errorf("parseBenchReport(empty): want fail, got OK")
	}
	if r := parseBenchReport("   ", benchmark.Bench{}); r.OK {
		t.Errorf("parseBenchReport(whitespace): want fail, got OK")
	}
}

// TestParseBenchReport_Ugly rejects malformed JSON without panicking.
func TestParseBenchReport_Ugly(t *testing.T) {
	for _, junk := range []string{`{"generation": {`, `not json`, `[1,2,3]`} {
		if r := parseBenchReport(junk, benchmark.Bench{}); r.OK {
			t.Errorf("parseBenchReport(%q): want fail, got OK", junk)
		}
	}
}

// TestBencher_CanBench gates on a model path being present.
func TestBencher_CanBench(t *testing.T) {
	b := NewBencher(nil)
	if !b.CanBench(benchmark.Bench{Model: "/path/to/model"}) {
		t.Errorf("CanBench(with model) = false, want true")
	}
	if b.CanBench(benchmark.Bench{Model: ""}) {
		t.Errorf("CanBench(no model) = true, want false")
	}
	if b.Name() != "lthn-mlx" || b.Kind() != benchmark.KindSubprocess {
		t.Errorf("Name/Kind = %q/%q, want lthn-mlx/subprocess", b.Name(), b.Kind())
	}
}
