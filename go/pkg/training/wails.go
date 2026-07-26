// SPDX-Licence-Identifier: EUPL-1.2

// Wails3 Service shape for the training package — wraps the stateful
// *training.Service so the operator UI can drive the autocratic-cascade
// Phase A rotation, observe live Status, and stop a rotation mid-flight.
// Wails generates a TS binding at
// frontend-ng/bindings/dappco.re/lthn/desktop/pkg/training/.
// Bound by application.NewService(training.NewWailsService(...)) in
// pkg/desktop/desktop.go; the underlying Service stays for non-Wails
// callers (CLI training drivers, in-process curriculum loops).

package training

import (
	core "dappco.re/go"
)

// WailsService is the WebView-facing handle on a *training.Service.
// Stateful — holds a single cancel-closure + running flag guarded by
// core.Mutex because Wails callers reach this from arbitrary goroutines
// (operator clicks Start in one render frame, Stop in the next, polls
// Status across both).
//
// The underlying Service already serialises Run via its own runMu, so
// a second StartFixture call lands cleanly on the existing
// "rotation already in progress" Fail — this surface adds a local
// running-flag mirror so IsRunning / ListGroked can answer without
// reaching into the Service state on every poll.
//
// Tier-auth: deliberately ungated at this layer. Training is
// operator-driven from the desktop's own Settings surface, not a
// user-facing capability — same posture as openaibench.WailsService
// (which gates underneath via pkg/auth + audit emits). The training
// surface produces no key material and no user-data exposure; the R₁
// records it writes are training-loop artefacts the operator opted in
// to by clicking Start. Wrapping a redundant tier gate here would
// double-tax every operator action without adding security.
//
// Usage example (boot wire):
//
//	svc := training.NewService(c, training.Options{})
//	_ = svc.Register(c)
//	wsvc := training.NewWailsService(c, svc)
//	// Append to wailsServices: application.NewService(wsvc)
type WailsService struct {
	core *core.Core
	svc  *Service

	mu      core.Mutex
	cancel  func()
	running bool
}

// NewWailsService constructs the WailsService. Both arguments are
// required — nil core or nil service surfaces a clear runtime failure
// (Status / StartFixture / Stop short-circuit to Fail) rather than
// silent partial functionality. Mirrors openaibench.NewWailsService.
//
// Usage example:
//
//	wsvc := training.NewWailsService(c, trainingSvc)
func NewWailsService(c *core.Core, svc *Service) *WailsService {
	return &WailsService{core: c, svc: svc}
}

// ServiceName labels the binding namespace exposed to JS as
// "Training" — Wails generates the TS binding under
// frontend-ng/bindings/dappco.re/lthn/desktop/pkg/training/.
func (w *WailsService) ServiceName() string { return "Training" }

// ServiceStartup is the Wails3 lifecycle hook called once after the
// WebView attaches. The underlying *training.Service is already
// constructed + Registered at boot, so this is a no-op.
func (w *WailsService) ServiceStartup(_ core.Context, _ any) core.Result {
	return core.Ok(nil)
}

// ServiceShutdown is the Wails3 lifecycle hook called once at app
// shutdown. Any in-flight rotation is cancelled cooperatively via the
// stashed cancel closure so R₁ writes drain cleanly per the
// Service.Run cancellation contract.
//
// Usage example (boot wire — automatic via application.NewService).
func (w *WailsService) ServiceShutdown() core.Result {
	w.mu.Lock()
	cancel := w.cancel
	w.cancel = nil
	w.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return core.Ok(nil)
}

// Status delegates to the wrapped *training.Service. Returns the live
// rotation snapshot (Running flag, ActiveSubject / ActiveProbe, step
// counters, groked-subjects list, completed-runs count).
//
// Usage example (TS):
//
//	import { Status } from "@desktop/training/trainingwailsservice";
//	const r = await Status();
//	if (r.OK) { renderStatus(r.Value); }
func (w *WailsService) Status() core.Result {
	if w.svc == nil {
		return core.Fail(core.E("training.Status", "service not wired", nil))
	}
	return w.svc.Status()
}

// StartFixture begins a rotation against an inline FixtureRunner —
// the operator-UI smoke path used before the real gomlxrunner adapter
// lands. Returns Fail immediately when a rotation is already in
// progress; the Run itself runs on a goroutine via core.Go so the
// Wails call returns without blocking the WebView.
//
// The cancel closure paired with the rotation's context is stashed in
// the WailsService so Stop can cancel mid-flight. When the goroutine
// returns (rotation completed or cancelled), running + cancel are
// cleared under the mutex so a subsequent StartFixture can land.
//
// Usage example (TS):
//
//	import { StartFixture } from "@desktop/training/trainingwailsservice";
//	const r = await StartFixture("fixture-e2b", "CONT", 0);
//	if (!r.OK) { showError(r.Error); }
func (w *WailsService) StartFixture(model, substrate string, tier int) core.Result {
	if w.svc == nil {
		return core.Fail(core.E("training.StartFixture", "service not wired", nil))
	}
	if w.core == nil {
		return core.Fail(core.E("training.StartFixture", "core not wired", nil))
	}

	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		return core.Fail(core.E("training.StartFixture", "rotation already in progress", nil))
	}
	ctx, cancel := core.WithCancel(w.core.Context())
	w.cancel = cancel
	w.running = true
	w.mu.Unlock()

	runner := NewFixtureRunner(FixtureConfig{
		Model:     model,
		Substrate: substrate,
		Tier:      tier,
	})

	w.core.Go(func() {
		defer func() {
			w.mu.Lock()
			w.running = false
			if w.cancel != nil {
				// Release the context resources even on natural
				// completion — context.CancelFunc is idempotent.
				w.cancel()
				w.cancel = nil
			}
			w.mu.Unlock()
		}()
		_ = w.svc.Run(ctx, runner)
	})

	return core.Ok(nil)
}

// Stop cancels the current rotation via the stashed cancel closure.
// Idempotent — returns Ok with a one-line summary even when no Run is
// active so the operator UI can wire a Stop button without checking
// IsRunning first. Status will report Cancelled:true on the next poll
// once the in-flight Run drains.
//
// Usage example (TS):
//
//	import { Stop } from "@desktop/training/trainingwailsservice";
//	await Stop();
func (w *WailsService) Stop() core.Result {
	w.mu.Lock()
	cancel := w.cancel
	w.cancel = nil
	wasRunning := w.running
	w.mu.Unlock()

	if cancel == nil {
		return core.Ok("no rotation in progress")
	}
	cancel()
	if wasRunning {
		return core.Ok("rotation cancellation requested")
	}
	return core.Ok("no rotation in progress")
}

// IsRunning reports whether a rotation is currently active on this
// WailsService. Returns core.Ok(bool) so the TS binding sees a typed
// result rather than the bare value.
//
// Usage example (TS):
//
//	import { IsRunning } from "@desktop/training/trainingwailsservice";
//	const r = await IsRunning();
//	if (r.OK && r.Value) { showRunningBadge(); }
func (w *WailsService) IsRunning() core.Result {
	w.mu.Lock()
	defer w.mu.Unlock()
	return core.Ok(w.running)
}

// ListGroked returns the groked-subjects slice from the underlying
// Service's current Status. Empty before any Run completes a probe;
// populated incrementally as CL-BPL fires EventGroked for each
// subject. Returns core.Ok([]string) so the TS binding sees a typed
// array.
//
// Usage example (TS):
//
//	import { ListGroked } from "@desktop/training/trainingwailsservice";
//	const r = await ListGroked();
//	if (r.OK) { renderGrokedChips(r.Value); }
func (w *WailsService) ListGroked() core.Result {
	if w.svc == nil {
		return core.Fail(core.E("training.ListGroked", "service not wired", nil))
	}
	st := w.svc.Status()
	if !st.OK {
		return st
	}
	snap, ok := st.Value.(Status)
	if !ok {
		return core.Fail(core.E("training.ListGroked", "Status returned non-Status value", nil))
	}
	out := make([]string, len(snap.GrokedSubjects))
	copy(out, snap.GrokedSubjects)
	return core.Ok(out)
}

// HasCheckpoint reports whether a persisted checkpoint exists for
// the given model. Returns core.Ok(true) when a resumable rotation
// is on disk, core.Ok(false) when no checkpoint matches. Drives the
// "Resume" affordance in the operator training-window UI — hide the
// button when this returns false.
//
// Usage example (TS):
//
//	import { HasCheckpoint } from "@desktop/training/trainingwailsservice";
//	const r = await HasCheckpoint("gemma4-e2b-it-q4");
//	if (r.OK && r.Value) { showResumeButton(); }
func (w *WailsService) HasCheckpoint(model string) core.Result {
	r := LoadCheckpoint(model)
	if !r.OK {
		return r
	}
	cp, ok := r.Value.(*Checkpoint)
	return core.Ok(ok && cp != nil)
}

// GetCheckpoint returns the persisted Checkpoint for the given model
// or core.Ok(nil) when no checkpoint exists. The frontend uses this
// to populate the resume UX (rotation started at X, N probes
// completed, M subjects groked) before the operator commits to a
// resume.
//
// Usage example (TS):
//
//	import { GetCheckpoint } from "@desktop/training/trainingwailsservice";
//	const r = await GetCheckpoint("gemma4-e2b-it-q4");
//	if (r.OK && r.Value) { renderCheckpointPreview(r.Value); }
func (w *WailsService) GetCheckpoint(model string) core.Result {
	return LoadCheckpoint(model)
}

// DiscardCheckpoint removes the persisted checkpoint for the given
// model without starting a new rotation. Operator-facing "Start over"
// flow — the operator inspects a checkpoint, decides not to resume,
// and wants the file gone so the next training run starts fresh.
//
// Idempotent — missing checkpoint is not an error.
//
// Usage example (TS):
//
//	import { DiscardCheckpoint } from "@desktop/training/trainingwailsservice";
//	await DiscardCheckpoint("gemma4-e2b-it-q4");
func (w *WailsService) DiscardCheckpoint(model string) core.Result {
	return ClearCheckpoint(model)
}

// ResumeFixture begins a rotation against an inline FixtureRunner
// using a previously-persisted checkpoint for the given model. Same
// goroutine + cancel-stash shape as StartFixture so Stop cancels
// cleanly mid-resume.
//
// Loads the checkpoint via LoadCheckpoint, validates it covers the
// requested model, then dispatches Service.Resume with a freshly
// constructed FixtureRunner. Returns Fail when no checkpoint exists
// or when a rotation is already in progress; the second caller does
// NOT block — fail-fast matches StartFixture's posture.
//
// substrate + tier override the checkpoint's stamped values so the
// operator can resume against a different substrate (e.g. test the
// substrate-shift hypothesis by re-running TRAD as CONT or vice
// versa). The checkpoint's Model is honoured as-is — resuming
// against a different model is meaningless (it'd target a different
// runner entirely).
//
// Usage example (TS):
//
//	import { ResumeFixture } from "@desktop/training/trainingwailsservice";
//	const r = await ResumeFixture("gemma4-e2b-it-q4", "CONT", 0);
//	if (!r.OK) { showError(r.Error); }
func (w *WailsService) ResumeFixture(model, substrate string, tier int) core.Result {
	if w.svc == nil {
		return core.Fail(core.E("training.ResumeFixture", "service not wired", nil))
	}
	if w.core == nil {
		return core.Fail(core.E("training.ResumeFixture", "core not wired", nil))
	}
	if model == "" {
		return core.Fail(core.E("training.ResumeFixture", "model is required", nil))
	}

	loadR := LoadCheckpoint(model)
	if !loadR.OK {
		return loadR
	}
	cp, _ := loadR.Value.(*Checkpoint)
	if cp == nil {
		return core.Fail(core.E("training.ResumeFixture",
			"no checkpoint found for "+model, nil))
	}

	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		return core.Fail(core.E("training.ResumeFixture", "rotation already in progress", nil))
	}
	ctx, cancel := core.WithCancel(w.core.Context())
	w.cancel = cancel
	w.running = true
	w.mu.Unlock()

	runner := NewFixtureRunner(FixtureConfig{
		Model:     model,
		Substrate: substrate,
		Tier:      tier,
	})

	w.core.Go(func() {
		defer func() {
			w.mu.Lock()
			w.running = false
			if w.cancel != nil {
				w.cancel()
				w.cancel = nil
			}
			w.mu.Unlock()
		}()
		_ = w.svc.Resume(ctx, runner, cp)
	})

	return core.Ok(nil)
}
