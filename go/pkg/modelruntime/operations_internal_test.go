// SPDX-Licence-Identifier: EUPL-1.2

// operations_internal_test.go — direct cover for operations.go's pure
// helper functions (readinessDelay, mutationFailureMessage,
// mapClientResult) that the fixture-driven scenarios in service_test.go
// only ever exercise through one or two switch arms in passing.

package modelruntime

import (
	"time"

	core "dappco.re/go"
)

func TestReadinessDelay_ExponentialGrowth_Good(t *core.T) {
	core.AssertEqual(t, 50*time.Millisecond, readinessDelay(0))
	core.AssertEqual(t, 100*time.Millisecond, readinessDelay(1))
	core.AssertEqual(t, 200*time.Millisecond, readinessDelay(2))
	core.AssertEqual(t, 400*time.Millisecond, readinessDelay(3))
}

// TestReadinessDelay_CapsAtMaximum_Good — attempt counts high enough
// that doubling would exceed the 6400ms ceiling must clamp there
// rather than overflowing.
func TestReadinessDelay_CapsAtMaximum_Good(t *core.T) {
	core.AssertEqual(t, 6400*time.Millisecond, readinessDelay(20))
	core.AssertEqual(t, 6400*time.Millisecond, readinessDelay(maxReadinessAttempts))
}

func TestMutationFailureMessage_AllBranches_Good(t *core.T) {
	cases := map[ErrorCode]string{
		ErrorModelNotFound:        "The selected model is unavailable.",
		ErrorModelNotLoadable:     "The selected model cannot be loaded locally.",
		ErrorAdminUnauthorised:    "LEM rejected the local admin credential.",
		ErrorOperationInProgress:  "Another model-runtime operation is already running.",
		ErrorRuntimeStopFailed:    "The LEM runtime could not be stopped.",
		ErrorModelUnloadFailed:    "The loaded model could not be unloaded.",
		ErrorModelLoadFailed:      "The selected model could not be loaded.",
		ErrorCatalogueUnavailable: "The local model catalogue is unavailable.",
	}
	for code, want := range cases {
		core.AssertEqual(t, want, mutationFailureMessage(code))
	}
}

func TestMutationFailureMessage_UnknownCode_DefaultsToGenericMessage_Bad(t *core.T) {
	core.AssertEqual(t,
		"The LEM runtime operation could not be completed.",
		mutationFailureMessage(ErrorCode("something-unmapped")))
}

func TestMapClientResult_OKPassesThrough_Good(t *core.T) {
	ok := core.Ok("value")
	result := mapClientResult("op", ok)
	core.AssertTrue(t, result.OK)
	core.AssertEqual(t, "value", result.Value)
}

func TestMapClientResult_UnauthorisedMapsToAdminMessage_Bad(t *core.T) {
	failed := core.Fail(&ClientFailure{Code: ClientUnauthorised, Message: "denied"})
	result := mapClientResult("op", failed)
	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ErrorAdminUnauthorised, ErrorCodeOf(result))
	core.AssertContains(t, result.Error(), "LEM rejected the local admin credential.")
}

func TestMapClientResult_UnmappedClientCodeFallsBackToNotReady_Bad(t *core.T) {
	// A ClientFailure with an empty/unknown code maps through
	// mapClientCode to "" then falls back to ErrorRuntimeNotReady.
	failed := core.Fail(&ClientFailure{Code: ClientErrorCode("bogus"), Message: "x"})
	result := mapClientResult("op", failed)
	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ErrorRuntimeNotReady, ErrorCodeOf(result))
}

func TestMapClientResult_ForeignErrorFallsBackToNotReady_Bad(t *core.T) {
	failed := core.Fail(core.NewError("unrelated"))
	result := mapClientResult("op", failed)
	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ErrorRuntimeNotReady, ErrorCodeOf(result))
	core.AssertContains(t, result.Error(), "The LEM runtime request failed.")
}
