// SPDX-Licence-Identifier: EUPL-1.2

// Worker — the single-goroutine loop that processes pending jobs.
// Spawned by Service.OnStart via c.waitGroup.Go so Core's
// ServiceShutdown drains it cleanly.
//
// Loop body:
//   1. Sleep PollInterval (or exit if c.shutdown).
//   2. Query orm for the oldest pending job whose ScheduledFor <= now.
//   3. If found: CAS Status pending → running, dispatch via
//      c.Action("queue.kind.<Kind>"), record outcome.
//   4. Loop.
//
// Per-kind parallelism (v2 polish): the dispatch step gets wrapped
// in c.Lock("queue.kind." + job.Kind).TryLock(); skip the job if
// the kind's lock is already held. v1 has a single worker so
// per-kind locking is moot.

package queue

import (
	core "dappco.re/go"
	"dappco.re/go/orm"

	"dappco.re/lthn/desktop/pkg/auth"
)

// runWorker is the loop body. Exits when c.Context() cancels —
// Core's ServiceShutdown cancels the context before draining
// services, which is the queue's "stop now" signal.
func runWorker(c *core.Core, interval core.Duration) {
	if c == nil {
		return
	}
	ctx := c.Context()
	ticker := core.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		// Drain ready jobs in this tick — process more than one
		// per tick when the queue has a backlog so we're not
		// rate-limited by PollInterval. Bounded loop so a huge
		// backlog can't starve other Core work.
		for i := 0; i < 16; i++ {
			if ctx.Err() != nil {
				return
			}
			if !processOne(c) {
				break
			}
		}
	}
}

// processOne picks the oldest pending job and runs it. Returns true
// if a job was processed (regardless of outcome), false when the
// queue is empty / no job is ready yet.
func processOne(c *core.Core) bool {
	job, ok := claimNext(c)
	if !ok {
		return false
	}
	dispatch(c, job)
	return true
}

// claimNext atomically picks the oldest pending job whose
// ScheduledFor has passed and CAS-updates its Status to running.
// Returns (job, true) on success; (zero, false) when nothing is
// ready.
//
// CAS-via-orm: orm.Of[Job].Where("status","=","pending").
//
//	                           Where("scheduled_for","<=",now).
//	                           Order("enqueued_at","asc").
//	                           Limit(1).Get()
//	then Save with Status=running. Single-worker means no race;
//	multi-worker (v2) needs a real CAS (predicate-update returning
//	rows-affected) which orm.DuckDBMedium can support via the
//	ON CONFLICT clause if needed later.
func claimNext(c *core.Core) (Job, bool) {
	now := core.Now().UTC()
	// Memium's predicate engine doesn't honour `<=` on time
	// fields cleanly, so filter ScheduledFor in Go after fetching
	// pending jobs. Bounded by Limit(64) so the in-memory filter
	// stays cheap. DuckDBMedium would push this whole filter
	// server-side; the same code works on either backend.
	r := orm.Of[Job](c).
		Where("status", "=", StatusPending).
		Order("enqueued_at", "asc").
		Limit(64).
		Get()
	if !r.OK {
		return Job{}, false
	}
	jobs, ok := r.Value.([]Job)
	if !ok || len(jobs) == 0 {
		return Job{}, false
	}
	var picked *Job
	for i := range jobs {
		if !jobs[i].ScheduledFor.After(now) {
			picked = &jobs[i]
			break
		}
	}
	if picked == nil {
		return Job{}, false
	}
	job := *picked
	before := job
	job.Status = StatusRunning
	job.Attempts++
	job.StartedAt = now
	if r := orm.Save(c, &job); !r.OK {
		return Job{}, false
	}
	c.ACTION(JobChanged{
		Phase:  PhaseStarted,
		Job:    job,
		Before: before,
		At:     now,
	})
	return job, true
}

// dispatch invokes the registered handler for the job's kind +
// records the outcome. Panics inside the handler are recovered by
// Core's c.Action.Run wrapper; we still defensively recover here in
// case the recovery layer changes.
func dispatch(c *core.Core, job Job) {
	action := c.Action(ActionName(job.Kind))
	now := core.Now().UTC()

	defer func() {
		if rec := recover(); rec != nil {
			markFailed(c, job, core.Sprintf("panic: %v", rec), now)
		}
	}()

	if !action.Exists() {
		markFailed(c, job, "no handler registered for kind: "+job.Kind, now)
		return
	}
	// Reconstruct the enqueuer-CallerIdentity from persisted columns
	// and stamp the dispatch context as TierCascade with the original
	// Subject + Source preserved (inherit-not-promote per RFC §4.5 /
	// §4.5.1 / Mantis #1731). The renderer-tier elevation closure: a
	// renderer Enqueue lands with EnqueuerTier=TierRenderer; worker
	// stamps the handler's Context as TierCascade carrying the
	// renderer's Subject. Cascade-rule allow-lists then decide whether
	// the handler accepts TierCascade triggers — silent privilege
	// promotion is structurally impossible.
	//
	// Cron-registered handlers (wrapped via queue.AsCron at
	// RegisterKind time) override the TierCascade stamp inside their
	// own wrapper body — see register.go AsCron. The worker stamps the
	// dispatch Context unconditionally here; the cron-wrapper sees
	// that stamp arrive, then replaces it with TierCron via
	// auth.AsCron before delegating. This keeps the worker's
	// reconstruction logic ignorant of per-handler tier policy.
	dispatchCtx := auth.AsCascade(c.Context(),
		job.EnqueuerSubject,
		job.EnqueuerSource,
	)
	// Unseal Payload before dispatch (Mantis #1727 at-rest). Legacy
	// plaintext values round-trip unchanged; sealed envelopes
	// decrypt under the unlocked private key. A sealed-but-
	// unreadable Payload (account locked, MAC tampered) fails the
	// job closed rather than dispatching with ciphertext bytes.
	plainPayload, ok := unsealPayload(c, job.Payload)
	if !ok {
		markFailed(c, job, "queue.worker: sealed payload unreadable (account locked or MAC invalid)", core.Now().UTC())
		return
	}
	result := action.Run(dispatchCtx, decodeOptions(plainPayload))
	if !result.OK {
		markFailed(c, job, result.Error(), core.Now().UTC())
		return
	}
	markDone(c, job, core.Now().UTC())
}

// markDone transitions a job from running → done.
func markDone(c *core.Core, job Job, at core.Time) {
	before := job
	job.Status = StatusDone
	job.CompletedAt = at
	job.LastError = ""
	if r := orm.Save(c, &job); !r.OK {
		core.Warn("queue.markDone", "save", r.Error())
		return
	}
	c.ACTION(JobChanged{
		Phase:  PhaseDone,
		Job:    job,
		Before: before,
		At:     at,
	})
}

// markFailed transitions a job from running → failed with the
// stringified error. v1 has no automatic retry; failed jobs stay
// failed until manually re-enqueued. Retry policy is a v2 polish.
func markFailed(c *core.Core, job Job, errMsg string, at core.Time) {
	before := job
	job.Status = StatusFailed
	job.CompletedAt = at
	job.LastError = errMsg
	if r := orm.Save(c, &job); !r.OK {
		core.Warn("queue.markFailed", "save", r.Error())
		return
	}
	c.ACTION(JobChanged{
		Phase:  PhaseFailed,
		Job:    job,
		Before: before,
		At:     at,
	})
}
