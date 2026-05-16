// SPDX-Licence-Identifier: EUPL-1.2

// Tests for the account.Service Create contract. Triadic Good / Bad /
// Ugly coverage per AX-9 + the RFC §10 Stage B' requirement. One
// named test per Cerberus #1460 MUST-NOT so the contract is grep-able
// from `go test -run #1460`.
//
// Test isolation via homeFixture rebinding $HOME so the real
// ~/Lethean/ tree is never touched.

package account_test

import (
	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/account"
)

// homeFixture rebinds $HOME to a t-scoped temp dir for the test and
// returns the temp root. Every paths.Root() call resolves underneath
// it so writes can't escape into the developer's real account tree.
func homeFixture(t *core.T) string {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	return tmp
}

// validInput returns a fixture CreateInput whose AccountID matches
// the canonical derivation of the supplied PublicKey. Used by every
// Good-path test as the baseline "correct shape" caller; Bad/Ugly
// tests mutate one field to surface a single contract violation.
func validInput() subject.CreateInput {
	pub := []byte("-----BEGIN LTHN PUBLIC KEY-----\nfixture-public-bytes\n-----END LTHN PUBLIC KEY-----\n")
	return subject.CreateInput{
		PublicKey: pub,
		AccountID: subject.DeriveAccountID(pub),
	}
}

// --- Create — Good ---

func TestAccount_Create_Good(t *core.T) {
	home := homeFixture(t)
	svc := subject.NewService(nil)

	in := validInput()
	r := svc.Create(in)
	core.AssertTrue(t, r.OK, "Create on fresh install must succeed")

	out, ok := r.Value.(subject.CreateOutput)
	core.AssertTrue(t, ok, "Result.Value must be CreateOutput")
	core.AssertEqual(t, in.AccountID, out.AccountID)

	wantPath := core.PathJoin(home, "Lethean", "account", in.AccountID)
	core.AssertEqual(t, wantPath, out.Path)

	// Leaf file landed — AccountStatus must now report has_account=true
	// (mirrors the #1471 leaf-signal rule).
	statR := svc.AccountStatus()
	core.AssertTrue(t, statR.OK)
	st := statR.Value.(subject.AccountStatus)
	core.AssertTrue(t, st.HasAccount, "AccountStatus must report true after Create")
	core.AssertEqual(t, in.AccountID, st.AccountID)
}

// --- Create — #1460 (a) — Bad: refuse to overwrite ---

func TestAccount_Create_AccountExists_Bad_1460a(t *core.T) {
	_ = homeFixture(t)
	svc := subject.NewService(nil)

	in := validInput()
	first := svc.Create(in)
	core.AssertTrue(t, first.OK, "first Create must succeed")

	second := svc.Create(in)
	core.AssertFalse(t, second.OK, "second Create with same id must fail")
	core.AssertEqual(t, "account.exists", second.Code(),
		"second Create must surface account.exists (Cerberus #1460 (a))")
}

// --- Create — #1460 (b) — Bad: ID mismatch rejection ---

func TestAccount_Create_IDMismatch_Bad_1460b(t *core.T) {
	_ = homeFixture(t)
	svc := subject.NewService(nil)

	in := validInput()
	in.AccountID = "0000000000000000" // deliberately wrong — not the SHA-256[0:16] of PublicKey

	r := svc.Create(in)
	core.AssertFalse(t, r.OK, "Create with mismatched id must fail")
	core.AssertEqual(t, "account.id_mismatch", r.Code(),
		"mismatch must surface account.id_mismatch (Cerberus #1460 (b))")

	// No disk write should have happened — verify the canonical id's
	// directory was never created.
	canonical := subject.DeriveAccountID(in.PublicKey)
	statR := svc.AccountStatus()
	core.AssertTrue(t, statR.OK)
	st := statR.Value.(subject.AccountStatus)
	core.AssertFalse(t, st.HasAccount, "Create must not persist on id mismatch")
	core.AssertNotEqual(t, canonical, in.AccountID, "fixture sanity: canonical and supplied id must differ")
}

// --- Create — #1460 (c) — Ugly: private.key MUST be written last ---

func TestAccount_Create_PrivateKeyWrittenLast_Ugly_1460c(t *core.T) {
	// Cerberus #1460 (c) crash-safety check: when AccountStatus reads
	// a partial directory (public.key + meta.json present, no
	// private.key), it must report has_user_account=false because
	// private.key is the LEAF signal (#1471).
	//
	// We can't kill the goroutine mid-rename inside a unit test, but
	// we CAN simulate the exact post-crash filesystem state: manually
	// land public.key + meta.json under the account dir, omit
	// private.key, then assert AccountStatus reports false. This pins
	// the contract that future Create implementations must preserve
	// the leaf-rename ordering.
	home := homeFixture(t)
	svc := subject.NewService(nil)

	in := validInput()
	dir := core.PathJoin(home, "Lethean", "account", in.AccountID)
	mk := core.MkdirAll(dir, 0o700)
	core.AssertTrue(t, mk.OK, "fixture mkdir must succeed")

	// Pretend Create crashed AFTER public.key + meta.json rename, BEFORE
	// private.key rename.
	w1 := core.WriteFile(core.PathJoin(dir, "public.key"), in.PublicKey, 0o600)
	core.AssertTrue(t, w1.OK)
	w2 := core.WriteFile(core.PathJoin(dir, "meta.json"), []byte(`{"account_id":"x"}`), 0o600)
	core.AssertTrue(t, w2.OK)

	st := svc.AccountStatus()
	core.AssertTrue(t, st.OK)
	out := st.Value.(subject.AccountStatus)
	core.AssertFalse(t, out.HasAccount,
		"partial account (no private.key) MUST report has_account=false (Cerberus #1460 (c) + #1471)")

	// And the retry must still be permitted — Create() refuses only
	// when the leaf file is present, so the half-written state can be
	// recovered by running Create again.
	r := svc.Create(in)
	core.AssertTrue(t, r.OK,
		"Create must succeed when retrying over a half-written directory (no private.key present)")
}

// --- Create — #1460 (d) — middleware-enforced nonce burn ---
//
// The nonce-consumption-before-write contract is structurally enforced
// by pkg/server.BootstrapAuthMiddleware running BEFORE the handler
// (the middleware calls serverkey.VerifyBootstrapToken, which adds the
// nonce to the consumed set on return). pkg/server's bootstrap_auth_test
// already pins the middleware burn ordering; pkg/account's Create()
// does NOT touch nonces and therefore has no nonce-specific assertion
// here. The routes-level test in routes_test.go closes the loop end-
// to-end (token issue → POST → handler → 200) so the absence of a
// duplicate burn here is intentional, not a gap.

// --- DeriveAccountID — Good / Bad ---

func TestAccount_DeriveAccountID_Good(t *core.T) {
	pub := []byte("fixture-public-bytes")
	id := subject.DeriveAccountID(pub)
	core.AssertLen(t, id, 16, "canonical account id is 16 hex chars")

	// Deterministic — same input, same output, every call.
	core.AssertEqual(t, id, subject.DeriveAccountID(pub))

	// Different input, different output.
	id2 := subject.DeriveAccountID([]byte("different-bytes"))
	core.AssertNotEqual(t, id, id2)
}

func TestAccount_DeriveAccountID_Bad_Empty(t *core.T) {
	// Empty input is still derivable (SHA-256 of empty bytes is a
	// well-defined value); the API is total. Service.Create rejects
	// empty PublicKey separately with codePublicKeyRequired so the
	// deriver doesn't need to police it.
	id := subject.DeriveAccountID([]byte{})
	core.AssertLen(t, id, 16)
}

// --- Create — input validation ---

func TestAccount_Create_PublicKeyRequired_Bad(t *core.T) {
	_ = homeFixture(t)
	svc := subject.NewService(nil)

	r := svc.Create(subject.CreateInput{PublicKey: nil, AccountID: "anything"})
	core.AssertFalse(t, r.OK)
	core.AssertEqual(t, "account.public_key.required", r.Code())
}

func TestAccount_Create_AccountIDRequired_Bad(t *core.T) {
	_ = homeFixture(t)
	svc := subject.NewService(nil)

	r := svc.Create(subject.CreateInput{PublicKey: []byte("anything"), AccountID: ""})
	core.AssertFalse(t, r.OK)
	core.AssertEqual(t, "account.id.required", r.Code())
}

// --- AccountStatus — fresh install ---

func TestAccount_AccountStatus_Good_FreshInstall(t *core.T) {
	_ = homeFixture(t)
	svc := subject.NewService(nil)

	r := svc.AccountStatus()
	core.AssertTrue(t, r.OK)
	out := r.Value.(subject.AccountStatus)
	core.AssertFalse(t, out.HasAccount, "fresh install — no account expected")
	core.AssertEqual(t, "", out.AccountID)
}
