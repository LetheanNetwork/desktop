// SPDX-Licence-Identifier: EUPL-1.2
//
// audit-constants.ts — TS mirror of the Go const block in
// go/pkg/audit/types.go (RFC.stage-e-audit-viewer v2 §5.6 + Q6).
//
// PARITY-LIST DISCIPLINE: every event-name + outcome literal the
// backend reserves MUST appear in the arrays below. The golden-list
// test in audit-constants.test.ts greps the Go source at
// `go/pkg/audit/types.go` and asserts the two literal sets match
// exactly. Adding a new const in Go without updating these arrays
// fails the test with the prompt:
//
//   "audit constants drifted — re-mirror from Go and update this golden."
//
// Hand-maintained on purpose — automatic codegen would obscure the
// reserved-schema-change discipline. The chip-filter UI in
// <lthn-audit-viewer> reads from these arrays so the operator only
// ever sees the closed surface (no free-text event input).
//
// Usage:
//
//   import { AUDIT_EVENT_NAMES, AUDIT_OUTCOMES } from "./audit-constants";
//   for (const ev of AUDIT_EVENT_NAMES) { /* render chip */ }

/** Reserved audit event-name literals — mirror of the const block at
 *  the top of `go/pkg/audit/types.go` starting at line 128.
 *
 *  Order matches the Go file source order so a diff-side review is
 *  cheap. The golden-list test asserts set-equality, not order. */
export const AUDIT_EVENT_NAMES = [
  "auth.unlock.failed",
  "auth.lockout.triggered",
  "auth.lock.requested",
  "auth.session.issued",
  "auth.session.verify_failed",
  "auth.account.provisioned",
  "auth.account.created",
  "auth.account.sealed",
  "auth.account.seal_failed",
  "auth.tier.reject",
  "auth.tier.rejectable_transitional",
  "plugin.view.capability_granted",
  "inference.generate.requested",
  "inference.generate.completed",
  "inference.generate.failed",
  "inference.chat.requested",
  "inference.chat.completed",
  "inference.chat.failed",
  "sandbox.spawn.requested",
  "sandbox.spawn.succeeded",
  "sandbox.spawn.failed",
  "sandbox.long.requested",
  "sandbox.long.succeeded",
  "sandbox.long.failed",
  "sandbox.kill.requested",
  "sandbox.kill.succeeded",
  "sandbox.kill.failed",
  "sandbox.volume.rejected",
  "sandbox.spawn.rejected",
  "process.run.requested",
  "process.run.succeeded",
  "process.run.failed",
  "process.start.requested",
  "process.start.succeeded",
  "process.start.failed",
  "process.kill.requested",
  "process.kill.succeeded",
  "process.kill.failed",
  "marketplace.install.requested",
  "marketplace.install.succeeded",
  "marketplace.install.failed",
  "marketplace.uninstall.requested",
  "marketplace.uninstall.succeeded",
  "marketplace.uninstall.failed",
  "marketplace.launch.requested",
  "marketplace.launch.succeeded",
  "marketplace.launch.failed",
  "marketplace.stop.requested",
  "marketplace.stop.succeeded",
  "marketplace.stop.failed",
  "marketplace.fetch_manifest.requested",
  "marketplace.fetch_manifest.succeeded",
  "marketplace.fetch_manifest.failed",
  "marketplace.fetch_manifest.rejected",
  "marketplace.validate_manifest.failed",
  "marketplace.trusted_key.mutated",
  "marketplace.catalogue.stale_install_attempt",
  "marketplace.manifest.version.pending",
  "marketplace.manifest.version.transition",
  "marketplace.image.digest_mismatch",
  "marketplace.image.digest_unverifiable",
  "marketplace.permission.diff_requires_re_consent",
  "gateway.dispatch.requested",
  "gateway.dispatch.succeeded",
  "gateway.dispatch.failed",
  "gateway.dispatch.rejected",
  "downloader.fetch.requested",
  "downloader.fetch.succeeded",
  "downloader.fetch.failed",
  "downloader.fetch.rejected",
  "queue.job.started",
  "queue.job.succeeded",
  "queue.job.failed",
  "queue.job.cancelled",
  "sessions.create.requested",
  "sessions.create.succeeded",
  "sessions.create.failed",
  "sessions.rename.requested",
  "sessions.rename.succeeded",
  "sessions.rename.failed",
  "sessions.delete.requested",
  "sessions.delete.succeeded",
  "sessions.delete.failed",
  "sessions.append_message.requested",
  "sessions.append_message.succeeded",
  "sessions.append_message.failed",
  "composition.services.registered",
  "vi.pr_fetch.requested",
  "vi.pr_fetch.succeeded",
  "vi.pr_fetch.failed",
  "vi.pr_fetch.rejected",
  "vi.pr_fetch.token_loaded",
  "vi.probe.requested",
  "vi.probe.succeeded",
  "vi.probe.failed",
  "vi.token.migrated",
  "tray.plugin.clicked",
  "desktop.second_instance.fallback",
  "provider.credential.stored",
  "provider.credential.migrated",
  "provider.credential.invalidated",
  "provider.credential.deleted",
  "provider.credential.migration_pending_observed",
  "provider.credential.stored_orphan",
  "keys.tier0.stored",
  "keys.tier0.deleted",
  "keys.tier1.stored",
  "keys.tier1.replaced",
  "keys.tier1.deleted",
  "opencode.image.signature_verified",
  "opencode.image.signature_rejected",
] as const;

export type AuditEventName = (typeof AUDIT_EVENT_NAMES)[number];

/** Reserved outcome literals — mirror of the OutcomeOK/Failed/Denied/
 *  Error const block in `go/pkg/audit/types.go` starting at line 113.
 *  The chip-filter shows exactly these four values per RFC §5.3
 *  (Q3 AFFIRMED STRONGLY). */
export const AUDIT_OUTCOMES = [
  "ok",
  "failed",
  "denied",
  "error",
] as const;

export type AuditOutcome = (typeof AUDIT_OUTCOMES)[number];

/** Reserved Meta keys — mirror of the const block at the top of the
 *  `MetaKey*` declaration in `go/pkg/audit/types.go` (Mantis #1720 v2
 *  dual-key shape). The chip-filter UI in <lthn-audit-viewer> reads
 *  from these keys to surface the closed set of substrate-emitted Meta
 *  values so the operator can group / filter on a stable vocabulary
 *  rather than free-text Meta map keys.
 *
 *  v2 dual-key contract (RFC.error-code-cascade §7 Q-4 v2 / #1720):
 *  every Failed-event substrate emit MUST carry BOTH `error_code`
 *  (canonical Lethean codespace, answers WHY) and `error_scope`
 *  (operation-scope boundary, answers WHAT). v1 cascade emitted a
 *  single `error_code` key that conflated the two semantic axes — v2
 *  splits them so a forensic walker can correlate by either axis. */
export const AUDIT_META_KEYS = [
  "error_code",
  "error_scope",
] as const;

export type AuditMetaKey = (typeof AUDIT_META_KEYS)[number];
