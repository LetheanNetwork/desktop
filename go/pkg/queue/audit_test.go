// SPDX-Licence-Identifier: EUPL-1.2

// Tests for the Cerberus #64 F-5 (Mantis #1725) STRIDE-R Repudiation
// gap close: pkg/queue's AttachAudit subscriber now projects each
// Job lifecycle phase onto its own reserved audit literal so a
// forensic walker can distinguish "job started" / "job succeeded" /
// "job failed" / "job cancelled" by event-name alone instead of
// introspecting Meta. Mirror of pkg/process/audit_test.go +
// pkg/sandbox/audit_test.go fixture shape — the recordingRecorder
// shape is duplicated rather than shared because audit.SetDefault is
// process-global and the fixture must not leak across packages
// running in parallel.

package queue_test

import (
	"sync"

	core "dappco.re/go"

	"dappco.re/lthn/desktop/pkg/audit"
	"dappco.re/lthn/desktop/pkg/auth"
	"dappco.re/lthn/desktop/pkg/queue"
)

// recordingRecorder captures every audit.Event the queue subscriber
// emits during a test. Mirror of the same-named fixture in
// pkg/sandbox/audit_test.go and pkg/process/audit_test.go.
type recordingRecorder struct {
	mu     sync.Mutex
	events []audit.Event
}

func (r *recordingRecorder) Record(ev audit.Event) core.Result {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, ev)
	return core.Ok(nil)
}

func (r *recordingRecorder) snapshot() []audit.Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]audit.Event, len(r.events))
	copy(out, r.events)
	return out
}

func (r *recordingRecorder) filterByName(name string) []audit.Event {
	all := r.snapshot()
	out := make([]audit.Event, 0, len(all))
	for _, ev := range all {
		if ev.Event == name {
			out = append(out, ev)
		}
	}
	return out
}

// installAuditRecorder wires a recordingRecorder as audit.Default and
// restores the prior default at test end. Mirrors the helper in
// pkg/sandbox + pkg/process audit_test.go.
func installAuditRecorder(t *core.T) *recordingRecorder {
	t.Helper()
	rec := &recordingRecorder{}
	audit.SetDefault(rec)
	t.Cleanup(func() { audit.SetDefault(nil) })
	return rec
}

// waitForEvent polls the recorder up to 2 seconds for at least one
// event with the named literal. The queue worker dispatch is async
// (goroutine + ticker + IPC bus); a Sleep is unreliable on busy CI.
func waitForEvent(rec *recordingRecorder, name string) bool {
	deadline := core.Now().Add(2 * core.Second)
	for core.Now().Before(deadline) {
		if len(rec.filterByName(name)) > 0 {
			return true
		}
		core.Sleep(10 * core.Millisecond)
	}
	return false
}

// TestQueueAudit_EmitsTrio_Good drives the worker through the
// happy path (Enqueue → claim → handler returns OK → markDone) and
// asserts the typed Started + Succeeded literals land on the audit
// substrate with the expected Meta shape. Cerberus #64 F-5: pre-cascade
// PhaseStarted and PhaseDone both projected onto "queue.dequeued"; the
// split MUST surface each phase under its own canonical literal.
func TestQueueAudit_EmitsTrio_Good(t *core.T) {
	rec := installAuditRecorder(t)
	c := newQueueCore(t)
	queue.AttachAudit(c)

	queue.RegisterKind(c, queue.HandlerOptions{
		Kind:           "echo",
		PermittedTiers: []auth.CallerTier{auth.TierOperator, auth.TierCascade, auth.TierInternal},
		Handler: func(_ core.Context, _ core.Options) core.Result {
			return core.Ok("done")
		},
	})

	svc := queue.NewService(queue.Options{PollInterval: 30 * core.Millisecond})(c).
		Value.(*queue.Service)
	core.RequireTrue(t, svc.OnStart().OK)

	r := queue.Enqueue(c, "echo", core.NewOptions())
	core.RequireTrue(t, r.OK)
	jobID := r.Value.(queue.Job).ID

	core.AssertTrue(t, waitForEvent(rec, audit.EventQueueJobSucceeded),
		"Succeeded must land on audit substrate within 2s")

	// Enqueue MUST emit the queue.enqueued literal (Enqueued sits before
	// the worker pickup; it remains in the pkg/queue namespace).
	enqueued := rec.filterByName(queue.AuditEventEnqueued)
	core.AssertEqual(t, 1, len(enqueued),
		"Enqueue must emit exactly one queue.enqueued row")
	core.AssertEqual(t, audit.OutcomeOK, enqueued[0].Outcome)
	core.AssertEqual(t, jobID, enqueued[0].Meta["job_id"])
	core.AssertEqual(t, "echo", enqueued[0].Meta["kind"])

	// Worker pickup MUST emit Started, NOT queue.dequeued. Pre-cascade
	// this row carried the collapsed "queue.dequeued" literal.
	started := rec.filterByName(audit.EventQueueJobStarted)
	core.AssertEqual(t, 1, len(started),
		"worker claim must emit exactly one queue.job.started row")
	core.AssertEqual(t, audit.OutcomeOK, started[0].Outcome)
	core.AssertEqual(t, jobID, started[0].Meta["job_id"])
	core.AssertEqual(t, "echo", started[0].Meta["kind"])

	// Handler-OK MUST emit Succeeded, NOT queue.dequeued.
	succeeded := rec.filterByName(audit.EventQueueJobSucceeded)
	core.AssertEqual(t, 1, len(succeeded),
		"handler-OK must emit exactly one queue.job.succeeded row")
	core.AssertEqual(t, audit.OutcomeOK, succeeded[0].Outcome)
	core.AssertEqual(t, jobID, succeeded[0].Meta["job_id"])
	core.AssertEqual(t, queue.StatusDone, succeeded[0].Meta["status"])

	// Defensive: zero "queue.dequeued" rows on any path. The collapsed
	// literal is gone from the emit surface.
	dequeued := rec.filterByName("queue.dequeued")
	core.AssertEqual(t, 0, len(dequeued),
		"queue.dequeued literal MUST NOT appear post-cascade")

	// Defensive: zero Failed / Cancelled on the happy path.
	failed := rec.filterByName(audit.EventQueueJobFailed)
	core.AssertEqual(t, 0, len(failed))
	cancelled := rec.filterByName(audit.EventQueueJobCancelled)
	core.AssertEqual(t, 0, len(cancelled))
}

// TestQueueAudit_EmitsFailed_Bad drives the worker through a handler
// that returns core.Fail and asserts the typed Failed literal lands —
// pre-cascade this projected onto "queue.failed"; the rename moves the
// terminal-error row onto the canonical queue.job.* namespace per the
// audit-cluster trio shape.
func TestQueueAudit_EmitsFailed_Bad(t *core.T) {
	rec := installAuditRecorder(t)
	c := newQueueCore(t)
	queue.AttachAudit(c)

	queue.RegisterKind(c, queue.HandlerOptions{
		Kind:           "boom",
		PermittedTiers: []auth.CallerTier{auth.TierOperator, auth.TierCascade, auth.TierInternal},
		Handler: func(_ core.Context, _ core.Options) core.Result {
			return core.Fail(core.E("test", "intentional", nil))
		},
	})

	svc := queue.NewService(queue.Options{PollInterval: 30 * core.Millisecond})(c).
		Value.(*queue.Service)
	core.RequireTrue(t, svc.OnStart().OK)

	r := queue.Enqueue(c, "boom", core.NewOptions())
	core.RequireTrue(t, r.OK)
	jobID := r.Value.(queue.Job).ID

	core.AssertTrue(t, waitForEvent(rec, audit.EventQueueJobFailed),
		"Failed must land on audit substrate within 2s")

	failed := rec.filterByName(audit.EventQueueJobFailed)
	core.AssertEqual(t, 1, len(failed),
		"handler-Fail must emit exactly one queue.job.failed row")
	core.AssertEqual(t, audit.OutcomeFailed, failed[0].Outcome)
	core.AssertEqual(t, jobID, failed[0].Meta["job_id"])
	core.AssertEqual(t, queue.StatusFailed, failed[0].Meta["status"])

	// Defensive: pre-cascade "queue.failed" literal MUST be absent.
	legacy := rec.filterByName("queue.failed")
	core.AssertEqual(t, 0, len(legacy),
		"queue.failed literal MUST NOT appear post-cascade")

	// Worker still emitted Started before Fail (the claim succeeded).
	started := rec.filterByName(audit.EventQueueJobStarted)
	core.AssertEqual(t, 1, len(started))

	// Defensive: zero Succeeded / Cancelled on the failure path.
	succeeded := rec.filterByName(audit.EventQueueJobSucceeded)
	core.AssertEqual(t, 0, len(succeeded))
	cancelled := rec.filterByName(audit.EventQueueJobCancelled)
	core.AssertEqual(t, 0, len(cancelled))
}

// TestQueueAudit_EmitsCancelled_Ugly drives queue.Cancel on a pending
// job and asserts the typed Cancelled literal lands — pre-cascade this
// projected onto "queue.dequeued"; the split lets the forensic walker
// distinguish operator-initiated cancellation from worker-completed
// and handler-error terminal states.
func TestQueueAudit_EmitsCancelled_Ugly(t *core.T) {
	rec := installAuditRecorder(t)
	c := newQueueCore(t)
	queue.AttachAudit(c)

	// Do NOT start a worker — Cancel transitions a pending job to
	// StatusCancelled before the worker can claim it.
	r := queue.Enqueue(c, "lint", core.NewOptions())
	core.RequireTrue(t, r.OK)
	job := r.Value.(queue.Job)

	core.RequireTrue(t, queue.Cancel(c, job.ID).OK)

	core.AssertTrue(t, waitForEvent(rec, audit.EventQueueJobCancelled),
		"Cancelled must land on audit substrate within 2s")

	cancelled := rec.filterByName(audit.EventQueueJobCancelled)
	core.AssertEqual(t, 1, len(cancelled),
		"Cancel must emit exactly one queue.job.cancelled row")
	core.AssertEqual(t, audit.OutcomeOK, cancelled[0].Outcome,
		"cancellation is operator-initiated; outcome=ok per RFC §2.1")
	core.AssertEqual(t, job.ID, cancelled[0].Meta["job_id"])
	core.AssertEqual(t, queue.StatusCancelled, cancelled[0].Meta["status"])

	// Defensive: pre-cascade "queue.dequeued" literal MUST be absent.
	legacy := rec.filterByName("queue.dequeued")
	core.AssertEqual(t, 0, len(legacy),
		"queue.dequeued literal MUST NOT appear post-cascade")

	// Worker never started, so no Started / Succeeded / Failed rows.
	started := rec.filterByName(audit.EventQueueJobStarted)
	core.AssertEqual(t, 0, len(started))
	succeeded := rec.filterByName(audit.EventQueueJobSucceeded)
	core.AssertEqual(t, 0, len(succeeded))
	failed := rec.filterByName(audit.EventQueueJobFailed)
	core.AssertEqual(t, 0, len(failed))
}
