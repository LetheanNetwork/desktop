// SPDX-Licence-Identifier: EUPL-1.2

package runbooks

import core "dappco.re/go"

// EventRehearsed is broadcast on the Core ACTION bus when a runbook's
// LastRehearsed timestamp is updated via MarkRehearsed(). Payload is
// the updated RunbookEntry.
//
// Usage example:
//
//	c.RegisterAction(func(c *core.Core, msg core.Message) core.Result {
//	    if ev, ok := msg.(runbooks.RunbookEvent); ok {
//	        _ = ev.Entry.ID
//	    }
//	    return core.Result{OK: true}
//	})
const EventRehearsed = "runbooks.rehearsed"

// RunbookEvent is the Core ACTION bus payload for runbook events.
//
// Usage example:
//
//	c.RegisterAction(func(c *core.Core, msg core.Message) core.Result {
//	    if ev, ok := msg.(runbooks.RunbookEvent); ok {
//	        _ = ev.Entry.Health
//	    }
//	    return core.Result{OK: true}
//	})
type RunbookEvent struct {
	// EventName is the event constant that triggered this message.
	EventName string `json:"event"`

	// Entry is the runbook state after the change.
	Entry RunbookEntry `json:"entry"`

	// At is the event timestamp.
	At core.Time `json:"at"`
}
