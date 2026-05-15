// SPDX-Licence-Identifier: EUPL-1.2

package queue_test

import (
	core "dappco.re/go"
	"dappco.re/go/orm"
	"dappco.re/lthn/desktop/pkg/queue"
)

// TestSchedule_FuturePastIsImmediate — Schedule with a time in the
// past (or now) lets the worker pick it up on the next tick.
func TestSchedule_FuturePastIsImmediate(t *core.T) {
	c := newQueueCore(t)

	var ran core.WaitGroup
	ran.Add(1)
	queue.RegisterKind(c, queue.HandlerOptions{
		Kind:    "imm",
		Handler: func(core.Context, core.Options) core.Result { ran.Done(); return core.Ok(nil) },
	})

	svc := queue.NewService(queue.Options{PollInterval: 30 * core.Millisecond})(c).
		Value.(*queue.Service)
	_ = svc.OnStart()

	// Schedule for "now" — worker should pick it up next tick.
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

// TestSchedule_FutureNotPickedUpYet — Schedule for the far future
// keeps the job in StatusPending past several worker ticks.
func TestSchedule_FutureNotPickedUpYet(t *core.T) {
	c := newQueueCore(t)

	var fired bool
	queue.RegisterKind(c, queue.HandlerOptions{
		Kind:    "future",
		Handler: func(core.Context, core.Options) core.Result { fired = true; return core.Ok(nil) },
	})

	svc := queue.NewService(queue.Options{PollInterval: 30 * core.Millisecond})(c).
		Value.(*queue.Service)
	_ = svc.OnStart()

	// Schedule one hour from now.
	r := queue.Schedule(c, core.Now().UTC().Add(core.Hour), "future", core.NewOptions())
	core.RequireTrue(t, r.OK)
	jobID := r.Value.(queue.Job).ID

	// Wait through several ticks — worker MUST NOT pick it up.
	core.Sleep(200 * core.Millisecond)
	core.AssertFalse(t, fired)
	gr := queue.Get(c, jobID)
	job, _, _ := orm.Detail[queue.Job](gr)
	core.AssertEqual(t, queue.StatusPending, job.Status)
}

// TestScheduleAfter_FiresAfterDuration — ScheduleAfter(50ms) gets
// processed within ~3x that duration (allows for tick latency).
func TestScheduleAfter_FiresAfterDuration(t *core.T) {
	c := newQueueCore(t)

	var ran core.WaitGroup
	ran.Add(1)
	queue.RegisterKind(c, queue.HandlerOptions{
		Kind:    "delayed",
		Handler: func(core.Context, core.Options) core.Result { ran.Done(); return core.Ok(nil) },
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

// TestSchedule_HandlerSelfReschedule — handlers can re-enqueue
// themselves via ScheduleAfter for the back-off-and-retry pattern
// (canonical "in 20m re-check the PR" flow from the design memory).
func TestSchedule_HandlerSelfReschedule(t *core.T) {
	c := newQueueCore(t)

	var attempts int
	var done core.WaitGroup
	done.Add(1)
	queue.RegisterKind(c, queue.HandlerOptions{
		Kind: "retry-twice",
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
