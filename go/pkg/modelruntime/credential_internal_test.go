// SPDX-Licence-Identifier: EUPL-1.2

// credential_internal_test.go — direct cover for CredentialFailure's
// error-interface methods and CredentialErrorCodeOf's OK/foreign-error
// branches, plus Invalidate on a nil receiver — none of which
// credential_test.go's Medium-driven scenarios reach directly.

package modelruntime

import core "dappco.re/go"

func TestCredentialFailure_Error_Good(t *core.T) {
	failure := &CredentialFailure{Message: "boom"}
	core.AssertEqual(t, "boom", failure.Error())
}

func TestCredentialFailure_Error_NilReceiver_Bad(t *core.T) {
	var failure *CredentialFailure
	core.AssertEqual(t, "", failure.Error())
}

func TestCredentialFailure_Unwrap_Good(t *core.T) {
	cause := core.NewError("root cause")
	failure := &CredentialFailure{Cause: cause}
	core.AssertEqual(t, cause, failure.Unwrap())
}

func TestCredentialFailure_Unwrap_NilReceiver_Bad(t *core.T) {
	var failure *CredentialFailure
	core.AssertTrue(t, failure.Unwrap() == nil)
}

func TestCredentialErrorCodeOf_OKResult_Good(t *core.T) {
	core.AssertEqual(t, CredentialErrorCode(""), CredentialErrorCodeOf(core.Ok(nil)))
}

func TestCredentialErrorCodeOf_ForeignError_Bad(t *core.T) {
	foreign := core.Fail(core.NewError("unrelated"))
	core.AssertEqual(t, CredentialErrorCode(""), CredentialErrorCodeOf(foreign))
}

// TestMediumCredentialProvider_Invalidate_NilReceiver_Bad — Invalidate
// on a nil *MediumCredentialProvider must be a no-op, not a nil-deref.
func TestMediumCredentialProvider_Invalidate_NilReceiver_Bad(t *core.T) {
	var provider *MediumCredentialProvider
	provider.Invalidate()
}
