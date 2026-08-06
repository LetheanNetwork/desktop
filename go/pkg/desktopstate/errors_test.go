// SPDX-License-Identifier: EUPL-1.2

package desktopstate

import (
	core "dappco.re/go"
)

func TestFailure_Error_Good(t *core.T) {
	withOperation := &Failure{
		Operation: "desktopstate.Save",
		Message:   "revision is stale",
	}
	core.AssertEqual(
		t,
		"desktopstate.Save: revision is stale",
		withOperation.Error(),
	)

	bare := &Failure{Message: "revision is stale"}
	core.AssertEqual(t, "revision is stale", bare.Error())
}

func TestFailure_Error_UglyNilReceiverReturnsEmptyString(t *core.T) {
	var nilFailure *Failure

	core.AssertEqual(t, "", nilFailure.Error())
}

func TestFailure_Unwrap_Good(t *core.T) {
	cause := core.E("test", "boom", nil)
	failure := &Failure{Cause: cause}

	core.AssertSame(t, cause, failure.Unwrap())
}

func TestFailure_Unwrap_UglyNilReceiverReturnsNil(t *core.T) {
	var nilFailure *Failure

	core.AssertNoError(t, nilFailure.Unwrap())
}

func TestErrorCodeOf_GoodReturnsCarriedCode(t *core.T) {
	result := stateFailure(
		ErrorStateConflict,
		"desktopstate.Save",
		"revision is stale",
		nil,
	)

	core.AssertEqual(t, ErrorStateConflict, ErrorCodeOf(result))
}

func TestErrorCodeOf_BadReturnsEmptyForOKResult(t *core.T) {
	core.AssertEqual(t, ErrorCode(""), ErrorCodeOf(core.Ok(nil)))
}

func TestErrorCodeOf_UglyReturnsEmptyForForeignErrorType(t *core.T) {
	result := core.Fail(core.E("test", "opaque failure", nil))

	core.AssertEqual(t, ErrorCode(""), ErrorCodeOf(result))
}
