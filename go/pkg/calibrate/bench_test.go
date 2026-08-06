// SPDX-Licence-Identifier: EUPL-1.2

package calibrate

import (
	"os"
	"path/filepath"
	"testing"

	core "dappco.re/go"

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

// TestParseBenchReport_ModelFallback covers the branch none of the
// three cases above reach: the report omits "model" entirely, so the
// mapper falls back to the request's Model field.
func TestParseBenchReport_ModelFallback(t *testing.T) {
	noModel := `{"model_info":{},"generation":{"decode_tokens_per_sec":5}}`
	r := parseBenchReport(noModel, benchmark.Bench{Model: "/path/to/fallback-model"})
	if !r.OK {
		t.Fatalf("parseBenchReport: want OK, got %v", r.Error())
	}
	if run := r.Value.(benchmark.Run); run.Model != "/path/to/fallback-model" {
		t.Errorf("Model = %q, want the request's fallback model", run.Model)
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

// TestBencher_Models_Good the lthn-mlx bencher advertises no fixed
// catalogue — always an empty-but-OK list (the caller supplies an
// explicit model path).
func TestBencher_Models_Good(t *testing.T) {
	b := NewBencher(nil)
	r := b.Models(core.Background())
	if !r.OK {
		t.Fatalf("Models: want OK, got fail: %v", r.Error())
	}
	models, ok := r.Value.([]string)
	if !ok {
		t.Fatalf("Models value is %T, want []string", r.Value)
	}
	if len(models) != 0 {
		t.Errorf("len(Models) = %d, want 0", len(models))
	}
}

// --- Real tests for Bencher.Bench against a fake lthn-mlx binary. See
// calibrate_test.go for hermeticHome / newCalibrateServiceWithProcess /
// writeFakeLthnMlx — the same hermetic-PATH sandbox is required here so
// resolveBinary() never picks up a real lthn-mlx installed on the host.

func TestBencher_Bench_Good(t *testing.T) {
	dir := t.TempDir()
	script := "#!/bin/sh\n" +
		"if [ \"$1\" = \"bench\" ]; then\n" +
		"cat <<'EOF'\n" +
		`{"model":"lemer-lite","model_info":{"context_length":8192},"generation":{"prompt_tokens":100,"generated_tokens":50,"prefill_tokens_per_sec":100.5,"decode_tokens_per_sec":10.5,"peak_memory_bytes":1048576}}` + "\n" +
		"EOF\n" +
		"exit 0\n" +
		"fi\n" +
		"exit 1\n"
	writeFakeLthnMlx(t, dir, script)
	svc := newCalibrateServiceWithProcess(t, dir)
	b := NewBencher(svc.core)

	r := b.Bench(core.Background(), benchmark.Bench{Model: "/models/lemer-lite"})
	if !r.OK {
		t.Fatalf("Bench: want OK, got fail: %v", r.Error())
	}
	run := r.Value.(benchmark.Run)
	if run.Model != "lemer-lite" || run.Ctx != 8192 {
		t.Errorf("Model/Ctx = %q/%d, want lemer-lite/8192", run.Model, run.Ctx)
	}
}

func TestBencher_Bench_Good_PassesContextAndMaxTokensFlags(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "argv.txt")
	script := "#!/bin/sh\n" +
		"echo \"$*\" > " + marker + "\n" +
		`echo '{"model":"m","generation":{}}'` + "\n" +
		"exit 0\n"
	writeFakeLthnMlx(t, dir, script)
	svc := newCalibrateServiceWithProcess(t, dir)
	b := NewBencher(svc.core)

	r := b.Bench(core.Background(), benchmark.Bench{Model: "/models/m", Ctx: 2048, MaxOutput: 111})
	if !r.OK {
		t.Fatalf("Bench: want OK, got fail: %v", r.Error())
	}
	argv, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("read argv marker: %v", err)
	}
	got := string(argv)
	if !core.Contains(got, "-context 2048") {
		t.Errorf("argv = %q, want it to carry -context 2048", got)
	}
	if !core.Contains(got, "-max-tokens 111") {
		t.Errorf("argv = %q, want it to carry -max-tokens 111", got)
	}
}

func TestBencher_Bench_Bad_NilCore(t *testing.T) {
	b := NewBencher(nil)
	r := b.Bench(core.Background(), benchmark.Bench{Model: "/models/m"})
	if r.OK {
		t.Fatal("Bench with nil core: want fail, got OK")
	}
	if !core.Contains(r.Error(), "core not bound") {
		t.Errorf("error = %q, want it to mention core not bound", r.Error())
	}
}

func TestBencher_Bench_Bad_EmptyModel(t *testing.T) {
	hermeticHome(t)
	svc := NewService(core.New())
	b := NewBencher(svc.core)
	r := b.Bench(core.Background(), benchmark.Bench{})
	if r.OK {
		t.Fatal("Bench with empty model: want fail, got OK")
	}
	if !core.Contains(r.Error(), "model path required") {
		t.Errorf("error = %q, want it to mention model path required", r.Error())
	}
}

func TestBencher_Bench_Bad_NoProcessService(t *testing.T) {
	hermeticHome(t)
	c := core.New() // no "process" service wired
	b := NewBencher(c)
	r := b.Bench(core.Background(), benchmark.Bench{Model: "/models/m"})
	if r.OK {
		t.Fatal("Bench with no process service: want fail, got OK")
	}
	if !core.Contains(r.Error(), "process service unavailable") {
		t.Errorf("error = %q, want it to mention process service unavailable", r.Error())
	}
}

func TestBencher_Bench_Ugly_CommandFails(t *testing.T) {
	dir := t.TempDir()
	writeFakeLthnMlx(t, dir, "#!/bin/sh\necho boom 1>&2\nexit 3\n")
	svc := newCalibrateServiceWithProcess(t, dir)
	b := NewBencher(svc.core)

	r := b.Bench(core.Background(), benchmark.Bench{Model: "/models/m"})
	if r.OK {
		t.Fatal("Bench against a failing binary: want fail, got OK")
	}
}
