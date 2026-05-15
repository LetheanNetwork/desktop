// SPDX-Licence-Identifier: EUPL-1.2

// Tasks event bus — Issue lifecycle changes broadcast IssueChanged
// messages on Core's IPC bus. Subsystems (linter, builder, PR-watcher,
// agent-prep pipeline, future cascade rules) subscribe via the
// canonical c.RegisterAction shape; this file just defines the
// message type + a small typed-subscribe helper so listeners don't
// need to re-do the msg.(IssueChanged) cast at every callsite.
//
// Per design_cooperative_task_queue: state-changes are the
// orchestration substrate. Core's broadcast does the heavy lifting
// (panic recovery, multi-subscriber fan-out, sync dispatch); this
// package contributes just the Message contract.

package tasks

import (
	"time"

	core "dappco.re/go"
)

// IssueChanged is broadcast on the Core IPC bus whenever an Issue's
// lifecycle moves — created, updated (any field), closed (state →
// done/cancelled), or noted (a Note appended). Listeners receive
// the full Issue snapshot AFTER the change + the BEFORE snapshot so
// they can compute their own diffs without re-querying orm.
//
// Usage example (subscriber):
//
//	c.RegisterAction(func(_ *core.Core, msg core.Message) core.Result {
//	    if ev, ok := msg.(tasks.IssueChanged); ok {
//	        if ev.Kind == tasks.KindClosed { /* … */ }
//	    }
//	    return core.Result{OK: true}
//	})
//
// Or via the typed helper:
//
//	tasks.Subscribe(c, func(_ *core.Core, ev tasks.IssueChanged) {
//	    if ev.Kind == tasks.KindClosed { /* … */ }
//	})
type IssueChanged struct {
	// Kind names which lifecycle moment fired this event. One of
	// the Kind* constants below.
	Kind string `json:"kind"`

	// Issue is the current Issue state (after the change).
	Issue Issue `json:"issue"`

	// Before is the Issue state prior to the change. Zero-value
	// when Kind == KindCreated.
	Before Issue `json:"before"`

	// Note is set only when Kind == KindNoted; the appended note.
	Note Note `json:"note,omitempty"`

	// At is when the broadcast fired. Distinct from Issue.UpdatedAt
	// because consumers may want event-receive time vs orm-write
	// time (clock skew / replay scenarios).
	At time.Time `json:"at"`
}

// Kind enumerates the lifecycle moments. Field names use Kind to
// avoid colliding with Core's named Action system (c.Action("…")).
const (
	KindCreated = "created"
	KindUpdated = "updated"
	KindClosed  = "closed" // a transition to StateDone or StateCancelled
	KindNoted   = "noted"  // a Note appended to an Issue
)

// Subscribe is the typed shorthand for the common case of "I only
// care about IssueChanged messages on this Core". Internally registers
// via c.RegisterAction with the type-assert inline so subscribers
// don't redo it. For multi-message subscribers (caring about
// IssueChanged AND something else), use c.RegisterAction directly.
//
// Listeners run synchronously in the firing goroutine (Core's
// broadcast semantics — handlers panic-recover individually so one
// bad subscriber doesn't take the cascade down). Keep them cheap;
// heavy work goes through the throttled job queue (#367).
//
// Usage example:
//
//	tasks.Subscribe(c, func(c *core.Core, ev tasks.IssueChanged) {
//	    if ev.Issue.Project != "ide" { return }
//	    if ev.Kind == tasks.KindClosed { /* enqueue follow-up */ }
//	})
func Subscribe(c *core.Core, fn func(*core.Core, IssueChanged)) {
	if c == nil || fn == nil {
		return
	}
	c.RegisterAction(func(c *core.Core, msg core.Message) core.Result {
		if ev, ok := msg.(IssueChanged); ok {
			fn(c, ev)
		}
		return core.Result{OK: true}
	})
}
