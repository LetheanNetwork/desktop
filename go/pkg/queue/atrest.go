// SPDX-Licence-Identifier: EUPL-1.2

// At-rest sealing for Job.Payload — Mantis #1727 / Cerberus #64 F-4.
// Job rows persist into DuckDB alongside tasks_* tables; the Payload
// column carries handler-input JSON which frequently contains
// user-prompt strings (lint paths, agent dispatch text, runner
// instructions). Sealing protects that surface against the same
// at-rest snapshot threat the sibling pkg/tasks at-rest helper
// closes for Issue.Description + Note.Body.
//
// Wire shape, invariants, locked-account fallback all mirror the
// sessions + tasks atrest helpers — see pkg/tasks/atrest.go for the
// substrate rationale. Duplicated rather than imported because
// pkg/queue is structurally a peer to pkg/tasks (the IPC cascade
// makes them sibling consumers of pkg/account, NOT one consumer of
// the other), and the helper is small + boundary-bound.

package queue

import (

	core "dappco.re/go"

	"dappco.re/lthn/desktop/pkg/account"

	"forge.lthn.ai/Snider/Enchantrix/pkg/crypt/std/pgp"
)

// envelopeVersion is the magic literal stamped into every sealed
// envelope.
const envelopeVersion = "v1"

// envelopeMagic is the leading-byte sentinel that lets the read
// path cheaply identify a sealed payload without paying a full
// JSON parse.
const envelopeMagic = `{"sealed":`

// sealedEnvelope is the on-disk shape for a sealed Payload column.
type sealedEnvelope struct {
	Sealed      string `json:"sealed"`
	AccountID   string `json:"account_id"`
	Fingerprint string `json:"fingerprint"`
	MAC         string `json:"mac"`
	CT          string `json:"ct"`
}

// atrestAccountSurface is the narrow live-read surface queue
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
// when the service isn't registered.
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

// sealPayload encrypts plaintext under the unlocked account's
// public key and returns the JSON-encoded envelope as a string.
// Returns (input, false) when sealing isn't possible — caller writes
// plaintext in that case so the operator flow survives the locked-
// account window without losing data.
func sealPayload(c *core.Core, plaintext string) (string, bool) {
	if plaintext == "" {
		return plaintext, false
	}
	accountID, pub, ok := singleUnlockedPubKey(c)
	if !ok {
		return plaintext, false
	}
	pgpSvc := pgp.NewService()
	ciphertext, err := pgpSvc.Encrypt(pub, []byte(plaintext))
	if err != nil || len(ciphertext) == 0 {
		return plaintext, false
	}
	ctB64 := core.Base64Encode(ciphertext)
	fingerprint := core.SHA256Hex(pub)
	macInput := envelopeMACInput(envelopeVersion, accountID, fingerprint, ctB64)
	macR := core.HMAC("sha256", pub, []byte(macInput))
	if !macR.OK {
		return plaintext, false
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
		return plaintext, false
	}
	bytes, ok := encoded.Value.([]byte)
	if !ok {
		return plaintext, false
	}
	return string(bytes), true
}

// unsealPayload decrypts a sealed Payload column. Legacy plaintext
// values (no `{"sealed":` prefix) round-trip unchanged so existing
// pending jobs survive the upgrade. A sealed-but-unreadable value
// returns ("", false) so the worker can fail closed rather than
// dispatching the handler with ciphertext bytes.
func unsealPayload(c *core.Core, raw string) (string, bool) {
	if !isSealedEnvelopeString(raw) {
		return raw, true
	}
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

// envelopeMACInput canonicalises the MAC input fields.
func envelopeMACInput(sealed, accountID, fingerprint, ct string) string {
	return sealed + "|" + accountID + "|" + fingerprint + "|" + ct
}

// singleUnlockedPubKey returns the single-unlocked tuple or
// (_, _, false) when locked / multi.
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
