// SPDX-Licence-Identifier: EUPL-1.2

// Package passphrase exposes shape-and-strength validators for
// user-supplied passphrases. Today's surface: IsCommon — checks
// the candidate against a curated set of well-known leaked
// passwords so a 12-char-floor passphrase like "passwordpassword"
// (caught by length but trivial to brute-force) rejects on the
// common-list gate before reaching downstream key-derivation.
//
// Cross-cutting by design: account creation, future password-reset,
// SSH/PGP passphrase setup, and any future passphrase-bearing
// surface all consume the same primitive. Pkg/account would be a
// too-narrow home — passphrase strength is a property of the
// passphrase, not of the account.
//
// On the dataset (Stage X RFC v2 §5.1 HIGH-3): the canonical
// long-form intent is a top-N HIBP-style breach list (100K+
// entries). This v1 ships a curated ~50-entry seed sourced from
// the universally-known leaked-password lists (SecLists,
// CrackStation, NIST examples) — the "worst of the worst" subset
// that any reasonable bulk-N expansion would include. The
// architecture (binary SHA1 hashes, sorted slice, binary-search
// lookup) is the same one a 100K expansion uses; growing the
// dataset is a `data` array edit, no API change.
//
// Future revision path: when Snider routes the dataset-source
// procurement decision (HIBP v8 vs newer vs SecLists vs roll-our-
// own — captured in pkg-level docs.md when filed), drop a binary
// SHA1 file at pkg/passphrase/dataset/top10k.bin and switch
// init() to `//go:embed`. The lookup logic stays identical.
//
// Privacy note: storing hashes (not plaintexts) means the binary
// doesn't surface a readable leaked-password dictionary to anyone
// inspecting it. Costs us nothing at v1 scale; matters at 100K.
//
// Usage example:
//
//	if passphrase.IsCommon(input.Passphrase) {
//	    return core.Fail(core.NewCode("passphrase_too_common",
//	        "passphrase appears in well-known breach lists — choose a longer or less common phrase"))
//	}

package passphrase

import (
	"crypto/sha1"
	"encoding/hex"
	"sort"

	core "dappco.re/go"
)

// hashSize is the byte-length of a SHA-1 digest. Fixed by RFC 3174.
const hashSize = 20

// hashes is the sorted set of binary SHA-1 digests for the
// embedded common-passphrase dataset. Populated at init() from
// rawHashHex; sorted-binary-encoded so IsCommon can sort.Search()
// it in O(log N).
//
// Sorted-on-init (not pre-sorted in source) so the literal source
// list reads in roughly-by-frequency order, but the runtime
// representation is binary-search-optimal.
var hashes [][]byte

func init() {
	hashes = make([][]byte, 0, len(rawHashHex))
	for _, hexStr := range rawHashHex {
		b, err := hex.DecodeString(hexStr)
		if err != nil || len(b) != hashSize {
			// Skip malformed entries silently — they represent
			// hard-coded data and a malformed entry would be a
			// dev-time typo caught by TestPassphrase_Dataset_NoMalformedEntries_Good.
			continue
		}
		hashes = append(hashes, b)
	}
	sort.Slice(hashes, func(i, j int) bool {
		return compareHashBytes(hashes[i], hashes[j]) < 0
	})
}

// IsCommon reports whether the candidate passphrase appears in
// the curated common-leaked-passphrase dataset. Empty candidate
// returns false (no leak, just empty — caller validates
// non-emptiness separately).
//
// Lookup: SHA-1 the candidate → binary search the sorted hash
// list. O(log N) with N currently ≈ 50; microseconds even at
// the planned 10K+ expansion.
//
// NOT a substitute for length / entropy checks — IsCommon
// catches "passwordpassword" (length-12 passes the floor but
// the string is trivially-guessable), but doesn't catch a
// random-looking 8-char string that just happens to not be in
// the list. Caller composes length + IsCommon + future entropy
// estimation.
//
// Usage example:
//
//	if len(p) < 12 || passphrase.IsCommon(p) {
//	    return core.Fail(...)
//	}
func IsCommon(candidate string) bool {
	if candidate == "" {
		return false
	}
	sum := sha1.Sum([]byte(candidate))
	// Binary search the sorted hashes slice.
	idx := sort.Search(len(hashes), func(i int) bool {
		return compareHashBytes(hashes[i], sum[:]) >= 0
	})
	if idx >= len(hashes) {
		return false
	}
	return compareHashBytes(hashes[idx], sum[:]) == 0
}

// IsCommonNormalised lower-cases the candidate before checking.
// Catches case-variant trivia ("PASSWORD" → "password" → in list)
// without bloating the dataset with per-case duplicates. Use this
// at the user-facing validation boundary; IsCommon is the exact-
// match primitive for callers who already normalised.
//
// Usage example:
//
//	if passphrase.IsCommonNormalised(input.Passphrase) {
//	    return tooCommonError
//	}
func IsCommonNormalised(candidate string) bool {
	if candidate == "" {
		return false
	}
	return IsCommon(core.Lower(candidate))
}

// DatasetSize reports the number of distinct hashes in the
// loaded dataset. Test-facing so the suite can assert non-empty
// + bump-aware (future bump to 10K will cause the test to
// surface the new size to the operator's attention).
func DatasetSize() int {
	return len(hashes)
}

// compareHashBytes returns -1 / 0 / 1 like bytes.Compare. Local
// helper to avoid a `bytes` import for a single call site.
func compareHashBytes(a, b []byte) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	if len(a) < len(b) {
		return -1
	}
	if len(a) > len(b) {
		return 1
	}
	return 0
}
