// SPDX-Licence-Identifier: EUPL-1.2

package clbpl

// Detector wraps the stateless helpers in detect.go as a streaming
// envelope tracker. Single-loop-per-Detector contract — not
// concurrent-safe. Callers sharing a Detector across goroutines must
// provide their own synchronisation; one Detector per training loop
// is the canonical pattern.
//
// Buffer growth is bounded by the rolling window plus the running
// peak list. Both grow O(1) per observation in steady state — only
// peaks accumulate and peak count grows logarithmically with sample
// count (one peak per oscillation half-period).
//
// Streaming usage:
//
//	d := clbpl.NewDetector(clbpl.Defaults())
//	for step, loss := range trainingLoop() {
//	    ev := d.Observe(step, loss)
//	    if ev.Kind != clbpl.EventNone {
//	        emit(ev)
//	    }
//	}
type Detector struct {
	opts        Options
	samples     []Sample // rolling window of recent samples (capped to opts.Window+1)
	peaks       []Sample // every confirmed peak, in order
	grokedFired bool     // true once EventGroked has fired (suppresses repeats)
}

// NewDetector constructs a Detector with the given Options. Zero-valued
// fields fall back to Defaults().
//
//	d := clbpl.NewDetector(clbpl.Options{})              // canonical defaults
//	d := clbpl.NewDetector(clbpl.Options{Window: 12})    // wider peak window
//	d := clbpl.NewDetector(clbpl.Options{                 // strict grok
//	    GrokThreshold:    0.02,
//	    GrokConfirmPeaks: 4,
//	})
func NewDetector(opts Options) *Detector {
	return &Detector{
		opts:    opts.applyDefaults(),
		samples: make([]Sample, 0, opts.applyDefaults().Window+1),
	}
}

// Observe accepts the next (step, loss) sample and returns the event
// fired by this observation. Returns Event{Kind: EventNone} when no
// event fires (the common case during normal oscillation).
//
// Steps must be monotonically increasing — out-of-order samples are
// accepted but produce undefined peak ordering. Loss may be any
// finite float64; NaN samples are accepted and stored but never
// qualify as peaks.
//
// Event order on a given Observe call: at most one event per call.
// EventGroked fires at most once per Detector instance (subsequent
// peaks within threshold do not re-fire).
//
//	ev := d.Observe(step, loss)
//	switch ev.Kind {
//	case clbpl.EventPeak:    onBreakout(ev)
//	case clbpl.EventGroked:  onGroked(ev)
//	}
func (d *Detector) Observe(step int, loss float64) Event {
	d.samples = append(d.samples, Sample{Step: step, Loss: loss})

	// Cap the rolling buffer at Window+1 — we only need enough to
	// confirm a peak at the centre of the window. Discard older
	// samples once the buffer exceeds the window size.
	if len(d.samples) > d.opts.Window+1 {
		d.samples = d.samples[len(d.samples)-(d.opts.Window+1):]
	}

	// Peak detection runs against the rolling window. The candidate
	// position is the centre of the window — the most recent sample
	// for which we have full neighbour visibility on both sides.
	if peak, ok := d.detectPeak(); ok {
		d.peaks = append(d.peaks, peak)
		ev := Event{
			Kind:      EventPeak,
			Step:      peak.Step,
			Value:     peak.Loss,
			PeakIndex: len(d.peaks) - 1,
		}
		// Check if this newly-confirmed peak completes a grok run.
		if !d.grokedFired && IsGroked(d.peaks, d.opts.GrokThreshold, d.opts.GrokConfirmPeaks) {
			d.grokedFired = true
			return Event{
				Kind:              EventGroked,
				Step:              peak.Step,
				Value:             peak.Loss,
				PeakIndex:         len(d.peaks) - 1,
				PredictedNextPeak: PredictNextPeak(d.peaks),
			}
		}
		return ev
	}
	return Event{Kind: EventNone}
}

// Peaks returns a copy of every peak observed so far. Useful for
// offline analysis at the end of a training pass.
//
//	for _, p := range d.Peaks() { ... }
func (d *Detector) Peaks() []Sample {
	out := make([]Sample, len(d.peaks))
	copy(out, d.peaks)
	return out
}

// Groked reports whether EventGroked has already fired on this
// Detector. Useful for callers that observe events asynchronously and
// want to query the terminal state.
func (d *Detector) Groked() bool { return d.grokedFired }

// detectPeak runs LocalMaxima against the rolling buffer and returns
// the sample at the buffer's centre if it qualifies. Returns (zero,
// false) when the centre is not a peak (or the buffer is not yet
// full).
func (d *Detector) detectPeak() (Sample, bool) {
	if len(d.samples) < d.opts.Window {
		return Sample{}, false
	}
	half := d.opts.Window / 2
	if half < 1 {
		half = 1
	}
	centre := len(d.samples) - 1 - half
	if centre < half {
		return Sample{}, false
	}
	if !isLocalMax(d.samples, centre, half) {
		return Sample{}, false
	}
	// Guard against re-emitting the same peak when multiple calls hit
	// the same centre — peaks are unique by Step.
	c := d.samples[centre]
	if n := len(d.peaks); n > 0 && d.peaks[n-1].Step == c.Step {
		return Sample{}, false
	}
	return c, true
}
