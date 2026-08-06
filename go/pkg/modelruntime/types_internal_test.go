// SPDX-Licence-Identifier: EUPL-1.2

// types_internal_test.go — direct cover for types.go's Failure/
// FailureView/ErrorCodeOf machinery. The other tests in this package
// exercise these only incidentally (through a mutation's failure
// path), so the accessor methods themselves — the ones actually
// implementing the `error` interface contract — had never been called
// directly and several branches (nil receiver, the Operation-less
// message shape, an OK result, a foreign error type) were dark.

package modelruntime

import core "dappco.re/go"

func TestFailure_Error_WithOperation_Good(t *core.T) {
	failure := &Failure{Operation: "modelruntime.Service.Load", Message: "boom"}
	core.AssertEqual(t, "modelruntime.Service.Load: boom", failure.Error())
}

func TestFailure_Error_WithoutOperation_Good(t *core.T) {
	failure := &Failure{Message: "boom"}
	core.AssertEqual(t, "boom", failure.Error())
}

func TestFailure_Error_NilReceiver_Bad(t *core.T) {
	var failure *Failure
	core.AssertEqual(t, "", failure.Error())
}

func TestFailure_Unwrap_Good(t *core.T) {
	cause := core.NewError("root cause")
	failure := &Failure{Cause: cause}
	core.AssertEqual(t, cause, failure.Unwrap())
}

func TestFailure_Unwrap_NilReceiver_Bad(t *core.T) {
	var failure *Failure
	core.AssertTrue(t, failure.Unwrap() == nil)
}

func TestErrorCodeOf_OKResult_Good(t *core.T) {
	core.AssertEqual(t, ErrorCode(""), ErrorCodeOf(core.Ok(nil)))
}

func TestErrorCodeOf_ForeignError_Bad(t *core.T) {
	foreign := core.Fail(core.NewError("unrelated"))
	core.AssertEqual(t, ErrorCode(""), ErrorCodeOf(foreign))
}

// TestFailureView_UsesFallbackWhenResultUnclassified_Bad — a Fail
// result wrapping a non-*Failure error has no extractable code, so
// failureView must fall back to the caller-supplied ErrorCode instead
// of leaving Code empty.
func TestFailureView_UsesFallbackWhenResultUnclassified_Bad(t *core.T) {
	view := failureView(core.Fail(core.NewError("x")), ErrorRuntimeUnavailable, "message text")
	core.RequireTrue(t, view != nil)
	core.AssertEqual(t, ErrorRuntimeUnavailable, view.Code)
	core.AssertEqual(t, "message text", view.Message)
}

// TestFailureView_UsesResultCodeWhenClassified_Good — when the result
// DOES wrap a *Failure, its own code wins over the fallback.
func TestFailureView_UsesResultCodeWhenClassified_Good(t *core.T) {
	result := fail(ErrorModelNotFound, "op", "not found", nil)
	view := failureView(result, ErrorRuntimeUnavailable, "message text")
	core.RequireTrue(t, view != nil)
	core.AssertEqual(t, ErrorModelNotFound, view.Code)
}
