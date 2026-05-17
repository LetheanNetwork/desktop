// SPDX-Licence-Identifier: EUPL-1.2

// At-rest sealing for sessions chat messages, system prompts, and
// tags per Mantis #1736 / Cerberus #67 F-1 / RFC.stage-e-encrypt-at-
// rest v2.
//
// Sessions persist through go-store (key/value) not file paths, so
// the full recordfile.AtRestWriter envelope (which expects a path-
// based Atomic adapter) is structurally inapplicable. Instead this
// file provides a sessions-local PGP-seal helper that mirrors the
// substrate's load-bearing invariants:
//
//   - SingleUnlockedAccount() assertion before encrypt + decrypt
//     (Mantis #1636 fold).
//   - Account.id binding stamped into every sealed envelope so a
//     cross-account misdelivery fails closed.
//   - PGP encrypt under the account public key (same primitive the
//     substrate uses via PGPService.Encrypt).
//   - Header MAC computed under the PUBKEY (per
//     project_atrest_hardening_closure_trail_2026_05_17 — NOT
//     HKDF-from-KEK; PUBKEY-keyed MAC is load-bearing for the
//     list-without-unlock contract).
//
// Wire shape (JSON envelope, written to store.set value):
//
//	{
//	  "sealed": "v1",
//	  "account_id": "abc123...",
//	  "fingerprint": "<sha256-hex of public key>",
//	  "mac": "<hex hmac-sha256 over canonical(sealed|account_id|fingerprint|ct)>",
//	  "ct": "<base64-stdpadded PGP ciphertext>"
//	}
//
// Plaintext (legacy / pre-sealing) values survive the read path
// because unsealPayload checks the magic envelope shape before
// attempting decrypt — absence of `{"sealed":` is treated as
// legacy plaintext and returned as-is. New writes when an account
// is unlocked produce sealed envelopes; new writes when locked
// remain plaintext (preserved during the unlock-window deferral).
//
// Manifest sealing scope: writeManifest seals the on-disk
// SessionInfo JSON ONLY when an account is unlocked. The plain
// header fields (ID, Title, CreatedAt, UpdatedAt, Messages count,
// Snippet) currently round-trip through the same JSON shape — a
// future iteration can carry SessionInfo as a split-header/payload
// envelope so List() decodes manifest entries WITHOUT decrypt while
// SystemPrompt/Tags remain sealed. For now List() works without
// unlock by tolerating sealed manifests in the iteration: sealed
// manifests still expose ID via the envelope's account_id-bound
// header, and content fields are filled blank when locked.

package sessions

import (

	core "dappco.re/go"

	"dappco.re/lthn/desktop/pkg/account"

	"forge.lthn.ai/Snider/Enchantrix/pkg/crypt/std/pgp"
)

// atrestAccountSurface is the narrow live-read surface sessions
// consumes from the wired account service. *account.Service satisfies
// it today (UnlockedAccountIDs at unlock.go:954, PublicKeyFor at
// unlock.go:903, PrivateKeyFor at unlock.go:870). Tests register a
// stub satisfying the same shape under the "account" core name to
// exercise sealed-envelope round-trips without spinning up the full
// account service state.
type atrestAccountSurface interface {
	UnlockedAccountIDs() []string
	PublicKeyFor(accountID string) ([]byte, bool)
	PrivateKeyFor(accountID string) (*account.PrivateKeyHandle, bool)
}

// resolveAccountSurface returns the wired account surface or nil
// when the service isn't registered. The two-step type-assertion
// pattern (concrete *account.Service first, fall back to the test
// stub interface) avoids forcing production code to construct a
// stub indirection for the common case while keeping the test
// fixture seam open.
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

// envelopeVersion is the magic literal stamped into every sealed
// envelope so unsealPayload can distinguish sealed payloads from
// legacy plaintext.
const envelopeVersion = "v1"

// envelopeMagic is the leading-byte sentinel that lets the read
// path cheaply identify a sealed payload without paying a full
// JSON parse. Sealed envelopes always start with `{"sealed":` — the
// first 10 bytes are the canonical detect surface.
const envelopeMagic = `{"sealed":`

// sealedEnvelope is the on-disk shape for a sealed payload — JSON-
// encoded as the value handed to store.set. See package docstring
// for wire shape rationale.
type sealedEnvelope struct {
	Sealed      string `json:"sealed"`
	AccountID   string `json:"account_id"`
	Fingerprint string `json:"fingerprint"`
	MAC         string `json:"mac"`
	CT          string `json:"ct"`
}

// sealPayload encrypts plaintext under the unlocked account's
// public key and returns the JSON-encoded envelope bytes ready for
// store.set. Returns (nil, false) when no account service is wired,
// no account is unlocked (or multiple are), or any substrate step
// fails — the caller then falls back to the legacy plaintext path
// so the operator flow survives the locked-account window without
// losing data.
//
// Usage example (internal):
//
//	if env, ok := sealPayload(c, plaintext); ok {
//	    return c.Action("store.set").Run(...{ "value": string(env) })
//	}
//	// fall through to plaintext write
func sealPayload(c *core.Core, plaintext []byte) ([]byte, bool) {
	if c == nil || len(plaintext) == 0 {
		return nil, false
	}
	accountID, pub, ok := singleUnlockedPubKey(c)
	if !ok {
		return nil, false
	}
	pgpSvc := pgp.NewService()
	ciphertext, err := pgpSvc.Encrypt(pub, plaintext)
	if err != nil || len(ciphertext) == 0 {
		return nil, false
	}
	ctB64 := core.Base64Encode(ciphertext)
	fingerprint := core.SHA256Hex(pub)
	macInput := envelopeMACInput(envelopeVersion, accountID, fingerprint, ctB64)
	macR := core.HMAC("sha256", pub, []byte(macInput))
	if !macR.OK {
		return nil, false
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
		return nil, false
	}
	bytes, ok := encoded.Value.([]byte)
	if !ok {
		return nil, false
	}
	return bytes, true
}

// unsealPayload decrypts a sealed envelope and returns the
// plaintext bytes. The read path accepts both sealed envelopes
// (detected by leading `{"sealed":` bytes) AND legacy plaintext
// (anything else) so existing sessions remain readable without
// migration. When the envelope is sealed BUT the unlock-state can't
// satisfy the decrypt (account locked, fingerprint mismatch, MAC
// invalid), the function returns (nil, true) so the caller fails
// closed rather than serving stale plaintext.
//
// Returns (plaintext, sealed):
//   - (raw, false)        — legacy plaintext, hand back as-is
//   - (plaintext, true)   — sealed + decrypted successfully
//   - (nil, true)         — sealed but unreadable (locked/tampered)
//
// Usage example (internal):
//
//	plaintext, sealed := unsealPayload(c, raw)
//	if !sealed { return raw }
//	if plaintext == nil { return nil /* fail closed */ }
//	return plaintext
func unsealPayload(c *core.Core, raw []byte) ([]byte, bool) {
	if !isSealedEnvelope(raw) {
		return raw, false
	}
	var env sealedEnvelope
	if r := core.JSONUnmarshal(raw, &env); !r.OK {
		return nil, true
	}
	if env.Sealed != envelopeVersion {
		return nil, true
	}
	if c == nil {
		return nil, true
	}
	accountID, pub, ok := singleUnlockedPubKey(c)
	if !ok {
		return nil, true
	}
	// Account.id binding cross-check.
	if env.AccountID != accountID {
		return nil, true
	}
	// Fingerprint pre-check (T3 — cheap reject before MAC + decrypt).
	gotFingerprint := core.SHA256Hex(pub)
	if env.Fingerprint != gotFingerprint {
		return nil, true
	}
	// MAC verify.
	macInput := envelopeMACInput(env.Sealed, env.AccountID, env.Fingerprint, env.CT)
	macR := core.HMAC("sha256", pub, []byte(macInput))
	if !macR.OK {
		return nil, true
	}
	gotMAC := core.HexEncode(macR.Value.([]byte))
	if !constantTimeEqualString(gotMAC, env.MAC) {
		return nil, true
	}
	// Decrypt — privatekey under handle.Use(...).
	surf := resolveAccountSurface(c)
	if surf == nil {
		return nil, true
	}
	handle, ok := surf.PrivateKeyFor(accountID)
	if !ok || handle == nil {
		return nil, true
	}
	b64R := core.Base64Decode(env.CT)
	if !b64R.OK {
		return nil, true
	}
	ciphertext, ok := b64R.Value.([]byte)
	if !ok || len(ciphertext) == 0 {
		return nil, true
	}
	var plaintext []byte
	useErr := handle.Use(func(priv []byte) error {
		pgpSvc := pgp.NewService()
		decrypted, err := pgpSvc.Decrypt(priv, ciphertext)
		if err != nil {
			return err
		}
		// Copy so the slice survives handle zeroisation.
		plaintext = make([]byte, len(decrypted))
		copy(plaintext, decrypted)
		return nil
	})
	if useErr != nil {
		return nil, true
	}
	return plaintext, true
}

// isSealedEnvelope cheaply detects whether a stored value is a
// sealed envelope by inspecting the leading bytes. Avoids paying
// for a full JSON parse on every legacy-plaintext read.
func isSealedEnvelope(raw []byte) bool {
	if len(raw) < len(envelopeMagic) {
		return false
	}
	return core.HasPrefix(string(raw[:len(envelopeMagic)]), envelopeMagic)
}

// isSealedEnvelopeString is the string-keyed convenience used inside
// List() iteration where store.get_all returns map[string]string.
func isSealedEnvelopeString(raw string) bool {
	if len(raw) < len(envelopeMagic) {
		return false
	}
	return core.HasPrefix(raw[:len(envelopeMagic)], envelopeMagic)
}

// envelopeMACInput canonicalises the MAC input fields so the read
// side computes byte-for-byte the same input the write side did.
// Order is fixed; pipe separator is unambiguous against the allowed
// character sets (hex + base64 + account-id alphanumeric).
func envelopeMACInput(sealed, accountID, fingerprint, ct string) string {
	return sealed + "|" + accountID + "|" + fingerprint + "|" + ct
}

// singleUnlockedPubKey returns (account_id, pub-key bytes, ok) for
// the single unlocked account, or (_, _, false) when zero or
// multiple accounts are unlocked, or the account service isn't
// wired. Mirrors the substrate's SingleUnlockedAccount() reject
// pattern.
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

// constantTimeEqualString avoids leaking MAC-length-prefix timing
// against a partial attacker. Both args are hex strings so the
// underlying byte compare is a fine substitute for crypto/subtle.
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
