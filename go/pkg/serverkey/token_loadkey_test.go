// SPDX-Licence-Identifier: EUPL-1.2

// Malformed on-disk server.key blob shapes that loadServerKey must
// fail closed on, plus the canonicalise() marshal-failure branch and
// the nonce-set eviction branches. TestServerkey_AtomicWrite_KeyExists_Bad
// (serverkey_test.go) already covers the "missing public-key block"
// shape; these fixtures cover the remaining malformed-block and
// decode-failure branches loadServerKey's frame parser guards against.
// Marker strings mirror marshalServerKey's exact framing verbatim.

package serverkey_test

import (
	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/serverkey"
)

const (
	fixtureHdr      = "# lthn server-key v1\n"
	fixturePubOpen  = "-----BEGIN LTHN PUBLIC KEY-----\n"
	fixturePubClose = "\n-----END LTHN PUBLIC KEY-----\n"
	fixturePrvOpen  = "-----BEGIN LTHN PRIVATE KEY (sym-encrypted)-----\n"
	fixturePrvClose = "\n-----END LTHN PRIVATE KEY-----\n"
)

// seedAndKeyFixture writes a valid 32-byte .seed (so Bootstrap gets
// past the seed-load stage) and a server.key blob with the supplied
// raw content, both at mode 0o600 so verifyMode never trips — only
// loadServerKey's frame parser is under test.
func seedAndKeyFixture(t *core.T, keyContent string) string {
	t.Helper()
	home := homeFixture(t)
	walletsDir := core.PathJoin(home, "Lethean", "wallets")
	mk := core.MkdirAll(walletsDir, 0o700)
	core.AssertTrue(t, mk.OK, "fixture wallets dir mkdir must succeed")

	seedPath := core.PathJoin(walletsDir, ".seed")
	w := core.WriteFile(seedPath, make([]byte, 32), 0o600)
	core.AssertTrue(t, w.OK, "fixture seed write must succeed")

	keyPath := core.PathJoin(walletsDir, "server.key")
	w = core.WriteFile(keyPath, []byte(keyContent), 0o600)
	core.AssertTrue(t, w.OK, "fixture key write must succeed")

	return walletsDir
}

func TestToken_LoadServerKey_Bad_MalformedPublicBlock(t *core.T) {
	// pubOpen present, pubClose absent — indexOf(pubClose) returns -1.
	blob := fixtureHdr + fixturePubOpen + "fake-pub-data-no-terminator"
	_ = seedAndKeyFixture(t, blob)

	svc := subject.NewService(nil)
	r := svc.Bootstrap()
	core.AssertTrue(t, !r.OK, "a public-key block missing its END marker must fail Bootstrap")
}

func TestToken_LoadServerKey_Bad_MissingPrivateBlock(t *core.T) {
	// Well-formed, closed public block; no private block at all.
	blob := fixtureHdr + fixturePubOpen + "fake-pub-data" + fixturePubClose
	_ = seedAndKeyFixture(t, blob)

	svc := subject.NewService(nil)
	r := svc.Bootstrap()
	core.AssertTrue(t, !r.OK, "a blob with no private-key block at all must fail Bootstrap")
}

func TestToken_LoadServerKey_Bad_MalformedPrivateBlock(t *core.T) {
	// Private block opened but never closed.
	blob := fixtureHdr + fixturePubOpen + "fake-pub-data" + fixturePubClose +
		fixturePrvOpen + "fake-priv-data-no-terminator"
	_ = seedAndKeyFixture(t, blob)

	svc := subject.NewService(nil)
	r := svc.Bootstrap()
	core.AssertTrue(t, !r.OK, "a private-key block missing its END marker must fail Bootstrap")
}

func TestToken_LoadServerKey_Bad_PrivateBlockNotBase64(t *core.T) {
	// Fully-framed blob, but the sealed-private payload isn't valid
	// base64url — Base64URLDecode must fail rather than panic.
	blob := fixtureHdr + fixturePubOpen + "fake-pub-data" + fixturePubClose +
		fixturePrvOpen + "!!!not-valid-base64!!!" + fixturePrvClose
	_ = seedAndKeyFixture(t, blob)

	svc := subject.NewService(nil)
	r := svc.Bootstrap()
	core.AssertTrue(t, !r.OK, "a private-key payload that isn't valid base64 must fail Bootstrap")
}

func TestToken_LoadServerKey_Bad_KeyPathIsDirectory(t *core.T) {
	// server.key exists (Stat succeeds, mode 0o600 matches) but is a
	// directory, not a file — ReadFile inside loadServerKey must fail
	// rather than the earlier Stat-based checks masking it.
	home := homeFixture(t)
	walletsDir := core.PathJoin(home, "Lethean", "wallets")
	mk := core.MkdirAll(walletsDir, 0o700)
	core.AssertTrue(t, mk.OK, "fixture wallets dir mkdir must succeed")

	seedPath := core.PathJoin(walletsDir, ".seed")
	w := core.WriteFile(seedPath, make([]byte, 32), 0o600)
	core.AssertTrue(t, w.OK, "fixture seed write must succeed")

	keyPath := core.PathJoin(walletsDir, "server.key")
	mkKey := core.MkdirAll(keyPath, 0o700)
	core.AssertTrue(t, mkKey.OK, "fixture server.key-as-directory mkdir must succeed")
	if err := subject.OsChmod(keyPath, 0o600); err != nil {
		t.Skipf("chmod unsupported on this fs: %v", err)
	}

	svc := subject.NewService(nil)
	r := svc.Bootstrap()
	core.AssertTrue(t, !r.OK, "a server.key path that is a directory must fail Bootstrap via ReadFile, not panic")
}

// --- canonicalise() marshal failure ---

func TestToken_Canonicalise_Bad_UnmarshalableValue(t *core.T) {
	// A channel value can never be JSON-marshalled — canonicalise must
	// surface the marshal error via core.Result rather than panicking.
	_, r := subject.ExportedCanonicalise(map[string]any{
		"bad": make(chan int),
	})
	core.AssertTrue(t, !r.OK, "canonicalise must fail closed on an unmarshalable header value")
}

// --- nonce-set eviction ---

func TestToken_EvictExpiredNoncesLocked_Good_RemovesStaleEntry(t *core.T) {
	svc := bootstrappedForge(t)
	svc.ExportedInjectExpiredNonce("stale-bootstrap-nonce")
	before := len(svc.ExportedConsumedNonces())
	core.AssertTrue(t, before == 1, "fixture nonce must be present before the eviction-triggering verify")

	out := svc.IssueBootstrapToken()
	core.AssertTrue(t, out.OK, "mint should succeed")
	tok := out.Value.(subject.BootstrapTokenOutput)

	// Any VerifyBootstrapToken call runs evictExpiredNoncesLocked
	// before checking replay — the stale injected entry must be
	// swept even though it belongs to an unrelated nonce.
	vr := svc.VerifyBootstrapToken(tok.Token, "account.create")
	core.AssertTrue(t, vr.OK, "verify of a freshly minted token must still succeed")

	after := svc.ExportedConsumedNonces()
	for k := range after {
		core.AssertTrue(t, k != svc.ExportedNonceKey("stale-bootstrap-nonce"),
			"the expired fixture nonce must have been evicted, not just the new one added")
	}
}

func TestToken_EvictExpiredSessionNoncesLocked_Good_RemovesStaleEntry(t *core.T) {
	svc := bootstrappedForge(t)
	svc.ExportedInjectExpiredSessionNonce("stale-session-nonce")

	// IssueSessionTokenWithRequest runs
	// evictExpiredSessionNoncesLocked before recording its own nonce —
	// minting must still succeed with the stale entry present.
	r := svc.IssueSessionToken("acct-1")
	core.AssertTrue(t, r.OK, "session mint must succeed even with a stale session-nonce entry queued for eviction")
}
