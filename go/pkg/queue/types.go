// SPDX-Licence-Identifier: EUPL-1.2

// Package queue is the throttled background job substrate for
// lthn/desktop. Per design_cooperative_task_queue: capture-greedy /
// execute-throttled. Subsystems enqueue work (lint runs, http
// probes, image pulls, agent-prep pipelines); a single worker
// processes them sequentially so lthn never burst-takes the
// machine. Per-kind parallelism is a one-line Lock-based addition
// when a workload demands it; v1 stays sequential by design.
//
// Surface (Go):
//
//	queue.Register(c, queue.HandlerOptions{
//	    Kind: "lint",
//	    Handler: func(ctx core.Context, opts core.Options) core.Result { … },
//	})
//	job := queue.Enqueue(c, "lint", core.NewOptions(
//	    core.Option{Key: "path", Value: "/foo"},
//	))
//	queue.Subscribe(c, func(c *core.Core, ev queue.JobChanged) { … })
//
// Surface (CLI / MCP / API — auto-bound via c.Command registrations
// in api.go):
//
//	lthn queue list
//	lthn queue enqueue --kind lint --payload '{"path":"/foo"}'
//	lthn queue cancel <job-id>
//
// Persistence: Job records live in ~/Lethean/data/tasks.duckdb
// alongside tasks_issues / tasks_notes / opencode_* tables.

package queue

import (
	"time"

	"dappco.re/go/orm"
)

// Job is one unit of background work persisted in the orm.
//
// Lifecycle: enqueued → pending → running → done | failed | cancelled.
// Worker claims a job by CAS-updating Status from pending to running.
type Job struct {
	// ID is the primary key — generated at enqueue time.
	ID string

	// Kind drives handler dispatch via the Core action
	// "queue.kind.<Kind>". Handlers are registered via Register.
	Kind string

	// Payload is the JSON-encoded options the handler receives.
	// Worker passes them through as core.Options at dispatch.
	Payload string

	// Status moves through the lifecycle (StatusPending → StatusRunning
	// → StatusDone / StatusFailed / StatusCancelled). Worker uses
	// CAS-like orm updates to claim work safely under concurrent
	// workers (v2 — v1 has a single worker so contention is moot).
	Status string

	// ScheduledFor is when the job becomes eligible to run. Set to
	// time.Now() for immediate execution; future for deferred work.
	// The worker's WHERE clause filters out jobs scheduled for the
	// future. (Schedule(at, fn) helper lands in #368.)
	ScheduledFor time.Time

	// Attempts increments each time the worker dispatches the job.
	// Failed jobs that should retry get Status reset to pending +
	// ScheduledFor advanced; v1 has no retry policy (failed = stays
	// failed until manually re-enqueued).
	Attempts int

	// LastError is the stringified failure from the most recent
	// dispatch. Empty when the job has never failed.
	LastError string

	// Project + IssueID optionally link the job to a tasks.Issue
	// — the cross-cascade glue. Empty when the job stands alone
	// (e.g. user-triggered ad-hoc enqueue).
	Project string
	IssueID string

	// EnqueuedAt is when the job was created.
	EnqueuedAt time.Time

	// StartedAt + CompletedAt are the worker's claim/release
	// timestamps. Zero when the job hasn't reached that lifecycle
	// moment yet.
	StartedAt   time.Time
	CompletedAt time.Time
}

// Schema declares the orm shape for Job. Registered in
// cmd/lthn/app.go alongside the opencode + tasks schemas so a
// fresh ~/Lethean/data/tasks.duckdb gets the table on first boot.
//
// Indexes target the worker's hot query: WHERE status = 'pending'
// AND scheduled_for <= now ORDER BY enqueued_at ASC. Composite
// index would be optimal; v1 uses single-column indexes which are
// good enough at lthn-scale (thousands of pending jobs max).
func (Job) Schema() orm.Schema {
	return orm.Define(func(b *orm.Builder) {
		b.Name("queue_jobs")
		b.PK("id")
		// Only the routing keys are NotNull — same lesson learned
		// in pkg/opencode/imports.go: Memium treats zero-values as
		// null, so b.Int("attempts").NotNull() with attempts=0
		// rejects every fresh insert. Empty strings + zero times
		// + zero ints stay nullable.
		b.String("id").NotNull()
		b.String("kind").NotNull()
		b.String("status").NotNull()
		b.String("payload")
		b.Time("scheduled_for")
		b.Int("attempts")
		b.String("last_error")
		b.String("project")
		b.String("issue_id")
		b.Time("enqueued_at")
		b.Time("started_at")
		b.Time("completed_at")
		b.Index("status")
		b.Index("kind")
		b.Index("scheduled_for")
		b.Index("issue_id")
	})
}

// Schemas exposes the orm schemas this package owns. Registered by
// cmd/lthn/app.go alongside opencode + tasks schemas.
func Schemas() []orm.Schema {
	return []orm.Schema{Job{}.Schema()}
}

// Status values trace the Job lifecycle.
const (
	// StatusPending — created + ready to run when ScheduledFor passes.
	StatusPending = "pending"
	// StatusRunning — worker has claimed the job + is dispatching.
	StatusRunning = "running"
	// StatusDone — handler returned core.Ok.
	StatusDone = "done"
	// StatusFailed — handler returned core.Fail (or panicked).
	StatusFailed = "failed"
	// StatusCancelled — user / system cancelled before completion.
	StatusCancelled = "cancelled"
)
