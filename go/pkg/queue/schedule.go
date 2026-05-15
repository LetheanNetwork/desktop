// SPDX-Licence-Identifier: EUPL-1.2

// Schedule — deferred-work helpers built on top of Enqueue. Per
// design_cooperative_task_queue: "in 20m check the PR" is the
// canonical pattern. The substrate is the same Job orm record;
// these functions just wrap EnqueueOptions.ScheduledFor with
// friendlier signatures (absolute time, relative duration).
//
// Recurring jobs (cron-style) are a separate primitive — fired by
// a self-rescheduling pattern: the handler completes its work then
// calls Schedule again for the next interval. v2 polish can add
// `queue.Recur(c, every, kind, payload)` that automates this for
// the common cases.

package queue

import (

	core "dappco.re/go"
)

// Schedule enqueues a job to run at a specific future time.
// Returns the persisted Job (Status=pending, ScheduledFor=at).
//
// The worker picks the job up on the first tick after `at` passes.
// Polling latency = the Service's PollInterval (default 1s); jobs
// don't fire EXACTLY at `at`, but within PollInterval after it.
//
// Usage example:
//
//	r := queue.Schedule(c, core.Now().Add(20*core.Minute), "pr-check",
//	    core.NewOptions(core.Option{Key: "pr_id", Value: "1352"}))
func Schedule(c *core.Core, at core.Time, kind string, payload core.Options) core.Result {
	return EnqueueWithOptions(c, kind, EnqueueOptions{
		Payload:      encodeOptions(payload),
		ScheduledFor: at.UTC(),
	})
}

// ScheduleAfter enqueues a job to run after the given duration.
// Convenience wrapper for the most common case ("do X in N minutes").
//
// Usage example:
//
//	r := queue.ScheduleAfter(c, 20*core.Minute, "pr-check",
//	    core.NewOptions(core.Option{Key: "pr_id", Value: "1352"}))
func ScheduleAfter(c *core.Core, after core.Duration, kind string, payload core.Options) core.Result {
	return Schedule(c, core.Now().UTC().Add(after), kind, payload)
}

// ScheduleWithOptions is the full-control deferred enqueue. Use
// when you need Project / IssueID linkage on a scheduled job.
//
// Usage example:
//
//	r := queue.ScheduleWithOptions(c, "pr-check", queue.EnqueueOptions{
//	    Payload:      `{"pr_id":"1352"}`,
//	    ScheduledFor: core.Now().Add(20*core.Minute).UTC(),
//	    Project:      "ide",
//	    IssueID:      "01ABC...",
//	})
func ScheduleWithOptions(c *core.Core, kind string, opts EnqueueOptions) core.Result {
	if opts.ScheduledFor.IsZero() {
		opts.ScheduledFor = core.Now().UTC()
	}
	return EnqueueWithOptions(c, kind, opts)
}

// Reschedule moves an already-enqueued job to a new future time.
// Useful for "back off and try again later" handlers — the handler
// returns Ok (so the job moves to Done), but ALSO enqueues a fresh
// scheduled copy via Reschedule. v2 polish: a richer "retry with
// backoff" helper that keeps Job.Attempts increasing across the
// chain.
//
// For now, Reschedule is just `Schedule` with the same payload —
// callers compose retry/backoff themselves.
//
// Usage example (handler self-retry):
//
//	queue.RegisterKind(c, queue.HandlerOptions{
//	    Kind: "pr-check",
//	    Handler: func(ctx core.Context, opts core.Options) core.Result {
//	        if !checkReady(opts.String("pr_id")) {
//	            queue.ScheduleAfter(c, 5*core.Minute, "pr-check", opts)
//	            return core.Ok("not yet, rescheduled")
//	        }
//	        return doWork(opts)
//	    },
//	})
