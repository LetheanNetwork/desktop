// SPDX-Licence-Identifier: EUPL-1.2

// Package training is the autocratic-cascade training orchestrator
// (Phase A — single-tier, multi-subject rotation) per
// plans/project/lthn/lem/RFC.fork-tree.md §5.
//
// The orchestrator composes five existing packages — seeds (probe
// corpus), sandwich (LEK-1 prompt builder), r1 (R₁ persistence), clbpl
// (loss-envelope detector), contentshield (optional fingerprint) — and
// drives a hypothetical training runner through a fixed subject
// rotation, reacting to CL-BPL grok events to advance the curriculum.
//
// The runner is consumed via the Runner interface so the package builds
// and tests without a real go-mlx wrapper. A FixtureRunner shipped
// alongside provides deterministic-loss synthetic responses for tests
// and for the GUI fixture mode. Production runners (go-mlx, etc.) live
// elsewhere behind the same interface.
//
//	svc := training.NewService(c, training.Options{
//	    Subjects:         []string{"english", "european"},
//	    ProbesPerSubject: 3,
//	})
//	svc.Register(c)
//	res := svc.Run(c.Context(), training.NewFixtureRunner(training.FixtureConfig{
//	    Model: "gemma4-e2b-it-q4", Tier: 0, Substrate: "CONT",
//	}))
package training

import (
	core "dappco.re/go"

	"dappco.re/lthn/desktop/pkg/clbpl"
)

// canonicalSubjects is the Phase A rotation order per
// plans/project/lthn/lem/RFC.fork-tree.md §5. NewService falls back to
// this list when Options.Subjects is empty.
var canonicalSubjects = []string{
	"english",
	"european",
	"latam",
	"russian",
	"middle-east",
	"chinese",
	"african",
}

// defaultEpoch2MaxSteps caps epoch-2 step count per probe when the
// CL-BPL detector never groks. Stops a flat or pathological loss
// stream from running forever. Options.Epoch2MaxSteps overrides per
// Service; 0 in Options means "use this default".
const defaultEpoch2MaxSteps = 1024

// Event-name constants — exported so subscribers can match them
// without re-typing the string. Wire stability matters: the frontend
// listens for these via Wails Events.On, so renames are a wire break.
const (
	EventPeak           = "training:peak"
	EventGroked         = "training:groked"
	EventSubjectAdvance = "training:subject_advance"
	EventR1Captured     = "training:r1_captured"
	EventRunComplete    = "training:run_complete"
	EventDivergence     = "training:divergence"
)

// Divergence reason constants — wire-stable identifiers stamped onto
// DivergenceInfo.Reason and the EventDivergence payload so frontend
// consumers can branch on the trigger without parsing the human
// summary string.
const (
	DivergenceReasonNaNLoss       = "nan_loss"
	DivergenceReasonMonotonicRise = "monotonic_rise"
)

// Runner is the interface the training orchestrator drives.
//
// Production implementations (go-mlx wrapper, llama.cpp shim, etc.)
// live elsewhere; the orchestrator only needs the operations below.
// Keep this interface narrow — every new method is a constraint on
// every alternative runtime.
//
//	r := training.NewFixtureRunner(training.FixtureConfig{
//	    Model: "gemma4-e2b-it-q4", Tier: 0, Substrate: "CONT",
//	})
//	// r is a Runner; pass it to Service.Run.
type Runner interface {
	// StepBatch runs one training step against the (prompt, target)
	// pair. Returns core.Ok(loss float64) on success or core.Fail on
	// runner error. Called many times per probe during epoch 2.
	StepBatch(prompt, target string) core.Result

	// GenerateResponse runs epoch-1 inference: feeds the sandwich
	// prompt to the model, captures the response as text. Returns
	// core.Ok(string) on success.
	GenerateResponse(prompt string) core.Result

	// ModelID returns the canonical model identifier this runner is
	// loaded against (e.g. "gemma4-e2b-it-q4"). Captured into every
	// R₁ written during a Run.
	ModelID() string

	// Substrate returns the canonical substrate condition label: "CONT",
	// "TRAD", "TRAD-no-replay", or "CONT-with-gap". Captured into every
	// R₁ — the substrate-shift experiment depends on this being honest.
	Substrate() string

	// Tier returns the cascade tier this runner serves
	// (0 = E2B, 1 = E4B, 2 = 26B, 3 = 31B).
	Tier() int
}

// Options configures the Service.
//
// Zero-valued fields fall back to canonical defaults — Subjects to
// the RFC.fork-tree §5 list, CLBPLOptions to clbpl.Defaults() — so the
// canonical entry shape (training.NewService(c, training.Options{})) is
// valid.
//
//	opts := training.Options{
//	    Subjects:         []string{"english", "chinese"},
//	    ProbesPerSubject: 3,
//	    CaptureFingerprint: true,
//	}
type Options struct {
	// Subjects in rotation order. Empty falls back to the seven
	// canonical subjects from RFC.fork-tree §5.
	Subjects []string

	// SeedsRoot overrides the seed-corpus root passed to pkg/seeds.
	// Empty uses the package default (/Volumes/Data/lem/training/seeds).
	SeedsRoot string

	// ProbesPerSubject caps the number of probes per subject in a
	// single Run. 0 = unlimited. The fixture test caps at 3 to keep
	// wall-time tight.
	ProbesPerSubject int

	// CLBPLOptions configures the per-subject loss-envelope detector.
	// Zero-valued fields fall back to clbpl.Defaults() — Window=8,
	// GrokThreshold=0.05, GrokConfirmPeaks=3.
	CLBPLOptions clbpl.Options

	// CaptureFingerprint, when true, scores every R₁ response via
	// contentshield.Score and embeds the resulting ImprintScores
	// dimensions into R₁.Fingerprint. Default false — training paths
	// often skip this to keep the step loop tight.
	CaptureFingerprint bool

	// Epoch2MaxSteps caps epoch-2 step count per probe when the
	// detector never groks. 0 uses the package default
	// (defaultEpoch2MaxSteps). Useful for tests that want a tight
	// bound, and a safety net in production against pathological
	// loss streams.
	Epoch2MaxSteps int
}

// Status is the immutable snapshot of Service rotation state returned
// by Service.Status. Safe to hand to TS bindings — every field is a
// scalar / slice / string.
//
// ActiveSubject is "" when no rotation is in progress. GrokedSubjects
// lists the subjects whose CL-BPL detector fired EventGroked during
// the most recent (or current) Run.
type Status struct {
	Running        bool            `json:"running"`
	ActiveSubject  string          `json:"active_subject,omitempty"`
	ActiveProbe    string          `json:"active_probe,omitempty"`
	Step           int             `json:"step"`
	PeakCount      int             `json:"peak_count"`
	GrokedSubjects []string        `json:"groked_subjects,omitempty"`
	CompletedRuns  int             `json:"completed_runs"`
	Divergence     *DivergenceInfo `json:"divergence,omitempty"`
}

// DivergenceInfo is the trip-record produced when the in-loop guard
// pauses a Run because the loss stream went pathological — either NaN
// or strictly rising for divergenceWindowSize consecutive steps. nil
// on Status when no guard has fired during the most recent (or
// current) Run.
//
// Reason carries one of the DivergenceReason* constants so frontend
// branches off a stable identifier rather than the human summary.
// WindowSize is non-zero only for the monotonic-rise trigger; the NaN
// trigger leaves it at 0.
type DivergenceInfo struct {
	Subject    string `json:"subject"`
	ProbeID    string `json:"probe_id"`
	Step       int    `json:"step"`
	Reason     string `json:"reason"`
	WindowSize int    `json:"window_size,omitempty"`
}

// DivergenceEvent is the payload broadcast on EventDivergence
// ("training:divergence") when the in-loop guard trips. Mirrors
// DivergenceInfo so frontend subscribers can decode either shape with
// the same handler.
type DivergenceEvent struct {
	Subject    string `json:"subject"`
	ProbeID    string `json:"probe_id"`
	Step       int    `json:"step"`
	Reason     string `json:"reason"`
	WindowSize int    `json:"window_size,omitempty"`
}

// PeakEvent is the payload broadcast on EventPeak ("training:peak").
//
// Fired each time the CL-BPL detector confirms a local maximum during
// epoch-2 step iteration. PeakIndex is the zero-based ordinal of the
// peak in this (subject, probe) detector's lifetime.
type PeakEvent struct {
	Subject   string  `json:"subject"`
	ProbeID   string  `json:"probe_id"`
	Step      int     `json:"step"`
	Loss      float64 `json:"loss"`
	PeakIndex int     `json:"peak_index"`
}

// GrokedEvent is the payload broadcast on EventGroked ("training:groked").
//
// Fired once N consecutive peaks fall within the configured amplitude
// threshold — model has internalised the current (subject, probe). The
// orchestrator stops epoch-2 iteration for this probe when it fires.
type GrokedEvent struct {
	Subject           string  `json:"subject"`
	ProbeID           string  `json:"probe_id"`
	Step              int     `json:"step"`
	Loss              float64 `json:"loss"`
	PredictedNextPeak int     `json:"predicted_next_peak,omitempty"`
}

// SubjectAdvanceEvent is the payload broadcast on EventSubjectAdvance
// ("training:subject_advance") each time the orchestrator finishes one
// subject and moves to the next in the rotation.
//
// ToSubject is "" when FromSubject was the last in the rotation; the
// EventRunComplete event fires immediately after the advance.
type SubjectAdvanceEvent struct {
	FromSubject string `json:"from_subject"`
	ToSubject   string `json:"to_subject,omitempty"`
	GrokedCount int    `json:"groked_count"`
	Total       int    `json:"total"`
}

// R1CapturedEvent is the payload broadcast on EventR1Captured
// ("training:r1_captured") immediately after r1.Write succeeds for a
// freshly-generated epoch-1 response.
type R1CapturedEvent struct {
	Subject string `json:"subject"`
	ProbeID string `json:"probe_id"`
	Hash    string `json:"hash"`
}

// RunCompleteEvent is the payload broadcast on EventRunComplete
// ("training:run_complete") once the full rotation finishes (every
// subject either groked or exhausted its probe budget) or once the
// context is cancelled.
type RunCompleteEvent struct {
	TotalSubjects  int     `json:"total_subjects"`
	GrokedSubjects int     `json:"groked_subjects"`
	ElapsedSeconds float64 `json:"elapsed_seconds"`
	Cancelled      bool    `json:"cancelled,omitempty"`
}

// CheckpointSchemaVersion is the wire version of the Checkpoint JSON
// blob. Increment on any breaking shape change so the loader can
// branch on it (return Fail with a clear message rather than decode a
// stale shape silently). v1 is the current shape; no migration path
// needed yet.
const CheckpointSchemaVersion = 1

// ProbeKey is one entry in Checkpoint.CompletedProbes — a
// (subject, probe_id) pair that finished epoch-2 in the current Run
// (groked or hit the cap). Used on resume to skip probes whose R₁ is
// already on disk + whose epoch-2 either settled or terminated.
type ProbeKey struct {
	Subject string `json:"subject"`
	ProbeID string `json:"probe_id"`
}

// Checkpoint is the persistent on-disk snapshot of a Service rotation
// — saved at probe boundaries, cleared on clean Run completion. One
// checkpoint per canonical model ID, at
// paths.TrainingCheckpointDir()/<model>.json.
//
// Scope: ORCHESTRATOR state only. Model weights, KV cache, optimizer
// state are runner-side (go-mlx native snapshot, LoRA adapter, etc.) —
// the runner's own resume path takes care of those. This file
// captures the curriculum lattice: which subjects we're rotating
// through, where we are in the rotation, which probes have already
// written R₁s, which subjects have settled per CL-BPL.
//
// Atomicity: Save writes via core.Fs() atomic write (tmp + rename) so
// a crash mid-write leaves the previous valid checkpoint intact. The
// reader sees either the old contents or the new — never a torn JSON.
//
//	cp := training.Checkpoint{
//	    SchemaVersion: training.CheckpointSchemaVersion,
//	    Model: "gemma4-e2b-it-q4", Substrate: "CONT", Tier: 0,
//	    Subjects: []string{"english", "european"},
//	    SubjectIndex: 1, ProbeIndex: 4,
//	    GrokedSubjects: []string{"english"},
//	    CompletedProbes: []training.ProbeKey{
//	        {Subject: "english", ProbeID: "P01"},
//	    },
//	    StartedAt: 1747728000, SavedAt: 1747728900,
//	}
//	if r := training.SaveCheckpoint(c, cp); !r.OK { /* fall back */ }
type Checkpoint struct {
	// SchemaVersion is stamped on every Save so a future reader can
	// reject incompatible shapes cleanly. Must equal
	// CheckpointSchemaVersion when calling Save; Load reports Fail if
	// the persisted value is unknown.
	SchemaVersion int `json:"schema_version"`

	// Model is the canonical runner identifier the rotation is
	// targeting (e.g. "gemma4-e2b-it-q4"). Doubles as the filename
	// stem for the on-disk checkpoint.
	Model string `json:"model"`

	// Substrate captures the substrate-shift experiment branch — "CONT"
	// (continuous stream, no re-prefill) or "TRAD" (re-prefill per
	// turn). Load-bearing: the same model trained under different
	// substrates produces detectably different curves per
	// experiments/worf/01-theory.md H1.
	Substrate string `json:"substrate"`

	// Tier is the autocratic-cascade tier the runner serves
	// (0 = E2B, 1 = E4B, 2 = 26B, 3 = 31B).
	Tier int `json:"tier"`

	// Subjects is the rotation order as ordered. Stamped on save so
	// the reader can detect curriculum-shape changes (operator edited
	// the canonical list between sessions) and refuse to resume.
	Subjects []string `json:"subjects"`

	// SubjectIndex is the position of the currently-active subject in
	// Subjects. 0 at the start of a rotation; advances on each
	// EventSubjectAdvance. When equal to len(Subjects), the rotation
	// is complete and the checkpoint will have been cleared by the
	// time we reach that state on a clean exit.
	SubjectIndex int `json:"subject_index"`

	// ProbeIndex is the position of the currently-active probe within
	// the active subject's probe list. Reset to 0 on subject advance.
	ProbeIndex int `json:"probe_index"`

	// GrokedSubjects is the set of subjects whose CL-BPL detector
	// fired EventGroked during this Run. On resume the orchestrator
	// skips them — they've already settled.
	GrokedSubjects []string `json:"groked_subjects,omitempty"`

	// CompletedProbes is every (subject, probe_id) pair whose epoch-2
	// loop has terminated this Run (groked OR hit Epoch2MaxSteps OR
	// runner returned an error). Used on resume to skip probes whose
	// R₁ is already persisted — re-running an epoch-1 generate against
	// an unchanged model is wasted work.
	CompletedProbes []ProbeKey `json:"completed_probes,omitempty"`

	// StartedAt is the Unix-seconds timestamp the rotation began
	// (stamped on the first Save of this Run, copied forward on
	// subsequent saves). Lets the operator UI show "Resume from
	// rotation started 2026-05-21 09:42" rather than a bare timestamp
	// of the last save.
	StartedAt int64 `json:"started_at"`

	// SavedAt is the Unix-seconds timestamp this checkpoint was
	// written. Re-stamped on every Save. Pairs with StartedAt for
	// "rotation has been alive for N hours" UX.
	SavedAt int64 `json:"saved_at"`
}
