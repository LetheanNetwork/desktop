// SPDX-Licence-Identifier: EUPL-1.2

package training

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	core "dappco.re/go"

	"dappco.re/lthn/desktop/pkg/paths"
)

// tempCheckpointHome repoints $HOME at a per-test tempdir so
// paths.TrainingCheckpointDir() resolves under an isolated root.
// Mirrors the pattern in store_test.go and analytics tests.
func tempCheckpointHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	return dir
}

// --- SaveCheckpoint / LoadCheckpoint roundtrip ---

func TestCheckpoint_SaveLoad_RoundTrip_Good(t *testing.T) {
	tempCheckpointHome(t)

	cp := Checkpoint{
		Model:     "gemma4-e2b-it-q4",
		Substrate: "CONT",
		Tier:      0,
		Subjects:  []string{"english", "european", "russian"},
		SubjectIndex:  1,
		ProbeIndex:    4,
		GrokedSubjects: []string{"english"},
		CompletedProbes: []ProbeKey{
			{Subject: "english", ProbeID: "P01"},
			{Subject: "english", ProbeID: "P02"},
		},
		StartedAt: 1747728000,
	}

	if r := SaveCheckpoint(cp); !r.OK {
		t.Fatalf("SaveCheckpoint: %v", r.Error())
	}

	r := LoadCheckpoint(cp.Model)
	if !r.OK {
		t.Fatalf("LoadCheckpoint: %v", r.Error())
	}
	got, ok := r.Value.(*Checkpoint)
	if !ok || got == nil {
		t.Fatalf("LoadCheckpoint value = %T(%v), want *Checkpoint", r.Value, r.Value)
	}
	if got.Model != cp.Model {
		t.Errorf("Model = %q, want %q", got.Model, cp.Model)
	}
	if got.Substrate != cp.Substrate {
		t.Errorf("Substrate = %q, want %q", got.Substrate, cp.Substrate)
	}
	if got.Tier != cp.Tier {
		t.Errorf("Tier = %d, want %d", got.Tier, cp.Tier)
	}
	if len(got.Subjects) != 3 || got.Subjects[2] != "russian" {
		t.Errorf("Subjects = %v, want [english european russian]", got.Subjects)
	}
	if got.SubjectIndex != 1 || got.ProbeIndex != 4 {
		t.Errorf("indices = (%d, %d), want (1, 4)", got.SubjectIndex, got.ProbeIndex)
	}
	if len(got.GrokedSubjects) != 1 || got.GrokedSubjects[0] != "english" {
		t.Errorf("GrokedSubjects = %v, want [english]", got.GrokedSubjects)
	}
	if len(got.CompletedProbes) != 2 {
		t.Errorf("CompletedProbes len = %d, want 2", len(got.CompletedProbes))
	}
	if got.StartedAt != 1747728000 {
		t.Errorf("StartedAt = %d, want 1747728000", got.StartedAt)
	}
}

func TestCheckpoint_SaveStampsSchemaVersionAndSavedAt_Good(t *testing.T) {
	tempCheckpointHome(t)

	// Caller passes zero values for SchemaVersion + SavedAt — Save
	// stamps them defensively so callers don't have to remember.
	cp := Checkpoint{Model: "gemma4-e2b-it-q4", StartedAt: 1747728000}
	if r := SaveCheckpoint(cp); !r.OK {
		t.Fatalf("SaveCheckpoint: %v", r.Error())
	}
	before := core.UnixNow()

	r := LoadCheckpoint(cp.Model)
	if !r.OK {
		t.Fatalf("LoadCheckpoint: %v", r.Error())
	}
	got := r.Value.(*Checkpoint)
	if got.SchemaVersion != CheckpointSchemaVersion {
		t.Errorf("SchemaVersion = %d, want %d", got.SchemaVersion, CheckpointSchemaVersion)
	}
	// SavedAt should be within a small window of "now".
	if got.SavedAt > before+5 || got.SavedAt < before-5 {
		t.Errorf("SavedAt = %d, want ~%d", got.SavedAt, before)
	}
	// StartedAt is preserved (not overwritten by Save).
	if got.StartedAt != 1747728000 {
		t.Errorf("StartedAt = %d, want 1747728000 (Save must not overwrite)", got.StartedAt)
	}
}

// --- LoadCheckpoint absence ---

func TestCheckpoint_Load_MissingFileReturnsOkNil_Good(t *testing.T) {
	tempCheckpointHome(t)

	r := LoadCheckpoint("gemma4-e2b-it-q4")
	if !r.OK {
		t.Fatalf("LoadCheckpoint on missing file should succeed, got: %v", r.Error())
	}
	cp, ok := r.Value.(*Checkpoint)
	if !ok {
		t.Fatalf("Value type = %T, want *Checkpoint", r.Value)
	}
	if cp != nil {
		t.Errorf("missing-file Value should be nil, got %+v", cp)
	}
}

// --- ClearCheckpoint ---

func TestCheckpoint_Clear_RemovesFile_Good(t *testing.T) {
	tempCheckpointHome(t)

	cp := Checkpoint{Model: "gemma4-e2b-it-q4", StartedAt: 1}
	if r := SaveCheckpoint(cp); !r.OK {
		t.Fatalf("SaveCheckpoint: %v", r.Error())
	}
	// Confirm Load sees it first.
	if r := LoadCheckpoint(cp.Model); !r.OK || r.Value.(*Checkpoint) == nil {
		t.Fatalf("pre-Clear: LoadCheckpoint should return a non-nil checkpoint")
	}

	if r := ClearCheckpoint(cp.Model); !r.OK {
		t.Fatalf("ClearCheckpoint: %v", r.Error())
	}

	r := LoadCheckpoint(cp.Model)
	if !r.OK {
		t.Fatalf("post-Clear Load: %v", r.Error())
	}
	if r.Value.(*Checkpoint) != nil {
		t.Errorf("post-Clear Load should return nil, got %+v", r.Value)
	}
}

func TestCheckpoint_Clear_IdempotentOnMissingFile_Good(t *testing.T) {
	tempCheckpointHome(t)

	// First clear — nothing to clear, must not Fail.
	if r := ClearCheckpoint("never-saved-model"); !r.OK {
		t.Fatalf("first Clear on missing file should succeed, got: %v", r.Error())
	}
	// Second clear — still missing, still ok.
	if r := ClearCheckpoint("never-saved-model"); !r.OK {
		t.Fatalf("second Clear on missing file should succeed, got: %v", r.Error())
	}
}

// --- SaveCheckpoint overwrite ---

func TestCheckpoint_Save_OverwritesExisting_Good(t *testing.T) {
	tempCheckpointHome(t)

	first := Checkpoint{
		Model: "gemma4-e2b-it-q4", Substrate: "TRAD", Tier: 0,
		SubjectIndex: 0, ProbeIndex: 1,
		StartedAt: 1747728000,
	}
	if r := SaveCheckpoint(first); !r.OK {
		t.Fatalf("first SaveCheckpoint: %v", r.Error())
	}

	second := Checkpoint{
		Model: "gemma4-e2b-it-q4", Substrate: "CONT", Tier: 0,
		SubjectIndex: 2, ProbeIndex: 7,
		StartedAt: 1747728000,
	}
	if r := SaveCheckpoint(second); !r.OK {
		t.Fatalf("second SaveCheckpoint (overwrite): %v", r.Error())
	}

	r := LoadCheckpoint("gemma4-e2b-it-q4")
	if !r.OK {
		t.Fatalf("LoadCheckpoint: %v", r.Error())
	}
	got := r.Value.(*Checkpoint)
	if got.Substrate != "CONT" || got.SubjectIndex != 2 || got.ProbeIndex != 7 {
		t.Errorf("overwrite failed: got %+v, want substrate=CONT, idx=(2,7)", got)
	}
}

// --- Schema version gate ---

func TestCheckpoint_Load_SchemaMismatchFails_Bad(t *testing.T) {
	tempCheckpointHome(t)

	// Hand-write a checkpoint with a future schema version directly to
	// the canonical path, bypassing SaveCheckpoint (which would re-stamp
	// the version to the current one).
	rootR := paths.TrainingCheckpointDir()
	if !rootR.OK {
		t.Fatalf("TrainingCheckpointDir: %v", rootR.Error())
	}
	target := filepath.Join(rootR.Value.(string), "gemma4-e2b-it-q4.json")

	body := []byte(`{"schema_version":999,"model":"gemma4-e2b-it-q4","substrate":"CONT","tier":0,"subjects":[],"subject_index":0,"probe_index":0,"started_at":1,"saved_at":2}`)
	if err := os.WriteFile(target, body, 0o600); err != nil {
		t.Fatalf("seed write: %v", err)
	}

	if r := LoadCheckpoint("gemma4-e2b-it-q4"); r.OK {
		t.Error("LoadCheckpoint with schema_version=999 should Fail, got OK")
	}
}

// --- Input validation ---

func TestCheckpoint_Save_EmptyModelFails_Bad(t *testing.T) {
	tempCheckpointHome(t)
	if r := SaveCheckpoint(Checkpoint{Model: ""}); r.OK {
		t.Error("SaveCheckpoint with empty Model should Fail, got OK")
	}
}

func TestCheckpoint_Load_EmptyModelFails_Bad(t *testing.T) {
	tempCheckpointHome(t)
	if r := LoadCheckpoint(""); r.OK {
		t.Error("LoadCheckpoint with empty model should Fail, got OK")
	}
}

func TestCheckpoint_Clear_EmptyModelFails_Bad(t *testing.T) {
	tempCheckpointHome(t)
	if r := ClearCheckpoint(""); r.OK {
		t.Error("ClearCheckpoint with empty model should Fail, got OK")
	}
}

// --- Atomic write under concurrent Save ---

// TestCheckpoint_ConcurrentSave_NoTornReads_Ugly fires N goroutines
// racing to Save against the same model file. After all goroutines
// drain, Load must succeed and the file must decode cleanly to ONE
// of the values written (not a torn JSON, not a corrupt payload).
// Demonstrates the AtomicWriteWithVersion + tmp + rename serialisation
// the primitive provides under the file-lock substrate.
func TestCheckpoint_ConcurrentSave_NoTornReads_Ugly(t *testing.T) {
	tempCheckpointHome(t)

	const writers = 8
	var wg sync.WaitGroup
	wg.Add(writers)
	for i := 0; i < writers; i++ {
		go func(idx int) {
			defer wg.Done()
			cp := Checkpoint{
				Model:        "gemma4-e2b-it-q4",
				Substrate:    "CONT",
				Tier:         0,
				Subjects:     []string{"english"},
				SubjectIndex: idx,
				ProbeIndex:   idx * 10,
				StartedAt:    1747728000,
			}
			if r := SaveCheckpoint(cp); !r.OK {
				t.Errorf("concurrent SaveCheckpoint[%d]: %v", idx, r.Error())
			}
		}(i)
	}
	wg.Wait()

	r := LoadCheckpoint("gemma4-e2b-it-q4")
	if !r.OK {
		t.Fatalf("post-race LoadCheckpoint must decode cleanly: %v", r.Error())
	}
	got := r.Value.(*Checkpoint)
	if got == nil {
		t.Fatal("post-race Load returned nil")
	}
	if got.SubjectIndex < 0 || got.SubjectIndex >= writers {
		t.Errorf("post-race SubjectIndex = %d, want one of 0..%d", got.SubjectIndex, writers-1)
	}
	// Sanity — SchemaVersion is stamped, SavedAt is non-zero.
	if got.SchemaVersion != CheckpointSchemaVersion {
		t.Errorf("post-race SchemaVersion = %d, want %d", got.SchemaVersion, CheckpointSchemaVersion)
	}
	if got.SavedAt == 0 {
		t.Error("post-race SavedAt = 0, want stamped")
	}
}
