// SPDX-Licence-Identifier: EUPL-1.2

package queue_test

import (
	core "dappco.re/go"
	"dappco.re/go/orm"
	"dappco.re/lthn/desktop/pkg/queue"
)

// newQueueCore is a test-local Core with orm wired and the queue
// schema registered. Mirrors the test-core helpers in pkg/tasks
// + pkg/opencode tests.
func newQueueCore(t *core.T) *core.Core {
	t.Helper()
	c := core.New()
	core.RequireTrue(t, orm.Register(c).OK)
	mem := orm.NewMemium()
	core.RequireTrue(t, orm.Mount(c, "default", mem).OK)
	for _, schema := range queue.Schemas() {
		core.RequireTrue(t, orm.RegisterSchema(c, schema).OK)
		mem.RegisterTable(schema.Name, schema)
	}
	// ServiceStartup wires c.context (cancellable) which the worker
	// loop selects on. Without this, c.Context() returns nil and
	// the worker blocks on a nil channel forever.
	core.RequireTrue(t, c.ServiceStartup(core.Background(), nil).OK)
	t.Cleanup(func() { _ = c.ServiceShutdown(core.Background()) })
	return c
}

// TestQueue_Enqueue_PersistsAsPending — Enqueue creates a Job with
// Status=pending, fires JobChanged{Phase: Enqueued}.
func TestQueue_Enqueue_PersistsAsPending(t *core.T) {
	c := newQueueCore(t)

	var got queue.JobChanged
	queue.Subscribe(c, func(_ *core.Core, ev queue.JobChanged) {
		got = ev
	})

	r := queue.Enqueue(c, "lint", core.NewOptions(
		core.Option{Key: "path", Value: "/foo"},
	))
	core.RequireTrue(t, r.OK)
	job := r.Value.(queue.Job)
	core.AssertEqual(t, queue.StatusPending, job.Status)
	core.AssertEqual(t, queue.PhaseEnqueued, got.Phase)
	core.AssertEqual(t, "lint", got.Job.Kind)
}

// TestQueue_Worker_DispatchesAndCompletes — full round-trip:
// register handler, start service, enqueue, wait, confirm Done.
func TestQueue_Worker_DispatchesAndCompletes(t *core.T) {
	c := newQueueCore(t)

	var ran core.WaitGroup
	ran.Add(1)
	var receivedPath string
	queue.RegisterKind(c, queue.HandlerOptions{
		Kind: "echo",
		Handler: func(_ core.Context, opts core.Options) core.Result {
			receivedPath = opts.String("path")
			ran.Done()
			return core.Ok("done")
		},
	})

	// Start the worker via the Service lifecycle.
	svc := queue.NewService(queue.Options{PollInterval: 50 * core.Millisecond})(c).
		Value.(*queue.Service)
	core.RequireTrue(t, svc.OnStart().OK)

	r := queue.Enqueue(c, "echo", core.NewOptions(
		core.Option{Key: "path", Value: "/echo-target"},
	))
	core.RequireTrue(t, r.OK)
	jobID := r.Value.(queue.Job).ID

	// Wait for handler to fire.
	done := make(chan struct{})
	go func() { ran.Wait(); close(done) }()
	select {
	case <-done:
	case <-core.After(2 * core.Second):
		t.Fatalf("handler never ran within 2s")
	}

	core.AssertEqual(t, "/echo-target", receivedPath)

	// Wait briefly for worker to finish marking Done.
	deadline := core.Now().Add(1 * core.Second)
	for core.Now().Before(deadline) {
		gr := queue.Get(c, jobID)
		if gr.OK {
			job, _, _ := orm.Detail[queue.Job](gr)
			if job.Status == queue.StatusDone {
				core.AssertEqual(t, 1, job.Attempts)
				return
			}
		}
		core.Sleep(20 * core.Millisecond)
	}
	t.Fatalf("job never reached StatusDone")
}

// TestQueue_Worker_FailureMarksFailedWithError — handler returning
// Fail leaves Job in StatusFailed with LastError populated.
func TestQueue_Worker_FailureMarksFailedWithError(t *core.T) {
	c := newQueueCore(t)

	queue.RegisterKind(c, queue.HandlerOptions{
		Kind: "boom",
		Handler: func(_ core.Context, _ core.Options) core.Result {
			return core.Fail(core.E("test", "intentional", nil))
		},
	})

	svc := queue.NewService(queue.Options{PollInterval: 30 * core.Millisecond})(c).
		Value.(*queue.Service)
	_ = svc.OnStart()

	r := queue.Enqueue(c, "boom", core.NewOptions())
	jobID := r.Value.(queue.Job).ID

	deadline := core.Now().Add(2 * core.Second)
	for core.Now().Before(deadline) {
		gr := queue.Get(c, jobID)
		if gr.OK {
			job, _, _ := orm.Detail[queue.Job](gr)
			if job.Status == queue.StatusFailed {
				core.AssertNotEmpty(t, job.LastError)
				return
			}
		}
		core.Sleep(20 * core.Millisecond)
	}
	t.Fatalf("job never reached StatusFailed")
}

// TestQueue_Worker_MissingHandlerFails — enqueueing for a kind
// with no registered handler marks the job Failed (rather than
// hanging forever or panicking).
func TestQueue_Worker_MissingHandlerFails(t *core.T) {
	c := newQueueCore(t)

	svc := queue.NewService(queue.Options{PollInterval: 30 * core.Millisecond})(c).
		Value.(*queue.Service)
	_ = svc.OnStart()

	r := queue.Enqueue(c, "no-handler-for-this", core.NewOptions())
	jobID := r.Value.(queue.Job).ID

	deadline := core.Now().Add(2 * core.Second)
	for core.Now().Before(deadline) {
		gr := queue.Get(c, jobID)
		if gr.OK {
			job, _, _ := orm.Detail[queue.Job](gr)
			if job.Status == queue.StatusFailed {
				return
			}
		}
		core.Sleep(20 * core.Millisecond)
	}
	t.Fatalf("missing handler should mark job failed")
}

// TestQueue_Cancel_PendingJob — cancelling a pending job moves it
// to StatusCancelled + fires JobChanged{Phase: Cancelled}.
func TestQueue_Cancel_PendingJob(t *core.T) {
	c := newQueueCore(t)

	r := queue.Enqueue(c, "lint", core.NewOptions())
	job := r.Value.(queue.Job)

	core.RequireTrue(t, queue.Cancel(c, job.ID).OK)
	gr := queue.Get(c, job.ID)
	core.RequireTrue(t, gr.OK)
	cancelled, _, _ := orm.Detail[queue.Job](gr)
	core.AssertEqual(t, queue.StatusCancelled, cancelled.Status)
}

// TestQueue_RegisterKind_AppearsInKinds — registered handlers show
// up via queue.Kinds(c).
func TestQueue_RegisterKind_AppearsInKinds(t *core.T) {
	c := newQueueCore(t)
	queue.RegisterKind(c, queue.HandlerOptions{
		Kind:    "lint",
		Handler: func(core.Context, core.Options) core.Result { return core.Ok(nil) },
	})
	queue.RegisterKind(c, queue.HandlerOptions{
		Kind:    "build",
		Handler: func(core.Context, core.Options) core.Result { return core.Ok(nil) },
	})
	kinds := queue.Kinds(c)
	core.AssertGreaterOrEqual(t, len(kinds), 2)
}
