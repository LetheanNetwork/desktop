// SPDX-Licence-Identifier: EUPL-1.2

// Tests for atrest_keys.go — the shared recordfile.AccountKeys bridge
// over a consumer's SessionIDs + AccountKeyProvider pair. Exercised
// against small in-package fakes (this is the whole point of the
// scope-parameterised adapter: any consumer can wire it against a
// stub during their own tests, so ours does the same).

package account_test

import (
	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/account"
)

// fakeGate satisfies subject.SessionIDs with a fixed set of unlocked ids.
type fakeGate struct{ ids []string }

func (g fakeGate) UnlockedAccountIDs() []string { return g.ids }

// fakeKeyProvider satisfies subject.AccountKeyProvider from fixed maps.
type fakeKeyProvider struct {
	pub  map[string][]byte
	priv map[string]*subject.PrivateKeyHandle
}

func (p fakeKeyProvider) PublicKeyFor(accountID string) ([]byte, bool) {
	b, ok := p.pub[accountID]
	return b, ok
}

func (p fakeKeyProvider) PrivateKeyFor(accountID string) (*subject.PrivateKeyHandle, bool) {
	h, ok := p.priv[accountID]
	return h, ok
}

func TestNewAtRestKeys_Good_EmptyScopeDefaultsToAtrest(t *core.T) {
	keys := subject.NewAtRestKeys("", subject.AtRestKeysDeps{
		Gate: fakeGate{},
		Keys: fakeKeyProvider{},
	})

	// No unlocked accounts under the default "atrest" scope.
	_, err := keys.SingleUnlockedAccount()
	core.AssertError(t, err)
	core.AssertContains(t, err.Error(), "atrest.no_unlocked_account")
}

func TestAtRestKeys_PublicKeyFor_Good(t *core.T) {
	keys := subject.NewAtRestKeys("deals", subject.AtRestKeysDeps{
		Gate: fakeGate{},
		Keys: fakeKeyProvider{pub: map[string][]byte{
			"acct-1": []byte("armoured-public-bytes"),
		}},
	})

	pub, err := keys.PublicKeyFor("acct-1")

	core.AssertNoError(t, err)
	core.AssertEqual(t, "armoured-public-bytes", string(pub))
}

func TestAtRestKeys_PublicKeyFor_BadUnknownAccountIsTypedError(t *core.T) {
	keys := subject.NewAtRestKeys("deals", subject.AtRestKeysDeps{
		Gate: fakeGate{},
		Keys: fakeKeyProvider{},
	})

	pub, err := keys.PublicKeyFor("unknown")

	core.AssertNil(t, pub)
	core.AssertError(t, err)
	core.AssertContains(t, err.Error(), "deals.atrest.public_key_unavailable")
}

func TestAtRestKeys_PrivateKeyFor_Good(t *core.T) {
	handle := subject.NewPrivateKeyHandleForTest([]byte("secret-priv-bytes"))
	keys := subject.NewAtRestKeys("deals", subject.AtRestKeysDeps{
		Gate: fakeGate{},
		Keys: fakeKeyProvider{priv: map[string]*subject.PrivateKeyHandle{
			"acct-1": handle,
		}},
	})

	got, ok := keys.PrivateKeyFor("acct-1")

	core.RequireTrue(t, ok)
	var seen string
	core.AssertNoError(t, got.Use(func(priv []byte) error {
		seen = string(priv)
		return nil
	}))
	core.AssertEqual(t, "secret-priv-bytes", seen)
}

func TestAtRestKeys_PrivateKeyFor_BadUnknownAccount(t *core.T) {
	keys := subject.NewAtRestKeys("deals", subject.AtRestKeysDeps{
		Gate: fakeGate{},
		Keys: fakeKeyProvider{},
	})

	got, ok := keys.PrivateKeyFor("unknown")

	core.AssertFalse(t, ok)
	core.AssertNil(t, got)
}

func TestAtRestKeys_PrivateKeyFor_UglyNilHandleTreatedAsMissing(t *core.T) {
	keys := subject.NewAtRestKeys("deals", subject.AtRestKeysDeps{
		Gate: fakeGate{},
		Keys: fakeKeyProvider{priv: map[string]*subject.PrivateKeyHandle{
			"acct-1": nil,
		}},
	})

	got, ok := keys.PrivateKeyFor("acct-1")

	core.AssertFalse(t, ok)
	core.AssertNil(t, got)
}

func TestAtRestKeys_SingleUnlockedAccount_Good(t *core.T) {
	keys := subject.NewAtRestKeys("deals", subject.AtRestKeysDeps{
		Gate: fakeGate{ids: []string{"acct-1"}},
		Keys: fakeKeyProvider{},
	})

	id, err := keys.SingleUnlockedAccount()

	core.AssertNoError(t, err)
	core.AssertEqual(t, "acct-1", id)
}

func TestAtRestKeys_SingleUnlockedAccount_BadNoneUnlocked(t *core.T) {
	keys := subject.NewAtRestKeys("deals", subject.AtRestKeysDeps{
		Gate: fakeGate{},
		Keys: fakeKeyProvider{},
	})

	id, err := keys.SingleUnlockedAccount()

	core.AssertEqual(t, "", id)
	core.AssertError(t, err)
	core.AssertContains(t, err.Error(), "deals.no_unlocked_account")
}

func TestAtRestKeys_SingleUnlockedAccount_UglyMultipleUnlockedRejected(
	t *core.T,
) {
	keys := subject.NewAtRestKeys("deals", subject.AtRestKeysDeps{
		Gate: fakeGate{ids: []string{"acct-1", "acct-2"}},
		Keys: fakeKeyProvider{},
	})

	id, err := keys.SingleUnlockedAccount()

	core.AssertEqual(t, "", id)
	core.AssertError(t, err)
	core.AssertContains(t, err.Error(), "deals.multi_account_not_supported")
}
