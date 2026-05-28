// SPDX-Licence-Identifier: EUPL-1.2

package training

import (
	"math"

	core "dappco.re/go"
)

// FixtureConfig parameterises a FixtureRunner.
//
// Model, Substrate, Tier are returned verbatim by the corresponding
// Runner accessor — they end up stamped on every R₁ the Service writes
// while the fixture is in use.
//
// StepsPerGrok is the number of steps the fixture's loss curve takes
// to traverse from "first peak" through the configured detector's
// confirm-count, so the orchestrator reliably observes EventGroked. A
// value < 30 is recommended for tests; production GUI fixture mode can
// use the default (60).
//
// SeedSequence, when set, is the deterministic loss sequence the
// fixture replays from step 0. Empty SeedSequence falls back to a
// damped-cosine envelope shaped to produce StepsPerGrok worth of peaks
// inside clbpl.Defaults() bounds.
//
//	cfg := training.FixtureConfig{
//	    Model: "gemma4-e2b-it-q4", Tier: 0, Substrate: "CONT",
//	    StepsPerGrok: 24,
//	}
type FixtureConfig struct {
	Model        string
	Substrate    string
	Tier         int
	StepsPerGrok int
	SeedSequence []float64
}

// FixtureRunner implements Runner with deterministic synthetic
// responses + a damped-cosine loss curve.
//
// Used by the package's own tests (so they have no dependency on a
// real go-mlx runner) and by the GUI fixture mode (so the training
// surface is exercisable without a model loaded).
//
//	r := training.NewFixtureRunner(training.FixtureConfig{
//	    Model: "gemma4-e2b-it-q4", Tier: 0, Substrate: "CONT",
//	})
//	// Pass to Service.Run; the fixture produces ~24 steps to grok per probe.
type FixtureRunner struct {
	cfg FixtureConfig
	// step is the global step counter across all StepBatch calls.
	// Drives the synthetic loss sequence.
	mu   core.Mutex
	step int
	// genCount tracks how many GenerateResponse calls have happened
	// so the synthetic response text varies per call without becoming
	// random (deterministic for golden-record tests).
	genCount int
}

// NewFixtureRunner constructs a FixtureRunner. Zero-valued cfg fields
// fall back to defaults that make the runner immediately useful in
// tests:
//
//   - Model defaults to "fixture-runner".
//   - Substrate defaults to "CONT".
//   - Tier defaults to 0 (E2B-equivalent).
//   - StepsPerGrok defaults to 60 — enough peaks for clbpl.Defaults
//     to fire EventGroked under the canonical Window=8 / Confirm=3.
//
//	r := training.NewFixtureRunner(training.FixtureConfig{})
//	_ = r.ModelID() // "fixture-runner"
func NewFixtureRunner(cfg FixtureConfig) *FixtureRunner {
	if cfg.Model == "" {
		cfg.Model = "fixture-runner"
	}
	if cfg.Substrate == "" {
		cfg.Substrate = "CONT"
	}
	if cfg.StepsPerGrok <= 0 {
		cfg.StepsPerGrok = 60
	}
	return &FixtureRunner{cfg: cfg}
}

// StepBatch returns a deterministic loss value derived from a
// damped-cosine envelope: peaks shrink linearly with step count so the
// CL-BPL detector reliably sees N consecutive in-threshold peaks and
// fires EventGroked.
//
//	loss := f.StepBatch("probe", "target").Value.(float64)
func (f *FixtureRunner) StepBatch(_, _ string) core.Result {
	f.mu.Lock()
	step := f.step
	f.step++
	f.mu.Unlock()

	if len(f.cfg.SeedSequence) > 0 {
		// Replay the seed sequence, wrapping when exhausted. Useful
		// for deterministic curve-shape tests that feed a specific
		// loss-vs-step trajectory.
		loss := f.cfg.SeedSequence[step%len(f.cfg.SeedSequence)]
		return core.Ok(loss)
	}

	// Damped cosine: loss = base + amp * cos(2π * step / period).
	// amp decays linearly with step so peaks shrink — exactly the
	// pattern the CL-BPL detector is built to catch.
	//
	// Tuned so that the third peak amplitude lies inside the default
	// GrokThreshold=0.05 envelope, given StepsPerGrok in [24..120].
	period := 8.0
	if f.cfg.StepsPerGrok >= 24 {
		period = float64(f.cfg.StepsPerGrok) / 6.0
	}
	stepF := float64(step)
	decay := 1.0 - (stepF / float64(f.cfg.StepsPerGrok))
	if decay < 0 {
		decay = 0
	}
	amp := 0.5 * decay
	loss := 0.3 + amp*math.Cos(2.0*math.Pi*stepF/period)
	return core.Ok(loss)
}

// GenerateResponse returns a deterministic synthetic response string.
//
// The output encodes the call sequence so callers verifying R₁ corpus
// content can assert exact bytes without needing to mock the runner.
//
//	res := f.GenerateResponse("probe").Value.(string)
//	// e.g. "fixture-response-0", "fixture-response-1", ...
func (f *FixtureRunner) GenerateResponse(_ string) core.Result {
	f.mu.Lock()
	idx := f.genCount
	f.genCount++
	f.mu.Unlock()
	return core.Ok(core.Sprintf("fixture-response-%d", idx))
}

// ModelID returns the configured model identifier.
func (f *FixtureRunner) ModelID() string { return f.cfg.Model }

// Substrate returns the configured substrate label.
func (f *FixtureRunner) Substrate() string { return f.cfg.Substrate }

// Tier returns the configured cascade tier.
func (f *FixtureRunner) Tier() int { return f.cfg.Tier }

// Reset returns the fixture to step 0 / genCount 0. Tests use this to
// re-run a deterministic sequence inside a single test function.
//
//	f.Reset()
//	_ = f.StepBatch("p", "t") // back at step 0
func (f *FixtureRunner) Reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.step = 0
	f.genCount = 0
}
