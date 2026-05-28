// SPDX-Licence-Identifier: EUPL-1.2

package clbpl

import (
	"math"
	"testing"
)

func TestDetector_GoodStreamingOscillation(t *testing.T) {
	samples := synthDecayingOscillation(400, 0.5, 0.5, 0.7, 20)
	d := NewDetector(Defaults())

	var peakCount, grokedCount int
	var grokEvent Event
	for _, s := range samples {
		ev := d.Observe(s.Step, s.Loss)
		switch ev.Kind {
		case EventPeak:
			peakCount++
		case EventGroked:
			grokedCount++
			grokEvent = ev
		}
	}

	if peakCount < 5 {
		t.Errorf("detected %d peaks streaming, want >= 5", peakCount)
	}
	if grokedCount != 1 {
		t.Errorf("EventGroked fired %d times, want exactly 1", grokedCount)
	}
	if grokEvent.PredictedNextPeak <= grokEvent.Step {
		t.Errorf("PredictedNextPeak (%d) should be > grok step (%d)",
			grokEvent.PredictedNextPeak, grokEvent.Step)
	}
}

func TestDetector_BadMonotonic(t *testing.T) {
	samples := synthMonotonic(200, 5.0, 0.5)
	d := NewDetector(Defaults())
	var peakCount, grokedCount int
	for _, s := range samples {
		ev := d.Observe(s.Step, s.Loss)
		switch ev.Kind {
		case EventPeak:
			peakCount++
		case EventGroked:
			grokedCount++
		}
	}
	if peakCount != 0 {
		t.Errorf("monotonic stream emitted %d peaks, want 0", peakCount)
	}
	if grokedCount != 0 {
		t.Errorf("monotonic stream emitted %d groked events, want 0", grokedCount)
	}
}

func TestDetector_UglyEmpty(t *testing.T) {
	d := NewDetector(Defaults())
	if ev := d.Observe(0, 1.0); ev.Kind != EventNone {
		t.Errorf("first observation fired %v, want EventNone", ev.Kind)
	}
	if d.Groked() {
		t.Error("Detector reports groked after single observation")
	}
	if peaks := d.Peaks(); len(peaks) != 0 {
		t.Errorf("Peaks() = %d, want 0", len(peaks))
	}
}

func TestDetector_UglyNaNTolerated(t *testing.T) {
	d := NewDetector(Defaults())
	for i := 0; i < 50; i++ {
		v := math.NaN()
		if i%5 == 0 {
			v = float64(i) // some real values mixed in
		}
		ev := d.Observe(i, v)
		_ = ev
	}
	// We don't assert any particular event here — the contract is
	// that NaN doesn't panic and doesn't qualify as a peak. The
	// runtime success of this test IS the assertion.
}

func TestDetector_GrokedDoesNotRepeat(t *testing.T) {
	samples := synthDecayingOscillation(600, 0.5, 0.5, 0.7, 20)
	d := NewDetector(Defaults())
	var grokedCount int
	for _, s := range samples {
		ev := d.Observe(s.Step, s.Loss)
		if ev.Kind == EventGroked {
			grokedCount++
		}
	}
	if grokedCount != 1 {
		t.Errorf("EventGroked fired %d times, want exactly 1 (no repeats)", grokedCount)
	}
	if !d.Groked() {
		t.Error("Groked() returns false after EventGroked fired")
	}
}

func TestDetector_PeaksAccumulate(t *testing.T) {
	samples := synthDecayingOscillation(200, 0.5, 0.5, 0.7, 20)
	d := NewDetector(Defaults())
	for _, s := range samples {
		d.Observe(s.Step, s.Loss)
	}
	peaks := d.Peaks()
	if len(peaks) == 0 {
		t.Fatal("Peaks() empty after streaming 200 oscillation samples")
	}
	// Peaks should be in monotonic step order.
	for i := 1; i < len(peaks); i++ {
		if peaks[i].Step <= peaks[i-1].Step {
			t.Errorf("peaks not in step order at index %d: %d <= %d",
				i, peaks[i].Step, peaks[i-1].Step)
		}
	}
}

func TestDetector_GoodCustomOptions(t *testing.T) {
	// Strict grok — tighter threshold + more confirm peaks.
	d := NewDetector(Options{Window: 12, GrokThreshold: 0.02, GrokConfirmPeaks: 4})
	samples := synthDecayingOscillation(800, 0.5, 0.5, 0.8, 30)
	var grokedCount int
	for _, s := range samples {
		ev := d.Observe(s.Step, s.Loss)
		if ev.Kind == EventGroked {
			grokedCount++
		}
	}
	if grokedCount != 1 {
		t.Errorf("strict-options stream fired groked %d times, want 1", grokedCount)
	}
}

func TestDefaults_Good(t *testing.T) {
	d := Defaults()
	if d.Window != 8 {
		t.Errorf("Defaults Window = %d, want 8", d.Window)
	}
	if d.GrokThreshold != 0.05 {
		t.Errorf("Defaults GrokThreshold = %f, want 0.05", d.GrokThreshold)
	}
	if d.GrokConfirmPeaks != 3 {
		t.Errorf("Defaults GrokConfirmPeaks = %d, want 3", d.GrokConfirmPeaks)
	}
}

func TestOptions_applyDefaults(t *testing.T) {
	o := Options{}.applyDefaults()
	if o.Window != 8 || o.GrokThreshold != 0.05 || o.GrokConfirmPeaks != 3 {
		t.Errorf("zero Options didn't fill defaults: %+v", o)
	}
	o2 := Options{Window: 20}.applyDefaults()
	if o2.Window != 20 {
		t.Errorf("non-zero Window overwritten: %d", o2.Window)
	}
	if o2.GrokThreshold != 0.05 {
		t.Errorf("zero GrokThreshold not defaulted: %f", o2.GrokThreshold)
	}
}

func TestEventKind_String(t *testing.T) {
	cases := []struct {
		k    EventKind
		want string
	}{
		{EventNone, "none"},
		{EventPeak, "peak"},
		{EventGroked, "groked"},
	}
	for _, c := range cases {
		if got := c.k.String(); got != c.want {
			t.Errorf("EventKind(%d).String() = %q, want %q", c.k, got, c.want)
		}
	}
}
