// SPDX-Licence-Identifier: EUPL-1.2

// Tests for the serverkey package. Triadic Good / Bad / Ugly coverage
// per AX-9 + the RFC §10 Stage B requirement. Test isolation via
// homeFixture rebinding $HOME so the real ~/Lethean/ tree is never
// touched.

package serverkey_test

import (
	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/serverkey"
)

// homeFixture rebinds $HOME to a t-scoped temp dir for the test, then
// returns the temp root. Every paths.* call resolves underneath it.
func homeFixture(t *core.T) string {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	return tmp
}

// --- Bootstrap ---

func TestServerkey_Bootstrap_Good(t *core.T) {
	_ = homeFixture(t)
	svc := subject.NewService(nil)
	r := svc.Bootstrap()
	core.AssertTrue(t, r.OK, "Bootstrap should succeed on fresh install")

	// Idempotent — second call short-circuits.
	r2 := svc.Bootstrap()
	core.AssertTrue(t, r2.OK, "Bootstrap should be idempotent across in-process calls")
}

func TestServerkey_Bootstrap_Bad(t *core.T) {
	// Point $HOME at a path that exists but isn't writable. The
	// MkdirAll for ~/Lethean/wallets/ should fail when we can't write
	// inside it.
	tmp := t.TempDir()
	// Make $HOME a regular file rather than a directory — MkdirAll
	// will fail trying to create the wallets subdir.
	tmpFile := core.PathJoin(tmp, "home-as-file")
	w := core.WriteFile(tmpFile, []byte{}, 0o600)
	core.AssertTrue(t, w.OK, "fixture file write should succeed")
	t.Setenv("HOME", tmpFile)

	svc := subject.NewService(nil)
	r := svc.Bootstrap()
	core.AssertTrue(t, !r.OK, "Bootstrap should fail when HOME is unwritable")
}

func TestServerkey_Bootstrap_Ugly_Reload(t *core.T) {
	// Cerberus #1470 — the same .seed on disk must reload the same
	// passphrase, decrypting the persisted server.key. We construct
	// twice with the same $HOME — second NewService must Bootstrap
	// via the load path (key exists) rather than re-generate.
	_ = homeFixture(t)

	svc1 := subject.NewService(nil)
	r1 := svc1.Bootstrap()
	core.AssertTrue(t, r1.OK, "first Bootstrap should succeed")

	// Mint a token on svc1 so we can later verify svc2 holds the
	// same key — a re-generated key wouldn't satisfy the verify.
	tokR := svc1.IssueBootstrapToken()
	core.AssertTrue(t, tokR.OK, "IssueBootstrapToken after Bootstrap should succeed")
	out := tokR.Value.(subject.BootstrapTokenOutput)

	svc2 := subject.NewService(nil)
	r2 := svc2.Bootstrap()
	core.AssertTrue(t, r2.OK, "second Bootstrap should succeed via load path")

	// svc2 should accept svc1's freshly-minted token (same key on disk).
	verR := svc2.VerifyBootstrapToken(out.Token, "account.create")
	core.AssertTrue(t, verR.OK, "second instance must verify first instance's token (same on-disk key)")
}

// --- AccountStatus (Cerberus #1471) ---

func TestServerkey_AccountStatus_Good_FreshInstall(t *core.T) {
	_ = homeFixture(t)
	svc := subject.NewService(nil)
	r := svc.AccountStatus()
	core.AssertTrue(t, r.OK)
	out := r.Value.(subject.AccountStatusOutput)
	core.AssertTrue(t, !out.HasUserAccount, "fresh install — no account expected")
}

func TestServerkey_AccountStatus_Bad_EmptyAccountDir(t *core.T) {
	// Cerberus #1471 — directory presence ALONE must NOT signal
	// has_user_account=true. A partial/abandoned account dir without
	// private.key returns false so the gate falls back to `setup`.
	home := homeFixture(t)
	accountRoot := core.PathJoin(home, "Lethean", "account", "abc123")
	mk := core.MkdirAll(accountRoot, 0o755)
	core.AssertTrue(t, mk.OK)

	svc := subject.NewService(nil)
	r := svc.AccountStatus()
	core.AssertTrue(t, r.OK)
	out := r.Value.(subject.AccountStatusOutput)
	core.AssertTrue(t, !out.HasUserAccount, "empty account dir must NOT signal has_user_account=true (Cerberus #1471)")
}

func TestServerkey_AccountStatus_Ugly_PartialAccount(t *core.T) {
	// Account dir exists with a NON-private.key file — still false.
	home := homeFixture(t)
	accountRoot := core.PathJoin(home, "Lethean", "account", "abc123")
	mk := core.MkdirAll(accountRoot, 0o755)
	core.AssertTrue(t, mk.OK)
	w := core.WriteFile(core.PathJoin(accountRoot, "manifest.json"), []byte("{}"), 0o600)
	core.AssertTrue(t, w.OK)

	svc := subject.NewService(nil)
	r := svc.AccountStatus()
	core.AssertTrue(t, r.OK)
	out := r.Value.(subject.AccountStatusOutput)
	core.AssertTrue(t, !out.HasUserAccount, "partial account dir without private.key must NOT signal has_user_account=true")
}

func TestServerkey_AccountStatus_Good_AccountPresent(t *core.T) {
	home := homeFixture(t)
	accountRoot := core.PathJoin(home, "Lethean", "account", "abc123")
	mk := core.MkdirAll(accountRoot, 0o700)
	core.AssertTrue(t, mk.OK)
	w := core.WriteFile(core.PathJoin(accountRoot, "private.key"), []byte("armoured-key"), 0o600)
	core.AssertTrue(t, w.OK)

	svc := subject.NewService(nil)
	r := svc.AccountStatus()
	core.AssertTrue(t, r.OK)
	out := r.Value.(subject.AccountStatusOutput)
	core.AssertTrue(t, out.HasUserAccount, "private.key presence must signal has_user_account=true")
}

// --- IssueBootstrapToken / VerifyBootstrapToken (round-trip + replay) ---

func TestServerkey_TokenRoundTrip_Good(t *core.T) {
	_ = homeFixture(t)
	svc := subject.NewService(nil)
	core.AssertTrue(t, svc.Bootstrap().OK)

	r := svc.IssueBootstrapToken()
	core.AssertTrue(t, r.OK)
	out := r.Value.(subject.BootstrapTokenOutput)

	// Token format — LTHN-BOOT-1.<header>.<sig>
	core.AssertTrue(t, core.HasPrefix(out.Token, "LTHN-BOOT-1."), "token must carry version prefix")
	parts := core.Split(out.Token[len("LTHN-BOOT-1."):], ".")
	core.AssertEqual(t, 2, len(parts))

	// Round-trip: same service verifies its own token.
	vr := svc.VerifyBootstrapToken(out.Token, "account.create")
	core.AssertTrue(t, vr.OK, "fresh token must verify against issuing service")
}

func TestServerkey_TokenReplay_Bad(t *core.T) {
	// Cerberus #1463 — nonce single-use. Second verify on the same
	// token must fail with auth.bootstrap.replay.
	_ = homeFixture(t)
	svc := subject.NewService(nil)
	core.AssertTrue(t, svc.Bootstrap().OK)

	out := svc.IssueBootstrapToken().Value.(subject.BootstrapTokenOutput)
	first := svc.VerifyBootstrapToken(out.Token, "account.create")
	core.AssertTrue(t, first.OK, "first verify should succeed")

	second := svc.VerifyBootstrapToken(out.Token, "account.create")
	core.AssertTrue(t, !second.OK, "replay of same token must fail")
}

func TestServerkey_TokenScopeMismatch_Bad(t *core.T) {
	// Cerberus #1467 — token scope MUST match the requested scope.
	// A token minted with scope=account.create cannot satisfy a
	// different scope, even if the signature is valid.
	_ = homeFixture(t)
	svc := subject.NewService(nil)
	core.AssertTrue(t, svc.Bootstrap().OK)

	out := svc.IssueBootstrapToken().Value.(subject.BootstrapTokenOutput)
	r := svc.VerifyBootstrapToken(out.Token, "some.other.scope")
	core.AssertTrue(t, !r.OK, "scope mismatch must reject token")
}

func TestServerkey_TokenTampered_Ugly(t *core.T) {
	// Cerberus #1469 — flipping any byte in the header part must
	// fail verify (re-canonicalised bytes won't match the signed
	// originals).
	_ = homeFixture(t)
	svc := subject.NewService(nil)
	core.AssertTrue(t, svc.Bootstrap().OK)

	out := svc.IssueBootstrapToken().Value.(subject.BootstrapTokenOutput)
	// Mangle one character in the middle of the header segment.
	rest := out.Token[len("LTHN-BOOT-1."):]
	parts := core.Split(rest, ".")
	core.AssertEqual(t, 2, len(parts))
	header := parts[0]
	mid := len(header) / 2
	swapped := byte('A')
	if header[mid] == 'A' {
		swapped = 'B'
	}
	mangled := header[:mid] + string([]byte{swapped}) + header[mid+1:]
	tampered := "LTHN-BOOT-1." + mangled + "." + parts[1]

	r := svc.VerifyBootstrapToken(tampered, "account.create")
	core.AssertTrue(t, !r.OK, "tampered header must reject token")
}

func TestServerkey_TokenWithoutBootstrap_Bad(t *core.T) {
	// Calling IssueBootstrapToken before Bootstrap must fail —
	// there's no private key in memory yet.
	_ = homeFixture(t)
	svc := subject.NewService(nil)
	r := svc.IssueBootstrapToken()
	core.AssertTrue(t, !r.OK, "IssueBootstrapToken without Bootstrap must fail")
}

func TestServerkey_TokenVerifyWithoutBootstrap_Bad(t *core.T) {
	_ = homeFixture(t)
	svc := subject.NewService(nil)
	r := svc.VerifyBootstrapToken("LTHN-BOOT-1.x.y", "account.create")
	core.AssertTrue(t, !r.OK, "VerifyBootstrapToken without Bootstrap must fail")
}

func TestServerkey_TokenFormat_Bad(t *core.T) {
	_ = homeFixture(t)
	svc := subject.NewService(nil)
	core.AssertTrue(t, svc.Bootstrap().OK)

	// Wrong prefix.
	r1 := svc.VerifyBootstrapToken("WRONG-PREFIX.x.y", "account.create")
	core.AssertTrue(t, !r1.OK, "non-LTHN-BOOT prefix must reject")

	// Wrong segment count.
	r2 := svc.VerifyBootstrapToken("LTHN-BOOT-1.onlyonesegment", "account.create")
	core.AssertTrue(t, !r2.OK, "single-segment token must reject")
}

// --- Bootstrap idempotency (Cerberus #1466 race-resistance) ---

func TestServerkey_BootstrapIdempotency_Good(t *core.T) {
	// Repeated Bootstrap calls from the same Service must all
	// succeed without re-generating the on-disk key.
	_ = homeFixture(t)
	svc := subject.NewService(nil)
	for i := 0; i < 5; i++ {
		r := svc.Bootstrap()
		core.AssertTrue(t, r.OK, "Bootstrap iteration must succeed")
	}
	// A token minted after the loop must still verify.
	out := svc.IssueBootstrapToken().Value.(subject.BootstrapTokenOutput)
	core.AssertTrue(t, svc.VerifyBootstrapToken(out.Token, "account.create").OK)
}

// --- On-disk file mode (Cerberus #1464) ---

func TestServerkey_FileModes_Good(t *core.T) {
	// Cerberus #1464 — both .seed and server.key MUST be mode 0o600
	// after Bootstrap.
	home := homeFixture(t)
	svc := subject.NewService(nil)
	core.AssertTrue(t, svc.Bootstrap().OK)

	walletsDir := core.PathJoin(home, "Lethean", "wallets")
	for _, name := range []string{".seed", "server.key"} {
		statR := core.Stat(core.PathJoin(walletsDir, name))
		core.AssertTrue(t, statR.OK, "stat must succeed for "+name)
		info := statR.Value.(core.FsFileInfo)
		got := info.Mode().Perm()
		core.AssertEqual(t, core.FileMode(0o600), got)
	}
}

func TestServerkey_FileMode_Bad_Tampered(t *core.T) {
	// Cerberus #1464 — widened mode on .seed triggers fail-closed
	// on the NEXT Bootstrap call (new Service instance).
	home := homeFixture(t)
	svc := subject.NewService(nil)
	core.AssertTrue(t, svc.Bootstrap().OK)

	// Widen the seed file mode behind the service's back.
	seedPath := core.PathJoin(home, "Lethean", "wallets", ".seed")
	if err := subject.OsChmod(seedPath, 0o644); err != nil {
		t.Skipf("chmod unsupported on this fs: %v", err)
	}

	// Fresh service instance — has to re-load from disk.
	svc2 := subject.NewService(nil)
	r := svc2.Bootstrap()
	core.AssertTrue(t, !r.OK, "widened seed mode must trigger fail-closed Bootstrap (Cerberus #1464)")
}

// --- Canonicalise (Cerberus #1469) ---

func TestServerkey_Canonicalise_Good(t *core.T) {
	// Same header keys in different insertion orders must produce
	// IDENTICAL canonical bytes — that's what makes the signature
	// roundtripable across encoders.
	_ = homeFixture(t)
	svc := subject.NewService(nil)
	core.AssertTrue(t, svc.Bootstrap().OK)

	out1 := svc.IssueBootstrapToken().Value.(subject.BootstrapTokenOutput)
	out2 := svc.IssueBootstrapToken().Value.(subject.BootstrapTokenOutput)
	// Different nonces guarantee different tokens, but both verify.
	core.AssertTrue(t, svc.VerifyBootstrapToken(out1.Token, "account.create").OK)
	core.AssertTrue(t, svc.VerifyBootstrapToken(out2.Token, "account.create").OK)
	// Sanity: distinct tokens (different nonces).
	core.AssertTrue(t, out1.Token != out2.Token, "two mints must produce distinct tokens")
}

// --- IssueBootstrapTokenForScope (Stage E.B multi-scope) ---

func TestServerkey_IssueBootstrapTokenForScope_Good(t *core.T) {
	_ = homeFixture(t)
	svc := subject.NewService(nil)
	core.AssertTrue(t, svc.Bootstrap().OK)

	// account.unlock — the Stage E.B addition. Token verifies with
	// the matching scope.
	r := svc.IssueBootstrapTokenForScope("account.unlock")
	core.AssertTrue(t, r.OK, "unlock-scoped mint should succeed")
	out := r.Value.(subject.BootstrapTokenOutput)
	core.AssertTrue(t, svc.VerifyBootstrapToken(out.Token, "account.unlock").OK,
		"unlock-scoped token must verify with unlock scope")

	// Scope-mismatch protection (Cerberus #1467) — an unlock-scoped
	// token does NOT satisfy account.create.
	core.AssertTrue(t, !svc.VerifyBootstrapToken(out.Token, "account.create").OK,
		"unlock-scoped token MUST NOT satisfy account.create scope")
}

func TestServerkey_IssueBootstrapTokenForScope_RejectsUnknown_Bad(t *core.T) {
	// Unknown scope must reject — defends against a typo silently
	// minting an unverifiable token (the allow-list is the lockstep
	// other half of pkg/server.BootstrapPathScopes).
	_ = homeFixture(t)
	svc := subject.NewService(nil)
	core.AssertTrue(t, svc.Bootstrap().OK)

	r := svc.IssueBootstrapTokenForScope("totally.made.up.scope")
	core.AssertTrue(t, !r.OK, "unknown scope must reject")
	core.AssertEqual(t, "auth.bootstrap.scope", r.Code())
}

// --- IssueSessionToken / VerifySessionToken (Stage E.B per RFC §3) ---

const testAccountID = "abc123def4567890"

func TestServerkey_IssueSessionToken_Good(t *core.T) {
	_ = homeFixture(t)
	svc := subject.NewService(nil)
	core.AssertTrue(t, svc.Bootstrap().OK)

	r := svc.IssueSessionToken(testAccountID)
	core.AssertTrue(t, r.OK, "session-token mint should succeed")
	out := r.Value.(subject.SessionTokenOutput)

	// Token format — LTHN-SESS-1.<header>.<sig>
	core.AssertTrue(t, core.HasPrefix(out.Token, "LTHN-SESS-1."),
		"session token must carry the session prefix")
	parts := core.Split(out.Token[len("LTHN-SESS-1."):], ".")
	core.AssertEqual(t, 2, len(parts))

	// Carries the requested account_id for the bearer middleware.
	core.AssertEqual(t, testAccountID, out.AccountID)
}

func TestServerkey_SessionTokenRoundTrip_Good(t *core.T) {
	_ = homeFixture(t)
	svc := subject.NewService(nil)
	core.AssertTrue(t, svc.Bootstrap().OK)

	out := svc.IssueSessionToken(testAccountID).Value.(subject.SessionTokenOutput)
	vr := svc.VerifySessionToken(out.Token)
	core.AssertTrue(t, vr.OK, "fresh session token must verify")

	sv, ok := vr.Value.(subject.SessionVerifyOutput)
	core.AssertTrue(t, ok, "verify Result.Value must be SessionVerifyOutput")
	core.AssertEqual(t, testAccountID, sv.AccountID)
}

func TestServerkey_SessionTokenReusable_Good(t *core.T) {
	// Per RFC §3.1 — session tokens are consumed N times per session.
	// Multiple verifies of the SAME token must all succeed (the
	// bootstrap-token replay defence does NOT apply here).
	_ = homeFixture(t)
	svc := subject.NewService(nil)
	core.AssertTrue(t, svc.Bootstrap().OK)

	out := svc.IssueSessionToken(testAccountID).Value.(subject.SessionTokenOutput)
	for i := 0; i < 5; i++ {
		vr := svc.VerifySessionToken(out.Token)
		core.AssertTrue(t, vr.OK, "session token must verify repeatedly within TTL")
	}
}

func TestServerkey_IssueSessionToken_EmptyAccountID_Bad(t *core.T) {
	_ = homeFixture(t)
	svc := subject.NewService(nil)
	core.AssertTrue(t, svc.Bootstrap().OK)

	r := svc.IssueSessionToken("")
	core.AssertTrue(t, !r.OK, "empty account_id must reject")
	core.AssertEqual(t, "auth.session.account_id", r.Code())
}

func TestServerkey_IssueSessionToken_WithoutBootstrap_Bad(t *core.T) {
	_ = homeFixture(t)
	svc := subject.NewService(nil)
	r := svc.IssueSessionToken(testAccountID)
	core.AssertTrue(t, !r.OK, "IssueSessionToken without Bootstrap must fail")
}

func TestServerkey_VerifySessionToken_WithoutBootstrap_Bad(t *core.T) {
	_ = homeFixture(t)
	svc := subject.NewService(nil)
	r := svc.VerifySessionToken("LTHN-SESS-1.x.y")
	core.AssertTrue(t, !r.OK, "VerifySessionToken without Bootstrap must fail")
}

func TestServerkey_VerifySessionToken_Format_Bad(t *core.T) {
	_ = homeFixture(t)
	svc := subject.NewService(nil)
	core.AssertTrue(t, svc.Bootstrap().OK)

	// Wrong prefix — bootstrap token must NOT satisfy the session
	// verifier (Cerberus #1467 scope-laundering defence at the
	// type-system layer).
	bootOut := svc.IssueBootstrapToken().Value.(subject.BootstrapTokenOutput)
	r1 := svc.VerifySessionToken(bootOut.Token)
	core.AssertTrue(t, !r1.OK, "bootstrap-prefixed token must reject session verify")
	core.AssertEqual(t, "auth.session.format", r1.Code())

	// Single segment — malformed body.
	r2 := svc.VerifySessionToken("LTHN-SESS-1.onlyonesegment")
	core.AssertTrue(t, !r2.OK)
	core.AssertEqual(t, "auth.session.format", r2.Code())
}

func TestServerkey_VerifySessionToken_Tampered_Ugly(t *core.T) {
	// Cerberus #1469 — flipping any byte in the header part must
	// fail verify (re-canonicalised bytes won't match the signed
	// originals — same discipline as bootstrap-token tampering).
	_ = homeFixture(t)
	svc := subject.NewService(nil)
	core.AssertTrue(t, svc.Bootstrap().OK)

	out := svc.IssueSessionToken(testAccountID).Value.(subject.SessionTokenOutput)
	rest := out.Token[len("LTHN-SESS-1."):]
	parts := core.Split(rest, ".")
	core.AssertEqual(t, 2, len(parts))
	header := parts[0]
	mid := len(header) / 2
	swapped := byte('A')
	if header[mid] == 'A' {
		swapped = 'B'
	}
	mangled := header[:mid] + string([]byte{swapped}) + header[mid+1:]
	tampered := "LTHN-SESS-1." + mangled + "." + parts[1]

	r := svc.VerifySessionToken(tampered)
	core.AssertTrue(t, !r.OK, "tampered session-token header must reject")
}

func TestServerkey_SessionToken_DistinctMints_Good(t *core.T) {
	// Two consecutive mints produce distinct tokens (different
	// nonces) — defends against an internal mint loop emitting the
	// same nonce twice. Both verify independently.
	_ = homeFixture(t)
	svc := subject.NewService(nil)
	core.AssertTrue(t, svc.Bootstrap().OK)

	out1 := svc.IssueSessionToken(testAccountID).Value.(subject.SessionTokenOutput)
	out2 := svc.IssueSessionToken(testAccountID).Value.(subject.SessionTokenOutput)
	core.AssertTrue(t, out1.Token != out2.Token, "two mints must produce distinct tokens")
	core.AssertTrue(t, svc.VerifySessionToken(out1.Token).OK)
	core.AssertTrue(t, svc.VerifySessionToken(out2.Token).OK)
}
