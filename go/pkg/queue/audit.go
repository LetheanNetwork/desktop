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

// Reserved audit event-names. Parity-grep contract (RFC §3.3.1) sees
// these constants when walking the package AST; emit-side projects
// the same constants so a typo fails at compile.
const (
	AuditEventEnqueued = "queue.enqueued"
	AuditEventDequeued = "queue.dequeued"
	AuditEventFailed   = "queue.failed"
)

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

// mapPhaseToAuditEvent collapses the queue phase constants onto the
// three reserved audit event names. Started + Done both project
// onto queue.dequeued (the audit-tail's concern is "the job left
// the queue," not the post-pickup lifecycle); Cancelled also folds
// into queue.dequeued because the canonical contract is "this job
// is no longer pending."
func mapPhaseToAuditEvent(phase string) string {
	switch phase {
	case PhaseEnqueued:
		return AuditEventEnqueued
	case PhaseStarted, PhaseDone, PhaseCancelled:
		return AuditEventDequeued
	case PhaseFailed:
		return AuditEventFailed
	}
	return ""
}

// phaseToOutcome maps the queue phase to the closed audit outcome
// enum. PhaseFailed → OutcomeFailed; everything else → OutcomeOK
// (PhaseStarted / Done / Cancelled all represent normal-path
// progression from the audit's perspective).
func phaseToOutcome(phase string) string {
	if phase == PhaseFailed {
		return audit.OutcomeFailed
	}
	return audit.OutcomeOK
}
