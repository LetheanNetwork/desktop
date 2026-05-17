// SPDX-Licence-Identifier: EUPL-1.2

// Package audit owns the write-path of the Stage F audit-log substrate
// per plans/code/lthn/desktop/auth-gate/RFC.stage-f.md (v2).
//
// Phase 1 scope (the substrate landed in this commit):
//
//   - Event struct — typed forensic-record shape per RFC §2
//   - Recorder interface — sink contract that lets the package wire
//     against an in-memory test fixture without spinning the NDJSON
//     file sink
//   - NDJSON file sink writing to ~/Lethean/audit/<YYYY-MM-DD>.ndjson
//     per RFC §4 — append-only, atomic per-record, file-per-day
//   - Secret-shape redactor per RFC §2.1.1 — refuses to persist any
//     event whose Meta values match a token-prefix / Bearer-prefix /
//     long-base64-like recogniser
//
// Phase 1 does NOT yet ship (deferred to Phase 2 + Phase 3 per the
// dispatch brief):
//
//   - REST surface (GET /v1/audit/events) and the Query path
//   - Frontend Activity-view consumer
//   - Typed cross-package buses (tasks/queue/pipeline/incidents) +
//     parity-grep contract from RFC §3.3.1
//   - HMAC-hashed account_id at-rest (RFC §6.4)
//   - Rotation goroutine + compression + .candidate retention marker
//   - lthn-mlx separate-Service plumbing (RFC §6.5)
//
// Cross-cut consumers (Stage E + Stage X auth events) live in their
// own packages and call Recorder directly via the package-level Default
// sink the boot path wires once. Phase 1's recorder is constructed in
// each package's existing audit helper (see pkg/account/unlock.go +
// pkg/serverkey/token.go + pkg/account/provision.go).
//
// Coding constraints (per AX-6 + RFC §10):
//
//   - All I/O via core.* wrappers (no os / path/filepath / encoding/json)
//   - All errors via core.E() / core.NewCode()
//   - File mode 0o600 + dir mode 0o700 (matches ~/Lethean/wallets/
//     discipline)
//   - Newline-delimited JSON, never pretty-printed (one Event per line)

package audit

import (
	core "dappco.re/go"
)

// Event is the typed forensic record persisted to ~/Lethean/audit/.
// Fields follow RFC §2 verbatim. Phase 1 uses map[string]any for Meta
// (CoreGO has no core.Map alias today — RFC v2's schema names it
// core.Map; we adopt the equivalent stdlib-shaped value, untyped per
// RFC §2.2). The on-disk JSON is the contract; the Go field tags are
// the contract's keys.
//
// Usage example:
//
//	ev := audit.Event{
//	    Event:     "auth.session.issued",
//	    AccountID: "abc123def4567890",
//	    TS:        core.Now().UTC().Unix(),
//	    Exp:       core.Now().UTC().Add(15 * core.Minute).Unix(),
//	    Scope:     "session",
//	    Outcome:   "ok",
//	}
//	_ = audit.Default().Record(ev)
type Event struct {
	// Event is the dotted event name, e.g. "auth.session.issued" /
	// "auth.unlock.failed" / "auth.account.provisioned". Reserved schema
	// — the Stage F spec pins these literals; renaming a name without a
	// spec bump breaks any downstream log-tailer.
	Event string `json:"event"`

	// AccountID is the account this event pertains to. Empty for system
	// events. RFC §6.4 mandates HMAC-hashing at rest; Phase 1 stores the
	// raw account_id (a Phase 2 sweep wires the HMAC + flips this).
	AccountID string `json:"account_id,omitempty"`

	// TS is the unix-seconds timestamp at recorder-receive time. Per
	// RFC §6.2 this is order-of-receipt, not order-of-occurrence.
	TS int64 `json:"ts"`

	// Exp is the unix-seconds timestamp the event references (e.g.
	// session-token expiry). Zero when not applicable.
	Exp int64 `json:"exp,omitempty"`

	// Scope is the categorical label for the event-family (e.g.
	// "session" / "unlock" / "account.create"). Used by the future
	// Operations panel's facet chrome.
	Scope string `json:"scope,omitempty"`

	// Outcome is one of: ok | failed | denied | error. Closed set so
	// the Operations panel's outcome-dropdown stays stable across
	// future event additions.
	Outcome string `json:"outcome,omitempty"`

	// RequestID correlates the event with the gin-request that emitted
	// it. Empty for non-HTTP origins.
	RequestID string `json:"request_id,omitempty"`

	// Meta carries free-form per-event fields. RFC §2.1 — Meta MUST
	// NEVER contain token bytes / passphrases / decrypted private-key
	// material / raw Authorization header content. The secret-shape
	// detector in redact.go runs over every string value (recursively)
	// and refuses to persist matching content.
	Meta map[string]any `json:"meta,omitempty"`
}

// Outcome enum literals — declared as named constants so callers stay
// in lockstep with the closed set the Operations panel renders. Adding
// a new outcome is a reserved-schema change; the frontend dropdown
// MUST learn the new literal in the same commit.
const (
	OutcomeOK     = "ok"
	OutcomeFailed = "failed"
	OutcomeDenied = "denied"
	OutcomeError  = "error"
)

// Event-name literals reserved by Stage E + Stage F + Stage X. Declared
// at package scope so callers stay in lockstep with the parity-grep
// contract that Phase 2 lands (RFC §3.3.1) and so a typo in a caller's
// emit-site fails at compile rather than silently writing a new event
// name the audit-tail can't recognise.
//
// New event-name additions are reserved-schema changes — the
// Operations panel + Stage F log-tailer learn them in the same commit.
const (
	EventAuthUnlockFailed       = "auth.unlock.failed"
	EventAuthLockoutTriggered   = "auth.lockout.triggered"
	EventAuthLockRequested      = "auth.lock.requested"
	EventAuthSessionIssued      = "auth.session.issued"
	EventAuthSessionVerifyFailed = "auth.session.verify_failed"
	EventAuthAccountProvisioned = "auth.account.provisioned"

	// EventAuthAccountCreated fires when pkg/account.Service.Create
	// successfully lands a new account on disk via the
	// /v1/account/create bootstrap-auth endpoint (Mantis #1574 / Cerberus
	// #13). Sibling of EventAuthAccountProvisioned — Create writes the
	// public-key + meta + private-key triple in the legacy two-step
	// onboarding flow, Provision in the consolidated one-shot keygen +
	// session-issue flow per RFC.stage-x.md §3.
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   path_hash — SHA-256 hex of the canonical account directory path
	//   account_id — already in AccountID; not duplicated in Meta
	//
	// The raw canonical pubkey bytes are NEVER in Meta — Cerberus #1465
	// closure-only-scope discipline keeps the bytes off the audit
	// substrate; this event records the create-success decision, not
	// the credential.
	EventAuthAccountCreated = "auth.account.created"

	// EventAuthAccountSealed fires when pkg/account.Service.Seal
	// successfully replaces the Create-time marker at
	// ~/Lethean/account/<id>/private.key with a user-encrypted PGP
	// envelope via PUT /v1/account/:id/seal (Stage E.A per
	// plans/code/lthn/desktop/auth-gate/RFC.stage-e-seal.md v2 / Mantis
	// #1610 / Mantis #1631 H#141 follow-up). Seal-once invariant — the
	// audit row marks the MARKER → SEALED transition and is the join-key
	// for the future Stage F log-tailer's seal-event parity-grep.
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   path_hash — SHA-256 hex of the canonical account directory path
	//   version   — sealed-envelope schema version (Stage E.A v2: always 1)
	//
	// The canonical encrypted blob is NEVER in Meta per Cerberus #1465
	// closure-only-scope discipline; this event records the seal-success
	// decision, not the credential.
	EventAuthAccountSealed = "auth.account.sealed"

	// EventAuthAccountSealFailed fires when pkg/account.Service.Seal
	// rejects a seal attempt for any reason (validation, state-conflict,
	// version-unsupported, write-failure). Sibling of EventAuthAccountSealed
	// per Stage F.B Phase 1 dual-emit shape.
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   reason — categorical failure reason matching the Service.Seal
	//            recordSealFailure / auditSealFailed taxonomy
	//            (blob_required / blob_invalid / version_unsupported /
	//            not_found / already_sealed / write_failed)
	//
	// Lockout-tick discipline lives at the caller (recordSealFailure
	// runs BEFORE this emit when applicable per Cerberus #25 ADD-MED-2);
	// the audit row reflects the freshly-decremented counter.
	EventAuthAccountSealFailed = "auth.account.seal_failed"

	// EventPluginViewCapabilityGranted fires when a plugin's iframe
	// view receives a capability via the host-side broker's postMessage
	// handshake per plans/code/lthn/desktop/views/RFC.plugin-views.md
	// §5.1. Frontend broker (api-fetch.ts grantTokenToFrame) POSTs
	// /v1/plugin-view/capability-grant BEFORE delivering the token
	// bytes to the iframe so the audit row is committed first; if the
	// audit emit fails the broker must NOT proceed with postMessage
	// (Mantis #1523 + Mantis #1576).
	//
	// Meta keys per RFC.plugin-view-audit-atomicity.md v2 (Mantis #1576,
	// target_version v1.0.0-beta.1) — Option A: ONE row per broker call:
	//
	//   plugin_id      — installed plugin code that owns the iframe view
	//   capabilities   — array of brokered scope literals (e.g.
	//                    ["session-token"]). Pre-v2 scalar `capability`
	//                    field is REJECTED at the handler (400) per
	//                    §5(a) hard-cutover discipline.
	//   origin         — the iframe's loopback origin (per-port allowlist)
	//   outcome        — reserved literal "granted" in v1; any other
	//                    value rejected with 400 per §3.1.
	//   correlation_id — handler-generated UUIDv4 disambiguating
	//                    near-simultaneous grants for the same
	//                    (plugin_id, origin, capability) tuple within
	//                    one NDJSON day file. Request body MUST NOT
	//                    carry a caller-asserted value (handler is the
	//                    sole authority per §3.1).
	//
	// The token BYTES are NEVER in Meta — only the capability literal.
	// The Cerberus #1465 closure-only-scope discipline keeps the bytes
	// off the audit substrate; this event records the grant decision,
	// not the credential.
	EventPluginViewCapabilityGranted = "plugin.view.capability_granted"
)

// Error codes the package emits via core.NewCode. Mirrors the
// pkg/account / pkg/serverkey discipline — the "audit.*" namespace is
// reserved; callers pattern-match on the literal when categorising
// failures.
const (
	codeAuditRecordRedacted = "audit.record.redacted"
	codeAuditOpenFailed     = "audit.open.failed"
	codeAuditWriteFailed    = "audit.write.failed"
	codeAuditMarshalFailed  = "audit.marshal.failed"
	codeAuditEventInvalid   = "audit.event.invalid"
)

// File + directory mode discipline — matches ~/Lethean/wallets/ +
// ~/Lethean/account/<id>/. Audit records are forensic + may carry
// per-account context; 0o600 keeps a same-user-different-process
// reader from skimming them.
const (
	dirMode  core.FileMode = 0o700
	fileMode core.FileMode = 0o600
)

// Root subdirectory under ~/Lethean/ where Phase 1 writes the day-
// rolled NDJSON files. RFC §4.1 names this verbatim.
const auditDir = "audit"
