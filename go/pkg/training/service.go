// SPDX-Licence-Identifier: EUPL-1.2

package training

import (
	core "dappco.re/go"

	"dappco.re/lthn/desktop/pkg/clbpl"
	"dappco.re/lthn/desktop/pkg/contentshield"
	"dappco.re/lthn/desktop/pkg/r1"
	"dappco.re/lthn/desktop/pkg/sandwich"
	"dappco.re/lthn/desktop/pkg/seeds"
)

// divergenceWindowSize is the count of strictly-increasing loss steps
// required to trip the monotonic-rise guard. 50 is wide enough that
// natural loss oscillation never accidentally fires it, narrow enough
// that a real divergence (learning-rate too high, overflow) is caught
// in seconds rather than thousands of wasted steps.
const divergenceWindowSize = 50

// Service drives the autocratic-cascade Phase A rotation against a
// Runner.
//
// One Service instance owns one rotation lane — Run is guarded by a
// core.Mutex so a second concurrent Run on the same Service fails fast
// rather than scrambling the CL-BPL detector state mid-stream. Two
// independent Services can rotate against two independent Runners
// (e.g. parallel E2B + E4B lanes) without contention.
//
// Lifecycle: NewService(c, opts) → Register(c) → Run(ctx, runner).
// Register wires Core actions so non-Wails callers can query status
// through the action bus. Run blocks until the rotation finishes or
// ctx is cancelled.
//
//	svc := training.NewService(c, training.Options{
//	    Subjects: []string{"english"}, ProbesPerSubject: 3,
//	})
//	_ = svc.Register(c)
//	r := svc.Run(c.Context(), training.NewFixtureRunner(training.FixtureConfig{}))
type Service struct {
	core *core.Core
	opts Options

	// runMu serialises Run calls — only one rotation per Service.
	runMu core.Mutex

	// stateMu protects the live status snapshot.
	stateMu        core.Mutex
	running        bool
	activeSubject  string
	activeProbe    string
	currentStep    int
	currentPeaks   int
	grokedSubjects []string
	completedRuns  int
	divergence     *DivergenceInfo

	// startedAt is the Unix-seconds timestamp of the current Run
	// (stamped in beginRun). Copied into every Checkpoint persisted
	// during this Run so resume-side UX can show "rotation started at
	// X" rather than just the latest save timestamp.
	startedAt int64

	// completedProbes is the set of (subject, probe_id) pairs whose
	// epoch-2 loop has terminated this Run. Appended in runProbe on
	// every exit path (groked / cap / runner-error / ctx-cancel),
	// snapshotted into every persisted Checkpoint. Reset in beginRun.
	completedProbes []ProbeKey
}

// NewService constructs a Service. The Core handle is stamped for
// action registration and event broadcast; callers must still invoke
// Register(c) to wire the action bus.
//
//	svc := training.NewService(c, training.Options{})
func NewService(c *core.Core, opts Options) *Service {
	if len(opts.Subjects) == 0 {
		opts.Subjects = append([]string(nil), canonicalSubjects...)
	}
	if opts.Epoch2MaxSteps <= 0 {
		opts.Epoch2MaxSteps = defaultEpoch2MaxSteps
	}
	return &Service{core: c, opts: opts}
}

// Register wires the Service onto the Core action bus per the
// canonical Mantis #1336 pattern.
//
// Actions registered:
//
//   - "training.status" — returns the current Status snapshot.
//
// Run is NOT exposed as a Core action — it requires a Runner instance
// that the action bus cannot reasonably marshal. Callers wire Run
// through pkg/desktop boot (the Cladius pass) or via direct method
// call from tests.
//
//	if r := svc.Register(c); !r.OK { return r }
func (s *Service) Register(c *core.Core) core.Result {
	if c == nil {
		return core.Fail(core.E("training.Register", "core is nil", nil))
	}
	s.core = c
	c.Action("training.status", func(_ core.Context, _ core.Options) core.Result {
		return s.Status()
	})
	return core.Ok(nil)
}

// Status returns the current rotation state. Safe to call concurrently
// with Run; the snapshot is consistent within the call (mutex-guarded
// copy).
//
//	st := svc.Status().Value.(training.Status)
func (s *Service) Status() core.Result {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	groked := make([]string, len(s.grokedSubjects))
	copy(groked, s.grokedSubjects)
	var div *DivergenceInfo
	if s.divergence != nil {
		// Copy so callers cannot mutate Service state through the
		// returned pointer.
		cp := *s.divergence
		div = &cp
	}
	return core.Ok(Status{
		Running:        s.running,
		ActiveSubject:  s.activeSubject,
		ActiveProbe:    s.activeProbe,
		Step:           s.currentStep,
		PeakCount:      s.currentPeaks,
		GrokedSubjects: groked,
		CompletedRuns:  s.completedRuns,
		Divergence:     div,
	})
}

// Run drives a full Phase A rotation against the provided Runner.
//
// Blocks until every subject in Options.Subjects either groks (CL-BPL
// EventGroked fires) or exhausts its probe budget. Ctx cancellation is
// honoured at every loop boundary — every captured R₁ before the
// cancel point has already been persisted, so cancellation is a clean
// shutdown rather than a lost-work hazard.
//
// Concurrent calls on the same Service return Fail immediately
// ("rotation already in progress"); the second caller does NOT block
// waiting for the first.
//
// Run starts a fresh rotation — every subject from scratch, no
// resume state. Callers wanting to resume from a persisted
// checkpoint use Service.Resume instead; it reuses the same
// machinery via a shared internal entry point.
//
//	res := svc.Run(c.Context(), runner)
//	if !res.OK { return res }
func (s *Service) Run(ctx core.Context, runner Runner) core.Result {
	return s.runInternal(ctx, runner, nil)
}

// Resume drives a rotation pre-stamped with the curriculum lattice
// captured in cp. Subjects already in cp.GrokedSubjects are skipped
// without running any probes (they've already settled); probes whose
// (subject, probe_id) appears in cp.CompletedProbes are skipped at
// the probe-level entry so their R₁ doesn't duplicate on disk.
//
// cp must be non-nil and target the same model as runner — Resume
// returns Fail immediately if cp.Model != runner.ModelID(). The
// checkpoint's StartedAt is preserved on subsequent saves so the
// "rotation has been alive for N hours" UX reads off the original
// start time rather than the resume point.
//
// On clean completion the checkpoint is cleared the same way Run
// does. On divergence trip or cancel, the checkpoint stays for the
// operator to inspect.
//
//	cp := training.LoadCheckpoint(runner.ModelID()).Value.(*training.Checkpoint)
//	res := svc.Resume(c.Context(), runner, cp)
//	if !res.OK { return res }
func (s *Service) Resume(ctx core.Context, runner Runner, cp *Checkpoint) core.Result {
	if runner == nil {
		return core.Fail(core.E("training.Resume", "runner is nil", nil))
	}
	if cp == nil {
		return core.Fail(core.E("training.Resume", "checkpoint is nil", nil))
	}
	if cp.SchemaVersion != CheckpointSchemaVersion {
		return core.Fail(core.E("training.Resume",
			core.Sprintf("unknown schema_version=%d (current=%d)",
				cp.SchemaVersion, CheckpointSchemaVersion),
			nil))
	}
	if cp.Model != runner.ModelID() {
		return core.Fail(core.E("training.Resume",
			core.Sprintf("checkpoint Model=%q != runner Model=%q",
				cp.Model, runner.ModelID()),
			nil))
	}
	return s.runInternal(ctx, runner, cp)
}

// runInternal is the shared body for Run + Resume. resumeFrom is nil
// for the fresh-start path (Run) and non-nil for the resume path
// (Resume); the only difference at the loop level is whether the
// subject loop skips already-groked subjects and whether runProbe
// skips already-completed probes.
func (s *Service) runInternal(ctx core.Context, runner Runner, resumeFrom *Checkpoint) core.Result {
	if runner == nil {
		return core.Fail(core.E("training.Run", "runner is nil", nil))
	}
	if !s.runMu.TryLock().OK {
		return core.Fail(core.E("training.Run", "rotation already in progress", nil))
	}
	defer s.runMu.Unlock()

	if resumeFrom != nil {
		s.beginResumeRun(resumeFrom)
	} else {
		s.beginRun()
	}
	defer s.endRun()

	startedAt := core.UnixNow()
	cancelled := false
	subjects := append([]string(nil), s.opts.Subjects...)
	totalGroked := 0

	var divergence *DivergenceInfo

	for i, subject := range subjects {
		if isCancelled(ctx) {
			cancelled = true
			break
		}
		// Resume skip — already-groked subjects produce no new probes
		// (curriculum advanced past them in the saved rotation). Counted
		// in totalGroked for the RunCompleteEvent payload so the resume
		// caller sees the cumulative grok count, not just the delta from
		// the resume point.
		if resumeFrom != nil && s.isGroked(subject) {
			totalGroked++
			continue
		}
		groked, div := s.runSubject(ctx, runner, subjects, i, subject)
		if groked {
			totalGroked++
			s.recordGroked(subject)
		}
		if div != nil {
			// Guard tripped — record it, emit the run-complete event so
			// frontend subscribers see the rotation end, then surface a
			// Fail to the caller. R₁s captured before the trip have
			// already been persisted via r1.Write inside runProbe.
			divergence = div
			s.recordDivergence(div)
			s.emit(EventSubjectAdvance, SubjectAdvanceEvent{
				FromSubject: subject,
				ToSubject:   "",
				GrokedCount: totalGroked,
				Total:       len(subjects),
			})
			break
		}
		if isCancelled(ctx) {
			cancelled = true
			// Emit advance for the subject we just finished, but
			// targeting an empty ToSubject — the Run is ending.
			s.emit(EventSubjectAdvance, SubjectAdvanceEvent{
				FromSubject: subject,
				ToSubject:   "",
				GrokedCount: totalGroked,
				Total:       len(subjects),
			})
			break
		}
		next := ""
		if i+1 < len(subjects) {
			next = subjects[i+1]
		}
		s.emit(EventSubjectAdvance, SubjectAdvanceEvent{
			FromSubject: subject,
			ToSubject:   next,
			GrokedCount: totalGroked,
			Total:       len(subjects),
		})
	}

	elapsed := float64(core.UnixNow() - startedAt)
	s.emit(EventRunComplete, RunCompleteEvent{
		TotalSubjects:  len(subjects),
		GrokedSubjects: totalGroked,
		ElapsedSeconds: elapsed,
		Cancelled:      cancelled,
	})
	if divergence != nil {
		// Divergence trip — checkpoint stays on disk so the operator
		// can inspect the rotation state that led to the trip.
		switch divergence.Reason {
		case DivergenceReasonNaNLoss:
			return core.Fail(core.E(
				"training.guard.nan",
				core.Sprintf("loss became NaN at step %d", divergence.Step),
				nil,
			))
		case DivergenceReasonMonotonicRise:
			return core.Fail(core.E(
				"training.guard.divergence",
				core.Sprintf("loss rose monotonically over %d steps", divergence.WindowSize),
				nil,
			))
		}
	}
	if !cancelled {
		// Clean rotation end — every subject either groked or
		// exhausted its probe budget. Discard the checkpoint so the
		// next Service.Run starts fresh rather than resuming an
		// already-completed curriculum.
		if r := ClearCheckpoint(runner.ModelID()); !r.OK {
			core.Warn("training.ClearCheckpoint", "error", r.Error())
		}
	}
	return core.Ok(RunCompleteEvent{
		TotalSubjects:  len(subjects),
		GrokedSubjects: totalGroked,
		ElapsedSeconds: elapsed,
		Cancelled:      cancelled,
	})
}

// runSubject drives epoch 1 (R₁ capture) + epoch 2 (loss observation)
// for every probe of one subject. Returns (groked, divergence) where
// groked reports whether any probe groked and divergence is non-nil
// when the in-loop guard tripped (NaN or monotonic-rise).
//
// subjects + subjectIndex are passed through so the checkpoint snapshot
// at probe boundaries records the rotation position accurately (this
// subject is index `subjectIndex` of the canonical Subjects slice).
//
// Probes that fail at the seed-read or sandwich-build step are skipped
// with a single emit-and-continue; one bad probe does not halt the
// rotation. A divergence trip, however, ends the subject immediately —
// the caller (Run) ends the whole rotation.
func (s *Service) runSubject(ctx core.Context, runner Runner, subjects []string, subjectIndex int, subject string) (bool, *DivergenceInfo) {
	s.setActive(subject, "", 0, 0)

	probesRes := seeds.ListProbes(subject, seeds.Options{Root: s.opts.SeedsRoot})
	if !probesRes.OK {
		return false, nil
	}
	probes, _ := probesRes.Value.([]string)

	if s.opts.ProbesPerSubject > 0 && len(probes) > s.opts.ProbesPerSubject {
		probes = probes[:s.opts.ProbesPerSubject]
	}

	detector := clbpl.NewDetector(s.opts.CLBPLOptions)

	for probeIndex, probeID := range probes {
		if isCancelled(ctx) {
			return detector.Groked(), nil
		}
		div := s.runProbe(ctx, runner, detector, subjects, subjectIndex, probeIndex, subject, probeID)
		if div != nil {
			return detector.Groked(), div
		}
		if detector.Groked() {
			// Probe-level grok ends the subject — CL-BPL fired, so
			// the curriculum advances per RFC.fork-tree §5.
			return true, nil
		}
	}
	return detector.Groked(), nil
}

// runProbe runs epoch 1 (one GenerateResponse + r1.Write) and epoch 2
// (StepBatch loop observed by the CL-BPL detector) for a single
// (subject, probeID) pair.
//
// subjects + subjectIndex + probeIndex are passed through so the
// checkpoint snapshot at probe-boundary save records rotation
// position accurately.
//
// Errors at the seed-read / sandwich-build / generate step are
// surfaced into Status but do not halt the rotation — the caller
// (runSubject) moves on to the next probe.
//
// Returns a *DivergenceInfo when the in-loop guard trips (NaN loss or
// monotonic-rise over divergenceWindowSize steps); nil on clean exit
// (grok, budget exhaust, ctx cancel, or any non-loss error).
//
// A checkpoint snapshot fires at every probe-boundary exit (defer-driven)
// so the on-disk file reflects the latest CompletedProbes set even when
// the probe terminates abnormally (runner error, ctx cancel). The
// checkpoint write is fire-and-warn — a failed write logs at Warn and
// does NOT halt the rotation.
func (s *Service) runProbe(ctx core.Context, runner Runner, detector *clbpl.Detector, subjects []string, subjectIndex, probeIndex int, subject, probeID string) *DivergenceInfo {
	// Resume skip — the (subject, probeID) finished epoch-2 in the
	// rotation that produced the checkpoint we resumed from. Skipping
	// here avoids regenerating an R₁ that's already on disk (which
	// would duplicate the corpus) and re-running an epoch-2 loop the
	// detector has already observed.
	if s.isCompletedProbe(subject, probeID) {
		return nil
	}
	s.setActiveProbe(subject, probeID)
	defer s.snapshotCheckpoint(runner, subjects, subjectIndex, probeIndex, subject, probeID)

	probeText := s.readProbe(subject, probeID)
	if probeText == "" {
		return nil
	}

	sandwichRes := sandwich.BuildLEK1(probeText)
	if !sandwichRes.OK {
		return nil
	}
	prompt, _ := sandwichRes.Value.(string)

	// Epoch 1 — generate the R₁, persist it.
	respRes := runner.GenerateResponse(prompt)
	if !respRes.OK {
		return nil
	}
	response, _ := respRes.Value.(string)

	rec := r1.R1{
		Model:     runner.ModelID(),
		Subject:   subject,
		ProbeID:   probeID,
		Substrate: runner.Substrate(),
		Tier:      runner.Tier(),
		Kernel:    "lek-1",
		Sig:       "lek-1-sig",
		Prompt:    prompt,
		Response:  response,
	}
	if s.opts.CaptureFingerprint {
		rec.Fingerprint = fingerprintFor(response)
	}
	if wr := r1.Write(rec); wr.OK {
		// r1.Write enriches Hash inside enrich(); re-read the
		// persisted hash from the corpus so the event payload
		// matches what's on disk verbatim.
		hash := lookupR1Hash(runner.ModelID(), subject, probeID, response)
		s.emit(EventR1Captured, R1CapturedEvent{
			Subject: subject,
			ProbeID: probeID,
			Hash:    hash,
		})
	}

	// Epoch 2 — observe the loss stream until grok or budget exhaust.
	// Stack-local ring buffer + count for the monotonic-rise guard.
	// Each (subject, probe) iteration starts fresh — a rise streak
	// does not bleed across probes. ring[head] is the most recent
	// loss; rises counts consecutive strictly-increasing samples.
	var ring [divergenceWindowSize]float64
	var head int
	rises := 0
	haveLast := false
	var lastLoss float64

	cap := s.opts.Epoch2MaxSteps
	for stepIdx := 0; cap == 0 || stepIdx < cap; stepIdx++ {
		if isCancelled(ctx) {
			return nil
		}
		stepRes := runner.StepBatch(probeText, response)
		if !stepRes.OK {
			return nil
		}
		loss, _ := stepRes.Value.(float64)

		// Guard 1 — NaN loss immediately ends the Run. A NaN sample
		// would silently corrupt the CL-BPL envelope detector, so we
		// trip before handing it to Observe.
		if core.IsNaN(loss) {
			return &DivergenceInfo{
				Subject: subject,
				ProbeID: probeID,
				Step:    stepIdx,
				Reason:  DivergenceReasonNaNLoss,
			}
		}

		// Guard 2 — monotonic rise. Track consecutive strict
		// increases; trip when we see divergenceWindowSize of them
		// in a row.
		if haveLast {
			if loss > lastLoss {
				rises++
			} else {
				rises = 0
			}
		}
		ring[head] = loss
		head = (head + 1) % divergenceWindowSize
		lastLoss = loss
		haveLast = true
		if rises >= divergenceWindowSize {
			return &DivergenceInfo{
				Subject:    subject,
				ProbeID:    probeID,
				Step:       stepIdx,
				Reason:     DivergenceReasonMonotonicRise,
				WindowSize: divergenceWindowSize,
			}
		}

		ev := detector.Observe(stepIdx, loss)
		s.advanceStep(stepIdx, len(detector.Peaks()))

		switch ev.Kind {
		case clbpl.EventPeak:
			s.emit(EventPeak, PeakEvent{
				Subject:   subject,
				ProbeID:   probeID,
				Step:      ev.Step,
				Loss:      ev.Value,
				PeakIndex: ev.PeakIndex,
			})
		case clbpl.EventGroked:
			s.emit(EventGroked, GrokedEvent{
				Subject:           subject,
				ProbeID:           probeID,
				Step:              ev.Step,
				Loss:              ev.Value,
				PredictedNextPeak: ev.PredictedNextPeak,
			})
			return nil
		}
	}
	return nil
}

// readProbe pulls the probe prompt text from the seed corpus, or
// returns "" if the read fails (orchestrator skips on empty).
func (s *Service) readProbe(subject, probeID string) string {
	res := seeds.Read(subject, probeID, seeds.Options{Root: s.opts.SeedsRoot})
	if !res.OK {
		return ""
	}
	text, _ := res.Value.(string)
	return text
}

// emit broadcasts an event onto the Core action bus and, when the
// Wails event-bridge action is registered, also forwards the named
// event so frontend subscribers (Events.On("training:peak", ...)) see
// the same payload.
//
// Safe to call before Register — emit is a no-op when s.core is nil.
// The "events.emit" action is wired by pkg/desktop at boot; in tests
// it's absent and the named-bridge call no-ops harmlessly.
func (s *Service) emit(name string, payload any) {
	if s.core == nil {
		return
	}
	// Typed broadcast for in-process subscribers (matches the
	// benchmark.BenchCompleted pattern). Tests subscribe through
	// c.RegisterAction.
	s.core.ACTION(payload)
	// Named broadcast for frontend / cross-language subscribers via
	// the Wails event bridge. The bridge wraps name + payload into a
	// guievents.TaskEmit shape; we pass a plain map so this package
	// stays decoupled from the gui import — the bridge handles the
	// repack on receipt.
	if bridge := s.core.Action("events.emit"); bridge.Exists() {
		_ = bridge.Run(
			core.Background(),
			core.NewOptions(
				core.Option{Key: "name", Value: name},
				core.Option{Key: "data", Value: payload},
			),
		)
	}
}

// beginRun stamps the running flag at the top of Run.
func (s *Service) beginRun() {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	s.running = true
	s.activeSubject = ""
	s.activeProbe = ""
	s.currentStep = 0
	s.currentPeaks = 0
	s.grokedSubjects = s.grokedSubjects[:0]
	s.divergence = nil
	s.startedAt = core.UnixNow()
	s.completedProbes = s.completedProbes[:0]
}

// beginResumeRun stamps the running flag + pre-populates state from
// a persisted checkpoint. Preserves cp.StartedAt as the rotation
// origin so the SavedAt - StartedAt window reflects total elapsed
// rotation time (not just the resumed portion). grokedSubjects +
// completedProbes are copied verbatim so the subject loop + runProbe
// skip-checks can consult them via isGroked / isCompletedProbe.
func (s *Service) beginResumeRun(cp *Checkpoint) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	s.running = true
	s.activeSubject = ""
	s.activeProbe = ""
	s.currentStep = 0
	s.currentPeaks = 0
	s.divergence = nil
	if cp.StartedAt > 0 {
		s.startedAt = cp.StartedAt
	} else {
		s.startedAt = core.UnixNow()
	}
	s.grokedSubjects = append(s.grokedSubjects[:0], cp.GrokedSubjects...)
	s.completedProbes = append(s.completedProbes[:0], cp.CompletedProbes...)
}

// isGroked reports whether subject is in the current grokedSubjects
// set. Resume uses this in the subject loop to short-circuit subjects
// that have already settled.
func (s *Service) isGroked(subject string) bool {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	for _, g := range s.grokedSubjects {
		if g == subject {
			return true
		}
	}
	return false
}

// isCompletedProbe reports whether (subject, probeID) is in the
// completedProbes set. Resume uses this in runProbe to skip probes
// whose R₁ is already persisted + epoch-2 already terminated.
func (s *Service) isCompletedProbe(subject, probeID string) bool {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	for _, pk := range s.completedProbes {
		if pk.Subject == subject && pk.ProbeID == probeID {
			return true
		}
	}
	return false
}

// recordDivergence stamps the trip onto Status and broadcasts the
// EventDivergence event. Called from Run when runSubject returns a
// non-nil *DivergenceInfo.
func (s *Service) recordDivergence(div *DivergenceInfo) {
	s.stateMu.Lock()
	cp := *div
	s.divergence = &cp
	s.stateMu.Unlock()
	s.emit(EventDivergence, DivergenceEvent{
		Subject:    div.Subject,
		ProbeID:    div.ProbeID,
		Step:       div.Step,
		Reason:     div.Reason,
		WindowSize: div.WindowSize,
	})
}

// endRun clears the running flag at the bottom of Run.
func (s *Service) endRun() {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	s.running = false
	s.activeSubject = ""
	s.activeProbe = ""
	s.completedRuns++
}

// setActive stamps the subject + initial probe / step counters.
func (s *Service) setActive(subject, probe string, step, peaks int) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	s.activeSubject = subject
	s.activeProbe = probe
	s.currentStep = step
	s.currentPeaks = peaks
}

// setActiveProbe stamps the active probe while keeping subject + step
// counters intact.
func (s *Service) setActiveProbe(subject, probe string) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	s.activeSubject = subject
	s.activeProbe = probe
	s.currentStep = 0
	s.currentPeaks = 0
}

// advanceStep stamps the current step + peak counters from the
// detector's most recent observation.
func (s *Service) advanceStep(step, peaks int) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	s.currentStep = step
	s.currentPeaks = peaks
}

// recordGroked appends a subject to the groked list (dedup-safe).
func (s *Service) recordGroked(subject string) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	for _, g := range s.grokedSubjects {
		if g == subject {
			return
		}
	}
	s.grokedSubjects = append(s.grokedSubjects, subject)
}

// isCancelled reports whether ctx has been cancelled. Non-blocking.
func isCancelled(ctx core.Context) bool {
	if ctx == nil {
		return false
	}
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}

// fingerprintFor scores text via contentshield and flattens the
// resulting ImprintScores into a dim map. Returns nil when scoring
// produces no imprint (very short / empty text).
//
// Wire-stable: dim keys mirror the JSON tag names on
// contentshield.ImprintScores so downstream consumers can decode
// against the same schema.
func fingerprintFor(text string) map[string]float64 {
	res := contentshield.Score(text)
	if res.Imprint == nil {
		return nil
	}
	imp := res.Imprint
	return map[string]float64{
		"vocab_richness": imp.VocabRichness,
		"tense_entropy":  imp.TenseEntropy,
		"question_ratio": imp.QuestionRatio,
		"domain_depth":   imp.DomainDepth,
		"verb_diversity": imp.VerbDiversity,
		"noun_diversity": imp.NounDiversity,
		// U lane phonetic-tier dimensions — captured at R₁ generation
		// per [[feedback-data-is-the-return-no-rescoring]]. SyllableCount
		// converted to float64 to fit the uniform fingerprint map type.
		"syllable_count":       float64(imp.SyllableCount),
		"rhyme_density":        imp.RhymeDensity,
		"sigil_entropy":        imp.SigilEntropy,
		"alliteration_density": imp.AlliterationDensity,
		"assonance_density":    imp.AssonanceDensity,
		"pun_density":          imp.PunDensity,
		"pseudo_jargon_density": imp.PseudoJargonDensity,
		"meter_regularity":     imp.MeterRegularity,
	}
}

// snapshotCheckpoint persists the current rotation state to disk via
// SaveCheckpoint, appending (subject, probeID) to the completed-probes
// set first. Called via defer at every runProbe exit so every probe
// boundary (clean grok, cap-hit, runner error, ctx cancel, divergence
// trip) leaves an up-to-date checkpoint behind for the operator's
// next session.
//
// Fire-and-warn: a write failure is logged at Warn and does NOT halt
// the rotation. A failed checkpoint write is a regression, not a
// crisis — the next probe-boundary save will succeed and supersede
// whatever the previous attempt would have written.
//
// State is sampled under stateMu so the snapshot is internally
// consistent even when Status() / Run() are racing against the read.
func (s *Service) snapshotCheckpoint(runner Runner, subjects []string, subjectIndex, probeIndex int, subject, probeID string) {
	if runner == nil || runner.ModelID() == "" {
		return
	}
	s.recordCompletedProbe(subject, probeID)
	cp := s.buildCheckpoint(runner, subjects, subjectIndex, probeIndex)
	if r := SaveCheckpoint(cp); !r.OK {
		core.Warn("training.SaveCheckpoint", "model", runner.ModelID(), "error", r.Error())
	}
}

// recordCompletedProbe appends (subject, probeID) to the completed set
// (dedup-safe). The set persists across the Run until beginRun resets
// it on the next Service.Run invocation.
func (s *Service) recordCompletedProbe(subject, probeID string) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	for _, pk := range s.completedProbes {
		if pk.Subject == subject && pk.ProbeID == probeID {
			return
		}
	}
	s.completedProbes = append(s.completedProbes, ProbeKey{Subject: subject, ProbeID: probeID})
}

// buildCheckpoint copies Service state into a Checkpoint shape for
// persistence. Reads under stateMu so the snapshot is consistent
// with concurrent Status() calls + ongoing runProbe state writes.
func (s *Service) buildCheckpoint(runner Runner, subjects []string, subjectIndex, probeIndex int) Checkpoint {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	groked := make([]string, len(s.grokedSubjects))
	copy(groked, s.grokedSubjects)
	completed := make([]ProbeKey, len(s.completedProbes))
	copy(completed, s.completedProbes)
	subjectsCopy := make([]string, len(subjects))
	copy(subjectsCopy, subjects)
	return Checkpoint{
		Model:           runner.ModelID(),
		Substrate:       runner.Substrate(),
		Tier:            runner.Tier(),
		Subjects:        subjectsCopy,
		SubjectIndex:    subjectIndex,
		ProbeIndex:      probeIndex,
		GrokedSubjects:  groked,
		CompletedProbes: completed,
		StartedAt:       s.startedAt,
	}
}

// lookupR1Hash returns the Hash stamp r1.Write enriched onto the
// most recently persisted record matching (model, subject, probeID,
// response). Returns "" when no match is found — emit-and-continue.
func lookupR1Hash(model, subject, probeID, response string) string {
	res := r1.Read(r1.Filter{Model: model, Subject: subject, ProbeID: probeID})
	if !res.OK {
		return ""
	}
	recs, _ := res.Value.([]r1.R1)
	// Walk newest-to-oldest so the most recent matching write wins.
	for i := len(recs) - 1; i >= 0; i-- {
		if recs[i].Response == response {
			return recs[i].Hash
		}
	}
	return ""
}
