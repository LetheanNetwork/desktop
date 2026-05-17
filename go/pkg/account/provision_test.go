// SPDX-Licence-Identifier: EUPL-1.2

// Tests for the account.Service Provision contract per Stage X.B
// Phase 2a (RFC.stage-x.md §3). Covers the happy-path round-trip,
// the missing-passphrase + misconfigured-issuer rejections, and the
// load-bearing cross-platform NFKC parity guarantee: an account
// provisioned with NFD-typed passphrase unlocks against NFC input
// (and vice-versa), so the same logical passphrase typed on any OS
// keyboard works.
//
// Deferred to later Phase 2 tests (RFC.stage-x.md §10):
//   - request_id 5min dedup (Phase 2c)
//   - hibp common-passphrase rejection (Phase 2b, pkg/passphrase)
//   - raw-length cap before NFKC (Phase 2b)
//   - memory-zero best-effort assertions (Phase 2d)

package account_test

import (
	"os"

	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/account"
	"dappco.re/lthn/desktop/pkg/audit"
	"dappco.re/lthn/desktop/pkg/paths"
)

// TestAccount_Provision_Good — happy round-trip. Provision returns
// the canonical account_id + a non-empty session token; the on-disk
// private.key file exists; AccountStatus afterwards reports
// has_user_account=true with the same account_id.
func TestAccount_Provision_Good(t *core.T) {
	home := homeFixture(t)
	svc := newUnlockable(t, home)

	r := svc.Provision(subject.ProvisionInput{
		Passphrase: "correct horse battery staple",
		RequestID:  "test-uuid-good",
	})
	core.AssertTrue(t, r.OK, "valid passphrase must provision a fresh account")

	out, _ := r.Value.(subject.ProvisionOutput)
	core.AssertTrue(t, len(out.AccountID) == 16,
		"account_id must be 16 hex chars (deriveAccountID truncation)")
	core.AssertTrue(t, out.SessionToken != "",
		"provision MUST issue a session token in the success response")
	core.AssertTrue(t, out.Path != "", "provision MUST return the on-disk path")
	core.AssertTrue(t, !out.DedupHit, "Phase 2a — DedupHit always false")

	// AccountStatus afterwards reports has_user_account=true. The
	// AccountID surfaced by AccountStatus matches Provision's return.
	statusR := svc.AccountStatus()
	core.AssertTrue(t, statusR.OK)
	st, _ := statusR.Value.(subject.AccountStatus)
	core.AssertTrue(t, st.HasAccount, "after Provision, AccountStatus MUST report has_account=true")
	core.AssertEqual(t, out.AccountID, st.AccountID)
}

// TestAccount_Provision_PassphraseRequired_Bad — empty passphrase
// rejects before any keygen runs. Returns the canonical code so the
// frontend can render the "enter your passphrase" inline error.
func TestAccount_Provision_PassphraseRequired_Bad(t *core.T) {
	_ = homeFixture(t)
	svc := newUnlockable(t, "")

	r := svc.Provision(subject.ProvisionInput{Passphrase: "", RequestID: "x"})
	core.AssertFalse(t, r.OK)
	core.AssertEqual(t, "account.provision.passphrase.required", r.Code())
}

// TestAccount_Provision_Misconfigured_Bad — Service without a wired
// SessionTokenIssuer rejects so the misconfiguration surfaces loud at
// first call rather than silently writing the account then failing to
// hand back a session token.
func TestAccount_Provision_Misconfigured_Bad(t *core.T) {
	_ = homeFixture(t)
	svc := subject.NewService(nil) // NO SetServerKey call

	r := svc.Provision(subject.ProvisionInput{Passphrase: "any", RequestID: "x"})
	core.AssertFalse(t, r.OK)
	core.AssertEqual(t, "account.provision.server_misconfigured", r.Code())
}

// TestProvision_RoundTripUnlock_Ugly — full create→unlock round-trip
// with the SAME passphrase. Proves the encrypted blob written by
// Provision is a real PGP private-key blob the Unlock path can
// decrypt, end-to-end. This is the load-bearing functional invariant
// without which Stage X.B is useless.
func TestProvision_RoundTripUnlock_Ugly(t *core.T) {
	home := homeFixture(t)
	svc := newUnlockable(t, home)

	pp := "round-trip-test-passphrase"

	provR := svc.Provision(subject.ProvisionInput{
		Passphrase: pp,
		RequestID:  "uuid-roundtrip",
	})
	core.AssertTrue(t, provR.OK, "provision must succeed")
	out, _ := provR.Value.(subject.ProvisionOutput)

	// New service instance so unlocked-keyring + lockouts are fresh;
	// exercises the on-disk artefact rather than the in-memory state
	// Provision left behind.
	svc2 := newUnlockable(t, home)
	unlockR := svc2.Unlock(subject.UnlockInput{
		AccountID:  out.AccountID,
		Passphrase: pp,
	})
	core.AssertTrue(t, unlockR.OK,
		"Unlock MUST decrypt what Provision encrypted using the same passphrase")
	core.AssertTrue(t, svc2.HasUnlocked(out.AccountID),
		"after a successful Unlock the in-memory ring holds the decrypted private key")
}

// TestProvision_WrongPassphraseDoesNotUnlock_Ugly — proves Provision
// actually encrypts (not stores plaintext / a marker). A different
// passphrase MUST yield bad_passphrase from Unlock.
func TestProvision_WrongPassphraseDoesNotUnlock_Ugly(t *core.T) {
	home := homeFixture(t)
	svc := newUnlockable(t, home)

	provR := svc.Provision(subject.ProvisionInput{
		Passphrase: "passphrase-A",
		RequestID:  "uuid-encrypt-check",
	})
	core.AssertTrue(t, provR.OK)
	out, _ := provR.Value.(subject.ProvisionOutput)

	svc2 := newUnlockable(t, home)
	unlockR := svc2.Unlock(subject.UnlockInput{
		AccountID:  out.AccountID,
		Passphrase: "passphrase-B",
	})
	core.AssertFalse(t, unlockR.OK,
		"wrong passphrase MUST NOT decrypt — proves the blob is real AES-256, not a marker")
	core.AssertEqual(t, "account.unlock.bad_passphrase", unlockR.Code())
}

// TestProvision_NFKCCrossPlatformParity_Ugly — RFC.stage-x.md §6,
// closes the cross-platform self-lockout from the create side. An
// account provisioned with NFD-typed passphrase (macOS keyboard)
// unlocks against NFC input (Linux keyboard). Without the NFKC
// normalisation on BOTH sides, this round-trip fails.
func TestProvision_NFKCCrossPlatformParity_Ugly(t *core.T) {
	home := homeFixture(t)
	svc := newUnlockable(t, home)

	// User on macOS provisions with NFD-form passphrase.
	provR := svc.Provision(subject.ProvisionInput{
		Passphrase: nfdCafe, // "cafe" + combining acute (constants in nfkc_test.go)
		RequestID:  "uuid-nfkc-parity",
	})
	core.AssertTrue(t, provR.OK, "provision with NFD passphrase must succeed")
	out, _ := provR.Value.(subject.ProvisionOutput)

	// User re-installs on Linux and types the NFC form of the SAME
	// logical character. Both forms NFKC-normalise to the same
	// canonical, so unlock MUST succeed.
	svc2 := newUnlockable(t, home)
	unlockR := svc2.Unlock(subject.UnlockInput{
		AccountID:  out.AccountID,
		Passphrase: nfcCafe,
	})
	core.AssertTrue(t, unlockR.OK,
		"NFC-typed passphrase MUST unlock account provisioned with NFD — cross-platform parity")
}

// TestProvision_EmitsAuditEvent_PathHashDiscipline_Good pins the
// Mantis #1577 contract: the auth.account.provisioned emission at
// provision.go:225 MUST carry Meta.path_hash (SHA-256 hex of the
// canonical account directory) and MUST NOT carry the raw path bytes.
// Mirrors the sibling Service.Create emit-site shape (commit 7feaa8b,
// Mantis #1574) — Cerberus #1465 closure-only-scope discipline
// applies to filesystem layout (username / install location) too.
func TestProvision_EmitsAuditEvent_PathHashDiscipline_Good(t *core.T) {
	home := homeFixture(t)
	svc := newUnlockable(t, home)

	rec := &recordingRecorder{}
	audit.SetDefault(rec)
	t.Cleanup(func() { audit.SetDefault(nil) })

	r := svc.Provision(subject.ProvisionInput{
		Passphrase: "correct horse battery staple",
		RequestID:  "test-req-1577",
	})
	core.AssertTrue(t, r.OK, "Provision must succeed for the audit-emit assertion")

	core.AssertEqual(t, 1, len(rec.events),
		"Provision must emit exactly one audit event on success")
	ev := rec.events[0]
	core.AssertEqual(t, audit.EventAuthAccountProvisioned, ev.Event)
	core.AssertEqual(t, audit.OutcomeOK, ev.Outcome)
	core.AssertEqual(t, "account.create", ev.Scope)
	core.AssertEqual(t, "test-req-1577", ev.RequestID)

	// Meta MUST carry path_hash (SHA-256 hex string, 64 chars) and
	// MUST NOT carry the raw path bytes — Mantis #1577 / Cerberus
	// #1465 closure-only-scope discipline.
	pathHash, ok := ev.Meta["path_hash"].(string)
	core.AssertTrue(t, ok, "Meta.path_hash must be present as string")
	core.AssertLen(t, pathHash, 64, "path_hash must be SHA-256 hex (64 chars)")
	_, hasRawPath := ev.Meta["path"]
	core.AssertFalse(t, hasRawPath, "Meta MUST NOT carry raw path (Cerberus #1465 discipline)")
}

// --- Cutover tests (Mantis #1578) ---
//
// Pin the post-cutover contracts so a future regression that bypasses
// paths.AtomicWriteWithVersion (e.g. someone reintroduces a local
// atomicWrite shim) trips a named test. Service-tier round-trip per
// RFC §6: invoke Provision, observe disk state + Fail codes.

// TestProvision_Cutover_Atomic_Good — pins post-cutover happy-path
// invariants: all three files (public.key, meta.json, private.key) land
// in correct order with 0o600 mode under the canonical account dir.
// Mirrors the service-tier round-trip discipline from RFC §6.
func TestProvision_Cutover_Atomic_Good(t *core.T) {
	home := homeFixture(t)
	svc := newUnlockable(t, home)

	r := svc.Provision(subject.ProvisionInput{
		Passphrase: "cutover-good-pp",
		RequestID:  "cutover-good",
	})
	core.AssertTrue(t, r.OK, "Provision MUST succeed on fresh install")
	out, _ := r.Value.(subject.ProvisionOutput)

	dir := core.PathJoin(home, "Lethean", "account", out.AccountID)
	for _, name := range []string{"public.key", "meta.json", "private.key"} {
		p := core.PathJoin(dir, name)
		statR := core.Stat(p)
		core.AssertTrue(t, statR.OK, "post-cutover Provision MUST land "+name)
		info, _ := statR.Value.(core.FsFileInfo)
		core.AssertEqual(t, core.FileMode(0o600), info.Mode().Perm(),
			name+" MUST be written 0o600 by the primitive's at-rest gate")
	}
}

// TestProvision_Cutover_PartialFail_Bad — pins the partial-fail
// invariant: per RFC §5.2, the three writes are sequential (each owns
// its own primitive call). If the second write (meta.json) fails via
// the primitive's mode-tamper gate, public.key has already landed but
// meta.json and private.key MUST NOT — so AccountStatus reports
// has_user_account=false and the next Provision retry can recover.
//
// Fixture: arm SetPostRenameModeTamperForTest with a per-call counter
// so the FIRST call (public.key) passes and the SECOND call (meta.json)
// trips the gate.
func TestProvision_Cutover_PartialFail_Bad(t *core.T) {
	home := homeFixture(t)
	svc := newUnlockable(t, home)

	// Per-call counter: first invocation (public.key) is a no-op, the
	// second invocation (meta.json) widens perms so the verify gate
	// fires.
	calls := 0
	paths.SetPostRenameModeTamperForTest(func(p string) {
		calls++
		if calls == 2 {
			_ = os.Chmod(p, 0o644)
		}
	})
	t.Cleanup(func() { paths.SetPostRenameModeTamperForTest(nil) })

	r := svc.Provision(subject.ProvisionInput{
		Passphrase: "cutover-partial-pp",
		RequestID:  "cutover-partial",
	})
	core.AssertFalse(t, r.OK, "second-write mode-tamper MUST fail Provision")
	core.AssertContains(t, r.Error(), "meta.json",
		"the failing write MUST be meta.json (sequence: public.key → meta.json → private.key)")

	// AccountStatus MUST report has_user_account=false because
	// private.key never landed (leaf-rule per Cerberus #1471).
	st := svc.AccountStatus()
	core.AssertTrue(t, st.OK)
	statusOut, _ := st.Value.(subject.AccountStatus)
	core.AssertFalse(t, statusOut.HasAccount,
		"partial-write (no private.key) MUST report has_account=false")
}
