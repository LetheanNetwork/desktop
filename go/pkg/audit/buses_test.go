// SPDX-Licence-Identifier: EUPL-1.2

// buses_test.go — Stage F.B Phase 2.4 typed-bus cross-cut smoke
// tests. Confirms the reserved audit event-name constants in
// tasks / queue / pipeline / incidents match the literals the
// pkg/audit consumer expects. This is the run-time complement
// to parity_test.go's source-grep — catches a constant rename
// in either side that broke the contract.

package audit_test

import (
	"testing"

	"dappco.re/lthn/desktop/pkg/audit"
	"dappco.re/lthn/desktop/pkg/incidents"
	"dappco.re/lthn/desktop/pkg/queue"
	"dappco.re/lthn/desktop/pkg/sales/pipeline"
	"dappco.re/lthn/desktop/pkg/tasks"
)

func TestBuses_ReservedAuditEventNames_Good(t *testing.T) {
	// Reserved event-names per RFC §3.3. Pinning the literals here
	// makes a rename in any consumer package surface as a build-time
	// test failure.
	//
	// Cerberus #64 F-5 (Mantis #1725) cascade: the pre-cascade trio
	// (Enqueued / Dequeued / Failed) collapsed Started + Done + Cancelled
	// onto a single "queue.dequeued" literal. The audit-cluster-canon
	// split puts each lifecycle phase on its own typed literal; only
	// AuditEventEnqueued remains in the pkg/queue namespace because
	// Enqueued sits before worker pickup. Started / Succeeded / Failed
	// / Cancelled live on pkg/audit's EventQueueJob* surface per the
	// reserved-schema discipline.
	cases := []struct {
		got, want string
	}{
		{tasks.AuditEventCreated, "tasks.created"},
		{tasks.AuditEventTransitioned, "tasks.transitioned"},
		{tasks.AuditEventDeleted, "tasks.deleted"},
		{queue.AuditEventEnqueued, "queue.enqueued"},
		{audit.EventQueueJobStarted, "queue.job.started"},
		{audit.EventQueueJobSucceeded, "queue.job.succeeded"},
		{audit.EventQueueJobFailed, "queue.job.failed"},
		{audit.EventQueueJobCancelled, "queue.job.cancelled"},
		{pipeline.AuditEventStageChanged, "pipeline.stage_changed"},
		{incidents.AuditEventCreated, "incidents.created"},
		{incidents.AuditEventTransitioned, "incidents.transitioned"},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("reserved audit event-name drift: got %q want %q",
				tc.got, tc.want)
		}
	}
}
