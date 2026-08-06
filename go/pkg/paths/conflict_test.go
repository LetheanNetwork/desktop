// SPDX-Licence-Identifier: EUPL-1.2

// Tests for paths.ConflictEnvelope — the wire-stable conflict shape
// (Mantis #1544). Zero coverage before this file: nothing in the
// existing atomic-write / cascade-writer suites happened to construct
// or introspect the envelope type directly.

package paths_test

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/paths"
)

func TestConflict_NewConflictEnvelope_Good(t *core.T) {
	stale := paths.VersionStale{
		CurrentVersion: 7,
		CurrentHash:    "deadbeef",
	}
	env := paths.NewConflictEnvelope("deals.update.conflict", stale)
	core.AssertEqual(t, "deals.update.conflict", env.Code)
	core.AssertEqual(t, 7, env.CurrentVersion)
	core.AssertEqual(t, "deadbeef", env.CurrentHash)
}

func TestConflict_Error_Good_ReturnsCode(t *core.T) {
	env := paths.ConflictEnvelope{Code: "contacts.update.conflict"}
	var err error = env
	core.AssertEqual(t, "contacts.update.conflict", err.Error())
}

func TestConflict_ConflictEnvelopeFrom_Good_ValueType(t *core.T) {
	env := paths.NewConflictEnvelope("pipeline.update.conflict", paths.VersionStale{CurrentVersion: 3})
	got, ok := paths.ConflictEnvelopeFrom(env)
	core.AssertTrue(t, ok, "a ConflictEnvelope value must extract")
	core.AssertEqual(t, "pipeline.update.conflict", got.Code)
}

func TestConflict_ConflictEnvelopeFrom_Good_PointerType(t *core.T) {
	env := paths.NewConflictEnvelope("deals.update.conflict", paths.VersionStale{CurrentVersion: 9})
	got, ok := paths.ConflictEnvelopeFrom(&env)
	core.AssertTrue(t, ok, "a *ConflictEnvelope pointer must extract")
	core.AssertEqual(t, 9, got.CurrentVersion)
}

func TestConflict_ConflictEnvelopeFrom_Bad_NilPointer(t *core.T) {
	var env *paths.ConflictEnvelope
	_, ok := paths.ConflictEnvelopeFrom(env)
	core.AssertFalse(t, ok, "a nil *ConflictEnvelope must not extract")
}

func TestConflict_ConflictEnvelopeFrom_Bad_UnrelatedType(t *core.T) {
	_, ok := paths.ConflictEnvelopeFrom("not-an-envelope")
	core.AssertFalse(t, ok, "an unrelated value type must not extract")
}
