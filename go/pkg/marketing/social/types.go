// SPDX-Licence-Identifier: EUPL-1.2

// Package social is the lthn-side social post queue service. Manages
// social posts at ~/Lethean/marketing/social/{id}.md — each post carries
// a channel list, schedule time, state, body text, and optional attachment.
//
// Wire shapes match the SocialPost interface consumed by the
// <lthn-view-social> Lit element in the Marketing role view.
//
// Usage example (Wails):
//
//	r := socialSvc.List(social.ListInput{})
//	if r.OK { out := r.Value.(social.ListOutput) }
package social

// SocialPost is the JSON wire type for a single social post card.
// Field names match the SocialPost interface in
// frontend/src/lit/views/marketing/social.ts.
//
// Usage example:
//
//	p := social.SocialPost{
//	    ID: "post-20260516", Ch: []string{"mastodon", "x"},
//	    When: "today · 16:00", State: "scheduled",
//	    Text: "Lethean v0.2 is out.",
//	}
type SocialPost struct {
	// ID is the post slug (filename key).
	ID string `json:"id"`
	// Ch is the list of target channels: "mastodon"|"x"|"linkedin"|"bluesky".
	Ch []string `json:"ch"`
	// When is the human-friendly schedule label ("today · 16:00", "yest · 11:14").
	When string `json:"when"`
	// State is the post state: "scheduled"|"sent"|"draft".
	State string `json:"state"`
	// Text is the post body text.
	Text string `json:"text"`
	// Attach is the optional attachment label ("image", "video"). Empty = no attachment.
	Attach string `json:"attach,omitempty"`

	// Version is the monotonic optimistic-lock version (Cascade W2,
	// RFC §B.3 row 5). Stamped by writePost; 0 on legacy files
	// predating the cutover, >=1 after first write through
	// paths.AtomicWriteWithVersion. Internal-only — `json:"-"` keeps
	// it out of the Wails wire shape consumed by the frontend.
	Version int `json:"-"`
}

// ListInput drives the List method.
//
// Usage example:
//
//	r := svc.List(social.ListInput{Channel: "mastodon"})
type ListInput struct {
	// Channel filters to posts targeting a specific channel. Empty = all.
	Channel string `json:"channel,omitempty"`
	// State filters to a single state. Empty = all states.
	State string `json:"state,omitempty"`
}

// ListOutput is the List response envelope.
//
// Usage example:
//
//	out := r.Value.(social.ListOutput)
//	for _, p := range out.Posts { _ = p.Text }
type ListOutput struct {
	// Posts is the ordered list of posts.
	Posts []SocialPost `json:"posts"`
	// ScheduledCount is the count of posts in "scheduled" state.
	ScheduledCount int `json:"scheduledCount"`
}

// CreateInput drives the Create method.
//
// Usage example:
//
//	r := svc.Create(social.CreateInput{
//	    Ch: []string{"mastodon", "x"}, When: "today · 16:00",
//	    Text: "Lethean v0.2 is out.",
//	})
type CreateInput struct {
	// Ch is the target channel list. Required; at least one entry.
	Ch []string `json:"ch"`
	// When is the schedule label. Required.
	When string `json:"when"`
	// Text is the post body. Required.
	Text string `json:"text"`
	// State is the initial state. Defaults to "draft".
	State string `json:"state,omitempty"`
	// Attach is the optional attachment label.
	Attach string `json:"attach,omitempty"`
}

// UpdateInput drives the Update method. All fields except ID are optional patches.
//
// Usage example:
//
//	r := svc.Update(social.UpdateInput{ID: "post-20260516", State: "sent"})
type UpdateInput struct {
	// ID is the post slug. Required.
	ID string `json:"id"`
	// Ch, When, State, Text, Attach — optional patch fields.
	Ch     []string `json:"ch,omitempty"`
	When   string   `json:"when,omitempty"`
	State  string   `json:"state,omitempty"`
	Text   string   `json:"text,omitempty"`
	Attach string   `json:"attach,omitempty"`
}
