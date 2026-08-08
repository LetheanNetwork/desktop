// SPDX-Licence-Identifier: EUPL-1.2

// Package clbpl implements the Cymatic-Linguistic Back-Propagation
// envelope detector — a small DSP layer that watches a training loss
// stream and surfaces the oscillation structure as discrete events.
//
// The detector turns Snider's eyeballed pattern (peaks of the loss
// oscillation shrink in amplitude after the first cycle, then
// collapse into a settled / "groked" state) into actionable signal
// for an autonomous curriculum loop:
//
//   - EventPeak    fires on each detected local maximum (a "breakout"
//     in the iteration N descent toward grok)
//   - EventGroked  fires once N consecutive peaks fall within a small
//     amplitude band — the model has internalised the
//     current iteration's target, ready to switch subject
//
// Stream usage:
//
//	d := clbpl.NewDetector(clbpl.Options{Window: 8})
//	for step, loss := range trainingLoop() {
//	    ev := d.Observe(step, loss)
//	    switch ev.Kind {
//	    case clbpl.EventPeak:
//	        // breakout at ev.Step, value ev.Value, peakIndex ev.PeakIndex
//	    case clbpl.EventGroked:
//	        // settle — model groked; next predicted breakout at ev.PredictedNextPeak
//	        curriculum.RotateSubject()
//	    }
//	}
//
// Offline usage (Snider's eyeballed analysis path — pass a captured
// loss series, get the same events without the streaming wrapper):
//
//	samples := loadLossSeries("training.jsonl")
//	peaks := clbpl.LocalMaxima(samples, 8)
//	if clbpl.IsGroked(peaks, 0.05, 3) {
//	    // model groked
//	}
package clbpl

// Sample is one observation in the loss stream — a (step, loss) pair.
// Steps are caller-defined units (training step, epoch, batch index);
// the detector treats them as opaque monotonic markers.
type Sample struct {
	Step int     `json:"step"`
	Loss float64 `json:"loss"`
}

// EventKind labels the type of event emitted by Detector.Observe.
type EventKind int

// EventNone is the absence of an event. Most Observe calls return
// EventNone — the detector accumulates samples until a peak or grok
// condition fires.
//
// EventPeak fires when a local maximum is confirmed within the rolling
// window. The "breakout" terminology from CL-BPL — each peak is one
// breakout in the descent toward grok.
//
// EventGroked fires once GrokConfirmPeaks consecutive peaks fall within
// GrokThreshold amplitude of each other. The signal to rotate subject
// in Phase A of the autocratic-cascade curriculum.
const (
	EventNone EventKind = iota
	EventPeak
	EventGroked
)

// String returns a short label for the event kind. Useful for logging
// and the Wails event envelope.
//
//	fmt.Println(clbpl.EventPeak.String()) // "peak"
func (k EventKind) String() string {
	switch k {
	case EventPeak:
		return "peak"
	case EventGroked:
		return "groked"
	default:
		return "none"
	}
}

// Event is the structured output of a single Detector.Observe call.
//
// Kind discriminates the event type. Step + Value name the sample that
// triggered the event. PeakIndex is the zero-based ordinal of the peak
// in the sequence of peaks observed so far (0 = first peak this
// detector saw). PredictedNextPeak is populated on EventGroked with
// the extrapolated step at which the next iteration's first breakout
// is expected — derived from the average cycle length of the observed
// peaks.
//
//	ev := d.Observe(step, loss)
//	if ev.Kind == clbpl.EventPeak {
//	    log.Printf("breakout #%d at step %d, value %f",
//	        ev.PeakIndex, ev.Step, ev.Value)
//	}
type Event struct {
	Kind              EventKind `json:"kind"`
	Step              int       `json:"step"`
	Value             float64   `json:"value"`
	PeakIndex         int       `json:"peak_index,omitempty"`
	PredictedNextPeak int       `json:"predicted_next_peak,omitempty"`
}

// Options configures the detector.
//
//   - Window is the rolling-window size used to detect local maxima.
//     Should be larger than the oscillation half-period. Default 8.
//
//   - GrokThreshold is the peak amplitude (max - min across recent
//     peaks) below which the detector considers the model groked,
//     relative to the loss scale of those peaks. Default 0.05.
//
//   - GrokConfirmPeaks is the number of consecutive peaks within the
//     threshold needed to fire EventGroked. Default 3 — avoids
//     false-positive groked on a single small peak.
//
//     opts := clbpl.Options{Window: 12, GrokThreshold: 0.03, GrokConfirmPeaks: 4}
type Options struct {
	Window           int     `json:"window,omitempty"`
	GrokThreshold    float64 `json:"grok_threshold,omitempty"`
	GrokConfirmPeaks int     `json:"grok_confirm_peaks,omitempty"`
}

// Defaults returns an Options populated with the canonical defaults.
// Used when callers pass a zero-value Options or omit fields.
//
//	opts := clbpl.Defaults() // Window=8, GrokThreshold=0.05, GrokConfirmPeaks=3
func Defaults() Options {
	return Options{
		Window:           8,
		GrokThreshold:    0.05,
		GrokConfirmPeaks: 3,
	}
}

// applyDefaults fills in zero-valued Options fields with Defaults().
// Used internally by the detector + helpers to accept partial Options.
func (o Options) applyDefaults() Options {
	d := Defaults()
	if o.Window <= 0 {
		o.Window = d.Window
	}
	if o.GrokThreshold <= 0 {
		o.GrokThreshold = d.GrokThreshold
	}
	if o.GrokConfirmPeaks <= 0 {
		o.GrokConfirmPeaks = d.GrokConfirmPeaks
	}
	return o
}
