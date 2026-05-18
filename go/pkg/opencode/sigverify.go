// SPDX-Licence-Identifier: EUPL-1.2

// sigverify.go — supply-chain image signature verification for
// pkg/opencode.Service.UpgradeWithConsent. Cerberus #22 MED-2 /
// Mantis #1622 — pin-by-digest (#1621) blocks a registry tag-swap
// attack but does not protect the case where an attacker controls
// both the registry AND the pin-registration path. Binding the
// digest to a release-engineer key the operator pins out-of-band
// via ~/Lethean/conf/opencode/trusted_publishers.json closes the
// loop.
//
// Shape decisions:
//
//  1. Use marketplace.{Sign,Verify,ParsePublicKey} as the crypto
//     primitive surface — ed25519 raw-base64 keys, not PEM. The
//     marketplace substrate landed for #1637/#1647/#1650/#1648 (Wave 1
//     marketplace signing trio) and is already battle-tested. Per
//     project_corego_export_gaps.md the long-term home is core/app;
//     when that lands, both opencode and marketplace lift in lockstep.
//
//  2. trusted_publishers.json mirrors the marketplace trusted_keys.json
//     shape (same TrustedKey row format: name/keyid/pubkey/added_at/
//     added_by_account) so an operator already familiar with the
//     marketplace trust-store learns nothing new. Path is distinct so
//     marketplace bundle authors and opencode release engineers can be
//     governed independently.
//
//  3. Canonical signing bytes = digest + "\n" + tag + "\n" + release_id
//     (each line trimmed, joined with newline). Deterministic, no
//     CBOR-canonical complexity needed because the three inputs are
//     already bounded strings — the only attack surface is field-
//     delimiter ambiguity (digest "x\ntag1" vs "x" tag "tag1") which
//     the bounded format of sha256:<64hex> + tag-char-restrict closes
//     at the input gate. release_id is treated as opaque + must NOT
//     contain newlines (gated at signature-verify time).
//
//  4. require_signature is a config knob, NOT a per-call input. The
//     operator chooses once whether their deployment requires signed
//     upgrades; UpgradeInput threads SignatureBytes + PublicKeyPEM-
//     equivalent but the policy gate (must-be-signed) lives in
//     Options.UpgradeRequireSignature. This keeps the upgrade RPC
//     surface single-shape regardless of policy.
//
// Usage example (internal):
//
//	canonical := canonicalSigningBytes(in.ImageDigest, tag, in.ReleaseID)
//	if !verifySignatureWithPolicy(s, in, canonical).OK {
//	    return Fail("upgrade.signature_invalid")
//	}

package opencode

import (
	"crypto/ed25519"

	core "dappco.re/go"

	"dappco.re/lthn/desktop/pkg/audit"
	"dappco.re/lthn/desktop/pkg/marketplace"
)

const (
	sigVerifyOp = "opencode.SignatureVerify"

	// trustedPublishersFileName is the on-disk name under
	// ~/Lethean/conf/opencode/. Distinct from marketplace's
	// trusted_keys.json so operators can govern bundle authors and
	// opencode release engineers independently.
	trustedPublishersFileName = "trusted_publishers.json"

	// Closed-set rejection reasons for EventOpencodeImageSignatureRejected.
	// MUST stay in lockstep with the const block in types.go and the
	// audit-constants.ts mirror.
	sigReasonMissing  = "signature_missing"
	sigReasonNoKey    = "key_not_found"
	sigReasonCorrupt  = "sig.corrupt"
	sigReasonInvalid  = "sig.invalid"
	sigReasonNoNewLine = "release_id.newline_forbidden"
)

// trustedPublishersPath returns the on-disk location of the opencode
// trusted_publishers.json store. UserHomeDir failures fall back to
// /tmp so unit tests that shim core.UserHomeDir via env still find
// the file deterministically — mirrors marketplace.trustedKeysPath.
func trustedPublishersPath() string {
	homeR := core.UserHomeDir()
	if homeR.OK {
		return core.PathJoin(homeR.Value.(string),
			"Lethean", "conf", "opencode", trustedPublishersFileName)
	}
	return core.PathJoin("/tmp", "lthn-opencode", trustedPublishersFileName)
}

// loadTrustedPublishers reads trusted_publishers.json and returns the
// parsed list. Mirrors marketplace.LoadTrustedKeys discipline: same
// name with different keyid REJECTS at load (DREAD v2 N1 HIGH); empty
// file (file absent) is NOT an error — bootstrap state has no trusted
// publishers yet, and the caller's require_signature policy decides
// whether that bootstrap is acceptable.
//
// Usage example (internal):
//
//	r := loadTrustedPublishers()
//	if r.OK { tpf := r.Value.(marketplace.TrustedKeysFile) }
func loadTrustedPublishers() core.Result {
	path := trustedPublishersPath()
	statR := core.Stat(path)
	if !statR.OK {
		return core.Ok(marketplace.TrustedKeysFile{})
	}
	readR := core.ReadFile(path)
	if !readR.OK {
		return core.Fail(core.E(sigVerifyOp,
			"trusted_publishers.json read failed", nil))
	}
	raw, _ := readR.Value.([]byte)
	var tf marketplace.TrustedKeysFile
	if r := core.JSONUnmarshal(raw, &tf); !r.OK {
		return core.Fail(core.E(sigVerifyOp,
			"trusted_publishers.json parse failed", nil))
	}
	// Mirror marketplace N1 invariant — same Name with different
	// KeyID is REJECT (an attacker who can append a row to the store
	// would otherwise shadow a legitimate publisher entry).
	seenNameKeyID := map[string]string{}
	for _, k := range tf.Keys {
		name := core.Trim(k.Name)
		keyid := core.Trim(k.KeyID)
		if name == "" || keyid == "" {
			return core.Fail(core.E(sigVerifyOp,
				"trusted_publishers.json: name and keyid are required", nil))
		}
		if prior, ok := seenNameKeyID[name]; ok && prior != keyid {
			return core.Fail(core.E(sigVerifyOp,
				"trusted_publishers.json: duplicate name with different keyid: "+name, nil))
		}
		seenNameKeyID[name] = keyid
	}
	return core.Ok(tf)
}

// canonicalSigningBytes returns the deterministic byte sequence the
// release engineer signed: digest + "\n" + tag + "\n" + release_id.
// Each component is trimmed before join so trailing whitespace can't
// be a malleability vector.
//
// release_id MUST NOT contain a newline (returns "" + ok=false in
// that case). The bounded sha256:<64hex> digest shape and the
// well-defined OCI tag charset close the delimiter-ambiguity vector
// for those two fields at the input gate.
//
// Usage example (internal):
//
//	bytes, ok := canonicalSigningBytes(digest, tag, releaseID)
//	if !ok { return Fail("release_id.newline_forbidden") }
//	r := marketplace.Verify(pub, bytes, sig)
func canonicalSigningBytes(digest, tag, releaseID string) ([]byte, bool) {
	d := core.Trim(digest)
	tg := core.Trim(tag)
	rid := core.Trim(releaseID)
	if core.Contains(rid, "\n") || core.Contains(rid, "\r") {
		return nil, false
	}
	canon := d + "\n" + tg + "\n" + rid
	return []byte(canon), true
}

// verifySignatureForUpgrade applies the require_signature policy to
// the supplied UpgradeInput. Returns Ok(nil) on accept (either the
// policy is off and no signature was supplied, OR the policy is on
// and the signature verified successfully). Returns Fail with a
// typed *core.Err whose Operation is sigVerifyOp and whose Message
// embeds one of the sigReason* literals.
//
// The function emits the audit row exactly once per call — either
// EventOpencodeImageSignatureVerified on success or
// EventOpencodeImageSignatureRejected on every failure facet.
//
// require_signature semantics:
//
//   - true  + no signature supplied → reject with sigReasonMissing
//   - true  + signature supplied   → verify; reject on any mismatch
//   - false + no signature supplied → ACCEPT (legacy / bootstrap)
//   - false + signature supplied   → verify-when-supplied; reject on
//                                    mismatch (defence-in-depth — if
//                                    the operator threaded a sig, we
//                                    treat it as load-bearing)
//
// Usage example (internal — called from UpgradeWithConsent after
// the digest gate passes, before the docker pull side-effect):
//
//	canon, ok := canonicalSigningBytes(in.ImageDigest, tag, in.ReleaseID)
//	if !ok { ... }
//	if r := verifySignatureForUpgrade(s, in, canon); !r.OK { return r }
func verifySignatureForUpgrade(s *Service, in UpgradeInput, canonical []byte) core.Result {
	requireSig := s.requireSignature()
	hasSig := len(in.SignatureBytes) > 0 && len(in.PublicKeyBase64) > 0

	// Policy off + no signature → accept silently. No audit row.
	if !requireSig && !hasSig {
		return core.Ok(nil)
	}

	// Policy on + no signature → reject. The Cerberus #22 MED-2
	// threat model explicitly classes this case as "operator opted
	// into signing but the upgrade pipeline supplied no signature
	// bytes" — typically a misconfigured release pipeline.
	if requireSig && !hasSig {
		emitSignatureRejected(in.ImageDigest, "", sigReasonMissing,
			core.Fail(core.E(sigVerifyOp,
				"upgrade.signature_invalid: require_signature=true but no signature supplied",
				nil)))
		return core.Fail(core.E(sigVerifyOp,
			"upgrade.signature_invalid: "+sigReasonMissing, nil))
	}

	// Parse the operator-supplied public key. ParsePublicKey accepts
	// base64-encoded raw ed25519 bytes (32 bytes pre-encoding) per
	// marketplace's discipline — no PEM armouring (PEM parsers have
	// historically been a source of signature-bypass CVEs).
	pubR := marketplace.ParsePublicKey(string(in.PublicKeyBase64))
	if !pubR.OK {
		emitSignatureRejected(in.ImageDigest, "", sigReasonCorrupt,
			core.Fail(core.E(sigVerifyOp,
				"upgrade.signature_invalid: "+sigReasonCorrupt+" (public key parse failed)",
				pubR.Value.(error))))
		return core.Fail(core.E(sigVerifyOp,
			"upgrade.signature_invalid: "+sigReasonCorrupt+" (public key parse failed)",
			nil))
	}

	// Cross-check against the trusted_publishers.json store — the
	// pubkey must be present in the operator's pinned trust store,
	// not just any well-formed ed25519 key. Mantis #1622 design
	// requires out-of-band publisher pinning; otherwise an attacker
	// with both registry and pin-registration control could supply
	// their own freshly-generated key alongside their malicious
	// digest and pass verification.
	tpR := loadTrustedPublishers()
	if !tpR.OK {
		emitSignatureRejected(in.ImageDigest, "", sigReasonNoKey, tpR)
		return tpR
	}
	tp, _ := tpR.Value.(marketplace.TrustedKeysFile)
	matched := false
	for _, tk := range tp.Keys {
		// Compare the raw pubkey bytes (post-base64-decode) — the
		// store's Pubkey field is base64 of the same 32-byte raw
		// key. Direct string comparison of the base64 form is
		// adequate because marketplace stores canonical encoding.
		if core.Trim(tk.Pubkey) == core.Trim(string(in.PublicKeyBase64)) {
			matched = true
			break
		}
	}
	if !matched {
		emitSignatureRejected(in.ImageDigest, "", sigReasonNoKey,
			core.Fail(core.E(sigVerifyOp,
				"upgrade.signature_invalid: "+sigReasonNoKey+" (pubkey not in trusted_publishers.json)",
				nil)))
		return core.Fail(core.E(sigVerifyOp,
			"upgrade.signature_invalid: "+sigReasonNoKey, nil))
	}

	// Verify the signature over the canonical bytes. marketplace.Verify
	// returns the bounded sig.corrupt / sig.invalid reason codes; we
	// surface them verbatim in the audit row + reject error.
	pub, ok := pubR.Value.(ed25519.PublicKey)
	if !ok {
		emitSignatureRejected(in.ImageDigest, "", sigReasonCorrupt,
			core.Fail(core.E(sigVerifyOp,
				"upgrade.signature_invalid: "+sigReasonCorrupt+" (parse-key type assertion failed)",
				nil)))
		return core.Fail(core.E(sigVerifyOp,
			"upgrade.signature_invalid: "+sigReasonCorrupt+" (parse-key type assertion failed)",
			nil))
	}
	verifyR := marketplace.Verify(pub, canonical, in.SignatureBytes)
	if !verifyR.OK {
		reason := sigReasonInvalid
		if msg := verifyR.Error(); core.Contains(msg, sigReasonCorrupt) {
			reason = sigReasonCorrupt
		}
		emitSignatureRejected(in.ImageDigest, "", reason, verifyR)
		return core.Fail(core.E(sigVerifyOp,
			"upgrade.signature_invalid: "+reason, nil))
	}

	// Verified. Emit the success row; UpgradeWithConsent proceeds to
	// the side-effect docker pull.
	emitSignatureVerified(in.ImageDigest, "")
	return core.Ok(nil)
}

// emitSignatureVerified records the success row. Mantis #1622 / #1720
// dual-key shape — no error_code/error_scope on success rows.
func emitSignatureVerified(imageDigest, keyid string) {
	_ = audit.Default().Record(audit.Event{
		Event:   audit.EventOpencodeImageSignatureVerified,
		TS:      core.Now().UTC().Unix(),
		Scope:   "opencode",
		Outcome: audit.OutcomeOK,
		Meta: map[string]any{
			"image_digest": imageDigest,
			"keyid":        keyid,
		},
	})
}

// emitSignatureRejected records the failure row. Mantis #1622 +
// Mantis #1720 dual-key shape — error_code/error_scope sourced via
// audit.ErrorCode/ErrorScope so the bounded-keyspace promise holds.
// reason is one of the closed-set sigReason* literals declared at
// the top of this file.
func emitSignatureRejected(imageDigest, keyid, reason string, r core.Result) {
	_ = audit.Default().Record(audit.Event{
		Event:   audit.EventOpencodeImageSignatureRejected,
		TS:      core.Now().UTC().Unix(),
		Scope:   "opencode",
		Outcome: audit.OutcomeDenied,
		Meta: map[string]any{
			"image_digest":          imageDigest,
			"keyid":                 keyid,
			"reason":                reason,
			audit.MetaKeyErrorCode:  audit.ErrorCode(r),
			audit.MetaKeyErrorScope: audit.ErrorScope(r),
		},
	})
}
