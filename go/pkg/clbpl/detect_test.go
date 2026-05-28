// SPDX-Licence-Identifier: EUPL-1.2

package clbpl

import (
	"math"
	"testing"
)

// synthDecayingOscillation generates a loss curve shaped like Snider's
// eyeballed CL-BPL pattern — a damped sinusoid descending to a
// settled value. Peak amplitudes shrink with each cycle; the curve
// is fully deterministic for stable tests.
//
//	loss(step) = base + amplitude * decay^cycle * cos(2π * step / period)
//
// where decay < 1 shrinks the amplitude with each cycle and base is
// the asymptote (the "groked" value).
func synthDecayingOscillation(steps int, base, amplitude, decay float64, period int) []Sample {
	out := make([]Sample, steps)
	for i := 0; i < steps; i++ {
		cycle := float64(i) / float64(period)
		shrink := math.Pow(decay, cycle)
		val := base + amplitude*shrink*math.Cos(2*math.Pi*float64(i)/float64(period))
		out[i] = Sample{Step: i, Loss: val}
	}
	return out
}

// synthMonotonic generates a monotonic descent — no peaks, no
// oscillation. Used as the negative-control input for peak detection.
func synthMonotonic(steps int, start, end float64) []Sample {
	out := make([]Sample, steps)
	for i := 0; i < steps; i++ {
		frac := float64(i) / float64(steps-1)
		out[i] = Sample{Step: i, Loss: start + (end-start)*frac}
	}
	return out
}

// --- LocalMaxima ---

func TestLocalMaxima_GoodOscillating(t *testing.T) {
	samples := synthDecayingOscillation(200, 0.5, 0.5, 0.7, 20)
	peaks := LocalMaxima(samples, 8)
	if len(peaks) < 5 {
		t.Errorf("decaying oscillation produced %d peaks, want >= 5", len(peaks))
	}
	// Peaks should be (roughly) one per period — 200/20 = 10 cycles,
	// expect around 9 detected peaks (boundary samples don't qualify).
	if len(peaks) > 15 {
		t.Errorf("decaying oscillation produced %d peaks, want <= 15", len(peaks))
	}
}

func TestLocalMaxima_BadMonotonic(t *testing.T) {
	samples := synthMonotonic(100, 5.0, 0.5)
	peaks := LocalMaxima(samples, 8)
	if len(peaks) != 0 {
		t.Errorf("monotonic descent produced %d peaks, want 0", len(peaks))
	}
}

func TestLocalMaxima_UglyEmpty(t *testing.T) {
	if peaks := LocalMaxima(nil, 8); len(peaks) != 0 {
		t.Errorf("nil samples produced %d peaks, want 0", len(peaks))
	}
	if peaks := LocalMaxima([]Sample{}, 8); len(peaks) != 0 {
		t.Errorf("empty samples produced %d peaks, want 0", len(peaks))
	}
}

func TestLocalMaxima_UglyTooShortForWindow(t *testing.T) {
	samples := synthDecayingOscillation(4, 0.5, 0.5, 0.7, 20)
	peaks := LocalMaxima(samples, 8)
	if len(peaks) != 0 {
		t.Errorf("4-sample input with window=8 produced %d peaks, want 0", len(peaks))
	}
}

func TestLocalMaxima_UglyNaN(t *testing.T) {
	samples := []Sample{
		{Step: 0, Loss: 1.0},
		{Step: 1, Loss: math.NaN()},
		{Step: 2, Loss: 2.0},
		{Step: 3, Loss: math.NaN()},
		{Step: 4, Loss: 1.5},
	}
	peaks := LocalMaxima(samples, 4)
	for _, p := range peaks {
		if math.IsNaN(p.Loss) {
			t.Errorf("NaN sample reported as peak: %+v", p)
		}
	}
}

// --- EnvelopeAmplitude ---

func TestEnvelopeAmplitude_Good(t *testing.T) {
	peaks := []Sample{
		{Step: 0, Loss: 1.0},
		{Step: 10, Loss: 0.8},
		{Step: 20, Loss: 0.6},
	}
	got := EnvelopeAmplitude(peaks)
	want := 0.4
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("EnvelopeAmplitude = %f, want %f", got, want)
	}
}

func TestEnvelopeAmplitude_UglyEmpty(t *testing.T) {
	if a := EnvelopeAmplitude(nil); a != 0 {
		t.Errorf("EnvelopeAmplitude(nil) = %f, want 0", a)
	}
	if a := EnvelopeAmplitude([]Sample{{Step: 0, Loss: 1.0}}); a != 0 {
		t.Errorf("EnvelopeAmplitude(single) = %f, want 0", a)
	}
}

// --- IsGroked ---

func TestIsGroked_GoodSettled(t *testing.T) {
	peaks := []Sample{
		{Step: 0, Loss: 1.0},
		{Step: 10, Loss: 0.7},
		{Step: 20, Loss: 0.42},
		{Step: 30, Loss: 0.43},
		{Step: 40, Loss: 0.42},
	}
	if !IsGroked(peaks, 0.05, 3) {
		t.Error("settled tail (last 3 peaks within 0.01) not detected as groked")
	}
}

func TestIsGroked_BadStillOscillating(t *testing.T) {
	peaks := []Sample{
		{Step: 0, Loss: 1.0},
		{Step: 10, Loss: 0.7},
		{Step: 20, Loss: 0.5},
	}
	if IsGroked(peaks, 0.05, 3) {
		t.Error("still-shrinking peaks reported as groked")
	}
}

func TestIsGroked_UglyTooFewPeaks(t *testing.T) {
	peaks := []Sample{{Step: 0, Loss: 1.0}}
	if IsGroked(peaks, 0.05, 3) {
		t.Error("single peak reported as groked")
	}
}

// --- PredictNextPeak ---

func TestPredictNextPeak_GoodConsistentInterval(t *testing.T) {
	peaks := []Sample{
		{Step: 100, Loss: 1.0},
		{Step: 200, Loss: 0.8},
		{Step: 300, Loss: 0.7},
	}
	want := 400
	got := PredictNextPeak(peaks)
	if got != want {
		t.Errorf("PredictNextPeak = %d, want %d", got, want)
	}
}

func TestPredictNextPeak_UglyTooFewPeaks(t *testing.T) {
	if got := PredictNextPeak([]Sample{{Step: 0, Loss: 1.0}}); got != 0 {
		t.Errorf("PredictNextPeak(single) = %d, want 0", got)
	}
	if got := PredictNextPeak(nil); got != 0 {
		t.Errorf("PredictNextPeak(nil) = %d, want 0", got)
	}
}
