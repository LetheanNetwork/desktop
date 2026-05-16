// SPDX-Licence-Identifier: EUPL-1.2

package audience

import core "dappco.re/go"

// EventAudienceCreated is broadcast on the Core ACTION bus when a segment
// is created via Create().
//
// Usage example:
//
//	c.Subscribe(audience.EventAudienceCreated, func(ctx core.Context, opts core.Options) core.Result {
//	    id := opts.String("segmentId")
//	    return core.Ok(nil)
//	})
const EventAudienceCreated = "marketing.audience.created"

// EventAudienceUpdated is broadcast on the Core ACTION bus when a segment
// is updated via Update().
const EventAudienceUpdated = "marketing.audience.updated"

// AudienceEvent is the Core ACTION bus payload for a segment state change.
//
// Usage example:
//
//	c.RegisterAction(func(c *core.Core, msg core.Message) core.Result {
//	    if ev, ok := msg.(audience.AudienceEvent); ok {
//	        _ = ev.SegmentID
//	        _ = ev.N
//	    }
//	    return core.Result{OK: true}
//	})
type AudienceEvent struct {
	// EventName is the event constant.
	EventName string `json:"event"`
	// SegmentID is the segment slug.
	SegmentID string `json:"segmentId"`
	// N is the current subscriber count at the time of the event.
	N int `json:"n"`
	// At is the event timestamp.
	At core.Time `json:"at"`
}
