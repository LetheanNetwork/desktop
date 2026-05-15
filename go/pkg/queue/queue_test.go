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

// TestQueue_Schemas_Good — Schemas() returns the queue_jobs schema
// shape required by orm.RegisterSchema; smoke check on PK + indexes.
func TestQueue_Schemas_Good(t *core.T) {
	schemas := queue.Schemas()
	core.AssertLen(t, schemas, 1)
	core.AssertEqual(t, "queue_jobs", schemas[0].Name)
	core.AssertEqual(t, "id", schemas[0].PK[0])
}

// TestQueue_Schemas_Bad — Schemas() never returns nil or zero-length;
// callers can iterate without a guard.
func TestQueue_Schemas_Bad(t *core.T) {
	schemas := queue.Schemas()
	core.AssertNotEmpty(t, schemas)
	core.AssertNotEqual(t, "tasks_issues", schemas[0].Name)
}

// TestQueue_Schemas_Ugly — every field name in the returned Schema
// is unique; defensive against a future refactor adding a duplicate
// b.String / b.Int / b.Time call.
func TestQueue_Schemas_Ugly(t *core.T) {
	schema := queue.Schemas()[0]
	seen := map[string]bool{}
	for _, f := range schema.Fields {
		core.AssertFalse(t, seen[f.Name])
		seen[f.Name] = true
	}
}

// TestQueue_Enqueue_Good — Enqueue persists a Job at StatusPending,
// fires a JobChanged{Phase: PhaseEnqueued} broadcast carrying the
// Kind through to the listener.
func TestQueue_Enqueue_Good(t *core.T) {
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

// TestQueue_Enqueue_Bad — nil core or empty kind both Fail with a
// shaped core.E error so callers can branch on r.OK without panicking.
func TestQueue_Enqueue_Bad(t *core.T) {
	r := queue.Enqueue(nil, "lint", core.NewOptions())
	core.AssertFalse(t, r.OK)

	c := newQueueCore(t)
	r = queue.Enqueue(c, "", core.NewOptions())
	core.AssertFalse(t, r.OK)
}

// TestQueue_Enqueue_Ugly — payloads with empty options round-trip
// through orm as "{}" + the persisted Job carries a non-empty ID
// with the queue's "j-" prefix convention.
func TestQueue_Enqueue_Ugly(t *core.T) {
	c := newQueueCore(t)
	r := queue.Enqueue(c, "lint", core.NewOptions())
	core.RequireTrue(t, r.OK)
	job := r.Value.(queue.Job)
	core.AssertNotEmpty(t, job.ID)
	core.AssertEqual(t, "{}", job.Payload)
}

// TestQueue_EnqueueWithOptions_Good — full-control enqueue persists
// Project + IssueID linkage AND a future ScheduledFor as supplied.
func TestQueue_EnqueueWithOptions_Good(t *core.T) {
	c := newQueueCore(t)
	when := core.Now().UTC().Add(core.Hour)
	r := queue.EnqueueWithOptions(c, "lint", queue.EnqueueOptions{
		Payload:      `{"path":"/x"}`,
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

// TestQueue_EnqueueWithOptions_Bad — empty kind fails; nil core fails.
func TestQueue_EnqueueWithOptions_Bad(t *core.T) {
	r := queue.EnqueueWithOptions(nil, "lint", queue.EnqueueOptions{})
	core.AssertFalse(t, r.OK)
	c := newQueueCore(t)
	r = queue.EnqueueWithOptions(c, "", queue.EnqueueOptions{})
	core.AssertFalse(t, r.OK)
}

// TestQueue_EnqueueWithOptions_Ugly — zero ScheduledFor defaults to
// "now" rather than persisting a zero-time row that the worker would
// skip forever.
func TestQueue_EnqueueWithOptions_Ugly(t *core.T) {
	c := newQueueCore(t)
	r := queue.EnqueueWithOptions(c, "lint", queue.EnqueueOptions{})
	core.RequireTrue(t, r.OK)
	job := r.Value.(queue.Job)
	core.AssertFalse(t, job.ScheduledFor.IsZero())
}

// TestQueue_Cancel_Good — cancelling a pending job moves it to
// StatusCancelled + fires JobChanged{Phase: PhaseCancelled}.
func TestQueue_Cancel_Good(t *core.T) {
	c := newQueueCore(t)
	r := queue.Enqueue(c, "lint", core.NewOptions())
	job := r.Value.(queue.Job)

	core.RequireTrue(t, queue.Cancel(c, job.ID).OK)
	gr := queue.Get(c, job.ID)
	core.RequireTrue(t, gr.OK)
	cancelled, _, _ := orm.Detail[queue.Job](gr)
	core.AssertEqual(t, queue.StatusCancelled, cancelled.Status)
	core.AssertFalse(t, cancelled.CompletedAt.IsZero())
}

// TestQueue_Cancel_Bad — cancelling an unknown id fails with a
// shaped error; cancelling an already-terminal job fails per the
// "job is not pending" guard in queue.Cancel.
func TestQueue_Cancel_Bad(t *core.T) {
	c := newQueueCore(t)
	r := queue.Cancel(c, "no-such-job")
	core.AssertFalse(t, r.OK)

	enq := queue.Enqueue(c, "lint", core.NewOptions())
	job := enq.Value.(queue.Job)
	core.RequireTrue(t, queue.Cancel(c, job.ID).OK)
	r = queue.Cancel(c, job.ID)
	core.AssertFalse(t, r.OK)
}

// TestQueue_Cancel_Ugly — cancellation broadcasts the BEFORE snapshot
// (Status=pending) alongside the AFTER snapshot (Status=cancelled);
// listeners can compute their own diffs without re-querying orm.
func TestQueue_Cancel_Ugly(t *core.T) {
	c := newQueueCore(t)

	var got queue.JobChanged
	queue.Subscribe(c, func(_ *core.Core, ev queue.JobChanged) {
		if ev.Phase == queue.PhaseCancelled {
			got = ev
		}
	})

	r := queue.Enqueue(c, "lint", core.NewOptions())
	job := r.Value.(queue.Job)
	core.RequireTrue(t, queue.Cancel(c, job.ID).OK)

	core.AssertEqual(t, queue.StatusPending, got.Before.Status)
	core.AssertEqual(t, queue.StatusCancelled, got.Job.Status)
}

// TestQueue_List_Good — List returns every enqueued Job, newest
// EnqueuedAt first.
func TestQueue_List_Good(t *core.T) {
	c := newQueueCore(t)
	core.RequireTrue(t, queue.Enqueue(c, "lint", core.NewOptions()).OK)
	core.RequireTrue(t, queue.Enqueue(c, "build", core.NewOptions()).OK)

	r := queue.List(c, queue.ListFilter{})
	core.RequireTrue(t, r.OK)
	jobs, ok := orm.Cast[[]queue.Job](r)
	core.RequireTrue(t, ok)
	core.AssertLen(t, jobs, 2)
}

// TestQueue_List_Bad — filtering by an unknown kind returns an empty
// slice, not a Fail; callers iterate without a special-case branch.
func TestQueue_List_Bad(t *core.T) {
	c := newQueueCore(t)
	core.RequireTrue(t, queue.Enqueue(c, "lint", core.NewOptions()).OK)

	r := queue.List(c, queue.ListFilter{Kind: "no-such-kind"})
	core.RequireTrue(t, r.OK)
	jobs, ok := orm.Cast[[]queue.Job](r)
	core.RequireTrue(t, ok)
	core.AssertLen(t, jobs, 0)
}

// TestQueue_List_Ugly — IssueID + Project filters narrow correctly
// across a multi-row queue; the Order("enqueued_at","desc") clause
// puts the most-recent insert first when no narrower filter applies.
func TestQueue_List_Ugly(t *core.T) {
	c := newQueueCore(t)
	core.RequireTrue(t, queue.EnqueueWithOptions(c, "lint", queue.EnqueueOptions{
		Project: "ide", IssueID: "I1",
	}).OK)
	core.RequireTrue(t, queue.EnqueueWithOptions(c, "lint", queue.EnqueueOptions{
		Project: "ide", IssueID: "I2",
	}).OK)
	core.RequireTrue(t, queue.EnqueueWithOptions(c, "build", queue.EnqueueOptions{
		Project: "store",
	}).OK)

	// IssueID filter narrows to one specific row.
	r := queue.List(c, queue.ListFilter{IssueID: "I2"})
	core.RequireTrue(t, r.OK)
	jobs, _ := orm.Cast[[]queue.Job](r)
	core.AssertLen(t, jobs, 1)
	core.AssertEqual(t, "I2", jobs[0].IssueID)

	// Project filter narrows to the two rows under the same Project.
	r = queue.List(c, queue.ListFilter{Project: "ide"})
	core.RequireTrue(t, r.OK)
	jobs, _ = orm.Cast[[]queue.Job](r)
	core.AssertLen(t, jobs, 2)
}

// TestQueue_Get_Good — Get returns the persisted Job by id.
func TestQueue_Get_Good(t *core.T) {
	c := newQueueCore(t)
	r := queue.Enqueue(c, "lint", core.NewOptions())
	core.RequireTrue(t, r.OK)
	id := r.Value.(queue.Job).ID

	gr := queue.Get(c, id)
	core.RequireTrue(t, gr.OK)
	job, _, ok := orm.Detail[queue.Job](gr)
	core.RequireTrue(t, ok)
	core.AssertEqual(t, id, job.ID)
}

// TestQueue_Get_Bad — Get on an unknown id fails (orm.Find passthrough).
func TestQueue_Get_Bad(t *core.T) {
	c := newQueueCore(t)
	r := queue.Get(c, "no-such-job")
	core.AssertFalse(t, r.OK)
}

// TestQueue_Get_Ugly — Get on a freshly-enqueued Job returns the
// orm snapshot with Kind + Status intact.
func TestQueue_Get_Ugly(t *core.T) {
	c := newQueueCore(t)
	r := queue.Enqueue(c, "lint", core.NewOptions())
	id := r.Value.(queue.Job).ID
	gr := queue.Get(c, id)
	core.RequireTrue(t, gr.OK)
	job, _, _ := orm.Detail[queue.Job](gr)
	core.AssertEqual(t, "lint", job.Kind)
	core.AssertEqual(t, queue.StatusPending, job.Status)
}

// TestQueue_RegisterKind_Good — RegisterKind installs a handler under
// the queue.kind.<name> action namespace; subsequent Kinds(c) lists
// the registered kind.
func TestQueue_RegisterKind_Good(t *core.T) {
	c := newQueueCore(t)
	r := queue.RegisterKind(c, queue.HandlerOptions{
		Kind:    "lint",
		Handler: func(core.Context, core.Options) core.Result { return core.Ok(nil) },
	})
	core.RequireTrue(t, r.OK)
	kinds := queue.Kinds(c)
	core.AssertContains(t, kinds, "lint")
}

// TestQueue_RegisterKind_Bad — nil Core, empty kind, and nil handler
// each Fail with a shaped error so the caller branches on r.OK.
func TestQueue_RegisterKind_Bad(t *core.T) {
	r := queue.RegisterKind(nil, queue.HandlerOptions{Kind: "x"})
	core.AssertFalse(t, r.OK)

	c := newQueueCore(t)
	r = queue.RegisterKind(c, queue.HandlerOptions{
		Handler: func(core.Context, core.Options) core.Result { return core.Ok(nil) },
	})
	core.AssertFalse(t, r.OK)
	r = queue.RegisterKind(c, queue.HandlerOptions{Kind: "x"})
	core.AssertFalse(t, r.OK)
}

// TestQueue_RegisterKind_Ugly — re-registering the same kind is
// last-write-wins (matches Core's c.Action semantics); Kinds(c)
// still lists the kind exactly once.
func TestQueue_RegisterKind_Ugly(t *core.T) {
	c := newQueueCore(t)
	core.RequireTrue(t, queue.RegisterKind(c, queue.HandlerOptions{
		Kind:    "lint",
		Handler: func(core.Context, core.Options) core.Result { return core.Ok("first") },
	}).OK)
	core.RequireTrue(t, queue.RegisterKind(c, queue.HandlerOptions{
		Kind:    "lint",
		Handler: func(core.Context, core.Options) core.Result { return core.Ok("second") },
	}).OK)
	count := 0
	for _, k := range queue.Kinds(c) {
		if k == "lint" {
			count++
		}
	}
	core.AssertEqual(t, 1, count)
}

// TestQueue_ActionName_Good — ActionName composes the canonical
// queue.kind.<name> handler-dispatch identifier.
func TestQueue_ActionName_Good(t *core.T) {
	core.AssertEqual(t, "queue.kind.lint", queue.ActionName("lint"))
}

// TestQueue_ActionName_Bad — empty kind still composes (the prefix
// alone), so callers don't get a panic; downstream RegisterKind /
// Enqueue catch the empty-kind misuse.
func TestQueue_ActionName_Bad(t *core.T) {
	core.AssertEqual(t, "queue.kind.", queue.ActionName(""))
}

// TestQueue_ActionName_Ugly — a kind containing dots round-trips
// without any escaping; the full namespace is part of the action
// name as supplied.
func TestQueue_ActionName_Ugly(t *core.T) {
	core.AssertEqual(t, "queue.kind.lint.go.vet", queue.ActionName("lint.go.vet"))
}

// TestQueue_Kinds_Good — Kinds(c) returns every registered kind
// (the queue.kind.* action subset of c.Actions()).
func TestQueue_Kinds_Good(t *core.T) {
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

// TestQueue_Kinds_Bad — nil Core returns nil, never panics; safe to
// range over the zero result.
func TestQueue_Kinds_Bad(t *core.T) {
	core.AssertNotPanics(t, func() {
		_ = queue.Kinds(nil)
	})
	core.AssertNil(t, queue.Kinds(nil))
}

// TestQueue_Kinds_Ugly — Kinds(c) on a fresh Core (nothing registered)
// never returns the queue.kind.* prefix as a literal entry.
func TestQueue_Kinds_Ugly(t *core.T) {
	c := newQueueCore(t)
	for _, k := range queue.Kinds(c) {
		core.AssertNotEqual(t, "", k)
	}
}

// TestQueue_Subscribe_Good — Subscribe receives a JobChanged for
// every lifecycle transition (enqueued path here).
func TestQueue_Subscribe_Good(t *core.T) {
	c := newQueueCore(t)
	var received bool
	queue.Subscribe(c, func(_ *core.Core, ev queue.JobChanged) {
		if ev.Phase == queue.PhaseEnqueued {
			received = true
		}
	})
	core.RequireTrue(t, queue.Enqueue(c, "lint", core.NewOptions()).OK)
	core.AssertTrue(t, received)
}

// TestQueue_Subscribe_Bad — Subscribe with nil Core or nil callback
// is a defensive no-op; AssertNotPanics across both nil paths.
func TestQueue_Subscribe_Bad(t *core.T) {
	core.AssertNotPanics(t, func() {
		queue.Subscribe(nil, func(*core.Core, queue.JobChanged) {})
	})
	c := newQueueCore(t)
	core.AssertNotPanics(t, func() {
		queue.Subscribe(c, nil)
	})
}

// TestQueue_Subscribe_Ugly — multiple subscribers all receive the
// same event; one panicking subscriber doesn't take the cascade down.
func TestQueue_Subscribe_Ugly(t *core.T) {
	c := newQueueCore(t)
	var aCount, bCount int
	var mu core.Mutex
	queue.Subscribe(c, func(*core.Core, queue.JobChanged) {
		mu.Lock()
		aCount++
		mu.Unlock()
	})
	queue.Subscribe(c, func(*core.Core, queue.JobChanged) {
		panic("simulated bad subscriber — Core's recover keeps cascade alive")
	})
	queue.Subscribe(c, func(*core.Core, queue.JobChanged) {
		mu.Lock()
		bCount++
		mu.Unlock()
	})
	core.RequireTrue(t, queue.Enqueue(c, "lint", core.NewOptions()).OK)
	core.Sleep(5 * core.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	core.AssertEqual(t, 1, aCount)
	core.AssertEqual(t, 1, bCount)
}

// TestQueue_NewService_Good — NewService returns a constructor
// usable as the second arg to core.WithName; OnStart spawns a worker
// + OnStop is the documented no-op (Core's ServiceShutdown drains).
func TestQueue_NewService_Good(t *core.T) {
	c := newQueueCore(t)
	r := queue.NewService(queue.Options{PollInterval: 50 * core.Millisecond})(c)
	core.RequireTrue(t, r.OK)
	svc, ok := r.Value.(*queue.Service)
	core.RequireTrue(t, ok)
	core.AssertEqual(t, "Queue", svc.ServiceName())
	core.RequireTrue(t, svc.OnStart().OK)
	core.AssertTrue(t, svc.OnStop().OK)
}

// TestQueue_NewService_Bad — calling OnStart on a nil service Fails
// rather than panicking; the constructor itself returns Ok with a
// populated Service even when Options is the zero value.
func TestQueue_NewService_Bad(t *core.T) {
	var nilSvc *queue.Service
	core.AssertFalse(t, nilSvc.OnStart().OK)

	c := newQueueCore(t)
	r := queue.NewService(queue.Options{})(c)
	core.RequireTrue(t, r.OK)
}

// TestQueue_NewService_Ugly — a non-positive PollInterval falls back
// to the 1-second default; the populated Service still answers
// ServiceName and OnStop after construction.
func TestQueue_NewService_Ugly(t *core.T) {
	c := newQueueCore(t)
	r := queue.NewService(queue.Options{PollInterval: -1})(c)
	core.RequireTrue(t, r.OK)
	svc := r.Value.(*queue.Service)
	core.AssertEqual(t, core.Second, svc.Options().PollInterval)
}

// TestQueue_Register_Good — Register is the bare-Options factory
// shorthand suitable for core.WithName("queue", queue.Register).
func TestQueue_Register_Good(t *core.T) {
	c := newQueueCore(t)
	r := queue.Register(c)
	core.RequireTrue(t, r.OK)
	svc, ok := r.Value.(*queue.Service)
	core.RequireTrue(t, ok)
	core.AssertEqual(t, "Queue", svc.ServiceName())
}

// TestQueue_Register_Bad — bare Register on a fresh Core (no orm,
// no schemas) still constructs the Service successfully; the failure
// surface lives at OnStart / Enqueue time, not at construction.
func TestQueue_Register_Bad(t *core.T) {
	c := core.New()
	r := queue.Register(c)
	core.RequireTrue(t, r.OK)
}

// TestQueue_Register_Ugly — invoking Register multiple times produces
// independent Service instances (factory semantics, not singleton).
func TestQueue_Register_Ugly(t *core.T) {
	c := newQueueCore(t)
	a := queue.Register(c).Value.(*queue.Service)
	b := queue.Register(c).Value.(*queue.Service)
	core.AssertNotEqual(t, core.Sprintf("%p", a), core.Sprintf("%p", b))
}

// TestQueue_Worker_DispatchesAndCompletes — full round-trip:
// register handler, start service, enqueue, wait, confirm StatusDone.
// Integration coverage of the worker loop's happy path; per-public
// G/B/U for OnStart lives under TestQueue_NewService_*.
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

	svc := queue.NewService(queue.Options{PollInterval: 50 * core.Millisecond})(c).
		Value.(*queue.Service)
	core.RequireTrue(t, svc.OnStart().OK)

	r := queue.Enqueue(c, "echo", core.NewOptions(
		core.Option{Key: "path", Value: "/echo-target"},
	))
	core.RequireTrue(t, r.OK)
	jobID := r.Value.(queue.Job).ID

	done := make(chan struct{})
	go func() { ran.Wait(); close(done) }()
	select {
	case <-done:
	case <-core.After(2 * core.Second):
		t.Fatalf("handler never ran within 2s")
	}

	core.AssertEqual(t, "/echo-target", receivedPath)

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
// with no registered handler marks the job Failed.
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
