// SPDX-Licence-Identifier: EUPL-1.2

package clbpl

import "math"

// LocalMaxima returns every sample that is a local maximum within a
// sliding window of the given size. A sample at index i is a local
// maximum when its loss is strictly greater than every other loss
// within [i-window/2, i+window/2] (clamped to the slice bounds).
//
// The first and last (window/2) samples can never qualify because
// their window is incomplete on one side — peak detection is causal
// in the centre of the series only.
//
// LocalMaxima is pure. Safe to call concurrently on shared input.
//
//	peaks := clbpl.LocalMaxima(samples, 8)
//	for _, p := range peaks { ... }
func LocalMaxima(samples []Sample, window int) []Sample {
	if window <= 0 {
		window = Defaults().Window
	}
	half := window / 2
	if half < 1 {
		half = 1
	}
	out := make([]Sample, 0, len(samples)/window+1)
	for i := half; i < len(samples)-half; i++ {
		if isLocalMax(samples, i, half) {
			out = append(out, samples[i])
		}
	}
	return out
}

// isLocalMax reports whether samples[i] is strictly greater than
// every sample in [i-half, i+half] except itself.
func isLocalMax(samples []Sample, i, half int) bool {
	v := samples[i].Loss
	if math.IsNaN(v) {
		return false
	}
	for j := i - half; j <= i+half; j++ {
		if j == i {
			continue
		}
		if !math.IsNaN(samples[j].Loss) && samples[j].Loss >= v {
			return false
		}
	}
	return true
}

// EnvelopeAmplitude returns the peak-to-peak amplitude over a slice
// of peaks — the maximum loss minus the minimum loss across the
// peaks. Returns 0 for fewer than two peaks (no amplitude to measure).
//
//	amp := clbpl.EnvelopeAmplitude(peaks[len(peaks)-5:])
//	if amp < 0.05 { /* near-flat — likely groked */ }
func EnvelopeAmplitude(peaks []Sample) float64 {
	if len(peaks) < 2 {
		return 0
	}
	lo := peaks[0].Loss
	hi := peaks[0].Loss
	for _, p := range peaks[1:] {
		if math.IsNaN(p.Loss) {
			continue
		}
		if p.Loss < lo {
			lo = p.Loss
		}
		if p.Loss > hi {
			hi = p.Loss
		}
	}
	return hi - lo
}

// IsGroked reports whether the last `confirm` peaks fall within
// `threshold` amplitude of each other. Returns false when there are
// fewer than `confirm` peaks (insufficient evidence — not groked yet).
//
// Threshold is in the same units as Loss. A threshold of 0.05 on
// cross-entropy loss means "the last N peaks differ by less than
// 0.05" — a near-collapse of the oscillation amplitude.
//
//	if clbpl.IsGroked(peaks, 0.05, 3) {
//	    // model has reached the current iteration's plateau
//	}
func IsGroked(peaks []Sample, threshold float64, confirm int) bool {
	if confirm <= 0 {
		confirm = Defaults().GrokConfirmPeaks
	}
	if threshold <= 0 {
		threshold = Defaults().GrokThreshold
	}
	if len(peaks) < confirm {
		return false
	}
	recent := peaks[len(peaks)-confirm:]
	return EnvelopeAmplitude(recent) < threshold
}

// PredictNextPeak extrapolates the step at which the next peak is
// expected, based on the average cycle length (step delta) of the
// observed peaks. Returns 0 when fewer than two peaks exist (no
// cycle observed yet).
//
// Used by the Detector to populate Event.PredictedNextPeak on
// EventGroked — once the model is groked on iter N, the next
// iteration's first breakout is expected around this step.
//
//	next := clbpl.PredictNextPeak(peaks)
//	if next > 0 { scheduleSubjectRotation(next) }
func PredictNextPeak(peaks []Sample) int {
	if len(peaks) < 2 {
		return 0
	}
	var sum, n float64
	for i := 1; i < len(peaks); i++ {
		sum += float64(peaks[i].Step - peaks[i-1].Step)
		n++
	}
	if n == 0 {
		return 0
	}
	avgInterval := sum / n
	return peaks[len(peaks)-1].Step + int(avgInterval+0.5)
}
