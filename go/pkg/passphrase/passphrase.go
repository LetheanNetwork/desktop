// SPDX-Licence-Identifier: EUPL-1.2

// Package passphrase exposes shape-and-strength validators for
// user-supplied passphrases. Today's surface: IsCommon — checks
// the candidate against the top-100K leaked-passphrase set so
// a 12-char-floor passphrase like "passwordpassword" (caught by
// length but trivial to brute-force) rejects on the common-list
// gate before reaching downstream key-derivation.
//
// Cross-cutting by design: account creation, future password-reset,
// SSH/PGP passphrase setup, and any future passphrase-bearing
// surface all consume the same primitive. Pkg/account would be a
// too-narrow home — passphrase strength is a property of the
// passphrase, not of the account.
//
// On the dataset (Stage X RFC v2 §5.1 HIGH-3): ships the NCSC's
// HIBP-derived top-100K leaked-passwords list — ~99,839 unique
// SHA-1 digests embedded as a 1.9MB binary blob at
// data/top100k.bin, lexicographically sorted so binary-search
// lookup is O(log N) ≈ 17 comparisons. Source + attribution at
// /LICENSES/HIBP.md. Bumping the dataset is a binary-file
// replacement — no Go-code change.
//
// Privacy note: digests not plaintexts, so a binary inspection
// (file / strings / hex-dump) does not surface a readable leaked-
// password dictionary. The embed is opaque entropy by inspection.
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
	"sort"

	core "dappco.re/go"
)

// hashSize is the byte-length of a SHA-1 digest. Fixed by RFC 3174.
const hashSize = 20

// hashCount holds the number of SHA-1 digests loaded from the
// embedded blob. Computed at init().
var hashCount int

func init() {
	// data/top100k.bin is concatenated 20-byte SHA-1 digests,
	// lexicographically sorted on disk. mustDigestCount validates
	// the blob length is a clean multiple of hashSize — anything
	// else means a truncated or malformed embed and we MUST
	// surface that loudly (passphrase rejection is a security
	// floor — silently loading a half-truncated dataset would
	// leak weak passphrases past the gate).
	hashCount = mustDigestCount(len(top100kBin))
}

// mustDigestCount validates that n (the embedded blob's byte
// length) is a clean multiple of hashSize and returns the resulting
// digest count, panicking on a malformed length. Extracted out of
// init() as a pure, directly-testable seam — init() itself cannot
// be re-invoked from a test, so the corrupt-embed guard would
// otherwise be permanently untestable dead code from the coverage
// tool's point of view. Behaviour and panic message are unchanged
// from the inline form this replaces.
func mustDigestCount(n int) int {
	if n%hashSize != 0 {
		panic(core.Sprintf(
			"passphrase: embedded dataset length %d is not a multiple of SHA-1 size %d — top100k.bin corrupt",
			n, hashSize))
	}
	return n / hashSize
}

// digestAt returns the i-th 20-byte digest as a slice view into
// the embedded blob. Zero-copy.
func digestAt(i int) []byte {
	off := i * hashSize
	return top100kBin[off : off+hashSize]
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
	// Binary search the sorted-on-disk digest blob via digestAt
	// (zero-copy view into top100kBin).
	idx := sort.Search(hashCount, func(i int) bool {
		return compareHashBytes(digestAt(i), sum[:]) >= 0
	})
	if idx >= hashCount {
		return false
	}
	return compareHashBytes(digestAt(idx), sum[:]) == 0
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
// + bump-aware (each future bump to the embedded blob will
// surface the new size to the operator's attention via the
// pinning test).
func DatasetSize() int {
	return hashCount
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
