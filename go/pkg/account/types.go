// SPDX-Licence-Identifier: EUPL-1.2

// Package account owns the /v1/account/create endpoint handler that
// closes the seam left open by Stage B (Mantis #1474). The endpoint is
// the only consumer of bootstrap-tokens minted by pkg/serverkey, and
// its contract is load-bearing per Cerberus #1460:
//
//   - MUST NOT overwrite an existing account (returns
//     core.NewCode("account.exists", ...) → HTTP 409).
//   - MUST recompute the account ID from the supplied public key and
//     reject if the supplied AccountID mismatches
//     (core.NewCode("account.id_mismatch", ...) → HTTP 400).
//   - MUST consume the bootstrap-token nonce BEFORE any disk write so
//     a failed write still burns the nonce (replay defence holds even
//     on partial failure).
//   - MUST write atomically. public.key + meta.json + private.key all
//     write as `<file>.tmp`, fsync, then rename. private.key renames
//     LAST so a crash mid-write leaves AccountStatus() reporting
//     has_user_account=false (per Cerberus #1471 the leaf signal is
//     private.key, not the directory).
//
// On-disk layout (Snider canon — visible, never hidden):
//
//	~/Lethean/account/<id>/public.key      armoured PGP public block. Mode 0600.
//	~/Lethean/account/<id>/private.key     armoured PGP private block. Mode 0600.
//	                                       Written LAST — leaf signal for AccountStatus.
//	~/Lethean/account/<id>/meta.json       creation metadata. Mode 0600.
//
// Account ID derivation (documented inline so future readers don't
// need to git-blame to discover the algorithm): the account ID is
// hex(SHA-256(public_key))[0:16] — first 16 hex characters of the
// SHA-256 fingerprint of the supplied public-key bytes. 64 bits of
// keyspace gives near-zero collision risk for the desktop use-case
// (one Mac = a handful of accounts, ever) while keeping the on-disk
// directory name short enough to type. The deterministic derivation
// lets the endpoint reject mismatched supplied IDs without server-
// side ID storage (Cerberus #1460 (b)).
//
// Usage example:
//
//	svc := account.NewService(c)
//	r := svc.Create(account.CreateInput{
//	    PublicKey: pubBytes,
//	    AccountID: "abc123def4567890",
//	})
//	if r.OK {
//	    out := r.Value.(account.CreateOutput)
//	    _ = out.AccountID  // canonical (recomputed) id
//	    _ = out.Path       // ~/Lethean/account/<id>/
//	}
//
// Coding constraints (same as Stage B):
//   - All errors via core.E() / core.NewCode().
//   - All I/O through core.* wrappers (no os / path/filepath / strings).
//   - Atomic writes mirror pkg/serverkey's atomicWrite helper pattern.
//   - File mode 0o600 verified at OPEN-time (Cerberus #1464 carried
//     forward from pkg/serverkey).

package account

import (
	core "dappco.re/go"
)

// CreateInput is the request body the frontend setup flow POSTs to
// /v1/account/create. PublicKey is the armoured PGP public block the
// client generated. AccountID is the client-computed ID for the same
// bytes — the server recomputes it and rejects a mismatch (Cerberus
// #1460 (b)) so a malicious client can't steer the install into a
// directory of its choosing.
//
// Usage example:
//
//	in := account.CreateInput{
//	    PublicKey: armouredPub,
//	    AccountID: "abc123def4567890",
//	}
type CreateInput struct {
	// PublicKey is the armoured PGP public-key block. Required; an
	// empty value is rejected with code "account.public_key.required".
	PublicKey []byte `json:"public_key"`

	// AccountID is the client-computed ID. Required; an empty value is
	// rejected with code "account.id.required". The server recomputes
	// the canonical ID from PublicKey and rejects mismatches with
	// "account.id_mismatch" (Cerberus #1460 (b)).
	AccountID string `json:"account_id"`

	// RequestID is the server-generated audit identifier for the
	// create call. Mirrors UnlockInput/ProvisionInput per Cerberus
	// #1524 — the gin handler DROPS any caller-supplied value and
	// overwrites with a server-side UUID v4 BEFORE invoking
	// Service.Create. Forward-contract field today (Create has no
	// audit emission yet); when an audit event lands it consumes
	// this field. Empty is acceptable for non-HTTP callers (CLI /
	// tests).
	RequestID string `json:"request_id,omitempty"`
}

// CreateOutput is the success-shape returned on a 200 OK. AccountID
// is the canonical (server-recomputed) ID — always equal to the
// requested AccountID because we reject mismatches before reaching
// the persist path. Path is the absolute on-disk directory so the
// frontend can surface "your account lives at …" copy if it wants.
//
// Usage example:
//
//	out := r.Value.(account.CreateOutput)
//	core.Print(core.Stdout(), "wrote account %s at %s\n", out.AccountID, out.Path)
type CreateOutput struct {
	AccountID string `json:"account_id"`
	Path      string `json:"path"`
}

// AccountStatus is a lightweight, JSON-shaped projection of the
// underlying serverkey.AccountStatusOutput for callers that want to
// consume it as a package-local type. Today's Stage B' shipping
// surface delegates AccountStatus() rendering to pkg/serverkey via
// the existing Wails binding, so this type exists only as a
// canonical shape for future REST exposure (the spec's §1 trigger
// flow today reads AccountStatus over the Wails binding, not REST).
//
// Usage example:
//
//	st := account.AccountStatus{HasAccount: true, AccountID: "abc123"}
//	_ = st
type AccountStatus struct {
	HasAccount bool   `json:"has_account"`
	AccountID  string `json:"account_id,omitempty"`
}

// UnlockInput is the request body the frontend sends to
// /v1/account/unlock per RFC.stage-e.md §2. Both fields are required;
// empties reject with the canonical "account.unlock.*" codes before
// any disk I/O so a malformed POST cannot consume a lockout slot.
//
// Usage example:
//
//	in := account.UnlockInput{
//	    AccountID:  "abc123def4567890",
//	    Passphrase: "user-typed",
//	}
type UnlockInput struct {
	// AccountID is the literal id under ~/Lethean/account/. Required.
	AccountID string `json:"account_id"`

	// Passphrase is the symmetric-decrypt key the user typed. Required.
	// Frontend MUST NOT log / persist / cache this value — same
	// closure-only discipline Cerberus #1465 demanded for bootstrap-
	// tokens, applied to passphrases too.
	Passphrase string `json:"passphrase"`

	// RequestID is the originating gin request-id threaded into the
	// auth.session.issued audit event on success (Cerberus #1711
	// Stage F.B Phase 2.5 punch-list). Optional — empty is acceptable
	// for non-HTTP callers (CLI / tests). When non-empty the audit
	// event carries request_id=<id> so a forensic Query can JOIN
	// session-mint events back to their HTTP request.
	RequestID string `json:"request_id,omitempty"`
}

// UnlockOutput is the success-shape returned on a 200 OK to
// /v1/account/unlock. SessionToken is the LTHN-SESS-1.* bearer the
// frontend feeds into api-fetch.ts:setSessionToken; ExpiresAt is a
// UI hint (the verifier is authoritative).
//
// Usage example:
//
//	out := r.Value.(account.UnlockOutput)
//	_ = out.SessionToken
type UnlockOutput struct {
	SessionToken string `json:"session_token"`
	ExpiresAt    int64  `json:"expires_at"`
	AccountID    string `json:"account_id"`
}

// LockInput is the request body for /v1/account/lock. The forced-
// lock endpoint clears the in-process unlocked private key for the
// supplied account_id; the next REST request without a valid session
// token re-arms the auth gate. Per RFC §6.
//
// Usage example:
//
//	in := account.LockInput{AccountID: "abc123def4567890"}
type LockInput struct {
	AccountID string `json:"account_id"`
}

// LockOutput is the success-shape returned by /v1/account/lock.
//
// Usage example:
//
//	out := r.Value.(account.LockOutput)
//	_ = out.AccountID
type LockOutput struct {
	AccountID string `json:"account_id"`
}

// SessionTokenIssuer is the narrow contract Unlock consumes from
// pkg/serverkey for session-token issuance. Decoupled so account_test
// can inject an in-process fake without depending on the full
// serverkey.Service (which would require Bootstrap() + a real PGP
// key pair under $HOME).
//
// Usage example:
//
//	svc.SetServerKey(serverkeySvc) // *serverkey.Service satisfies this
type SessionTokenIssuer interface {
	// IssueSessionToken mints a session token authorising accountID.
	// Pre-Stage-F.B-Phase-2.5 surface — kept on the interface for
	// callers (CLI / tests) that don't carry a gin request-id.
	IssueSessionToken(accountID string) core.Result

	// IssueSessionTokenWithRequest mints a session token AND threads
	// the originating gin request-id into the auth.session.issued
	// audit event Meta so forensic Query can join session-mint events
	// to their originating HTTP request (Cerberus #1711 Stage F.B
	// Phase 2.5 punch-list). Empty requestID is acceptable — the
	// audit event simply omits the field.
	IssueSessionTokenWithRequest(accountID, requestID string) core.Result
}

// ProvisionInput is the request body the frontend sends to
// POST /v1/account/provision per RFC.stage-x.md §2.1. Passphrase is
// required; empty rejects with account.provision.passphrase.required
// BEFORE any keygen so a malformed POST cannot consume the expensive
// keygen path. RequestID is the client-generated UUID v4 for the
// §3.2 idempotency dedup window — Phase 2a accepts an empty value
// (dedup ships in Phase 2c).
//
// Usage example:
//
//	in := account.ProvisionInput{
//	    Passphrase: "correct horse battery staple",
//	    RequestID:  "8b1d4a3e-0a4c-4f6e-9c4e-2a3b9d6f8e1c",
//	}
type ProvisionInput struct {
	Passphrase string `json:"passphrase"`
	RequestID  string `json:"request_id"`
}

// ProvisionOutput is the success-shape returned on a 200 OK from
// /v1/account/provision. SessionToken mirrors UnlockOutput's shape so
// the auth-gate frontend handles both responses with one branch.
// DedupHit ships false in Phase 2a (idempotency dedup is Phase 2c);
// the field is reserved here so callers can rely on its presence
// today without a contract change later.
//
// Usage example:
//
//	out := r.Value.(account.ProvisionOutput)
//	setSessionToken(out.SessionToken)
type ProvisionOutput struct {
	AccountID    string `json:"account_id"`
	Path         string `json:"path"`
	SessionToken string `json:"session_token"`
	ExpiresAt    int64  `json:"expires_at"`
	DedupHit     bool   `json:"dedup_hit,omitempty"`
}

// lockoutEntry tracks per-account_id failed-unlock state per RFC §5
// M1 (Cerberus DREAD ruling — counter is per-account, NOT global).
// attempts holds the unix-seconds of each failed attempt inside the
// rolling lockoutWindowSeconds; unlockAt > now means the account is
// currently locked.
type lockoutEntry struct {
	attempts []int64
	unlockAt int64
}

// File modes — declared at package scope so service.go / routes.go
// all reference the same value. 0o600 is load-bearing for Cerberus
// #1464 (open-time mode verification carried forward).
const (
	dirMode  core.FileMode = 0o700
	fileMode core.FileMode = 0o600
)

// File names — declared at package scope so service.go and any
// future writers stay aligned. private.key MUST be written last per
// Cerberus #1471 (it is the leaf signal AccountStatus reads).
const (
	publicKeyFile  = "public.key"
	privateKeyFile = "private.key"
	metaFile       = "meta.json"
)

// Container directory under ~/Lethean/. Mirrors pkg/serverkey's
// accountDir constant so callers writing through both packages stay
// in lockstep.
const accountDir = "account"

// accountIDLength is the byte-length (hex chars) of the canonical
// account ID — first 16 hex chars of SHA-256(public_key). 64 bits
// of keyspace for the desktop use-case (one Mac, handful of accounts
// ever). Trimming early keeps the directory name short enough to
// type while leaving no collision risk in practice.
const accountIDLength = 16

// Error codes — declared at package scope so tests, handlers, and
// future loaders consume the same literal. The "account.*" namespace
// is the public contract the frontend keys error UI off (e.g. the
// auth-gate's "account already exists" branch reads error.code ==
// "account.exists"). Adding a new code requires updating the
// frontend's known-codes set in the same commit.
const (
	codeAccountExists      = "account.exists"
	codeAccountIDMismatch  = "account.id_mismatch"
	codeAccountIDRequired  = "account.id.required"
	codePublicKeyRequired  = "account.public_key.required"
	codeAccountWriteFailed = "account.write_failed"
	codeAccountInvalidBody = "account.invalid_body"

	// Stage E.B per RFC.stage-e.md §5 — unlock failure namespace.
	// The frontend keys inline-copy off these codes (bad_passphrase
	// renders "N attempts remaining"; locked_out renders the cooldown
	// countdown; corrupted_key renders the repair instruction).
	codeUnlockBadPassphrase      = "account.unlock.bad_passphrase"
	codeUnlockLockedOut          = "account.unlock.locked_out"
	codeUnlockCorruptedKey       = "account.unlock.corrupted_key"
	codeUnlockPassphraseRequired = "account.unlock.passphrase.required"
	codeUnlockMisconfigured      = "account.unlock.server_misconfigured"
	codeUnlockSessionMintFailed  = "account.unlock.session_mint_failed"

	// Stage X.B per RFC.stage-x.md §2.1 — server-side provision
	// (keygen + encrypt + write + session-token) failure namespace.
	// Frontend keys inline-copy off these codes (passphrase.required
	// renders "Enter a passphrase"; keygen_failed flips to error
	// state; session_mint_failed mirrors the unlock-side variant so
	// the same UX branch handles both).
	codeProvisionPassphraseRequired = "account.provision.passphrase.required"
	codeProvisionKeygenFailed       = "account.provision.keygen_failed"
	codeProvisionEncryptFailed      = "account.provision.encrypt_failed"
	codeProvisionMisconfigured      = "account.provision.server_misconfigured"
	codeProvisionSessionMintFailed  = "account.provision.session_mint_failed"
)

// Lockout policy constants per RFC.stage-e.md §5 + Cerberus DREAD M1.
//
// lockoutThreshold — failed attempts inside the rolling window
// before the account_id is locked.
// lockoutWindowSeconds — sliding window over which attempts count.
// lockoutCooldownSeconds — how long the account stays locked after
// crossing the threshold.
//
// 5 / 300s / 60s matches the RFC's "5 failed attempts within 5
// minutes → lock the account_id for 60 seconds". Tunable at the
// package level so the test fixture can pin the values without
// mocking the clock.
const (
	lockoutThreshold        = 5
	lockoutWindowSeconds    = 5 * 60
	lockoutCooldownSeconds  = 60
)
