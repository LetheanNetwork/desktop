// SPDX-Licence-Identifier: EUPL-1.2

package social

import core "dappco.re/go"

// EventSocialCreated is broadcast on the Core ACTION bus when a post
// is created via Create().
//
// Usage example:
//
//	c.Subscribe(social.EventSocialCreated, func(ctx core.Context, opts core.Options) core.Result {
//	    id := opts.String("postId")
//	    return core.Ok(nil)
//	})
const EventSocialCreated = "marketing.social.created"

// EventSocialSent is broadcast on the Core ACTION bus when a post is
// marked sent via MarkSent().
const EventSocialSent = "marketing.social.sent"

// SocialEvent is the Core ACTION bus payload for a social post state change.
//
// Usage example:
//
//	c.RegisterAction(func(c *core.Core, msg core.Message) core.Result {
//	    if ev, ok := msg.(social.SocialEvent); ok {
//	        _ = ev.PostID
//	        _ = ev.State
//	    }
//	    return core.Result{OK: true}
//	})
type SocialEvent struct {
	// EventName is the event constant.
	EventName string `json:"event"`
	// PostID is the post slug.
	PostID string `json:"postId"`
	// State is the new post state.
	State string `json:"state"`
	// At is the event timestamp.
	At core.Time `json:"at"`
}
