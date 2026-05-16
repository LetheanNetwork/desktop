// SPDX-Licence-Identifier: EUPL-1.2

package account

import (
	"runtime"

	"golang.org/x/text/unicode/norm"
)

// nfkcPassphrase canonicalises a user-typed passphrase to NFKC form
// so the same logical input — `café` typed on a macOS keyboard (often
// NFD: c, a, f, e + combining acute) and the same `café` typed on a
// Linux keyboard (often NFC: c, a, f, é) — produces identical bytes
// for the openpgp symmetric-decrypt path.
//
// Cerberus DREAD CRIT-1 (RFC.stage-x.md §6): without this, an account
// created on one OS permanently locks out the same user on another
// OS. The fix MUST be symmetric across the create side (Provision —
// Stage X.B Phase 2) and the unlock side (this file's consumer).
//
// CoreGO gap log: `core.NormaliseNFKC` is the named primitive RFC
// §10 + memory `project_corego_export_gaps` flag for upstream. This
// helper is a local stand-in until the export lands; same call-site
// pattern in both consumers so the upstream-bump is a one-line swap.
//
// Usage example:
//
//	pp := nfkcPassphrase(input.Passphrase)
//	defer zeroAndKeepAlive(pp)
//	_, _, err := distinguishDecrypt(ciphertext, pp)
func nfkcPassphrase(s string) []byte {
	return norm.NFKC.Bytes([]byte(s))
}

// zeroAndKeepAlive overwrites b in place then anchors the slice live
// through the call so the compiler cannot elide the writes via escape
// analysis (RFC.stage-x.md §3.3, Cerberus DREAD Q4 ruling).
//
// Without runtime.KeepAlive the Go compiler can prove no subsequent
// reads of b and drop the zero loop — passphrase bytes then remain
// in process memory across the function lifetime + GC delay, which
// defeats the explicit-wipe intent and shows up in a post-mortem
// core dump on a crashed lthn process.
//
// Usage example:
//
//	defer zeroAndKeepAlive(pp)
func zeroAndKeepAlive(b []byte) {
	for i := range b {
		b[i] = 0
	}
	runtime.KeepAlive(b)
}
