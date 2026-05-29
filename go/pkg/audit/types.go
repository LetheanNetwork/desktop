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

// MetaKey* literals — the reserved keys the substrate writes into
// Event.Meta. Exported as named constants so callsites and the TS
// mirror (frontend/src/lit/obs/audit-constants.ts AUDIT_META_KEYS)
// stay in lockstep, and so a forensic walker can grep on a single
// canonical literal rather than the bare "error_code" string.
//
// Mantis #1720 v2 dual-key shape — MetaKeyErrorCode answers WHY
// (canonical Lethean codespace, sourced via audit.ErrorCode), and
// MetaKeyErrorScope answers WHAT (operation-scope boundary, sourced
// via audit.ErrorScope). See pkg/audit/errorcode.go canonical
// codespace registry for the closed set of per-package prefixes.
//
// Canonical emit shape (every Failed callsite per the dual-key spec):
//
//	_ = audit.Default().Record(audit.Event{
//	    Event:   audit.EventMarketplaceInstallFailed,
//	    Outcome: audit.OutcomeFailed,
//	    Meta: map[string]any{
//	        audit.MetaKeyErrorCode:  audit.ErrorCode(r),
//	        audit.MetaKeyErrorScope: audit.ErrorScope(r),
//	    },
//	})
const (
	MetaKeyErrorCode  = "error_code"
	MetaKeyErrorScope = "error_scope"
)

// Event-name literals reserved by Stage E + Stage F + Stage X. Declared
// at package scope so callers stay in lockstep with the parity-grep
// contract that Phase 2 lands (RFC §3.3.1) and so a typo in a caller's
// emit-site fails at compile rather than silently writing a new event
// name the audit-tail can't recognise.
//
// New event-name additions are reserved-schema changes — the
// Operations panel + Stage F log-tailer learn them in the same commit.
//
// error_code substrate contract (RFC.error-code-cascade.md, Mantis #1718
// W1–W7 cascade complete 2026-05-17):
//
// EVERY Failed-event Meta["error_code"] in this catalogue MUST be sourced
// via the audit.ErrorCode(r core.Result) substrate at pkg/audit/errorcode.go.
// The substrate returns a bounded keyspace — r.Code() OR (*core.Err).Operation
// OR the canonical "unknown_error" fallback — so the chip-filter UI and the
// forensic log-tailer can group failures by a stable, finite vocabulary.
//
//   // Canonical emit shape (every Failed callsite):
//   _ = audit.Default().Record(audit.Event{
//       Event:   audit.EventMarketplaceInstallFailed,
//       Outcome: audit.OutcomeFailed,
//       Meta:    map[string]any{"error_code": audit.ErrorCode(r)},
//   })
//
// NEVER pass r.Error() / err.Error() / r.Value.(error).Error() directly to
// Meta["error_code"]. Raw error prose routinely echoes caller-controlled
// input (URLs, file paths, prompts, Authorization bytes) and defeats the
// bounded-keyspace promise the substrate exists to enforce. The pkg/audit
// secret-shape redactor is token-shaped, not prose-shaped — it is not a
// substitute for upstream-of-emit code-derivation.
//
// References:
//
//   - pkg/audit/errorcode.go — canonical substrate + Usage example
//   - plans/code/lthn/desktop/audit/RFC.error-code-cascade.md — full cascade
//     design, FOLD-1 canonical-fallback ruling, Cerberus #62 DREAD log
//
// Requested-emit ordering canon (Mantis #1704 — established 2026-05-17):
//
// The Requested row MUST commit BEFORE any policy gate (tier-auth, host-
// allowlist, lockout-state, rate-limit, capability-resolver, image-allow).
// Rationale — a probe attempt that the gate denies still leaves a forensic
// record of caller intent; if Requested fired POST-gate, a denied request
// would be invisible to the auditor and we lose the attack-walk surface
// that the audit substrate exists to expose (Cerberus #45 / Mantis #1658
// Shape A surfacing).
//
//   - Requested  — emitted at top of handler, AFTER input parsing succeeds,
//                  BEFORE any policy gate. Outcome: ok.
//   - Rejected   — emitted if a policy gate denies. Outcome: denied.
//                  Carries Meta["reason"] from the package's closed-set.
//   - Succeeded  — emitted post-handler on the OK path. Outcome: ok.
//   - Failed     — emitted post-handler on the non-OK path. Outcome: failed
//                  (or error for an internal/transport class). Carries
//                  Meta["error_code"] sourced via audit.ErrorCode(r).
//
// EventTierReject is the substrate-level Rejected emit for tier-auth gates
// (Mantis #1758) — it fires BEFORE the per-package Rejected helper would,
// because tier-auth is the outermost gate. Per-package Rejected helpers
// fire for package-internal gates (host-allowlist, image-allowlist, etc.).
//
// Existing adopters honouring this canon: pkg/runner (Generate/Chat),
// pkg/sandbox (Spawn/SpawnLong/Kill), pkg/marketplace (Install/Uninstall/
// Launch/Stop/FetchManifest), pkg/downloader, pkg/gateway, pkg/queue,
// pkg/sessions, pkg/vi (PRFetch/Probe), pkg/process.
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

	// EventTierReject fires from the auth.Require substrate-gate deny
	// path (pkg/auth/auth.go) when a caller's stamped tier is NOT in the
	// method's allowed-tier set. The 9th audit-cluster adopter + 1st
	// substrate-tier emit per RFC.tier-auth-substrate §4.7 (Cerberus #73
	// F-4 / Mantis #1758). Closes the STRIDE-R Repudiation gap left open
	// by Phase 1 (substrate ratchet to Phase 2 default-DENY was blind
	// without this emit baseline).
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   op             — method-name string passed into Require, convention
	//                    "<package>.<Type>.<Method>" (e.g.
	//                    "tasks.Service.Create"). Drives forensic queries
	//                    `event="auth.tier.reject" AND op=<method>`.
	//   caller_tier    — the rejected caller's stamped CallerTier (string
	//                    form: "operator" / "renderer" / "plugin" /
	//                    "cascade" / "cron" / "cli" / "internal"). Empty
	//                    tier resolves to "internal" via ensureTier before
	//                    emit so the literal set stays bounded.
	//   caller_subject — opaque subject identifier (account_id or session
	//                    breadcrumb); bounded-length truncation applied at
	//                    emit per Cerberus #1465 (NEVER token bytes —
	//                    Subject is already opaque by construction; the
	//                    truncation defends against an upstream caller
	//                    accidentally stamping a longer-than-bounded
	//                    breadcrumb).
	//   caller_source  — source breadcrumb (e.g. "cli" / "wails" /
	//                    "http-session:LTHN-SESS-1"); bounded-length
	//                    truncation applied at emit for the same reason.
	//   allowed_tiers  — comma-joined CallerTier literals naming the
	//                    set the gate accepted. Lets the forensic walker
	//                    distinguish a renderer-against-operator-only
	//                    deny from a renderer-against-internal-only deny
	//                    without re-running the call-site.
	//
	// The raw method args / return values are NEVER in Meta — this row
	// records the gate decision, not the call payload. The Cerberus
	// #1465 closure-only-scope discipline binds: Meta values are
	// short, bounded, categorical; user content stays off the substrate.
	EventTierReject = "auth.tier.reject"

	// EventTierRejectableTransitional fires alongside the Phase 1.5
	// broad-ALLOW posture for unmigrated Wails services per RFC §4.7
	// (Q-1 caveat). Reserved literal — registered in lockstep with
	// EventTierReject so the parity-grep contract (Go ↔ TS mirror) sees
	// both names today; emit sites land as §5.2 adopters migrate. Lets
	// the operator grep audit-trail for renderer calls that WOULD be
	// denied under Phase 2 default-DENY without the ratchet breaking
	// renderer flows mid-cycle.
	//
	// Meta-shape mirrors EventTierReject so a forensic consumer can
	// merge the two row-streams under a single facet when triaging
	// migration candidates.
	EventTierRejectableTransitional = "auth.tier.rejectable_transitional"

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

	// EventInferenceGenerateRequested fires when pkg/runner.Service.Generate
	// is about to dispatch a prompt to ai.ProviderRouter.Chat. Sibling of
	// the Completed / Failed pair below; the Requested row commits BEFORE
	// the egress so a crash mid-call still leaves the request decision in
	// the audit substrate (Cerberus #45 / Mantis #1658 — Shape A audit
	// at the egress boundary per H#179 surfacing).
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   provider  — first configured route name (router fallback order)
	//   model     — first configured route ModelID
	//   msg_count — 1 for Generate (single-prompt shape)
	//
	// The prompt body is NEVER in Meta — Cerberus #1465 closure-only-scope
	// discipline keeps user-content off the audit substrate. The API key
	// for the provider is NEVER in Meta — provider/model identifiers only.
	EventInferenceGenerateRequested = "inference.generate.requested"

	// EventInferenceGenerateCompleted fires when ai.ProviderRouter.Chat
	// returns successfully from a pkg/runner.Service.Generate dispatch.
	// Sibling of Requested above.
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   provider   — provider actually selected by the router (post-fallback)
	//   model      — model_id actually selected by the router (post-fallback)
	//   tokens     — generated-token count from ProviderChatResponse.Metrics
	//   latency_ms — wall-clock duration from Requested emit to response
	EventInferenceGenerateCompleted = "inference.generate.completed"

	// EventInferenceGenerateFailed fires when ai.ProviderRouter.Chat
	// returns a non-OK result from a pkg/runner.Service.Generate dispatch.
	// Sibling of Requested above.
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   provider   — first configured route name (router fallback order)
	//   model      — first configured route ModelID
	//   error_code — categorical failure code (core.E scope literal); the
	//                full error message is NOT recorded per the SECURITY-
	//                NOTE escape valve in the H#181 brief (defensive
	//                against provider error strings that may echo the
	//                Authorization header / API key bytes).
	EventInferenceGenerateFailed = "inference.generate.failed"

	// EventInferenceChatRequested fires when pkg/runner.Service.Chat is
	// about to dispatch a messages-array request to ai.ProviderRouter.Chat.
	// Sibling of Generate's Requested above; messages-array variant.
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   provider  — first configured route name (router fallback order)
	//   model     — first configured route ModelID
	//   msg_count — len(messages) — number of turns in the conversation
	//
	// The message bodies are NEVER in Meta — same closure-only-scope
	// discipline as EventInferenceGenerateRequested.
	EventInferenceChatRequested = "inference.chat.requested"

	// EventInferenceChatCompleted fires when ai.ProviderRouter.Chat
	// returns successfully from a pkg/runner.Service.Chat dispatch.
	// Sibling of Requested above; messages-array variant.
	//
	// Meta keys match EventInferenceGenerateCompleted: provider, model,
	// tokens, latency_ms.
	EventInferenceChatCompleted = "inference.chat.completed"

	// EventInferenceChatFailed fires when ai.ProviderRouter.Chat returns
	// a non-OK result from a pkg/runner.Service.Chat dispatch. Sibling of
	// Requested above; messages-array variant.
	//
	// Meta keys match EventInferenceGenerateFailed: provider, model,
	// error_code.
	EventInferenceChatFailed = "inference.chat.failed"

	// EventWelfareIntervened fires when the welfare gate (RFC.welfare) acts on
	// a chat turn before it reaches the model — the engine detected hostility
	// and the model chose how to handle it. Emitted at the egress boundary
	// alongside the inference.chat.* events so a welfare action is never a
	// silent branch.
	//
	// Meta keys:
	//
	//   provider — head provider the turn would have routed to
	//   model    — head model id
	//   action   — pause | rephrase | engine_ok | proceed (the model's choice;
	//              proceed = mediation was unreachable, turn went through as-is)
	EventWelfareIntervened = "welfare.intervened"

	// EventSandboxSpawnRequested fires when pkg/sandbox.Service.Spawn
	// is about to dispatch a one-shot container run via the chosen
	// runtime (Apple Container CLI or Docker/Podman via process.Service).
	// Cerberus #47 S-4 (Mantis #1666) — Repudiation gap: lifecycle was
	// invisible to the auditor. The Requested row commits BEFORE the
	// container start so a crash mid-call still leaves the request
	// decision in the audit substrate.
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   image          — OCI tag the container was started from
	//   command_hash   — SHA-256 hex of the entrypoint command. The
	//                    raw command string is NEVER in Meta — the
	//                    SECURITY-NOTE escape valve from the brief
	//                    keeps API tokens / paths off the substrate
	//                    in the (rare) case the entrypoint embeds them.
	//   container_name — best-effort name the runtime stamps; empty for
	//                    the one-shot path because Docker/Podman --rm
	//                    auto-names. Always populated for the Apple path
	//                    (lthn-sandbox-<nanos> per spawnApple()).
	//
	// The raw args / env are NEVER in Meta — Cerberus #1465 closure-
	// only-scope discipline keeps user-content off the audit substrate.
	EventSandboxSpawnRequested = "sandbox.spawn.requested"

	// EventSandboxSpawnSucceeded fires when pkg/sandbox.Service.Spawn
	// returns OK from a one-shot container run. Sibling of Requested.
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   container_id — runtime-assigned identifier when surfaced (Apple
	//                  path); empty for the CLI path (Docker --rm
	//                  doesn't surface the container ID through the
	//                  shell-out capture).
	//   exit_code    — process exit status (0 = clean, -1 = runtime error)
	//   duration_ms  — wall-clock duration of the container run
	EventSandboxSpawnSucceeded = "sandbox.spawn.succeeded"

	// EventSandboxSpawnFailed fires when pkg/sandbox.Service.Spawn
	// returns a non-OK Result from any validation or runtime path.
	// Sibling of Requested.
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   error_code     — core.E scope literal categorising the failure
	//   container_name — best-effort name when assigned before failure;
	//                    empty otherwise
	EventSandboxSpawnFailed = "sandbox.spawn.failed"

	// EventSandboxLongRequested fires when pkg/sandbox.Service.SpawnLong
	// is about to dispatch a long-running container detach via
	// process.Service. Long-running sibling of EventSandboxSpawnRequested
	// per Cerberus #47 S-4 (Mantis #1666). The Requested row commits
	// BEFORE the docker-run shell-out so a crash mid-call still leaves
	// the request decision in the audit substrate.
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   image          — OCI tag the container is started from
	//   command_hash   — SHA-256 hex of the entrypoint command. The raw
	//                    command string is NEVER in Meta (SECURITY-NOTE
	//                    escape valve from the brief).
	//   container_name — runtime-stable name (lthn-sandbox-<sandbox_id>)
	//
	// The raw args / env / volumes / network are NEVER in Meta.
	EventSandboxLongRequested = "sandbox.long.requested"

	// EventSandboxLongSucceeded fires when pkg/sandbox.Service.SpawnLong
	// returns OK and the resulting ContainerHandle is registered. Sibling
	// of Requested. The emit happens AFTER the readiness wait, so a
	// Succeeded row implies the container's exposed port responded (when
	// ExposedPort > 0) or that the container started (when ExposedPort = 0).
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   container_id — assigned sandbox_id (sb-<8-char-random>); the
	//                  CLI-side container_name is the join key for any
	//                  follow-up docker inspect
	//   exit_code    — always 0 on the Succeeded path (the container is
	//                  still running by definition); kept for shape
	//                  parity with EventSandboxSpawnSucceeded
	EventSandboxLongSucceeded = "sandbox.long.succeeded"

	// EventSandboxLongFailed fires when pkg/sandbox.Service.SpawnLong
	// returns a non-OK Result from any validation, runtime, or readiness
	// path. Sibling of Requested.
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   error_code     — core.E scope literal categorising the failure
	//   container_name — best-effort name when assigned before failure
	EventSandboxLongFailed = "sandbox.long.failed"

	// EventSandboxKillRequested fires when pkg/sandbox.Service.Kill is
	// about to dispatch `<runtime> rm -f <container>`. Cerberus #47 S-4
	// (Mantis #1666) — even no-op kills (sandbox-not-found) emit the
	// Requested row so a forensic auditor can correlate caller intent
	// against the registry state at the moment of the call.
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   sandbox_id     — caller-supplied identifier
	//   container_name — derived lthn-sandbox-<sandbox_id>
	EventSandboxKillRequested = "sandbox.kill.requested"

	// EventSandboxKillSucceeded fires when pkg/sandbox.Service.Kill
	// returns OK (the registered handle was found and removed).
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   sandbox_id     — caller-supplied identifier
	//   container_name — derived lthn-sandbox-<sandbox_id>
	EventSandboxKillSucceeded = "sandbox.kill.succeeded"

	// EventSandboxKillFailed fires when pkg/sandbox.Service.Kill returns
	// a non-OK Result (empty id or sandbox-not-found). The docker rm -f
	// shell-out is best-effort even on the failure path so the emit
	// reflects caller intent regardless of registry state.
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   sandbox_id     — caller-supplied identifier (may be empty)
	//   error_code     — core.E scope literal categorising the failure
	EventSandboxKillFailed = "sandbox.kill.failed"

	// EventSandboxVolumeRejected fires from pkg/sandbox.buildLongRunArgs
	// when a LongVolumeMount entry is dropped because it fails one of
	// the Cerberus #1431 (host-side IsValidVolumeName) or #1446
	// (container-side IsValidContainerPath) gates. Cerberus #47 S-4
	// (Mantis #1666) — promotes the defence-in-depth core.Warn to a
	// typed audit event so a future reconcile / forensic walker can
	// flag any caller that attempts to bypass marketplace's primary
	// volume validator (marketplace's resolveVolumes is the loud-error
	// path; this is the silent-skip backstop).
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   volume_name — caller-asserted name that failed validation; safe
	//                 to record because IsValidVolumeName already
	//                 rejects everything that could embed credentials
	//                 (length 1..64, alnum + _.- only)
	//   container   — caller-asserted container-side mount path; safe
	//                 to record because IsValidContainerPath rejects
	//                 NUL / control chars / colons / commas / whitespace
	//   reason      — reserved literal "invalid_name" or
	//                 "invalid_container_path" so a downstream consumer
	//                 can categorise without re-running the validators
	EventSandboxVolumeRejected = "sandbox.volume.rejected"

	// EventSandboxSpawnRejected fires from the TierGoOnly substrate-shim
	// when an in-process plugin spawn-request is rejected at the gate
	// (Cerberus #55 ADD-2 / Mantis #1664 Phase A reservation). Sibling
	// of EventSandboxSpawnFailed — Failed covers runtime/validation
	// failures from the container path; Rejected covers the policy gate
	// that refuses to dispatch into the substrate at all (e.g. tier
	// mismatch, missing plugin entitlement, sandbox-substrate not wired).
	//
	// Phase A reserves the literal so the parity-grep contract enforces
	// Go ↔ TS lockstep ahead of the Phase B substrate refactor (deferred
	// pending Cladius adjudication of #1697 — internal/sandboxsubstrate
	// shape). No emit site references this constant today; the Phase B
	// shim will wire it in the same commit it introduces the substrate
	// gate.
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced; final list
	// pinned at Phase B when emit sites land):
	//
	//   plugin_id  — installed plugin code requesting the spawn
	//   reason     — categorical rejection reason (closed set TBD at
	//                Phase B; expected values include "tier_mismatch" /
	//                "substrate_unwired" / "entitlement_missing")
	//
	// The raw spawn args / env are NEVER in Meta — same Cerberus #1465
	// closure-only-scope discipline as the sibling sandbox events.
	EventSandboxSpawnRejected = "sandbox.spawn.rejected"

	// EventProcessRunRequested fires when pkg/process.Service.Run is
	// about to dispatch `process.run` (synchronous exec) via the upstream
	// dappco.re/go/process action surface. Cerberus #50 (Mantis #1683) —
	// Repudiation gap close. process.run / process.start / process.kill
	// previously emitted ZERO audit events; arbitrary spawn was
	// forensically silent. The Requested row commits BEFORE the action
	// dispatch so a crash mid-call still leaves the request decision in
	// the audit substrate.
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   command_hash — SHA-256 hex of the entrypoint command. The raw
	//                  command string is NEVER in Meta per the brief's
	//                  SECURITY-NOTE escape valve (entrypoint commands
	//                  occasionally embed tokens / paths). Mirror of
	//                  pkg/sandbox's command_hash discipline.
	//   runtime      — reserved literal "core-process" identifying the
	//                  dispatch path (lthn-side wrapper around upstream
	//                  go-process Service). Reserved for forensic walker
	//                  if a future runtime variant ships (e.g. sandboxed
	//                  process dispatch).
	//
	// The raw args / env / cwd are NEVER in Meta — Cerberus #1465
	// closure-only-scope discipline keeps user-content off the audit
	// substrate.
	EventProcessRunRequested = "process.run.requested"

	// EventProcessRunSucceeded fires when pkg/process.Service.Run
	// returns OK from a `process.run` action dispatch. Sibling of
	// Requested above.
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   exit_code — process exit status. Always 0 on the Succeeded path
	//               (upstream go-process surfaces non-zero exits as a
	//               non-OK Result with the exit code folded into the
	//               error message); kept for shape parity with sibling
	//               sandbox Succeeded rows.
	EventProcessRunSucceeded = "process.run.succeeded"

	// EventProcessRunFailed fires when pkg/process.Service.Run returns
	// a non-OK Result from any validation, dispatch, or runtime path.
	// Sibling of Requested above.
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   error_code — bounded-keyspace failure literal derived via
	//                audit.ErrorCode(r) (pkg/audit/errorcode.go); NEVER
	//                the raw Result.Error() / err.Error() prose — see
	//                const-block preamble for the substrate contract.
	EventProcessRunFailed = "process.run.failed"

	// EventProcessStartRequested fires when pkg/process.Service.Start is
	// about to dispatch `process.start` (background detach) via the
	// upstream go-process action surface. Long-running sibling of
	// EventProcessRunRequested per Cerberus #50 (Mantis #1683).
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   command_hash — SHA-256 hex of the entrypoint command (raw bytes
	//                  never in Meta — SECURITY-NOTE escape valve).
	//   runtime      — reserved literal "core-process" — see Requested
	//                  above.
	EventProcessStartRequested = "process.start.requested"

	// EventProcessStartSucceeded fires when pkg/process.Service.Start
	// returns OK from a `process.start` action dispatch. Sibling of
	// Requested.
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   process_id — upstream go-process registry identifier (Result.Value
	//                from handleStart). Forensic join key — every
	//                subsequent Get / Kill on this process correlates via
	//                this id. The OS-level PID is NOT recorded here;
	//                callers needing it perform a follow-up process.get
	//                round-trip outside the audit substrate so the
	//                Succeeded row commits without a second action call
	//                in the hot path.
	EventProcessStartSucceeded = "process.start.succeeded"

	// EventProcessStartFailed fires when pkg/process.Service.Start
	// returns a non-OK Result. Sibling of Requested.
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   error_code — bounded-keyspace failure literal via audit.ErrorCode(r)
	//                (NEVER raw Result.Error() — substrate contract above).
	EventProcessStartFailed = "process.start.failed"

	// EventProcessKillRequested fires when pkg/process.Service.Kill is
	// about to dispatch `process.kill` for a given registry id.
	// Cerberus #50 (Mantis #1683) — even no-op kills (process-not-found)
	// emit the Requested row so a forensic auditor can correlate caller
	// intent against the registry state at the moment of the call.
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   process_id — caller-supplied registry identifier.
	EventProcessKillRequested = "process.kill.requested"

	// EventProcessKillSucceeded fires when pkg/process.Service.Kill
	// returns OK (the registered process was found and signalled).
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   process_id — caller-supplied registry identifier.
	EventProcessKillSucceeded = "process.kill.succeeded"

	// EventProcessKillFailed fires when pkg/process.Service.Kill returns
	// a non-OK Result (empty id, process-not-found, signal failure).
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   process_id — caller-supplied registry identifier (may be empty).
	//   error_code — bounded-keyspace failure literal via audit.ErrorCode(r)
	//                (NEVER raw Result.Error() — substrate contract above).
	EventProcessKillFailed = "process.kill.failed"

	// EventMarketplaceInstallRequested fires when pkg/marketplace.Service.Install
	// is about to spawn images / persist the InstalledBundle record for a
	// validated bundle manifest. Cerberus #54 C4 (Mantis #1692) —
	// Repudiation gap close. Install / Uninstall / Launch / Stop /
	// FetchManifest previously emitted ZERO audit events; the bundle
	// install surface (sandbox spawn + plugin registration + view-registry
	// mutation) was forensically silent. The Requested row commits BEFORE
	// the sandbox SpawnLong loop so a crash mid-call still leaves the
	// install decision in the audit substrate.
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   bundle_id   — manifest.Name (validated, alphanumeric + dash)
	//   plugin_code — pluginCodeOf(manifest) — falls back to bundle_id when
	//                 the manifest omits plugin.code (legacy v1 shape)
	//
	// The raw manifest body / image refs / env values are NEVER in Meta —
	// Cerberus #1465 closure-only-scope discipline keeps user-content off
	// the audit substrate. The plugin_code is safe because Install gates
	// against the BundleName regex (manifest.go:isValidBundleName) and the
	// plugin-code uniqueness check (checkPluginCodeCollision) before the
	// emit fires; both reject anything that could embed secrets or paths.
	EventMarketplaceInstallRequested = "marketplace.install.requested"

	// EventMarketplaceInstallSucceeded fires when pkg/marketplace.Service.Install
	// returns OK — all images spawned, orm record persisted, plugin-view
	// registry populated. Sibling of Requested above.
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   bundle_id    — manifest.Name
	//   plugin_code  — effective plugin code (pluginCodeOf)
	//   image_count  — number of image entries successfully spawned
	EventMarketplaceInstallSucceeded = "marketplace.install.succeeded"

	// EventMarketplaceInstallFailed fires when pkg/marketplace.Service.Install
	// returns a non-OK Result from any validation, sandbox-spawn, orm-save,
	// or rollback path. Sibling of Requested above.
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   bundle_id  — manifest.Name (may be empty when ValidateManifest
	//                rejected the input before Name was resolved)
	//   error_code — bounded-keyspace failure literal via audit.ErrorCode(r)
	//                (pkg/audit/errorcode.go); NEVER raw Result.Error() prose
	//                — substrate contract above.
	EventMarketplaceInstallFailed = "marketplace.install.failed"

	// EventMarketplaceUninstallRequested fires when pkg/marketplace.Service.Uninstall
	// is about to drop the plugin-view registry entry / stop sandboxes /
	// remove the orm record for a bundle. Cerberus #54 C4 (Mantis #1692) —
	// the Requested row commits BEFORE the multi-step teardown so a crash
	// mid-uninstall still leaves the operator intent in the audit substrate
	// (forensic walker can pair the Requested with whichever step landed).
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   bundle_id   — caller-supplied identifier
	//   plugin_code — resolved via resolvePluginCode (manifest-on-disk
	//                 lookup) — falls back to bundle_id when the manifest
	//                 is missing or unparseable (legacy uninstall path)
	EventMarketplaceUninstallRequested = "marketplace.uninstall.requested"

	// EventMarketplaceUninstallSucceeded fires when Uninstall returns OK.
	// Sibling of Requested above.
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   bundle_id   — caller-supplied identifier
	//   plugin_code — resolved plugin code at the moment of uninstall
	EventMarketplaceUninstallSucceeded = "marketplace.uninstall.succeeded"

	// EventMarketplaceUninstallFailed fires when Uninstall returns a
	// non-OK Result (empty id, orm delete failure post-sandbox-teardown).
	// Sibling of Requested above.
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   bundle_id  — caller-supplied identifier (may be empty)
	//   error_code — bounded-keyspace failure literal via audit.ErrorCode(r)
	//                (NEVER raw Result.Error() — substrate contract above).
	EventMarketplaceUninstallFailed = "marketplace.uninstall.failed"

	// EventMarketplaceLaunchRequested fires when pkg/marketplace.Service.Launch
	// is about to re-spawn sandboxes for an already-installed bundle.
	// Cerberus #54 C4 (Mantis #1692) — Launch is the second-most-common
	// path to plugin-container live state after Install; forensic walker
	// needs the Requested row to distinguish "Install spawned the
	// containers" from "Launch re-spawned them after a host restart".
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   bundle_id — caller-supplied identifier
	EventMarketplaceLaunchRequested = "marketplace.launch.requested"

	// EventMarketplaceLaunchSucceeded fires when Launch returns OK.
	// Sibling of Requested above.
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   bundle_id    — caller-supplied identifier
	//   image_count  — number of image entries successfully re-spawned
	EventMarketplaceLaunchSucceeded = "marketplace.launch.succeeded"

	// EventMarketplaceLaunchFailed fires when Launch returns a non-OK
	// Result (empty id, bundle not installed, manifest missing on disk,
	// orm save failure post-spawn). Sibling of Requested above.
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   bundle_id  — caller-supplied identifier (may be empty)
	//   error_code — bounded-keyspace failure literal via audit.ErrorCode(r)
	//                (NEVER raw Result.Error() — substrate contract above).
	EventMarketplaceLaunchFailed = "marketplace.launch.failed"

	// EventMarketplaceStopRequested fires when pkg/marketplace.Service.Stop
	// is about to Kill all running sandboxes for a bundle + flip the orm
	// Status to BundleStatusStopped. Cerberus #54 C4 (Mantis #1692) —
	// closes the lifecycle quartet (Install / Launch / Stop / Uninstall)
	// so a forensic walker can reconstruct the per-bundle state machine
	// from audit alone.
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   bundle_id — caller-supplied identifier
	EventMarketplaceStopRequested = "marketplace.stop.requested"

	// EventMarketplaceStopSucceeded fires when Stop returns OK. Sibling
	// of Requested above.
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   bundle_id — caller-supplied identifier
	EventMarketplaceStopSucceeded = "marketplace.stop.succeeded"

	// EventMarketplaceStopFailed fires when Stop returns a non-OK Result
	// (empty id, orm save failure after the Kill loop). Sibling of
	// Requested above.
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   bundle_id  — caller-supplied identifier (may be empty)
	//   error_code — bounded-keyspace failure literal via audit.ErrorCode(r)
	//                (NEVER raw Result.Error() — substrate contract above).
	EventMarketplaceStopFailed = "marketplace.stop.failed"

	// EventMarketplaceFetchManifestRequested fires when
	// pkg/marketplace.Service.FetchManifest is about to HTTP GET a bundle
	// manifest from a caller-supplied source URL. Cerberus #54 C4 (Mantis
	// #1692) — manifest fetch is the network-bound entry to the install
	// pipeline; the Requested row commits BEFORE the host-allowlist gate
	// so a forensic walker can correlate rejected hosts with caller intent.
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   domain_only — URL hostname only (no path / query / fragment) per
	//                 the brief's SECURITY-NOTE escape valve. Manifest URL
	//                 paths occasionally embed agent identifiers in query
	//                 strings; recording host-only keeps the audit row
	//                 forensic without leaking path-traversal evidence
	//                 attempts.
	EventMarketplaceFetchManifestRequested = "marketplace.fetch_manifest.requested"

	// EventMarketplaceFetchManifestSucceeded fires when FetchManifest
	// returns OK from the full fetch + parse + validate pipeline. Sibling
	// of Requested above.
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   domain_only — URL hostname (mirrors Requested)
	//   bundle_id   — manifest.Name resolved from the parsed manifest
	EventMarketplaceFetchManifestSucceeded = "marketplace.fetch_manifest.succeeded"

	// EventMarketplaceFetchManifestFailed fires when FetchManifest returns
	// a non-OK Result from any validation, network, parse, or downstream-
	// validate path. Sibling of Requested above. Distinct from
	// FetchManifestRejected (host-allowlist gate) — Failed covers every
	// other failure mode so the auditor can distinguish "policy blocked
	// the fetch" from "fetch happened but downstream failed".
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   domain_only — URL hostname (when parseable; empty when the URL
	//                 was malformed enough to reject before parse)
	//   error_code  — bounded-keyspace failure literal via audit.ErrorCode(r)
	//                 (NEVER raw Result.Error() — substrate contract above).
	EventMarketplaceFetchManifestFailed = "marketplace.fetch_manifest.failed"

	// EventMarketplaceFetchManifestRejected fires when FetchManifest
	// rejects a caller-supplied URL at the host-allowlist gate
	// (requireAllowedManifestHost — Cerberus #1690 C2). Sibling of
	// EventSandboxVolumeRejected — Rejected covers the policy gate that
	// refuses to dispatch at all; Failed above covers runtime/validation
	// failures from URLs that passed the gate.
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   domain_only — URL hostname that failed the allowlist (when
	//                 parseable; empty for malformed input)
	//   reason      — reserved literal "host_not_allowed" so a downstream
	//                 consumer can categorise without re-running the gate
	EventMarketplaceFetchManifestRejected = "marketplace.fetch_manifest.rejected"

	// EventMarketplaceValidateManifestFailed fires when
	// pkg/marketplace.ValidateManifest rejects a parsed BundleManifest
	// (bad name, unsupported schema, malformed plugin block, invalid view
	// custom-element tag, etc). Cerberus #54 C4 (Mantis #1692) — every
	// validation failure becomes a forensic row so a future imagetrust /
	// yaml-bomb attack pattern leaves a trace even when no install lands.
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   bundle_id  — manifest.Name (may be empty when the rejection was
	//                on the Name field itself)
	//   error_code — bounded-keyspace failure literal via audit.ErrorCode(r)
	//                (pkg/audit/errorcode.go). The validator's typed scope
	//                literals (validateOp etc) surface through r.Code() /
	//                (*core.Err).Operation so the chip-filter UI can group
	//                by the failure class; NEVER raw Result.Error() prose
	//                — substrate contract above.
	//
	// NOTE: ValidateManifest succeeds quietly (no audit row on the OK
	// path) — emitting Succeeded for every parse-then-validate call would
	// flood the substrate; the install pipeline already emits
	// InstallRequested which subsumes the validate-OK signal.
	EventMarketplaceValidateManifestFailed = "marketplace.validate_manifest.failed"

	// EventMarketplaceTrustedKeyMutated fires whenever the marketplace
	// trusted_keys.json store at ~/Lethean/conf/marketplace/trusted_keys.json
	// is mutated (key added, key replaced, key removed). DREAD F4 HIGH
	// (Mantis #1644) — without this row, an attacker who reaches the
	// conf path can silently add an attacker-controlled key + serve
	// signed manifests that pass the verify gate. The Mutated row is a
	// privesc-grade forensic anchor: every trust-store delta carries
	// old + new fingerprint + reason + operator-confirmation context so
	// the auditor can replay the chain-of-custody.
	//
	// Outcome is OutcomeOK for mutations the operator authorised (the
	// normal "I am adding a key" path) and OutcomeDenied for mutations
	// the validator refused (duplicate-name-different-keyid load-time
	// reject per DREAD N1 / Mantis #1647).
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   name              — the trusted-key NAME (priority-order alias,
	//                       e.g. "lthn-official"). Safe — gated against
	//                       a basename regex at load time.
	//   old_fingerprint   — SHA256 prefix of the previous pubkey for
	//                       this name (16 hex chars, empty on add).
	//   new_fingerprint   — SHA256 prefix of the new pubkey (16 hex
	//                       chars, empty on remove).
	//   reason            — closed-set literal:
	//                         "add" — first time this name landed
	//                         "replace" — name existed, key changed
	//                         "remove" — name dropped
	//                         "duplicate_name_keyid_mismatch" — DREAD N1
	//                                                          load-time reject
	//   operator_confirmed — bool the caller passed when invoking the
	//                        mutation API; the host UI MUST require an
	//                        explicit "I understand" confirmation before
	//                        flipping this true. Replay attacks where
	//                        a caller programmatically asserts true
	//                        without UI confirmation still leave the
	//                        row, just without operator-confirmation
	//                        evidence — the auditor can spot the gap.
	EventMarketplaceTrustedKeyMutated = "marketplace.trusted_key.mutated"

	// Wave 2 marketplace audit emits (Mantis #1745 Cerberus #71 F-3/F-4/
	// F-5/F-6) — extracted from pkg/marketplace/install.go inline string
	// literals into typed constants now that the brief allowlist
	// includes pkg/audit/types.go. The string values are unchanged from
	// the shipped Wave 2 commit (f866c00) so on-disk audit history +
	// the TS chip-filter golden-list keep working without migration.

	// EventMarketplaceCatalogueStaleInstallAttempt fires when an
	// operator triggers Install with `ForceStaleInstall=true` against a
	// catalogue entry whose age exceeds the staleness threshold. F3
	// override-attempted row — distinct from the routine install
	// audit pair so the auditor can grep stale-override frequency.
	EventMarketplaceCatalogueStaleInstallAttempt = "marketplace.catalogue.stale_install_attempt"

	// EventMarketplaceManifestVersionPending fires when an operator
	// hits Install on a bundle whose manifest version is Pending (i.e.
	// the digest has not yet been confirmed by the operator's Promote
	// action). F6 manifest-version-bump gate; Outcome=denied because
	// the install short-circuits with a friendly "confirm the version
	// bump first" message rather than landing the unverified payload.
	EventMarketplaceManifestVersionPending = "marketplace.manifest.version.pending"

	// EventMarketplaceManifestVersionTransition fires when the operator
	// confirms Pending → Canonical OR a rollback drops Pending back to
	// the prior canonical. F6 promotion lifecycle row. Meta["transition"]
	// is one of "confirmed" / "rolled_back" / "auto_promoted" (forward-
	// arc; auto_promoted is reserved for an idle-window auto-promotion
	// feature not yet shipped).
	EventMarketplaceManifestVersionTransition = "marketplace.manifest.version.transition"

	// EventMarketplaceImageDigestMismatch fires when the runtime image
	// digest the registry served differs from the digest the manifest
	// pinned. F2 hard-reject row; Outcome=denied because the install
	// aborts before any container starts. Both expected + actual
	// digests land in Meta so a forensic correlation against the
	// registry's history is direct.
	EventMarketplaceImageDigestMismatch = "marketplace.image.digest_mismatch"

	// EventMarketplaceImageDigestUnverifiable fires when the
	// install path cannot verify the image digest at all (docker
	// missing, inspect failed, parse failed). F2 degraded-verify row;
	// Outcome=failed because the failure is environmental, not a
	// security denial. Meta["reason"] carries the typed prefix the
	// sandbox stamped so an operator can distinguish the modes.
	EventMarketplaceImageDigestUnverifiable = "marketplace.image.digest_unverifiable"

	// EventMarketplacePermissionDiffRequiresReConsent fires when a
	// manifest update adds permissions not previously granted by the
	// operator. F5 re-consent row; the install short-circuits and the
	// frontend surfaces a modal with the new scope:mode pairs. The
	// new_scopes Meta key carries the closed-set list so the auditor
	// can grep for "what new powers did an upgrade request".
	EventMarketplacePermissionDiffRequiresReConsent = "marketplace.permission.diff_requires_re_consent"

	// EventGatewayDispatchRequested fires when pkg/gateway.Service.Handle
	// is about to dispatch a resolved (bundle, scope, mode) tuple to its
	// registered Handler. Cerberus #57 F-2 (Mantis #1700) — Repudiation
	// gap close. Plugin → host data dispatch was forensically silent;
	// every bundle call to /v1/api/gateway/<scope>/<mode> previously
	// emitted ZERO audit events even though the gateway is the last mute
	// boundary in the audit cluster (sandbox / runner / process /
	// marketplace already adopt). The Requested row commits BEFORE the
	// scope-Handler invocation so a crash mid-call still leaves the
	// dispatch decision in the audit substrate.
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   bundle_id     — authoritative bundle code resolved via
	//                   BundleCodeResolver (X-Bundle-Token) when present;
	//                   otherwise the caller-asserted Bundle-ID header.
	//                   Safe — Install gates against the BundleName regex
	//                   so the value cannot embed credentials.
	//                   TODO (#1702 F-4 forward-arc): replace raw bundle_id
	//                   with a per-launch forensic discriminator once the
	//                   X-Bundle-Token-issuing path stamps one at Launch
	//                   time so a forensic walker can distinguish
	//                   concurrent launches of the same bundle.
	//   route_pattern — "<scope>:<mode>" join (the public URL path shape
	//                   stamped by pkg/server's POST route). Safe — both
	//                   halves are gin.Context params on a registered
	//                   route; arbitrary user-controlled strings never
	//                   reach this field.
	//   method        — HTTP verb (always "POST" today, kept open so a
	//                   future GET-shape variant in the v2 schema doesn't
	//                   need a Meta-shape migration).
	//
	// The request body / headers / query params / X-Bundle-Token bytes
	// are NEVER in Meta — Cerberus #1465 closure-only-scope discipline
	// keeps user-content and bearer secrets off the audit substrate.
	EventGatewayDispatchRequested = "gateway.dispatch.requested"

	// EventGatewayDispatchSucceeded fires when the registered scope
	// Handler returns OK from a Handle dispatch. Sibling of Requested.
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   bundle_id     — authoritative bundle code (mirrors Requested)
	//   route_pattern — "<scope>:<mode>" join (mirrors Requested)
	//   method        — HTTP verb (mirrors Requested)
	EventGatewayDispatchSucceeded = "gateway.dispatch.succeeded"

	// EventGatewayDispatchFailed fires when the registered scope Handler
	// returns a non-OK Result from a Handle dispatch. Sibling of
	// Requested. Distinct from EventGatewayDispatchRejected — Failed
	// covers handler-layer failures (validation, downstream service
	// error) AFTER the auth/identity/permission gates passed; Rejected
	// covers the gate paths that refuse to dispatch at all.
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   bundle_id     — authoritative bundle code (mirrors Requested)
	//   route_pattern — "<scope>:<mode>" join (mirrors Requested)
	//   method        — HTTP verb (mirrors Requested)
	//   error_code    — bounded-keyspace failure literal via audit.ErrorCode(r)
	//                   (pkg/audit/errorcode.go); NEVER raw Result.Error()
	//                   prose. The substrate derives a stable literal from
	//                   r.Code() / (*core.Err).Operation so handler-specific
	//                   context cannot leak into the audit error_code field
	//                   — substrate contract above.
	EventGatewayDispatchFailed = "gateway.dispatch.failed"

	// EventGatewayDispatchRejected fires when Handle refuses to dispatch
	// at one of the auth / identity / scope / permission gates BEFORE
	// the scope Handler runs. Sibling of EventMarketplaceFetchManifestRejected
	// (host-allowlist gate) and EventSandboxSpawnRejected (substrate
	// policy gate) — Rejected distinguishes policy-block from runtime
	// failure across the audit cluster.
	//
	// Closed reason set (Meta["reason"]):
	//
	//   invalid_bundle_token — X-Bundle-Token present but resolver did
	//                          not produce a known plugin code (covers
	//                          spoof / replay / expired-token attempts).
	//   missing_bundle_id    — neither X-Bundle-Token nor Bundle-ID
	//                          header produced a non-empty identity.
	//   scope_unavailable    — (scope, mode) tuple has no registered
	//                          Handler in the gateway's dispatch table.
	//   permission_denied    — bundle is installed but did NOT declare
	//                          the (scope, mode) pair in its install-
	//                          time manifest.permissions block.
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   bundle_id     — best-effort bundle code at rejection time; empty
	//                   when the rejection happened before identity
	//                   resolution (invalid_bundle_token / missing_bundle_id).
	//   route_pattern — "<scope>:<mode>" join. Always populated — the
	//                   gin route already extracted both params before
	//                   any gate fires.
	//   method        — HTTP verb (mirrors Requested).
	//   reason        — one of the closed-set literals above. Reserved
	//                   so a downstream chip-filter UI / forensic walker
	//                   can categorise without re-running the gate logic.
	//
	// The rejected request body / X-Bundle-Token bytes / Authorization
	// header are NEVER in Meta — same closure-only-scope discipline as
	// the sibling gateway events.
	EventGatewayDispatchRejected = "gateway.dispatch.rejected"

	// EventDownloaderFetchRequested fires when pkg/downloader.FetchVerified
	// is about to dispatch a quarantine-routed HTTP GET for a model file
	// after the URL + name shape gates pass but BEFORE the host-allowlist
	// + DNS-resolve-private-IP policy gates. Sibling of the marketplace
	// FetchManifest quartet — FetchVerified is the supply-chain primary
	// (model weights) where FetchManifest is the install-pipeline primary
	// (bundle manifests). Both share the host-allowlist + SSRF defence so
	// both grow the same Requested / Succeeded / Failed / Rejected shape.
	//
	// The Requested row commits BEFORE the policy gates so a forensic
	// walker can correlate caller intent (which host) with the gate
	// outcome (which policy fired). Wraps at FetchVerified so Fetch +
	// FetchWithProgress + WailsService.Download all flow through one emit
	// surface (no double-emit risk — the wrapper chain is single-call).
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   source_host     — URL hostname only (no path / query / fragment)
	//                     per the SECURITY-NOTE escape valve mirroring the
	//                     marketplace FetchManifest discipline. HuggingFace
	//                     URLs occasionally embed access tokens in query
	//                     strings; recording host-only keeps the row
	//                     forensic without leaking secret material.
	//   sha256_expected — caller-supplied expected digest hex (safe — the
	//                     expected hash is a public verification value
	//                     drawn from a model card or marketplace manifest;
	//                     empty when the caller routes through the
	//                     quarantine-only path with no digest assertion).
	//
	// The raw URL / Authorization header / response body is NEVER in Meta —
	// Cerberus #1465 closure-only-scope discipline keeps secret material
	// off the audit substrate.
	EventDownloaderFetchRequested = "downloader.fetch.requested"

	// EventDownloaderFetchSucceeded fires when FetchVerified returns OK
	// from the full pipeline (gate / GET / quarantine / digest-verify /
	// atomic-promote). Sibling of Requested above.
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   source_host — URL hostname (mirrors Requested)
	//   bytes       — byte count of the promoted file. Forensic correlation
	//                 size for the day-file tailer; allows pairing the
	//                 Succeeded row against disk-usage / bandwidth
	//                 accounting without re-stat-ing the model path.
	EventDownloaderFetchSucceeded = "downloader.fetch.succeeded"

	// EventDownloaderFetchFailed fires when FetchVerified returns a non-OK
	// Result from any path EXCEPT the host-allowlist or private-IP gates
	// (which have their own Rejected event so the auditor can distinguish
	// policy-block from runtime / network / digest-mismatch failure).
	// Covers: URL parse failure, HTTP non-2xx, body copy failure, size cap
	// exceeded, digest mismatch, atomic-promote rename failure, and the
	// pre-network name / URL shape gates.
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   source_host — URL hostname (when parseable; empty when the URL was
	//                 malformed enough to reject before parse)
	//   error_code  — bounded-keyspace failure literal via audit.ErrorCode(r)
	//                 (pkg/audit/errorcode.go); NEVER raw Result.Error() prose.
	//                 The substrate derives the literal from r.Code() /
	//                 (*core.Err).Operation so raw URL paths / Authorization
	//                 bytes cannot leak — substrate contract above.
	EventDownloaderFetchFailed = "downloader.fetch.failed"

	// EventDownloaderFetchRejected fires when FetchVerified rejects the
	// caller-supplied URL at one of the policy gates — host-allowlist
	// (AllowedSource) or DNS-resolve-private-IP (verifyResolvedIPNotPrivate
	// per Cerberus #1429). Sibling of EventMarketplaceFetchManifestRejected
	// — Rejected distinguishes policy-block from runtime / network failure
	// so a forensic walker can audit attempted SSRF / off-allowlist fetch
	// patterns without scanning every Failed row.
	//
	// Closed reason set (Meta["reason"]):
	//
	//   host_not_allowed     — AllowedSource rejected the suffix-match
	//                          gate (hostname not under the compile-time
	//                          allowlist).
	//   private_ip_resolved  — verifyResolvedIPNotPrivate fired (DNS
	//                          rebinding / SSRF defence; the allowlisted
	//                          hostname resolved to a private / loopback /
	//                          link-local / cloud-IMDS address).
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   source_host — URL hostname that failed the gate (when parseable;
	//                 empty for malformed input — those land as Failed)
	//   reason      — one of the closed-set literals above. Reserved so a
	//                 downstream chip-filter UI / forensic walker can
	//                 categorise without re-running the gate logic. New
	//                 literals require a reserved-schema bump in the same
	//                 commit that introduces the new gate.
	EventDownloaderFetchRejected = "downloader.fetch.rejected"

	// EventQueueJobStarted fires when pkg/queue's worker loop claims a
	// pending Job (CAS Status pending → running) and is about to dispatch
	// its registered handler. Cerberus #64 F-5 (Mantis #1725) — STRIDE-R
	// Repudiation gap close. Pre-cascade the pkg/queue audit emit collapsed
	// PhaseStarted / PhaseDone / PhaseCancelled all onto the single
	// "queue.dequeued" literal so a forensic walker could not distinguish
	// "job started" from "job completed" from "job cancelled" by event-name
	// alone; trio split mirrors the established audit-cluster shape across
	// runner / sandbox / process / marketplace / gateway / downloader so
	// the parity-grep contract enforces Go ↔ TS lockstep on lifecycle.
	//
	// The Started row commits as the claimNext() CAS broadcasts JobChanged
	// onto the typed bus; pkg/queue's AttachAudit subscriber projects the
	// PhaseStarted event onto this literal so a crash mid-handler still
	// leaves the claim decision in the audit substrate.
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   job_id   — orm-assigned job identifier (j-<ksuid>); forensic join
	//              key for the Started → Succeeded/Failed/Cancelled trio
	//              and for any downstream incident correlation.
	//   kind     — registered handler kind (queue.kind.<kind> action
	//              namespace). Safe — RegisterKind rejects empty / unknown
	//              kinds at registration time so this literal is bounded.
	//   status   — the job's post-CAS Status literal ("running" on the
	//              Started path) for shape parity with sibling rows.
	//
	// The handler payload bytes are NEVER in Meta — Cerberus #1465 closure-
	// only-scope discipline keeps user-content (which may carry account
	// identifiers, paths, or downstream credentials) off the audit substrate.
	EventQueueJobStarted = "queue.job.started"

	// EventQueueJobSucceeded fires when pkg/queue's worker dispatch returns
	// OK from a registered handler (markDone CAS Status running → done).
	// Sibling of EventQueueJobStarted. Pre-cascade this projected onto the
	// shared "queue.dequeued" literal alongside Started + Cancelled; the
	// split is load-bearing for the forensic walker's lifecycle reconstruction.
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   job_id   — orm-assigned job identifier (mirrors Started)
	//   kind     — registered handler kind (mirrors Started)
	//   status   — "done" on this path
	EventQueueJobSucceeded = "queue.job.succeeded"

	// EventQueueJobFailed fires when pkg/queue's worker dispatch returns a
	// non-OK Result from a registered handler OR the handler panics
	// (markFailed CAS Status running → failed). Sibling of EventQueueJobStarted.
	// Replaces the pre-cascade "queue.failed" literal — moving onto the
	// "queue.job.*" namespace pins the lifecycle trio onto a stable prefix
	// the chip-filter UI can group, mirroring sandbox.spawn.* + process.run.*
	// + marketplace.install.* family naming.
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   job_id   — orm-assigned job identifier (mirrors Started)
	//   kind     — registered handler kind (mirrors Started)
	//   status   — "failed" on this path
	//
	// The handler's error message is NEVER in Meta — markFailed records the
	// stringified error onto Job.LastError (the orm row); the audit substrate
	// records the decision-fact only so handler errors that may embed user-
	// content remain quarantined to the per-job persistence layer.
	EventQueueJobFailed = "queue.job.failed"

	// EventQueueJobCancelled fires when pkg/queue.Cancel transitions a
	// pending Job to StatusCancelled (user / system cancelled before
	// completion). Sibling of EventQueueJobStarted. Pre-cascade this
	// projected onto the shared "queue.dequeued" literal; the split lets a
	// forensic walker distinguish operator-initiated cancellation from
	// worker-completed (Succeeded) and handler-error (Failed) terminal
	// states.
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   job_id   — orm-assigned job identifier (mirrors Started)
	//   kind     — registered handler kind (mirrors Started)
	//   status   — "cancelled" on this path
	EventQueueJobCancelled = "queue.job.cancelled"

	// EventSessionsCreateRequested fires when pkg/sessions.Create is
	// about to allocate a new session id + write the manifest + empty
	// message log. Cerberus #67 F-2 (Mantis #1737) — STRIDE-R Repudiation
	// gap close. Chat session lifecycle (create / rename / delete /
	// message-append) was forensically silent; the 7th audit-cluster
	// adoption following runner / sandbox / process / marketplace /
	// gateway / downloader / queue. The Requested row commits BEFORE the
	// id allocation so a crash mid-call still leaves caller intent in
	// the audit substrate.
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   title_len — rune count of the caller-supplied title (defensive:
	//               raw title bytes are NEVER in Meta because session
	//               titles routinely embed user PII per H#212 chat-content
	//               discipline — "Q3 budget for project X", "Bob's notes",
	//               etc. The length is forensically useful — pairs with
	//               manifest reads when reconstructing a session's lineage —
	//               without leaking the content.
	//
	// The session_id is NOT in the Meta on Requested because it has not
	// yet been allocated — the Succeeded row stamps it. Outcome=ok per
	// the established Requested-row convention.
	EventSessionsCreateRequested = "sessions.create.requested"

	// EventSessionsCreateSucceeded fires when pkg/sessions.Create returns
	// OK from the manifest-write + empty-message-log-write pipeline.
	// Sibling of Requested above.
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   session_id — newly-allocated session identifier (16-byte random hex
	//                via core.RandomString; opaque to the user, safe to
	//                record). Forensic join key for the Requested →
	//                Succeeded pair and for every subsequent Append /
	//                Rename / Delete on this id.
	//   title_len  — rune count of the caller-supplied title (mirrors
	//                Requested for symmetry).
	EventSessionsCreateSucceeded = "sessions.create.succeeded"

	// EventSessionsCreateFailed fires when pkg/sessions.Create returns a
	// non-OK Result from any path (nil core, RandomString failure,
	// manifest write failure, message-log write failure). Sibling of
	// Requested above.
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   error_code — bounded-keyspace failure literal via audit.ErrorCode(r)
	//                (pkg/audit/errorcode.go); NEVER raw Result.Error() prose
	//                — substrate contract at the top of this const block.
	//   title_len  — rune count of the caller-supplied title (mirrors
	//                Requested for symmetry).
	//
	// session_id is NOT in Meta on Failed — the id allocation may not have
	// succeeded; recording an empty string would mislead the forensic walker.
	EventSessionsCreateFailed = "sessions.create.failed"

	// EventSessionsRenameRequested fires when pkg/sessions.Rename is about
	// to flip a session's manifest title + bump UpdatedAt. Sibling of the
	// Create trio. The Requested row commits BEFORE the manifest read so a
	// crash mid-call still leaves the rename intent in the audit substrate.
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   session_id — caller-supplied identifier (safe — opaque random hex).
	//
	// The old title and the new title are NEVER in Meta — same PII
	// discipline as EventSessionsCreateRequested. A title_len for the
	// new title is recorded on Succeeded but not Requested (the new
	// title's content was the caller's input; its length sits on the
	// post-write row alongside the resulting state).
	EventSessionsRenameRequested = "sessions.rename.requested"

	// EventSessionsRenameSucceeded fires when pkg/sessions.Rename returns
	// OK. Sibling of Requested above.
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   session_id    — caller-supplied identifier
	//   new_title_len — rune count of the new title (PII-safe forensic
	//                   discriminator; the bytes themselves never appear).
	EventSessionsRenameSucceeded = "sessions.rename.succeeded"

	// EventSessionsRenameFailed fires when pkg/sessions.Rename returns a
	// non-OK Result (nil core, empty id, empty title, session not found,
	// manifest write failure). Sibling of Requested above.
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   session_id — caller-supplied identifier (may be empty when the
	//                empty-id guard fired before any state was touched).
	//   error_code — bounded-keyspace failure literal via audit.ErrorCode(r)
	//                (NEVER raw Result.Error() — substrate contract above).
	EventSessionsRenameFailed = "sessions.rename.failed"

	// EventSessionsDeleteRequested fires when pkg/sessions.Delete is about
	// to issue the store.delete pair (messages + manifest). Sibling of the
	// Create trio. Delete is idempotent on a non-existent id — the
	// Requested row commits regardless so a forensic walker can correlate
	// caller intent against registry state at the moment of the call.
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   session_id — caller-supplied identifier (safe — opaque random hex).
	EventSessionsDeleteRequested = "sessions.delete.requested"

	// EventSessionsDeleteSucceeded fires when pkg/sessions.Delete returns
	// OK (both store.delete calls landed; idempotent on absent id).
	// Sibling of Requested above.
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   session_id — caller-supplied identifier
	EventSessionsDeleteSucceeded = "sessions.delete.succeeded"

	// EventSessionsDeleteFailed fires when pkg/sessions.Delete returns a
	// non-OK Result (nil core, empty id, store.delete failure on either
	// group). Sibling of Requested above.
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   session_id — caller-supplied identifier (may be empty)
	//   error_code — bounded-keyspace failure literal via audit.ErrorCode(r)
	//                (NEVER raw Result.Error() — substrate contract above).
	EventSessionsDeleteFailed = "sessions.delete.failed"

	// EventSessionsAppendMessageRequested fires when pkg/sessions.Append
	// is about to read-modify-write a session's message log (append one
	// message + refresh manifest). Sibling of the Create trio. The
	// Requested row commits BEFORE the read so a crash mid-call still
	// leaves the append intent in the audit substrate.
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   session_id  — caller-supplied identifier (safe — opaque random hex).
	//   role        — closed-set message role literal ("user" / "assistant"
	//                 / "system"). Safe — the inference contract bounds the
	//                 role vocabulary at the runner boundary; the audit
	//                 surface records the role-class only, never bytes the
	//                 role identifies. Forensically useful for reconstructing
	//                 turn-counts per actor without reading the message log.
	//   content_len — byte count of the message content (NOT the content
	//                 bytes themselves — chat messages routinely carry user
	//                 PII, system_prompt material, tool-call payloads, and
	//                 occasional bearer tokens echoed from upstream errors).
	//                 The length pairs with the manifest Snippet (already
	//                 capped + redacted at the manifest layer) when an
	//                 auditor needs message-size accounting.
	//
	// The message content bytes are NEVER in Meta — Cerberus #1465
	// closure-only-scope discipline plus the SECURITY-NOTE escape valve
	// in the F-2 brief. The PII-leak canary test in audit_test.go is the
	// load-bearing assertion that future regressions cannot reintroduce
	// content into Meta.
	EventSessionsAppendMessageRequested = "sessions.append_message.requested"

	// EventSessionsAppendMessageSucceeded fires when pkg/sessions.Append
	// returns OK (message-log write + manifest write both landed).
	// Sibling of Requested above.
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   session_id  — caller-supplied identifier
	//   role        — closed-set role literal (mirrors Requested)
	//   content_len — byte count of the appended message content (mirrors
	//                 Requested for symmetry).
	EventSessionsAppendMessageSucceeded = "sessions.append_message.succeeded"

	// EventSessionsAppendMessageFailed fires when pkg/sessions.Append
	// returns a non-OK Result (nil core, read-failure on the message log,
	// write-failure on the message log, manifest read-failure, manifest
	// write-failure). Sibling of Requested above.
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   session_id  — caller-supplied identifier
	//   role        — closed-set role literal (mirrors Requested)
	//   error_code  — bounded-keyspace failure literal via audit.ErrorCode(r)
	//                 (NEVER raw Result.Error() — substrate contract above).
	EventSessionsAppendMessageFailed = "sessions.append_message.failed"

	// EventCompositionServicesRegistered fires ONCE per boot at the end of
	// cmd/lthn/app.go newAppCore, after every Core service + Wails-binding
	// surface has been wired and ServiceStartup has returned OK. Closes
	// Cerberus #70 F-2 MED — STRIDE-R Repudiation + A1 forensic substrate
	// gap. Pre-cascade the forensic question "which services were live at
	// boot when X happened?" could not be answered from the audit trail —
	// service-set composition was the upstream variant of the
	// [[feedback_action_bus_zero_audit_class]] pattern (action-bus dispatch
	// has zero default audit; service-set composition likewise dark).
	// This row gives a forensic walker the canonical at-boot fingerprint
	// for both the Core service-bus and the Wails-bindings surface (the TS
	// renderer's reachable method-id set), so a later compromise / drift
	// claim can be pinned against the row's hashes.
	//
	// One row per boot — trivial cost, closes the forensic question. Use
	// audit.Default().Record (NOT RecordSync) — boot-time, the fsync
	// latency belongs to auth events not enumeration.
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   service_count       — int count of Core-registered services
	//                         (matches c.Services() length post-startup).
	//                         Forensic discriminator pairs with
	//                         service_names_hash for fingerprinting.
	//   service_names_hash  — 16-char hex prefix of SHA-256 over the
	//                         sorted, comma-joined service-name list
	//                         (lexicographic). Stable across boots when
	//                         the registered-service set is unchanged;
	//                         any drift (add / remove / rename) flips
	//                         the hash. Hashed not raw — keeps Meta size
	//                         constant regardless of how many services
	//                         exist. Truncated to 16 chars (64 bits) so
	//                         pkg/audit/redact.go's 32-char secret-shape
	//                         floor does not flag the value as token-
	//                         shaped; same idiom as pkg/plugin/proxy.go
	//                         + pkg/recordfile/atrest_schema.go.
	//   wails_binding_count — int count of Wails-binding services exposed
	//                         to the renderer (the application.Service
	//                         list pkg/desktop/desktop.go assembles at
	//                         buildServerOpts time). Forensic
	//                         discriminator for the renderer-reachable
	//                         method-id surface.
	//   wails_bindings_hash — 16-char hex prefix of SHA-256 over the
	//                         sorted, comma-joined Wails binding-name
	//                         list. Mirrors service_names_hash discipline
	//                         for the renderer surface (same 64-bit
	//                         truncation rationale). The binding-name
	//                         catalogue lives in cmd/lthn/app.go alongside
	//                         the emit-site (pkg/desktop's wailsServices
	//                         list is intentionally NOT introspected —
	//                         the catalogue here IS the forensic contract;
	//                         if pkg/desktop adds a binding without
	//                         updating the catalogue here, the resulting
	//                         hash drift IS the regression signal per the
	//                         H#241 SECURITY-NOTE escape valve).
	//   version             — lthn binary release tag (dappco.re/lthn/
	//                         desktop.Version; "dev" for unstamped builds,
	//                         "vX.Y.Z(-N-gSHA[-dirty])" for tagged
	//                         releases). Forensic join-key against the
	//                         release notes / SBOM for a given boot.
	//
	// No raw secret-shape material is in Meta — service names, binding
	// names, counts, hashes, and the build tag are all bounded-vocabulary
	// public identifiers. The secret-shape redactor at recordCommon
	// runs over Meta unconditionally as a defence-in-depth gate.
	EventCompositionServicesRegistered = "composition.services.registered"

	// EventViPRFetchRequested fires when pkg/vi.Fetch is about to dispatch
	// the HTTPS GET to a watched repo's open-PR list. Cerberus #72 F-2
	// (Mantis #1749) — STRIDE-R Repudiation gap close. The Vi PR-fetch
	// path was forensically silent prior to this row; the 9th audit-
	// cluster adoption following runner / sandbox / process / marketplace /
	// gateway / downloader / queue / sessions. Requested commits BEFORE
	// the network round-trip so a forensic walker can correlate caller
	// intent (which repo, when) with the eventual Succeeded / Failed /
	// Rejected outcome.
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   provider   — backend literal ("forge" or "github"). Closed-set per
	//                pkg/vi.ProviderForge / ProviderGitHub; safe.
	//   repo_owner — repo owner (public metadata from watched-repo config;
	//                e.g. "lthn", "LetheanNetwork"). Safe.
	//   repo_name  — repo name (public metadata; e.g. "desktop"). Safe.
	//   token_set  — bool, mirrors the existing pr_fetch.go:127 log shape.
	//                NEVER the token bytes — closure-only-scope discipline
	//                per Cerberus #1465. Even a hashed token is forbidden:
	//                the bool is the forensic signal (auth attempted vs
	//                not), the value never reaches the substrate.
	//
	// The raw URL / query-string is NEVER in Meta — Forgejo + GitHub PR
	// list URLs are structurally derivable from (provider, owner, name),
	// keeping the substrate row idempotent without round-tripping any
	// query bytes that could embed pagination cursors / state markers.
	EventViPRFetchRequested = "vi.pr_fetch.requested"

	// EventViPRFetchSucceeded fires when pkg/vi.Fetch completes the
	// upstream round-trip + per-row persistence loop with a non-error
	// status (status < 400 from doFetch). Sibling of Requested above.
	// Note: 401 / 403 / other 4xx codes route here too — the upstream
	// responded, the auditor pairs the http_status to derive intent
	// (auth failure vs success vs malformed body).
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   provider    — backend literal (mirrors Requested)
	//   repo_owner  — repo owner (mirrors Requested)
	//   repo_name   — repo name (mirrors Requested)
	//   http_status — int HTTP status code from the upstream response
	//                 (200 / 304 on happy path; 401 / 403 on auth fail;
	//                 forensic discriminator for the response class).
	//   pr_count    — int number of PRActivity rows inserted this tick
	//                 (post-unsafe-URL-drop filter). Forensic for "did
	//                 the fetch produce any rows" without re-querying
	//                 the orm.
	EventViPRFetchSucceeded = "vi.pr_fetch.succeeded"

	// EventViPRFetchFailed fires when pkg/vi.Fetch hits a transport-side
	// error from doFetch (network / TLS / parse failure) so the row
	// loop is skipped entirely. Sibling of Requested above. Distinct
	// from Rejected — Failed is "the call could not complete";
	// Rejected is "policy refused one or more rows".
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   provider    — backend literal (mirrors Requested; may be empty
	//                 when the surface guard fired before provider was
	//                 set, but Fetch's empty-owner guard runs first so
	//                 in practice provider is always populated here).
	//   repo_owner  — repo owner
	//   repo_name   — repo name
	//   http_status — int HTTP status code (0 when no response was
	//                 produced — e.g. DNS / TLS / dial failure).
	//   error_code  — bounded-keyspace failure literal via audit.ErrorCode(r)
	//                 substrate (pkg/audit/errorcode.go). The doFetch
	//                 transport-error string is NEVER recorded raw —
	//                 the substrate derives a stable canon so caller-
	//                 controlled URLs / TLS chain bytes cannot leak.
	EventViPRFetchFailed = "vi.pr_fetch.failed"

	// EventViPRFetchRejected fires when pkg/vi.Fetch drops a per-row
	// upstream PR because its html_url is not https:// (pr_fetch.go:148
	// isSafePRURL gate). This is the SECURITY-RELEVANT signal — a
	// compromised or malicious upstream forge returning `javascript:`
	// or `data:` URLs would land here, and an auditor can grep for
	// reason="unsafe_url" to surface attempted XSS-via-upstream events
	// without scanning every Succeeded row.
	//
	// One row per dropped PR — a flood of rejected rows in a single
	// fetch is itself a forensic signal (compromise / hostile upstream).
	//
	// Closed reason set (Meta["reason"]):
	//
	//   unsafe_url — html_url did not start with https:// (the only
	//                rejection class today; future per-row gates would
	//                add closed-set literals here).
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   provider   — backend literal (mirrors Requested)
	//   repo_owner — repo owner
	//   repo_name  — repo name
	//   pr_number  — int upstream PR number (forensic discriminator;
	//                pairs with provider+owner+name to identify the
	//                offending row in upstream's PR list).
	//   reason     — one of the closed-set literals above.
	//
	// The dropped URL bytes are NEVER in Meta — the raw URL is the
	// hostile payload (e.g. `javascript:fetch(...)`); recording it
	// would persist the attack string into a long-lived forensic
	// surface. The (provider, owner, name, pr_number) tuple is
	// sufficient to identify the offending upstream row for an
	// auditor who needs to re-query the source.
	EventViPRFetchRejected = "vi.pr_fetch.rejected"

	// EventViPRTokenLoaded fires inside pkg/vi.Fetch immediately after
	// LoadToken returns, recording the token-state observation (set vs
	// not-set). Single event (no Requested/Succeeded/Failed trio) —
	// LoadToken's outcome is binary at the audit substrate; failure
	// modes (config service absent, key absent) all collapse to "token
	// not set". Closes the forensic question "when was the watched
	// repo's PAT first read into memory after disk-write?" raised in
	// Cerberus #72 F-2 (Mantis #1749). Compounds the F-1 #1748 gap
	// (no at-rest crypto on the token) by giving a load-bearing timeline
	// row that pairs with any future at-rest decrypt event.
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   provider  — backend literal (closed-set; ties the observation to
	//               a specific watched-token namespace).
	//   token_set — bool. NEVER the token value, NEVER a hash of the
	//               value — the bool is sufficient forensic signal per
	//               the closure-only-scope discipline. PII-leak canary
	//               test in pkg/vi/audit_test.go pins this contract.
	EventViPRTokenLoaded = "vi.pr_fetch.token_loaded"

	// EventViTokenMigrated fires inside pkg/vi.MigrateLegacyTokens once
	// per provider whose legacy `vi.tokens.<provider>` config literal is
	// dispatched into the pkg/keys tier-1 substrate under ref
	// `vi-<provider>` and the legacy literal blanked. Closes the Stage E
	// at-rest sealing requirement (Mantis #1748): a forensic walker can
	// correlate "plaintext token disappeared from disk at T" with "the
	// ciphertext was sealed at T-ε".
	//
	// Single-shot per provider — once migrated the source literal is
	// empty, the scan skips that provider on subsequent calls, so the
	// row only fires on the actual at-rest mutation tick. Pair with the
	// existing EventViPRTokenLoaded which fires on every PR-fetch read
	// regardless of resolution path.
	//
	// Meta keys (secret-shape redactor enforced):
	//
	//   provider — backend literal (forge / github / closed-set).
	//   ref      — tier-1 ref the plaintext was sealed under, of shape
	//              `vi-<provider>`. NEVER the token bytes; ref alone is
	//              sufficient to walk back to the keys.Service audit
	//              cluster for the EventKeysTier1Stored partner row.
	EventViTokenMigrated = "vi.token.migrated"

	// EventViProbeRequested fires when pkg/vi.Probe is about to dispatch
	// the HTTPS GET to a catalogued site. Sibling of EventViPRFetchRequested.
	// Requested commits BEFORE the network round-trip so a forensic walker
	// can correlate caller intent (which domain, when) with the eventual
	// Succeeded / Failed outcome. Probe lacks a Rejected event because
	// the per-row gate (https-only via probeClient.CheckRedirect) routes
	// through Failed rather than a policy-gate rejection — the redirect
	// refusal is a transport error from probeClient.Do, not a per-row
	// filter outcome.
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   domain — canonical site identifier (host only, no scheme / path).
	//            Public metadata — the configured probe set is curated,
	//            never user-typed at the probe-fire path; safe.
	EventViProbeRequested = "vi.probe.requested"

	// EventViProbeSucceeded fires when pkg/vi.Probe completes a probe
	// (probeClient.Do returned without transport error). Note: "Succeeded"
	// here is "the probe ran cleanly", NOT "the upstream returned 2xx" —
	// the probe.OK derivation lives in the SiteProbe row; the audit row
	// records the probe ran. 4xx / 5xx upstream responses route here too,
	// the auditor pairs http_status to derive the response class.
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   domain      — canonical site identifier (mirrors Requested)
	//   http_status — int HTTP status code from the upstream response
	//                 (200 on healthy; 503 on degraded; 0 on transport
	//                 error — but transport errors route to Failed).
	//   latency_ms  — int round-trip in milliseconds. Forensic for the
	//                 site-uptime story without re-reading the orm row.
	EventViProbeSucceeded = "vi.probe.succeeded"

	// EventViProbeFailed fires when pkg/vi.Probe hits a transport error
	// (probeClient.Do returned non-nil err — timeout, TLS failure,
	// non-https redirect refusal, dial error). Sibling of Requested above.
	// The Probe surface ALSO routes through Failed when the surface
	// guards (nil-core, non-https URL) fire OR the orm.Insert at the
	// bottom returns non-OK (genuinely broken — orm down / disk full)
	// so an auditor sees the failure regardless of which layer produced
	// it.
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   domain     — canonical site identifier (mirrors Requested; may be
	//                empty when the URL-shape guard fired before domain
	//                derivation, but DomainOf is total over any non-
	//                empty https URL).
	//   error_code — bounded-keyspace failure literal via audit.ErrorCode(r)
	//                substrate. Raw doProbe error string is NEVER recorded
	//                — the substrate derives a stable canon so caller-
	//                controlled URLs / TLS chain bytes cannot leak.
	EventViProbeFailed = "vi.probe.failed"

	// EventTrayPluginClicked fires when the tray-menu click router in
	// pkg/desktop dispatches a plugin-rooted ActionID (one tagged with
	// trayPluginPrefix). Closes Cerberus #70 F-3 MED — STRIDE-T Tampering
	// + STRIDE-R Repudiation. Pre-cascade the plugin enumeration surface
	// on the system tray was forensic-dark: a hostile manifest could
	// surface a plugin code via Menus() and the click handler would route
	// to openPluginWindow without a substrate row recording that the
	// renderer opened a (potentially attacker-named) plugin view. The
	// click router now ALSO validates the resolved code against
	// paths.IsValidPluginCode before opening + before this emit, so a
	// hostile entry that slipped past the menu-build filter (race
	// between Menus() snapshot and click resolution) still cannot reach
	// the renderer.
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   plugin_code   — validated plugin code that the click resolved
	//                   to (post-trayPluginPrefix-strip, post-
	//                   paths.IsValidPluginCode gate). Bounded vocab
	//                   per the validator (lowercase ascii + dash +
	//                   underscore, 1..MaxPluginCodeBytes=64 bytes).
	//   resolved_via  — literal "tray_menu" — origin discriminator. A
	//                   later instrumentation lane covering Dock-menu
	//                   or popover-menu click surfaces can re-use the
	//                   event with a distinct literal, keeping the
	//                   audit substrate a single forensic-grep target.
	//
	// The plugin label is NEVER in Meta — labels are attacker-controlled
	// strings out of the plugin manifest. The Cerberus #70 finding
	// names control-char and length injection as the load-bearing risk;
	// recording the label would persist the attack string into the
	// long-lived forensic substrate (sibling discipline to the URL
	// bytes drop in EventViPRFetchRejected). The plugin code is
	// bounded-vocab and forensically equivalent for a walker — it joins
	// 1:1 against the installed-plugins manifest catalogue.
	EventTrayPluginClicked = "tray.plugin.clicked"

	// EventDesktopSecondInstanceFallback fires when the OnSecondInstanceLaunch
	// handler in pkg/desktop cannot bring an existing "app" OR "tray" window
	// forward (both restoreFocusedWindow calls returned false) and falls
	// through to a window.open create-and-show as a last-resort UX path.
	// Closes Cerberus #70 F-4 LOW — STRIDE-R Repudiation / defence-in-depth
	// (UX + forensic). Pre-fix the silent-no-op path emitted only the
	// lthn:app:second-instance bus event for renderer consumers to act on
	// the incoming Args / WorkingDir / AdditionalData payload, while no
	// window came forward visibly. An operator clicking the second binary
	// would observe nothing and might launch a third — and a forensic
	// walker had no row recording that the fallback path engaged.
	//
	// The fallback path itself is operator-safety value-add (a visible
	// window is preferable to a silent dead click). The audit row is the
	// observability value-add: every time the fallback engages the
	// substrate carries a row a walker can grep for, regardless of
	// whether the create-and-show itself succeeded.
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   primary_targets — comma-joined ordered list of the window names
	//                     the handler tried via restoreFocusedWindow
	//                     before falling through. Always "app,tray" for
	//                     the current handler shape — recorded literally
	//                     so a future second-instance-target evolution
	//                     can be reasoned about from the substrate.
	//   fallback_target — name of the window the fallback path attempts
	//                     to open via window.open. Literal "app" today
	//                     (the unified Lethean Desktop shell — the primary
	//                     operator-facing registry-backed surface);
	//                     reserved as a key so a future fallback evolution
	//                     remains grep-compatible.
	//   fallback_via    — literal "window.open" — origin discriminator.
	//                     Mirrors the resolved_via convention on
	//                     EventTrayPluginClicked — keeps a forensic walker
	//                     able to filter fallback rows by mechanism even
	//                     once the substrate accumulates sibling fallback
	//                     paths (e.g. Dock click, lthn:// URL fallthrough).
	//
	// The SecondInstanceData payload (Args, WorkingDir, AdditionalData)
	// is NEVER in Meta — those bytes are caller-controlled and would
	// defeat the bounded-keyspace promise the audit substrate enforces
	// (sibling discipline to the plugin-label drop above + the URL bytes
	// drop in EventViPRFetchRejected). The handler still emits the
	// payload onto the lthn:app:second-instance event bus where the
	// renderer can consume it; the audit row records the OBSERVED FACT
	// that the fallback engaged, not the input bytes that triggered it.
	EventDesktopSecondInstanceFallback = "desktop.second_instance.fallback"

	// EventProviderCredentialStored fires when pkg/runner stores a
	// provider API key into the tier-1 KEK-wrapped substrate via the
	// WSetDynamicRoutes auto-migration path OR the auto-import-on-boot
	// path (pkg/runner/migration.go). Mantis #1657 / RFC.provider-creds-
	// tier1.md v1.1 §7 ADD-5 — cluster-canon naming (`.stored` mirrors
	// the established `.stored` family rather than the v1 draft `.put`).
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   provider — short provider name (e.g. "openai", "anthropic") derived
	//              from the route entry the credential was attached to
	//   ref      — opaque tier-1 reference handle (e.g. "openai-default");
	//              this is the audit handle, NEVER the credential bytes
	//   source   — origin discriminator: "wails_input" for WSetDynamicRoutes
	//              auto-migration; "config" for boot-time import-and-strip
	//
	// The credential bytes are NEVER in Meta — Cerberus #1465 closure-
	// only-scope discipline keeps the plaintext off the audit substrate;
	// this event records the storage decision, not the credential.
	EventProviderCredentialStored = "provider.credential.stored"

	// EventProviderCredentialMigrated fires when pkg/runner auto-migrates
	// a legacy plaintext `api_key:` field from the config file into the
	// tier-1 substrate, rewriting the config with an opaque `api_key_ref`
	// in place of the plaintext (RFC v1.1 §6 — Option A auto-import-and-
	// strip). Sibling of EventProviderCredentialStored — Migrated fires
	// from the boot-time config-scan path, Stored fires from the Wails-
	// input path; both signal "plaintext is now sealed at-rest".
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   provider — short provider name (route entry the credential
	//              originated from)
	//   ref      — synthesised tier-1 ref (e.g. "openai-migrated")
	//   source   — literal "config" — distinguishes the boot-scan path
	//              from the Wails-input path that emits Stored
	//
	// The legacy plaintext bytes are NEVER in Meta — the migration path
	// reads the plaintext into the tier-1 envelope and then zeroes the
	// config field; the audit row records the transition, not the
	// credential.
	EventProviderCredentialMigrated = "provider.credential.migrated"

	// EventProviderCredentialInvalidated fires when pkg/runner clears
	// a cached resolved-plaintext from a *openai.Backend wrapper in
	// response to a keys.tier1.{deleted,replaced} event-bus broadcast
	// (RFC v1.1 §4 ADD-1 — Cerberus #1742 STRIDE-T survival-past-delete).
	// Without the cache-invalidation, an operator-visible "I deleted it"
	// would diverge from system reality because the deleted credential
	// would survive in process memory until ServiceShutdown.
	//
	// Mantis #1746 / Cerberus #71 ADD-5 — name normalised from
	// `cache_invalidated` to `invalidated` so the verb stays single-
	// word in line with the rest of the provider.credential.* cluster
	// (stored / migrated / deleted / observed). The `cache_` qualifier
	// was redundant: the only thing that can be invalidated here IS
	// the cached resolved plaintext, so it belongs in the const name
	// not the event-name literal.
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   provider — short provider name
	//   ref      — tier-1 ref whose cached plaintext was flushed
	//   reason   — categorical: "deleted" (DeleteTier1 fired) or
	//              "replaced" (PutTier1 fired with an existing ref)
	//
	// The cleared plaintext bytes are NEVER in Meta — the cache-flush
	// path zeroes the cached slice and emits this row; the row records
	// the invalidation decision, not the credential.
	EventProviderCredentialInvalidated = "provider.credential.invalidated"

	// EventProviderCredentialDeleted fires when an operator
	// dispatches pkg/runner.Service.WForceDeleteTier1 with the literal
	// "operator_acknowledged_orphan" confirmation token (RFC v1.1 §5
	// ADD-3 — Cerberus #1744 force-delete escape). The default
	// WDeleteTier1 path REJECTS deletion when any route references the
	// ref; force-delete bypasses that check, which can leave routes in
	// a broken state. The audit row makes the blast-radius visible.
	//
	// Mantis #1746 / Cerberus #71 ADD-5 — name normalised from
	// `force_deleted` to `deleted` so the verb stays single-word in
	// line with the rest of the provider.credential.* cluster (stored
	// / migrated / invalidated / observed). The "force" discriminator
	// belongs in Meta (the literal operator_confirmation token already
	// carries it explicitly: "operator_acknowledged_orphan"), not in
	// the event-name literal — a future non-force deletion path would
	// emit the same event-name with a different Meta shape rather than
	// fork a new cluster member.
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   provider              — short provider name
	//   ref                   — tier-1 ref that was force-deleted
	//   referenced_by         — comma-joined ordered list of route names
	//                           that held a reference to the deleted ref
	//                           at delete-time; empty string when the
	//                           force-delete was issued against an
	//                           unreferenced ref (still recorded so a
	//                           forensic walker can grep for force-delete
	//                           invocations regardless of blast radius)
	//   operator_confirmation — literal "operator_acknowledged_orphan"
	//                           (the only accepted token). Recorded
	//                           literally so a future token-evolution
	//                           can be reasoned about from the substrate.
	//                           This field IS the "force-ness" signal
	//                           that the event-name used to carry.
	//
	// The credential bytes are NEVER in Meta — DeleteTier1 removes the
	// ciphertext from disk and emits this row; the row records the
	// deletion decision, not the credential.
	EventProviderCredentialDeleted = "provider.credential.deleted"

	// EventProviderCredentialMigrationPendingObserved fires once per
	// boot when pkg/runner detects routes with plaintext `api_key:`
	// fields still on disk AND the tier-1 KEK provider is not yet live
	// (the account has not unlocked, so auto-migration is deferred per
	// RFC v1.1 §6). Drives the persistent banner per RFC §6 ADD-4 —
	// gives external observability the same signal the operator sees
	// in the global app-chrome banner ("N provider credentials are
	// stored as plaintext. Unlock your account to migrate them to
	// secure storage.").
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   count — number of route entries with non-empty `api_key:` AND
	//           empty `api_key_ref:` observed at boot-scan time
	//
	// No provider/ref keys — this row records the aggregate pending
	// state, not per-route detail. The per-route migration emit is
	// EventProviderCredentialMigrated, which fires AFTER the unlock
	// completes and the deferred migration drains.
	EventProviderCredentialMigrationPendingObserved = "provider.credential.migration_pending_observed"

	// EventProviderCredentialStoredOrphan fires when pkg/runner's
	// WSetDynamicRoutes two-phase commit attempts to roll back a
	// previously-stored tier-1 ref (after a sibling PutTier1 failed
	// mid-loop OR the post-commit saveRouteConfigsToCore failed) AND
	// the rollback DeleteTier1 itself fails — leaving a tier-1
	// ciphertext at rest with no RouteConfig binding (Mantis #1760 /
	// Cerberus #75 ADD-1 option (b) safety-net).
	//
	// Without this row the orphan ciphertext would survive silently;
	// the operator-retry path then dispatches PutTier1 again, which
	// fires Tier1KeyChanged{Kind: Tier1KeyReplaced} — visually
	// indistinguishable from a routine credential rotation in the
	// audit log. The Orphan row makes the partial-failure path
	// distinguishable + grep-able for forensic walkers.
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   ref    — opaque tier-1 reference handle that survived the
	//            rollback attempt; this is the audit handle, NEVER
	//            the credential bytes
	//   reason — Error().String() of the DeleteTier1 cause (file-
	//            permission denial, disk failure, etc.); recorded so
	//            an incident review can correlate the orphan with the
	//            underlying substrate failure
	//
	// The credential bytes are NEVER in Meta — the tier-1 ciphertext
	// stays sealed under the tier-1 master; this row records the
	// fact-of-orphan, not the credential.
	EventProviderCredentialStoredOrphan = "provider.credential.stored_orphan"

	// EventKeysTier0Stored fires when pkg/keys.Service.PutTier0
	// successfully writes a tier-0 (pre-unlock substrate) ciphertext
	// to disk. Tier-0 holds machine-local secrets that survive across
	// account-lock cycles (today: the SingleInstance IPC key the
	// Wails second-instance bridge authenticates against — Mantis
	// #1625 / Cerberus #1442). Mantis #1763 / Cerberus #77 F-1 —
	// 10th audit-cluster adoption; closes the STRIDE-R Repudiation +
	// A2/A3 audit-trail gap on operator-tier credential-mutation
	// verbs in pkg/keys.
	//
	// The broadcastTier1Change event-bus broadcast in this package
	// is a consumer-cache flush signal — NOT an audit substrate; the
	// forensic-trail discipline this row provides is orthogonal to
	// that pub/sub channel.
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   ref    — opaque tier-0 reference handle (e.g. "single-
	//            instance"); this is the audit handle, NEVER the
	//            key bytes or any derived material
	//   kind   — literal "tier0" — pins the partition the mutation
	//            landed in so a forensic walker can grep cleanly
	//   tier   — duplicate of kind, retained as a stable cluster-
	//            canon meta key per the keys.* family discipline
	//   source — origin discriminator: "internal" for Go-side calls
	//            (boot SingleInstanceKey, GetOrCreateTier0); the
	//            tier-0 surface has no Wails binding by design
	//            (RFC.stage-e-keys-partition §4.3 Q4) so "wails_input"
	//            never appears for this event
	//
	// The key bytes are NEVER in Meta — Cerberus #1465 closure-only-
	// scope discipline keeps the plaintext off the audit substrate;
	// this row records the storage decision, not the credential.
	EventKeysTier0Stored = "keys.tier0.stored"

	// EventKeysTier0Deleted fires when pkg/keys.Service.DeleteTier0
	// actually removes a tier-0 ciphertext (the non-idempotent path —
	// the no-op variant where the file did not exist emits nothing,
	// mirroring the broadcastTier1Change discipline on the tier-1
	// twin DeleteTier1 at service.go:1394). Mantis #1763 / Cerberus
	// #77 F-1.
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   ref    — opaque tier-0 reference handle that was removed
	//   kind   — literal "tier0"
	//   tier   — duplicate of kind (cluster-canon meta key)
	//   source — "internal" (no Wails surface for tier-0 per
	//            RFC.stage-e-keys-partition §4.3 Q4)
	//
	// The key bytes are NEVER in Meta — DeleteTier0 unlinks the
	// ciphertext from disk and emits this row; the row records the
	// deletion decision, not the credential.
	EventKeysTier0Deleted = "keys.tier0.deleted"

	// EventKeysTier1Stored fires when pkg/keys.Service.PutTier1
	// successfully writes a tier-1 (post-unlock provider-credential)
	// ciphertext to disk AND no prior ciphertext existed under the
	// same ref (the "first-write" path; the EXISTING-ref overwrite
	// path emits EventKeysTier1Replaced instead). The wasPresent
	// gate at service.go:1332 distinguishes the two shapes.
	//
	// Sibling of EventProviderCredentialStored (which fires from the
	// pkg/runner Wails-input + boot-scan paths in addition to this
	// row) — pkg/keys emits at the at-rest mutation site; pkg/runner
	// emits at the higher-level credential-import decision site;
	// both rows land for the same Wails-input flow so the forensic
	// trail covers the entire pipeline. Mantis #1763 / Cerberus #77
	// F-1.
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   ref    — opaque tier-1 reference handle (e.g. "openai-
	//            default"); this is the audit handle, NEVER the
	//            credential bytes
	//   kind   — literal "tier1"
	//   tier   — duplicate of kind (cluster-canon meta key)
	//   source — origin discriminator: "wails_input" for calls
	//            via WPutTier1 (renderer-reachable surface),
	//            "internal" for Go-side calls (GetOrCreateTier1,
	//            cmd/lthn wiring)
	//
	// The credential bytes are NEVER in Meta — Cerberus #1465
	// closure-only-scope discipline keeps the plaintext off the
	// audit substrate; this row records the storage decision, not
	// the credential.
	EventKeysTier1Stored = "keys.tier1.stored"

	// EventKeysTier1Replaced fires when pkg/keys.Service.PutTier1
	// successfully writes a tier-1 ciphertext over an existing ref
	// (the wasPresent path at service.go:1342 that also fires the
	// Tier1KeyChanged{Kind: Tier1KeyReplaced} consumer-cache flush
	// broadcast). Distinguished from EventKeysTier1Stored so a
	// forensic walker can grep the rotate-credential path separately
	// from the first-write path; without this split, every overwrite
	// would be visually indistinguishable from a fresh storage.
	// Mantis #1763 / Cerberus #77 F-1.
	//
	// Meta keys: identical shape to EventKeysTier1Stored (ref / kind
	// / tier / source). The two events share Meta shape on purpose —
	// the only difference at the audit-row level is the event name
	// reflecting first-write vs overwrite.
	EventKeysTier1Replaced = "keys.tier1.replaced"

	// EventKeysTier1Deleted fires when pkg/keys.Service.DeleteTier1
	// actually removes a tier-1 ciphertext (the non-idempotent path
	// at service.go:1406 that also fires the Tier1KeyChanged{Kind:
	// Tier1KeyDeleted} consumer-cache flush broadcast). The no-op
	// idempotent variant — DeleteTier1 called against a ref with no
	// ciphertext on disk — emits nothing, mirroring the broadcast
	// gate.
	//
	// Sibling of EventProviderCredentialDeleted: that row fires
	// from the pkg/runner WForceDeleteTier1 escape path with the
	// `referenced_by` blast-radius Meta; this row fires from the
	// default-path DeleteTier1 at the at-rest substrate level.
	// Mantis #1763 / Cerberus #77 F-1.
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   ref    — opaque tier-1 reference handle that was removed
	//   kind   — literal "tier1"
	//   tier   — duplicate of kind (cluster-canon meta key)
	//   source — "wails_input" for calls via WDeleteTier1
	//            (renderer-reachable surface), "internal" for Go-
	//            side calls
	//
	// The credential bytes are NEVER in Meta — DeleteTier1 unlinks
	// the ciphertext from disk and emits this row; the row records
	// the deletion decision, not the credential.
	EventKeysTier1Deleted = "keys.tier1.deleted"

	// EventOpencodeImageSignatureVerified fires when pkg/opencode.Service.
	// UpgradeWithConsent successfully verifies the operator-supplied
	// ed25519 signature over the canonical pull bytes (digest + tag +
	// release_id) BEFORE the actual docker pull side-effect runs.
	// Cerberus #22 MED-2 / Mantis #1622 — the supply-chain trust chain
	// previously ended at the registry; pin-by-digest blocks a tag-swap
	// attack but does not protect against the case where the attacker
	// controls both the registry AND the pin-registration path. Binding
	// the digest to a release-engineer key the operator pins out-of-band
	// via ~/Lethean/conf/opencode/trusted_publishers.json closes the
	// loop.
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   image_digest — the sha256:<64hex> manifest digest that was
	//                   signature-verified
	//   keyid        — the trusted publisher keyid that the signature
	//                   verified under (alias resolved via
	//                   trusted_publishers.json; never raw pubkey bytes)
	//
	// The signature bytes themselves are NEVER in Meta — the row records
	// the verification decision, not the credential or signing material.
	EventOpencodeImageSignatureVerified = "opencode.image.signature_verified"

	// EventOpencodeImageSignatureRejected fires when pkg/opencode.Service.
	// UpgradeWithConsent rejects the supplied signature. Reasons cover
	// the closed-set surface from pkg/marketplace.Verify (sig.corrupt
	// for wrong-length / encoding error, sig.invalid for parses-but-
	// mismatch) plus a "key_not_found" facet when the manifest's KeyID
	// is not present in trusted_publishers.json and a "signature_missing"
	// facet when require_signature=true but no signature was supplied.
	//
	// Cerberus #22 MED-2 / Mantis #1622 — the row makes signature-
	// verification failures grep-able from the audit log so a forensic
	// walker can rank "registry served signed-but-untrusted artefact"
	// above "user clicked update on the wrong release".
	//
	// Meta keys (RFC §2.1, secret-shape redactor enforced):
	//
	//   image_digest — the sha256:<64hex> manifest digest that failed
	//                   verification
	//   keyid        — the manifest's declared KeyID (may be empty when
	//                   reason="signature_missing")
	//   reason       — one of: sig.corrupt / sig.invalid / key_not_found
	//                   / signature_missing
	//   error_code   — substrate-derived per audit.ErrorCode (canonical
	//                   "opencode.SignatureVerify" scope)
	//   error_scope  — substrate-derived per audit.ErrorScope (Mantis
	//                   #1720 v2 dual-key)
	//
	// The signature bytes themselves are NEVER in Meta — the row records
	// the verification decision, not the credential or signing material.
	EventOpencodeImageSignatureRejected = "opencode.image.signature_rejected"
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
