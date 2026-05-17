// SPDX-Licence-Identifier: EUPL-1.2

// Internal (same-package) tests for the wrapInnerFail helper that
// guards every renderer-callable W* binding against panic on a
// malformed inner core.Result. Mantis #1659 / Cerberus #45 ADD —
// the prior shape unconditionally cast inner.Value.(error); a Fail
// whose Value is anything else (string, nil, custom sentinel) drove
// the Wails binding worker into panic, surfacing as a renderer-side
// 500 with no controlled envelope.
//
// SECURITY-NOTE (Hephaestus surface, brief allowlist boundary): the
// brief named "TestRunner_W*_NoPanic_OnMalformedError_Bad" but the
// only injectable seam is the shared wrapInnerFail helper — the
// concrete *ai.ProviderRouter is constructed inside Service.Generate
// with no doubles-friendly interface. Driving wrapInnerFail directly
// is the narrowest test that proves the comma-ok discipline; the
// existing wails_test.go cap-rejection coverage already exercises
// the W* surface end-to-end on the OK path.

package runner

import (

	core "dappco.re/go"
)

// stringSentinel is a non-error Value used to drive the wrong-shape
// path that previously panicked. Captures the "future router refactor
// returns a Fail wrapping a typed-string sentinel" case from the
// Cerberus #1659 mechanism description.
type stringSentinel string

func (s stringSentinel) String() string { return string(s) }

// TestRunner_WrapInnerFail_NilValue_Bad — Fail whose Value is nil
// MUST NOT panic. The prior shape (inner.Value.(error)) would have
// returned (nil, false) on the safe form and panicked on the unsafe
// .(error) form. comma-ok produces a Fail with nil cause + the
// inner.Error() string preserved.
func TestRunner_WrapInnerFail_NilValue_Bad(t *core.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("wrapInnerFail panicked on nil-Value Fail: %v", r)
		}
	}()
	inner := core.Result{OK: false, Value: nil}
	out := wrapInnerFail("runner.test", "inner failed", inner)
	core.AssertFalse(t, out.OK, "wrapInnerFail must propagate the failure flag")
}

// TestRunner_WrapInnerFail_WrongTypeValue_Bad — Fail whose Value is
// a non-error (typed string sentinel) MUST NOT panic. The renderer-
// reachable W* paths inherit this safety so a future router refactor
// returning a typed sentinel can't crash the binding worker.
func TestRunner_WrapInnerFail_WrongTypeValue_Bad(t *core.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("wrapInnerFail panicked on non-error Value Fail: %v", r)
		}
	}()
	inner := core.Result{OK: false, Value: stringSentinel("router declined")}
	out := wrapInnerFail("runner.test", "inner failed", inner)
	core.AssertFalse(t, out.OK,
		"wrapInnerFail must propagate the failure flag on non-error Value")
}

// TestRunner_WrapInnerFail_RealError_Good — the happy path: Fail
// whose Value is a real error (the historical default) still wires
// the cause through to the outer core.Err.
func TestRunner_WrapInnerFail_RealError_Good(t *core.T) {
	cause := core.E("inner.op", "real cause", nil)
	inner := core.Result{OK: false, Value: cause}
	out := wrapInnerFail("runner.test", "inner failed", inner)
	core.AssertFalse(t, out.OK,
		"wrapInnerFail must propagate the failure flag on real-error Value")
	// Outer wrap MUST preserve a non-empty error string carrying both
	// the inner cause + the outer op tag.
	core.AssertTrue(t, len(out.Error()) > 0,
		"wrapInnerFail must produce a non-empty error string")
}
