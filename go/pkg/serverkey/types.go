// SPDX-Licence-Identifier: EUPL-1.2

// Package serverkey owns the "base egg" PGP key the lthn binary
// generates on first start — the cryptographic root that signs
// short-lived bootstrap tokens for the account-creation endpoint
// family. See plans/code/lthn/desktop/auth-gate/RFC.md (v2) §2.
//
// On-disk layout (Snider canon — visible, never hidden):
//
//	~/Lethean/wallets/server.key   PGP key blob (public + symmetric-encrypted
//	                               private). Mode 0600.
//	~/Lethean/wallets/.seed        32 random bytes. The unique-per-machine
//	                               secret HKDF-derives the symmetric pass-
//	                               phrase from. Mode 0600.
//
// Lifecycle:
//
//	r := serverkey.NewService(c).Bootstrap()
//	if !r.OK { return r }                       // server-key live in memory
//	tokR := svc.IssueBootstrapToken()           // 60s lifetime
//	verifyR := svc.VerifyBootstrapToken(tok, "account.create")
//
// The middleware in pkg/server/bootstrap_auth.go consumes Verifier
// (the interface this package satisfies) to gate the account-creation
// endpoint family. The Wails surface (wails.go) exposes AccountStatus
// + IssueBootstrapToken to the frontend gate (Stage C).
//
// Coding constraints:
//   - All errors via core.E() / core.NewError.
//   - All I/O through core.* wrappers (no os / path/filepath / etc).
//   - JSON via core.JSONMarshal* / core.JSONUnmarshal*.
//   - File mode 0o600 verified at OPEN-time (not just write-time —
//     Cerberus #1464).

package serverkey

import (
	core "dappco.re/go"
)

// Service owns the server-key lifecycle + bootstrap-token mint/verify
// pipeline. Construct via NewService; call Bootstrap once at boot
// before the HTTP server starts listening. Subsequent calls on the
// same process re-use the in-memory keys.
//
// Usage example:
//
//	svc := serverkey.NewService(c)
//	if r := svc.Bootstrap(); !r.OK { return r }
//	tokR := svc.IssueBootstrapToken()
type Service struct {
	core *core.Core

	// mu guards the in-memory key cache + the consumed-nonce set.
	// Held during Bootstrap's stat/generate/decrypt sequence so two
	// goroutines in the same process can't race the file-load path.
	mu core.RWMutex

	// publicKey + privateKey are populated by Bootstrap. publicKey is
	// the armoured PGP public block; privateKey is the decrypted
	// armoured private block held only in memory (never written to
	// disk in cleartext).
	publicKey  []byte
	privateKey []byte

	// processSalt randomises consumed-nonce-set keys per process
	// (Cerberus #1463 — defends a memory-read attacker from learning
	// the actual nonces in flight). Set once during NewService via
	// core.RandomBytes(32) so it persists across Bootstrap calls.
	processSalt []byte

	// consumedNonces tracks nonces that have already been redeemed.
	// Keyed by hex(HMAC(processSalt, nonce-bytes)) — never the raw
	// nonce. Entries expire on next Verify call past now-verifierTTL.
	consumedNonces map[string]core.Time

	// consumedSessionNonces tracks per-issue session-token nonce
	// uniqueness — disjoint from consumedNonces so a bootstrap nonce
	// can't fake-collide with a session nonce. Per RFC §3.1, session-
	// token Verify does NOT add to this set (session tokens are
	// consumed N times per session). Issue DOES add — defends against
	// an internal mint loop yielding duplicate nonces. Entries expire
	// on next Issue call past now-sessionVerifierTTL.
	consumedSessionNonces map[string]core.Time

	// singleInstanceCheck overrides the default CoreGUI single-instance
	// probe in tests. Production wires nil; the Bootstrap call falls
	// back to the OpenFile(O_EXCL) sentinel lock for the bootstrap
	// critical section because CoreGUI's SingleInstance only kicks in
	// after application.New().Run() — which is AFTER newAppCore
	// returns. See Cerberus #1466.
	singleInstanceCheck func() bool
}

// Verifier is the interface pkg/server consumes from serverkey to gate
// bootstrap-token-authenticated endpoints. The concrete *Service
// satisfies it; tests inject in-memory fakes.
//
// Usage example:
//
//	srv := server.NewService(server.Options{ServerKey: svc, ...})
type Verifier interface {
	// VerifyBootstrapToken returns OK iff token parses, signature
	// verifies against the in-memory server public key, the embedded
	// scope claim equals wantScope, the token is inside the TTL
	// ceiling, and the nonce has not been consumed. On OK the nonce is
	// added to the consumed set before this call returns.
	//
	// On any failure returns core.Fail with a code from the
	// "auth.bootstrap.*" namespace so the middleware can shape an
	// error envelope without leaking which check tripped.
	VerifyBootstrapToken(token, wantScope string) core.Result

	// VerifySessionToken returns OK iff token parses with the
	// LTHN-SESS-1. prefix, signature verifies against the in-memory
	// server public key, scope == "session", and the embedded TTL +
	// verifier ceiling are intact. On OK Result.Value is the
	// SessionVerifyOutput shape carrying the account_id claim. On
	// failure returns core.Fail with a code from "auth.session.*".
	//
	// Per RFC §3.1 the nonce is NOT added to the consumed set on
	// verify (session tokens are consumed N times per session). Replay
	// defence relies on TTL + closure-only frontend storage.
	//
	// Usage example:
	//
	//	r := svc.VerifySessionToken(token)
	//	if !r.OK { /* 401 */ }
	//	out := r.Value.(serverkey.SessionVerifyOutput)
	//	_ = out.AccountID
	VerifySessionToken(token string) core.Result
}

// AccountStatusOutput is the Wails-binding shape consumed by the
// frontend auth-gate to derive `setup` vs `auth` state. Cerberus #1471
// — `HasUserAccount` is true iff at least one ~/Lethean/account/<id>/
// private.key file exists. Directory presence ALONE is not the signal
// (a partial account directory created-then-deleted strands the gate
// in `auth` with nothing to unlock).
//
// Usage example (TS):
//
//	import { AccountStatus } from "@desktop/serverkey/service";
//	const s = await AccountStatus();
//	if (!s.has_user_account) showSetupGate();
type AccountStatusOutput struct {
	HasUserAccount bool `json:"has_user_account"`
}

// SessionTokenOutput wraps the freshly-minted session token + its
// issuance metadata for the unlock-flow handler. Same closure-only
// storage discipline as BootstrapTokenOutput per RFC §3.2 — NEVER
// localStorage, NEVER sessionStorage, NEVER cookie. App restart =
// re-unlock.
//
// Usage example:
//
//	r := svc.IssueSessionToken(accountID)
//	if r.OK { out := r.Value.(SessionTokenOutput); _ = out.Token }
type SessionTokenOutput struct {
	Token     string `json:"session_token"`         // LTHN-SESS-1.<header>.<sig>
	ExpiresAt int64  `json:"expires_at"`            // unix seconds — for UI hints only; verifier is authoritative
	AccountID string `json:"account_id"`            // the unlocked account_id this token authorises
}

// SessionVerifyOutput is the success-shape Verifier.VerifySessionToken
// returns inside core.Result.Value. AccountID is the literal value of
// the embedded `account_id` claim after signature verification — the
// bearer-auth middleware writes it to c.Set("account_id", ...) so
// downstream handlers can scope per-account reads/writes without
// re-parsing the token.
//
// Usage example:
//
//	r := svc.VerifySessionToken(token)
//	if r.OK {
//	    out := r.Value.(serverkey.SessionVerifyOutput)
//	    _ = out.AccountID
//	}
type SessionVerifyOutput struct {
	AccountID string
}

// BootstrapTokenOutput wraps the freshly-minted token + its issuance
// metadata for the frontend gate. The token itself MUST live only in
// the closure scope of the click handler that consumes it (Cerberus
// #1465); never persisted to storage.
//
// Usage example (TS):
//
//	import { IssueBootstrapToken } from "@desktop/serverkey/service";
//	const { token } = await IssueBootstrapToken();
//	const res = await fetch("/v1/account/create", {
//	    method: "POST",
//	    headers: { "Authorization": "Bootstrap " + token },
//	    body: JSON.stringify(input),
//	});
type BootstrapTokenOutput struct {
	Token     string `json:"token"`      // LTHN-BOOT-1.<header>.<sig>
	ExpiresAt int64  `json:"expires_at"` // unix seconds — for UI hints only; verifier is authoritative
}

// CreateAccountInput is the payload the frontend setup flow sends to
// the /v1/account/create endpoint. Stage B owns the serverkey-level
// shape; the account-creation endpoint handler (separate ticket) will
// validate + execute the on-disk write per RFC §2.5.
//
// Stage B intentionally does NOT ship the endpoint handler itself —
// that lives in cmd/lthn or pkg/server and is the next ticket once
// the bootstrap-token plumbing is live.
type CreateAccountInput struct {
	// Passphrase the user enters to encrypt their PGP private key.
	// Required; empty rejected with "account.passphrase.required".
	Passphrase string `json:"passphrase"`
	// DisplayName is the human-readable account label (UI only — does
	// NOT bind to the account ID). Optional.
	DisplayName string `json:"display_name,omitempty"`
}

// File modes — declared at package scope so token.go / service.go /
// wails.go all reference the same value. 0o600 is load-bearing for
// Cerberus #1464 (open-time mode verification).
const (
	dirMode  core.FileMode = 0o700
	fileMode core.FileMode = 0o600
)

// File names — declared at package scope so paths.go / token.go agree.
const (
	serverKeyFile = "server.key"
	seedFile      = ".seed"
	bootstrapLock = ".bootstrap.lock"
)

// HKDF parameters — declared at package scope so derivation is the
// same across every call site. salt is the domain-separation anchor
// for future contexts; info is the per-use sub-context.
var (
	hkdfSalt = []byte("lthn/serverkey/v1")
	hkdfInfo = []byte("passphrase")
)

// bootstrapAllowedScopes is the closed set of scopes
// IssueBootstrapTokenForScope will mint. Per Cerberus #1467 path/
// scope lockstep — adding a scope here is a security-policy decision
// that requires a new Mantis ticket + DREAD review (the matching
// pathScopes entry in pkg/server.BootstrapPathScopes is the other
// half of the lockstep).
//
// Stage E.B adds account.unlock alongside account.create — the only
// two scopes bootstrap-tokens authorise today.
var bootstrapAllowedScopes = map[string]bool{
	scopeAccountCreate: true,
	scopeAccountUnlock: true,
}

const (
	// seedSize is the byte-length of ~/Lethean/wallets/.seed. 32 bytes
	// = 256 bits of entropy fed into HKDF-SHA256.
	seedSize = 32

	// passphraseSize is the symmetric-passphrase derivation length —
	// matches the SymmetricallyEncrypt input contract.
	passphraseSize = 32

	// processSaltSize sizes the per-process HMAC salt for nonce-set
	// keying (Cerberus #1463). 32 bytes for SHA-256 alignment.
	processSaltSize = 32

	// nonceSize is the byte-length of the bootstrap-token nonce
	// claim. 16 bytes hex-encoded → 32 ASCII characters.
	nonceSize = 16

	// issuerTTL is the lifetime stamped onto fresh tokens
	// (Cerberus #1462).
	issuerTTL = 60

	// verifierTTL is the absolute ceiling the verifier enforces
	// regardless of `exp` (Cerberus #1462 — defends a future spec
	// change that lengthens `exp` without re-reviewing the verifier).
	verifierTTL = 120

	// clockSkewTolerance permits up to 5s of issuer-clock-ahead skew
	// before rejecting `iat > now` (Cerberus #1462).
	clockSkewTolerance = 5

	// tokenPrefix identifies the bootstrap-token version. Future
	// revisions bump the suffix integer.
	tokenPrefix = "LTHN-BOOT-1."

	// sessionTokenPrefix identifies the session-token version
	// (Stage E.B per RFC §3). LTHN-SESS-1. so the bearer-auth
	// middleware can dispatch on prefix without parsing the body.
	sessionTokenPrefix = "LTHN-SESS-1."

	// scopeAccountCreate is the only bootstrap-token scope Stage B
	// mints. Per Cerberus #1467 — adding a new scope means a new
	// Mantis ticket AND a new Cerberus DREAD review.
	scopeAccountCreate = "account.create"

	// scopeAccountUnlock is the bootstrap-token scope minted for the
	// unlock-flow precondition (Stage E.B per RFC §2.2 / §4 — the
	// /v1/account/unlock POST is gated by a fresh bootstrap-token with
	// this scope so the same single-use + signed-by-server-key
	// discipline as account-create applies to unlock).
	scopeAccountUnlock = "account.unlock"

	// scopeSession is the session-token scope embedded in the
	// LTHN-SESS-1. header. Per RFC §3.1 + Cerberus #1467 scope-
	// laundering defence — a token minted with scope=session cannot
	// satisfy a bootstrap-tier gate, and a bootstrap token cannot
	// satisfy a session-tier gate.
	scopeSession = "session"

	// sessionIssuerTTL is the lifetime stamped onto fresh session
	// tokens per RFC §6 — 15 minutes, chosen as the desk-leave
	// threshold (the load-bearing question is "how long until an
	// attacker who acquires the token at moment T can still use it",
	// NOT "how long can the user work without re-unlocking").
	sessionIssuerTTL = 15 * 60

	// sessionVerifierTTL is the absolute verifier ceiling per RFC §6
	// — 30 minutes (2× sessionIssuerTTL, mirrors the bootstrap
	// 60s/120s pattern). Defends a future spec change that lengthens
	// `exp` without re-reviewing the verifier.
	sessionVerifierTTL = 30 * 60

	// accountKeyFile is the leaf-file Cerberus #1471 designates as
	// the "account is real" signal — directory presence alone is not
	// enough. Per-account path is
	// ~/Lethean/account/<id>/private.key.
	accountKeyFile = "private.key"

	// accountDir is the on-disk namespace under ~/Lethean/.
	accountDir = "account"
)
