// SPDX-Licence-Identifier: EUPL-1.2

// Tests for the Wails-bound surface in wails.go. Each method is a
// thin delegator to the underlying Service method already covered in
// depth elsewhere — these tests exist only to exercise the bound
// method itself (0% coverage otherwise, since nothing else calls
// through the W-prefixed names).

package serverkey_test

import (
	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/serverkey"
)

func TestWails_WAccountStatus_Good_DelegatesToAccountStatus(t *core.T) {
	_ = homeFixture(t)
	svc := subject.NewService(nil)

	r := svc.WAccountStatus()
	core.AssertTrue(t, r.OK, "WAccountStatus should succeed on fresh install")
	out, ok := r.Value.(subject.AccountStatusOutput)
	core.AssertTrue(t, ok, "WAccountStatus must return AccountStatusOutput")
	core.AssertTrue(t, !out.HasUserAccount, "fresh install must report no user account")
}

func TestWails_WIssueBootstrapToken_Good_MintsAccountCreateScope(t *core.T) {
	_ = homeFixture(t)
	svc := subject.NewService(nil)
	core.AssertTrue(t, svc.Bootstrap().OK, "Bootstrap should succeed")

	r := svc.WIssueBootstrapToken()
	core.AssertTrue(t, r.OK, "WIssueBootstrapToken should succeed once bootstrapped")
	out, ok := r.Value.(subject.BootstrapTokenOutput)
	core.AssertTrue(t, ok, "WIssueBootstrapToken must return BootstrapTokenOutput")
	core.AssertTrue(t, out.Token != "", "minted token must be non-empty")

	// The minted token must verify against the account.create scope —
	// confirming WIssueBootstrapToken really is the default-scope path.
	vr := svc.VerifyBootstrapToken(out.Token, "account.create")
	core.AssertTrue(t, vr.OK, "token minted via WIssueBootstrapToken must verify as account.create")
}

func TestWails_WIssueUnlockBootstrapToken_Good_MintsUnlockScope(t *core.T) {
	_ = homeFixture(t)
	svc := subject.NewService(nil)
	core.AssertTrue(t, svc.Bootstrap().OK, "Bootstrap should succeed")

	r := svc.WIssueUnlockBootstrapToken()
	core.AssertTrue(t, r.OK, "WIssueUnlockBootstrapToken should succeed once bootstrapped")
	out, ok := r.Value.(subject.BootstrapTokenOutput)
	core.AssertTrue(t, ok, "WIssueUnlockBootstrapToken must return BootstrapTokenOutput")

	// Must verify against account.unlock (not account.create) — proves
	// the W-wrapper really threads the "account.unlock" scope through.
	vr := svc.VerifyBootstrapToken(out.Token, "account.unlock")
	core.AssertTrue(t, vr.OK, "token minted via WIssueUnlockBootstrapToken must verify as account.unlock")
	mismatch := svc.VerifyBootstrapToken(out.Token, "account.create")
	core.AssertTrue(t, !mismatch.OK, "an unlock-scoped token must NOT verify against account.create")
}
