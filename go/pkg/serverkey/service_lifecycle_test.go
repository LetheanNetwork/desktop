// SPDX-Licence-Identifier: EUPL-1.2

// Tests for the CoreGUI service-lifecycle surface (Register,
// ServiceName, ServiceStartup, ServiceShutdown) plus the AccountStatus
// / verifyMode error branches that the happy-path tests in
// serverkey_test.go never reach.

package serverkey_test

import (
	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/serverkey"
)

// --- CoreGUI lifecycle ---

func TestService_Register_Good(t *core.T) {
	_ = homeFixture(t)
	r := subject.Register(nil)
	core.AssertTrue(t, r.OK, "Register should always succeed — construction only")
	svc, ok := r.Value.(*subject.Service)
	core.AssertTrue(t, ok, "Register must return *Service")
	core.AssertTrue(t, svc != nil, "Register must not return a nil Service")
}

func TestService_ServiceName_Good_ReturnsBindingNamespace(t *core.T) {
	svc := subject.NewService(nil)
	core.AssertEqual(t, "ServerKey", svc.ServiceName())
}

func TestService_ServiceStartup_Good_IsNoop(t *core.T) {
	svc := subject.NewService(nil)
	r := svc.ServiceStartup(core.Background(), nil)
	core.AssertTrue(t, r.OK, "ServiceStartup is a documented no-op — must always return OK")
}

func TestService_ServiceShutdown_Good_ClearsPrivateKey(t *core.T) {
	_ = homeFixture(t)
	svc := subject.NewService(nil)
	core.AssertTrue(t, svc.Bootstrap().OK, "Bootstrap should succeed")

	// Bootstrapped — a session token mint proves the private key is
	// live in memory before shutdown.
	core.AssertTrue(t, svc.IssueBootstrapToken().OK, "mint should succeed while bootstrapped")

	r := svc.ServiceShutdown()
	core.AssertTrue(t, r.OK, "ServiceShutdown should always return OK")

	// Private key cleared — minting now fails with the
	// not-bootstrapped error, proving ServiceShutdown really zeroed
	// s.privateKey rather than being a no-op.
	after := svc.IssueBootstrapToken()
	core.AssertTrue(t, !after.OK, "mint after ServiceShutdown must fail — private key must be cleared")
}

// --- AccountStatus error branches ---

func TestService_AccountStatus_Bad_RootUnwritable(t *core.T) {
	// Mirrors TestServerkey_Bootstrap_Bad's HOME-as-file trick —
	// paths.Root()'s MkdirAll fails, so AccountStatus must surface
	// that failure rather than silently reporting no-account.
	tmp := t.TempDir()
	tmpFile := core.PathJoin(tmp, "home-as-file")
	w := core.WriteFile(tmpFile, []byte{}, 0o600)
	core.AssertTrue(t, w.OK, "fixture file write should succeed")
	t.Setenv("HOME", tmpFile)

	svc := subject.NewService(nil)
	r := svc.AccountStatus()
	core.AssertTrue(t, !r.OK, "AccountStatus should surface paths.Root() failure when HOME is unwritable")
}

func TestService_AccountStatus_Bad_AccountDirUnreadable(t *core.T) {
	home := homeFixture(t)
	accountRoot := core.PathJoin(home, "Lethean", "account")
	mk := core.MkdirAll(accountRoot, 0o700)
	core.AssertTrue(t, mk.OK, "fixture account dir mkdir must succeed")

	// Strip read+execute so ReadDir fails even though the directory
	// (and its parent) exist and are Stat-able.
	if err := subject.OsChmod(accountRoot, 0o000); err != nil {
		t.Skipf("chmod unsupported on this fs: %v", err)
	}
	defer func() { _ = subject.OsChmod(accountRoot, 0o700) }() // restore before t.TempDir() cleanup runs

	svc := subject.NewService(nil)
	r := svc.AccountStatus()
	core.AssertTrue(t, r.OK, "an unreadable account dir must fall back to has_user_account=false, not error")
	out, ok := r.Value.(subject.AccountStatusOutput)
	core.AssertTrue(t, ok, "AccountStatus must return AccountStatusOutput")
	core.AssertTrue(t, !out.HasUserAccount, "unreadable account dir must report has_user_account=false")
}

func TestService_AccountStatus_Ugly_NonDirEntrySkipped(t *core.T) {
	// A regular file sitting directly under ~/Lethean/account/ (never
	// produced by the real account-creation path, but a corrupt/
	// tampered directory listing is exactly the sort of fault this
	// package must not choke on) must be skipped by the `!e.IsDir()`
	// guard rather than treated as a candidate account directory.
	home := homeFixture(t)
	accountRoot := core.PathJoin(home, "Lethean", "account")
	mk := core.MkdirAll(accountRoot, 0o700)
	core.AssertTrue(t, mk.OK, "fixture account dir mkdir must succeed")

	strayFile := core.PathJoin(accountRoot, "stray-file")
	w := core.WriteFile(strayFile, []byte("not an account dir"), 0o600)
	core.AssertTrue(t, w.OK, "fixture stray file write must succeed")

	svc := subject.NewService(nil)
	r := svc.AccountStatus()
	core.AssertTrue(t, r.OK, "AccountStatus should succeed")
	out, ok := r.Value.(subject.AccountStatusOutput)
	core.AssertTrue(t, ok, "AccountStatus must return AccountStatusOutput")
	core.AssertTrue(t, !out.HasUserAccount, "a stray non-directory entry must never be treated as an account")
}

// --- verifyMode (Cerberus #1464) — the server.key half ---

func TestService_VerifyMode_Bad_ServerKeyTampered(t *core.T) {
	// TestServerkey_FileMode_Bad_Tampered (serverkey_test.go) covers
	// the .seed half of Cerberus #1464; this covers the server.key
	// half — the OTHER verifyMode call site inside Bootstrap's
	// load-path branch.
	home := homeFixture(t)
	svc := subject.NewService(nil)
	core.AssertTrue(t, svc.Bootstrap().OK, "first Bootstrap should succeed")

	keyPath := core.PathJoin(home, "Lethean", "wallets", "server.key")
	if err := subject.OsChmod(keyPath, 0o644); err != nil {
		t.Skipf("chmod unsupported on this fs: %v", err)
	}

	svc2 := subject.NewService(nil)
	r := svc2.Bootstrap()
	core.AssertTrue(t, !r.OK, "widened server.key mode must trigger fail-closed Bootstrap (Cerberus #1464)")
}
