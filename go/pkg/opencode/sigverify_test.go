// SPDX-Licence-Identifier: EUPL-1.2

// sigverify_test.go — Cerberus #22 MED-2 / Mantis #1622 supply-chain
// signature-verification gate tests for UpgradeWithConsent. The
// happy-path test PROVES the gate is wired (a real ed25519 signature
// against a trusted_publishers.json pin reaches the substrate, which
// then trips on the zero-Service "process service unavailable"
// surface — the same proof-of-wiring shape TestUpgrade_DigestPinned_
// PassesGate_Good uses for the digest gate). The Bad tests pin each
// of the four rejection facets (signature_missing / key_not_found /
// sig.invalid / release_id.newline_forbidden). The Ugly test pins
// the require_signature=false bootstrap path so a deployment without
// signing wired yet doesn't silently break.

package opencode

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	core "dappco.re/go"
)

// withTempTrustedPublishers writes a trusted_publishers.json file
// under a temp HOME for the duration of the test and restores the
// original HOME on cleanup. Returns the publishers' base64 pubkey
// bytes for the signing helper.
func withTempTrustedPublishers(t *testing.T, name string, pub ed25519.PublicKey) string {
	t.Helper()
	tmp := t.TempDir()
	origHome, hadHome := os.LookupEnv("HOME")
	t.Setenv("HOME", tmp)
	t.Cleanup(func() {
		if hadHome {
			_ = os.Setenv("HOME", origHome)
		} else {
			_ = os.Unsetenv("HOME")
		}
	})

	dir := filepath.Join(tmp, "Lethean", "conf", "opencode")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir trusted_publishers dir: %v", err)
	}
	pubB64 := base64.StdEncoding.EncodeToString(pub)
	// Hand-write JSON to avoid coupling the test to any internal
	// marketshape change.
	body := `{"keys":[{"name":"` + name + `","keyid":"test-keyid","pubkey":"` + pubB64 + `","added_at":"2026-05-18T00:00:00Z","added_by_account":"test"}]}`
	if err := os.WriteFile(filepath.Join(dir, "trusted_publishers.json"), []byte(body), 0o600); err != nil {
		t.Fatalf("write trusted_publishers.json: %v", err)
	}
	return pubB64
}

const sigTestDigest = "sha256:ca59eb28d5ea6a1f50c45a1f1df5c1a9286343e41b389fe89fb4ffac96dbeb84"

// TestOpencode_Upgrade_SignatureVerified_Good — UpgradeWithConsent
// with a valid ed25519 signature whose pubkey is pinned in
// trusted_publishers.json MUST pass the signature gate and proceed
// to the substrate. Proof-of-wiring against a zero Service{}: the
// failure surface MUST be "process service unavailable" (every gate
// passed) rather than any "upgrade.signature_*" rejection.
func TestOpencode_Upgrade_SignatureVerified_Good(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("ed25519.GenerateKey: %v", err)
	}
	pubB64 := withTempTrustedPublishers(t, "test-publisher", pub)

	// Sign the canonical bytes (digest + "\n" + tag + "\n" + release_id).
	// Construct service via NewService so image() / requireSignature()
	// don't nil-deref on the embedded ServiceRuntime[Options].
	svc := newServiceWithPolicy(t, true)
	canon, ok := canonicalSigningBytes(sigTestDigest, imageTag(svc.image()), "v1.2.3")
	if !ok {
		t.Fatalf("canonicalSigningBytes returned !ok for valid release_id")
	}
	sig := ed25519.Sign(priv, canon)

	r := svc.UpgradeWithConsent(UpgradeInput{
		ConfirmedByUser: true,
		ImageDigest:     sigTestDigest,
		SignatureBytes:  sig,
		PublicKeyBase64: []byte(pubB64),
		ReleaseID:       "v1.2.3",
	})
	if r.OK {
		t.Fatalf("UpgradeWithConsent against zero Service{} returned OK; want substrate Fail")
	}
	got := r.Error()
	for _, gateString := range []string{
		"upgrade.requires_confirmation",
		"upgrade.digest_required",
		"upgrade.digest_invalid",
		"upgrade.signature_invalid",
	} {
		if strings.Contains(got, gateString) {
			t.Fatalf("error = %q contains gate-refusal %q; want every gate passed", got, gateString)
		}
	}
	if !strings.Contains(got, "process service unavailable") {
		t.Errorf("error = %q; want 'process service unavailable' (the only path past every gate on a zero Service{})", got)
	}
}

// TestOpencode_Upgrade_SignatureRejected_Bad — every rejection facet
// MUST be reachable and produce the typed "upgrade.signature_invalid"
// + the closed-set reason literal in r.Error(). Covers the four
// facets:
//
//   - signature_missing — policy on, no sig supplied
//   - key_not_found     — sig + pubkey supplied, pubkey not in trust store
//   - sig.invalid       — sig + pubkey supplied, pubkey IS trusted, but
//                          signature bytes don't verify under canon
//   - release_id.newline_forbidden — caller put "\n" in release_id
func TestOpencode_Upgrade_SignatureRejected_Bad(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("ed25519.GenerateKey: %v", err)
	}

	t.Run("signature_missing", func(t *testing.T) {
		// Set up trusted_publishers (otherwise loadTrustedPublishers
		// returns empty fine, but the policy-on-no-sig case rejects
		// BEFORE the trust-store load anyway).
		_ = withTempTrustedPublishers(t, "test-publisher", pub)
		// Need require_signature=true. Construct a service with that
		// policy via NewService.
		svc := newServiceWithPolicy(t, true)
		r := svc.UpgradeWithConsent(UpgradeInput{
			ConfirmedByUser: true,
			ImageDigest:     sigTestDigest,
			// no SignatureBytes / PublicKeyBase64
		})
		if r.OK {
			t.Fatalf("UpgradeWithConsent succeeded without signature when require_signature=true; want Fail")
		}
		got := r.Error()
		if !strings.Contains(got, "upgrade.signature_invalid") || !strings.Contains(got, "signature_missing") {
			t.Errorf("error = %q; want substring 'upgrade.signature_invalid' + 'signature_missing'", got)
		}
	})

	t.Run("key_not_found", func(t *testing.T) {
		// trusted_publishers.json with publisher A; signature with
		// freshly-generated key B → key_not_found.
		_ = withTempTrustedPublishers(t, "publisher-A", pub) // A pinned
		otherPub, otherPriv, _ := ed25519.GenerateKey(rand.Reader)
		otherPubB64 := base64.StdEncoding.EncodeToString(otherPub)

		svc := newServiceWithPolicy(t, false)
		canon, _ := canonicalSigningBytes(sigTestDigest, imageTag(svc.image()), "v1.0")
		sig := ed25519.Sign(otherPriv, canon)

		r := svc.UpgradeWithConsent(UpgradeInput{
			ConfirmedByUser: true,
			ImageDigest:     sigTestDigest,
			SignatureBytes:  sig,
			PublicKeyBase64: []byte(otherPubB64), // B, NOT in trust store
			ReleaseID:       "v1.0",
		})
		if r.OK {
			t.Fatalf("UpgradeWithConsent succeeded with untrusted pubkey; want Fail")
		}
		got := r.Error()
		if !strings.Contains(got, "upgrade.signature_invalid") || !strings.Contains(got, "key_not_found") {
			t.Errorf("error = %q; want substring 'upgrade.signature_invalid' + 'key_not_found'", got)
		}
	})

	t.Run("sig.invalid", func(t *testing.T) {
		// Pubkey IS trusted, but the signature bytes are random garbage
		// of the right length (valid ed25519 signature shape, but
		// don't actually verify under the canonical bytes).
		pubB64 := withTempTrustedPublishers(t, "publisher-A", pub)
		// Sign WRONG bytes — sig will be ed25519-shape-valid but won't
		// verify under canonical(digest, tag, release_id).
		wrongSig := ed25519.Sign(priv, []byte("WRONG-CANONICAL-BYTES"))

		svc := newServiceWithPolicy(t, false)
		r := svc.UpgradeWithConsent(UpgradeInput{
			ConfirmedByUser: true,
			ImageDigest:     sigTestDigest,
			SignatureBytes:  wrongSig,
			PublicKeyBase64: []byte(pubB64),
			ReleaseID:       "v1.0",
		})
		if r.OK {
			t.Fatalf("UpgradeWithConsent succeeded with wrong-canonical signature; want Fail")
		}
		got := r.Error()
		if !strings.Contains(got, "upgrade.signature_invalid") || !strings.Contains(got, "sig.invalid") {
			t.Errorf("error = %q; want substring 'upgrade.signature_invalid' + 'sig.invalid'", got)
		}
	})

	t.Run("release_id_newline_forbidden", func(t *testing.T) {
		pubB64 := withTempTrustedPublishers(t, "publisher-A", pub)
		svc := newServiceWithPolicy(t, false)
		// release_id with embedded newline → fails at canonical gate.
		sig := ed25519.Sign(priv, []byte("anything"))
		r := svc.UpgradeWithConsent(UpgradeInput{
			ConfirmedByUser: true,
			ImageDigest:     sigTestDigest,
			SignatureBytes:  sig,
			PublicKeyBase64: []byte(pubB64),
			ReleaseID:       "v1.0\nsmuggle",
		})
		if r.OK {
			t.Fatalf("UpgradeWithConsent succeeded with newline release_id; want Fail")
		}
		got := r.Error()
		if !strings.Contains(got, "upgrade.signature_invalid") || !strings.Contains(got, "release_id.newline_forbidden") {
			t.Errorf("error = %q; want substring 'upgrade.signature_invalid' + 'release_id.newline_forbidden'", got)
		}
	})
}

// TestOpencode_Upgrade_NoSignatureRequiredOff_Ugly — the bootstrap
// path: require_signature=false AND no signature supplied MUST pass
// the signature gate entirely (no rejection, no audit row). Proves
// the legacy / first-deploy case stays unblocked.
//
// Proof-of-wiring: failure surface is "process service unavailable"
// (every gate passed), NOT any signature-related error.
func TestOpencode_Upgrade_NoSignatureRequiredOff_Ugly(t *testing.T) {
	svc := newServiceWithPolicy(t, false)
	r := svc.UpgradeWithConsent(UpgradeInput{
		ConfirmedByUser: true,
		ImageDigest:     sigTestDigest,
		// no signature, no pubkey
	})
	if r.OK {
		t.Fatalf("UpgradeWithConsent against zero Service{} returned OK; want substrate Fail")
	}
	got := r.Error()
	for _, gateString := range []string{
		"upgrade.signature_invalid",
		"signature_missing",
		"key_not_found",
	} {
		if strings.Contains(got, gateString) {
			t.Fatalf("error = %q contains signature-gate refusal %q; want gate bypassed when require_signature=false", got, gateString)
		}
	}
	if !strings.Contains(got, "process service unavailable") {
		t.Errorf("error = %q; want 'process service unavailable' (signature gate bypassed when off)", got)
	}
}

// newServiceWithPolicy constructs a *Service whose Options carry the
// requested UpgradeRequireSignature policy. The Core runtime is
// stubbed via core.New so the proc() lookup still returns nil (no
// process service registered) — the test relies on
// "process service unavailable" as the proof-of-wiring tail just like
// the existing digest-gate tests.
func newServiceWithPolicy(t *testing.T, requireSig bool) *Service {
	t.Helper()
	c := core.New()
	r := NewService(Options{UpgradeRequireSignature: requireSig})(c)
	if !r.OK {
		t.Fatalf("NewService failed: %s", r.Error())
	}
	svc, _ := r.Value.(*Service)
	if svc == nil {
		t.Fatalf("NewService did not return *Service")
	}
	return svc
}
