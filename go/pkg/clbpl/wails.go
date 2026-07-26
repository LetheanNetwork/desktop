// SPDX-Licence-Identifier: EUPL-1.2

// Wails3 Service shape for the clbpl package — wraps the stateful
// Detector so the training UI can push (step, loss) samples in and
// poll Peaks / Groked / subscribed envelope events. Wails generates
// a TS binding at
// frontend-ng/bindings/dappco.re/lthn/desktop/pkg/clbpl/.
// Bound by application.NewService(clbpl.NewWailsService(...)) in
// pkg/desktop/desktop.go; the package-level Detector stays for
// non-WebView callers (offline analysis, CLI training drivers,
// in-process curriculum loops).

package clbpl

import core "dappco.re/go"

// WailsService is the WebView-facing handle on the clbpl envelope
// detector. Stateful — holds one Detector instance bound to the
// initial Options, guarded by a core.Mutex because Wails callers
// reach this from arbitrary goroutines (the frontend's Observe
// pushes interleave with UI reads of Peaks / Groked).
//
// The underlying Detector contract is single-loop-per-Detector
// (see detector.go) — that contract is preserved here by the mutex,
// which serialises every method against the same Detector instance.
//
// Reset rebuilds the Detector from the originally-supplied Options
// so the next training run starts clean without losing operator
// tuning. SetOptions replaces both the Detector AND the stored
// Options so operator-driven mid-session retuning persists across
// subsequent Resets.
//
// Tier-auth: deliberately ungated at this layer. CLBPL is
// non-secret operational telemetry — the detector sees only
// training-loop loss values and emits discrete envelope events.
// No user data, no key material, no LLM context. Mirrors the
// ungated posture of contentshield's WailsService.
//
// Usage example (boot wire):
//
//	application.NewService(clbpl.NewWailsService(clbpl.Defaults()))
type WailsService struct {
	mu       core.Mutex
	opts     Options
	detector *Detector
}

// NewWailsService constructs the WailsService with an initial
// Detector built from the supplied Options. Zero-valued Options
// fields fall back to Defaults() via the Detector's existing
// applyDefaults path.
//
// Usage example:
//
//	application.NewService(clbpl.NewWailsService(clbpl.Defaults()))
//	application.NewService(clbpl.NewWailsService(clbpl.Options{Window: 12}))
func NewWailsService(opts Options) *WailsService {
	return &WailsService{
		opts:     opts,
		detector: NewDetector(opts),
	}
}

// ServiceName labels the binding namespace exposed to JS as
// "CLBPL" — Wails generates the TS binding under
// frontend-ng/bindings/dappco.re/lthn/desktop/pkg/clbpl/.
func (s *WailsService) ServiceName() string { return "CLBPL" }

// ServiceStartup is the Wails3 lifecycle hook called once after the
// WebView attaches. The Detector is already constructed at
// NewWailsService time, so this is a no-op.
func (s *WailsService) ServiceStartup(_ core.Context, _ any) core.Result {
	return core.Ok(nil)
}

// ServiceShutdown is the Wails3 lifecycle hook called once at app
// shutdown. The Detector holds only in-memory sample / peak slices;
// nothing to release.
func (s *WailsService) ServiceShutdown() core.Result { return core.Ok(nil) }

// Observe pushes the next (step, loss) sample into the bound
// Detector and returns the Event fired by this observation —
// Event{Kind: EventNone} when no event fires (the common case
// during normal oscillation).
//
// Usage example (TS):
//
//	import { Observe } from "@desktop/clbpl/clbpl";
//	const ev = await Observe(step, loss);
//	if (ev.kind === "peak") { /* surface breakout glyph */ }
func (s *WailsService) Observe(step int, loss float64) core.Result {
	s.mu.Lock()
	defer s.mu.Unlock()
	return core.Ok(s.detector.Observe(step, loss))
}

// Peaks returns a copy of every peak observed so far. Useful for
// the UI to render the running envelope at any point during the
// training pass.
//
// Usage example (TS):
//
//	import { Peaks } from "@desktop/clbpl/clbpl";
//	const peaks = await Peaks();
//	renderEnvelope(peaks);
func (s *WailsService) Peaks() core.Result {
	s.mu.Lock()
	defer s.mu.Unlock()
	return core.Ok(s.detector.Peaks())
}

// Groked reports whether EventGroked has already fired on the
// bound Detector. Useful for the UI to render the terminal state
// without having to subscribe to every event.
//
// Usage example (TS):
//
//	import { Groked } from "@desktop/clbpl/clbpl";
//	if (await Groked()) { /* show settled glyph */ }
func (s *WailsService) Groked() core.Result {
	s.mu.Lock()
	defer s.mu.Unlock()
	return core.Ok(s.detector.Groked())
}

// Reset rebuilds the Detector from the originally-supplied Options
// so the next training run starts clean. Operator tuning applied
// via SetOptions persists across Reset (Reset uses the stored
// Options, not the constructor's snapshot).
//
// Usage example (TS):
//
//	import { Reset } from "@desktop/clbpl/clbpl";
//	await Reset(); // ready for the next training pass
func (s *WailsService) Reset() core.Result {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.detector = NewDetector(s.opts)
	return core.Ok(nil)
}

// SetOptions replaces the Detector with a fresh instance built
// from new Options — used by the operator to tune the window /
// threshold / confirm-peaks mid-session. Zero-valued fields fall
// back to Defaults via the Detector's existing applyDefaults path.
//
// SetOptions discards any peaks / samples accumulated by the
// previous Detector; callers that want to keep the running envelope
// across a tuning change should snapshot Peaks() first.
//
// Usage example (TS):
//
//	import { SetOptions } from "@desktop/clbpl/clbpl";
//	await SetOptions({ window: 12, grok_threshold: 0.03 });
func (s *WailsService) SetOptions(opts Options) core.Result {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.opts = opts
	s.detector = NewDetector(opts)
	return core.Ok(nil)
}
