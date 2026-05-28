// SPDX-Licence-Identifier: EUPL-1.2

package training

import (
	core "dappco.re/go"

	"dappco.re/lthn/desktop/pkg/paths"
)

// SaveCheckpoint writes cp to disk at
// paths.TrainingCheckpointDir()/<cp.Model>.json via the
// paths.AtomicWriteWithVersion primitive — tmp + fsync + rename so a
// crash mid-write leaves the previous valid checkpoint intact and the
// reader sees either old or new bytes (never a torn JSON).
//
// Stamps cp.SchemaVersion + cp.SavedAt defensively before writing so
// callers passing literal Checkpoints don't have to remember either
// field. cp.StartedAt is left untouched — the caller stamps it on
// the first save of a rotation and Save preserves it on subsequent
// writes for accurate "rotation started" UX.
//
// Fails when cp.Model is empty (no filename), when the checkpoint
// dir resolution fails (paths.DataDir / mkdir / etc.), or when the
// underlying atomic write fails.
//
// Usage example:
//
//	cp := training.Checkpoint{
//	    Model: "gemma4-e2b-it-q4", Substrate: "CONT", Tier: 0,
//	    Subjects: subjects, SubjectIndex: 1, ProbeIndex: 4,
//	    GrokedSubjects: []string{"english"},
//	    StartedAt: startedAt,
//	}
//	if r := training.SaveCheckpoint(cp); !r.OK { core.Warn("ckpt", "err", r.Error()) }
func SaveCheckpoint(cp Checkpoint) core.Result {
	if cp.Model == "" {
		return core.Fail(core.E("training.SaveCheckpoint", "Model is required", nil))
	}
	cp.SchemaVersion = CheckpointSchemaVersion
	cp.SavedAt = core.UnixNow()

	pathR := checkpointPath(cp.Model)
	if !pathR.OK {
		return pathR
	}
	target := pathR.Value.(string)

	encoded := core.JSONMarshalIndent(cp, "", "  ")
	if !encoded.OK {
		return encoded
	}
	body, _ := encoded.Value.([]byte)

	wr := paths.AtomicWriteWithVersion(target, paths.WriteInput{Body: body})
	if !wr.OK {
		return wr
	}
	return core.Ok(target)
}

// LoadCheckpoint reads the checkpoint for the given model. Returns
// core.Ok(nil) when no checkpoint exists for this model — that's a
// legitimate state (fresh rotation, post-Clear, or an operator that
// never started training this model). Returns core.Ok(*Checkpoint)
// when one is found, or core.Fail on read / decode / schema-version
// mismatch.
//
// The pointer-vs-nil shape lets the caller branch on presence
// without inspecting Result.OK for two different meanings.
//
// Usage example:
//
//	r := training.LoadCheckpoint("gemma4-e2b-it-q4")
//	if !r.OK { return r }
//	if cp, ok := r.Value.(*training.Checkpoint); ok && cp != nil {
//	    // resume from cp
//	} else {
//	    // fresh start
//	}
func LoadCheckpoint(model string) core.Result {
	if model == "" {
		return core.Fail(core.E("training.LoadCheckpoint", "model is required", nil))
	}
	pathR := checkpointPath(model)
	if !pathR.OK {
		return pathR
	}
	target := pathR.Value.(string)

	if !core.Stat(target).OK {
		return core.Ok((*Checkpoint)(nil))
	}

	body := core.ReadFile(target)
	if !body.OK {
		return body
	}
	var cp Checkpoint
	if r := core.JSONUnmarshal(body.Value.([]byte), &cp); !r.OK {
		return core.Fail(core.E("training.LoadCheckpoint",
			"decode failed: "+r.Error(), nil))
	}
	if cp.SchemaVersion != CheckpointSchemaVersion {
		return core.Fail(core.E("training.LoadCheckpoint",
			core.Sprintf("unknown schema_version=%d (current=%d)",
				cp.SchemaVersion, CheckpointSchemaVersion),
			nil))
	}
	return core.Ok(&cp)
}

// ClearCheckpoint removes the checkpoint file for the given model.
// Idempotent — a missing file is not an error (this is the post-
// clean-run state where the caller wants to ensure no resume happens
// on next startup).
//
// Called automatically by Service.Run on clean rotation completion.
// Operator-side flows (manual "discard checkpoint" UX) call it
// directly.
//
// Usage example:
//
//	if r := training.ClearCheckpoint("gemma4-e2b-it-q4"); !r.OK { return r }
func ClearCheckpoint(model string) core.Result {
	if model == "" {
		return core.Fail(core.E("training.ClearCheckpoint", "model is required", nil))
	}
	pathR := checkpointPath(model)
	if !pathR.OK {
		return pathR
	}
	target := pathR.Value.(string)

	if !core.Stat(target).OK {
		return core.Ok(nil)
	}
	if r := core.Remove(target); !r.OK {
		return r
	}
	return core.Ok(nil)
}

// checkpointPath resolves the canonical
// paths.TrainingCheckpointDir()/<model>.json target for one model.
// Centralised so Save / Load / Clear all use the same naming rule —
// rename the model and a future caller can't accidentally diverge
// the path-builders.
func checkpointPath(model string) core.Result {
	rootR := paths.TrainingCheckpointDir()
	if !rootR.OK {
		return rootR
	}
	return core.Ok(core.PathJoin(rootR.Value.(string), model+".json"))
}
