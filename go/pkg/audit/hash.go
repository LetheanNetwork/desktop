// SPDX-Licence-Identifier: EUPL-1.2

// hash.go — RFC.stage-f.md §6.4 account_id hashing at-rest. The audit
// log MUST NOT carry raw account_id values: backup-readers (Time
// Machine, Backblaze, iCloud sync) operate outside the same-UID
// attack-surface boundary that protects ~/Lethean/account/ itself, so
// hashing defends against backup-leak enumeration.
//
// Mechanism: hex(HMAC-SHA256(secret, account_id))[:32] — 32 hex chars
// (16 bytes) is enough for collision resistance at desktop scale.
// Secret is held by the Service (Options.AuditSecret) and derived in
// boot wiring via serverkey.AuditHMACSecret().
//
// Determinism: same raw account_id under same secret yields same hash,
// so Query-by-account still works — the Service hashes the
// QueryInput.AccountID before comparing against on-disk values.

package audit

import (
	core "dappco.re/go"
)

// HashAccountID is the public hashing helper. Exposed so callers
// outside pkg/audit (Query consumers, test fixtures) can hash a raw
// account_id with the same secret + same shape the recorder uses.
//
// Returns the empty string for an empty input or a missing secret —
// downstream Query code treats empty as "no account filter," matching
// the no-account-id-in-Event semantics.
//
// Usage example:
//
//	hashed := audit.HashAccountID(secret, "abc123def4567890")
//	results := svc.Query(audit.QueryInput{AccountID: hashed})
func HashAccountID(secret []byte, accountID string) string {
	if accountID == "" || len(secret) == 0 {
		return ""
	}
	return hashAccountID(secret, accountID)
}

// hashAccountID is the internal hasher — callers MUST go through
// HashAccountID so the empty-input / empty-secret guard runs.
func hashAccountID(secret []byte, accountID string) string {
	mac := core.HMAC("sha256", secret, []byte(accountID))
	if !mac.OK {
		// HMAC only fails on unsupported algorithm — sha256 always
		// works. Defensive fallback: salted SHA-256 still scrambles
		// the input so we don't leak the raw value.
		return core.SHA256HexString(string(secret) + ":" + accountID)[:32]
	}
	digest, _ := mac.Value.([]byte)
	hex := core.HexEncode(digest)
	if len(hex) >= 32 {
		return hex[:32]
	}
	return hex
}
