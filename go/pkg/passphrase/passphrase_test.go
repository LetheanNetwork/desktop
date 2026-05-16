// SPDX-Licence-Identifier: EUPL-1.2

// Tests for the passphrase common-list check. Triadic Good / Bad /
// Ugly coverage per AX-9 — Stage X RFC v2 §5.1 HIGH-3.

package passphrase_test

import (
	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/passphrase"
)

// --- IsCommon — Good ---

func TestPassphrase_IsCommon_Good_KnownLeaked(t *core.T) {
	// Every entry in this list MUST be flagged. If any returns
	// false, the dataset has been bumped + a plaintext-comment
	// got out of sync with its hash. Triggers obvious test failure.
	for _, p := range []string{
		"password",
		"123456",
		"qwerty",
		"abc123",
		"letmein",
		"iloveyou",
		"passwordpassword", // the load-bearing case from the brief
		"admin",
		"welcome",
		"trustno1",
	} {
		core.AssertTrue(t, subject.IsCommon(p),
			"IsCommon("+p+") MUST flag — well-known leaked passphrase")
	}
}

func TestPassphrase_IsCommon_Good_UncommonRejects(t *core.T) {
	// Random-ish strong passphrases MUST NOT flag — defends
	// against a buggy lookup that always returns true.
	for _, p := range []string{
		"correct horse battery staple",     // diceware
		"ssauwL4HrNDvyJPbcfEUYbBE",         // random 24-char
		"my-cat-is-named-fluffy-and-i-love-her", // semantically structured but not leaked
		"Tr0ub4dor&3",                       // xkcd-style strong-enough
		"a-very-long-passphrase-that-no-one-would-use-elsewhere",
	} {
		core.AssertFalse(t, subject.IsCommon(p),
			"IsCommon("+p+") MUST NOT flag — not in dataset")
	}
}

// --- IsCommon — Bad / empty ---

func TestPassphrase_IsCommon_Bad_Empty(t *core.T) {
	// Empty candidate is not "common", it's empty. Caller
	// validates non-emptiness separately; IsCommon returns false
	// without doing any hash work.
	core.AssertFalse(t, subject.IsCommon(""), "empty candidate returns false (not 'leaked')")
}

// --- IsCommon — Ugly: case sensitivity ---

func TestPassphrase_IsCommon_Ugly_CaseSensitive(t *core.T) {
	// IsCommon is the EXACT-MATCH primitive — "PASSWORD" does NOT
	// match the stored hash for "password" (different bytes,
	// different SHA-1). IsCommonNormalised is the case-folding
	// variant. This test pins the contract — separating concerns
	// means a future caller doing their own normalisation
	// (Unicode, trim, etc) can compose without surprises.
	core.AssertTrue(t, subject.IsCommon("password"))
	core.AssertFalse(t, subject.IsCommon("PASSWORD"),
		"IsCommon is case-sensitive — use IsCommonNormalised for case-folding")
	core.AssertFalse(t, subject.IsCommon("Password"))
}

// --- IsCommonNormalised — Good ---

func TestPassphrase_IsCommonNormalised_Good_CaseVariants(t *core.T) {
	// IsCommonNormalised lower-cases first → catches the case-
	// variant trivia that IsCommon doesn't.
	for _, p := range []string{
		"PASSWORD",
		"Password",
		"PaSSwoRd",
		"QWERTY",
		"Letmein",
		"PASSWORDPASSWORD",
	} {
		core.AssertTrue(t, subject.IsCommonNormalised(p),
			"IsCommonNormalised("+p+") MUST flag — case-variant of leaked")
	}
}

func TestPassphrase_IsCommonNormalised_Good_LowerCaseAlsoFlags(t *core.T) {
	// Lower-case input that's in the dataset still flags
	// (normalisation is idempotent on already-lower input).
	core.AssertTrue(t, subject.IsCommonNormalised("password"))
}

// --- IsCommonNormalised — Bad / Ugly ---

func TestPassphrase_IsCommonNormalised_Bad_Empty(t *core.T) {
	core.AssertFalse(t, subject.IsCommonNormalised(""))
}

func TestPassphrase_IsCommonNormalised_Ugly_StrongStaysStrong(t *core.T) {
	// Lower-casing a strong passphrase MUST NOT make it common
	// — defends against a buggy normaliser that strips too much
	// and creates collisions.
	core.AssertFalse(t, subject.IsCommonNormalised("Tr0ub4dor&3"))
	core.AssertFalse(t, subject.IsCommonNormalised("correct horse battery staple"))
}

// --- Dataset integrity ---

func TestPassphrase_Dataset_NonEmpty_Good(t *core.T) {
	n := subject.DatasetSize()
	core.AssertTrue(t, n >= 20,
		core.Sprintf("dataset MUST contain at least 20 entries (got %d)", n))
}

// TestPassphrase_Dataset_NoMalformedEntries_Good — if any source
// entry was malformed (wrong hex length, non-hex characters), it
// gets silently dropped at init(). Asserting DatasetSize() ==
// expected catches the silent-drop case: if a dev edit corrupts
// an entry, the loaded count drops below the source count and
// this test fires.
//
// Compares against a hardcoded expected count. When the dataset
// grows (Snider routes the top-N bulk import), update the
// expected value in lockstep so the test stays load-bearing.
func TestPassphrase_Dataset_AllEntriesValid_Good(t *core.T) {
	// Source list at dataset.go has 48 entries today. Bump this
	// number when the dataset grows.
	const expected = 48
	got := subject.DatasetSize()
	core.AssertEqual(t, expected, got,
		core.Sprintf(
			"DatasetSize() = %d; expected %d. If you added entries to dataset.go, bump this constant. "+
				"If you DIDN'T add entries, an init() silently dropped a malformed entry — check dataset.go for hex typos.",
			got, expected))
}

// --- Integration sanity: brief's load-bearing case ---

// TestPassphrase_BriefCase_passwordpassword pins the exact case
// the Stage X RFC v2 §5.1 HIGH-3 brief calls out: a 16-char
// all-lowercase passphrase that passes the 12-char floor but is
// trivially-guessable. IsCommon MUST flag it.
func TestPassphrase_BriefCase_passwordpassword(t *core.T) {
	core.AssertTrue(t, subject.IsCommon("passwordpassword"),
		"Stage X RFC v2 §5.1 HIGH-3 brief case — passwordpassword (16 chars, passes length floor) MUST flag")
}
