// SPDX-Licence-Identifier: EUPL-1.2

// Test-only helpers. Lives under _test.go so production callers
// can't reach the os.Chmod stand-in (banned in production per AX-6).

package serverkey

import (
	"os"

	core "dappco.re/go"
	"github.com/Snider/Enchantrix/pkg/crypt/std/pgp"
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

// ExportedIssueBootstrapTokenAtIat mints a bootstrap token using the
// supplied iat (UTC unix seconds) instead of the current wall-clock.
// Test-only — production callers always mint at now via
// IssueBootstrapToken. Lets the Cerberus #1462 clock-skew tests forge
// a token claiming an iat slightly in the future (within tolerance) or
// well in the future (beyond tolerance) without juggling system time.
//
// exp is stamped at iat + issuerTTL so the resulting token follows the
// same shape the production mint emits.
//
// Usage example (test):
//
//	tok := svc.ExportedIssueBootstrapTokenAtIat(core.Now().UTC().Unix()+10)
//	r := svc.VerifyBootstrapToken(tok, "account.create")
//	if r.OK { t.Fatal("iat in future beyond tolerance must reject") }
func (s *Service) ExportedIssueBootstrapTokenAtIat(iat int64) string {
	s.mu.RLock()
	priv := s.privateKey
	s.mu.RUnlock()
	nonceR := core.RandomBytes(nonceSize)
	if !nonceR.OK {
		return ""
	}
	nonce := core.HexEncode(nonceR.Value.([]byte))
	header := map[string]any{
		"iat":   iat,
		"exp":   iat + issuerTTL,
		"scope": scopeAccountCreate,
		"nonce": nonce,
	}
	canon, canonR := canonicalise(header)
	if !canonR.OK {
		return ""
	}
	pgpSvc := pgp.NewService()
	sig, err := pgpSvc.Sign(priv, canon)
	if err != nil {
		return ""
	}
	return tokenPrefix + core.Base64URLEncode(canon) + "." + core.Base64URLEncode(sig)
}

// ExportedClockSkewTolerance re-exposes the package-private
// clockSkewTolerance constant so Cerberus #1462 tests can assert the
// boundary value without re-declaring it (drift defence — a future
// constant bump propagates to the test on the next run).
func ExportedClockSkewTolerance() int64 { return clockSkewTolerance }

// ExportedIssuerTTL re-exposes the package-private issuerTTL constant
// for the same drift-defence reason as ExportedClockSkewTolerance.
func ExportedIssuerTTL() int64 { return issuerTTL }

// ExportedBootstrapAllowedScopes returns a copy of the closed
// allow-list IssueBootstrapTokenForScope mints against. Test-only
// surface for the Mantis #1467 lockstep parity check — the matching
// pkg/server.BootstrapPathScopes map MUST carry the same scope set,
// and the parity test asserts it. Returns a copy so callers cannot
// mutate package state.
func ExportedBootstrapAllowedScopes() map[string]bool {
	out := make(map[string]bool, len(bootstrapAllowedScopes))
	for k, v := range bootstrapAllowedScopes {
		out[k] = v
	}
	return out
}

// ExportedBuildBootstrapToken assembles an LTHN-BOOT-1. token string
// from an arbitrary header map + raw signature bytes, bypassing
// IssueBootstrapTokenForScope's mint discipline entirely. Test-only —
// lets the format/TTL/scope/claim edge cases inside VerifyBootstrapToken
// (missing iat/exp/nonce, expired exp, wrong scope, …) be forged
// directly without a live signing key, since every one of those
// checks runs BEFORE the PGP signature is verified.
func ExportedBuildBootstrapToken(header map[string]any, sig []byte) string {
	canon, r := canonicalise(header)
	if !r.OK {
		return ""
	}
	return tokenPrefix + core.Base64URLEncode(canon) + "." + core.Base64URLEncode(sig)
}

// ExportedBuildSessionToken is ExportedBuildBootstrapToken's
// LTHN-SESS-1. counterpart for VerifySessionToken's equivalent
// pre-signature claim checks.
func ExportedBuildSessionToken(header map[string]any, sig []byte) string {
	canon, r := canonicalise(header)
	if !r.OK {
		return ""
	}
	return sessionTokenPrefix + core.Base64URLEncode(canon) + "." + core.Base64URLEncode(sig)
}

// ExportedInjectExpiredNonce seeds an already-expired entry into the
// bootstrap consumed-nonce set so the next VerifyBootstrapToken call
// exercises evictExpiredNoncesLocked's delete branch — a state that
// otherwise only arises after minutes of real wall-clock time.
func (s *Service) ExportedInjectExpiredNonce(rawNonce string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.consumedNonces[s.nonceKey(rawNonce)] = core.UnixTime(0)
}

// ExportedInjectExpiredSessionNonce is
// ExportedInjectExpiredNonce's session-nonce-set counterpart, for
// evictExpiredSessionNoncesLocked coverage.
func (s *Service) ExportedInjectExpiredSessionNonce(rawNonce string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.consumedSessionNonces[s.nonceKey(rawNonce)] = core.UnixTime(0)
}
