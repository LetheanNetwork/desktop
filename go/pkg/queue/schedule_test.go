// SPDX-Licence-Identifier: EUPL-1.2

package queue_test

import (
	"sync"
	"testing"
	"time"

	core "dappco.re/go"
	"dappco.re/go/orm"
	"dappco.re/lthn/desktop/pkg/queue"
)

// TestSchedule_FuturePastIsImmediate — Schedule with a time in the
// past (or now) lets the worker pick it up on the next tick.
func TestSchedule_FuturePastIsImmediate(t *testing.T) {
	c := newQueueCore(t)

	var ran sync.WaitGroup
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
	if !r.OK {
		t.Fatalf("schedule: %s", r.Error())
	}

	done := make(chan struct{})
	go func() { ran.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * core.Second):
		t.Fatalf("scheduled-now job never ran within 2s")
	}
}

// TestSchedule_FutureNotPickedUpYet — Schedule for the far future
// keeps the job in StatusPending past several worker ticks.
func TestSchedule_FutureNotPickedUpYet(t *testing.T) {
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
	if !r.OK {
		t.Fatalf("schedule: %s", r.Error())
	}
	jobID := r.Value.(queue.Job).ID

	// Wait through several ticks — worker MUST NOT pick it up.
	core.Sleep(200 * core.Millisecond)
	if fired {
		t.Fatalf("future job fired prematurely")
	}
	gr := queue.Get(c, jobID)
	job, _, _ := orm.Detail[queue.Job](gr)
	if job.Status != queue.StatusPending {
		t.Errorf("status: got %q want pending", job.Status)
	}
}

// TestScheduleAfter_FiresAfterDuration — ScheduleAfter(50ms) gets
// processed within ~3x that duration (allows for tick latency).
func TestScheduleAfter_FiresAfterDuration(t *testing.T) {
	c := newQueueCore(t)

	var ran sync.WaitGroup
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
	if !r.OK {
		t.Fatalf("schedule-after: %s", r.Error())
	}

	done := make(chan struct{})
	go func() { ran.Wait(); close(done) }()
	select {
	case <-done:
		elapsed := core.Since(start)
		if elapsed < 80*core.Millisecond {
			t.Errorf("fired too early: %v < 80ms", elapsed)
		}
		if elapsed > 500*core.Millisecond {
			t.Errorf("fired too late: %v > 500ms", elapsed)
		}
	case <-time.After(2 * core.Second):
		t.Fatalf("delayed job never ran within 2s")
	}
}

// TestSchedule_HandlerSelfReschedule — handlers can re-enqueue
// themselves via ScheduleAfter for the back-off-and-retry pattern
// (canonical "in 20m re-check the PR" flow from the design memory).
func TestSchedule_HandlerSelfReschedule(t *testing.T) {
	c := newQueueCore(t)

	var attempts int
	var done sync.WaitGroup
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

	if r := queue.Enqueue(c, "retry-twice", core.NewOptions()); !r.OK {
		t.Fatalf("initial enqueue: %s", r.Error())
	}

	finished := make(chan struct{})
	go func() { done.Wait(); close(finished) }()
	select {
	case <-finished:
		if attempts != 3 {
			t.Errorf("attempts: got %d want 3", attempts)
		}
	case <-time.After(3 * core.Second):
		t.Fatalf("self-rescheduling chain didn't complete (attempts=%d)", attempts)
	}
}
