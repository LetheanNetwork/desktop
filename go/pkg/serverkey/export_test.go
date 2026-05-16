// SPDX-Licence-Identifier: EUPL-1.2

// Test-only helpers. Lives under _test.go so production callers
// can't reach the os.Chmod stand-in (banned in production per AX-6).

package serverkey

import (
	"os"

	core "dappco.re/go"
)

// OsChmod is the test-only stand-in for os.Chmod — CoreGO doesn't
// expose a Chmod primitive today, so tests that need to widen a file
// mode out-of-band (Cerberus #1464 negative-path coverage) reach
// stdlib directly. Test files are exempt from the production banned-
// imports rule.
func OsChmod(path string, mode os.FileMode) error {
	return os.Chmod(path, mode)
}

// ExportedCanonicalise re-exposes the package-private canonicalise
// helper so the Cerberus #1469 round-trip + sorted-key tests can
// assert on it directly. Test-only.
func ExportedCanonicalise(header map[string]any) ([]byte, core.Result) {
	return canonicalise(header)
}

// ExportedSetSingleInstanceCheck installs a test override of the
// CoreGUI single-instance probe so Bootstrap can short-circuit the
// file-lock path. Cerberus #1466 — both branches (CoreGUI-says-yes
// and file-lock-fallback) are exercised in unit tests via this hook.
func (s *Service) ExportedSetSingleInstanceCheck(fn func() bool) {
	s.singleInstanceCheck = fn
}

// ExportedNonceKey re-exposes the per-process HMAC-keyed nonce
// derivation so Cerberus #1463 tests can assert the consumed-nonce
// set holds HMAC(processSalt, nonce) — not the raw nonce.
func (s *Service) ExportedNonceKey(nonce string) string {
	return s.nonceKey(nonce)
}

// ExportedConsumedNonces returns a snapshot of the consumed-nonce
// map for Cerberus #1463 inspection in tests. Returns a copy so the
// caller can't mutate service state.
func (s *Service) ExportedConsumedNonces() map[string]core.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]core.Time, len(s.consumedNonces))
	for k, v := range s.consumedNonces {
		out[k] = v
	}
	return out
}
