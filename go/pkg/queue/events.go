// SPDX-Licence-Identifier: EUPL-1.2

// Queue event bus — Job lifecycle changes broadcast JobChanged on
// Core's IPC bus (c.ACTION + c.RegisterAction). Subsystems subscribe
// to track progress, drive cascade rules, surface to UI, etc.
//
// Mirrors the tasks.IssueChanged shape so subscribers using one
// pattern can use the other unchanged.

package queue

import (
	core "dappco.re/go"
)

// JobChanged is broadcast on the Core IPC bus on every Job lifecycle
// transition. Carries the full Job snapshot AFTER the change + the
// BEFORE snapshot (zero-value when Phase == PhaseEnqueued).
//
// Usage example (typed Subscribe shorthand):
//
//	queue.Subscribe(c, func(c *core.Core, ev queue.JobChanged) {
//	    if ev.Phase == queue.PhaseDone { /* … */ }
//	})
//
// Or via canonical Core IPC for multi-message subscribers:
//
//	c.RegisterAction(func(c *core.Core, msg core.Message) core.Result {
//	    if ev, ok := msg.(queue.JobChanged); ok { … }
//	    return core.Result{OK: true}
//	})
type JobChanged struct {
	// Phase names which lifecycle moment fired this event.
	Phase string `json:"phase"`

	// Job is the current Job state (after the change).
	Job Job `json:"job"`

	// Before is the Job state prior to the change. Zero-value when
	// Phase == PhaseEnqueued.
	Before Job `json:"before"`

	// At is when the broadcast fired.
	At core.Time `json:"at"`
}

// Phase constants name the lifecycle transitions a JobChanged event
// reports. Field name is Phase (not Action / Kind / Status) to
// avoid colliding with Core's named Action system (c.Action) AND
// with Job.Kind (the handler-dispatch identifier) AND with Job.Status
// (the persisted state).
const (
	PhaseEnqueued  = "enqueued"  // Job persisted, awaiting worker pickup
	PhaseStarted   = "started"   // Worker claimed; handler about to dispatch
	PhaseDone      = "done"      // Handler returned OK
	PhaseFailed    = "failed"    // Handler returned Fail or panicked
	PhaseCancelled = "cancelled" // User / system cancelled
)

// Subscribe is the typed shorthand for "I only care about JobChanged
// messages on this Core". Wraps c.RegisterAction with the
// msg.(JobChanged) cast inline. For multi-message subscribers, use
// c.RegisterAction directly.
//
// Listeners run synchronously in Core's broadcast goroutine; keep
// them cheap. Heavy work goes back into the queue as a follow-up
// Enqueue call (the cascade pattern).
//
// Usage example:
//
//	queue.Subscribe(c, func(c *core.Core, ev queue.JobChanged) {
//	    if ev.Job.Project != "ide" { return }
//	    if ev.Phase == queue.PhaseFailed {
//	        queue.Enqueue(c, "agent-prep", core.NewOptions(
//	            core.Option{Key: "job_id", Value: ev.Job.ID},
//	        ))
//	    }
//	})
func Subscribe(c *core.Core, fn func(*core.Core, JobChanged)) {
	if c == nil || fn == nil {
		return
	}
	c.RegisterAction(func(c *core.Core, msg core.Message) core.Result {
		if ev, ok := msg.(JobChanged); ok {
			fn(c, ev)
		}
		return core.Result{OK: true}
	})
}
