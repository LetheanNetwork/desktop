// SPDX-Licence-Identifier: EUPL-1.2

// At-rest sealing for user-prompt fields persisted into tasks_*
// DuckDB tables — Mantis #1727 / Cerberus #64 F-4. The persistence
// substrate is orm (DuckDB-backed) rather than file paths, so the
// full recordfile.AtRestWriter envelope (which expects a path-based
// Atomic adapter) is structurally inapplicable here.
//
// Sealing scope (per the brief):
//
//   - Issue.Description      — user-prompt body, sealed on write
//   - Note.Body              — user-prompt body, sealed on write
//   - Job.Payload            — handler-input JSON, sealed on write
//
// Orchestration metadata (project, kind, state, severity, priority,
// assignee, reporter, timestamps) stays plain so the orchestration
// substrate continues working without unlock — List() / Where()
// queries by these columns remain effective whether the account is
// unlocked or not. The encrypted-field round-trip happens at the
// FIELD boundary; everything else is structural.
//
// Wire shape (single-string envelope, JSON-encoded as the column
// value handed to orm):
//
//	{
//	  "sealed": "v1",
//	  "account_id": "abc123...",
//	  "fingerprint": "<sha256-hex of public key>",
//	  "mac": "<hex hmac-sha256 over canonical(sealed|account_id|fingerprint|ct)>",
//	  "ct": "<base64-stdpadded PGP ciphertext>"
//	}
//
// Legacy plaintext values (pre-this-change) round-trip unchanged
// because unsealField checks the `{"sealed":` magic prefix before
// attempting decrypt. New writes when an account is unlocked
// produce sealed strings; new writes when locked stay plaintext so
// CLI-driven first-launch flows survive the unlock-window deferral.
//
// Substrate-level invariants mirrored from recordfile.AtRestWriter:
//
//   - SingleUnlockedAccount() reject (zero-or-multi-unlock → no seal)
//   - account.id binding stamped into every sealed envelope
//   - PUBKEY-keyed header MAC per
//     project_atrest_hardening_closure_trail_2026_05_17
//   - Single-use PrivateKeyHandle.Use closure for decrypt

package tasks

import (

	core "dappco.re/go"

	"dappco.re/lthn/desktop/pkg/account"

	"github.com/Snider/Enchantrix/pkg/crypt/std/pgp"
)

// envelopeVersion is the magic literal stamped into every sealed
// envelope.
const envelopeVersion = "v1"

// envelopeMagic is the leading-byte sentinel that lets the read
// path cheaply identify a sealed payload without paying a full
// JSON parse. Sealed envelopes always start with `{"sealed":`.
const envelopeMagic = `{"sealed":`

// sealedEnvelope is the on-disk shape for a sealed string column.
type sealedEnvelope struct {
	Sealed      string `json:"sealed"`
	AccountID   string `json:"account_id"`
	Fingerprint string `json:"fingerprint"`
	MAC         string `json:"mac"`
	CT          string `json:"ct"`
}

// atrestAccountSurface is the narrow live-read surface tasks
// consumes from the wired account service. *account.Service satisfies
// it today (UnlockedAccountIDs at unlock.go:954, PublicKeyFor at
// unlock.go:903, PrivateKeyFor at unlock.go:870). Tests register a
// stub satisfying the same shape under the "account" core name.
type atrestAccountSurface interface {
	UnlockedAccountIDs() []string
	PublicKeyFor(accountID string) ([]byte, bool)
	PrivateKeyFor(accountID string) (*account.PrivateKeyHandle, bool)
}

// resolveAccountSurface returns the wired account surface or nil
// when the service isn't registered. Two-step assertion (concrete
// *account.Service first, fall back to the test stub interface)
// avoids forcing production code to construct a stub indirection.
func resolveAccountSurface(c *core.Core) atrestAccountSurface {
	if c == nil {
		return nil
	}
	if svc, ok := core.ServiceFor[*account.Service](c, "account"); ok && svc != nil {
		return svc
	}
	if surf, ok := core.ServiceFor[atrestAccountSurface](c, "account"); ok {
		return surf
	}
	return nil
}

// SealField encrypts a user-prompt string under the unlocked
// account's public key and returns the JSON-encoded envelope as a
// string (suitable for orm column storage). Returns the input
// unchanged when no account is unlocked or any substrate step fails
// — the caller writes plaintext in that case so the operator flow
// survives the locked-account window without losing data.
//
// Exported so other packages migrating into the at-rest contract
// (queue.Enqueue lives in pkg/queue and consumes this from pkg/tasks
// via the Stage E wave) can route their string-column writes
// through one helper.
//
// Usage example:
//
//	issue.Description = tasks.SealField(c, issue.Description)
//	orm.Insert(c, &issue)
func SealField(c *core.Core, plaintext string) string {
	if plaintext == "" {
		return plaintext
	}
	sealed, ok := sealString(c, plaintext)
	if !ok {
		return plaintext
	}
	return sealed
}

// UnsealField decrypts a previously-sealed string column. Sealed
// envelopes (detected by the `{"sealed":` prefix) are decrypted
// via the unlocked private key; legacy plaintext values round-trip
// unchanged. A sealed-but-unreadable value (account locked, MAC
// tampered) returns an empty string so the caller's downstream code
// fails closed rather than rendering ciphertext bytes to the user.
//
// Usage example:
//
//	issue, _, _ := orm.Detail[Issue](r)
//	issue.Description = tasks.UnsealField(c, issue.Description)
func UnsealField(c *core.Core, value string) string {
	if value == "" {
		return value
	}
	if !isSealedEnvelopeString(value) {
		return value
	}
	plaintext, ok := unsealString(c, value)
	if !ok {
		// Sealed-but-unreadable — fail closed rather than leak
		// ciphertext bytes.
		return ""
	}
	return plaintext
}

// sealString is the internal write-side worker — same shape as the
// sessions atrest helper. Returns (envelope-json, true) on success
// or ("", false) when sealing isn't possible (account locked, no
// keys, encrypt failure).
func sealString(c *core.Core, plaintext string) (string, bool) {
	accountID, pub, ok := singleUnlockedPubKey(c)
	if !ok {
		return "", false
	}
	pgpSvc := pgp.NewService()
	ciphertext, err := pgpSvc.Encrypt(pub, []byte(plaintext))
	if err != nil || len(ciphertext) == 0 {
		return "", false
	}
	ctB64 := core.Base64Encode(ciphertext)
	fingerprint := core.SHA256Hex(pub)
	macInput := envelopeMACInput(envelopeVersion, accountID, fingerprint, ctB64)
	macR := core.HMAC("sha256", pub, []byte(macInput))
	if !macR.OK {
		return "", false
	}
	macHex := core.HexEncode(macR.Value.([]byte))
	env := sealedEnvelope{
		Sealed:      envelopeVersion,
		AccountID:   accountID,
		Fingerprint: fingerprint,
		MAC:         macHex,
		CT:          ctB64,
	}
	encoded := core.JSONMarshal(env)
	if !encoded.OK {
		return "", false
	}
	bytes, ok := encoded.Value.([]byte)
	if !ok {
		return "", false
	}
	return string(bytes), true
}

// unsealString is the internal read-side worker. Returns
// (plaintext, true) on a successful decrypt, ("", false) when the
// envelope is malformed / account locked / MAC invalid.
func unsealString(c *core.Core, raw string) (string, bool) {
	var env sealedEnvelope
	if r := core.JSONUnmarshalString(raw, &env); !r.OK {
		return "", false
	}
	if env.Sealed != envelopeVersion {
		return "", false
	}
	accountID, pub, ok := singleUnlockedPubKey(c)
	if !ok {
		return "", false
	}
	if env.AccountID != accountID {
		return "", false
	}
	gotFingerprint := core.SHA256Hex(pub)
	if env.Fingerprint != gotFingerprint {
		return "", false
	}
	macInput := envelopeMACInput(env.Sealed, env.AccountID, env.Fingerprint, env.CT)
	macR := core.HMAC("sha256", pub, []byte(macInput))
	if !macR.OK {
		return "", false
	}
	gotMAC := core.HexEncode(macR.Value.([]byte))
	if !constantTimeEqualString(gotMAC, env.MAC) {
		return "", false
	}
	surf := resolveAccountSurface(c)
	if surf == nil {
		return "", false
	}
	handle, ok := surf.PrivateKeyFor(accountID)
	if !ok || handle == nil {
		return "", false
	}
	b64R := core.Base64Decode(env.CT)
	if !b64R.OK {
		return "", false
	}
	ciphertext, ok := b64R.Value.([]byte)
	if !ok || len(ciphertext) == 0 {
		return "", false
	}
	var plaintext []byte
	useErr := handle.Use(func(priv []byte) error {
		pgpSvc := pgp.NewService()
		decrypted, err := pgpSvc.Decrypt(priv, ciphertext)
		if err != nil {
			return err
		}
		plaintext = make([]byte, len(decrypted))
		copy(plaintext, decrypted)
		return nil
	})
	if useErr != nil {
		return "", false
	}
	return string(plaintext), true
}

// isSealedEnvelopeString cheaply detects whether a stored value is
// a sealed envelope.
func isSealedEnvelopeString(raw string) bool {
	if len(raw) < len(envelopeMagic) {
		return false
	}
	return raw[:len(envelopeMagic)] == envelopeMagic
}

// envelopeMACInput canonicalises the MAC input fields. Order is
// fixed; pipe separator is unambiguous against allowed character
// sets (hex + base64 + account-id alphanumeric).
func envelopeMACInput(sealed, accountID, fingerprint, ct string) string {
	return sealed + "|" + accountID + "|" + fingerprint + "|" + ct
}

// singleUnlockedPubKey returns (account_id, pub-key bytes, ok) for
// the single unlocked account, or (_, _, false) when zero or
// multiple accounts are unlocked. Mirrors the substrate's
// SingleUnlockedAccount() reject pattern.
func singleUnlockedPubKey(c *core.Core) (string, []byte, bool) {
	surf := resolveAccountSurface(c)
	if surf == nil {
		return "", nil, false
	}
	ids := surf.UnlockedAccountIDs()
	if len(ids) != 1 {
		return "", nil, false
	}
	accountID := ids[0]
	if accountID == "" {
		return "", nil, false
	}
	pub, ok := surf.PublicKeyFor(accountID)
	if !ok || len(pub) == 0 {
		return "", nil, false
	}
	return accountID, pub, true
}

// constantTimeEqualString avoids leaking MAC-length-prefix timing.
// Both args are hex strings so the underlying byte compare is a
// fine substitute for crypto/subtle.
func constantTimeEqualString(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var diff byte
	for i := 0; i < len(a); i++ {
		diff |= a[i] ^ b[i]
	}
	return diff == 0
}
