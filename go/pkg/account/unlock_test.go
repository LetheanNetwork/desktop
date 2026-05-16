// SPDX-Licence-Identifier: EUPL-1.2

// Tests for the account.Service Unlock + Lock contract per Stage E.B
// RFC.stage-e.md v2 §5 + §10. All seven of the RFC's mandatory test
// names that fall in pkg/account are pinned here; bootstrap-auth
// route-tier tests live in pkg/server.

package account_test

import (
	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/account"
	"dappco.re/lthn/desktop/pkg/serverkey"
	"forge.lthn.ai/Snider/Enchantrix/pkg/crypt/std/pgp"
)

// fixtureAccountID is the canonical id used by every unlock fixture
// — short enough to type, long enough to look real.
const fixtureAccountID = "abc123def4567890"

// fixturePassphrase is what real-world Lethean accounts would
// substitute for; test-only.
const fixturePassphrase = "fixture-passphrase-correct"

// fixtureWrongPassphrase exercises the bad-passphrase branch
// without colliding with the correct one even at the first byte.
const fixtureWrongPassphrase = "wrong-passphrase-here"

// writeEncryptedAccount lays down ~/Lethean/account/<id>/private.key
// containing a real symmetric-PGP-encrypted blob the unlock path
// can decrypt. Mirrors what the forthcoming PUT /seal endpoint
// (RFC §7) will do once it lands.
func writeEncryptedAccount(t *core.T, home, accountID, passphrase string) {
	t.Helper()
	dir := core.PathJoin(home, "Lethean", "account", accountID)
	core.AssertTrue(t, core.MkdirAll(dir, 0o700).OK)

	// Generate a real PGP key pair — same primitive Bootstrap uses
	// for server.key. Plaintext-private is what unlock decrypts back
	// to; the ciphertext is what lives on disk.
	pgpSvc := pgp.NewService()
	_, privPlain, err := pgpSvc.GenerateKeyPair("lthn-test", "test@lthn.local", "fixture")
	core.AssertTrue(t, err == nil, "PGP keygen must succeed")

	ct, err := pgpSvc.SymmetricallyEncrypt([]byte(passphrase), privPlain)
	core.AssertTrue(t, err == nil, "symmetric encrypt must succeed")

	priv := core.PathJoin(dir, "private.key")
	core.AssertTrue(t, core.WriteFile(priv, ct, 0o600).OK)
}

// writeRawAccount lays down a private.key file with arbitrary bytes
// — used by the corrupted-key sub-cases to land structurally invalid
// content the parser will reject.
func writeRawAccount(t *core.T, home, accountID string, content []byte) {
	t.Helper()
	dir := core.PathJoin(home, "Lethean", "account", accountID)
	core.AssertTrue(t, core.MkdirAll(dir, 0o700).OK)
	priv := core.PathJoin(dir, "private.key")
	core.AssertTrue(t, core.WriteFile(priv, content, 0o600).OK)
}

// newUnlockable builds a Service with the fake session-token issuer
// wired so Unlock can complete the round-trip without a real
// serverkey.Service Bootstrap underneath.
func newUnlockable(t *core.T, home string) *subject.Service {
	t.Helper()
	svc := subject.NewService(nil)
	svc.SetServerKey(&fakeSessionTokenIssuerAdapter{})
	_ = home
	return svc
}

// fakeSessionTokenIssuerAdapter satisfies subject.SessionTokenIssuer
// by returning the canonical serverkey.SessionTokenOutput shape so
// the Result.Value type assertion inside Unlock matches what real
// serverkey.Service.IssueSessionToken returns. Token content is
// stub-shaped; the unlock test path doesn't verify the signature
// (that lives in pkg/serverkey/serverkey_test.go).
type fakeSessionTokenIssuerAdapter struct{}

func (a *fakeSessionTokenIssuerAdapter) IssueSessionToken(accountID string) core.Result {
	return core.Ok(serverkey.SessionTokenOutput{
		Token:     "LTHN-SESS-1." + accountID + ".fake",
		ExpiresAt: 9999999999,
		AccountID: accountID,
	})
}

// --- Unlock — Good ---

func TestAccount_Unlock_Good(t *core.T) {
	home := homeFixture(t)
	writeEncryptedAccount(t, home, fixtureAccountID, fixturePassphrase)
	svc := newUnlockable(t, home)

	r := svc.Unlock(subject.UnlockInput{
		AccountID:  fixtureAccountID,
		Passphrase: fixturePassphrase,
	})
	core.AssertTrue(t, r.OK, "valid passphrase against well-formed account must unlock")

	// The unlocked private key now lives in memory; HasUnlocked
	// reflects this without re-decrypting.
	core.AssertTrue(t, svc.HasUnlocked(fixtureAccountID))
}

// --- Unlock — Bad: missing input ---

func TestAccount_Unlock_AccountIDRequired_Bad(t *core.T) {
	_ = homeFixture(t)
	svc := newUnlockable(t, "")

	r := svc.Unlock(subject.UnlockInput{AccountID: "", Passphrase: "x"})
	core.AssertFalse(t, r.OK)
	core.AssertEqual(t, "account.id.required", r.Code())
}

func TestAccount_Unlock_PassphraseRequired_Bad(t *core.T) {
	_ = homeFixture(t)
	svc := newUnlockable(t, "")

	r := svc.Unlock(subject.UnlockInput{AccountID: fixtureAccountID, Passphrase: ""})
	core.AssertFalse(t, r.OK)
	core.AssertEqual(t, "account.unlock.passphrase.required", r.Code())
}

// --- Unlock — Bad: wrong passphrase ---

func TestAccount_Unlock_BadPassphrase_Bad(t *core.T) {
	home := homeFixture(t)
	writeEncryptedAccount(t, home, fixtureAccountID, fixturePassphrase)
	svc := newUnlockable(t, home)

	r := svc.Unlock(subject.UnlockInput{
		AccountID:  fixtureAccountID,
		Passphrase: fixtureWrongPassphrase,
	})
	core.AssertFalse(t, r.OK, "wrong passphrase MUST fail")
	core.AssertEqual(t, "account.unlock.bad_passphrase", r.Code())

	// HasUnlocked must remain false on failure — no half-unlocked state.
	core.AssertFalse(t, svc.HasUnlocked(fixtureAccountID))
}

// --- Unlock — Ugly: account_id not found collapses to bad_passphrase ---

// TestUnlock_AccountNotFoundCollapsesToBadPassphrase_Ugly pins the
// RFC §5 ¹ privacy ruling — a missing account_id returns the SAME
// code + same shape as a wrong passphrase so the endpoint doesn't
// leak which account_ids exist on disk to a probe attacker.
func TestUnlock_AccountNotFoundCollapsesToBadPassphrase_Ugly(t *core.T) {
	home := homeFixture(t)
	writeEncryptedAccount(t, home, fixtureAccountID, fixturePassphrase)
	svc := newUnlockable(t, home)

	// Probe a NON-existent account_id. Must come back with the same
	// code as a wrong-passphrase result against the real account_id.
	missingR := svc.Unlock(subject.UnlockInput{
		AccountID:  "0000000000000000",
		Passphrase: "anything",
	})
	wrongR := svc.Unlock(subject.UnlockInput{
		AccountID:  fixtureAccountID,
		Passphrase: fixtureWrongPassphrase,
	})

	core.AssertFalse(t, missingR.OK)
	core.AssertFalse(t, wrongR.OK)
	core.AssertEqual(t, wrongR.Code(), missingR.Code(),
		"missing-account_id MUST surface the same code as wrong-passphrase (RFC §5 ¹)")
	core.AssertEqual(t, "account.unlock.bad_passphrase", missingR.Code())
}

// --- Unlock — Ugly: corrupted-key distinguishing by error type ---

// TestUnlock_CorruptedKeyBytewiseDistinguishing_Ugly pins the RFC
// §10 H4 ruling — four sub-cases distinguished by the decrypt error
// TYPE (not error string).
//
//   - intact ciphertext + correct passphrase → unlock OK
//   - intact ciphertext + WRONG passphrase   → bad_passphrase
//   - garbage-after-parse                    → corrupted_key
//   - truncated-armour                       → corrupted_key
//
// "Distinguished by type" means the test asserts on Result.Code()
// directly — no parsing of human-readable error text. The
// implementation routes via decryptFailureKind, populated by
// observing whether the openpgp prompt closure was invoked.
func TestUnlock_CorruptedKeyBytewiseDistinguishing_Ugly(t *core.T) {
	home := homeFixture(t)

	// Sub-case 1 — intact + correct → OK.
	idIntact := "1111111111111111"
	writeEncryptedAccount(t, home, idIntact, fixturePassphrase)
	svc := newUnlockable(t, home)
	r1 := svc.Unlock(subject.UnlockInput{AccountID: idIntact, Passphrase: fixturePassphrase})
	core.AssertTrue(t, r1.OK, "sub-case 1 — intact + correct → unlock OK")

	// Sub-case 2 — intact + WRONG → bad_passphrase.
	idWrong := "2222222222222222"
	writeEncryptedAccount(t, home, idWrong, fixturePassphrase)
	r2 := svc.Unlock(subject.UnlockInput{AccountID: idWrong, Passphrase: fixtureWrongPassphrase})
	core.AssertFalse(t, r2.OK)
	core.AssertEqual(t, "account.unlock.bad_passphrase", r2.Code(),
		"sub-case 2 — intact + wrong → bad_passphrase")

	// Sub-case 3 — garbage bytes that aren't a PGP packet stream at
	// all → corrupted_key. The parser fails before prompt is invoked.
	idGarbage := "3333333333333333"
	writeRawAccount(t, home, idGarbage, []byte("this is not pgp at all, just text"))
	r3 := svc.Unlock(subject.UnlockInput{AccountID: idGarbage, Passphrase: fixturePassphrase})
	core.AssertFalse(t, r3.OK)
	core.AssertEqual(t, "account.unlock.corrupted_key", r3.Code(),
		"sub-case 3 — garbage bytes → corrupted_key (prompt never invoked)")

	// Sub-case 4 — truncated armour: write the first N bytes of a
	// real encrypted blob, omitting the tail. Parser reads partial
	// packet headers then errors before reaching prompt.
	idTrunc := "4444444444444444"
	pgpSvc := pgp.NewService()
	_, privPlain, err := pgpSvc.GenerateKeyPair("lthn-test", "test@lthn.local", "fixture")
	core.AssertTrue(t, err == nil)
	full, err := pgpSvc.SymmetricallyEncrypt([]byte(fixturePassphrase), privPlain)
	core.AssertTrue(t, err == nil)
	core.AssertTrue(t, len(full) > 16, "fixture encrypted blob must be non-trivial")
	// Take just the first few bytes — enough that the parser sees a
	// packet header but errors on missing body.
	truncated := full[:8]
	writeRawAccount(t, home, idTrunc, truncated)
	r4 := svc.Unlock(subject.UnlockInput{AccountID: idTrunc, Passphrase: fixturePassphrase})
	core.AssertFalse(t, r4.OK)
	core.AssertEqual(t, "account.unlock.corrupted_key", r4.Code(),
		"sub-case 4 — truncated armour → corrupted_key (prompt never invoked)")
}

// --- Unlock — Ugly: per-account lockout (M1) ---

// TestUnlock_LockoutCounterScopedPerAccount_Ugly pins the Cerberus
// DREAD M1 ruling — counter is per-account_id, NOT global. A
// typo-spamming user MUST NOT lock out a sibling account on the
// same machine.
//
// Interleaving: account A receives lockoutThreshold (5) failed
// attempts, then account B receives one CORRECT attempt. Account B
// MUST succeed (its counter is independent of A's). Account A MUST
// be locked out — same window, same machine, distinct counter.
func TestUnlock_LockoutCounterScopedPerAccount_Ugly(t *core.T) {
	home := homeFixture(t)
	idA := "aaaaaaaaaaaaaaaa"
	idB := "bbbbbbbbbbbbbbbb"
	writeEncryptedAccount(t, home, idA, "passphrase-A")
	writeEncryptedAccount(t, home, idB, "passphrase-B")
	svc := newUnlockable(t, home)

	// Spam account A with wrong-passphrase attempts up to the
	// threshold. Each ticks A's counter; B's counter is unaffected.
	for i := 0; i < 5; i++ {
		r := svc.Unlock(subject.UnlockInput{
			AccountID:  idA,
			Passphrase: "definitely-wrong",
		})
		core.AssertFalse(t, r.OK, "spam attempt against A must fail")
	}

	// Account A is now locked — even a CORRECT passphrase rejects.
	rA := svc.Unlock(subject.UnlockInput{AccountID: idA, Passphrase: "passphrase-A"})
	core.AssertFalse(t, rA.OK, "A must be locked after 5 failed attempts")
	core.AssertEqual(t, "account.unlock.locked_out", rA.Code())

	// Account B remains UNAFFECTED — correct passphrase unlocks
	// cleanly because B's counter never ticked.
	rB := svc.Unlock(subject.UnlockInput{AccountID: idB, Passphrase: "passphrase-B"})
	core.AssertTrue(t, rB.OK, "B must unlock — A's lockout MUST NOT bleed across to B")
	core.AssertTrue(t, svc.HasUnlocked(idB))
	core.AssertFalse(t, svc.HasUnlocked(idA), "A stays sealed under lockout")
}

// --- Lock ---

func TestAccount_Lock_Good(t *core.T) {
	home := homeFixture(t)
	writeEncryptedAccount(t, home, fixtureAccountID, fixturePassphrase)
	svc := newUnlockable(t, home)

	// Unlock first so we have state to clear.
	core.AssertTrue(t, svc.Unlock(subject.UnlockInput{
		AccountID: fixtureAccountID, Passphrase: fixturePassphrase,
	}).OK)
	core.AssertTrue(t, svc.HasUnlocked(fixtureAccountID))

	r := svc.Lock(subject.LockInput{AccountID: fixtureAccountID})
	core.AssertTrue(t, r.OK, "Lock on an unlocked account must succeed")
	core.AssertFalse(t, svc.HasUnlocked(fixtureAccountID),
		"Lock MUST clear the in-memory unlocked private key")
}

func TestAccount_Lock_Idempotent_Good(t *core.T) {
	// Locking an account that was never unlocked MUST succeed —
	// callers shouldn't need to track unlock state to know whether
	// to Lock.
	_ = homeFixture(t)
	svc := newUnlockable(t, "")

	r := svc.Lock(subject.LockInput{AccountID: "never-unlocked"})
	core.AssertTrue(t, r.OK, "Lock on a never-unlocked id must be idempotent")
}

func TestAccount_Lock_AccountIDRequired_Bad(t *core.T) {
	_ = homeFixture(t)
	svc := newUnlockable(t, "")

	r := svc.Lock(subject.LockInput{AccountID: ""})
	core.AssertFalse(t, r.OK)
	core.AssertEqual(t, "account.id.required", r.Code())
}

// --- HasUnlocked ---

func TestAccount_HasUnlocked_Good(t *core.T) {
	_ = homeFixture(t)
	svc := newUnlockable(t, "")

	// Empty account_id always returns false — no leak about live state.
	core.AssertFalse(t, svc.HasUnlocked(""))

	// Unknown id returns false.
	core.AssertFalse(t, svc.HasUnlocked("never-seen"))
}

// --- Unlock — server-misconfigured (no serverkey wired) ---

func TestAccount_Unlock_ServerMisconfigured_Bad(t *core.T) {
	// Service constructed WITHOUT SetServerKey — Unlock must fail
	// loud rather than silently returning a token with no real
	// signature behind it.
	home := homeFixture(t)
	writeEncryptedAccount(t, home, fixtureAccountID, fixturePassphrase)
	svc := subject.NewService(nil) // no SetServerKey

	r := svc.Unlock(subject.UnlockInput{
		AccountID: fixtureAccountID, Passphrase: fixturePassphrase,
	})
	core.AssertFalse(t, r.OK)
	core.AssertEqual(t, "account.unlock.server_misconfigured", r.Code())
}
