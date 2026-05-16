// SPDX-Licence-Identifier: EUPL-1.2

// unlock.go — Stage E.B per RFC.stage-e.md v2 §2 + §5. Implements the
// account-unlock service that turns a passphrase + account_id into an
// in-memory unlocked private key + a freshly-minted session token.
//
// The on-disk shape this consumes is the symmetric-PGP encrypted blob
// pkg/account.Seal (forthcoming, RFC §7) will write to
// ~/Lethean/account/<id>/private.key. For Stage E.B today the unlock
// path is testable against the encrypted-blob fixture the unlock
// tests synthesise; production paths land once the seal-step ticket
// lands separately (gap is intentional and called out in RFC §9).
//
// Failure-mode discipline per RFC §5 + §10 mandatory test
// TestUnlock_CorruptedKeyBytewiseDistinguishing_Ugly:
//
//   - intact ciphertext + correct passphrase → OK
//   - intact ciphertext + WRONG passphrase   → bad_passphrase
//   - structurally garbage / truncated armour → corrupted_key
//
// The distinction is observed by tracking whether the openpgp prompt
// closure was invoked — a packet-parser failure (garbage / truncated)
// errors BEFORE the prompt ever fires, so a zero invocation count
// implies the ciphertext is malformed; a non-zero count implies the
// parser reached prompt and the passphrase was at fault. NOT string-
// matched on error message per RFC §10.

package account

import (
	"bytes"
	"io"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/paths"
	"dappco.re/lthn/desktop/pkg/serverkey"
	"github.com/ProtonMail/go-crypto/openpgp"
)

// Unlock decrypts ~/Lethean/account/<input.AccountID>/private.key
// with the supplied passphrase, holds the unlocked private key in
// memory for the session lifetime, mints a fresh session token via
// serverkey, and returns the token + expiry to the caller.
//
// Service.serverkey MUST be wired (via NewServiceWith or
// SetServerKey) before Unlock is callable — Service constructs that
// don't wire the verifier explicitly will return
// "account.unlock.server_misconfigured" so the misconfiguration
// surfaces loud at first call rather than silent-passing.
//
// Per-account_id lockout per RFC §5 M1 — 5 failed attempts within
// the lockout window locks the SPECIFIC account_id for the lockout
// cooldown, NOT global. One typo-spamming user MUST NOT lock out a
// sibling account on the same machine.
//
// Privacy: account-not-found returns the same code +
// account-shape as bad_passphrase per RFC §5 ¹ so the endpoint
// doesn't leak which account_ids exist on disk to a probe attacker.
//
// Usage example:
//
//	r := svc.Unlock(account.UnlockInput{
//	    AccountID:  "abc123def4567890",
//	    Passphrase: "user-typed",
//	})
//	if r.OK { out := r.Value.(account.UnlockOutput); _ = out.SessionToken }
func (s *Service) Unlock(input UnlockInput) core.Result {
	if input.AccountID == "" {
		return core.Fail(core.NewCode(codeAccountIDRequired, "account_id is required"))
	}
	// Cerberus Stage E.B DREAD ADD-HIGH-2 (Mantis #1488-class compound
	// finding) — account_id is concatenated directly into private.key
	// path below. Without paths.IsValidID gating, a malicious id like
	// "../../wallets/lethean-default" lets core.Stat follow the
	// resolved path → measurable timing oracle (stat-hit vs stat-miss
	// path-cache deltas) + lockout-map DoS (unbounded growth on
	// unique traversal strings). The shape check rejects '/', '..',
	// leading '.', NUL, and >255 bytes before any disk touch.
	if err := paths.IsValidID(input.AccountID); err != nil {
		return core.Fail(err)
	}
	if input.Passphrase == "" {
		return core.Fail(core.NewCode(codeUnlockPassphraseRequired, "passphrase is required"))
	}
	if s.serverkey == nil {
		return core.Fail(core.NewCode(codeUnlockMisconfigured,
			"server-key verifier is not wired — Unlock requires SetServerKey at service construction"))
	}

	// Cerberus #1463-mirror — check lockout BEFORE touching disk.
	// A locked account never reaches the decrypt path, so the attacker
	// can't accelerate brute-force by parallelism past the per-account
	// cap.
	//
	// Cerberus DREAD ADD-HIGH-3 — auth.lockout.triggered is reserved
	// for the moment the threshold trips. Subsequent attempts within
	// the cooldown emit auth.unlock.failed with attempts_remaining=0
	// (consistent with "the user has zero chances right now") and
	// return locked_out. Re-emitting auth.lockout.triggered on every
	// rejected attempt would pollute the Stage F log-tailer with
	// duplicate trigger events that don't reflect new state.
	now := core.Now().UTC().Unix()
	if locked, unlockAt := s.lockoutState(input.AccountID, now); locked {
		s.auditUnlockFailed(input.AccountID, 0)
		return core.Fail(core.NewCode(codeUnlockLockedOut,
			core.Sprintf("too many attempts — try again in %d seconds", unlockAt-now)))
	}

	rootR := paths.Root()
	if !rootR.OK {
		return rootR
	}
	root, _ := rootR.Value.(string)
	privatePath := core.PathJoin(root, accountDir, input.AccountID, privateKeyFile)

	// Per RFC §5 ¹ — account-not-found collapses to bad_passphrase so
	// the response is byte-identical to a wrong-passphrase result.
	// The lockout counter STILL ticks (a probe attacker can't dodge
	// the per-account cap by spamming non-existent ids).
	statR := core.Stat(privatePath)
	if !statR.OK {
		remaining, triggered, unlockAt := s.recordFailedAttempt(input.AccountID, now)
		s.auditUnlockFailed(input.AccountID, remaining)
		if triggered {
			s.auditLockoutTriggered(input.AccountID, unlockAt)
			return core.Fail(core.NewCode(codeUnlockLockedOut,
				core.Sprintf("too many attempts — try again in %d seconds", unlockAt-now)))
		}
		return core.Fail(core.NewCode(codeUnlockBadPassphrase,
			core.Sprintf("passphrase didn't unlock your account — %d attempts remaining", remaining)))
	}

	// Open-time mode verify per Cerberus #1464 — a widened mode is
	// treated as tamper, fail-closed.
	info, ok := statR.Value.(core.FsFileInfo)
	if ok {
		if got := info.Mode().Perm(); got != fileMode {
			core.Warn("account.Unlock: private.key mode wider than expected (Cerberus #1464)",
				"path", privatePath,
				"got", core.Sprintf("%o", got),
				"want", core.Sprintf("%o", fileMode))
			return core.Fail(core.NewCode(codeUnlockCorruptedKey,
				core.Sprintf("private.key mode tamper: got %o want %o", got, fileMode)))
		}
	}

	readR := core.ReadFile(privatePath)
	if !readR.OK {
		// Read errors (permission-denied, IO) surface as corrupted_key
		// — the file is structurally unreadable from the unlock
		// path's POV. Operator runs `lthn account repair` to recover.
		return core.Fail(core.NewCode(codeUnlockCorruptedKey,
			"couldn't read your account file — run `lthn account repair` from a terminal"))
	}
	ciphertext, _ := readR.Value.([]byte)

	// Distinguishing decrypt — track whether the openpgp prompt was
	// invoked. Parser failures (garbage / truncated armour) fail
	// BEFORE the prompt closure ever fires; bad-passphrase failures
	// happen AFTER the parser reaches the symmetric-key packet and
	// invokes prompt. This is the H4 / RFC §10 ruling:
	// NOT-string-match-on-error-message.
	plaintext, kind, decryptErr := distinguishDecrypt(ciphertext, []byte(input.Passphrase))
	if decryptErr != nil {
		switch kind {
		case decryptFailureBadPassphrase:
			remaining, triggered, unlockAt := s.recordFailedAttempt(input.AccountID, now)
			s.auditUnlockFailed(input.AccountID, remaining)
			if triggered {
				// Cerberus DREAD ADD-HIGH-3 — surface the locked_out
				// response on the SAME attempt that triggered, so the
				// UI doesn't show "5 remaining" the moment the user
				// has 0 chances left.
				s.auditLockoutTriggered(input.AccountID, unlockAt)
				return core.Fail(core.NewCode(codeUnlockLockedOut,
					core.Sprintf("too many attempts — try again in %d seconds", unlockAt-now)))
			}
			return core.Fail(core.NewCode(codeUnlockBadPassphrase,
				core.Sprintf("passphrase didn't unlock your account — %d attempts remaining", remaining)))
		default:
			// corrupted_key — do NOT tick the lockout counter (a
			// corrupted file isn't user error; locking the user out
			// for the operator's repair window is hostile).
			return core.Fail(core.NewCode(codeUnlockCorruptedKey,
				"couldn't read your account file — run `lthn account repair` from a terminal"))
		}
	}

	// Successful decrypt — reset the lockout counter for this id
	// before minting the session token. A successful unlock is the
	// canonical signal that the user IS who they say they are.
	s.clearLockout(input.AccountID)

	// Hold the unlocked private key in memory for the session
	// lifetime. Replaces any prior unlocked-state on re-unlock.
	s.mu.Lock()
	if s.unlocked == nil {
		s.unlocked = map[string][]byte{}
	}
	s.unlocked[input.AccountID] = plaintext
	s.mu.Unlock()

	// Mint the session token. Verifier is wired at construction.
	tokR := s.serverkey.IssueSessionToken(input.AccountID)
	if !tokR.OK {
		// Mint failure is a server-internal error — surface as a
		// 500-shaped code so the route mapping in statusForCode
		// returns InternalServerError, not 401.
		return core.Fail(core.NewCode(codeUnlockSessionMintFailed,
			"unlock succeeded but session-token mint failed: "+tokR.Error()))
	}
	out, _ := tokR.Value.(serverkey.SessionTokenOutput)

	return core.Ok(UnlockOutput{
		SessionToken: out.Token,
		ExpiresAt:    out.ExpiresAt,
		AccountID:    input.AccountID,
	})
}

// Lock clears the in-process unlocked private key for the supplied
// account_id. Returns OK even if nothing was held (idempotent —
// callers SHOULD NOT need to track unlock state on the client side).
// Per RFC §6 emits auth.lock.requested for Stage F log-tailing.
//
// Usage example:
//
//	r := svc.Lock(account.LockInput{AccountID: "abc123def4567890"})
//	_ = r.OK
func (s *Service) Lock(input LockInput) core.Result {
	if input.AccountID == "" {
		return core.Fail(core.NewCode(codeAccountIDRequired, "account_id is required"))
	}
	// Cerberus Stage E.B DREAD ADD-HIGH-2 — same shape-validation
	// gate as Unlock. Lock doesn't touch disk but the audit-event
	// emit writes a structured field with the literal account_id;
	// unvalidated input here lets the attacker pollute audit logs
	// with traversal strings the operator later greps.
	if err := paths.IsValidID(input.AccountID); err != nil {
		return core.Fail(err)
	}
	s.mu.Lock()
	delete(s.unlocked, input.AccountID)
	s.mu.Unlock()
	s.auditLockRequested(input.AccountID)
	return core.Ok(LockOutput{AccountID: input.AccountID})
}

// SetServerKey wires the session-token issuer into the account
// service. Called at construction time (cmd/lthn/app.go) so Unlock
// can mint session tokens against the same serverkey the bearer-
// middleware verifies them against.
//
// Usage example:
//
//	svc := account.NewService(c)
//	svc.SetServerKey(serverkeySvc)
func (s *Service) SetServerKey(sk SessionTokenIssuer) {
	s.serverkey = sk
}

// HasUnlocked reports whether the account_id has a live in-memory
// unlocked private key. Wails-bound surface for the frontend so the
// gate can render the unlocked state without round-tripping a
// session-token verification on every navigation.
//
// Usage example:
//
//	if svc.HasUnlocked("abc123def4567890") { /* render dashboard */ }
func (s *Service) HasUnlocked(accountID string) bool {
	if accountID == "" {
		return false
	}
	s.mu.RLock()
	_, ok := s.unlocked[accountID]
	s.mu.RUnlock()
	return ok
}

// distinguishDecrypt runs openpgp.ReadMessage with a tracking prompt
// so the caller can distinguish parser failure (corrupted_key) from
// passphrase failure (bad_passphrase) by error TYPE — specifically,
// by whether the prompt closure was invoked. Returns the plaintext
// + a decryptFailureKind sentinel on failure paths.
//
// Implementation rationale:
//   - openpgp.ReadMessage's prompt closure is the SOLE path through
//     which the passphrase reaches the symmetric-key decryptor.
//   - Garbage / truncated armour fails inside packets.Next() BEFORE
//     prompt is reachable → prompt never invoked.
//   - Wrong passphrase fails inside s.Decrypt(passphrase) — prompt
//     fired once with the user's value, the symmetric-key packet
//     rejected it, the outer FindKey loop re-enters prompt which
//     returns the sentinel "decryption failed" error.
//
// The boolean signal is type-safe — no error.String() matching,
// no openpgp-error-type peering. Mirrors the RFC §10 H4 ruling
// verbatim.
func distinguishDecrypt(ciphertext, passphrase []byte) ([]byte, decryptFailureKind, error) {
	buf := bytes.NewReader(ciphertext)
	calls := 0
	prompt := func(_ []openpgp.Key, _ bool) ([]byte, error) {
		calls++
		if calls > 1 {
			// Second invocation = the symmetric-key packet rejected
			// the first attempt — wrong passphrase.
			return nil, errBadPassphrase
		}
		return passphrase, nil
	}
	md, err := openpgp.ReadMessage(buf, nil, prompt, nil)
	if err != nil {
		if calls == 0 {
			return nil, decryptFailureCorruptedKey, err
		}
		return nil, decryptFailureBadPassphrase, err
	}
	plaintext, err := io.ReadAll(md.UnverifiedBody)
	if err != nil {
		// Body-read failures after a successful prompt indicate
		// integrity issues on the encrypted message body itself —
		// corrupted_key territory.
		return nil, decryptFailureCorruptedKey, err
	}
	return plaintext, decryptFailureNone, nil
}

// errBadPassphrase is the sentinel the prompt closure returns on its
// second invocation. It's the load-bearing signal that distinguishes
// "the user typed wrong" from "the file is malformed". Kept as a
// core.Err so callers using core.Is can inspect it cleanly.
var errBadPassphrase = core.NewError("account.unlock: passphrase did not decrypt the symmetric-key packet")

// decryptFailureKind classifies the three terminal states of an
// unlock decrypt attempt for the caller to route into the correct
// error code without string-matching.
type decryptFailureKind int

const (
	decryptFailureNone decryptFailureKind = iota
	decryptFailureBadPassphrase
	decryptFailureCorruptedKey
)

// lockoutState reports whether the account_id is currently locked
// out (locked, unlockAt). If locked, unlockAt is the unix timestamp
// the lockout expires. Caller MUST hold no locks; lockoutState
// takes/releases the RLock internally.
//
// Returns (false, 0) when the account_id has never been seen OR has
// fewer than the threshold of recent failures.
func (s *Service) lockoutState(accountID string, nowUnix int64) (bool, int64) {
	s.mu.RLock()
	st, ok := s.lockouts[accountID]
	s.mu.RUnlock()
	if !ok {
		return false, 0
	}
	if st.unlockAt > nowUnix {
		return true, st.unlockAt
	}
	return false, 0
}

// recordFailedAttempt increments the lockout counter for the
// account_id, returns (remaining, triggered, unlockAt).
//
//   - remaining is the number of attempts left AFTER this one BEFORE
//     the lockout trips. On the trigger-attempt itself remaining is 0
//     — the user just used the last chance.
//   - triggered is true ONLY on the attempt that crosses the
//     threshold. Caller MUST emit auth.lockout.triggered + surface
//     the locked_out response on this single boolean — the next
//     attempt would also see "locked" via lockoutState but won't
//     re-trigger.
//   - unlockAt is the unix timestamp the cooldown expires; zero when
//     not triggered.
//
// Cerberus Stage E.B DREAD ADD-HIGH-3 (Mantis #1480) — the previous
// shape returned `remaining=5` on the trigger attempt because the
// code cleared the attempts slice before computing the response.
// User saw "5 attempts remaining" + the audit lied + the lockout
// event fired one attempt late. Fixed by computing remaining
// BEFORE the post-trigger slice-clear AND returning the triggered
// boolean so Unlock can route to the locked_out code on the same
// attempt that triggered.
//
// Lockout counter is per-account_id per Cerberus DREAD M1 — a
// typo-spamming user MUST NOT lock out a sibling account on the
// same machine.
func (s *Service) recordFailedAttempt(accountID string, nowUnix int64) (remaining int, triggered bool, unlockAt int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lockouts == nil {
		s.lockouts = map[string]*lockoutEntry{}
	}
	entry, ok := s.lockouts[accountID]
	if !ok {
		entry = &lockoutEntry{}
		s.lockouts[accountID] = entry
	}
	// Drop attempts that fell out of the rolling window before
	// counting this one — a stale attempt from yesterday shouldn't
	// help reach the cap today.
	cutoff := nowUnix - lockoutWindowSeconds
	kept := entry.attempts[:0]
	for _, ts := range entry.attempts {
		if ts > cutoff {
			kept = append(kept, ts)
		}
	}
	entry.attempts = append(kept, nowUnix)
	// Compute remaining + trigger state BEFORE the post-trigger
	// slice clear — otherwise the clear hides the "this attempt
	// just used the last chance" signal.
	count := len(entry.attempts)
	if count >= lockoutThreshold {
		entry.unlockAt = nowUnix + lockoutCooldownSeconds
		entry.attempts = entry.attempts[:0]
		return 0, true, entry.unlockAt
	}
	remaining = lockoutThreshold - count
	if remaining < 0 {
		remaining = 0
	}
	return remaining, false, 0
}

// clearLockout resets the lockout state for the account_id —
// invoked on a successful unlock. Defends against the case where
// the user crossed the threshold AFTER fixing the typo and want a
// clean slate.
func (s *Service) clearLockout(accountID string) {
	s.mu.Lock()
	delete(s.lockouts, accountID)
	s.mu.Unlock()
}

// auditUnlockFailed emits auth.unlock.failed per RFC §6 M4. Reserved
// schema: changing the field names after Stage E ships breaks the
// Stage F log-tailing contract.
func (s *Service) auditUnlockFailed(accountID string, remaining int) {
	core.Print(core.Stderr(),
		"event=auth.unlock.failed account_id=%s ts=%d attempts_remaining=%d\n",
		accountID, core.Now().UTC().Unix(), remaining)
}

// auditLockoutTriggered emits auth.lockout.triggered per RFC §6 M4.
func (s *Service) auditLockoutTriggered(accountID string, unlockAt int64) {
	core.Print(core.Stderr(),
		"event=auth.lockout.triggered account_id=%s ts=%d unlock_after=%d\n",
		accountID, core.Now().UTC().Unix(), unlockAt)
}

// auditLockRequested emits auth.lock.requested per RFC §6 M4 when
// the frontend / CLI explicitly Sign-Out for a session.
func (s *Service) auditLockRequested(accountID string) {
	core.Print(core.Stderr(),
		"event=auth.lock.requested account_id=%s ts=%d\n",
		accountID, core.Now().UTC().Unix())
}
