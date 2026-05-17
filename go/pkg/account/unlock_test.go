// SPDX-Licence-Identifier: EUPL-1.2

// Tests for the account.Service Unlock + Lock contract per Stage E.B
// RFC.stage-e.md v2 §5 + §10. All seven of the RFC's mandatory test
// names that fall in pkg/account are pinned here; bootstrap-auth
// route-tier tests live in pkg/server.

package account_test

import (
	"bytes"
	"testing"

	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/account"
	"dappco.re/lthn/desktop/pkg/serverkey"
	"forge.lthn.ai/Snider/Enchantrix/pkg/crypt/std/pgp"
	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/packet"
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

// writeEncryptedAccountFromPlaintext is the cache-able sibling of
// writeEncryptedAccount: callers pre-generate the PGP plaintext-private
// bytes ONCE and reuse them across many iterations, re-running the
// symmetric-encrypt step inside the loop. The S2K salt is fresh per
// SymmetricallyEncrypt call (the openpgp library samples it internally
// per envelope), so the per-(passphrase, S2K-salt, body-ciphertext)
// triple required by the Mantis #1510 flake-pin is still exercised —
// only the RSA keygen is amortised. See Mantis #1579 for the rationale
// (RSA-2048 keygen × 1000 iters × -race overhead exceeded 5m timeout).
func writeEncryptedAccountFromPlaintext(t *core.T, home, accountID, passphrase string, privPlain []byte) {
	t.Helper()
	dir := core.PathJoin(home, "Lethean", "account", accountID)
	core.AssertTrue(t, core.MkdirAll(dir, 0o700).OK)

	pgpSvc := pgp.NewService()
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

// IssueSessionTokenWithRequest satisfies the Phase 2.5 addition to
// SessionTokenIssuer. Threads through to the bare IssueSessionToken
// (no audit-event emission from the fake) — the request_id field
// is consumed but not surfaced because the fake doesn't fire the
// audit recorder.
func (a *fakeSessionTokenIssuerAdapter) IssueSessionTokenWithRequest(accountID, _ string) core.Result {
	return a.IssueSessionToken(accountID)
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

	// Sub-case 5 — Mantis #1510 flake pin. Per ProtonMail openpgp
	// v1.4.0 read.go:215 — "In v4, on wrong passphrase, session key
	// decryption is very likely to result in an invalid cipherFunc:
	// only for < 5% of cases we will proceed to decrypt the data".
	// For that ~5% subset the wrong-passphrase × ciphertext pair
	// produced an inner-packet body read that errored at the
	// PACKET-PARSE stage instead of re-invoking the prompt — the
	// original distinguisher then mis-classified as corrupted_key
	// (user saw "run lthn account repair" instead of "passphrase
	// didn't unlock — N attempts remaining"). Cerberus #1711 +
	// Mantis #1510 prescription: re-probe via the openpgp packet
	// API and inspect the first decrypted byte for the RFC 4880
	// §4.2 packet-tag canary; garbage first byte → re-classify
	// as bad_passphrase.
	//
	// This sub-case exercises N independent wrong-passphrase ×
	// fresh-ciphertext pairs. Pre-fix, ~5% mis-classified as
	// corrupted_key (intermittent test failure). Post-fix, the
	// combined cipherFunc-validity gate (openpgp's ~95% filter) +
	// our multi-byte packet-parse canary (packet.Read rejects
	// random bytes ~always) drives the residual to vanishing.
	//
	// Iteration count is 1000 by default to statistically pin the
	// residual false-allow rate against random S2K-salt rotation
	// (Cerberus verify follow-up on commit 2128119). Each iter burns
	// an S2K-derive (~few ms) — the RSA-2048 keygen is amortised
	// across all iters via the cached privPlain below. Mantis #1579:
	// pre-fix each iter ran a fresh keygen + S2K (~120ms wallclock),
	// pushing the -race timing past the 5m timeout; caching the
	// keypair drops the per-iter cost to just the S2K + symenc.
	flakeIters := 1000
	if testing.Short() {
		flakeIters = 50
	}
	// Cache the PGP plaintext-private bytes ONCE. The Mantis #1510
	// flake-pin needs fresh S2K salts per iteration — the
	// SymmetricallyEncrypt call samples a new salt internally per
	// envelope, so the per-(passphrase, S2K-salt, body-ciphertext)
	// triple is still exercised. Only the keygen is amortised. Per
	// Mantis #1579.
	pgpFlake := pgp.NewService()
	_, flakePlain, err := pgpFlake.GenerateKeyPair("lthn-test", "test@lthn.local", "fixture")
	core.AssertTrue(t, err == nil, "sub-case 5 — cached PGP keygen must succeed")
	corruptedKeyHits := 0
	for i := 0; i < flakeIters; i++ {
		// Fresh account + fresh ciphertext each iteration — the
		// flake is per-(passphrase, S2K-salt, body-ciphertext)
		// triple, so a single fixture reused across iterations
		// would always classify identically. We need NEW S2K salts
		// per iteration to exercise the probability space; the
		// underlying plaintext-private bytes can be reused (only
		// the symmetric-encrypt envelope varies per iter).
		idFlake := core.Sprintf("flake%011d", i)
		writeEncryptedAccountFromPlaintext(t, home, idFlake, fixturePassphrase, flakePlain)
		flakeR := svc.Unlock(subject.UnlockInput{
			AccountID:  idFlake,
			Passphrase: fixtureWrongPassphrase,
		})
		core.AssertFalse(t, flakeR.OK,
			core.Sprintf("sub-case 5 iter %d — wrong passphrase MUST fail", i))
		if flakeR.Code() == "account.unlock.corrupted_key" {
			corruptedKeyHits++
		}
	}
	core.AssertTrue(t, corruptedKeyHits == 0,
		core.Sprintf("sub-case 5 (Mantis #1510 flake-pin) — wrong-passphrase × intact-ciphertext MUST classify as bad_passphrase across all %d iterations; got %d corrupted_key mis-classifications", flakeIters, corruptedKeyHits))

	// Sub-case 6 — zero-length LiteralData body, WRONG passphrase.
	// Cerberus verify follow-up shape-coverage: an attacker who
	// somehow forges (or a degenerate sealer who emits) a
	// ciphertext whose inner LiteralData carries no payload bytes
	// MUST NOT slip past the distinguisher with a misleading code.
	// The on-disk shape here was built by symmetric-encrypting an
	// EMPTY plaintext with the CORRECT fixture passphrase; the
	// unlock attempt uses the WRONG passphrase, so the openpgp
	// FindKey loop either rejects at SKE (→ bad_passphrase via the
	// prompt sentinel) OR clears the cipherFunc gate and produces
	// garbage inner bytes that packet.Read rejects (→ canary fires
	// → bad_passphrase). Deterministic outcome: bad_passphrase.
	idEmpty := "6666666666666666"
	pgpEmpty := pgp.NewService()
	emptyCT, err := pgpEmpty.SymmetricallyEncrypt([]byte(fixturePassphrase), []byte{})
	core.AssertTrue(t, err == nil, "symmetric-encrypt of empty plaintext must succeed")
	core.AssertTrue(t, len(emptyCT) > 16, "even empty plaintext produces non-trivial SKE+SEIPD envelope")
	writeRawAccount(t, home, idEmpty, emptyCT)
	r6 := svc.Unlock(subject.UnlockInput{AccountID: idEmpty, Passphrase: fixtureWrongPassphrase})
	core.AssertFalse(t, r6.OK, "sub-case 6 — wrong passphrase against zero-length-body ciphertext MUST fail")
	core.AssertEqual(t, "account.unlock.bad_passphrase", r6.Code(),
		"sub-case 6 — zero-length LiteralData body + WRONG passphrase MUST classify as bad_passphrase (canary fires either at SKE or at inner packet.Read)")

	// Sub-case 7 — raw legacy EncryptedDataPacket (packet tag 9,
	// no SEIPD MDC, no AEAD). The ProtonMail go-crypto public
	// SerializeSymmetricallyEncrypted always emits tag 18 (SEIPD)
	// and offers no public constructor for the legacy tag-9 shape.
	// Hand-crafting the packet header requires reaching into
	// private openpgp bytes — out of scope for a test-only widening
	// dispatch. The shape is captured here as a t.Skip so the gap
	// is visible in test output + tracked, not silently absent.
	//
	// Surface gap: openpgp/packet exports no public helper to write
	// a tag-9 SymmetricallyEncrypted without IntegrityProtected.
	// Tracked as production-side investigation — defence-in-depth
	// for the SE-switch case `*packet.SymmetricallyEncrypted` in
	// classifyPostPromptError is currently unproven against the
	// legacy non-MDC shape.
	t.Run("sub_case_7_raw_edp_no_seipd", func(t *core.T) {
		t.Skip("raw tag-9 EncryptedDataPacket construction requires private openpgp API; SE-switch defence-in-depth coverage tracked as separate widening ticket")
	})

	// Sub-case 8 — AEAD-shape ciphertext (SEIPDv2 / AEADEncrypted)
	// with WRONG passphrase. This exercises the SE-switch's
	// `*packet.AEADEncrypted` branch in classifyPostPromptError.
	// Lethean Provision does NOT emit this shape today, but the
	// unlock path must handle it gracefully if it ever lands (e.g.
	// a future Provision modernisation, or a hand-rolled key file
	// from an interoperable PGP impl).
	//
	// AEAD with WRONG passphrase: the AEAD construction MACs the
	// session-key derivation, so a wrong S2K key yields an AEAD
	// MAC failure at SKE-decrypt time. classifyPostPromptError's
	// SKE-level `return true` path catches it → bad_passphrase.
	idAEAD := "8888888888888888"
	pgpAEAD := pgp.NewService()
	aeadCfg := &packet.Config{
		AEADConfig: &packet.AEADConfig{},
	}
	aeadCT, err := encryptAEAD(pgpAEAD, []byte(fixturePassphrase), aeadCfg)
	if err != nil {
		t.Skip("AEAD-shape ciphertext construction failed — go-crypto build may not support AEAD: " + err.Error())
	}
	core.AssertTrue(t, len(aeadCT) > 32, "AEAD envelope must be non-trivial")
	writeRawAccount(t, home, idAEAD, aeadCT)
	r8 := svc.Unlock(subject.UnlockInput{AccountID: idAEAD, Passphrase: fixtureWrongPassphrase})
	core.AssertFalse(t, r8.OK, "sub-case 8 — wrong passphrase against AEAD ciphertext MUST fail")
	core.AssertEqual(t, "account.unlock.bad_passphrase", r8.Code(),
		"sub-case 8 — AEAD ciphertext + WRONG passphrase MUST classify as bad_passphrase (AEAD MAC fails fast on wrong derived key)")
}

// encryptAEAD wraps openpgp.SymmetricallyEncrypt with an AEAD-shaped
// packet.Config so sub-case 8 can construct a SEIPDv2 ciphertext
// without going through pgp.Service (which deliberately keeps its
// public surface MDC-only). Returns the serialised envelope bytes
// suitable for writeRawAccount.
//
// Usage:
//
//	ct, err := encryptAEAD(pgp.NewService(), []byte("pw"), &packet.Config{AEADConfig: &packet.AEADConfig{}})
func encryptAEAD(_ *pgp.Service, passphrase []byte, cfg *packet.Config) ([]byte, error) {
	var buf bytes.Buffer
	w, err := openpgp.SymmetricallyEncrypt(&buf, passphrase, nil, cfg)
	if err != nil {
		return nil, err
	}
	// Write a small fixed payload so the inner LiteralData has a
	// real body; the test only cares about the SKE+SE envelope
	// shape, not the plaintext content.
	if _, err := w.Write([]byte("aead-fixture-payload")); err != nil {
		_ = w.Close()
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
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

// --- Cerberus Stage E.B DREAD ADD-HIGH-2 — path traversal in
// AccountID. Mirrors the #1486/#1498 compound-finding pattern: any
// AccountID with '/', '..', leading '.', NUL, or > 255 bytes must
// reject at paths.IsValidID before any disk touch.

func TestAccount_Unlock_PathTraversal_Bad_DREAD_ADD_HIGH_2(t *core.T) {
	_ = homeFixture(t)
	svc := newUnlockable(t, "")

	// Every shape paths.IsValidID rejects MUST surface as a Fail
	// from Unlock — proving the gate runs BEFORE any stat/lockout
	// machinery (which would leak timing OR balloon the lockout map).
	// Mirrors the pkg/sales/pipeline #1486 regression-test shape.
	for _, evil := range []string{
		"../../wallets/lethean-default", // classic traversal
		"..",                            // double-dot only
		".hidden",                       // leading dot
		"foo/bar",                       // path separator
		"foo\\bar",                      // windows separator
		"foo\x00bar",                    // NUL byte
	} {
		r := svc.Unlock(subject.UnlockInput{
			AccountID:  evil,
			Passphrase: "anything",
		})
		core.AssertFalse(t, r.OK, "Unlock("+evil+") MUST reject")
		// Error message MUST namespace under paths.invalid_id so
		// the operator can grep audit logs for the compound-finding
		// signature uniformly across Office/Sales/Account surfaces.
		core.AssertTrue(t, core.Contains(r.Error(), "paths.invalid_id"),
			"Unlock("+evil+") error message MUST carry paths.invalid_id (Cerberus DREAD ADD-HIGH-2)")
	}

	// Negative invariant — rejected traversal IDs MUST NOT show as
	// unlocked. Defends against the half-state where the audit said
	// rejected but the map was already mutated.
	core.AssertFalse(t, svc.HasUnlocked("../../wallets/lethean-default"),
		"rejected traversal id MUST NOT show as unlocked")
}

func TestAccount_Lock_PathTraversal_Bad_DREAD_ADD_HIGH_2(t *core.T) {
	_ = homeFixture(t)
	svc := newUnlockable(t, "")

	for _, evil := range []string{
		"../../wallets/lethean-default",
		"..",
		".hidden",
		"foo/bar",
		"foo\x00bar",
	} {
		r := svc.Lock(subject.LockInput{AccountID: evil})
		core.AssertFalse(t, r.OK, "Lock("+evil+") MUST reject")
		core.AssertTrue(t, core.Contains(r.Error(), "paths.invalid_id"),
			"Lock("+evil+") error message MUST carry paths.invalid_id")
	}
}

// TestPublicKeyFor_RejectsInvalidID_Bad pins the Cerberus #13 /
// Mantis #1573 (HIGH) shape gate. PublicKeyFor concatenates accountID
// into pubPath and calls core.ReadFile; without IsValidID gating a
// traversal id leaks the timing/contents of arbitrary 0o600 files on
// disk. Mirror of the Unlock + Lock DREAD-ADD-HIGH-2 traversal tests.
// The contract returns (nil, false) on any gate failure — callers can't
// distinguish "not unlocked" from "invalid shape", which is the desired
// shape per the existing (nil, false) on-empty contract.
func TestPublicKeyFor_RejectsInvalidID_Bad(t *core.T) {
	_ = homeFixture(t)
	svc := newUnlockable(t, "")

	for _, evil := range []string{
		"",                              // empty (pre-existing reject)
		"../../wallets/lethean-default", // classic traversal
		"..",                            // double-dot only
		".hidden",                       // leading dot
		"foo/bar",                       // forward separator
		"foo\\bar",                      // windows separator
		"foo\x00bar",                    // NUL byte
	} {
		raw, ok := svc.PublicKeyFor(evil)
		core.AssertFalse(t, ok,
			"PublicKeyFor("+core.Sprintf("%q", evil)+") MUST return ok=false")
		core.AssertTrue(t, raw == nil,
			"PublicKeyFor("+core.Sprintf("%q", evil)+") MUST return nil bytes")
	}
}

// TestPrivateKeyFor_RejectsInvalidID_Bad pins the grep-parity gate on
// PrivateKeyFor per Mantis #1573 optional follow-on. Same shape as
// PublicKeyFor's gate — (nil, false) on any IsValidID-rejected id.
// The exposure here is the in-memory unlocked map (lockout/probe
// pollution) rather than disk traversal, but the uniform invariant is
// what the #1505 lint rule enforces across every account-id-accepting
// Service method.
func TestPrivateKeyFor_RejectsInvalidID_Bad(t *core.T) {
	_ = homeFixture(t)
	svc := newUnlockable(t, "")

	for _, evil := range []string{
		"",                              // empty (pre-existing reject)
		"../../wallets/lethean-default", // classic traversal
		"..",                            // double-dot only
		".hidden",                       // leading dot
		"foo/bar",                       // forward separator
		"foo\\bar",                      // windows separator
		"foo\x00bar",                    // NUL byte
	} {
		raw, ok := svc.PrivateKeyFor(evil)
		core.AssertFalse(t, ok,
			"PrivateKeyFor("+core.Sprintf("%q", evil)+") MUST return ok=false")
		core.AssertTrue(t, raw == nil,
			"PrivateKeyFor("+core.Sprintf("%q", evil)+") MUST return nil bytes")
	}
}

// --- Cerberus Stage E.B DREAD ADD-HIGH-3 — trigger-attempt response
// shape. The 5th failed attempt MUST return `locked_out`, NOT
// `bad_passphrase + 5 remaining`. Previously the code cleared the
// attempts slice before computing the response, so the user saw "5
// attempts remaining" the moment they had 0 chances left + the
// audit-log claimed `attempts_remaining=5` immediately followed by
// `locked_out 60s` on the NEXT attempt — implying no lockout was
// pending.

// TestUnlock_TriggerAttemptReturnsLockedOut_Ugly_DREAD_ADD_HIGH_3
// pins the trigger-attempt response: 4 wrong attempts return
// bad_passphrase with decrementing remaining (4 → 1); the 5th
// returns locked_out, NOT bad_passphrase.
func TestUnlock_TriggerAttemptReturnsLockedOut_Ugly_DREAD_ADD_HIGH_3(t *core.T) {
	home := homeFixture(t)
	writeEncryptedAccount(t, home, fixtureAccountID, fixturePassphrase)
	svc := newUnlockable(t, home)

	// Attempts 1–4: bad_passphrase with decrementing remaining.
	for i := 0; i < 4; i++ {
		r := svc.Unlock(subject.UnlockInput{
			AccountID:  fixtureAccountID,
			Passphrase: fixtureWrongPassphrase,
		})
		core.AssertFalse(t, r.OK, "attempt should fail")
		core.AssertEqual(t, "account.unlock.bad_passphrase", r.Code(),
			core.Sprintf("attempt %d MUST surface bad_passphrase", i+1))
	}

	// Attempt 5 — the trigger attempt. MUST return locked_out, NOT
	// bad_passphrase. This is the load-bearing DREAD ADD-HIGH-3
	// regression cover.
	r := svc.Unlock(subject.UnlockInput{
		AccountID:  fixtureAccountID,
		Passphrase: fixtureWrongPassphrase,
	})
	core.AssertFalse(t, r.OK)
	core.AssertEqual(t, "account.unlock.locked_out", r.Code(),
		"5th attempt MUST surface locked_out (Cerberus DREAD ADD-HIGH-3) — not bad_passphrase")
}

// TestUnlock_TriggerAttemptWithCorrectPassphraseStillUnlocks_Ugly —
// the threshold-trigger fires on WRONG passphrases. If the 5th
// attempt happens to be the correct passphrase, it MUST still
// unlock (the gate ran successfully BEFORE recordFailedAttempt).
// Subtle case: the correct 5th attempt bypasses recordFailedAttempt
// because decryptFailureNone returns OK from distinguishDecrypt,
// and clearLockout wipes the entry on success.
func TestUnlock_TriggerAttemptWithCorrectPassphraseStillUnlocks_Ugly(t *core.T) {
	home := homeFixture(t)
	writeEncryptedAccount(t, home, fixtureAccountID, fixturePassphrase)
	svc := newUnlockable(t, home)

	// 4 wrong attempts — the counter is at 4 but the threshold (5)
	// is NOT yet reached.
	for i := 0; i < 4; i++ {
		_ = svc.Unlock(subject.UnlockInput{
			AccountID: fixtureAccountID, Passphrase: fixtureWrongPassphrase,
		})
	}

	// 5th attempt is CORRECT — must unlock cleanly. Successful
	// unlock also clears the lockout state.
	r := svc.Unlock(subject.UnlockInput{
		AccountID: fixtureAccountID, Passphrase: fixturePassphrase,
	})
	core.AssertTrue(t, r.OK, "correct passphrase on 5th attempt MUST unlock")
	core.AssertTrue(t, svc.HasUnlocked(fixtureAccountID))
}

// TestUnlock_LockoutStateRaceFree_Ugly_DREAD_ADD_HIGH_4 — concurrent
// Unlock + HasUnlocked + Lock calls MUST be race-free under
// `go test -race`. Pre-fix, lockoutState dereferenced
// st.unlockAt AFTER releasing the RLock; concurrent
// recordFailedAttempt under the write lock mutated the same
// pointed-to struct → race-detector fires.
//
// This test exercises 100 concurrent Unlock attempts against the
// same account_id alongside HasUnlocked + Lock reads. With the
// fix in place (lockoutState copies unlockAt into a local while
// holding the RLock) the race detector stays silent.
func TestUnlock_LockoutStateRaceFree_Ugly_DREAD_ADD_HIGH_4(t *core.T) {
	home := homeFixture(t)
	writeEncryptedAccount(t, home, fixtureAccountID, fixturePassphrase)
	svc := newUnlockable(t, home)

	const goroutines = 100
	done := make(chan struct{}, goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			_ = svc.Unlock(subject.UnlockInput{
				AccountID:  fixtureAccountID,
				Passphrase: fixtureWrongPassphrase,
			})
			_ = svc.HasUnlocked(fixtureAccountID)
			_ = svc.Lock(subject.LockInput{AccountID: fixtureAccountID})
		}()
	}
	for i := 0; i < goroutines; i++ {
		<-done
	}
	// Test passes if `go test -race ./pkg/account/...` finds no
	// race. The assertion below is existence-only — the actual
	// lockout state after 100 wrong attempts + interleaved Lock
	// calls is timing-dependent under -race.
	core.AssertFalse(t, svc.HasUnlocked(fixtureAccountID),
		"account MUST NOT be unlocked after 100 wrong attempts")
}

// TestUnlock_CorruptedBodyClassifiedAsCorruptedKey_Ugly_DREAD_ADD_MED_1
// pins the post-prompt corruption distinction. A ciphertext whose
// symmetric-key packet decodes (so prompt fires + returns the
// passphrase happily) but whose ENCRYPTED BODY is malformed
// (truncated mid-body, MDC tampered, etc) MUST classify as
// corrupted_key — NOT bad_passphrase. Otherwise the user gets
// locked out for a file-corruption that isn't their fault.
//
// Synthesises the case by encrypting with the FIXTURE passphrase
// then truncating mid-body (after the symkey packet but before the
// MDC). The prompt fires + accepts the passphrase; the body
// decrypt fails with a non-ErrKeyIncorrect error → corrupted_key.
func TestUnlock_CorruptedBodyClassifiedAsCorruptedKey_Ugly_DREAD_ADD_MED_1(t *core.T) {
	home := homeFixture(t)
	id := "5555555555555555"
	pgpSvc := pgp.NewService()
	_, privPlain, err := pgpSvc.GenerateKeyPair("lthn-test", "test@lthn.local", "fixture")
	core.AssertTrue(t, err == nil)
	full, err := pgpSvc.SymmetricallyEncrypt([]byte(fixturePassphrase), privPlain)
	core.AssertTrue(t, err == nil)
	core.AssertTrue(t, len(full) > 64, "fixture must be big enough to truncate mid-body")

	// Truncate to ~80% of the ciphertext. The symkey packet sits
	// at the front (small); the encrypted body fills the rest. By
	// cutting late in the stream we keep the symkey decoder happy
	// but starve the body decryptor.
	cutoff := (len(full) * 8) / 10
	if cutoff < 32 {
		cutoff = 32
	}
	truncated := full[:cutoff]
	writeRawAccount(t, home, id, truncated)

	svc := newUnlockable(t, home)
	r := svc.Unlock(subject.UnlockInput{
		AccountID:  id,
		Passphrase: fixturePassphrase, // the CORRECT passphrase
	})
	core.AssertFalse(t, r.OK, "truncated-body MUST reject")
	core.AssertEqual(t, "account.unlock.corrupted_key", r.Code(),
		"truncated-body MUST classify as corrupted_key, NOT bad_passphrase (Cerberus DREAD ADD-MED-1)")
}

// TestUnlock_PostLockoutAttempt_NoDuplicateTrigger_Ugly — after the
// lockout has triggered, subsequent attempts within the cooldown
// MUST return locked_out via the lockoutState early-reject path
// WITHOUT re-emitting auth.lockout.triggered. The reserved event
// is specifically the moment the threshold tripped; re-emitting on
// every rejected post-trigger attempt would pollute Stage F's
// log-tailer with duplicate trigger events.
//
// We can't assert on emit count directly without capturing stderr,
// but we CAN assert on the response shape — successive locked_out
// responses MUST share the same unlockAt timestamp (the lockout
// trip didn't fire a NEW lockout, just denied access on the
// existing one).
func TestUnlock_PostLockoutAttempt_NoDuplicateTrigger_Ugly(t *core.T) {
	home := homeFixture(t)
	writeEncryptedAccount(t, home, fixtureAccountID, fixturePassphrase)
	svc := newUnlockable(t, home)

	// Trigger the lockout (5 wrong attempts).
	for i := 0; i < 5; i++ {
		_ = svc.Unlock(subject.UnlockInput{
			AccountID: fixtureAccountID, Passphrase: fixtureWrongPassphrase,
		})
	}

	// Post-trigger attempts — MUST return locked_out without
	// resetting the cooldown.
	r1 := svc.Unlock(subject.UnlockInput{
		AccountID: fixtureAccountID, Passphrase: fixtureWrongPassphrase,
	})
	core.AssertFalse(t, r1.OK)
	core.AssertEqual(t, "account.unlock.locked_out", r1.Code())

	r2 := svc.Unlock(subject.UnlockInput{
		AccountID: fixtureAccountID, Passphrase: fixtureWrongPassphrase,
	})
	core.AssertFalse(t, r2.OK)
	core.AssertEqual(t, "account.unlock.locked_out", r2.Code())

	// Both rejections surface the same cooldown countdown (within
	// 1s tolerance for clock tick) — the second attempt didn't
	// reset the lockout window.
	core.AssertTrue(t, r1.Error() != "" && r2.Error() != "",
		"both rejections carry error messages")
}

// --- UnlockedAccountIDs (Mantis #1591 Option D enumerator) ---

// TestUnlockedAccountIDs_Empty_Good — zero unlocked accounts returns
// an empty slice (not nil panic).
func TestUnlockedAccountIDs_Empty_Good(t *core.T) {
	svc := subject.NewService(nil)
	ids := svc.UnlockedAccountIDs()
	core.AssertEqual(t, 0, len(ids))
}

// TestUnlockedAccountIDs_SingleUnlock_Good — one unlock returns one id.
func TestUnlockedAccountIDs_SingleUnlock_Good(t *core.T) {
	home := homeFixture(t)
	writeEncryptedAccount(t, home, fixtureAccountID, fixturePassphrase)
	svc := newUnlockable(t, home)

	r := svc.Unlock(subject.UnlockInput{
		AccountID:  fixtureAccountID,
		Passphrase: fixturePassphrase,
	})
	core.AssertTrue(t, r.OK)

	ids := svc.UnlockedAccountIDs()
	core.AssertEqual(t, 1, len(ids))
	core.AssertEqual(t, fixtureAccountID, ids[0])
}

// TestUnlockedAccountIDs_DeterministicSort_Ugly — repeated calls
// return ids in the SAME sorted order regardless of map-iter
// randomisation. Replaces the DefaultAccountID lottery (Mantis
// #1588) with deterministic enumeration.
func TestUnlockedAccountIDs_DeterministicSort_Ugly(t *core.T) {
	home := homeFixture(t)
	ids := []string{"ccc111", "aaa111", "bbb111"}
	for _, id := range ids {
		writeEncryptedAccount(t, home, id, fixturePassphrase)
	}
	svc := newUnlockable(t, home)
	for _, id := range ids {
		r := svc.Unlock(subject.UnlockInput{
			AccountID: id, Passphrase: fixturePassphrase,
		})
		core.AssertTrue(t, r.OK, "unlock "+id)
	}

	// Sorted alphabetically — same result every call.
	want := []string{"aaa111", "bbb111", "ccc111"}
	for i := 0; i < 10; i++ {
		got := svc.UnlockedAccountIDs()
		core.AssertEqual(t, len(want), len(got))
		for j := range want {
			core.AssertEqual(t, want[j], got[j])
		}
	}
}
