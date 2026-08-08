// SPDX-Licence-Identifier: EUPL-1.2

package training

import (
	"testing"

	core "dappco.re/go"
)

// newWailsHarness wires a Core + *training.Service + WailsService
// against a per-test seed corpus + tight CL-BPL options so the fixture
// rotation finishes within the test wall-time budget (<2s).
func newWailsHarness(t *testing.T, subjects []string) *WailsService {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	liftPipeBufLimit(t)

	corpus := map[string][]string{}
	for _, s := range subjects {
		corpus[s] = []string{"P01_TEST"}
	}
	root := seedFixture(t, corpus)

	c := core.New()
	svc := NewService(c, Options{
		Subjects:         subjects,
		SeedsRoot:        root,
		ProbesPerSubject: 1,
		CLBPLOptions:     tightCLBPL(),
		Epoch2MaxSteps:   200,
	})
	if r := svc.Register(c); !r.OK {
		t.Fatalf("Register: %v", r.Value)
	}
	return NewWailsService(c, svc)
}

// --- Good ---

func TestWailsTraining_ServiceName_Good(t *testing.T) {
	w := NewWailsService(nil, nil)
	if got := w.ServiceName(); got != "Training" {
		t.Errorf("ServiceName = %q, want %q", got, "Training")
	}
}

func TestWailsTraining_Lifecycle_Good(t *testing.T) {
	w := NewWailsService(core.New(), NewService(core.New(), Options{}))
	if r := w.ServiceStartup(core.Background(), nil); !r.OK {
		t.Errorf("ServiceStartup: %v", r.Value)
	}
	if r := w.ServiceShutdown(); !r.OK {
		t.Errorf("ServiceShutdown: %v", r.Value)
	}
}

func TestWailsTraining_Status_Good(t *testing.T) {
	c := core.New()
	svc := NewService(c, Options{})
	w := NewWailsService(c, svc)

	r := w.Status()
	if !r.OK {
		t.Fatalf("Status: %v", r.Value)
	}
	snap, ok := r.Value.(Status)
	if !ok {
		t.Fatalf("Status returned %T, want Status", r.Value)
	}
	if snap.Running {
		t.Error("fresh WailsService should report Running=false")
	}
}

func TestWailsTraining_StartFixture_Good(t *testing.T) {
	w := newWailsHarness(t, []string{"english", "european"})

	r := w.StartFixture("fixture-e2b", "CONT", 0)
	if !r.OK {
		t.Fatalf("StartFixture: %v", r.Value)
	}

	// The goroutine flips running=true under the mutex before we
	// return; observable here without polling because StartFixture
	// itself acquired the mutex to set running before spawning.
	waitFor(t, func() bool {
		return w.IsRunning().Value.(bool)
	})

	// Let the rotation finish naturally — small corpus + tight CL-BPL
	// means it completes well inside the 2s test budget.
	waitFor(t, func() bool {
		return !w.IsRunning().Value.(bool)
	})

	// Post-run: Status reflects a completed run, not a cancelled one.
	snap := w.Status().Value.(Status)
	if snap.Running {
		t.Error("Status after natural finish reports Running=true")
	}
	if snap.CompletedRuns != 1 {
		t.Errorf("CompletedRuns = %d, want 1", snap.CompletedRuns)
	}
}

func TestWailsTraining_IsRunning_Good(t *testing.T) {
	c := core.New()
	svc := NewService(c, Options{})
	w := NewWailsService(c, svc)
	r := w.IsRunning()
	if !r.OK {
		t.Fatalf("IsRunning: %v", r.Value)
	}
	if r.Value.(bool) {
		t.Error("fresh WailsService should report IsRunning=false")
	}
}

func TestWailsTraining_ListGroked_Good(t *testing.T) {
	c := core.New()
	svc := NewService(c, Options{})
	w := NewWailsService(c, svc)
	r := w.ListGroked()
	if !r.OK {
		t.Fatalf("ListGroked: %v", r.Value)
	}
	got, ok := r.Value.([]string)
	if !ok {
		t.Fatalf("ListGroked returned %T, want []string", r.Value)
	}
	if len(got) != 0 {
		t.Errorf("fresh WailsService ListGroked = %v, want empty", got)
	}
}

func TestWailsTraining_ListGroked_PopulatedAfterRun_Good(t *testing.T) {
	w := newWailsHarness(t, []string{"english"})

	r := w.StartFixture("fixture-e2b", "CONT", 0)
	if !r.OK {
		t.Fatalf("StartFixture: %v", r.Value)
	}
	waitFor(t, func() bool { return !w.IsRunning().Value.(bool) })

	got := w.ListGroked().Value.([]string)
	if len(got) != 1 || got[0] != "english" {
		t.Errorf("ListGroked after run = %v, want [english]", got)
	}
}

// --- Bad ---

func TestWailsTraining_StartFixture_BadConcurrent(t *testing.T) {
	w := newWailsHarness(t, []string{"english", "european"})

	// First call locks in the running flag.
	if r := w.StartFixture("fixture-e2b", "CONT", 0); !r.OK {
		t.Fatalf("StartFixture #1: %v", r.Value)
	}
	// Wait until the goroutine has flipped running=true under the
	// mutex so the second call hits the in-progress branch
	// deterministically.
	waitFor(t, func() bool { return w.IsRunning().Value.(bool) })

	// Second call must Fail with "rotation already in progress".
	r := w.StartFixture("fixture-2", "CONT", 0)
	if r.OK {
		t.Error("concurrent StartFixture returned OK, want Fail")
	}

	// Drain the first run so the test exits cleanly.
	waitFor(t, func() bool { return !w.IsRunning().Value.(bool) })
}

func TestWailsTraining_Status_BadNilService(t *testing.T) {
	w := NewWailsService(core.New(), nil)
	if r := w.Status(); r.OK {
		t.Error("Status with nil service returned OK, want Fail")
	}
}

func TestWailsTraining_StartFixture_BadNilService(t *testing.T) {
	w := NewWailsService(core.New(), nil)
	if r := w.StartFixture("m", "CONT", 0); r.OK {
		t.Error("StartFixture with nil service returned OK, want Fail")
	}
}

func TestWailsTraining_StartFixture_BadNilCore(t *testing.T) {
	w := NewWailsService(nil, NewService(core.New(), Options{}))
	if r := w.StartFixture("m", "CONT", 0); r.OK {
		t.Error("StartFixture with nil core returned OK, want Fail")
	}
}

// --- Ugly ---

func TestWailsTraining_Stop_UglyIdempotent(t *testing.T) {
	c := core.New()
	svc := NewService(c, Options{})
	w := NewWailsService(c, svc)

	// Stop with no Run in flight: Ok with "no rotation in progress".
	r := w.Stop()
	if !r.OK {
		t.Errorf("Stop with no Run returned Fail, want Ok: %v", r.Value)
	}

	// Second call also Ok.
	r = w.Stop()
	if !r.OK {
		t.Errorf("idempotent Stop returned Fail: %v", r.Value)
	}
}

func TestWailsTraining_Stop_UglyCancelsMidRotation(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	liftPipeBufLimit(t)
	root := seedFixture(t, map[string][]string{
		"english":  {"P01_TEST", "P02_TEST"},
		"european": {"P03_TEST", "P04_TEST"},
	})

	c := core.New()
	// Wide Epoch2MaxSteps so the rotation cannot finish before Stop
	// is called — Stop's cancel is observable in Status afterwards.
	svc := NewService(c, Options{
		Subjects:         []string{"english", "european"},
		SeedsRoot:        root,
		ProbesPerSubject: 2,
		CLBPLOptions:     tightCLBPL(),
		Epoch2MaxSteps:   5000,
	})
	_ = svc.Register(c)
	w := NewWailsService(c, svc)

	r := w.StartFixture("fixture-stop", "CONT", 0)
	if !r.OK {
		t.Fatalf("StartFixture: %v", r.Value)
	}
	waitFor(t, func() bool { return w.IsRunning().Value.(bool) })

	// Stop mid-rotation — surface returns Ok with the cancellation
	// summary, and IsRunning flips back to false once the goroutine
	// observes the cancelled context.
	stopRes := w.Stop()
	if !stopRes.OK {
		t.Fatalf("Stop mid-rotation: %v", stopRes.Value)
	}

	waitFor(t, func() bool { return !w.IsRunning().Value.(bool) })

	// Status reflects the Cancelled flag's effect (Run returned and
	// the running flag is clear; CompletedRuns increments either way
	// because endRun runs in the Service's defer).
	snap := w.Status().Value.(Status)
	if snap.Running {
		t.Error("Status after Stop reports Running=true")
	}
}

func TestWailsTraining_ServiceShutdown_UglyCancelsActive(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	liftPipeBufLimit(t)
	root := seedFixture(t, map[string][]string{
		"english": {"P01_TEST", "P02_TEST"},
	})

	c := core.New()
	svc := NewService(c, Options{
		Subjects:         []string{"english"},
		SeedsRoot:        root,
		ProbesPerSubject: 2,
		CLBPLOptions:     tightCLBPL(),
		Epoch2MaxSteps:   5000,
	})
	_ = svc.Register(c)
	w := NewWailsService(c, svc)

	if r := w.StartFixture("fixture-shutdown", "CONT", 0); !r.OK {
		t.Fatalf("StartFixture: %v", r.Value)
	}
	waitFor(t, func() bool { return w.IsRunning().Value.(bool) })

	// ServiceShutdown cancels the live rotation cooperatively.
	if r := w.ServiceShutdown(); !r.OK {
		t.Fatalf("ServiceShutdown: %v", r.Value)
	}
	waitFor(t, func() bool { return !w.IsRunning().Value.(bool) })
}

// --- Checkpoint surface ---

func TestWailsTraining_HasCheckpoint_MissingReturnsFalse_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	w := NewWailsService(core.New(), NewService(core.New(), Options{}))
	r := w.HasCheckpoint("never-trained")
	if !r.OK {
		t.Fatalf("HasCheckpoint: %v", r.Value)
	}
	if r.Value.(bool) != false {
		t.Errorf("HasCheckpoint on missing model = true, want false")
	}
}

func TestWailsTraining_HasCheckpoint_PresentReturnsTrue_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if r := SaveCheckpoint(Checkpoint{Model: "trained-once", StartedAt: 1}); !r.OK {
		t.Fatalf("SaveCheckpoint: %v", r.Error())
	}
	w := NewWailsService(core.New(), NewService(core.New(), Options{}))
	r := w.HasCheckpoint("trained-once")
	if !r.OK {
		t.Fatalf("HasCheckpoint: %v", r.Value)
	}
	if r.Value.(bool) != true {
		t.Errorf("HasCheckpoint on saved model = false, want true")
	}
}

func TestWailsTraining_GetCheckpoint_RoundTrip_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cp := Checkpoint{
		Model:        "round-trip",
		Substrate:    "CONT",
		Tier:         1,
		Subjects:     []string{"english", "european"},
		SubjectIndex: 1,
		ProbeIndex:   3,
		StartedAt:    1747728000,
	}
	if r := SaveCheckpoint(cp); !r.OK {
		t.Fatalf("SaveCheckpoint: %v", r.Error())
	}
	w := NewWailsService(core.New(), NewService(core.New(), Options{}))
	r := w.GetCheckpoint("round-trip")
	if !r.OK {
		t.Fatalf("GetCheckpoint: %v", r.Value)
	}
	got := r.Value.(*Checkpoint)
	if got == nil {
		t.Fatal("GetCheckpoint value = nil, want non-nil")
	}
	if got.Substrate != "CONT" || got.Tier != 1 || got.SubjectIndex != 1 {
		t.Errorf("GetCheckpoint round-trip mismatch: got %+v", got)
	}
}

func TestWailsTraining_DiscardCheckpoint_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if r := SaveCheckpoint(Checkpoint{Model: "discard-me", StartedAt: 1}); !r.OK {
		t.Fatalf("SaveCheckpoint: %v", r.Error())
	}
	w := NewWailsService(core.New(), NewService(core.New(), Options{}))

	if r := w.DiscardCheckpoint("discard-me"); !r.OK {
		t.Fatalf("DiscardCheckpoint: %v", r.Value)
	}

	has := w.HasCheckpoint("discard-me")
	if !has.OK || has.Value.(bool) != false {
		t.Errorf("post-Discard HasCheckpoint = %+v, want OK+false", has)
	}
}

func TestWailsTraining_DiscardCheckpoint_IdempotentOnMissing_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	w := NewWailsService(core.New(), NewService(core.New(), Options{}))
	// Two discards on a model that was never saved — both succeed.
	for i := 0; i < 2; i++ {
		if r := w.DiscardCheckpoint("never-saved"); !r.OK {
			t.Errorf("DiscardCheckpoint #%d: %v", i, r.Value)
		}
	}
}

func TestWailsTraining_ResumeFixture_MissingCheckpoint_Bad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	w := newWailsHarness(t, []string{"english"})
	if r := w.ResumeFixture("never-saved", "CONT", 0); r.OK {
		t.Error("ResumeFixture with no saved checkpoint returned OK, want Fail")
	}
}

func TestWailsTraining_ResumeFixture_EmptyModel_Bad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	w := newWailsHarness(t, []string{"english"})
	if r := w.ResumeFixture("", "CONT", 0); r.OK {
		t.Error("ResumeFixture with empty model returned OK, want Fail")
	}
}

func TestWailsTraining_ResumeFixture_DrivesResumeAndClears_Good(t *testing.T) {
	w := newWailsHarness(t, []string{"english", "european"})

	// Save a checkpoint marking english as groked — resume should skip
	// english + only run european.
	cp := Checkpoint{
		SchemaVersion:  CheckpointSchemaVersion,
		Model:          "fixture-wresume",
		Substrate:      "CONT",
		Tier:           0,
		Subjects:       []string{"english", "european"},
		SubjectIndex:   1,
		GrokedSubjects: []string{"english"},
		StartedAt:      core.UnixNow() - 60,
	}
	if r := SaveCheckpoint(cp); !r.OK {
		t.Fatalf("SaveCheckpoint: %v", r.Error())
	}

	if r := w.ResumeFixture("fixture-wresume", "CONT", 0); !r.OK {
		t.Fatalf("ResumeFixture: %v", r.Value)
	}
	// Wait for the resume goroutine to drain.
	waitFor(t, func() bool { return !w.IsRunning().Value.(bool) })

	// Clean completion clears the checkpoint.
	has := w.HasCheckpoint("fixture-wresume")
	if !has.OK {
		t.Fatalf("HasCheckpoint: %v", has.Value)
	}
	if has.Value.(bool) != false {
		t.Errorf("post-ResumeFixture HasCheckpoint = true, want false (clean completion clears)")
	}
}

func TestWailsTraining_ResumeFixture_ConcurrentRotation_Bad(t *testing.T) {
	w := newWailsHarness(t, []string{"english"})

	// Save a checkpoint so ResumeFixture has something to load.
	if r := SaveCheckpoint(Checkpoint{
		SchemaVersion: CheckpointSchemaVersion,
		Model:         "fixture-busy",
		Substrate:     "CONT",
		Subjects:      []string{"english"},
		StartedAt:     1,
	}); !r.OK {
		t.Fatalf("SaveCheckpoint: %v", r.Error())
	}

	// Start a long-running fixture rotation first.
	if r := w.StartFixture("fixture-busy", "CONT", 0); !r.OK {
		t.Fatalf("StartFixture: %v", r.Value)
	}
	t.Cleanup(func() {
		_ = w.Stop()
		waitFor(t, func() bool { return !w.IsRunning().Value.(bool) })
	})
	waitFor(t, func() bool { return w.IsRunning().Value.(bool) })

	// ResumeFixture must fail-fast while one is in flight.
	if r := w.ResumeFixture("fixture-busy", "CONT", 0); r.OK {
		t.Error("ResumeFixture while running returned OK, want Fail")
	}
}
