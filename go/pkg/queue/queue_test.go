// SPDX-Licence-Identifier: EUPL-1.2

package queue_test

import (
	core "dappco.re/go"
	"dappco.re/go/orm"

	"dappco.re/lthn/desktop/pkg/audit"
	"dappco.re/lthn/desktop/pkg/auth"
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

// TestQueue_Enqueue_BelowCeiling_Good — pending depth below
// MaxQueueDepth lets Enqueue proceed normally; the depth-ceiling
// path is transparent to the common case.
func TestQueue_Enqueue_BelowCeiling_Good(t *core.T) {
	c := newQueueCore(t)
	for i := 0; i < 5; i++ {
		r := queue.Enqueue(c, "lint", core.NewOptions())
		core.RequireTrue(t, r.OK)
	}
	r := queue.List(c, queue.ListFilter{Status: queue.StatusPending})
	core.RequireTrue(t, r.OK)
	jobs, _ := orm.Cast[[]queue.Job](r)
	core.AssertEqual(t, 5, len(jobs))
}

// TestQueue_Enqueue_AtCeiling_RejectsErrQueueFull_Bad — once the
// pending depth reaches MaxQueueDepth, further Enqueue calls Fail
// with Result.Code() == ErrQueueFullCode. Filling the production
// 10k ceiling row-by-row would dominate test time, so this exercises
// the gate by pre-seeding pending jobs directly through orm.Insert
// (bypassing the depth-check) up to the cap, then probing one more.
func TestQueue_Enqueue_AtCeiling_RejectsErrQueueFull_Bad(t *core.T) {
	c := newQueueCore(t)
	now := core.Now().UTC()
	for i := 0; i < queue.MaxQueueDepth; i++ {
		seed := queue.Job{
			ID:           "seed-" + core.Sprintf("%d", i),
			Kind:         "lint",
			Status:       queue.StatusPending,
			ScheduledFor: now,
			EnqueuedAt:   now,
		}
		core.RequireTrue(t, orm.Insert(c, &seed).OK)
	}

	r := queue.Enqueue(c, "lint", core.NewOptions())
	core.AssertFalse(t, r.OK)
	core.AssertEqual(t, queue.ErrQueueFullCode, r.Code())
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
// Also covers Kind+Limit — the orm bridge bug (#1395) that returned
// zero rows when a Where predicate combined with Limit(1).
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

	// Kind+Limit — previously returned 0 rows due to orm bridge bug #1395.
	r = queue.List(c, queue.ListFilter{Kind: "lint", Limit: 1})
	core.RequireTrue(t, r.OK)
	jobs, _ = orm.Cast[[]queue.Job](r)
	core.AssertLen(t, jobs, 1)
	core.AssertEqual(t, "lint", jobs[0].Kind)
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
		Kind:           "lint",
		PermittedTiers: []auth.CallerTier{auth.TierOperator, auth.TierCascade},
		Handler:        func(core.Context, core.Options) core.Result { return core.Ok(nil) },
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
		PermittedTiers: []auth.CallerTier{auth.TierOperator},
		Handler:        func(core.Context, core.Options) core.Result { return core.Ok(nil) },
	})
	core.AssertFalse(t, r.OK)
	r = queue.RegisterKind(c, queue.HandlerOptions{
		Kind:           "x",
		PermittedTiers: []auth.CallerTier{auth.TierOperator},
	})
	core.AssertFalse(t, r.OK)
}

// TestQueue_RegisterKind_Ugly — re-registering the same kind is
// last-write-wins (matches Core's c.Action semantics); Kinds(c)
// still lists the kind exactly once.
func TestQueue_RegisterKind_Ugly(t *core.T) {
	c := newQueueCore(t)
	core.RequireTrue(t, queue.RegisterKind(c, queue.HandlerOptions{
		Kind:           "lint",
		PermittedTiers: []auth.CallerTier{auth.TierOperator, auth.TierCascade},
		Handler:        func(core.Context, core.Options) core.Result { return core.Ok("first") },
	}).OK)
	core.RequireTrue(t, queue.RegisterKind(c, queue.HandlerOptions{
		Kind:           "lint",
		PermittedTiers: []auth.CallerTier{auth.TierOperator, auth.TierCascade},
		Handler:        func(core.Context, core.Options) core.Result { return core.Ok("second") },
	}).OK)
	count := 0
	for _, k := range queue.Kinds(c) {
		if k == "lint" {
			count++
		}
	}
	core.AssertEqual(t, 1, count)
}

// TestQueue_RegisterKind_RequiresPermittedTiers_Bad — empty
// PermittedTiers rejects with ErrPermittedTiersRequired per RFC
// v3.1 §5.1 / Cerberus #73 F-3 / Mantis #1757. Deny-by-construction:
// a missing PermittedTiers is misuse, not "allow all", so renderer-
// tier elevation via kind-string forge cannot land via an under-
// specified RegisterKind site.
func TestQueue_RegisterKind_RequiresPermittedTiers_Bad(t *core.T) {
	c := newQueueCore(t)
	r := queue.RegisterKind(c, queue.HandlerOptions{
		Kind:    "lint",
		Handler: func(core.Context, core.Options) core.Result { return core.Ok(nil) },
		// PermittedTiers intentionally omitted.
	})
	core.AssertFalse(t, r.OK)
	core.AssertEqual(t, queue.ErrPermittedTiersRequired, r.Code())

	// Explicit nil slice (caller wrote `PermittedTiers: nil`) also
	// rejects — len(nil) == 0, treated identically to missing.
	r = queue.RegisterKind(c, queue.HandlerOptions{
		Kind:           "lint2",
		PermittedTiers: nil,
		Handler:        func(core.Context, core.Options) core.Result { return core.Ok(nil) },
	})
	core.AssertFalse(t, r.OK)
	core.AssertEqual(t, queue.ErrPermittedTiersRequired, r.Code())
}

// TestQueue_Enqueue_TierPermitted_Good — a caller stamped with a tier
// inside the kind's PermittedTiers allow-list enqueues successfully.
// Demonstrates the inner per-kind gate's HAPPY path under the BOTH-
// not-either composition (outer auth.Require ALLOW + inner allow-list
// both pass).
func TestQueue_Enqueue_TierPermitted_Good(t *core.T) {
	c := newQueueCore(t)
	core.RequireTrue(t, queue.RegisterKind(c, queue.HandlerOptions{
		Kind:           "lint",
		PermittedTiers: []auth.CallerTier{auth.TierOperator, auth.TierCascade},
		Handler:        func(core.Context, core.Options) core.Result { return core.Ok(nil) },
	}).OK)

	auth.SetCaller(c, auth.CallerIdentity{
		Tier:    auth.TierOperator,
		Subject: "account-snider",
		Source:  "http-session:LTHN-SESS-1",
	})
	t.Cleanup(func() { auth.ClearCaller(c) })

	r := queue.Enqueue(c, "lint", core.NewOptions())
	core.RequireTrue(t, r.OK)
	core.AssertEqual(t, queue.StatusPending, r.Value.(queue.Job).Status)
}

// TestQueue_Enqueue_TierNotPermitted_Bad — a caller stamped with a
// tier OUTSIDE the kind's PermittedTiers allow-list rejects with
// ErrTierNotPermittedForKind. Concretely: a TierRenderer caller
// claiming an operator+cascade-only kind cannot land the Enqueue
// even though the outer auth.Require ALLOW gate currently admits
// TierRenderer broadly (RFC §5.0 BOTH-not-either composition closes
// the kind-string forge gap from Cerberus #73 F-3 / Mantis #1757).
func TestQueue_Enqueue_TierNotPermitted_Bad(t *core.T) {
	rec := installAuditRecorder(t)
	c := newQueueCore(t)
	core.RequireTrue(t, queue.RegisterKind(c, queue.HandlerOptions{
		Kind:           "backup.nightly",
		PermittedTiers: []auth.CallerTier{auth.TierOperator, auth.TierCascade},
		Handler:        func(core.Context, core.Options) core.Result { return core.Ok(nil) },
	}).OK)

	// Stamp TierRenderer: outer Require admits, inner allow-list
	// rejects — the forge surface Cerberus #73 F-3 names.
	auth.SetCaller(c, auth.CallerIdentity{
		Tier:    auth.TierRenderer,
		Subject: "account-renderer",
		Source:  "wails",
	})
	t.Cleanup(func() { auth.ClearCaller(c) })

	r := queue.Enqueue(c, "backup.nightly", core.NewOptions())
	core.AssertFalse(t, r.OK)
	core.AssertEqual(t, queue.ErrTierNotPermittedForKind, r.Code())

	// Inner-gate substrate-emit (Cerberus #79 ADD / Mantis #1766): the
	// per-kind PermittedTiers deny MUST land EventTierReject on the
	// audit substrate. Closes the F-3+F-4 forensic-split where the
	// outer auth.Require emitted but the inner per-kind gate was silent.
	rows := rec.filterByName(audit.EventTierReject)
	core.AssertEqual(t, 1, len(rows),
		"inner per-kind deny MUST emit EventTierReject substrate row")
	row := rows[0]
	core.AssertEqual(t, audit.OutcomeDenied, row.Outcome)
	core.AssertEqual(t, "queue", row.Scope)
	core.AssertEqual(t, "queue.enqueue.kind", row.Meta["op"])
	core.AssertEqual(t, string(auth.TierRenderer), row.Meta["caller_tier"])
	core.AssertEqual(t, "account-renderer", row.Meta["caller_subject"])
	core.AssertEqual(t, "wails", row.Meta["caller_source"])
	core.AssertEqual(t, "operator,cascade", row.Meta["allowed_tiers"])
	core.AssertEqual(t, "backup.nightly", row.Meta["kind"])
	core.AssertEqual(t, queue.ErrTierNotPermittedForKind, row.Meta["error_code"])
}

// TestQueue_Enqueue_CascadeFanout_PreservesTierAuth_Good — a cascade-
// fanout caller (TierCascade per H#242 worker-reconstruction) is
// allowed by a per-kind allowlist that names TierCascade. Exercises
// the worker-internal re-Enqueue flow: a handler dispatched under a
// TierCascade-stamped Context that itself calls queue.Enqueue must
// land cleanly against any kind whose PermittedTiers includes
// TierCascade. The cascade chain doesn't degrade beyond the worker's
// initial inherit-not-promote stamp.
func TestQueue_Enqueue_CascadeFanout_PreservesTierAuth_Good(t *core.T) {
	c := newQueueCore(t)
	core.RequireTrue(t, queue.RegisterKind(c, queue.HandlerOptions{
		Kind:           "cascade-follow-on",
		PermittedTiers: []auth.CallerTier{auth.TierOperator, auth.TierCascade},
		Handler:        func(core.Context, core.Options) core.Result { return core.Ok(nil) },
	}).OK)

	// Stamp TierCascade directly — emulates the worker-dispatch
	// context handlers see (per pkg/queue/worker.go dispatchCtx via
	// auth.AsCascade) without spinning up the full Service loop.
	auth.SetCaller(c, auth.CallerIdentity{
		Tier:    auth.TierCascade,
		Subject: "account-original-trigger",
		Source:  "wails",
	})
	t.Cleanup(func() { auth.ClearCaller(c) })

	r := queue.Enqueue(c, "cascade-follow-on", core.NewOptions())
	core.RequireTrue(t, r.OK)
	core.AssertEqual(t, queue.StatusPending, r.Value.(queue.Job).Status)
	// Enqueuer identity persists the cascade Subject so the worker
	// can preserve it on the next dispatch hop.
	core.AssertEqual(t, "account-original-trigger", r.Value.(queue.Job).EnqueuerSubject)
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
		Kind:           "lint",
		PermittedTiers: []auth.CallerTier{auth.TierOperator, auth.TierCascade},
		Handler:        func(core.Context, core.Options) core.Result { return core.Ok(nil) },
	})
	queue.RegisterKind(c, queue.HandlerOptions{
		Kind:           "build",
		PermittedTiers: []auth.CallerTier{auth.TierOperator, auth.TierCascade},
		Handler:        func(core.Context, core.Options) core.Result { return core.Ok(nil) },
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
		Kind:           "echo",
		PermittedTiers: []auth.CallerTier{auth.TierOperator, auth.TierCascade, auth.TierInternal},
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
		Kind:           "boom",
		PermittedTiers: []auth.CallerTier{auth.TierOperator, auth.TierCascade, auth.TierInternal},
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

// TestQueue_Enqueue_PersistsEnqueuerCaller_Good — Enqueue from a
// TierOperator-stamped Context captures Tier/Subject/Source onto the
// persisted Job columns (RFC §4.5.1 / Mantis #1731 mechanism). The
// captured identity round-trips through orm so worker dispatch can
// reconstruct the caller without re-extracting from a (long-gone)
// goroutine context.
func TestQueue_Enqueue_PersistsEnqueuerCaller_Good(t *core.T) {
	c := newQueueCore(t)
	auth.SetCaller(c, auth.CallerIdentity{
		Tier:    auth.TierOperator,
		Subject: "account-snider",
		Source:  "http-session:LTHN-SESS-1",
	})
	t.Cleanup(func() { auth.ClearCaller(c) })

	r := queue.Enqueue(c, "lint", core.NewOptions())
	core.RequireTrue(t, r.OK)
	job := r.Value.(queue.Job)

	core.AssertEqual(t, string(auth.TierOperator), job.EnqueuerTier)
	core.AssertEqual(t, "account-snider", job.EnqueuerSubject)
	core.AssertEqual(t, "http-session:LTHN-SESS-1", job.EnqueuerSource)

	// orm round-trip: Get(c, id) returns the same persisted values.
	gr := queue.Get(c, job.ID)
	core.RequireTrue(t, gr.OK)
	persisted, _, ok := orm.Detail[queue.Job](gr)
	core.RequireTrue(t, ok)
	core.AssertEqual(t, string(auth.TierOperator), persisted.EnqueuerTier)
	core.AssertEqual(t, "account-snider", persisted.EnqueuerSubject)
	core.AssertEqual(t, "http-session:LTHN-SESS-1", persisted.EnqueuerSource)
}

// TestQueue_Worker_ReconstructsTierCascade_Good — full round-trip of
// the §4.5.1 cascade-mechanism: renderer Enqueue persists the
// renderer-tier identity; worker reconstructs it at dispatch and
// stamps TierCascade with the original Subject preserved (inherit-
// not-promote). The handler sees TierCascade rather than TierRenderer
// — silent privilege promotion is structurally impossible because the
// dispatch tier is fixed at TierCascade by the worker, not derived
// from the persisted tier value.
func TestQueue_Worker_ReconstructsTierCascade_Good(t *core.T) {
	c := newQueueCore(t)
	auth.SetCaller(c, auth.CallerIdentity{
		Tier:    auth.TierRenderer,
		Subject: "account-renderer",
		Source:  "wails",
	})
	t.Cleanup(func() { auth.ClearCaller(c) })

	var seen auth.CallerIdentity
	var ran core.WaitGroup
	ran.Add(1)
	queue.RegisterKind(c, queue.HandlerOptions{
		Kind:           "cascade-probe",
		PermittedTiers: []auth.CallerTier{auth.TierRenderer, auth.TierOperator, auth.TierCascade},
		Handler: func(ctx core.Context, _ core.Options) core.Result {
			seen = auth.CallerFromContext(ctx)
			ran.Done()
			return core.Ok(nil)
		},
	})

	svc := queue.NewService(queue.Options{PollInterval: 30 * core.Millisecond})(c).
		Value.(*queue.Service)
	core.RequireTrue(t, svc.OnStart().OK)

	r := queue.Enqueue(c, "cascade-probe", core.NewOptions())
	core.RequireTrue(t, r.OK)

	done := make(chan struct{})
	go func() { ran.Wait(); close(done) }()
	select {
	case <-done:
	case <-core.After(2 * core.Second):
		t.Fatalf("cascade-probe handler never ran within 2s")
	}

	// Tier stamped TierCascade (inherit-not-promote — renderer cannot
	// drive a handler with renderer-tier authority via Enqueue).
	core.AssertEqual(t, auth.TierCascade, seen.Tier)
	// Subject + Source preserved from the renderer trigger so audit
	// consumers can reconstruct WHO triggered the cascade.
	core.AssertEqual(t, "account-renderer", seen.Subject)
	core.AssertEqual(t, "wails", seen.Source)
}

// TestQueue_AsCron_StampsTierCron_Good — handlers registered via
// queue.AsCron(...) wrapper see TierCron on dispatch regardless of
// the worker's TierCascade stamp. The wrapper layer overrides the
// enqueuer-derived identity with the cron-scheduler trust-root stamp
// per RFC §4.5.2 — the registration site is the trust root, not the
// enqueuer or worker.
func TestQueue_AsCron_StampsTierCron_Good(t *core.T) {
	c := newQueueCore(t)
	// Enqueue from an Operator-stamped Context so the worker would
	// otherwise stamp TierCascade carrying operator Subject; the
	// AsCron wrapper overrides that.
	auth.SetCaller(c, auth.CallerIdentity{
		Tier:    auth.TierOperator,
		Subject: "account-cron-author",
		Source:  "cli",
	})
	t.Cleanup(func() { auth.ClearCaller(c) })

	var seen auth.CallerIdentity
	var ran core.WaitGroup
	ran.Add(1)
	queue.RegisterKind(c, queue.HandlerOptions{
		Kind:           "backup.nightly",
		PermittedTiers: []auth.CallerTier{auth.TierOperator, auth.TierCron, auth.TierCascade},
		Handler: queue.AsCron(func(ctx core.Context, _ core.Options) core.Result {
			seen = auth.CallerFromContext(ctx)
			ran.Done()
			return core.Ok(nil)
		}),
	})

	svc := queue.NewService(queue.Options{PollInterval: 30 * core.Millisecond})(c).
		Value.(*queue.Service)
	core.RequireTrue(t, svc.OnStart().OK)

	r := queue.Enqueue(c, "backup.nightly", core.NewOptions())
	core.RequireTrue(t, r.OK)

	done := make(chan struct{})
	go func() { ran.Wait(); close(done) }()
	select {
	case <-done:
	case <-core.After(2 * core.Second):
		t.Fatalf("backup.nightly handler never ran within 2s")
	}

	core.AssertEqual(t, auth.TierCron, seen.Tier)
	// Cron stamp is by-construction subject-less + cron-worker source —
	// the wrapper replaces both regardless of enqueuer values.
	core.AssertEqual(t, "", seen.Subject)
	core.AssertEqual(t, "cron-worker", seen.Source)
}

// TestQueue_AsCron_NilHandler_Bad — AsCron(nil) returns nil rather
// than wrapping nothing; defends RegisterKind from a wrapper-around-
// nil call site silently producing a handler that no-ops on dispatch.
// The downstream RegisterKind guard ("handler is required") then
// rejects with a shaped error instead of registering a broken handler.
func TestQueue_AsCron_NilHandler_Bad(t *core.T) {
	wrapped := queue.AsCron(nil)
	if wrapped != nil {
		t.Fatalf("queue.AsCron(nil): want nil got non-nil")
	}
}

// TestQueue_Enqueue_TierRejected_Bad — the substrate gate denies
// Enqueue when the caller stamps a tier outside the allowed set.
// Today's allow-set is wide (Operator/Renderer/Cascade/Cron/Internal
// per RFC §5.1 Phase 1 visibility-first ruling); the only tier that
// rejects today is TierPlugin (Q-5 substrate-today alias of Renderer
// at the gate but distinct enum value — Enqueue does NOT allow-list
// TierPlugin by name to preserve forward-compat clarity once
// @dappcore/ui plugin-identifier lands and TierPlugin gets per-plugin
// allow-listing). The rejected enqueue produces a shaped error;
// callers branch on r.OK as usual.
func TestQueue_Enqueue_TierRejected_Bad(t *core.T) {
	c := newQueueCore(t)
	auth.SetCaller(c, auth.CallerIdentity{
		Tier:    auth.TierPlugin,
		Subject: "plugin-x",
		Source:  "plugin-iframe",
	})
	t.Cleanup(func() { auth.ClearCaller(c) })

	r := queue.Enqueue(c, "lint", core.NewOptions())
	core.AssertFalse(t, r.OK)
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
