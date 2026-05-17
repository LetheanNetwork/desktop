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
	"sync"
	"sync/atomic"
	"time"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/audit"
	"dappco.re/lthn/desktop/pkg/paths"
	"dappco.re/lthn/desktop/pkg/serverkey"
	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/packet"
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

	// Mantis #1590 LOW (Cerberus #18) — distinguish a Create-marker
	// from genuine corruption BEFORE the PGP decrypt attempt. Per
	// service.go:240, Service.Create writes a deterministic placeholder
	// (hex SHA-256 of the public key, 64 ASCII chars) into private.key
	// so the leaf-file invariant (#1471) flips to has_user_account=true
	// while the seal-step is pending. That marker is structurally NOT a
	// PGP envelope, so distinguishDecrypt would mis-classify it as
	// corrupted_key + tell the user to run `lthn account repair` —
	// hostile, since the real remedy is to call /v1/account/provision
	// (or its Stage X.B successor). Route to a dedicated code so the
	// frontend can dispatch the provision-prompt UX instead of the
	// generic error screen.
	if isCreateMarker(ciphertext, input.AccountID) {
		s.auditUnlockNeedsProvision(input.AccountID)
		return core.Fail(core.NewCode(codeUnlockNeedsProvision,
			"account created but not provisioned — call /v1/account/provision to seal the private key"))
	}

	// Cerberus DREAD CRIT-1 (RFC.stage-x.md §6) — NFKC normalise the
	// passphrase BEFORE decrypt so the same logical input typed on
	// different OS keyboards (NFC vs NFD) unlocks the same account.
	// The create-side mirror lands in Stage X.B Phase 2's Provision.
	// Symmetry across encrypt + decrypt is load-bearing — diverging
	// here causes permanent cross-platform self-lockout.
	passphrase := nfkcPassphrase(input.Passphrase)
	defer zeroAndKeepAlive(passphrase)

	// Distinguishing decrypt — track whether the openpgp prompt was
	// invoked. Parser failures (garbage / truncated armour) fail
	// BEFORE the prompt closure ever fires; bad-passphrase failures
	// happen AFTER the parser reaches the symmetric-key packet and
	// invokes prompt. This is the H4 / RFC §10 ruling:
	// NOT-string-match-on-error-message.
	plaintext, kind, failureReason, decryptErr := distinguishDecrypt(ciphertext, passphrase)
	if decryptErr != nil {
		switch kind {
		case decryptFailureBadPassphrase:
			remaining, triggered, unlockAt := s.recordFailedAttempt(input.AccountID, now)
			s.auditUnlockFailedWithReason(input.AccountID, remaining, failureReason)
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
			// for the operator's repair window is hostile). The
			// forensic reason still rides the audit log so ops can
			// distinguish pre-parser garbage from post-prompt body
			// integrity issues without the user-facing response
			// timing exposing the difference (Mantis #1531).
			s.auditUnlockFailedWithReason(input.AccountID, 0, failureReason)
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
	// Threads input.RequestID into the auth.session.issued audit
	// event so a forensic Query can join the mint event back to
	// the originating HTTP request (Cerberus #1711 Phase 2.5
	// punch-list). Empty RequestID is acceptable for non-HTTP
	// callers (CLI / tests) — the audit event simply omits the
	// request_id field.
	tokR := s.serverkey.IssueSessionTokenWithRequest(input.AccountID, input.RequestID)
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
// by which sentinel the prompt closure returned. Returns plaintext
// + a decryptFailureKind sentinel on failure paths.
//
// Four signal regions:
//
//  1. calls == 0 → parser failed BEFORE the prompt closure was
//     reachable. Garbage bytes / truncated armour / unrecognised
//     packet types. Classified as corrupted_key.
//
//  2. err is (or wraps) errBadPassphrase → the prompt closure's
//     second invocation returned the sentinel. The first attempt
//     used the user's passphrase + the symmetric-key packet
//     rejected it; the FindKey loop re-entered prompt which
//     surfaced our sentinel. Classified as bad_passphrase.
//
//  3. calls > 0, err != nil, NOT errBadPassphrase → the prompt was
//     invoked but openpgp errored on the post-decrypt packet-parse
//     step. Cerberus #1711 + Mantis #1510 — for a ~5% subset of
//     wrong-passphrase × ciphertext pairs the v4 S2K-derived key
//     yields a cipherFunc that openpgp's FindKey loop accepts
//     (read.go:215 — "only for < 5% of cases we will proceed to
//     decrypt the data"). The decrypted body is then random bytes
//     that fail to parse as OpenPGP packets, surfacing as
//     ErrDecryptSessionKeyParsing — IDENTICAL sentinel to a
//     genuine truncated-body case (DREAD ADD-MED-1). To
//     distinguish, classifyPostPromptError re-probes via the
//     packet API: it decrypts the SKE+SE packets directly with
//     the user's passphrase and inspects the first decrypted byte
//     for an RFC 4880 §4.2 packet-tag canary (bit 7 set). Garbage
//     first byte → wrong passphrase that happened to clear the
//     cipherFunc gate. Valid first byte → real post-prompt
//     corruption, keep corrupted_key (so we don't lock the user
//     out for a file integrity issue that is NOT their fault).
//
//  4. io.ReadAll on md.UnverifiedBody after a successful
//     ReadMessage erroring → corrupted_key (post-prompt body
//     integrity error with a correct passphrase).
//
// NOT-string-match on error message — the signal is purely type-
// based (sentinel-identity check via core.Is, then byte-canary
// validation). Mirrors RFC §10 H4.
func distinguishDecrypt(ciphertext, passphrase []byte) ([]byte, decryptFailureKind, string, error) {
	// Mantis #1531 — single-hot-path timing bucket for ALL
	// bad_passphrase reasons. Previously classifyPostPromptError
	// (~4 KiB-bounded decrypt+parse) only ran on the post-
	// cipherFunc-gate path, leaving pre-prompt parser failures
	// (~5ms) measurably cheaper than sentinel/post-gate paths
	// (~24ms on a typical machine). A wallclock-measuring attacker
	// on a quiet loopback could partition rejected attempts by
	// sub-reason and prune brute-force search space by a small
	// constant factor.
	//
	// Equal-budget mechanism: snapshot the start time and pad EVERY
	// failure path up to timingFloor (a calibrated minimum cost
	// derived from observing the slowest path on the first SE-bearing
	// decrypt). The forensic distinction lives in the audit Meta
	// reason field returned alongside the kind. Damage envelope per
	// Cerberus #1531: timing reveals nothing beyond "bad attempt".
	start := core.Now()
	defer padToTimingFloor(start)

	buf := bytes.NewReader(ciphertext)
	calls := 0
	prompt := func(_ []openpgp.Key, _ bool) ([]byte, error) {
		calls++
		if calls > 1 {
			// Second invocation = the symmetric-key packet rejected
			// the first attempt — wrong passphrase. Sentinel return
			// is the type-safe signal the caller branches on.
			return nil, errBadPassphrase
		}
		return passphrase, nil
	}
	md, err := openpgp.ReadMessage(buf, nil, prompt, nil)
	if err != nil {
		// Run the post-prompt re-probe unconditionally on every
		// failure path. On the sentinel and pre-parser paths the
		// return value is discarded (we already know the bucket from
		// the sentinel-identity check / calls-count); on the post-
		// gate path the return value distinguishes bad_passphrase
		// from corrupted (Mantis #1510). Running it always also
		// raises the timing floor for the pre-parser path: classify
		// itself runs cheaply on garbage (packet.Read fails fast),
		// so the padToTimingFloor deferred pad finishes the
		// equalisation.
		postProbe := classifyPostPromptError(ciphertext, passphrase)
		// Record the observed cost as a candidate timing floor — the
		// post-gate path (which actually pays the SKE+SE decrypt
		// cost inside classifyPostPromptError) is structurally the
		// slowest of the failure paths, so its measurement is the
		// right value for padToTimingFloor to target.
		recordTimingObservation(core.Since(start))

		switch {
		case core.Is(err, errBadPassphrase):
			return nil, decryptFailureBadPassphrase, reasonBadPassphraseSentinel, err
		case calls > 0 && postProbe:
			// Mantis #1510 — wrong passphrase that slipped past the
			// openpgp FindKey cipherFunc gate. Re-probe confirmed
			// the decrypted first byte is garbage (no RFC 4880
			// packet-tag bit), so this is bad_passphrase, not
			// corruption. Wrap in errBadPassphrase so callers using
			// core.Is keep working.
			return nil, decryptFailureBadPassphrase, reasonBadPassphrasePostGate,
				core.E("account.unlock", "post-prompt decrypt yielded garbage — passphrase wrong", errBadPassphrase)
		default:
			// calls == 0 (pre-prompt parser failure) AND calls > 0
			// with a valid-shaped post-prompt plaintext indicate
			// genuine corruption (DREAD ADD-MED-1).
			if calls == 0 {
				return nil, decryptFailureCorruptedKey, reasonCorruptedPreParser, err
			}
			return nil, decryptFailureCorruptedKey, reasonCorruptedPostBody, err
		}
	}
	plaintext, err := io.ReadAll(md.UnverifiedBody)
	if err != nil {
		// Body-read failures after a successful prompt indicate
		// integrity issues on the encrypted message body itself —
		// corrupted_key territory. Equal-budget invariant: this
		// path follows a SUCCESSFUL openpgp.ReadMessage so it has
		// already paid the full parse + symmetric-decrypt cost;
		// padToTimingFloor's deferred pad still tops it up if the
		// configured floor exceeds what this path naturally spent.
		recordTimingObservation(core.Since(start))
		return nil, decryptFailureCorruptedKey, reasonCorruptedBodyRead, err
	}
	recordTimingObservation(core.Since(start))
	return plaintext, decryptFailureNone, reasonOK, nil
}

// timingFloorNanos is the atomic-tracked minimum total wallclock
// duration distinguishDecrypt MUST pay before returning. Starts at
// timingFloorSeedNanos (~25 ms — a conservative initial guess that
// covers a typical M-series + race-test run) and ratchets UP whenever
// recordTimingObservation sees a larger natural cost. Never ratchets
// down — once the slowest observed path is set as the floor, all
// future calls pay at least that much regardless of how cheap their
// natural execution is.
//
// Mantis #1531 — the goal is timing-uniformity across sub-reasons,
// NOT a fixed wallclock budget. Floor-ratcheting follows the
// structural cost of the heaviest path so the constant-time invariant
// survives hardware upgrades (faster boxes ratchet the floor down via
// recompilation, not at runtime).
var timingFloorNanos atomic.Int64

// timingFloorSeedNanos is the initial floor before any observation
// has landed. Chosen larger than a typical post-gate path on dev
// hardware so a first-attempt sentinel-bucket Unlock can't undershoot
// the cost of a yet-unobserved post-gate Unlock by enough for an
// attacker to distinguish the two. Tuned conservatively.
const timingFloorSeedNanos = int64(25 * time.Millisecond)

// timingFloorInit guards a one-shot seed of timingFloorNanos so the
// first distinguishDecrypt invocation observes a sane starting value.
var timingFloorInit sync.Once

// padToTimingFloor sleeps until at least timingFloorNanos has elapsed
// since the supplied start time. Cheap paths (pre-parser garbage,
// fast sentinel rejection) get padded up to the floor; paths that
// naturally exceed the floor return immediately with no extra delay.
//
// Mantis #1531 single-bucket invariant: every call to
// distinguishDecrypt's failure path returns no faster than the floor.
func padToTimingFloor(start time.Time) {
	timingFloorInit.Do(func() {
		if timingFloorNanos.Load() == 0 {
			timingFloorNanos.Store(timingFloorSeedNanos)
		}
	})
	floor := timingFloorNanos.Load()
	elapsed := core.Since(start).Nanoseconds()
	if elapsed < floor {
		core.Sleep(time.Duration(floor - elapsed))
	}
}

// recordTimingObservation ratchets timingFloorNanos UP if the
// observed cost exceeds the current floor. Never ratchets down —
// hardware speed-ups land via recompilation (re-tuning
// timingFloorSeedNanos), not runtime observation, so an attacker
// can't drive the floor down by spamming cheap pre-parser failures
// from a warm cache.
//
// Called on every distinguishDecrypt exit (success and failure)
// so the success-path cost is also folded into the floor — the
// happy-path Unlock is the only place where the full parse + body
// decrypt cost is actually paid on a well-formed envelope.
func recordTimingObservation(observed time.Duration) {
	timingFloorInit.Do(func() {
		if timingFloorNanos.Load() == 0 {
			timingFloorNanos.Store(timingFloorSeedNanos)
		}
	})
	obs := observed.Nanoseconds()
	for {
		cur := timingFloorNanos.Load()
		if obs <= cur {
			return
		}
		if timingFloorNanos.CompareAndSwap(cur, obs) {
			return
		}
	}
}

// Mantis #1531 — forensic-only reason codes the audit Meta carries so
// ops can partition bad_passphrase + corrupted_key buckets without the
// response timing exposing the same distinction. The strings are stable
// schema (consumed by the Stage F log-tailer); changing them breaks the
// reserved-schema contract per RFC §6 M4.
const (
	reasonOK                    = "ok"
	reasonBadPassphraseSentinel = "bad_passphrase.sentinel"
	reasonBadPassphrasePostGate = "bad_passphrase.post_cipherfunc_gate"
	reasonCorruptedPreParser    = "corrupted.pre_parser"
	reasonCorruptedPostBody     = "corrupted.post_prompt_body"
	reasonCorruptedBodyRead     = "corrupted.body_read"
)

// classifyPostPromptError re-probes a ciphertext that produced a
// post-prompt non-sentinel error from openpgp.ReadMessage. The
// helper reads the symmetric-key packet + symmetrically-encrypted
// packet directly via the openpgp/packet API, decrypts with the
// supplied passphrase, and inspects the first decrypted byte for
// the RFC 4880 §4.2 packet-tag canary (bit 7 MUST be set on any
// well-formed packet header).
//
// Returns true when the first byte looks like garbage → the
// passphrase was wrong but happened to clear the cipherFunc gate
// (openpgp read.go:215 — ~5% of wrong-passphrase decrypts proceed
// past the FindKey loop). Returns false when the first byte parses
// as a valid packet-tag (genuine post-prompt corruption — likely
// truncated body / tampered MDC with the CORRECT passphrase, DREAD
// ADD-MED-1) OR when the re-probe itself errors (we can't tell, so
// defer to the existing corrupted_key classification).
//
// NOT a security boundary — this is a UX classifier. The lockout
// counter is unaffected; the only observable difference is the
// human-readable message the user sees + which audit event fires.
//
// Usage:
//
//	if classifyPostPromptError(ciphertext, passphrase) {
//	    return nil, decryptFailureBadPassphrase, errBadPassphrase
//	}
func classifyPostPromptError(ciphertext, passphrase []byte) bool {
	buf := bytes.NewReader(ciphertext)
	var ske *packet.SymmetricKeyEncrypted
	var se packet.EncryptedDataPacket
	// Walk packets until we find both an SKE (passphrase-derived
	// session-key holder) and an SE/SEIPD/AEAD body packet. Bail
	// out on parse error — that's structural corruption, not the
	// case this helper is meant to disambiguate.
	for ske == nil || se == nil {
		p, err := packet.Read(buf)
		if err != nil {
			return false
		}
		switch pp := p.(type) {
		case *packet.SymmetricKeyEncrypted:
			ske = pp
		case *packet.SymmetricallyEncrypted:
			se = pp
		case *packet.AEADEncrypted:
			se = pp
		}
	}
	key, cipherFunc, err := ske.Decrypt(passphrase)
	if err != nil {
		// SKE-level rejection (v5/v6 AEAD MAC fail, or v4 invalid
		// cipherFunc enum) is the openpgp library's own
		// "definitely wrong passphrase" signal — surface it as
		// bad_passphrase. Path is rarely reached from this helper
		// (the caller only invokes us when openpgp.ReadMessage
		// already accepted the SKE) but defensive for v5/v6.
		return true
	}
	rc, err := se.Decrypt(cipherFunc, key)
	if err != nil {
		// Decrypt-setup error — can't probe further, keep
		// corrupted_key classification.
		return false
	}
	defer func() { _ = rc.Close() }()
	// Attempt to parse the decrypted stream AS a packet via
	// packet.Read. A genuine SEIPD body inner plaintext begins
	// with one of: Compressed (tag 8), LiteralData (tag 11), or
	// OnePassSignature (tag 4). Random bytes from a wrong-pp
	// decryption have a vanishing chance (~0.5%) of parsing
	// cleanly as one of those tagged structures — packet.Read
	// validates the tag byte AND header-length encoding AND
	// inner-type-specific parse rules, giving us a multi-byte
	// canary instead of a single bit.
	//
	// Cap the body read so a malicious ciphertext can't make us
	// stream gigabytes of garbage during the probe. 4 KiB is
	// enough to parse any inner-packet header + tail; if the body
	// is shorter, the LimitReader still surfaces the parse result.
	const probeBudget = 4096
	limited := io.LimitReader(rc, probeBudget)
	inner, err := packet.Read(limited)
	if err != nil {
		// Couldn't parse the decrypted bytes as a valid OpenPGP
		// packet header — the passphrase was wrong. Random-byte
		// noise practically never produces a valid tag + length
		// combo packet.Read accepts.
		return true
	}
	// Genuine inner packet types are Compressed / LiteralData /
	// OnePassSignature. SignatureV3/V4 also appear in some
	// signed-then-encrypted flows. Anything else (PublicKey, UserId,
	// Trust, SymmetricKeyEncrypted, …) inside an SEIPD body would
	// be structurally nonsensical — almost certainly the artefact
	// of garbage bytes that happened to parse as something. Be
	// conservative: only accept the canonical inner types as a
	// "real plaintext" signal; everything else → bad_passphrase.
	switch inner.(type) {
	case *packet.Compressed, *packet.LiteralData, *packet.OnePassSignature, *packet.Signature:
		return false // looks like a real inner packet — keep corrupted_key
	}
	return true // unexpected inner type → wrong passphrase
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
//
// Cerberus DREAD ADD-HIGH-4 — the map value is *lockoutEntry, so
// the map-RLock only protects the map header (which slot maps to
// which pointer), NOT the pointed-to lockoutEntry struct. A
// concurrent recordFailedAttempt holds the write lock + mutates
// entry.unlockAt; a lockoutState read of entry.unlockAt AFTER
// RUnlock races against that write. Fix: copy the field into a
// local while holding the RLock, then read the local. `go test
// -race ./pkg/account/...` covers the regression.
func (s *Service) lockoutState(accountID string, nowUnix int64) (bool, int64) {
	s.mu.RLock()
	var (
		unlockAt int64
		found    bool
	)
	if st, ok := s.lockouts[accountID]; ok {
		unlockAt = st.unlockAt
		found = true
	}
	s.mu.RUnlock()
	if !found {
		return false, 0
	}
	if unlockAt > nowUnix {
		return true, unlockAt
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
//
// Mantis #1586 HIGH (Cerberus #18) — the lockout map was an
// unbounded DoS surface: every failed attempt against an
// attacker-chosen account_id created a fresh *lockoutEntry keyed by
// the supplied string. 10M random valid-shape IDs → ~2 GB → OOM.
// Two-part fix below:
//
//   - Option 2 gate: only allocate / mutate a lockout entry when the
//     on-disk account file exists. Probe traffic against non-existent
//     IDs never touches the map. RFC §5 privacy invariant is
//     preserved — caller still surfaces bad_passphrase on the
//     Stat-fail branch (see Unlock line 122); only the in-memory
//     bookkeeping diverges.
//
//   - Option 3 opportunistic prune: every recordFailedAttempt write
//     also walks the map and drops entries whose latest attempt
//     fell out of the rolling window AND whose cooldown has
//     expired. Bounds map size over time without an LRU eviction
//     lottery.
//
// Skips Option 1 (LRU cap) per Cerberus #18 — the cap value is a
// guess; (2)+(3) close the attack vector AND keep legitimate
// long-stale entries from accumulating.
func (s *Service) recordFailedAttempt(accountID string, nowUnix int64) (remaining int, triggered bool, unlockAt int64) {
	// Mantis #1586 Option 2 gate — only track lockout for accounts
	// that actually exist on disk. accountExists is shape-validated
	// already (caller ran paths.IsValidID before reaching us); Stat
	// is path-cached after the Unlock-path Stat fired, so the
	// duplicate check is effectively free.
	//
	// On non-existent IDs we return ("0 remaining" / not triggered)
	// so the caller's bad_passphrase response is still coherent —
	// the user sees the same message a real wrong-passphrase
	// produces, but no map slot is created.
	if !s.accountExists(accountID) {
		return 0, false, 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lockouts == nil {
		s.lockouts = map[string]*lockoutEntry{}
	}
	// Mantis #1586 Option 3 — opportunistic prune of stale entries
	// before the new write. Runs under the write lock; bounded by
	// current map size so cost stays O(N) on a map already bounded
	// by (2). pruneStaleLockouts is no-op when nothing is stale.
	s.pruneStaleLockoutsLocked(nowUnix)
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

// accountExists reports whether ~/Lethean/account/<accountID>/private.key
// is present on disk. Used by recordFailedAttempt (Mantis #1586) to
// gate map growth — probe traffic against non-existent IDs MUST NOT
// allocate a lockout entry.
//
// Shape validation is the caller's responsibility (Unlock runs
// paths.IsValidID at the top of the function). This helper is
// I/O-only; on any error (root not resolvable, stat-fail) it returns
// false — fail-closed on the gate, which keeps the DoS-bounded path.
//
// Usage example:
//
//	if !s.accountExists(accountID) {
//	    return 0, false, 0 // do not grow lockout map
//	}
func (s *Service) accountExists(accountID string) bool {
	rootR := paths.Root()
	if !rootR.OK {
		return false
	}
	root, _ := rootR.Value.(string)
	privatePath := core.PathJoin(root, accountDir, accountID, privateKeyFile)
	statR := core.Stat(privatePath)
	return statR.OK
}

// pruneStaleLockoutsLocked walks s.lockouts and drops entries that
// have fully fallen out of the rolling window AND whose cooldown
// has expired. Caller MUST hold s.mu (write lock); the *Locked
// suffix mirrors Go's stdlib mutex naming convention so future
// touchers see the requirement at the call site.
//
// "Stale" means: zero attempts retained inside the rolling window
// AND no active cooldown (unlockAt has elapsed). Such entries are
// pure overhead — the next failed attempt against the same id
// would re-allocate identical state, so deletion is observably
// indistinguishable from keeping them.
//
// Mantis #1586 Option 3 — bounds map size over time without an LRU
// eviction lottery. Combined with the Option 2 gate in
// recordFailedAttempt, the map size is bounded by
//
//	min(active-failing-accounts, accounts-on-disk)
//
// at any given moment.
//
// Usage example (caller holds s.mu):
//
//	s.pruneStaleLockoutsLocked(nowUnix)
func (s *Service) pruneStaleLockoutsLocked(nowUnix int64) {
	cutoff := nowUnix - lockoutWindowSeconds
	for id, entry := range s.lockouts {
		// Active cooldown — keep so lockoutState still reports
		// "locked" until the cooldown elapses.
		if entry.unlockAt > nowUnix {
			continue
		}
		// At least one in-window attempt — keep so the counter
		// keeps accumulating toward the threshold.
		hasRecent := false
		for _, ts := range entry.attempts {
			if ts > cutoff {
				hasRecent = true
				break
			}
		}
		if hasRecent {
			continue
		}
		// No recent attempts AND no live cooldown — pure overhead.
		delete(s.lockouts, id)
	}
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
//
// Stage F.B Phase 1 — emit through the typed audit.Recorder substrate
// in addition to the human-debug core.Print echo (RFC.stage-f §1 — the
// print line remains as the human echo; the recorder receives the
// structured value). The recorder write failure is intentionally
// ignored — audit failures MUST NEVER block the auth path that emitted
// the event.
func (s *Service) auditUnlockFailed(accountID string, remaining int) {
	s.auditUnlockFailedWithReason(accountID, remaining, "")
}

// auditUnlockFailedWithReason is the Mantis #1531 reason-carrying
// sibling of auditUnlockFailed. The reason string is forensic-only —
// the user-facing response shape + timing is the same regardless of
// which sub-bucket the failure landed in, so the Meta.reason field
// gives ops a way to partition rejected attempts without the timing
// side-channel having to carry the same signal.
//
// Reason values are the const set declared in distinguishDecrypt
// (reasonBadPassphraseSentinel / reasonBadPassphrasePostGate /
// reasonCorruptedPreParser / reasonCorruptedPostBody /
// reasonCorruptedBodyRead). An empty reason ("") is the
// account-not-found and lockout-cap-already-tripped legacy paths
// which never reach distinguishDecrypt.
func (s *Service) auditUnlockFailedWithReason(accountID string, remaining int, reason string) {
	now := core.Now().UTC().Unix()
	if reason == "" {
		core.Print(core.Stderr(),
			"event=auth.unlock.failed account_id=%s ts=%d attempts_remaining=%d\n",
			accountID, now, remaining)
	} else {
		core.Print(core.Stderr(),
			"event=auth.unlock.failed account_id=%s ts=%d attempts_remaining=%d reason=%s\n",
			accountID, now, remaining, reason)
	}
	meta := map[string]any{
		"attempts_remaining": remaining,
	}
	if reason != "" {
		meta["reason"] = reason
	}
	_ = audit.Default().Record(audit.Event{
		Event:     audit.EventAuthUnlockFailed,
		AccountID: accountID,
		TS:        now,
		Scope:     "unlock",
		Outcome:   audit.OutcomeFailed,
		Meta:      meta,
	})
}

// auditLockoutTriggered emits auth.lockout.triggered per RFC §6 M4.
// Stage F.B Phase 1 — emit through audit.Recorder alongside core.Print
// (same RFC.stage-f §1 dual-emit shape as auditUnlockFailed).
func (s *Service) auditLockoutTriggered(accountID string, unlockAt int64) {
	now := core.Now().UTC().Unix()
	core.Print(core.Stderr(),
		"event=auth.lockout.triggered account_id=%s ts=%d unlock_after=%d\n",
		accountID, now, unlockAt)
	_ = audit.Default().Record(audit.Event{
		Event:     audit.EventAuthLockoutTriggered,
		AccountID: accountID,
		TS:        now,
		Exp:       unlockAt,
		Scope:     "lockout",
		Outcome:   audit.OutcomeDenied,
	})
}

// PrivateKeyHandle wraps a one-shot copy of an unlocked PGP private
// key. Callers receive the bytes only inside the closure passed to
// Use; the wrapper zeroises the backing slice when Use returns,
// regardless of whether the closure errored. A handle may be used
// AT MOST ONCE — second Use call surfaces account.private_key.not_available
// because the slice has already been zeroised + cleared.
//
// Mantis #1589 / Cerberus #18 — the previous PrivateKeyFor signature
// returned ([]byte, bool) and left zeroise to caller discipline. The
// 5 production callsites in pkg/office/mail all leaked N copies of
// the key bytes onto the heap per mail-poll cycle. The handle type
// makes zeroise structural — the only path to the bytes goes through
// Use's defer.
//
// Asymmetric scope: only PrivateKeyFor wraps. PublicKeyFor still
// returns raw bytes because the public key is not sensitive.
//
// Usage example:
//
//	handle, ok := svc.PrivateKeyFor("abc123def4567890")
//	if !ok { return errLocked }
//	err := handle.Use(func(priv []byte) error {
//		// use priv inside this closure; it is zeroised on return.
//		return nil
//	})
type PrivateKeyHandle struct {
	bytes []byte
}

// Use lets fn read the unlocked private-key bytes. Zeroises the
// backing slice on return regardless of error. Returns
// account.private_key.not_available when the handle has already
// been consumed or was nil.
//
// Usage example:
//
//	err := handle.Use(func(priv []byte) error { return s.decrypt(priv) })
func (h *PrivateKeyHandle) Use(fn func([]byte) error) error {
	if h == nil || h.bytes == nil {
		return core.NewCode("account.private_key.not_available",
			"private key handle not initialised or already consumed")
	}
	defer func() {
		for i := range h.bytes {
			h.bytes[i] = 0
		}
		h.bytes = nil
	}()
	return fn(h.bytes)
}

// NewPrivateKeyHandleForTest constructs a PrivateKeyHandle from raw
// bytes for test-double use by sibling packages. The handle takes
// ownership of the slice; Use will zeroise it on release.
//
// Production code MUST go through Service.PrivateKeyFor — this
// factory exists only so pkg/office/mail tests (and any future
// consumer's tests) can satisfy AccountProvider without reaching
// into pkg/account internals.
//
// Usage example:
//
//	handle := account.NewPrivateKeyHandleForTest(copyOfKeyBytes)
func NewPrivateKeyHandleForTest(b []byte) *PrivateKeyHandle {
	return &PrivateKeyHandle{bytes: b}
}

// PrivateKeyFor returns a single-use handle wrapping the in-memory
// unlocked PGP private key bytes for the given account_id. Returns
// (nil, false) when not unlocked. The handle zeroises the bytes on
// release (Mantis #1589 / Cerberus #18); see PrivateKeyHandle.Use.
// Used by pkg/office/mail to decrypt _accounts.enc for IMAP polling (§1.2).
//
// Usage example:
//
//	handle, ok := svc.PrivateKeyFor("abc123def4567890")
//	if !ok { return errLocked }
//	err := handle.Use(func(priv []byte) error { return decrypt(priv) })
func (s *Service) PrivateKeyFor(accountID string) (*PrivateKeyHandle, bool) {
	if accountID == "" {
		return nil, false
	}
	// Cerberus #13 / Mantis #1573 — grep-parity gate with Unlock + Lock
	// (lines 81 / 256). PrivateKeyFor reads s.unlocked[accountID] only,
	// so the disk-traversal exposure shape doesn't apply here, but the
	// lockout-map / unlocked-map can still be probed with traversal
	// strings; rejecting at the shape gate keeps the IsValidID invariant
	// uniform across every account-id-accepting Service method, which is
	// what the #1505 lint rule enforces.
	if err := paths.IsValidID(accountID); err != nil {
		_ = err // shape-only gate; callers receive the (nil, false) contract
		return nil, false
	}
	s.mu.RLock()
	priv, ok := s.unlocked[accountID]
	s.mu.RUnlock()
	if !ok {
		return nil, false
	}
	cp := make([]byte, len(priv))
	copy(cp, priv)
	return &PrivateKeyHandle{bytes: cp}, true
}

// PublicKeyFor reads the on-disk public key for the given account_id.
// Returns (nil, false) on any error. Always readable without unlock.
// Used by pkg/office/mail to encrypt _accounts.enc (§1.2).
//
// Usage example:
//
//	pub, ok := svc.PublicKeyFor("abc123def4567890")
func (s *Service) PublicKeyFor(accountID string) ([]byte, bool) {
	if accountID == "" {
		return nil, false
	}
	// Cerberus #13 / Mantis #1573 (HIGH) — mirror the Unlock (line 81)
	// + Lock (line 256) shape-validation gate. accountID is concatenated
	// directly into pubPath below; without paths.IsValidID gating, an
	// id like "../../wallets/lethean-default" lets core.ReadFile follow
	// the resolved path → measurable timing oracle (read-hit vs miss
	// path-cache deltas) and lets the bytes of any 0o600-readable file
	// on disk leak through the returned []byte. Reject at the shape
	// gate BEFORE any disk touch.
	if err := paths.IsValidID(accountID); err != nil {
		_ = err // shape-only gate; callers receive the (nil, false) contract
		return nil, false
	}
	rootR := paths.Root()
	if !rootR.OK {
		return nil, false
	}
	root := rootR.Value.(string)
	pubPath := core.PathJoin(root, accountDir, accountID, publicKeyFile)
	rawR := core.ReadFile(pubPath)
	if !rawR.OK {
		return nil, false
	}
	raw, _ := rawR.Value.([]byte)
	return raw, true
}

// UnlockedAccountIDs returns the account_ids of every currently
// unlocked account, sorted alphabetically. Replaces the previous
// DefaultAccountID() (Mantis #1588) which returned a random member
// of the unlocked map — Go map iteration is randomised per-iter, so
// callers that assumed a stable result mis-bound under multi-unlock.
//
// Callers that need exactly-one semantics MUST assert it themselves
// (see pkg/office/mail.singleUnlockedAccount — Mantis #1591 Option
// D). Returns a fresh slice; safe to mutate.
//
// Usage example:
//
//	ids := svc.UnlockedAccountIDs()
//	switch len(ids) {
//	case 0:
//	    // no account unlocked
//	case 1:
//	    // single-account path
//	default:
//	    // multi-account — caller policy
//	}
func (s *Service) UnlockedAccountIDs() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := make([]string, 0, len(s.unlocked))
	for id := range s.unlocked {
		ids = append(ids, id)
	}
	core.SliceSort(ids)
	return ids
}

// codeUnlockNeedsProvision is the error code Unlock returns when the
// on-disk private.key is the Create-marker sentinel (service.go:240 —
// hex SHA-256 of the account's public key) instead of a sealed PGP
// envelope. Frontend keys inline-copy off this code to route the user
// to the /v1/account/provision flow (or its Stage X.B successor)
// instead of the generic "run lthn account repair" copy that
// corrupted_key surfaces.
//
// Distinct from corrupted_key so the frontend can dispatch the right
// UX: corrupted_key → repair flow; needs_provision → seal flow.
// Mantis #1590 LOW / Cerberus #18.
const codeUnlockNeedsProvision = "account.needs_provision"

// isCreateMarker reports whether privBytes is the Create-time
// placeholder Service.Create writes to private.key when the account
// directory is laid down but no sealed PGP envelope has been pushed
// yet (see service.go:259 — `[]byte(core.SHA256HexString(string(input.PublicKey)))`).
//
// The marker is deterministic — hex SHA-256 of the on-disk public.key
// bytes, 64 ASCII hex chars. The check recomputes that hash and
// compares bytes; any divergence (non-marker content, different
// account's pubkey, tampered marker) falls through to the existing
// corrupted_key path so a malicious replacement can't smuggle in a
// "needs_provision" branch the lockout machinery would skip.
//
// Returns false if the public key can't be read — that branch is
// indistinguishable from "corrupted" from the consumer's POV and the
// existing corrupted_key path will handle it.
//
// Usage example:
//
//	if isCreateMarker(ciphertext, input.AccountID) {
//	    return core.Fail(core.NewCode(codeUnlockNeedsProvision, "..."))
//	}
func isCreateMarker(privBytes []byte, accountID string) bool {
	// Cheap shape pre-check — the marker is exactly 64 hex chars; any
	// other length is definitively not the marker and skips the
	// public-key disk read entirely.
	if len(privBytes) != 64 {
		return false
	}
	rootR := paths.Root()
	if !rootR.OK {
		return false
	}
	root, _ := rootR.Value.(string)
	pubPath := core.PathJoin(root, accountDir, accountID, publicKeyFile)
	pubR := core.ReadFile(pubPath)
	if !pubR.OK {
		return false
	}
	pubBytes, _ := pubR.Value.([]byte)
	expected := []byte(core.SHA256HexString(string(pubBytes)))
	// Cerberus #59 F2 / Mantis #1708 sibling — constant-time compare.
	// privBytes here is the on-disk private.key content (Create-marker
	// OR sealed PGP envelope from a prior Seal); expected is the
	// public-derived marker. Same shape as seal.go's classifier
	// comparisons; replace bytes.Equal with constantTimeBytesEqual so
	// the timing-channel discipline is uniform across the marker-vs-blob
	// classifier surface. See seal.go:constantTimeBytesEqual for the
	// banned-import exception note + the CoreGO export gap reference.
	return constantTimeBytesEqual(privBytes, expected)
}

// auditUnlockNeedsProvision emits the Create-marker distinguishing
// audit row alongside the human-debug core.Print echo. Mantis #1590
// LOW (Cerberus #18) — forensics MUST be able to tell a
// needs_provision probe apart from a corrupted_key event so an
// operator triaging "users blocked at unlock" can immediately route
// to provision-flow UX vs file-integrity remediation.
//
// SECURITY-NOTE (path-allowlist scope, Mantis #1590): a dedicated
// EventAuthUnlockNeedsProvision constant in pkg/audit/types.go would
// give the parity-grep + the Stage F log-tailer a typed surface. This
// commit lands inside the unlock.go + unlock_test.go allowlist; the
// audit-event surface change is tracked as a follow-on so this
// implementation stays scoped. For now we emit the existing
// EventAuthUnlockFailed event with a distinguishing Meta key
// (needs_provision=true) so the forensic separation lives in the
// stored row even before the typed const lands.
//
// Recorder write failure ignored — audit failures MUST NEVER block
// the auth path that emitted the event (sibling discipline to
// auditUnlockFailed at line 754).
func (s *Service) auditUnlockNeedsProvision(accountID string) {
	now := core.Now().UTC().Unix()
	core.Print(core.Stderr(),
		"event=auth.unlock.failed account_id=%s ts=%d needs_provision=true\n",
		accountID, now)
	_ = audit.Default().Record(audit.Event{
		Event:     audit.EventAuthUnlockFailed,
		AccountID: accountID,
		TS:        now,
		Scope:     "unlock",
		Outcome:   audit.OutcomeFailed,
		Meta: map[string]any{
			"needs_provision": true,
		},
	})
}

// auditLockRequested emits auth.lock.requested per RFC §6 M4 when
// the frontend / CLI explicitly Sign-Out for a session. Stage F.B
// Phase 1 — emit through audit.Recorder alongside core.Print.
func (s *Service) auditLockRequested(accountID string) {
	now := core.Now().UTC().Unix()
	core.Print(core.Stderr(),
		"event=auth.lock.requested account_id=%s ts=%d\n",
		accountID, now)
	_ = audit.Default().Record(audit.Event{
		Event:     audit.EventAuthLockRequested,
		AccountID: accountID,
		TS:        now,
		Scope:     "lock",
		Outcome:   audit.OutcomeOK,
	})
}
