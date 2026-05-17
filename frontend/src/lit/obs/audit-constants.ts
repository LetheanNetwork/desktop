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
  "plugin.view.capability_granted",
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
