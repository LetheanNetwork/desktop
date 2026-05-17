// SPDX-Licence-Identifier: EUPL-1.2

// audit.go — Stage F.B Phase 2.4 cross-cut. Subscribes to the
// queue.JobChanged typed bus and projects each event into pkg/audit
// via RecordBatch. Job lifecycles can burst high under heavy queue
// activity so cascade-mode (write + deferred fsync) is the right
// path per RFC §4.2.
//
// Boot wires via queue.AttachAudit(c). Lives next to events.go for
// the same grep-discoverability discipline as pkg/tasks/audit.go.

package queue

import (
	core "dappco.re/go"

	"dappco.re/lthn/desktop/pkg/audit"
)

// AuditEventEnqueued is the reserved audit event-name for the
// PhaseEnqueued lifecycle moment. Sibling literal of the
// audit.EventQueueJob* trio — Enqueued sits before the worker pickup
// (queue accepts the Job) so it lives on its own literal rather than
// the queue.job.* prefix the worker-side trio occupies.
//
// Kept in the pkg/queue namespace (not migrated into pkg/audit/types.go)
// because Enqueued is the queue's caller-facing surface; pkg/queue
// callers can branch on this literal without importing pkg/audit.
// Parity-grep contract (RFC §3.3.1) still sees it via the AttachAudit
// projection below.
const AuditEventEnqueued = "queue.enqueued"

// AttachAudit subscribes the audit recorder to JobChanged. Idempotent
// per-Core (calling more than once would double-emit); boot path
// SHOULD call exactly once.
//
// Usage example:
//
//	queue.AttachAudit(c)
func AttachAudit(c *core.Core) {
	Subscribe(c, func(_ *core.Core, ev JobChanged) {
		name := mapPhaseToAuditEvent(ev.Phase)
		if name == "" {
			return
		}
		outcome := phaseToOutcome(ev.Phase)
		_ = audit.Default().Record(audit.Event{
			Event:   name,
			Scope:   ev.Job.Project,
			Outcome: outcome,
			TS:      ev.At.Unix(),
			Meta: map[string]any{
				"job_id":    ev.Job.ID,
				"kind":      ev.Job.Kind,
				"status":    ev.Job.Status,
				"__emit_ts": ev.At.UnixNano(),
			},
		})
	})
}

// mapPhaseToAuditEvent projects each queue lifecycle phase onto its
// own reserved audit event-name. Cerberus #64 F-5 (Mantis #1725) —
// STRIDE-R Repudiation gap close. Pre-cascade PhaseStarted / PhaseDone
// / PhaseCancelled all projected onto the single dequeued literal so a
// forensic walker could not distinguish them by name; the trio split
// mirrors the audit-cluster shape across runner / sandbox / process /
// marketplace / gateway / downloader.
//
// PhaseEnqueued still projects onto pkg/queue's local AuditEventEnqueued
// — Enqueued sits BEFORE worker pickup so it belongs to the queue-
// substrate surface, not the worker-side queue.job.* trio.
func mapPhaseToAuditEvent(phase string) string {
	switch phase {
	case PhaseEnqueued:
		return AuditEventEnqueued
	case PhaseStarted:
		return audit.EventQueueJobStarted
	case PhaseDone:
		return audit.EventQueueJobSucceeded
	case PhaseFailed:
		return audit.EventQueueJobFailed
	case PhaseCancelled:
		return audit.EventQueueJobCancelled
	}
	return ""
}

// phaseToOutcome maps the queue phase to the closed audit outcome
// enum. PhaseFailed → OutcomeFailed; everything else → OutcomeOK
// (PhaseStarted / Done / Cancelled all represent normal-path
// progression from the audit's perspective — Cancelled is operator-
// initiated, not an error).
func phaseToOutcome(phase string) string {
	if phase == PhaseFailed {
		return audit.OutcomeFailed
	}
	return audit.OutcomeOK
}
