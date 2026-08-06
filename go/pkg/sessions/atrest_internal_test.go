// SPDX-Licence-Identifier: EUPL-1.2

// atrest_internal_test.go — white-box unit coverage for
// isSealedEnvelopeString.
//
// FINDING: isSealedEnvelopeString's doc comment says it is "the
// string-keyed convenience used inside List() iteration where
// store.get_all returns map[string]string" — but grep across the
// package shows zero call sites. List() (sessions.go) and every other
// reader use the []byte-keyed isSealedEnvelope instead. The function
// is dead code: implemented, documented as wired, never actually
// referenced. That's why it carried 0% coverage before this file —
// no production path reaches it. Flagged here rather than silently
// covered around; the package-external test suite genuinely cannot
// reach it (unexported, uncalled), so this internal-package test file
// exercises it directly as a pure-logic unit rather than leaving it
// untested. A maintainer should either wire it into List() as the
// doc comment describes, or remove the now-unreferenced doc claim.

package sessions

import "testing"

func TestAtRest_IsSealedEnvelopeString_Good_MatchesPrefix(t *testing.T) {
	sealed := envelopeMagic + "restofpayload"
	if !isSealedEnvelopeString(sealed) {
		t.Fatalf("expected true for a string carrying the envelope magic prefix")
	}
}

func TestAtRest_IsSealedEnvelopeString_Bad_PlaintextNoPrefix(t *testing.T) {
	if isSealedEnvelopeString("plain old markdown content") {
		t.Fatalf("expected false for plaintext without the envelope magic")
	}
}

func TestAtRest_IsSealedEnvelopeString_Ugly_ShorterThanMagic(t *testing.T) {
	if len(envelopeMagic) == 0 {
		t.Skip("envelopeMagic is empty in this build — length-guard branch not meaningful")
	}
	tooShort := envelopeMagic[:len(envelopeMagic)-1]
	if isSealedEnvelopeString(tooShort) {
		t.Fatalf("expected false for a string shorter than the envelope magic")
	}
}
