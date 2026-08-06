// SPDX-Licence-Identifier: EUPL-1.2

// Internal (white-box) tests for the audit-sample integrity
// invariants. Lives in the passphrase package itself (not
// passphrase_test) so it can read the unexported auditSample
// slice — that's the load-bearing point of this test file. If
// a comment-vs-hash mismatch creeps in (typo on the hex column,
// plaintext copy-paste error) this test fires.

package passphrase

import (
	"crypto/sha1"
	"encoding/hex"

	core "dappco.re/go"
)

// TestPassphrase_AuditSample_HexMatchesPlaintext_Good verifies
// every (Plaintext, SHA1Hex) row in auditSample is internally
// consistent — sha1(plaintext) == decoded hex. Defends against
// a hand-edit to dataset.go that corrupts either column.
func TestPassphrase_AuditSample_HexMatchesPlaintext_Good(t *core.T) {
	for _, row := range auditSample {
		sum := sha1.Sum([]byte(row.Plaintext))
		want, err := hex.DecodeString(row.SHA1Hex)
		core.AssertNoError(t, err, "auditSample row "+row.Plaintext+" SHA1Hex must be valid hex")
		if len(want) != hashSize {
			t.Fatalf("auditSample row %s SHA1Hex decodes to %d bytes; expected %d",
				row.Plaintext, len(want), hashSize)
		}
		if compareHashBytes(sum[:], want) != 0 {
			t.Errorf("auditSample row %s — sha1(plaintext) != SHA1Hex column", row.Plaintext)
		}
	}
}

// TestPassphrase_AuditSample_AllInDataset_Good verifies every
// audit-sample plaintext is actually present in the embedded
// top-100K blob. Catches the case where dataset.go's comments
// promise "password is in the dataset" but a future
// dataset-source switch (different breach corpus) silently
// drops some entries.
func TestPassphrase_AuditSample_AllInDataset_Good(t *core.T) {
	for _, row := range auditSample {
		if !IsCommon(row.Plaintext) {
			t.Errorf("auditSample plaintext %q absent from embedded dataset — "+
				"either the dataset shrunk or dataset.go's audit-sample is now lying",
				row.Plaintext)
		}
	}
}

// --- mustDigestCount — extracted init() validation seam ---

// TestPassphrase_MustDigestCount_Good_ExactMultiple pins the happy
// path: a clean multiple of hashSize divides down to the expected
// digest count. This is exactly what init() does with the real
// embed at package load, just called directly here since init()
// itself cannot be re-invoked from a test.
func TestPassphrase_MustDigestCount_Good_ExactMultiple(t *core.T) {
	core.AssertEqual(t, 5, mustDigestCount(5*hashSize),
		"5 digests worth of bytes must report count 5")
	core.AssertEqual(t, 0, mustDigestCount(0),
		"an empty blob is a degenerate but valid zero-count case")
}

// TestPassphrase_MustDigestCount_Bad_TruncatedEmbed pins the
// corrupt-embed guard: a byte length that is not a clean multiple
// of hashSize (20) MUST panic loudly rather than silently loading
// a truncated dataset. This is the security-floor case called out
// in the function's own doc comment — passphrase rejection must
// not silently degrade.
func TestPassphrase_MustDigestCount_Bad_TruncatedEmbed(t *core.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("mustDigestCount(%d) must panic on a non-multiple-of-%d length", hashSize+1, hashSize)
		}
	}()
	_ = mustDigestCount(hashSize + 1)
}

// TestPassphrase_MustDigestCount_Ugly_NegativeLength defends
// against a hypothetical negative length (would only arise from a
// caller bug, never from len()) still hitting the same modulo
// guard rather than e.g. panicking on the division.
func TestPassphrase_MustDigestCount_Ugly_NegativeLength(t *core.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("mustDigestCount(-1) must panic — -1 %% %d != 0", hashSize)
		}
	}()
	_ = mustDigestCount(-1)
}

// --- compareHashBytes — length-mismatch tie-breaks ---
//
// Every real call site (IsCommon's binary search, the audit-sample
// checks above) compares two full 20-byte SHA-1 digests, so the
// length-mismatch branches below never fire in normal operation.
// compareHashBytes is a general byte-lexicographic comparator
// though (documented "like bytes.Compare"), so its length
// tie-break logic is real, load-bearing behaviour worth pinning
// directly.

// TestCompareHashBytes_Good_EqualLength covers the common path:
// same-length slices compare byte-by-byte, no tie-break reached.
func TestCompareHashBytes_Good_EqualLength(t *core.T) {
	core.AssertEqual(t, 0, compareHashBytes([]byte{1, 2, 3}, []byte{1, 2, 3}))
	core.AssertEqual(t, -1, compareHashBytes([]byte{1, 2, 3}, []byte{1, 2, 4}))
	core.AssertEqual(t, 1, compareHashBytes([]byte{1, 2, 5}, []byte{1, 2, 4}))
}

// TestCompareHashBytes_Bad_ShorterB covers the len(b) < len(a)
// clamp (n = len(b)): a prefix-equal, then a's extra trailing byte
// makes it the longer slice, so the length tie-break at the end
// decides.
func TestCompareHashBytes_Bad_ShorterB(t *core.T) {
	core.AssertEqual(t, 1, compareHashBytes([]byte{1, 2, 3}, []byte{1, 2}),
		"a longer than prefix-equal b must compare greater")
	core.AssertEqual(t, -1, compareHashBytes([]byte{1, 2}, []byte{1, 2, 3}),
		"a shorter than prefix-equal b must compare less")
}

// TestCompareHashBytes_Ugly_DivergesBeforeLengthTie covers the
// case where the shared-prefix loop already finds a byte
// difference before the shorter slice runs out — the length
// tie-break must NOT override an in-prefix divergence.
func TestCompareHashBytes_Ugly_DivergesBeforeLengthTie(t *core.T) {
	core.AssertEqual(t, -1, compareHashBytes([]byte{1, 2}, []byte{1, 9, 9, 9}),
		"in-prefix divergence decides even though a is shorter")
	core.AssertEqual(t, 1, compareHashBytes([]byte{9, 9, 9}, []byte{1, 2}),
		"in-prefix divergence decides even though a is longer")
}

// --- IsCommon — binary-search miss-past-the-end branch ---

// TestPassphrase_IsCommon_Ugly_HashSortsPastDatasetEnd exercises
// the idx >= hashCount branch inside IsCommon's sort.Search callback
// chain — the case where a candidate's SHA-1 digest sorts
// lexicographically after every entry in the dataset. Every
// hand-picked passphrase elsewhere in the suite happens to hash
// below the dataset's maximum digest, so this branch was otherwise
// unreachable from black-box calls. Bounded brute-force search
// (SHA-1 is a random oracle; the maximum stored digest currently
// starts 0xffff8…, so roughly 1 in ~130K candidates clears it —
// resolves in well under a second and never touches KDF-style
// unbounded work) finds a real candidate string, confirms via
// direct digest comparison that it truly exceeds the maximum
// stored digest (guaranteeing idx == hashCount by sort.Search's
// construction), then drives the real exported IsCommon so the
// production line itself executes.
func TestPassphrase_IsCommon_Ugly_HashSortsPastDatasetEnd(t *core.T) {
	const searchBound = 2_000_000
	last := digestAt(hashCount - 1)
	var candidate string
	found := false
	for i := 0; i < searchBound; i++ {
		c := core.Sprintf("cov-w6b-past-dataset-end-%d", i)
		sum := sha1.Sum([]byte(c))
		if compareHashBytes(sum[:], last) > 0 {
			candidate = c
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("no candidate exceeding the maximum stored digest found within %d bounded tries", searchBound)
	}
	core.AssertFalse(t, IsCommon(candidate),
		"a candidate whose SHA-1 sorts past the dataset end must report not-common (idx >= hashCount branch)")
}
