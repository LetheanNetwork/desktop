// SPDX-Licence-Identifier: EUPL-1.2

// MARKETPLACE SIGNING SUBSTRATE — local placeholder.
//
// TODO: refactor to core/app once upstream exports land:
//   - app.Sign / app.Verify (replacing local ed25519 wrappers)
//   - app.ParsePublicKey (replacing local PEM parser)
//   - app.CanonicaliseCBOR (replacing local CBOR canonicaliser)
//
// Tracked by Mantis #1646 (precursor) — not blocking Wave 1 ship.
// Once #1646 lands, swap implementations in this file; consumer
// callsites (manifest.go, registry.go) should not need changes.
//
// signing.go — marketplace bundle-manifest signing primitives for the
// DREAD F1 / N1 / N2 / N3 / F4 closure (Mantis #1637 / #1647 / #1650
// / #1648 / #1644). Three layered concerns live here:
//
//  1. CanonicaliseManifest — RFC8949 deterministic CBOR encoding of a
//     BundleManifest with the Signature field zeroed. Output is the
//     "signed bytes" both sign and verify operate over. Re-marshal
//     malleability (DREAD F1 CRITICAL — distinct YAML serialisations
//     of the same logical manifest hashing differently) is closed by
//     routing through a single canonical encoder rather than the
//     manifest's wire YAML.
//
//  2. ed25519 sign / verify — direct crypto/ed25519 stdlib. Banned-
//     imports exception: see top-of-file TODO. Refactor surface when
//     core/app exports land per project_corego_export_gaps.md.
//
//  3. Trusted-keys store — JSON file at
//     ~/Lethean/conf/marketplace/trusted_keys.json (mode 0o600). Loader
//     enforces (a) name-keyid disambiguation invariant (DREAD N1 —
//     same name with different keyid REJECTS at load with typed error
//     + audit row) and (b) priority order is the array order (first
//     match wins). Mutations route through trustedKeysWrite which
//     emits EventMarketplaceTrustedKeyMutated with old + new
//     fingerprint, reason, and the operator-confirmation context.
//
// Usage example (internal):
//
//	canonical := marketplace.CanonicaliseManifest(m)
//	sig := marketplace.Sign(privkey, canonical)
//	if !marketplace.Verify(pubkey, canonical, sig) {
//	    return core.Fail(core.E(verifyOp, sigInvalidReason, nil))
//	}

package marketplace

import (
	"crypto/ed25519"

	core "dappco.re/go"

	"github.com/fxamacker/cbor/v2"
)

const (
	signingOp           = "marketplace.Sign"
	verifyOp            = "marketplace.Verify"
	parsePubKeyOp       = "marketplace.ParsePublicKey"
	canonicaliseOp      = "marketplace.CanonicaliseManifest"
	trustedKeysLoadOp   = "marketplace.TrustedKeysLoad"
	trustedKeysWriteOp  = "marketplace.TrustedKeysWrite"
	trustedKeysFileName = "trusted_keys.json"

	// maxTrustedKeyIDChars is the DREAD v2 N3 cap on the keyid string
	// (Mantis #1648). Long values aren't structurally signing-relevant
	// and become a log-bloat / UI-overflow vector; gate at 64 chars to
	// match the SHA256-hex fingerprint shape callers are likely to
	// adopt.
	maxTrustedKeyIDChars = 64

	// sigCorruptReason is the typed reason code emitted when a `.sig`
	// fetch returns bytes that are NOT a valid ed25519 signature shape
	// (wrong length / encoding error). Distinct from sigInvalidReason
	// so the audit + UI surfaces can distinguish "the wire bytes were
	// malformed" (look at the mirror) from "the bytes parsed but the
	// signature didn't match" (look at the key).
	sigCorruptReason = "sig.corrupt"

	// sigInvalidReason is emitted when a `.sig` parses cleanly but the
	// signature does not verify against any trusted-key for the
	// manifest's KeyID. DREAD F1 closes the malleability vector; this
	// reason fires when the verify chain rejects despite encoding
	// being well-formed.
	sigInvalidReason = "sig.invalid"

	// sigRotationRaceReason fires when the manifest body and `.sig`
	// fetch land with different ETag values, indicating the mirror
	// served a rotated body between the two requests. DREAD v2 N2
	// HIGH (Mantis #1650) — without this gate, an attacker who can
	// briefly serve a hostile body between the .sig fetch and the
	// body fetch can defeat the signature check.
	sigRotationRaceReason = "sig.rotation_race"

	// reasonAdd / reasonReplace / reasonRemove / reasonDuplicate are
	// the closed-set Meta["reason"] literals on
	// EventMarketplaceTrustedKeyMutated rows per the
	// pkg/audit/types.go const block.
	reasonAdd       = "add"
	reasonReplace   = "replace"
	reasonRemove    = "remove"
	reasonDuplicate = "duplicate_name_keyid_mismatch"
)

// TrustedKey is one entry in the trusted_keys.json store.
//
// Name is the human-readable priority alias (e.g. "lthn-official").
// Priority order is the array order — first match wins when multiple
// keys could verify a given signature.
//
// KeyID is the SHA256 fingerprint of the public key (hex) capped at
// maxTrustedKeyIDChars. The manifest's declared KeyID field is matched
// against this field to select the verifying key.
//
// Pubkey is the base64-encoded ed25519 public key bytes (raw key, 32
// bytes pre-encoding).
//
// AddedAt + AddedByAccount form the chain-of-custody trail for the
// EventMarketplaceTrustedKeyMutated audit row.
type TrustedKey struct {
	Name            string `json:"name"`
	KeyID           string `json:"keyid"`
	Pubkey          string `json:"pubkey"`
	AddedAt         string `json:"added_at"`
	AddedByAccount  string `json:"added_by_account"`
}

// TrustedKeysFile is the on-disk shape at
// ~/Lethean/conf/marketplace/trusted_keys.json.
type TrustedKeysFile struct {
	Keys []TrustedKey `json:"keys"`
}

// CanonicaliseManifest returns the deterministic CBOR encoding of m
// with the Signature field zeroed. This is the byte sequence Sign
// signs and Verify verifies — the manifest's wire YAML form is
// signing-irrelevant.
//
// DREAD F1 CRITICAL (Mantis #1637) — without this routing, distinct
// equivalent YAML serialisations of the same logical manifest produce
// different signed bytes, and an attacker can re-marshal-and-resign a
// previously-trusted manifest to defeat detached signature comparison.
// CBOR/RFC8949 canonical encoding (sorted map keys, deterministic
// int encoding, no indefinite-length items) closes the malleability
// vector at the encoder boundary.
//
// Usage example:
//
//	canonical := marketplace.CanonicaliseManifest(m)
//	r := marketplace.Sign(priv, canonical)
//	if r.OK { sig := r.Value.([]byte) }
func CanonicaliseManifest(m BundleManifest) core.Result {
	cleaned := m
	cleaned.Signature = nil
	mode, err := cbor.CanonicalEncOptions().EncMode()
	if err != nil {
		return core.Fail(core.E(canonicaliseOp,
			"cbor canonical encmode init failed", err))
	}
	raw, err := mode.Marshal(cleaned)
	if err != nil {
		return core.Fail(core.E(canonicaliseOp,
			"cbor canonical marshal failed", err))
	}
	return core.Ok(raw)
}

// Sign signs the canonical bytes with privkey, returning the
// detached signature.
//
// Usage example:
//
//	r := marketplace.Sign(priv, canonical)
//	if r.OK { sig := r.Value.([]byte) }
func Sign(privkey ed25519.PrivateKey, canonical []byte) core.Result {
	if len(privkey) != ed25519.PrivateKeySize {
		return core.Fail(core.E(signingOp,
			core.Sprintf("private key size %d (want %d)",
				len(privkey), ed25519.PrivateKeySize), nil))
	}
	sig := ed25519.Sign(privkey, canonical)
	return core.Ok(sig)
}

// Verify reports whether sig is a valid signature of canonical under
// pubkey. Distinguishes corrupt-signature (wrong length / encoding)
// from invalid-signature (parses but mismatches) via the returned
// reason code so audit + UI surfaces stamp the right facet.
//
// Returns Ok(nil) on verify success. On failure, Result.Error()
// contains either sigCorruptReason or sigInvalidReason as a stable
// prefix so callers can grep.
//
// Usage example:
//
//	r := marketplace.Verify(pub, canonical, sig)
//	if !r.OK {
//	    // r.Error() starts with "sig.corrupt: " or "sig.invalid: "
//	}
func Verify(pubkey ed25519.PublicKey, canonical, sig []byte) core.Result {
	if len(pubkey) != ed25519.PublicKeySize {
		return core.Fail(core.E(verifyOp,
			sigCorruptReason+": public key size "+
				core.Sprintf("%d", len(pubkey))+
				" (want "+core.Sprintf("%d", ed25519.PublicKeySize)+")", nil))
	}
	if len(sig) != ed25519.SignatureSize {
		return core.Fail(core.E(verifyOp,
			sigCorruptReason+": signature size "+
				core.Sprintf("%d", len(sig))+
				" (want "+core.Sprintf("%d", ed25519.SignatureSize)+")", nil))
	}
	if !ed25519.Verify(pubkey, canonical, sig) {
		return core.Fail(core.E(verifyOp,
			sigInvalidReason+": signature does not verify under key", nil))
	}
	return core.Ok(nil)
}

// ParsePublicKey decodes a base64-encoded raw ed25519 public key.
// Wrappers in this file all consume the raw 32-byte key form rather
// than PEM since the trusted_keys.json store carries base64 pubkey
// bytes directly — keeps the on-disk schema simple and forecloses
// PEM-armouring parser bugs that have historically been a source of
// signature-bypass CVEs.
//
// Usage example:
//
//	r := marketplace.ParsePublicKey("MCowBQYDK2VwAyEA...")
//	if r.OK { pub := r.Value.(ed25519.PublicKey) }
func ParsePublicKey(b64 string) core.Result {
	r := core.Base64Decode(core.Trim(b64))
	if !r.OK {
		return core.Fail(core.E(parsePubKeyOp,
			"public key not valid base64", nil))
	}
	raw, _ := r.Value.([]byte)
	if len(raw) != ed25519.PublicKeySize {
		return core.Fail(core.E(parsePubKeyOp,
			core.Sprintf("public key length %d (want %d)",
				len(raw), ed25519.PublicKeySize), nil))
	}
	return core.Ok(ed25519.PublicKey(raw))
}

// trustedKeysPath returns the on-disk location of the trusted_keys.json
// store. UserHomeDir failures fall back to /tmp so unit tests that
// shim core.UserHomeDir via env still find the file deterministically.
func trustedKeysPath() string {
	homeR := core.UserHomeDir()
	if homeR.OK {
		return core.PathJoin(homeR.Value.(string),
			"Lethean", "conf", "marketplace", trustedKeysFileName)
	}
	return core.PathJoin("/tmp", "lthn-marketplace", trustedKeysFileName)
}

// LoadTrustedKeys reads the trusted_keys.json store from disk and
// returns the parsed list. DREAD v2 N1 HIGH (Mantis #1647) — same
// name appearing with different keyid REJECTS at load with typed
// error + EventMarketplaceTrustedKeyMutated audit row at outcome
// "denied", reason="duplicate_name_keyid_mismatch". Without this
// gate, an attacker who can append a row to the store can shadow a
// legitimate trust entry by reusing its priority alias.
//
// Returns Ok with TrustedKeysFile on success. Empty file (file
// absent) is NOT an error — bootstrap state has no trusted keys
// yet.
//
// Usage example:
//
//	r := marketplace.LoadTrustedKeys()
//	if r.OK { tf := r.Value.(marketplace.TrustedKeysFile) }
func LoadTrustedKeys() core.Result {
	path := trustedKeysPath()
	statR := core.Stat(path)
	if !statR.OK {
		return core.Ok(TrustedKeysFile{})
	}
	readR := core.ReadFile(path)
	if !readR.OK {
		return core.Fail(core.E(trustedKeysLoadOp,
			"trusted_keys.json read failed", nil))
	}
	raw, _ := readR.Value.([]byte)
	var tf TrustedKeysFile
	if r := core.JSONUnmarshal(raw, &tf); !r.OK {
		return core.Fail(core.E(trustedKeysLoadOp,
			"trusted_keys.json parse failed", nil))
	}
	// N1 invariant — same Name with a different KeyID is REJECT.
	seenNameKeyID := map[string]string{}
	for _, k := range tf.Keys {
		name := core.Trim(k.Name)
		keyid := core.Trim(k.KeyID)
		if name == "" || keyid == "" {
			return core.Fail(core.E(trustedKeysLoadOp,
				"trusted_keys.json: name and keyid are required", nil))
		}
		if len(keyid) > maxTrustedKeyIDChars {
			return core.Fail(core.E(trustedKeysLoadOp,
				core.Sprintf("trusted_keys.json: keyid exceeds %d-char cap (got %d)",
					maxTrustedKeyIDChars, len(keyid)), nil))
		}
		if prior, ok := seenNameKeyID[name]; ok && prior != keyid {
			emitTrustedKeyMutated(name, prior, keyid, reasonDuplicate, false)
			return core.Fail(core.E(trustedKeysLoadOp,
				"trusted_keys.json: duplicate name with different keyid: "+name, nil))
		}
		seenNameKeyID[name] = keyid
	}
	return core.Ok(tf)
}

// WriteTrustedKeysInput carries the operator-confirmation context that
// trustedKeysWrite stamps on the EventMarketplaceTrustedKeyMutated
// audit row. OperatorConfirmed must be set true ONLY when the host UI
// has gathered an explicit "I understand" confirmation from the
// operator. Programmatic callers (tests, automation) that assert true
// without UI evidence still produce the row — the auditor can spot
// the missing confirmation chain.
type WriteTrustedKeysInput struct {
	Keys               []TrustedKey
	Reason             string
	OperatorConfirmed  bool
	OperatorAccountID  string
}

// WriteTrustedKeys persists keys to ~/Lethean/conf/marketplace/trusted_keys.json
// at mode 0o600 and emits EventMarketplaceTrustedKeyMutated per delta
// (added / replaced / removed). DREAD F4 HIGH (Mantis #1644) — every
// trust-store mutation leaves a forensic row carrying old + new
// fingerprint + reason + operator-confirmation context.
//
// Usage example:
//
//	r := marketplace.WriteTrustedKeys(marketplace.WriteTrustedKeysInput{
//	    Keys: []marketplace.TrustedKey{{Name: "lthn-official", KeyID: "...", Pubkey: "..."}},
//	    OperatorConfirmed: true,
//	    OperatorAccountID: "acct_abc",
//	})
func WriteTrustedKeys(in WriteTrustedKeysInput) core.Result {
	prior := TrustedKeysFile{}
	if r := LoadTrustedKeys(); r.OK {
		prior, _ = r.Value.(TrustedKeysFile)
	}

	next := TrustedKeysFile{Keys: in.Keys}
	for _, k := range next.Keys {
		name := core.Trim(k.Name)
		keyid := core.Trim(k.KeyID)
		if name == "" || keyid == "" {
			return core.Fail(core.E(trustedKeysWriteOp,
				"trusted key requires name and keyid", nil))
		}
		if len(keyid) > maxTrustedKeyIDChars {
			return core.Fail(core.E(trustedKeysWriteOp,
				core.Sprintf("keyid exceeds %d-char cap (got %d)",
					maxTrustedKeyIDChars, len(keyid)), nil))
		}
	}

	body := core.JSONMarshal(next)
	if !body.OK {
		return core.Fail(core.E(trustedKeysWriteOp,
			"trusted_keys.json marshal failed", nil))
	}
	rawBody, _ := body.Value.([]byte)

	path := trustedKeysPath()
	dir := core.PathDir(path)
	_ = core.MkdirAll(dir, 0o700)
	w := core.WriteFile(path, rawBody, 0o600)
	if !w.OK {
		return core.Fail(core.E(trustedKeysWriteOp,
			"trusted_keys.json write failed", nil))
	}

	// Emit one row per (name, delta) pair.
	priorByName := map[string]TrustedKey{}
	for _, k := range prior.Keys {
		priorByName[core.Trim(k.Name)] = k
	}
	nextByName := map[string]TrustedKey{}
	for _, k := range next.Keys {
		nextByName[core.Trim(k.Name)] = k
	}
	reason := core.Trim(in.Reason)

	for name, nk := range nextByName {
		if pk, ok := priorByName[name]; ok {
			if core.Trim(pk.KeyID) == core.Trim(nk.KeyID) {
				continue // no change
			}
			r := reason
			if r == "" {
				r = reasonReplace
			}
			emitTrustedKeyMutated(name,
				fingerprintFrom(pk),
				fingerprintFrom(nk),
				r, in.OperatorConfirmed)
		} else {
			r := reason
			if r == "" {
				r = reasonAdd
			}
			emitTrustedKeyMutated(name, "", fingerprintFrom(nk),
				r, in.OperatorConfirmed)
		}
	}
	for name, pk := range priorByName {
		if _, ok := nextByName[name]; ok {
			continue
		}
		r := reason
		if r == "" {
			r = reasonRemove
		}
		emitTrustedKeyMutated(name, fingerprintFrom(pk), "",
			r, in.OperatorConfirmed)
	}

	return core.Ok(next)
}

// fingerprintFrom returns the first 16 hex chars of SHA256(pubkey) as
// the audit-row fingerprint. Short prefix is enough to distinguish
// keys forensically without bloating audit Meta with full hashes.
func fingerprintFrom(k TrustedKey) string {
	raw := core.Base64Decode(core.Trim(k.Pubkey))
	if !raw.OK {
		return ""
	}
	b, _ := raw.Value.([]byte)
	hex := core.SHA256Hex(b)
	if len(hex) > 16 {
		return hex[:16]
	}
	return hex
}

// emitTrustedKeyMutated is implemented in audit_emit.go alongside the
// rest of the marketplace audit emit helpers — kept there so the
// pkg/audit substrate boundary stays in one file per the
// audit_emit.go package convention.
