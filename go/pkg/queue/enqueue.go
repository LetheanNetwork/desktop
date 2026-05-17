// SPDX-Licence-Identifier: EUPL-1.2

// Enqueue / Cancel / List — the writer surface. Persistence via
// orm; events via Core's IPC bus. No worker logic here (worker
// lives in service.go + worker.go); this file is "what users do
// to push work into the queue".

package queue

import (

	core "dappco.re/go"
	"dappco.re/go/orm"
)

// EnqueueOptions configures an Enqueue call. Most callers use
// Enqueue (the default-options shorthand); EnqueueWithOptions
// when fine-grained control is needed (Project linkage, deferred
// execution, etc.).
type EnqueueOptions struct {
	// Payload is the JSON-encoded args the handler receives at
	// dispatch. Use core.JSONMarshalString on a struct or
	// map[string]any to produce it.
	Payload string

	// ScheduledFor defers execution until this time. Zero =
	// run as soon as the worker picks it up. Future =
	// the worker skips until time arrives. (#368 Schedule helper
	// wraps this for cleaner API.)
	ScheduledFor core.Time

	// Project + IssueID optionally link the job to a tasks.Issue
	// for cascade traceability ("which Issue is this work for?").
	Project string
	IssueID string
}

// Enqueue persists a job for the given kind with default options
// (immediate execution, no Issue linkage, payload from the supplied
// core.Options encoded as JSON). The most common shorthand.
//
// Usage example:
//
//	r := queue.Enqueue(c, "lint", core.NewOptions(
//	    core.Option{Key: "path", Value: "/Users/snider/Code/lthn/desktop"},
//	))
//	if r.OK { job := r.Value.(queue.Job); _ = job }
func Enqueue(c *core.Core, kind string, payload core.Options) core.Result {
	return EnqueueWithOptions(c, kind, EnqueueOptions{
		Payload: encodeOptions(payload),
	})
}

// EnqueueWithOptions is the full-control enqueue. Persists the Job +
// fires JobChanged{Phase: PhaseEnqueued} on the IPC bus.
//
// Depth ceiling (Cerberus #64 F-3 / Mantis #1724): the pending-job
// count is checked against MaxQueueDepth under c.Lock("queue.enqueue")
// so concurrent Enqueue calls can't all race past the cap. A concurrent
// worker claim only DECREASES pending-count (pending → running), so
// the cap is never spuriously breached by claim activity outside the
// lock. Rejection returns ErrQueueFullCode via core.ErrorCode.
func EnqueueWithOptions(c *core.Core, kind string, opts EnqueueOptions) core.Result {
	if c == nil {
		return core.Fail(core.E("queue.Enqueue", "core is nil", nil))
	}
	if kind == "" {
		return core.Fail(core.E("queue.Enqueue", "kind is required", nil))
	}
	// Atomic depth-check + insert. Per-Core named mutex serialises
	// concurrent Enqueue paths; worker claim mutates Status outside
	// this lock but only ever lowers pending-count, so reading depth
	// under the lock is safe.
	lock := c.Lock("queue.enqueue")
	lock.Lock()
	defer lock.Unlock()
	if r := orm.Of[Job](c).Where("status", "=", StatusPending).Count(); r.OK {
		depth, _ := r.Value.(int64)
		if depth >= MaxQueueDepth {
			// NewCode populates Err.Code so Result.Code() exposes the
			// sentinel value directly. core.E sets Operation but leaves
			// Code empty — callers branching on r.Code() need the
			// Code-bearing constructor.
			return core.Fail(core.NewCode(ErrQueueFullCode,
				"queue.Enqueue: queue at max depth (10000); reject pending Enqueue"))
		}
	}
	now := core.Now().UTC()
	scheduled := opts.ScheduledFor
	if scheduled.IsZero() {
		scheduled = now
	}
	job := Job{
		ID:           newJobID(),
		Kind:         kind,
		Payload:      opts.Payload,
		Status:       StatusPending,
		ScheduledFor: scheduled,
		Project:      opts.Project,
		IssueID:      opts.IssueID,
		EnqueuedAt:   now,
	}
	if r := orm.Insert(c, &job); !r.OK {
		return r
	}
	c.ACTION(JobChanged{
		Phase: PhaseEnqueued,
		Job:   job,
		At:    now,
	})
	return core.Ok(job)
}

// Cancel marks a job as cancelled if it's still pending. Running
// jobs cannot be cancelled mid-flight in v1 (handler isn't passed
// a cancellable context yet); user-cancellation of running work is
// a v2 polish.
//
// Returns Ok with the cancelled Job, or Fail if the job is already
// terminal (done / failed / cancelled).
//
// Usage example:
//
//	r := queue.Cancel(c, "j-1735…")
//	if r.OK { /* cancelled */ }
func Cancel(c *core.Core, id string) core.Result {
	r := orm.Of[Job](c).Find(id)
	if !r.OK {
		return r
	}
	job, _, ok := orm.Detail[Job](r)
	if !ok {
		return core.Fail(core.E("queue.Cancel", "cast job", nil))
	}
	if job.Status != StatusPending {
		return core.Fail(core.E("queue.Cancel",
			"job is not pending (status="+job.Status+")", nil))
	}
	before := job
	job.Status = StatusCancelled
	job.CompletedAt = core.Now().UTC()
	if r := orm.Save(c, &job); !r.OK {
		return r
	}
	c.ACTION(JobChanged{
		Phase:  PhaseCancelled,
		Job:    job,
		Before: before,
		At:     job.CompletedAt,
	})
	return core.Ok(job)
}

// ListFilter narrows a List query. Empty fields = no constraint.
type ListFilter struct {
	Status  string // "" / StatusPending / StatusRunning / etc.
	Kind    string
	Project string
	IssueID string
	Limit   int
	Offset  int
}

// List returns jobs matching the filter, newest enqueued first.
//
// Usage example:
//
//	r := queue.List(c, queue.ListFilter{Status: queue.StatusPending, Limit: 50})
//	if r.OK { jobs := r.Value.([]queue.Job); _ = jobs }
func List(c *core.Core, filter ListFilter) core.Result {
	bridge := orm.Of[Job](c)
	if filter.Status != "" {
		bridge = bridge.Where("status", "=", filter.Status)
	}
	if filter.Kind != "" {
		bridge = bridge.Where("kind", "=", filter.Kind)
	}
	if filter.Project != "" {
		bridge = bridge.Where("project", "=", filter.Project)
	}
	if filter.IssueID != "" {
		bridge = bridge.Where("issue_id", "=", filter.IssueID)
	}
	bridge = bridge.Order("enqueued_at", "desc")
	if filter.Limit > 0 {
		bridge = bridge.Limit(filter.Limit)
	}
	if filter.Offset > 0 {
		bridge = bridge.Offset(filter.Offset)
	}
	return bridge.Get()
}

// Get returns one Job by id.
//
// Usage example:
//
//	r := queue.Get(c, "j-1735…")
//	if r.OK { job, _, _ := orm.Detail[queue.Job](r); _ = job }
func Get(c *core.Core, id string) core.Result {
	return orm.Of[Job](c).Find(id)
}

// newJobID is a small wrapper around core.RandomString so the
// queue package controls its own id shape ("j-" prefix for grep-
// friendliness in logs).
func newJobID() string {
	r := core.RandomString(12)
	if !r.OK {
		// Fallback: nanosecond timestamp — sortable, unique enough
		// for rare crypto/rand failure paths.
		return core.Sprintf("j-%d", core.Now().UnixNano())
	}
	return "j-" + r.Value.(string)
}

// encodeOptions renders a core.Options as JSON for the Job.Payload
// column. Worker decodes back at dispatch time.
//
// core.Options has its own JSON shape that doesn't round-trip to a
// flat object — Items() exposes the underlying []Option slice so we
// build the map[string]any view ourselves.
func encodeOptions(opts core.Options) string {
	if opts.Len() == 0 {
		return "{}"
	}
	flat := map[string]any{}
	for _, item := range opts.Items() {
		flat[item.Key] = item.Value
	}
	return core.JSONMarshalString(flat)
}

// decodeOptions reverses encodeOptions for the worker dispatch
// path. JSON object → core.Options.
func decodeOptions(payload string) core.Options {
	if payload == "" || payload == "{}" {
		return core.NewOptions()
	}
	out := map[string]any{}
	if r := core.JSONUnmarshalString(payload, &out); !r.OK {
		return core.NewOptions()
	}
	opts := core.NewOptions()
	for k, v := range out {
		opts.Set(k, v)
	}
	return opts
}
