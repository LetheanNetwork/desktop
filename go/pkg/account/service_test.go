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
	"os"

	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/account"
	"dappco.re/lthn/desktop/pkg/audit"
	"dappco.re/lthn/desktop/pkg/paths"
)

// recordingRecorder captures every Event the audit recorder receives.
// Mirrors pkg/server/plugin_view_capability_test.go — same fixture
// shape so the test layer stays uniform across audit-emitting handlers.
//
// Usage example:
//
//	rec := &recordingRecorder{}
//	audit.SetDefault(rec)
//	t.Cleanup(func() { audit.SetDefault(nil) })
type recordingRecorder struct {
	events []audit.Event
}

func (r *recordingRecorder) Record(ev audit.Event) core.Result {
	r.events = append(r.events, ev)
	return core.Ok(nil)
}

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

// TestAccount_AccountStatus_Bad_NonDirEntryIgnored — a stray file
// sitting directly under ~/Lethean/account/ (never a real account
// shape) must be skipped, not mistaken for an account directory.
func TestAccount_AccountStatus_Bad_NonDirEntryIgnored(t *core.T) {
	home := homeFixture(t)
	svc := subject.NewService(nil)
	accountRoot := core.PathJoin(home, "Lethean", "account")
	core.AssertTrue(t, core.MkdirAll(accountRoot, 0o700).OK)
	core.AssertTrue(t, core.WriteFile(
		core.PathJoin(accountRoot, "stray.txt"), []byte("not an account"), 0o600,
	).OK)

	r := svc.AccountStatus()

	core.AssertTrue(t, r.OK)
	out := r.Value.(subject.AccountStatus)
	core.AssertFalse(t, out.HasAccount)
}

// TestAccount_AccountStatus_Bad_AccountRootNotADirectory — the
// ~/Lethean/account path itself exists but is a plain file (corrupt
// install / manual tamper). Stat succeeds so the early not-found
// short-circuit doesn't fire, but the subsequent ReadDir must fail
// closed to HasAccount=false rather than propagate an error.
func TestAccount_AccountStatus_Bad_AccountRootNotADirectory(t *core.T) {
	home := homeFixture(t)
	svc := subject.NewService(nil)
	core.AssertTrue(t, core.MkdirAll(core.PathJoin(home, "Lethean"), 0o700).OK)
	core.AssertTrue(t, core.WriteFile(
		core.PathJoin(home, "Lethean", "account"), []byte("not a directory"), 0o600,
	).OK)

	r := svc.AccountStatus()

	core.AssertTrue(t, r.OK)
	out := r.Value.(subject.AccountStatus)
	core.AssertFalse(t, out.HasAccount)
}

// --- Create — #1574 audit emission ---

// TestCreate_EmitsAuditEvent_Good pins the Mantis #1574 (MED) /
// Cerberus #13 contract: every successful Create MUST emit a typed
// audit.EventAuthAccountCreated row through audit.Default(). Sibling of
// Provision's auth.account.provisioned emission — both flows feed the
// Operations panel from the same Recorder surface.
//
// The Meta shape MUST carry path_hash (SHA-256 hex of the canonical
// account directory) — NEVER the raw path (Cerberus #1465 closure-only
// scope discipline applies to filesystem layout too). The canonical
// account_id lands in Event.AccountID, not duplicated in Meta.
func TestCreate_EmitsAuditEvent_Good(t *core.T) {
	_ = homeFixture(t)

	rec := &recordingRecorder{}
	audit.SetDefault(rec)
	t.Cleanup(func() { audit.SetDefault(nil) })

	svc := subject.NewService(nil)
	in := validInput()
	in.RequestID = "test-req-1574"

	r := svc.Create(in)
	core.AssertTrue(t, r.OK, "Create must succeed for the audit-emit assertion")

	core.AssertEqual(t, 1, len(rec.events),
		"Create must emit exactly one audit event on success")
	ev := rec.events[0]
	core.AssertEqual(t, audit.EventAuthAccountCreated, ev.Event)
	core.AssertEqual(t, audit.OutcomeOK, ev.Outcome)
	core.AssertEqual(t, "account.create", ev.Scope)
	core.AssertEqual(t, in.AccountID, ev.AccountID)
	core.AssertEqual(t, "test-req-1574", ev.RequestID)

	// Meta MUST carry path_hash (SHA-256 hex string, 64 chars) and
	// MUST NOT carry the raw path bytes.
	pathHash, ok := ev.Meta["path_hash"].(string)
	core.AssertTrue(t, ok, "Meta.path_hash must be present as string")
	core.AssertLen(t, pathHash, 64, "path_hash must be SHA-256 hex (64 chars)")
	_, hasRawPath := ev.Meta["path"]
	core.AssertFalse(t, hasRawPath, "Meta MUST NOT carry raw path (Cerberus #1465 discipline)")
}

// TestCreate_NoAuditOnFailure_Bad pins the inverse contract: a Create
// that fails before reaching the success path (e.g. id_mismatch — the
// fail-fast validation gate) MUST NOT emit an audit row. Failure-mode
// audit events for create-attempts are a separate event-name reserved
// for a future ticket; today's emit-site only fires post-success.
func TestCreate_NoAuditOnFailure_Bad(t *core.T) {
	_ = homeFixture(t)

	rec := &recordingRecorder{}
	audit.SetDefault(rec)
	t.Cleanup(func() { audit.SetDefault(nil) })

	svc := subject.NewService(nil)
	in := validInput()
	in.AccountID = "0000000000000000" // forces id_mismatch

	r := svc.Create(in)
	core.AssertFalse(t, r.OK, "Create must fail with id_mismatch fixture")
	core.AssertEqual(t, "account.id_mismatch", r.Code())

	core.AssertEqual(t, 0, len(rec.events),
		"failed Create MUST NOT emit auth.account.created (only post-success)")
}

// --- Cutover tests (Mantis #1578) ---
//
// Pin the post-cutover contracts so a future regression that bypasses
// paths.AtomicWriteWithVersion (e.g. someone reintroduces a local
// atomicWrite shim) trips a named test.

// TestService_Cutover_Create_ModeVerify_Ugly — pins the Cerberus #1464
// mode-tamper defence post-cutover. After cutover, the defence lives
// in the primitive (paths.AtomicWriteWithVersion's at-rest mode-verify
// gate per Mantis #1592 / Cerberus #19 §5.1 Option C). A racing chmod
// between rename and the verify-Lstat MUST surface CodeWriteModeTamper
// (the primitive's typed code) propagated to the caller — NOT the
// legacy account.atomicWrite code, which is now deleted.
//
// Fixture: arm SetPostRenameModeTamperForTest on the privatePath the
// next write will land at, then invoke Create. The primitive's gate
// fires during the first AtomicWriteWithVersion call (public.key is
// also under "account/" prefix), so Create fails before private.key
// even attempts.
func TestService_Cutover_Create_ModeVerify_Ugly(t *core.T) {
	_ = homeFixture(t)
	svc := subject.NewService(nil)

	// Arm the primitive's post-rename mode-tamper hook to simulate a
	// racing chmod widening the perms between rename and the verify
	// Lstat. The hook fires on EVERY at-rest path write (the primitive
	// gates on IsAtRestEncryptedPath, which "account/" satisfies), so
	// the first write — public.key — trips it.
	paths.SetPostRenameModeTamperForTest(func(p string) {
		_ = os.Chmod(p, 0o644)
	})
	t.Cleanup(func() { paths.SetPostRenameModeTamperForTest(nil) })

	r := svc.Create(validInput())
	core.AssertFalse(t, r.OK, "mode tamper MUST be detected post-cutover")
	core.AssertContains(t, r.Error(), paths.CodeWriteModeTamper,
		"tampered mode MUST surface paths.write.mode_tamper (primitive code), not legacy account.atomicWrite")
}

// TestService_Cutover_Create_RefuseOverwrite_Bad — pins the
// refuse-to-overwrite invariant post-cutover. Pre-create the leaf
// private.key file and assert Create fails with account.exists,
// matching the pre-cutover behaviour. Cerberus #1460 (a) contract
// preserved verbatim.
//
// SECURITY-NOTE: per RFC §5.3, the stat-then-write window is NOT
// closed under a single lock in this cutover (WithFileLock is
// non-reentrant; primitive doesn't yet expose IfNotExist). The
// residual TOCTOU window matches pre-cutover behaviour — the primitive
// strictly narrows it. Follow-up Mantis filed.
func TestService_Cutover_Create_RefuseOverwrite_Bad(t *core.T) {
	home := homeFixture(t)
	svc := subject.NewService(nil)

	in := validInput()
	dir := core.PathJoin(home, "Lethean", "account", in.AccountID)
	core.AssertTrue(t, core.MkdirAll(dir, 0o700).OK)

	// Pre-create the leaf signal — Create MUST refuse.
	w := core.WriteFile(core.PathJoin(dir, "private.key"), []byte("preexisting-marker"), 0o600)
	core.AssertTrue(t, w.OK, "fixture private.key write must succeed")

	r := svc.Create(in)
	core.AssertFalse(t, r.OK, "Create MUST refuse to overwrite existing private.key")
	core.AssertEqual(t, "account.exists", r.Code(),
		"refuse-to-overwrite MUST surface account.exists (Cerberus #1460 (a))")
}

// TestAccount_Create_Bad_AccountDirBlockedByFile — a plain file
// already occupies the canonical account directory path (no
// private.key inside it, since it isn't a directory at all, so the
// refuse-to-overwrite leaf check doesn't fire) — MkdirAll for the
// account directory itself must fail closed rather than silently
// writing the private.key leaf into a directory it never actually
// created.
func TestAccount_Create_Bad_AccountDirBlockedByFile(t *core.T) {
	home := homeFixture(t)
	svc := subject.NewService(nil)
	in := validInput()
	blockedDir := core.PathJoin(home, "Lethean", "account", in.AccountID)
	core.AssertTrue(t, core.MkdirAll(
		core.PathJoin(home, "Lethean", "account"), 0o700,
	).OK)
	core.AssertTrue(t, core.WriteFile(blockedDir, []byte("in the way"), 0o600).OK)

	r := svc.Create(in)

	core.AssertFalse(t, r.OK,
		"MkdirAll over an existing file MUST fail, not silently proceed")
}

// --- Register / lifecycle (Wails3 + core.WithName contract) ---

func TestRegister_GoodReturnsUsableService(t *core.T) {
	r := subject.Register(nil)

	core.RequireTrue(t, r.OK)
	svc, ok := r.Value.(*subject.Service)
	core.RequireTrue(t, ok)
	core.AssertTrue(t, svc != nil)
}

func TestService_ServiceName_Good(t *core.T) {
	svc := subject.NewService(nil)

	core.AssertEqual(t, "Account", svc.ServiceName())
}

func TestService_ServiceStartup_GoodIsNoOp(t *core.T) {
	svc := subject.NewService(nil)

	r := svc.ServiceStartup(core.Background(), nil)

	core.AssertTrue(t, r.OK)
}

func TestService_ServiceShutdown_GoodClearsUnlockedState(t *core.T) {
	_ = homeFixture(t)
	svc := subject.NewService(nil)
	subject.SeedUnlockedForTest(svc, fixtureAccountID, []byte("fixture-priv-bytes"))
	core.AssertTrue(t, svc.HasUnlocked(fixtureAccountID))

	r := svc.ServiceShutdown()

	core.AssertTrue(t, r.OK)
	core.AssertFalse(t, svc.HasUnlocked(fixtureAccountID))

	// Idempotent — a second shutdown must not panic on nil maps.
	second := svc.ServiceShutdown()
	core.AssertTrue(t, second.OK)
}
