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
	//   error_code — categorical failure reason; sourced from the upstream
	//                Result.Error() string (already-scoped via core.E by
	//                the upstream handler).
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
	//   error_code — categorical failure reason (upstream Result.Error()).
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
	//   error_code — categorical failure reason (upstream Result.Error()).
	EventProcessKillFailed = "process.kill.failed"
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
