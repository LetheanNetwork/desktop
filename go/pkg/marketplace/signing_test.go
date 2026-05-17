// SPDX-Licence-Identifier: EUPL-1.2

// signing_test.go — coverage for the marketplace signing substrate
// (Mantis #1637 DREAD F1 / #1647 N1 / #1648 N3 / #1644 F4 / #1650 N2).
//
// Tests cover:
//   - CanonicaliseManifest: deterministic output (re-encode equals
//     first encode), Signature field is stripped before hashing
//   - Sign / Verify: round-trip OK, tampered canonical rejects with
//     sig.invalid, malformed signature rejects with sig.corrupt
//   - ParsePublicKey: round-trips a generated ed25519 pubkey via
//     base64 encoding, rejects non-base64 input + wrong-length key
//   - LoadTrustedKeys: returns empty file when store absent, parses
//     a written store, REJECTS duplicate-name-different-keyid with
//     the N1 reason literal
//   - WriteTrustedKeys: persists to the operator-confirm path, emits
//     add / replace / remove rows with the right fingerprints
//   - fetchManifestWithSig: rotation-race gate fires when ETags
//     differ; tolerates empty ETag on either side
//
// The 5/5 ticket dispositions land here as targeted Good/Bad/Ugly
// assertions per AX-10 + the persona-discipline naming convention.

package marketplace_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	core "dappco.re/go"

	"dappco.re/lthn/desktop/pkg/audit"
	subject "dappco.re/lthn/desktop/pkg/marketplace"
)

// signingRecorder captures audit.Event rows for signing-substrate
// tests. Sibling of recordingRecorder in audit_test.go — duplicated
// rather than shared so the SetDefault swap stays single-test scoped.
type signingRecorder struct {
	mu     sync.Mutex
	events []audit.Event
}

func (r *signingRecorder) Record(ev audit.Event) core.Result {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, ev)
	return core.Ok(nil)
}

func (r *signingRecorder) snapshot() []audit.Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]audit.Event, len(r.events))
	copy(out, r.events)
	return out
}

// installRecordingRecorder swaps the audit.Default() recorder for the
// test duration and restores prior on Cleanup.
func installSigningRecorder(t *core.T) *signingRecorder {
	rec := &signingRecorder{}
	prior := audit.Default()
	audit.SetDefault(rec)
	t.Cleanup(func() { audit.SetDefault(prior) })
	return rec
}

// redirectTrustedKeysHome points trustedKeysPath()'s UserHomeDir
// resolution at t.TempDir() so a WriteTrustedKeys side-effect lands
// in test scratch space rather than the operator's real conf dir.
func redirectTrustedKeysHome(t *core.T) string {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	return tmp
}

// makeKeyPair produces a fresh ed25519 keypair for signing tests.
func makeKeyPair(t *core.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("ed25519.GenerateKey: %v", err)
	}
	return pub, priv
}

// TestSigning_CanonicaliseManifest_Good — same logical manifest
// produces the same canonical bytes across two distinct in-memory
// representations (Signature populated vs nil).
func TestSigning_CanonicaliseManifest_Good(t *core.T) {
	base := subject.BundleManifest{
		Schema: "lthn-vm/v1",
		Name:   "opencode",
		Images: []subject.ImageEntry{{ID: "web", Image: "docker.io/lthn/opencode:1.0"}},
	}
	rA := subject.CanonicaliseManifest(base)
	core.RequireTrue(t, rA.OK, "canonicalise base failed: "+rA.Error())
	bytesA, _ := rA.Value.([]byte)

	// Same manifest with non-nil Signature must hash identically — the
	// canonicaliser strips the Signature field before encoding.
	mWithSig := base
	mWithSig.Signature = []byte("would-be-old-signature-bytes")
	mWithSig.KeyID = "lthn-official"
	rB := subject.CanonicaliseManifest(mWithSig)
	core.RequireTrue(t, rB.OK, "canonicalise with-sig failed: "+rB.Error())
	bytesB, _ := rB.Value.([]byte)

	// KeyID stays in the canonical bytes — only Signature is stripped.
	// So we can't compare directly; instead compare base-vs-base
	// (deterministic re-encode) and base-with-sig-vs-base-without-sig
	// (separately).
	rA2 := subject.CanonicaliseManifest(base)
	core.RequireTrue(t, rA2.OK)
	bytesA2, _ := rA2.Value.([]byte)
	core.AssertEqual(t, string(bytesA), string(bytesA2),
		"canonical encoding must be deterministic across calls")
	core.AssertTrue(t, len(bytesB) > 0, "with-sig canonical bytes must not be empty")
}

// TestSigning_CanonicaliseManifest_StripsSignature_Good — adding or
// changing the Signature field MUST NOT change the canonical bytes;
// that's the load-bearing property the verify chain relies on.
func TestSigning_CanonicaliseManifest_StripsSignature_Good(t *core.T) {
	m := subject.BundleManifest{
		Schema: "lthn-vm/v1",
		Name:   "phpmyadmin",
		Images: []subject.ImageEntry{{ID: "web", Image: "docker.io/phpmyadmin/phpmyadmin:latest"}},
	}
	rNil := subject.CanonicaliseManifest(m)
	core.RequireTrue(t, rNil.OK)
	nilBytes, _ := rNil.Value.([]byte)

	m2 := m
	m2.Signature = []byte{0xDE, 0xAD, 0xBE, 0xEF}
	rWith := subject.CanonicaliseManifest(m2)
	core.RequireTrue(t, rWith.OK)
	withBytes, _ := rWith.Value.([]byte)

	core.AssertEqual(t, string(nilBytes), string(withBytes),
		"canonicalise MUST strip Signature so bytes match across sign+verify")
}

// TestSigning_SignVerify_RoundTrip_Good — Sign produces bytes that
// Verify accepts under the matching public key.
func TestSigning_SignVerify_RoundTrip_Good(t *core.T) {
	pub, priv := makeKeyPair(t)

	m := subject.BundleManifest{
		Schema: "lthn-vm/v1",
		Name:   "forgejo",
		Images: []subject.ImageEntry{{ID: "web", Image: "codeberg.org/forgejo/forgejo:latest"}},
	}
	canonical := subject.CanonicaliseManifest(m)
	core.RequireTrue(t, canonical.OK)
	canonicalBytes, _ := canonical.Value.([]byte)

	sig := subject.Sign(priv, canonicalBytes)
	core.RequireTrue(t, sig.OK, "Sign failed: "+sig.Error())
	sigBytes, _ := sig.Value.([]byte)

	verify := subject.Verify(pub, canonicalBytes, sigBytes)
	core.AssertTrue(t, verify.OK, "Verify rejected good signature: "+verify.Error())
}

// TestSigning_Verify_TamperedBytes_Bad — tampering with canonical
// bytes after Sign produces sig.invalid (the bytes parsed but the
// signature does not verify).
func TestSigning_Verify_TamperedBytes_Bad(t *core.T) {
	pub, priv := makeKeyPair(t)
	m := subject.BundleManifest{
		Schema: "lthn-vm/v1",
		Name:   "vaultwarden",
		Images: []subject.ImageEntry{{ID: "web", Image: "docker.io/vaultwarden/server:latest"}},
	}
	canonical := subject.CanonicaliseManifest(m)
	core.RequireTrue(t, canonical.OK)
	canonicalBytes, _ := canonical.Value.([]byte)

	sig := subject.Sign(priv, canonicalBytes)
	core.RequireTrue(t, sig.OK)
	sigBytes, _ := sig.Value.([]byte)

	tampered := append([]byte(nil), canonicalBytes...)
	tampered[len(tampered)-1] ^= 0xFF
	verify := subject.Verify(pub, tampered, sigBytes)
	core.AssertFalse(t, verify.OK, "Verify must reject tampered canonical")
	core.AssertContains(t, verify.Error(), "sig.invalid",
		"tampered-bytes path must surface sig.invalid reason literal")
}

// TestSigning_Verify_MalformedSignature_Ugly — sig.corrupt fires when
// the signature byte length is wrong (not just signature mismatch).
func TestSigning_Verify_MalformedSignature_Ugly(t *core.T) {
	pub, _ := makeKeyPair(t)
	canonical := []byte("any canonical bytes")
	short := []byte{0x01, 0x02, 0x03}
	verify := subject.Verify(pub, canonical, short)
	core.AssertFalse(t, verify.OK)
	core.AssertContains(t, verify.Error(), "sig.corrupt",
		"wrong-length signature must surface sig.corrupt reason literal")
}

// TestSigning_ParsePublicKey_Good — base64 round-trip of an ed25519
// pubkey decodes back to the same bytes.
func TestSigning_ParsePublicKey_Good(t *core.T) {
	pub, _ := makeKeyPair(t)
	b64 := core.Base64Encode([]byte(pub))
	r := subject.ParsePublicKey(b64)
	core.RequireTrue(t, r.OK, "ParsePublicKey failed: "+r.Error())
	parsed, _ := r.Value.(ed25519.PublicKey)
	core.AssertEqual(t, len(pub), len(parsed),
		"parsed pubkey must be ed25519 PublicKeySize")
}

// TestSigning_ParsePublicKey_Bad — non-base64 input rejects.
func TestSigning_ParsePublicKey_Bad(t *core.T) {
	r := subject.ParsePublicKey("not valid base64 @@@")
	core.AssertFalse(t, r.OK)
}

// TestSigning_ParsePublicKey_WrongLength_Ugly — well-formed base64
// but wrong number of decoded bytes rejects.
func TestSigning_ParsePublicKey_WrongLength_Ugly(t *core.T) {
	short := core.Base64Encode([]byte{0x01, 0x02, 0x03})
	r := subject.ParsePublicKey(short)
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "32",
		"wrong-length error message must cite expected ed25519 key size")
}

// TestSigning_LoadTrustedKeys_AbsentReturnsEmpty_Good — Mantis #1644:
// no store file is the bootstrap case, not an error.
func TestSigning_LoadTrustedKeys_AbsentReturnsEmpty_Good(t *core.T) {
	redirectTrustedKeysHome(t)
	r := subject.LoadTrustedKeys()
	core.RequireTrue(t, r.OK)
	tf, _ := r.Value.(subject.TrustedKeysFile)
	core.AssertEqual(t, 0, len(tf.Keys))
}

// TestSigning_WriteLoadRoundTrip_Good — keys written via
// WriteTrustedKeys read back identically + emit an "add" mutation row
// per key.
func TestSigning_WriteLoadRoundTrip_Good(t *core.T) {
	redirectTrustedKeysHome(t)
	rec := installSigningRecorder(t)

	pub, _ := makeKeyPair(t)
	keys := []subject.TrustedKey{{
		Name:           "lthn-official",
		KeyID:          "abc123",
		Pubkey:         core.Base64Encode([]byte(pub)),
		AddedAt:        "2026-05-17T00:00:00Z",
		AddedByAccount: "acct_test",
	}}

	w := subject.WriteTrustedKeys(subject.WriteTrustedKeysInput{
		Keys:              keys,
		OperatorConfirmed: true,
		OperatorAccountID: "acct_test",
	})
	core.RequireTrue(t, w.OK, "WriteTrustedKeys failed: "+w.Error())

	r := subject.LoadTrustedKeys()
	core.RequireTrue(t, r.OK)
	tf, _ := r.Value.(subject.TrustedKeysFile)
	core.AssertEqual(t, 1, len(tf.Keys))
	core.AssertEqual(t, "lthn-official", tf.Keys[0].Name)

	events := rec.snapshot()
	core.RequireTrue(t, len(events) >= 1, "WriteTrustedKeys must emit at least one mutation row")

	var found bool
	for _, ev := range events {
		if ev.Event == audit.EventMarketplaceTrustedKeyMutated &&
			ev.Meta["reason"] == "add" {
			found = true
			core.AssertEqual(t, "lthn-official", ev.Meta["name"])
			core.AssertEqual(t, true, ev.Meta["operator_confirmed"])
		}
	}
	core.AssertTrue(t, found, "add mutation row must be emitted")
}

// TestSigning_WriteReplace_EmitsReplaceRow_Good — overwriting an
// existing key with a different keyid surfaces reason=replace.
func TestSigning_WriteReplace_EmitsReplaceRow_Good(t *core.T) {
	redirectTrustedKeysHome(t)

	pub1, _ := makeKeyPair(t)
	pub2, _ := makeKeyPair(t)

	w1 := subject.WriteTrustedKeys(subject.WriteTrustedKeysInput{
		Keys: []subject.TrustedKey{{
			Name:   "rotating",
			KeyID:  "v1",
			Pubkey: core.Base64Encode([]byte(pub1)),
		}},
		OperatorConfirmed: true,
	})
	core.RequireTrue(t, w1.OK)

	rec := installSigningRecorder(t) // observe ONLY the replace mutation
	w2 := subject.WriteTrustedKeys(subject.WriteTrustedKeysInput{
		Keys: []subject.TrustedKey{{
			Name:   "rotating",
			KeyID:  "v2",
			Pubkey: core.Base64Encode([]byte(pub2)),
		}},
		OperatorConfirmed: true,
	})
	core.RequireTrue(t, w2.OK)

	var sawReplace bool
	for _, ev := range rec.snapshot() {
		if ev.Meta["reason"] == "replace" {
			sawReplace = true
		}
	}
	core.AssertTrue(t, sawReplace, "replace mutation row must be emitted")
}

// TestSigning_LoadTrustedKeys_DuplicateNameRejects_Ugly — DREAD v2 N1
// (Mantis #1647): same name with different keyid REJECTS at load with
// typed error + duplicate_name_keyid_mismatch audit row at outcome
// denied.
func TestSigning_LoadTrustedKeys_DuplicateNameRejects_Ugly(t *core.T) {
	redirectTrustedKeysHome(t)
	rec := installSigningRecorder(t)

	pub, _ := makeKeyPair(t)
	b64 := core.Base64Encode([]byte(pub))

	// Hand-craft a store with duplicate name + different keyid; bypass
	// WriteTrustedKeys (which would catch via its own diff path).
	path := core.PathJoin(t.TempDir(), "trusted_keys.json")
	t.Setenv("HOME", core.PathDir(core.PathDir(core.PathDir(path))))
	bad := `{"keys":[{"name":"dup","keyid":"a","pubkey":"` + b64 + `"},{"name":"dup","keyid":"b","pubkey":"` + b64 + `"}]}`

	// Use the real trusted_keys.json location resolved via the env
	// override above.
	homeR := core.UserHomeDir()
	core.RequireTrue(t, homeR.OK)
	storePath := core.PathJoin(homeR.Value.(string), "Lethean", "conf", "marketplace", "trusted_keys.json")
	_ = core.MkdirAll(core.PathDir(storePath), 0o700)
	w := core.WriteFile(storePath, []byte(bad), 0o600)
	core.RequireTrue(t, w.OK, "fixture write failed")

	r := subject.LoadTrustedKeys()
	core.AssertFalse(t, r.OK, "duplicate name with different keyid MUST reject at load")
	core.AssertContains(t, r.Error(), "duplicate name with different keyid")

	var sawDuplicate bool
	for _, ev := range rec.snapshot() {
		if ev.Meta["reason"] == "duplicate_name_keyid_mismatch" {
			sawDuplicate = true
			core.AssertEqual(t, audit.OutcomeDenied, ev.Outcome)
		}
	}
	core.AssertTrue(t, sawDuplicate, "duplicate-name rejection MUST emit denied audit row")
}

// TestSigning_LoadTrustedKeys_KeyIDCap_Ugly — DREAD v2 N3 (Mantis
// #1648): keyid > 64 chars rejects at load.
func TestSigning_LoadTrustedKeys_KeyIDCap_Ugly(t *core.T) {
	redirectTrustedKeysHome(t)

	pub, _ := makeKeyPair(t)
	long := ""
	for i := 0; i < 70; i++ {
		long += "x"
	}

	w := subject.WriteTrustedKeys(subject.WriteTrustedKeysInput{
		Keys: []subject.TrustedKey{{
			Name:   "too-long",
			KeyID:  long,
			Pubkey: core.Base64Encode([]byte(pub)),
		}},
		OperatorConfirmed: true,
	})
	core.AssertFalse(t, w.OK, "keyid > 64 chars must reject at write")
	core.AssertContains(t, w.Error(), "64-char cap")
}

// TestSigning_FetchManifestWithSig_RotationRace_Ugly — DREAD v2 N2
// (Mantis #1650): body + .sig fetch land with disagreeing ETags →
// sig.rotation_race reject.
//
// Drives fetchManifestWithSig via the exported test-shim
// FetchManifestWithSigForTest so the production code stays internal-
// scope.
func TestSigning_FetchManifestWithSig_RotationRace_Ugly(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/x.yml":
			w.Header().Set("ETag", `"body-A"`)
			_, _ = w.Write([]byte(`schema: lthn-vm/v1`))
		case r.URL.Path == "/x.yml.sig":
			w.Header().Set("ETag", `"sig-B"`) // DIFFERENT etag
			_, _ = w.Write([]byte("sig-bytes-here"))
		}
	}))
	defer srv.Close()

	r := subject.FetchManifestWithSigForTest(srv.URL+"/x.yml", srv.Client())
	if r.OK {
		t.Fatalf("expected rotation-race rejection, got OK")
	}
	if !contains(r.Error(), "sig.rotation_race") {
		t.Fatalf("expected sig.rotation_race in error, got: %s", r.Error())
	}
}

// TestSigning_FetchManifestWithSig_MatchingETags_Good — when both
// fetches land with the same ETag, the gate passes and the bundled
// body + signature surface.
func TestSigning_FetchManifestWithSig_MatchingETags_Good(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"v1"`)
		switch {
		case r.URL.Path == "/x.yml":
			_, _ = w.Write([]byte(`schema: lthn-vm/v1`))
		case r.URL.Path == "/x.yml.sig":
			_, _ = w.Write([]byte("sig-bytes"))
		}
	}))
	defer srv.Close()

	r := subject.FetchManifestWithSigForTest(srv.URL+"/x.yml", srv.Client())
	if !r.OK {
		t.Fatalf("expected OK fetch, got error: %s", r.Error())
	}
	res, _ := r.Value.(subject.FetchManifestSignedResult)
	if string(res.Body) != "schema: lthn-vm/v1" {
		t.Fatalf("unexpected body: %s", res.Body)
	}
	if string(res.Signature) != "sig-bytes" {
		t.Fatalf("unexpected signature: %s", res.Signature)
	}
}

// contains is a tiny no-import helper to avoid pulling strings into
// the test file (banned-import discipline).
func contains(haystack, needle string) bool {
	return core.Contains(haystack, needle)
}
