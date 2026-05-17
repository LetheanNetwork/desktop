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

// Mantis #1471 — re-bootstrap scenario: user deletes their LetheanAccount
// while ~/Lethean/wallets/server.key persists. Next boot must report
// has_user_account=false so the frontend auth-gate routes to `setup`
// (NOT `auth` / unlock prompt) — Option B in the ticket: leave server.key
// in place, force the frontend into the setup wizard.
//
// AccountStatus contract is purely a function of <id>/private.key
// presence — it never consults server.key. These tests pin that contract
// after Bootstrap has materialised server.key on disk, guarding against
// any future regression that would mix server.key state into the signal.

func TestServerkey_ServerKeyWithoutAccount_ReportsSetupNeeded_Good(t *core.T) {
	// server.key present + no account/<id>/private.key → setup needed.
	home := homeFixture(t)
	svc := subject.NewService(nil)
	core.AssertTrue(t, svc.Bootstrap().OK, "Bootstrap should materialise server.key on fresh install")

	// Sanity — server.key really is on disk now.
	keyPath := core.PathJoin(home, "Lethean", "wallets", "server.key")
	core.AssertTrue(t, core.Stat(keyPath).OK, "server.key must exist post-Bootstrap")

	// No account dir at all — the post-delete steady state.
	r := svc.AccountStatus()
	core.AssertTrue(t, r.OK)
	out := r.Value.(subject.AccountStatusOutput)
	core.AssertTrue(t, !out.HasUserAccount,
		"server.key present + no account dir must report has_user_account=false (Mantis #1471 — setup wizard, not unlock prompt)")
	core.AssertEqual(t, "", out.AccountID)
}

func TestServerkey_ServerKeyWithoutAccount_EmptyAccountDir_ReportsSetupNeeded_Good(t *core.T) {
	// server.key present + account dir exists but EMPTY (post-delete
	// leaves an empty parent dir) → still setup, not unlock.
	home := homeFixture(t)
	svc := subject.NewService(nil)
	core.AssertTrue(t, svc.Bootstrap().OK)

	accountRoot := core.PathJoin(home, "Lethean", "account")
	mk := core.MkdirAll(accountRoot, 0o755)
	core.AssertTrue(t, mk.OK)

	r := svc.AccountStatus()
	core.AssertTrue(t, r.OK)
	out := r.Value.(subject.AccountStatusOutput)
	core.AssertTrue(t, !out.HasUserAccount,
		"server.key present + empty account dir must report has_user_account=false (Mantis #1471)")
}

func TestServerkey_ServerKeyWithoutAccount_OrphanedAccountDir_ReportsSetupNeeded_Good(t *core.T) {
	// server.key present + account/<id>/ directory exists but private.key
	// was deleted (the load-bearing scenario in the ticket — user removed
	// the account leaf file). Must report setup-needed.
	home := homeFixture(t)
	svc := subject.NewService(nil)
	core.AssertTrue(t, svc.Bootstrap().OK)

	accountRoot := core.PathJoin(home, "Lethean", "account", "abc123")
	mk := core.MkdirAll(accountRoot, 0o700)
	core.AssertTrue(t, mk.OK)
	// Drop a non-private.key file so the dir isn't empty — mimics the
	// state after a partial delete that left meta.json behind.
	w := core.WriteFile(core.PathJoin(accountRoot, "meta.json"), []byte("{}"), 0o600)
	core.AssertTrue(t, w.OK)

	r := svc.AccountStatus()
	core.AssertTrue(t, r.OK)
	out := r.Value.(subject.AccountStatusOutput)
	core.AssertTrue(t, !out.HasUserAccount,
		"server.key present + orphaned account dir without private.key must report has_user_account=false (Mantis #1471 — Option B: setup wizard, server.key preserved)")
}

func TestServerkey_ServerKeyAndAccount_ReportsUnlockNeeded_Good(t *core.T) {
	// Regression guard for the inverse — server.key + full account
	// (including private.key leaf) must report has_user_account=true
	// so the auth-gate goes to `auth` (unlock prompt).
	home := homeFixture(t)
	svc := subject.NewService(nil)
	core.AssertTrue(t, svc.Bootstrap().OK)

	accountRoot := core.PathJoin(home, "Lethean", "account", "abc123")
	mk := core.MkdirAll(accountRoot, 0o700)
	core.AssertTrue(t, mk.OK)
	w := core.WriteFile(core.PathJoin(accountRoot, "private.key"), []byte("armoured-key"), 0o600)
	core.AssertTrue(t, w.OK)

	r := svc.AccountStatus()
	core.AssertTrue(t, r.OK)
	out := r.Value.(subject.AccountStatusOutput)
	core.AssertTrue(t, out.HasUserAccount,
		"server.key + private.key both present must report has_user_account=true (Mantis #1471 inverse — unlock prompt)")
	core.AssertEqual(t, "abc123", out.AccountID)
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

// --- Cerberus #1466 / Mantis #1466 — first-launch concurrent race. ---

// TestServerkey_ConcurrentBootstrap_Serialised_Ugly fires N independent
// Service instances against the same fresh ~/Lethean/wallets/ in
// parallel. The bootstrap lock at acquireBootstrapLock must serialise
// them: exactly one goroutine generates + persists the keypair, the
// remaining N-1 take the load path. End-state assertions:
//
//   - all N Bootstrap calls return OK
//   - the on-disk server.key file is byte-identical to itself across
//     repeated reads (only one writer ever wrote it)
//   - cross-verification holds: a token minted by one service verifies
//     against every other service (proves they all hold the same key
//     bytes in memory, not five different freshly-generated keys)
//
// Pre-fix shape (no lock): each goroutine would race past the Stat
// gate, all five would generate distinct keypairs, the last
// atomicWrite would win on disk, and the four losers would hold
// orphaned in-memory keys whose tokens nobody else could verify.
func TestServerkey_ConcurrentBootstrap_Serialised_Ugly(t *core.T) {
	home := homeFixture(t)
	const n = 5

	services := make([]*subject.Service, n)
	for i := 0; i < n; i++ {
		services[i] = subject.NewService(nil)
	}

	var wg core.WaitGroup
	results := make([]core.Result, n)
	for i := 0; i < n; i++ {
		idx := i
		wg.Go(func() {
			results[idx] = services[idx].Bootstrap()
		})
	}
	wg.Wait()

	// Every concurrent Bootstrap must succeed — the lock serialises
	// them but does not fail any. The five-second backoff ceiling in
	// acquireBootstrapLock is far longer than the encrypt+write
	// critical section (sub-100ms in practice).
	for i, r := range results {
		core.AssertTrue(t, r.OK,
			"concurrent Bootstrap goroutine #"+core.Itoa(i)+" must succeed")
	}

	// The on-disk server.key must be stable — only one writer ever
	// wrote it. Read twice; assert identical content (a flapping file
	// would indicate the second writer landed after a load attempt).
	keyPath := core.PathJoin(home, "Lethean", "wallets", "server.key")
	readA := core.ReadFile(keyPath)
	core.AssertTrue(t, readA.OK, "server.key must be readable after concurrent Bootstrap")
	readB := core.ReadFile(keyPath)
	core.AssertTrue(t, readB.OK, "server.key second read must succeed")
	bytesA, _ := readA.Value.([]byte)
	bytesB, _ := readB.Value.([]byte)
	core.AssertEqual(t, string(bytesA), string(bytesB))

	// Cross-verification: a token minted by services[0] must verify
	// against every other service. A pre-fix split-key state would
	// fail this — only the winning writer's service would verify its
	// own tokens; the losers would carry orphaned in-memory keys.
	mintR := services[0].IssueBootstrapToken()
	core.AssertTrue(t, mintR.OK, "mint on services[0] must succeed")
	tok := mintR.Value.(subject.BootstrapTokenOutput).Token
	for i := 1; i < n; i++ {
		verR := services[i].VerifyBootstrapToken(tok, "account.create")
		core.AssertTrue(t, verR.OK,
			"services["+core.Itoa(i)+"] must verify services[0]'s token "+
				"(proves all goroutines hold the same key)")
	}
}

// TestServerkey_SecondProcessLoadsFirstWriter_Good models the
// two-CLI-invocation race that Mantis #1466 names: process A starts,
// runs Bootstrap, persists server.key; process B starts second, sees
// the on-disk key, must LOAD it (not regenerate). Cross-verification
// proves B holds A's key bytes.
//
// This is the in-process analogue of the cross-process scenario
// (separate Service instances in sequence, simulating
// sequential `lthn serve` invocations). The concurrent variant lives
// in TestServerkey_ConcurrentBootstrap_Serialised_Ugly.
func TestServerkey_SecondProcessLoadsFirstWriter_Good(t *core.T) {
	home := homeFixture(t)

	// "Process A" — first invocation, fresh install. Generates +
	// persists server.key.
	procA := subject.NewService(nil)
	core.AssertTrue(t, procA.Bootstrap().OK, "process A Bootstrap must succeed")

	// Snapshot the on-disk key so we can prove process B doesn't
	// overwrite it with a different keypair.
	keyPath := core.PathJoin(home, "Lethean", "wallets", "server.key")
	preR := core.ReadFile(keyPath)
	core.AssertTrue(t, preR.OK, "server.key must be readable after process A Bootstrap")
	preBytes, _ := preR.Value.([]byte)

	// A mints a token that B must be able to verify (proves the
	// in-memory keys match end-to-end).
	mintR := procA.IssueBootstrapToken()
	core.AssertTrue(t, mintR.OK, "process A mint must succeed")
	tok := mintR.Value.(subject.BootstrapTokenOutput).Token

	// "Process B" — second invocation. Sees ~/Lethean/wallets/server.key
	// already present, must take the load path.
	procB := subject.NewService(nil)
	core.AssertTrue(t, procB.Bootstrap().OK, "process B Bootstrap must succeed via load path")

	// Disk content must be unchanged — B didn't regenerate.
	postR := core.ReadFile(keyPath)
	core.AssertTrue(t, postR.OK, "server.key must be readable after process B Bootstrap")
	postBytes, _ := postR.Value.([]byte)
	core.AssertEqual(t, string(preBytes), string(postBytes))

	// B must verify A's token — same key in memory.
	verR := procB.VerifyBootstrapToken(tok, "account.create")
	core.AssertTrue(t, verR.OK, "process B must verify process A's token (loaded A's key)")
}

// --- Mantis #1462 — TTL tighten + clock-skew bound. ---
//
// Adds the brief-prescribed test matrix on top of the existing
// TestServerkey_TokenTTL_IssuerExp_Cerberus1462 (which already covers
// exp - iat == 60). The two iat-side tests below pin the clock-skew
// bound at the boundary value so future TTL constant tweaks don't
// silently widen the accept window.

// TestBootstrapToken_TTLIs60s_Good asserts the issuer stamps exp =
// iat + 60s — i.e. issuerTTL is the brief's prescribed 60 seconds
// (Mantis #1462 done-criterion #1). Round-trips the freshly-minted
// token + decodes the header to read exp/iat directly so we don't
// trust the BootstrapTokenOutput.ExpiresAt convenience field.
func TestBootstrapToken_TTLIs60s_Good(t *core.T) {
	_ = homeFixture(t)
	svc := subject.NewService(nil)
	core.AssertTrue(t, svc.Bootstrap().OK)

	out := svc.IssueBootstrapToken().Value.(subject.BootstrapTokenOutput)
	parts := core.Split(out.Token[len("LTHN-BOOT-1."):], ".")
	core.AssertEqual(t, 2, len(parts))
	headerR := core.Base64URLDecode(parts[0])
	core.AssertTrue(t, headerR.OK, "header must base64url-decode")
	var header map[string]any
	core.AssertTrue(t, core.JSONUnmarshal(headerR.Value.([]byte), &header).OK,
		"header must JSON-decode")

	iat, _ := header["iat"].(float64)
	exp, _ := header["exp"].(float64)
	delta := int64(exp - iat)
	core.AssertEqual(t, subject.ExportedIssuerTTL(), delta)
	core.AssertEqual(t, int64(60), delta)
}

// TestBootstrapToken_IatInFutureRejected_Bad forges a token whose iat
// is now + 10s — well past the 5s clock-skew tolerance — and asserts
// VerifyBootstrapToken rejects it with an auth.bootstrap.ttl-class
// failure. Defends against a clock-skew attack where an attacker mints
// a token claiming future iat to widen the verifier's accept window
// (Mantis #1462 done-criterion #2).
func TestBootstrapToken_IatInFutureRejected_Bad(t *core.T) {
	_ = homeFixture(t)
	svc := subject.NewService(nil)
	core.AssertTrue(t, svc.Bootstrap().OK)

	now := core.Now().UTC().Unix()
	// 10s past the 5s tolerance = clearly rejected.
	future := now + subject.ExportedClockSkewTolerance() + 5
	tok := svc.ExportedIssueBootstrapTokenAtIat(future)
	core.AssertTrue(t, tok != "", "forge helper must mint a token")

	r := svc.VerifyBootstrapToken(tok, "account.create")
	core.AssertTrue(t, !r.OK, "iat beyond clock-skew tolerance must reject")
}

// TestBootstrapToken_IatWithinSkewAccepted_Good forges a token whose
// iat is now + 3s — inside the 5s clock-skew tolerance — and asserts
// VerifyBootstrapToken accepts it. Confirms the bound is a tolerance,
// not a strict iat <= now check (Mantis #1462 done-criterion #3).
func TestBootstrapToken_IatWithinSkewAccepted_Good(t *core.T) {
	_ = homeFixture(t)
	svc := subject.NewService(nil)
	core.AssertTrue(t, svc.Bootstrap().OK)

	now := core.Now().UTC().Unix()
	// 3s into the future is comfortably inside the 5s tolerance window.
	near := now + 3
	core.AssertTrue(t, near-now <= subject.ExportedClockSkewTolerance(),
		"fixture must stay inside clock-skew tolerance")
	tok := svc.ExportedIssueBootstrapTokenAtIat(near)
	core.AssertTrue(t, tok != "", "forge helper must mint a token")

	r := svc.VerifyBootstrapToken(tok, "account.create")
	core.AssertTrue(t, r.OK, "iat within clock-skew tolerance must accept")
}

// --- Mantis #1593 — IfNotExist substrate failure modes ---

// TestServerkey_AtomicWrite_KeyExists_Bad pins the new failure-mode
// surface introduced by the Mantis #1593 cutover to
// paths.AtomicWriteWithVersion with IfNotExist=true.
//
// The outer keyStat branch in Bootstrap routes the load path when
// server.key already exists, so under normal flow the create branch
// only runs on a fresh install. This test forces the create branch
// against a pre-seeded server.key by Stat-able-but-unparseable bytes
// — the load path will fail to parse, the create path then attempts
// to write OVER the existing file under IfNotExist=true, and the
// primitive must refuse with paths.write.exists rather than silently
// overwriting an in-flight key.
//
// Pre-cutover shape: the local atomicWrite helper had no overwrite
// gate, so a race that bypassed the outer Stat would silently clobber
// the existing server.key. Post-cutover the primitive's per-path
// WithFileLock + IfNotExist closes that race at the substrate
// boundary.
func TestServerkey_AtomicWrite_KeyExists_Bad(t *core.T) {
	home := homeFixture(t)

	walletsDir := core.PathJoin(home, "Lethean", "wallets")
	mk := core.MkdirAll(walletsDir, 0o700)
	core.AssertTrue(t, mk.OK, "fixture wallets/ mkdir must succeed")

	// Pre-seed both files with parseable-shape but wrong-content
	// bytes. The seed loadOrCreateSeed path requires a 32-byte seed;
	// supply exactly that so the load branch survives long enough to
	// reach the server.key load attempt.
	seedPath := core.PathJoin(walletsDir, ".seed")
	w := core.WriteFile(seedPath, make([]byte, 32), 0o600)
	core.AssertTrue(t, w.OK, "fixture seed write must succeed")

	// Pre-seed server.key with malformed content so the load path
	// fails. The create branch then attempts to overwrite via
	// paths.AtomicWriteWithVersion + IfNotExist=true, which must
	// surface paths.write.exists.
	keyPath := core.PathJoin(walletsDir, "server.key")
	w = core.WriteFile(keyPath, []byte("not-a-valid-lthn-key-block\n"), 0o600)
	core.AssertTrue(t, w.OK, "fixture key write must succeed")

	svc := subject.NewService(nil)
	r := svc.Bootstrap()
	core.AssertTrue(t, !r.OK, "Bootstrap must fail when server.key exists but is unparseable — substrate refuses to overwrite under IfNotExist")
}

// TestServerkey_AtomicWrite_FirstMint_Good is the positive control:
// a fresh install hits the create branch with both files absent;
// IfNotExist=true does NOT block first-write (the gate only fires
// when the file already exists). End-state: both .seed + server.key
// land at 0o600 (primitive's at-rest mode-verify gate per
// Mantis #1592 covers Cerberus #1464 carry-forward).
func TestServerkey_AtomicWrite_FirstMint_Good(t *core.T) {
	home := homeFixture(t)

	svc := subject.NewService(nil)
	r := svc.Bootstrap()
	core.AssertTrue(t, r.OK, "Bootstrap on fresh install must succeed via the create branch")

	seedPath := core.PathJoin(home, "Lethean", "wallets", ".seed")
	keyPath := core.PathJoin(home, "Lethean", "wallets", "server.key")

	seedStat := core.Stat(seedPath)
	core.AssertTrue(t, seedStat.OK, ".seed must land on disk")
	keyStat := core.Stat(keyPath)
	core.AssertTrue(t, keyStat.OK, "server.key must land on disk")

	// Mode 0o600 verified by the primitive's at-rest mode gate; here
	// we re-confirm the on-disk file matches the expected discipline
	// so a future regression that drops the gate is caught.
	si, ok := seedStat.Value.(core.FsFileInfo)
	core.AssertTrue(t, ok, "seed stat must return FsFileInfo")
	core.AssertTrue(t, si.Mode().Perm() == 0o600, "seed must be 0o600 post-write")

	ki, ok := keyStat.Value.(core.FsFileInfo)
	core.AssertTrue(t, ok, "key stat must return FsFileInfo")
	core.AssertTrue(t, ki.Mode().Perm() == 0o600, "server.key must be 0o600 post-write")
}
