// SPDX-License-Identifier: EUPL-1.2

package files

import (
	"io/fs"

	core "dappco.re/go"
)

func TestFailure_Error_GoodMessageAndBareCode(t *core.T) {
	withMessage := newFailure(
		ErrorInvalidInput,
		"documents",
		"a.txt",
		"the path is invalid",
		nil,
	)
	core.AssertEqual(
		t,
		"files.invalid_input: the path is invalid",
		withMessage.Error(),
	)

	bare := &Failure{Code: ErrorMissingEntry}
	core.AssertEqual(t, string(ErrorMissingEntry), bare.Error())
}

func TestFailure_Error_UglyNilReceiverReturnsProviderUnavailable(
	t *core.T,
) {
	var nilFailure *Failure

	core.AssertEqual(t, string(ErrorProviderUnavailable), nilFailure.Error())
}

func TestFailure_Unwrap_GoodReturnsCauseAndNilForNilReceiver(t *core.T) {
	cause := fs.ErrPermission
	failure := newFailure(
		ErrorCapabilityDenied,
		"documents",
		"a.txt",
		"denied",
		cause,
	)

	core.AssertSame(t, cause, failure.Unwrap())
	core.AssertTrue(t, core.Is(failure, fs.ErrPermission))

	var nilFailure *Failure
	core.AssertNoError(t, nilFailure.Unwrap())
}
