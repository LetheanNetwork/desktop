// SPDX-Licence-Identifier: EUPL-1.2

package training

import (
	"testing"

	core "dappco.re/go"

	"dappco.re/lthn/desktop/pkg/clbpl"
	"dappco.re/lthn/desktop/pkg/paths"
	"dappco.re/lthn/desktop/pkg/r1"
)

// liftPipeBufLimit raises paths.AtomicAppendLine's record-size ceiling
// for the duration of the test. R₁ records carry full sandwich prompts
// (~3 KB) plus model responses, which trip the per-platform PIPE_BUF
// ceiling (512 B on Darwin). Production r1.Write callers will need a
// production-side decision on this (see report-back to Cladius); for
// the orchestrator tests we lift the ceiling so the canonical write
// path exercises end-to-end.
func liftPipeBufLimit(t *testing.T) {
	t.Helper()
	paths.SetPipeBufLimitForTest(1 << 20)
	t.Cleanup(func() { paths.SetPipeBufLimitForTest(0) })
}

// seedFixture writes a small per-test seed corpus and returns its root.
//
// Layout matches the canonical pkg/seeds shape — one JSONL file per
// subject, one record per line with seed_id + prompt fields.
//
//	root := seedFixture(t, map[string][]string{
//	    "english": {"P01_TEST", "P02_TEST"},
//	})
func seedFixture(t *testing.T, subjects map[string][]string) string {
	t.Helper()
	root := t.TempDir()
	for subject, probes := range subjects {
		dir := core.PathJoin(root, subject)
		if r := core.MkdirAll(dir, 0o755); !r.OK {
			t.Fatalf("MkdirAll(%s): %v", dir, r.Value)
		}
		var buf []byte
		for _, probeID := range probes {
			rec := map[string]any{
				"seed_id": probeID,
				"prompt":  "describe " + probeID,
			}
			enc := core.JSONMarshal(rec)
			if !enc.OK {
				t.Fatalf("JSONMarshal: %v", enc.Value)
			}
			buf = append(buf, enc.Value.([]byte)...)
			buf = append(buf, '\n')
		}
		file := core.PathJoin(dir, "seeds.jsonl")
		if r := core.WriteFile(file, buf, 0o644); !r.OK {
			t.Fatalf("WriteFile(%s): %v", file, r.Value)
		}
	}
	return root
}

// tightCLBPL returns CL-BPL Options tuned to fire EventGroked quickly
// against the FixtureRunner's damped-cosine curve — small window,
// loose threshold, low confirm-count.
func tightCLBPL() clbpl.Options {
	return clbpl.Options{
		Window:           4,
		GrokThreshold:    0.15,
		GrokConfirmPeaks: 2,
	}
}

// --- Good ---

func TestServiceTraining_Run_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	liftPipeBufLimit(t)
	root := seedFixture(t, map[string][]string{
		"english": {"P01_TEST", "P02_TEST", "P03_TEST"},
	})

	c := core.New()
	svc := NewService(c, Options{
		Subjects:         []string{"english"},
		SeedsRoot:        root,
		ProbesPerSubject: 3,
		CLBPLOptions:     tightCLBPL(),
		Epoch2MaxSteps:   200,
	})
	if r := svc.Register(c); !r.OK {
		t.Fatalf("Register: %v", r.Value)
	}

	runner := NewFixtureRunner(FixtureConfig{
		Model:        "fixture-e2b",
		Tier:         0,
		Substrate:    "CONT",
		StepsPerGrok: 60,
	})
	res := svc.Run(c.Context(), runner)
	if !res.OK {
		t.Fatalf("Run: %v", res.Value)
	}
	complete, ok := res.Value.(RunCompleteEvent)
	if !ok {
		t.Fatalf("Run returned %T, want RunCompleteEvent", res.Value)
	}
	if complete.TotalSubjects != 1 {
		t.Errorf("TotalSubjects = %d, want 1", complete.TotalSubjects)
	}
	if complete.Cancelled {
		t.Error("Run reported Cancelled, want non-cancelled completion")
	}

	// R₁ corpus should have at least one record persisted.
	read := r1.Read(r1.Filter{Model: "fixture-e2b", Subject: "english"})
	if !read.OK {
		t.Fatalf("r1.Read: %v", read.Value)
	}
	recs := read.Value.([]r1.R1)
	if len(recs) == 0 {
		t.Fatal("expected at least one R₁ persisted, got none")
	}
	for _, rec := range recs {
		if rec.Substrate != "CONT" {
			t.Errorf("rec.Substrate = %q, want CONT", rec.Substrate)
		}
		if rec.Tier != 0 {
			t.Errorf("rec.Tier = %d, want 0", rec.Tier)
		}
		if rec.Kernel != "lek-1" {
			t.Errorf("rec.Kernel = %q, want lek-1", rec.Kernel)
		}
		if rec.Hash == "" {
			t.Error("rec.Hash empty — r1.Write should have enriched")
		}
	}
}

func TestServiceTraining_Status_Good(t *testing.T) {
	c := core.New()
	svc := NewService(c, Options{})
	st := svc.Status()
	if !st.OK {
		t.Fatalf("Status: %v", st.Value)
	}
	snap := st.Value.(Status)
	if snap.Running {
		t.Error("fresh Service should not be Running")
	}
	if snap.CompletedRuns != 0 {
		t.Errorf("fresh Service CompletedRuns = %d, want 0", snap.CompletedRuns)
	}
}

func TestServiceTraining_RegisterAction_Good(t *testing.T) {
	c := core.New()
	svc := NewService(c, Options{})
	if r := svc.Register(c); !r.OK {
		t.Fatalf("Register: %v", r.Value)
	}
	// training.status action should be reachable.
	res := c.Action("training.status").Run(core.Background(), core.NewOptions())
	if !res.OK {
		t.Fatalf("training.status: %v", res.Value)
	}
	if _, ok := res.Value.(Status); !ok {
		t.Fatalf("training.status returned %T, want Status", res.Value)
	}
}

// --- Bad ---

func TestServiceTraining_Run_BadNilRunner(t *testing.T) {
	c := core.New()
	svc := NewService(c, Options{Subjects: []string{"english"}})
	if r := svc.Run(c.Context(), nil); r.OK {
		t.Fatal("Run with nil runner returned OK, want Fail")
	}
}

func TestServiceTraining_Run_BadConcurrentRotation(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	liftPipeBufLimit(t)
	root := seedFixture(t, map[string][]string{
		"english": {"P01_TEST"},
	})

	c := core.New()
	svc := NewService(c, Options{
		Subjects:         []string{"english"},
		SeedsRoot:        root,
		ProbesPerSubject: 1,
		CLBPLOptions:     tightCLBPL(),
		Epoch2MaxSteps:   400,
	})
	_ = svc.Register(c)

	// Block the first Run on a never-fires context; race a second
	// Run against the live runMu. The second call should Fail with
	// "rotation already in progress" — observable BEFORE the first
	// completes by gating the first call's runner on a slow loss
	// channel.
	gate := make(chan struct{})
	slow := &gatedRunner{
		inner: NewFixtureRunner(FixtureConfig{
			Model: "fixture-e2b", Substrate: "CONT",
			StepsPerGrok: 200, // never groks within Epoch2MaxSteps
		}),
		gate: gate,
	}
	done := make(chan core.Result, 1)
	go func() { done <- svc.Run(c.Context(), slow) }()
	// Wait until the first lane has acquired runMu (slow runner
	// records that via its mu-lock count).
	waitFor(t, func() bool { return slow.entered() })

	// Second concurrent Run must Fail immediately.
	second := svc.Run(c.Context(), NewFixtureRunner(FixtureConfig{Model: "fixture-2"}))
	if second.OK {
		t.Error("second concurrent Run returned OK, want Fail")
	}

	// Release the slow lane so the test exits cleanly.
	close(gate)
	<-done
}

func TestServiceTraining_Register_BadNilCore(t *testing.T) {
	svc := NewService(nil, Options{})
	if r := svc.Register(nil); r.OK {
		t.Error("Register(nil) returned OK, want Fail")
	}
}

// --- Ugly ---

func TestServiceTraining_Run_UglyContextCancel(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	liftPipeBufLimit(t)
	root := seedFixture(t, map[string][]string{
		"english":  {"P01_TEST", "P02_TEST"},
		"european": {"P03_TEST", "P04_TEST"},
	})

	c := core.New()
	svc := NewService(c, Options{
		Subjects:         []string{"english", "european"},
		SeedsRoot:        root,
		ProbesPerSubject: 2,
		CLBPLOptions:     tightCLBPL(),
		Epoch2MaxSteps:   5000,
	})
	_ = svc.Register(c)

	// Gated runner: epoch-1 R₁ writes succeed normally; epoch-2
	// StepBatch blocks on a channel so the test can observe
	// Run-in-flight state and trigger cancel deterministically.
	gate := make(chan struct{})
	runner := &gatedRunner{
		inner: NewFixtureRunner(FixtureConfig{
			Model: "fixture-cancel", Substrate: "CONT",
		}),
		gate: gate,
	}

	ctx, cancel := core.WithCancel(c.Context())
	done := make(chan core.Result, 1)
	go func() { done <- svc.Run(ctx, runner) }()

	// Wait until the runner has been entered — guarantees epoch-1
	// R₁.Write fired and epoch-2 is blocked on the gate.
	waitFor(t, runner.entered)
	cancel()
	close(gate)

	res := <-done
	if !res.OK {
		t.Fatalf("Run returned Fail after cancel: %v", res.Value)
	}
	complete := res.Value.(RunCompleteEvent)
	if !complete.Cancelled {
		t.Error("Run after cancel reported Cancelled=false, want true")
	}

	// At least one R₁ should have been persisted before cancel —
	// cancellation is clean-shutdown, not lose-everything.
	read := r1.Read(r1.Filter{Model: "fixture-cancel"})
	if !read.OK {
		t.Fatalf("r1.Read: %v", read.Value)
	}
	if len(read.Value.([]r1.R1)) == 0 {
		t.Error("expected at least one R₁ persisted before cancel, got none")
	}
}

func TestServiceTraining_Run_UglyEmptySubjects(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	// SeedsRoot points at an empty corpus root — every subject reads
	// zero probes, no R₁s persist, Run still returns Ok with
	// TotalSubjects = 7.
	c := core.New()
	svc := NewService(c, Options{
		SeedsRoot:    t.TempDir(),
		CLBPLOptions: tightCLBPL(),
	})
	res := svc.Run(c.Context(), NewFixtureRunner(FixtureConfig{Model: "fixture"}))
	if !res.OK {
		t.Fatalf("Run on empty corpus: %v", res.Value)
	}
	complete := res.Value.(RunCompleteEvent)
	if complete.TotalSubjects != len(canonicalSubjects) {
		t.Errorf("TotalSubjects = %d, want %d", complete.TotalSubjects, len(canonicalSubjects))
	}
	if complete.GrokedSubjects != 0 {
		t.Errorf("GrokedSubjects = %d on empty corpus, want 0", complete.GrokedSubjects)
	}
}

func TestServiceTraining_Run_UglyDefaultSubjectsApplied(t *testing.T) {
	svc := NewService(nil, Options{})
	if len(svc.opts.Subjects) != len(canonicalSubjects) {
		t.Errorf("default subjects = %d, want %d", len(svc.opts.Subjects), len(canonicalSubjects))
	}
	for i, want := range canonicalSubjects {
		if svc.opts.Subjects[i] != want {
			t.Errorf("subject[%d] = %q, want %q", i, svc.opts.Subjects[i], want)
		}
	}
}

// --- helpers ---

// gatedRunner wraps a FixtureRunner with a channel that StepBatch
// blocks on, used to keep a Run lane alive long enough for the test
// to observe runMu contention. Implements Runner.
type gatedRunner struct {
	inner   *FixtureRunner
	gate    chan struct{}
	mu      core.Mutex
	stepped bool
}

func (g *gatedRunner) StepBatch(prompt, target string) core.Result {
	g.mu.Lock()
	g.stepped = true
	g.mu.Unlock()
	// Block until the gate is closed so the rotation cannot finish
	// before the second Run attempt races against runMu.
	<-g.gate
	return core.Ok(0.3)
}

func (g *gatedRunner) GenerateResponse(prompt string) core.Result {
	g.mu.Lock()
	g.stepped = true
	g.mu.Unlock()
	return g.inner.GenerateResponse(prompt)
}
func (g *gatedRunner) ModelID() string   { return g.inner.ModelID() }
func (g *gatedRunner) Substrate() string { return g.inner.Substrate() }
func (g *gatedRunner) Tier() int         { return g.inner.Tier() }

func (g *gatedRunner) entered() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.stepped
}

// hostileRunner wraps a FixtureRunner with a custom loss curve so
// guard tests can exercise NaN-at-step-N and monotonic-rise shapes
// without touching FixtureRunner itself. GenerateResponse / ModelID /
// Substrate / Tier all forward to the inner fixture so epoch 1 still
// captures R₁s the same way the production lane does.
//
// nanAt: if >= 0, the StepBatch call at this zero-based step returns
// core.IsNaN(loss) instead of the configured curve. -1 disables.
// curve: pure function from step index → loss value (called only when
// nanAt does not match). nil falls back to a constant 0.3.
// onStep: optional hook invoked AFTER each StepBatch returns; useful
// for tests that need to observe progress (cancellation race).
type hostileRunner struct {
	inner  *FixtureRunner
	nanAt  int
	curve  func(step int) float64
	mu     core.Mutex
	step   int
	onStep func(step int)
}

func (h *hostileRunner) StepBatch(_, _ string) core.Result {
	h.mu.Lock()
	step := h.step
	h.step++
	h.mu.Unlock()
	if h.onStep != nil {
		h.onStep(step)
	}
	if h.nanAt >= 0 && step == h.nanAt {
		// 0/0 produces NaN without importing math from the test file.
		nan := 0.0 / func() float64 { return 0.0 }()
		return core.Ok(nan)
	}
	if h.curve == nil {
		return core.Ok(0.3)
	}
	return core.Ok(h.curve(step))
}

func (h *hostileRunner) GenerateResponse(p string) core.Result { return h.inner.GenerateResponse(p) }
func (h *hostileRunner) ModelID() string                       { return h.inner.ModelID() }
func (h *hostileRunner) Substrate() string                     { return h.inner.Substrate() }
func (h *hostileRunner) Tier() int                             { return h.inner.Tier() }

// newHostileRunner constructs a hostileRunner sharing the
// FixtureRunner accessor surface for ModelID / Substrate / Tier.
func newHostileRunner(model string) *hostileRunner {
	return &hostileRunner{
		inner: NewFixtureRunner(FixtureConfig{Model: model, Substrate: "CONT"}),
		nanAt: -1,
	}
}

// --- Guard tests ---

func TestGuard_GoodNoDivergence(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	liftPipeBufLimit(t)
	root := seedFixture(t, map[string][]string{
		"english": {"P01_TEST"},
	})
	c := core.New()
	svc := NewService(c, Options{
		Subjects:         []string{"english"},
		SeedsRoot:        root,
		ProbesPerSubject: 1,
		CLBPLOptions:     tightCLBPL(),
		Epoch2MaxSteps:   200,
	})
	_ = svc.Register(c)

	runner := NewFixtureRunner(FixtureConfig{
		Model: "guard-good", Substrate: "CONT", StepsPerGrok: 60,
	})
	res := svc.Run(c.Context(), runner)
	if !res.OK {
		t.Fatalf("Run: %v", res.Value)
	}
	snap := svc.Status().Value.(Status)
	if snap.Divergence != nil {
		t.Errorf("Status.Divergence = %+v, want nil on clean run", snap.Divergence)
	}
}

func TestGuard_BadNaNLossPausesRun(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	liftPipeBufLimit(t)
	root := seedFixture(t, map[string][]string{
		"english": {"P01_TEST"},
	})
	c := core.New()
	svc := NewService(c, Options{
		Subjects:         []string{"english"},
		SeedsRoot:        root,
		ProbesPerSubject: 1,
		CLBPLOptions:     tightCLBPL(),
		Epoch2MaxSteps:   500,
	})
	_ = svc.Register(c)

	runner := newHostileRunner("guard-nan")
	runner.nanAt = 50
	runner.curve = func(step int) float64 { return 0.3 } // benign before NaN

	res := svc.Run(c.Context(), runner)
	if res.OK {
		t.Fatalf("Run returned OK, want Fail with training.guard.nan")
	}
	errStr := core.Sprintf("%v", res.Value)
	if !containsAll(errStr, "training.guard.nan") {
		t.Errorf("error %q missing training.guard.nan scope", errStr)
	}

	snap := svc.Status().Value.(Status)
	if snap.Divergence == nil {
		t.Fatal("Status.Divergence is nil, want populated")
	}
	if snap.Divergence.Reason != DivergenceReasonNaNLoss {
		t.Errorf("Divergence.Reason = %q, want %q", snap.Divergence.Reason, DivergenceReasonNaNLoss)
	}
	if snap.Divergence.Step != 50 {
		t.Errorf("Divergence.Step = %d, want 50", snap.Divergence.Step)
	}

	// R₁ captured before the trip should be on disk — epoch-1 fires
	// before any StepBatch call, so the corpus has the response.
	read := r1.Read(r1.Filter{Model: "guard-nan", Subject: "english"})
	if !read.OK {
		t.Fatalf("r1.Read: %v", read.Value)
	}
	if len(read.Value.([]r1.R1)) == 0 {
		t.Error("expected R₁ persisted before NaN trip, got none")
	}
}

func TestGuard_BadMonotonicRisePausesRun(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	liftPipeBufLimit(t)
	root := seedFixture(t, map[string][]string{
		"english": {"P01_TEST"},
	})
	c := core.New()
	svc := NewService(c, Options{
		Subjects:         []string{"english"},
		SeedsRoot:        root,
		ProbesPerSubject: 1,
		CLBPLOptions:     tightCLBPL(),
		Epoch2MaxSteps:   500,
	})
	_ = svc.Register(c)

	runner := newHostileRunner("guard-mono")
	runner.curve = func(step int) float64 { return float64(step) * 0.01 }

	res := svc.Run(c.Context(), runner)
	if res.OK {
		t.Fatalf("Run returned OK, want Fail with training.guard.divergence")
	}
	errStr := core.Sprintf("%v", res.Value)
	if !containsAll(errStr, "training.guard.divergence") {
		t.Errorf("error %q missing training.guard.divergence scope", errStr)
	}

	snap := svc.Status().Value.(Status)
	if snap.Divergence == nil {
		t.Fatal("Status.Divergence is nil, want populated")
	}
	if snap.Divergence.Reason != DivergenceReasonMonotonicRise {
		t.Errorf("Divergence.Reason = %q, want %q", snap.Divergence.Reason, DivergenceReasonMonotonicRise)
	}
	if snap.Divergence.WindowSize != divergenceWindowSize {
		t.Errorf("Divergence.WindowSize = %d, want %d", snap.Divergence.WindowSize, divergenceWindowSize)
	}
}

func TestGuard_UglyExactly49Rises_DoesNotTrigger(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	liftPipeBufLimit(t)
	root := seedFixture(t, map[string][]string{
		"english": {"P01_TEST"},
	})
	c := core.New()
	svc := NewService(c, Options{
		Subjects:         []string{"english"},
		SeedsRoot:        root,
		ProbesPerSubject: 1,
		CLBPLOptions:     tightCLBPL(),
		// Cap at exactly the 49-rise + 1-drop sequence + tail; no
		// more steps after the drop, so the test exits via budget
		// exhaust rather than running into another rise streak.
		Epoch2MaxSteps: 51,
	})
	_ = svc.Register(c)

	// 49 strictly-rising samples (steps 0..48) followed by a drop at
	// step 49. The guard counts CONSECUTIVE rises — step 49's drop
	// resets the streak to 0, and step 50 (final step under cap) is
	// a fresh start, so the guard never trips.
	runner := newHostileRunner("guard-49")
	runner.curve = func(step int) float64 {
		if step < 49 {
			return float64(step) * 0.01 // 0.00, 0.01, ..., 0.48
		}
		if step == 49 {
			return 0.05 // drop — resets the rise streak
		}
		return 0.05
	}

	res := svc.Run(c.Context(), runner)
	if !res.OK {
		t.Fatalf("Run returned Fail at 49 rises; guard tripped early: %v", res.Value)
	}
	snap := svc.Status().Value.(Status)
	if snap.Divergence != nil {
		t.Errorf("Status.Divergence = %+v, want nil — 49 rises must NOT trip the guard", snap.Divergence)
	}
}

func TestGuard_UglyNaNAfterCancel_StopReturnsCleanly(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	liftPipeBufLimit(t)
	root := seedFixture(t, map[string][]string{
		"english": {"P01_TEST"},
	})
	c := core.New()
	svc := NewService(c, Options{
		Subjects:         []string{"english"},
		SeedsRoot:        root,
		ProbesPerSubject: 1,
		CLBPLOptions:     tightCLBPL(),
		Epoch2MaxSteps:   5000,
	})
	_ = svc.Register(c)

	// Cancel after step 5 has been observed; the runner returns NaN
	// from step 100 onward. Whichever fires first wins — cancel is
	// checked at the top of the loop, NaN inside the loop after
	// StepBatch returns. Both paths must produce a recoverable Run
	// shape (Fail-with-nan-scope OR Ok-with-Cancelled=true), and the
	// Service must not panic or deadlock.
	ctx, cancel := core.WithCancel(c.Context())
	runner := newHostileRunner("guard-cancel-nan")
	runner.curve = func(step int) float64 { return 0.3 }
	runner.nanAt = 100
	runner.onStep = func(step int) {
		if step == 5 {
			cancel()
		}
	}

	res := svc.Run(ctx, runner)
	// Either outcome is acceptable per the ticket — whichever fires
	// first wins. The important property is that Run returns and the
	// Service is back to a clean state.
	if res.OK {
		complete, ok := res.Value.(RunCompleteEvent)
		if !ok {
			t.Fatalf("Run returned OK %T, want RunCompleteEvent", res.Value)
		}
		if !complete.Cancelled {
			t.Errorf("Run returned OK but Cancelled=false; expected one of {Fail/nan, Ok/Cancelled}")
		}
	} else {
		errStr := core.Sprintf("%v", res.Value)
		if !containsAll(errStr, "training.guard.nan") {
			t.Errorf("Run returned Fail %q, want training.guard.nan scope", errStr)
		}
	}

	// Service must be in a clean state — running=false, status
	// reachable, no panic.
	snap := svc.Status().Value.(Status)
	if snap.Running {
		t.Error("Status.Running = true after Run returned, want false")
	}
}

// containsAll reports whether needle appears in haystack. Avoids
// importing strings per AX-6; runs over rune boundaries fine for ASCII
// scope strings used in core.E.
func containsAll(haystack, needle string) bool {
	if len(needle) == 0 {
		return true
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

// waitFor polls cond every 5ms up to 5 seconds, t.Fatal-ing if it
// never holds. Avoids importing time directly per AX-6.
func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline, cancel := core.WithTimeout(core.Background(), 5*core.Second)
	defer cancel()
	tick, tickCancel := core.WithTimeout(core.Background(), 5*core.Millisecond)
	_ = tick
	tickCancel()
	for {
		if cond() {
			return
		}
		select {
		case <-deadline.Done():
			t.Fatal("waitFor: condition never held within 5s")
		default:
		}
		// Cheap busy-yield via a short context.
		yield, yieldCancel := core.WithTimeout(core.Background(), 5*core.Millisecond)
		<-yield.Done()
		yieldCancel()
	}
}

// --- Checkpoint integration ---

// TestServiceTraining_Run_ClearsCheckpointOnCleanExit_Good runs a
// fixture rotation end-to-end and asserts the canonical checkpoint
// path is empty after the Run completes cleanly — the orchestrator
// must NOT leave stale rotation state on disk that a future Run
// would resume from.
func TestServiceTraining_Run_ClearsCheckpointOnCleanExit_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	liftPipeBufLimit(t)
	root := seedFixture(t, map[string][]string{
		"english": {"P01_TEST", "P02_TEST"},
	})

	c := core.New()
	svc := NewService(c, Options{
		Subjects:         []string{"english"},
		SeedsRoot:        root,
		ProbesPerSubject: 2,
		CLBPLOptions:     tightCLBPL(),
		Epoch2MaxSteps:   100,
	})
	_ = svc.Register(c)

	runner := NewFixtureRunner(FixtureConfig{
		Model:        "fixture-ckpt-clean",
		Tier:         0,
		Substrate:    "CONT",
		StepsPerGrok: 60,
	})
	res := svc.Run(c.Context(), runner)
	if !res.OK {
		t.Fatalf("Run: %v", res.Value)
	}

	// Checkpoint must be cleared on clean completion.
	r := LoadCheckpoint("fixture-ckpt-clean")
	if !r.OK {
		t.Fatalf("LoadCheckpoint post-Run: %v", r.Error())
	}
	if cp := r.Value.(*Checkpoint); cp != nil {
		t.Errorf("post-Run checkpoint should be nil, got %+v", cp)
	}
}

// TestServiceTraining_Run_PersistsCheckpointOnDivergence_Good drives
// a rotation through a divergence trip and asserts the checkpoint
// REMAINS on disk so the operator can inspect rotation state at the
// moment the guard fired.
func TestServiceTraining_Run_PersistsCheckpointOnDivergence_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	liftPipeBufLimit(t)
	root := seedFixture(t, map[string][]string{
		"english": {"P01_TEST"},
	})

	// SeedSequence with NaN at index 0 → first StepBatch returns NaN →
	// the in-loop guard trips immediately, ending the Run with Fail.
	naN := core.NaN()
	runner := NewFixtureRunner(FixtureConfig{
		Model:        "fixture-ckpt-diverge",
		Tier:         0,
		Substrate:    "CONT",
		SeedSequence: []float64{naN},
	})

	c := core.New()
	svc := NewService(c, Options{
		Subjects:         []string{"english"},
		SeedsRoot:        root,
		ProbesPerSubject: 1,
		Epoch2MaxSteps:   100,
	})
	_ = svc.Register(c)

	res := svc.Run(c.Context(), runner)
	if res.OK {
		t.Fatal("Run with NaN runner should Fail, got OK")
	}

	// Checkpoint must remain so the operator can inspect.
	r := LoadCheckpoint("fixture-ckpt-diverge")
	if !r.OK {
		t.Fatalf("LoadCheckpoint post-divergence: %v", r.Error())
	}
	cp, _ := r.Value.(*Checkpoint)
	if cp == nil {
		t.Fatal("post-divergence checkpoint should remain, got nil")
	}
	if cp.Model != "fixture-ckpt-diverge" {
		t.Errorf("checkpoint Model = %q, want fixture-ckpt-diverge", cp.Model)
	}
	if cp.Substrate != "CONT" {
		t.Errorf("checkpoint Substrate = %q, want CONT", cp.Substrate)
	}
	if len(cp.CompletedProbes) != 1 || cp.CompletedProbes[0].ProbeID != "P01_TEST" {
		t.Errorf("checkpoint CompletedProbes = %v, want [{english P01_TEST}]", cp.CompletedProbes)
	}
}

// TestServiceTraining_Run_CheckpointStampsRunnerIdentity_Good
// asserts the checkpoint persisted during a rotation carries the
// runner's Model + Substrate + Tier verbatim. Load-bearing for the
// substrate-shift experiment per worf/01-theory.md H1 — the resume
// path needs to know whether the saved curriculum was CONT or TRAD.
func TestServiceTraining_Run_CheckpointStampsRunnerIdentity_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	liftPipeBufLimit(t)
	root := seedFixture(t, map[string][]string{
		"english": {"P01_TEST"},
	})

	// Force divergence so the checkpoint survives for inspection.
	naN := core.NaN()
	runner := NewFixtureRunner(FixtureConfig{
		Model:        "fixture-ident-stamp",
		Tier:         2,
		Substrate:    "TRAD",
		SeedSequence: []float64{naN},
	})

	c := core.New()
	svc := NewService(c, Options{
		Subjects:         []string{"english"},
		SeedsRoot:        root,
		ProbesPerSubject: 1,
		Epoch2MaxSteps:   100,
	})
	_ = svc.Register(c)

	_ = svc.Run(c.Context(), runner)

	r := LoadCheckpoint("fixture-ident-stamp")
	if !r.OK {
		t.Fatalf("LoadCheckpoint: %v", r.Error())
	}
	cp := r.Value.(*Checkpoint)
	if cp == nil {
		t.Fatal("checkpoint should exist post-divergence")
	}
	if cp.Tier != 2 {
		t.Errorf("Tier = %d, want 2", cp.Tier)
	}
	if cp.Substrate != "TRAD" {
		t.Errorf("Substrate = %q, want TRAD", cp.Substrate)
	}
	if cp.StartedAt == 0 {
		t.Error("StartedAt should be stamped during Run")
	}
	if cp.SavedAt == 0 {
		t.Error("SavedAt should be stamped at probe-boundary save")
	}
}

// --- Resume ---

// TestServiceTraining_Resume_SkipsGrokedSubject_Good resumes a
// rotation whose checkpoint has "english" in GrokedSubjects. The
// resume must not re-run english (already settled per CL-BPL); only
// "european" should execute. Counted as groked in the
// RunCompleteEvent payload via the resume-skip path so the operator
// sees the cumulative count.
func TestServiceTraining_Resume_SkipsGrokedSubject_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	liftPipeBufLimit(t)
	root := seedFixture(t, map[string][]string{
		"english":  {"P01_TEST"},
		"european": {"P10_TEST"},
	})

	cp := &Checkpoint{
		SchemaVersion:  CheckpointSchemaVersion,
		Model:          "fixture-resume-skip",
		Substrate:      "CONT",
		Tier:           0,
		Subjects:       []string{"english", "european"},
		SubjectIndex:   1,
		ProbeIndex:     0,
		GrokedSubjects: []string{"english"},
		StartedAt:      core.UnixNow() - 100,
	}
	// Persist the checkpoint so the post-Resume clean-complete path
	// has something to Clear (mirrors the real operator workflow).
	if r := SaveCheckpoint(*cp); !r.OK {
		t.Fatalf("SaveCheckpoint: %v", r.Error())
	}

	c := core.New()
	svc := NewService(c, Options{
		Subjects:         []string{"english", "european"},
		SeedsRoot:        root,
		ProbesPerSubject: 1,
		CLBPLOptions:     tightCLBPL(),
		Epoch2MaxSteps:   100,
	})
	_ = svc.Register(c)

	runner := NewFixtureRunner(FixtureConfig{
		Model:        "fixture-resume-skip",
		Tier:         0,
		Substrate:    "CONT",
		StepsPerGrok: 60,
	})
	res := svc.Resume(c.Context(), runner, cp)
	if !res.OK {
		t.Fatalf("Resume: %v", res.Value)
	}

	// English should NOT have written an R₁ (skipped via grokedSubjects).
	englishR := r1.Read(r1.Filter{Model: "fixture-resume-skip", Subject: "english"})
	if !englishR.OK {
		t.Fatalf("r1.Read english: %v", englishR.Error())
	}
	if len(englishR.Value.([]r1.R1)) != 0 {
		t.Errorf("english R₁ count = %d, want 0 (skipped by resume)", len(englishR.Value.([]r1.R1)))
	}

	// European SHOULD have written at least one R₁.
	europeanR := r1.Read(r1.Filter{Model: "fixture-resume-skip", Subject: "european"})
	if !europeanR.OK {
		t.Fatalf("r1.Read european: %v", europeanR.Error())
	}
	if len(europeanR.Value.([]r1.R1)) == 0 {
		t.Error("european R₁ count = 0, want at least 1 (resume should have run this subject)")
	}

	// Clean completion clears the checkpoint.
	loadR := LoadCheckpoint("fixture-resume-skip")
	if !loadR.OK {
		t.Fatalf("LoadCheckpoint post-Resume: %v", loadR.Error())
	}
	if loadR.Value.(*Checkpoint) != nil {
		t.Error("post-Resume checkpoint should be cleared after clean completion")
	}
}

// TestServiceTraining_Resume_SkipsCompletedProbe_Good resumes a
// rotation whose checkpoint has (english, P01_TEST) in
// CompletedProbes. P01 must be skipped at the probe entry — no new
// R₁ written for it — while P02 runs normally.
func TestServiceTraining_Resume_SkipsCompletedProbe_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	liftPipeBufLimit(t)
	root := seedFixture(t, map[string][]string{
		"english": {"P01_TEST", "P02_TEST"},
	})

	cp := &Checkpoint{
		SchemaVersion: CheckpointSchemaVersion,
		Model:         "fixture-resume-probe",
		Substrate:     "CONT",
		Tier:          0,
		Subjects:      []string{"english"},
		SubjectIndex:  0,
		ProbeIndex:    1,
		CompletedProbes: []ProbeKey{
			{Subject: "english", ProbeID: "P01_TEST"},
		},
		StartedAt: core.UnixNow() - 100,
	}
	if r := SaveCheckpoint(*cp); !r.OK {
		t.Fatalf("SaveCheckpoint: %v", r.Error())
	}

	c := core.New()
	svc := NewService(c, Options{
		Subjects:         []string{"english"},
		SeedsRoot:        root,
		ProbesPerSubject: 2,
		CLBPLOptions:     tightCLBPL(),
		Epoch2MaxSteps:   100,
	})
	_ = svc.Register(c)

	runner := NewFixtureRunner(FixtureConfig{
		Model:        "fixture-resume-probe",
		Tier:         0,
		Substrate:    "CONT",
		StepsPerGrok: 60,
	})
	res := svc.Resume(c.Context(), runner, cp)
	if !res.OK {
		t.Fatalf("Resume: %v", res.Value)
	}

	// R₁ count should be 1 (P02 ran; P01 was skipped).
	rr := r1.Read(r1.Filter{Model: "fixture-resume-probe", Subject: "english"})
	if !rr.OK {
		t.Fatalf("r1.Read: %v", rr.Error())
	}
	recs := rr.Value.([]r1.R1)
	if len(recs) != 1 {
		t.Errorf("R₁ count = %d, want 1 (P01 skipped, P02 ran)", len(recs))
	}
	for _, rec := range recs {
		if rec.ProbeID == "P01_TEST" {
			t.Errorf("found P01_TEST R₁ — should have been skipped by resume")
		}
	}
}

// TestServiceTraining_Resume_PreservesStartedAt_Good asserts that the
// checkpoint's StartedAt is carried forward across the resume run so
// the operator's "rotation has been alive for N hours" UX reads the
// original rotation origin, not the resume point.
func TestServiceTraining_Resume_PreservesStartedAt_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	liftPipeBufLimit(t)
	root := seedFixture(t, map[string][]string{
		"english": {"P01_TEST"},
	})

	originalStart := core.UnixNow() - 3600 // an hour ago
	naN := core.NaN()
	runner := NewFixtureRunner(FixtureConfig{
		Model:        "fixture-resume-start",
		Tier:         0,
		Substrate:    "CONT",
		SeedSequence: []float64{naN}, // force divergence so checkpoint stays
	})
	cp := &Checkpoint{
		SchemaVersion: CheckpointSchemaVersion,
		Model:         "fixture-resume-start",
		Substrate:     "CONT",
		Tier:          0,
		Subjects:      []string{"english"},
		StartedAt:     originalStart,
	}

	c := core.New()
	svc := NewService(c, Options{
		Subjects:         []string{"english"},
		SeedsRoot:        root,
		ProbesPerSubject: 1,
		Epoch2MaxSteps:   100,
	})
	_ = svc.Register(c)

	// Resume against the divergence-producing runner — Run will fail
	// (NaN guard trips), leaving the checkpoint on disk.
	_ = svc.Resume(c.Context(), runner, cp)

	// The persisted checkpoint after divergence must carry originalStart.
	r := LoadCheckpoint("fixture-resume-start")
	if !r.OK {
		t.Fatalf("LoadCheckpoint: %v", r.Error())
	}
	got := r.Value.(*Checkpoint)
	if got == nil {
		t.Fatal("checkpoint should exist post-divergence")
	}
	if got.StartedAt != originalStart {
		t.Errorf("post-Resume StartedAt = %d, want %d (resume must preserve original origin)",
			got.StartedAt, originalStart)
	}
}

// --- Resume Bad/Ugly ---

func TestServiceTraining_Resume_NilCheckpoint_Bad(t *testing.T) {
	c := core.New()
	svc := NewService(c, Options{Subjects: []string{"english"}})
	runner := NewFixtureRunner(FixtureConfig{Model: "fixture-resume-nil"})
	if r := svc.Resume(c.Context(), runner, nil); r.OK {
		t.Error("Resume with nil checkpoint returned OK, want Fail")
	}
}

func TestServiceTraining_Resume_NilRunner_Bad(t *testing.T) {
	c := core.New()
	svc := NewService(c, Options{Subjects: []string{"english"}})
	cp := &Checkpoint{SchemaVersion: CheckpointSchemaVersion, Model: "any"}
	if r := svc.Resume(c.Context(), nil, cp); r.OK {
		t.Error("Resume with nil runner returned OK, want Fail")
	}
}

func TestServiceTraining_Resume_ModelMismatch_Bad(t *testing.T) {
	c := core.New()
	svc := NewService(c, Options{Subjects: []string{"english"}})
	runner := NewFixtureRunner(FixtureConfig{Model: "runner-A"})
	cp := &Checkpoint{
		SchemaVersion: CheckpointSchemaVersion,
		Model:         "runner-B",
		Subjects:      []string{"english"},
	}
	if r := svc.Resume(c.Context(), runner, cp); r.OK {
		t.Error("Resume with mismatched model returned OK, want Fail")
	}
}

func TestServiceTraining_Resume_SchemaMismatch_Bad(t *testing.T) {
	c := core.New()
	svc := NewService(c, Options{Subjects: []string{"english"}})
	runner := NewFixtureRunner(FixtureConfig{Model: "runner-future"})
	cp := &Checkpoint{
		SchemaVersion: 999,
		Model:         "runner-future",
		Subjects:      []string{"english"},
	}
	if r := svc.Resume(c.Context(), runner, cp); r.OK {
		t.Error("Resume with unknown schema_version returned OK, want Fail")
	}
}
