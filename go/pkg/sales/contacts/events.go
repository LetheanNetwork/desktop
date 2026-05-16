// SPDX-Licence-Identifier: EUPL-1.2

package contacts

import core "dappco.re/go"

// EventContactCreated is broadcast on the Core ACTION bus when a new
// contact is created via Create().
//
// Usage example:
//
//	c.Subscribe(contacts.EventContactCreated, func(ctx core.Context, opts core.Options) core.Result {
//	    id := opts.String("id")
//	    return core.Ok(nil)
//	})
const EventContactCreated = "sales.contacts.created"

// EventContactUpdated is broadcast when Update() changes a contact record.
//
// Usage example:
//
//	c.Subscribe(contacts.EventContactUpdated, func(ctx core.Context, opts core.Options) core.Result {
//	    id := opts.String("id")
//	    return core.Ok(nil)
//	})
const EventContactUpdated = "sales.contacts.updated"

// ContactEvent is the Core ACTION bus payload for contact lifecycle changes.
//
// Usage example:
//
//	c.RegisterAction(func(c *core.Core, msg core.Message) core.Result {
//	    if ev, ok := msg.(contacts.ContactEvent); ok {
//	        _ = ev.Contact.ID
//	    }
//	    return core.Result{OK: true}
//	})
type ContactEvent struct {
	// EventName is the event constant that triggered this message.
	EventName string `json:"event"`

	// Contact is the contact state after the change.
	Contact Contact `json:"contact"`

	// At is the event timestamp.
	At core.Time `json:"at"`
}
