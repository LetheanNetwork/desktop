// SPDX-Licence-Identifier: EUPL-1.2

// Cross-platform NFKC parity test for the Stage X.B Phase 1 unlock
// patch (RFC.stage-x.md §6, Cerberus DREAD CRIT-1).
//
// macOS keyboards typically emit NFD (`e` + combining acute) for
// accented characters; Linux keyboards typically emit NFC (`é`).
// Both byte-sequences represent the same logical character but their
// raw bytes differ. Before this patch, an account created on one OS
// could not be unlocked on the other — permanent self-lockout.
//
// The fix in unlock.go normalises the user-typed passphrase to NFKC
// before handing it to the openpgp symmetric-decrypt path. The test
// pins the invariant by encrypting with the NFKC canonical form and
// asserting unlock succeeds when the user types EITHER NFC or NFD of
// the same character.

package account_test

import (
	"golang.org/x/text/unicode/norm"

	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/account"
	"forge.lthn.ai/Snider/Enchantrix/pkg/crypt/std/pgp"
)

// nfcCafe is `café` with the e+acute as a single code point (U+00E9).
const nfcCafe = "café"

// nfdCafe is `café` decomposed: `e` (U+0065) followed by combining
// acute (U+0301). Byte-different from nfcCafe but NFKC-equal.
const nfdCafe = "café"

// TestUnlock_NFKCCrossPlatformParity_Ugly proves the create-side
// encrypts with the NFKC canonical form and the unlock-side normalises
// any user-typed variant to the same canonical, so NFC/NFD-typed
// passphrases interoperate across macOS + Linux keyboards.
func TestUnlock_NFKCCrossPlatformParity_Ugly(t *core.T) {
	home := homeFixture(t)

	// Pre-condition assertion — make the test's premise self-evident.
	// If these ever pass (NFC and NFD bytes equal raw), the test loses
	// its point because the OS keyboards aren't producing the variants
	// we're claiming to handle.
	core.AssertTrue(t, nfcCafe != nfdCafe,
		"test fixture broken — NFC and NFD MUST differ at the byte level")
	core.AssertTrue(t, string(norm.NFKC.Bytes([]byte(nfcCafe))) ==
		string(norm.NFKC.Bytes([]byte(nfdCafe))),
		"test fixture broken — NFC and NFD MUST be NFKC-equal")

	// Stage X.B Provision will encrypt with NFKC-of-typed-passphrase.
	// Simulate that here by encrypting with the NFKC canonical form
	// of the NFC variant — both inputs (NFC + NFD) normalise to it.
	storedPassphrase := string(norm.NFKC.Bytes([]byte(nfcCafe)))
	writeEncryptedAccount(t, home, fixtureAccountID, storedPassphrase)

	// Branch 1 — user typed the NFC form (Linux keyboard).
	svcA := newUnlockable(t, home)
	rA := svcA.Unlock(subject.UnlockInput{
		AccountID:  fixtureAccountID,
		Passphrase: nfcCafe,
	})
	core.AssertTrue(t, rA.OK,
		"NFC-typed passphrase MUST unlock account stored in NFKC form")

	// Branch 2 — user typed the NFD form (macOS keyboard).
	svcB := newUnlockable(t, home)
	rB := svcB.Unlock(subject.UnlockInput{
		AccountID:  fixtureAccountID,
		Passphrase: nfdCafe,
	})
	core.AssertTrue(t, rB.OK,
		"NFD-typed passphrase MUST unlock the SAME account — cross-platform lockout was the bug")
}

// TestUnlock_NFKCAsciiIdentity_Good pins that pure-ASCII passphrases
// are unaffected by the NFKC patch — NFKC of any ASCII string is
// identity, so existing accounts encrypted with raw ASCII bytes still
// unlock with raw ASCII input. Defensive against future "let's also
// fold case / strip whitespace" suggestions that would silently break
// existing accounts.
func TestUnlock_NFKCAsciiIdentity_Good(t *core.T) {
	home := homeFixture(t)
	writeEncryptedAccount(t, home, fixtureAccountID, fixturePassphrase)

	svc := newUnlockable(t, home)
	r := svc.Unlock(subject.UnlockInput{
		AccountID:  fixtureAccountID,
		Passphrase: fixturePassphrase,
	})
	core.AssertTrue(t, r.OK, "ASCII passphrase MUST unlock unchanged after NFKC patch")
}

// Compile-time assertion — pgp.NewService is the same primitive
// writeEncryptedAccount uses, so an import-trim audit catches this
// before the test fixtures silently break.
var _ = pgp.NewService
