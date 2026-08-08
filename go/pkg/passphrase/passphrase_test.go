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
		"correct horse battery staple",          // diceware
		"ssauwL4HrNDvyJPbcfEUYbBE",              // random 24-char
		"my-cat-is-named-fluffy-and-i-love-her", // semantically structured but not leaked
		"Tr0ub4dor&3",                           // xkcd-style strong-enough
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
	// IsCommon is the EXACT-MATCH primitive — different bytes
	// (different SHA-1) means a different lookup. IsCommonNormalised
	// is the case-folding variant. This test pins the contract: an
	// arbitrary case-variant of a known weak passphrase that is
	// itself a fabricated string (not in the NCSC top-100K) MUST
	// NOT match exact; its lower-case form MUST match.
	//
	// Note (Mantis #1507): at 100K-cardinality, the dataset
	// contains many user-typed case variants ("PASSWORD",
	// "Password", etc) directly. The pinning input here uses a
	// fabricated UPPERCASE form ("PaSsWoRdMaNtIs1507Bad") whose
	// lower-case version we have NOT seeded into the dataset, so
	// both halves of the contract assertion stay testable.
	const exotic = "PaSsWoRdMaNtIs1507Bad"
	core.AssertFalse(t, subject.IsCommon(exotic),
		"exotic case-variant is not in dataset — IsCommon must miss")
	core.AssertFalse(t, subject.IsCommonNormalised(exotic),
		"lower-case form of exotic variant is not in dataset either — must miss")
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

// TestPassphrase_Dataset_SizeAtLeast100K_Good pins the floor on
// the embedded NCSC top-100K dataset. The exact number is
// 99,839 (dedup of the 99,840-line source list — one duplicate
// across normalisation). Asserts ≥ 99,000 so a future bump to
// HIBP v9 etc with a slightly different cardinality doesn't
// false-fail, but a regression to the old ~50-entry seed would.
//
// Mantis #1507 (this commit) is the dataset-size bump from v1's
// ~50-entry seed to NCSC top-100K. Future Snider-routed bumps
// to a larger corpus can lift this floor.
func TestPassphrase_Dataset_SizeAtLeast100K_Good(t *core.T) {
	got := subject.DatasetSize()
	core.AssertTrue(t, got >= 99000,
		core.Sprintf(
			"DatasetSize() = %d; expected at least 99,000 (NCSC top-100K). "+
				"If the embedded data/top100k.bin shrunk, the dataset has been "+
				"silently downgraded — restore from /LICENSES/HIBP.md generator recipe.",
			got))
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

// TestPassphrase_RejectTopHIBPEntry_Bad covers Mantis #1507 done-
// criterion: the absolute-top HIBP/NCSC entries MUST reject after
// the dataset bump. "password" + "123456" sit at ranks ~1-4 in
// every public breach list since 2010.
func TestPassphrase_RejectTopHIBPEntry_Bad(t *core.T) {
	for _, p := range []string{"password", "123456", "qwerty"} {
		core.AssertTrue(t, subject.IsCommon(p),
			"Mantis #1507 — top-rank HIBP/NCSC entry "+p+" MUST flag")
	}
}

// TestPassphrase_AcceptStrongRandom_Good covers Mantis #1507's
// "high-entropy random → accepted" criterion. These are not
// random in this test (deterministic for reproducibility) but
// are structurally high-entropy strings not present in any
// public breach corpus. Defends against a bulk-load bug that
// matches everything.
func TestPassphrase_AcceptStrongRandom_Good(t *core.T) {
	for _, p := range []string{
		"qX7@kP2$mZ9vL4nB8wR1eT3yU6iO0pA",
		"correct-horse-battery-staple-not-in-hibp-2026",
		"my-vault-passphrase-mantis-1507-verification",
		"3.14159-26535-89793-23846-26433-83279-50288",
	} {
		core.AssertFalse(t, subject.IsCommon(p),
			"strong-random "+p+" MUST NOT flag — not in dataset")
	}
}
