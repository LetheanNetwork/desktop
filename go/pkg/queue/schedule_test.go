// SPDX-Licence-Identifier: EUPL-1.2

package queue_test

import (
	core "dappco.re/go"
	"dappco.re/go/orm"

	"dappco.re/lthn/desktop/pkg/auth"
	"dappco.re/lthn/desktop/pkg/queue"
)

// TestSchedule_Schedule_Good — Schedule for "now" lets the worker
// pick the job up on the next tick + the handler fires.
func TestSchedule_Schedule_Good(t *core.T) {
	c := newQueueCore(t)

	var ran core.WaitGroup
	ran.Add(1)
	queue.RegisterKind(c, queue.HandlerOptions{
		Kind:           "imm",
		PermittedTiers: []auth.CallerTier{auth.TierOperator, auth.TierCascade, auth.TierInternal},
		Handler:        func(core.Context, core.Options) core.Result { ran.Done(); return core.Ok(nil) },
	})

	svc := queue.NewService(queue.Options{PollInterval: 30 * core.Millisecond})(c).
		Value.(*queue.Service)
	_ = svc.OnStart()

	r := queue.Schedule(c, core.Now().UTC(), "imm", core.NewOptions())
	core.RequireTrue(t, r.OK)

	done := make(chan struct{})
	go func() { ran.Wait(); close(done) }()
	select {
	case <-done:
	case <-core.After(2 * core.Second):
		t.Fatalf("scheduled-now job never ran within 2s")
	}
}

// TestSchedule_Schedule_Bad — Schedule on a nil core or empty kind
// fails (delegates to EnqueueWithOptions which guards both).
func TestSchedule_Schedule_Bad(t *core.T) {
	r := queue.Schedule(nil, core.Now().UTC(), "imm", core.NewOptions())
	core.AssertFalse(t, r.OK)

	c := newQueueCore(t)
	r = queue.Schedule(c, core.Now().UTC(), "", core.NewOptions())
	core.AssertFalse(t, r.OK)
}

// TestSchedule_Schedule_Ugly — far-future schedule keeps the Job at
// StatusPending past several worker ticks; the handler must NOT fire.
func TestSchedule_Schedule_Ugly(t *core.T) {
	c := newQueueCore(t)

	var fired bool
	queue.RegisterKind(c, queue.HandlerOptions{
		Kind:           "future",
		PermittedTiers: []auth.CallerTier{auth.TierOperator, auth.TierCascade, auth.TierInternal},
		Handler:        func(core.Context, core.Options) core.Result { fired = true; return core.Ok(nil) },
	})

	svc := queue.NewService(queue.Options{PollInterval: 30 * core.Millisecond})(c).
		Value.(*queue.Service)
	_ = svc.OnStart()

	r := queue.Schedule(c, core.Now().UTC().Add(core.Hour), "future", core.NewOptions())
	core.RequireTrue(t, r.OK)
	jobID := r.Value.(queue.Job).ID

	core.Sleep(200 * core.Millisecond)
	core.AssertFalse(t, fired)
	gr := queue.Get(c, jobID)
	job, _, _ := orm.Detail[queue.Job](gr)
	core.AssertEqual(t, queue.StatusPending, job.Status)
}

// TestSchedule_ScheduleAfter_Good — ScheduleAfter(80ms) fires within
// ~6× that duration (allows for tick latency on a busy CI host).
func TestSchedule_ScheduleAfter_Good(t *core.T) {
	c := newQueueCore(t)

	var ran core.WaitGroup
	ran.Add(1)
	queue.RegisterKind(c, queue.HandlerOptions{
		Kind:           "delayed",
		PermittedTiers: []auth.CallerTier{auth.TierOperator, auth.TierCascade, auth.TierInternal},
		Handler:        func(core.Context, core.Options) core.Result { ran.Done(); return core.Ok(nil) },
	})

	svc := queue.NewService(queue.Options{PollInterval: 20 * core.Millisecond})(c).
		Value.(*queue.Service)
	_ = svc.OnStart()

	start := core.Now()
	r := queue.ScheduleAfter(c, 80*core.Millisecond, "delayed", core.NewOptions())
	core.RequireTrue(t, r.OK)

	done := make(chan struct{})
	go func() { ran.Wait(); close(done) }()
	select {
	case <-done:
		elapsed := core.Since(start)
		core.AssertGreaterOrEqual(t, elapsed, 80*core.Millisecond)
		core.AssertLessOrEqual(t, elapsed, 500*core.Millisecond)
	case <-core.After(2 * core.Second):
		t.Fatalf("delayed job never ran within 2s")
	}
}

// TestSchedule_ScheduleAfter_Bad — empty kind fails (delegates to
// Schedule → EnqueueWithOptions guards).
func TestSchedule_ScheduleAfter_Bad(t *core.T) {
	c := newQueueCore(t)
	r := queue.ScheduleAfter(c, 50*core.Millisecond, "", core.NewOptions())
	core.AssertFalse(t, r.OK)
}

// TestSchedule_ScheduleAfter_Ugly — handlers can re-enqueue themselves
// for the back-off-and-retry pattern (the canonical "in 20m re-check
// the PR" flow from design_cooperative_task_queue).
func TestSchedule_ScheduleAfter_Ugly(t *core.T) {
	c := newQueueCore(t)

	var attempts int
	var done core.WaitGroup
	done.Add(1)
	queue.RegisterKind(c, queue.HandlerOptions{
		Kind:           "retry-twice",
		PermittedTiers: []auth.CallerTier{auth.TierOperator, auth.TierCascade, auth.TierInternal},
		Handler: func(_ core.Context, _ core.Options) core.Result {
			attempts++
			if attempts < 3 {
				queue.ScheduleAfter(c, 30*core.Millisecond, "retry-twice", core.NewOptions())
				return core.Ok("rescheduled")
			}
			done.Done()
			return core.Ok("final")
		},
	})

	svc := queue.NewService(queue.Options{PollInterval: 20 * core.Millisecond})(c).
		Value.(*queue.Service)
	_ = svc.OnStart()

	core.RequireTrue(t, queue.Enqueue(c, "retry-twice", core.NewOptions()).OK)

	finished := make(chan struct{})
	go func() { done.Wait(); close(finished) }()
	select {
	case <-finished:
		core.AssertEqual(t, 3, attempts)
	case <-core.After(3 * core.Second):
		t.Fatalf("self-rescheduling chain didn't complete (attempts=%d)", attempts)
	}
}

// TestSchedule_ScheduleWithOptions_Good — full-control deferred
// enqueue persists Project + IssueID linkage on a scheduled Job.
func TestSchedule_ScheduleWithOptions_Good(t *core.T) {
	c := newQueueCore(t)
	when := core.Now().UTC().Add(core.Hour)
	r := queue.ScheduleWithOptions(c, "pr-check", queue.EnqueueOptions{
		Payload:      `{"pr_id":"1352"}`,
		ScheduledFor: when,
		Project:      "ide",
		IssueID:      "01ABC",
	})
	core.RequireTrue(t, r.OK)
	job := r.Value.(queue.Job)
	core.AssertEqual(t, "ide", job.Project)
	core.AssertEqual(t, "01ABC", job.IssueID)
	core.AssertEqual(t, when.Unix(), job.ScheduledFor.Unix())
}

// TestSchedule_ScheduleWithOptions_Bad — empty kind fails (delegates
// to EnqueueWithOptions).
func TestSchedule_ScheduleWithOptions_Bad(t *core.T) {
	c := newQueueCore(t)
	r := queue.ScheduleWithOptions(c, "", queue.EnqueueOptions{
		ScheduledFor: core.Now().UTC().Add(core.Hour),
	})
	core.AssertFalse(t, r.OK)
}

// TestSchedule_ScheduleWithOptions_Ugly — zero ScheduledFor defaults
// to "now" (same fallback EnqueueWithOptions applies); the scheduled
// job is eligible for immediate worker pickup.
func TestSchedule_ScheduleWithOptions_Ugly(t *core.T) {
	c := newQueueCore(t)
	r := queue.ScheduleWithOptions(c, "imm", queue.EnqueueOptions{})
	core.RequireTrue(t, r.OK)
	job := r.Value.(queue.Job)
	core.AssertFalse(t, job.ScheduledFor.IsZero())
}
