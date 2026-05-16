// SPDX-Licence-Identifier: EUPL-1.2

package content

import core "dappco.re/go"

// EventContentCreated is broadcast on the Core ACTION bus when a content
// item is created via Create().
//
// Usage example:
//
//	c.Subscribe(content.EventContentCreated, func(ctx core.Context, opts core.Options) core.Result {
//	    id := opts.String("itemId")
//	    return core.Ok(nil)
//	})
const EventContentCreated = "marketing.content.created"

// EventContentAdvanced is broadcast on the Core ACTION bus when a content
// item is advanced to the next pipeline column via Advance().
const EventContentAdvanced = "marketing.content.advanced"

// ContentEvent is the Core ACTION bus payload for a content item state change.
//
// Usage example:
//
//	c.RegisterAction(func(c *core.Core, msg core.Message) core.Result {
//	    if ev, ok := msg.(content.ContentEvent); ok {
//	        _ = ev.ItemID
//	        _ = ev.Col
//	    }
//	    return core.Result{OK: true}
//	})
type ContentEvent struct {
	// EventName is the event constant.
	EventName string `json:"event"`
	// ItemID is the content item slug.
	ItemID string `json:"itemId"`
	// Col is the current column after the event.
	Col string `json:"col"`
	// At is the event timestamp.
	At core.Time `json:"at"`
}
