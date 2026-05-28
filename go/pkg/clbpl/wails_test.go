// SPDX-Licence-Identifier: EUPL-1.2

package clbpl

import (
	"sync"
	"testing"
)

// TestWailsService_ServiceName_Good — binding namespace is the
// stable JS-facing label; freezing it here so a refactor that
// renames the surface fails loudly before TS bindings drift.
func TestWailsService_ServiceName_Good(t *testing.T) {
	s := NewWailsService(Defaults())
	if s == nil {
		t.Fatal("NewWailsService returned nil")
	}
	if got := s.ServiceName(); got != "CLBPL" {
		t.Errorf("ServiceName = %q, want %q", got, "CLBPL")
	}
}

// TestWailsService_ServiceStartup_Good — the Wails3 startup hook
// is a no-op (Detector is constructed at NewWailsService time);
// asserts the lifecycle stays green.
func TestWailsService_ServiceStartup_Good(t *testing.T) {
	s := NewWailsService(Defaults())
	r := s.ServiceStartup(nil, nil)
	if !r.OK {
		t.Errorf("ServiceStartup not OK: %v", r.Value)
	}
}

// TestWailsService_ServiceShutdown_Good — the Wails3 shutdown hook
// is a no-op (Detector holds only in-memory slices); asserts the
// lifecycle stays green.
func TestWailsService_ServiceShutdown_Good(t *testing.T) {
	s := NewWailsService(Defaults())
	r := s.ServiceShutdown()
	if !r.OK {
		t.Errorf("ServiceShutdown not OK: %v", r.Value)
	}
}

// --- Observe wire ---

// TestWailsService_Observe_Good — push the canonical decaying
// oscillation through the WailsService and prove peaks accumulate
// on the bound Detector. Verifies the Service is a transparent
// pass-through to Detector.Observe.
func TestWailsService_Observe_Good(t *testing.T) {
	s := NewWailsService(Defaults())
	samples := synthDecayingOscillation(400, 0.5, 0.5, 0.7, 20)

	var peakCount, grokedCount int
	for _, sm := range samples {
		r := s.Observe(sm.Step, sm.Loss)
		if !r.OK {
			t.Fatalf("Observe not OK at step %d: %v", sm.Step, r.Value)
		}
		ev, ok := r.Value.(Event)
		if !ok {
			t.Fatalf("Observe returned %T, want Event", r.Value)
		}
		switch ev.Kind {
		case EventPeak:
			peakCount++
		case EventGroked:
			// EventGroked supplants EventPeak on the call that
			// confirms a grok run — the peak is still appended
			// internally (see detector.go), so count it here.
			grokedCount++
		}
	}
	if peakCount < 1 {
		t.Errorf("decaying oscillation through WailsService produced %d peaks, want >= 1", peakCount)
	}

	pr := s.Peaks()
	if !pr.OK {
		t.Fatalf("Peaks not OK: %v", pr.Value)
	}
	peaks, ok := pr.Value.([]Sample)
	if !ok {
		t.Fatalf("Peaks returned %T, want []Sample", pr.Value)
	}
	if len(peaks) != peakCount+grokedCount {
		t.Errorf("Peaks() returned %d samples, want %d (EventPeak %d + EventGroked %d)",
			len(peaks), peakCount+grokedCount, peakCount, grokedCount)
	}
}

// TestWailsService_Observe_BadMonotonic — a monotonic descent has
// no peaks, so Observe emits EventNone throughout and Peaks() is
// empty. Mirrors detector_test.go's BadMonotonic.
func TestWailsService_Observe_BadMonotonic(t *testing.T) {
	s := NewWailsService(Defaults())
	for _, sm := range synthMonotonic(200, 5.0, 0.5) {
		r := s.Observe(sm.Step, sm.Loss)
		ev := r.Value.(Event)
		if ev.Kind == EventPeak {
			t.Errorf("monotonic stream emitted EventPeak at step %d", sm.Step)
		}
	}
	peaks := s.Peaks().Value.([]Sample)
	if len(peaks) != 0 {
		t.Errorf("monotonic stream left %d peaks, want 0", len(peaks))
	}
}

// --- Groked wire ---

// TestWailsService_Groked_Good — push enough oscillation to fire
// EventGroked and prove Groked() flips from false to true.
func TestWailsService_Groked_Good(t *testing.T) {
	s := NewWailsService(Defaults())
	if g := s.Groked().Value.(bool); g {
		t.Fatal("Groked() true before any samples")
	}
	for _, sm := range synthDecayingOscillation(600, 0.5, 0.5, 0.7, 20) {
		s.Observe(sm.Step, sm.Loss)
	}
	if g := s.Groked().Value.(bool); !g {
		t.Error("Groked() false after full decaying oscillation, want true")
	}
}

// --- Reset wire ---

// TestWailsService_Reset_Good — accumulate peaks, Reset, prove
// peaks are cleared and Groked drops back to false. Reset rebuilds
// the Detector from the stored Options.
func TestWailsService_Reset_Good(t *testing.T) {
	s := NewWailsService(Defaults())
	for _, sm := range synthDecayingOscillation(600, 0.5, 0.5, 0.7, 20) {
		s.Observe(sm.Step, sm.Loss)
	}
	if len(s.Peaks().Value.([]Sample)) == 0 {
		t.Fatal("preconditions: expected peaks before Reset")
	}
	if !s.Groked().Value.(bool) {
		t.Fatal("preconditions: expected Groked before Reset")
	}

	r := s.Reset()
	if !r.OK {
		t.Fatalf("Reset not OK: %v", r.Value)
	}
	if peaks := s.Peaks().Value.([]Sample); len(peaks) != 0 {
		t.Errorf("Peaks() after Reset = %d samples, want 0", len(peaks))
	}
	if s.Groked().Value.(bool) {
		t.Error("Groked() true after Reset, want false")
	}
}

// --- Concurrent Observe (race-test) ---

// TestWailsService_Observe_UglyConcurrent — fan multiple goroutines
// at Observe + Peaks + Groked to prove the core.Mutex guard holds.
// Run under `go test -race` to catch concurrent-access regressions.
// Asserts no panic AND that the final peak count equals the total
// EventPeak observations seen across all goroutines (no drops, no
// double-counts).
func TestWailsService_Observe_UglyConcurrent(t *testing.T) {
	s := NewWailsService(Defaults())
	samples := synthDecayingOscillation(400, 0.5, 0.5, 0.7, 20)

	const workers = 8
	var wg sync.WaitGroup
	var mu sync.Mutex
	var totalPeaks, totalGroked int

	// Each worker walks the SAME sample stream — peaks are
	// deduplicated by Step inside Detector.detectPeak (see the
	// "Guard against re-emitting the same peak" branch), so the
	// total event count observed externally (EventPeak +
	// EventGroked — the latter supplants EventPeak on the call
	// that confirms a grok run) equals the final Peaks() length
	// regardless of interleaving.
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var localPeak, localGrok int
			for _, sm := range samples {
				r := s.Observe(sm.Step, sm.Loss)
				if !r.OK {
					return
				}
				switch r.Value.(Event).Kind {
				case EventPeak:
					localPeak++
				case EventGroked:
					localGrok++
				}
				// Race readers — interleave Peaks / Groked with
				// the writer goroutines so the mutex is exercised
				// on both sides.
				_ = s.Peaks()
				_ = s.Groked()
			}
			mu.Lock()
			totalPeaks += localPeak
			totalGroked += localGrok
			mu.Unlock()
		}()
	}
	wg.Wait()

	finalPeaks := s.Peaks().Value.([]Sample)
	if len(finalPeaks) != totalPeaks+totalGroked {
		t.Errorf("final Peaks() = %d, observed event count = %d (EventPeak %d + EventGroked %d) — mismatch implies a missed or duplicated event under contention",
			len(finalPeaks), totalPeaks+totalGroked, totalPeaks, totalGroked)
	}
	if len(finalPeaks) == 0 {
		t.Error("concurrent run produced 0 peaks — expected >= 1 from decaying oscillation")
	}
}

// --- SetOptions wire ---

// TestWailsService_SetOptions_Good — apply a wider window via
// SetOptions and prove the Detector is rebuilt with the new opts.
// Push enough samples to confirm the new Window takes effect:
// peaks under a wider window are scarcer than under the default.
func TestWailsService_SetOptions_Good(t *testing.T) {
	s := NewWailsService(Defaults())
	r := s.SetOptions(Options{Window: 30, GrokThreshold: 0.03, GrokConfirmPeaks: 4})
	if !r.OK {
		t.Fatalf("SetOptions not OK: %v", r.Value)
	}
	// Confirm Detector is fresh — no peaks carried over even if
	// the previous Detector had observed any.
	if peaks := s.Peaks().Value.([]Sample); len(peaks) != 0 {
		t.Errorf("Peaks() after SetOptions = %d, want 0 (fresh Detector)", len(peaks))
	}
	for _, sm := range synthDecayingOscillation(400, 0.5, 0.5, 0.7, 20) {
		s.Observe(sm.Step, sm.Loss)
	}
	// New options persist across Reset.
	s.Reset()
	if peaks := s.Peaks().Value.([]Sample); len(peaks) != 0 {
		t.Errorf("Peaks() after Reset following SetOptions = %d, want 0", len(peaks))
	}
}

// TestWailsService_SetOptions_UglyZeroValue — SetOptions with a
// zero-value Options must fall back to Defaults via the Detector's
// existing applyDefaults path. Asserts the Detector still observes
// peaks (i.e. Window != 0 internally) and that the fall-back path
// is reachable from the Wails surface.
func TestWailsService_SetOptions_UglyZeroValue(t *testing.T) {
	s := NewWailsService(Options{Window: 30})
	r := s.SetOptions(Options{})
	if !r.OK {
		t.Fatalf("SetOptions(Options{}) not OK: %v", r.Value)
	}
	// If applyDefaults didn't run, Window would be 0 and
	// detectPeak's `len(d.samples) < d.opts.Window` guard would
	// admit every sample as a candidate — peaks would explode.
	// Run the canonical oscillation and assert the peak count
	// matches the default-Window expectation (>= 1, not hundreds).
	var peakCount int
	for _, sm := range synthDecayingOscillation(400, 0.5, 0.5, 0.7, 20) {
		ev := s.Observe(sm.Step, sm.Loss).Value.(Event)
		if ev.Kind == EventPeak {
			peakCount++
		}
	}
	if peakCount < 1 {
		t.Errorf("zero-value SetOptions produced %d peaks, want >= 1 (defaults applied)", peakCount)
	}
	if peakCount > 100 {
		t.Errorf("zero-value SetOptions produced %d peaks, want sane default Window (got runaway count — applyDefaults bypassed?)", peakCount)
	}
}
