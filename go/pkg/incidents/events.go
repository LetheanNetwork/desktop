// SPDX-Licence-Identifier: EUPL-1.2

package incidents

// EventOpened is broadcast on the Core ACTION bus when a new incident
// is created via Create(). Payload is the IncidentEntry JSON.
//
// Usage example:
//
//	c.Subscribe(incidents.EventOpened, func(ctx core.Context, opts core.Options) core.Result {
//	    id := opts.String("id")
//	    return core.Ok(nil)
//	})
const EventOpened = "incidents.opened"

// EventTransitioned is broadcast when UpdateState() changes an
// incident's lifecycle state. Payload is the updated IncidentEntry JSON.
//
// Usage example:
//
//	c.Subscribe(incidents.EventTransitioned, func(ctx core.Context, opts core.Options) core.Result {
//	    state := opts.String("state")
//	    return core.Ok(nil)
//	})
const EventTransitioned = "incidents.transitioned"
