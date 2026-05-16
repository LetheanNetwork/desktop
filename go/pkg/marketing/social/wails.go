// SPDX-Licence-Identifier: EUPL-1.2

// Wails surface for the social service. Methods are bound on the
// *Service receiver and exposed to the frontend via wails3 generate
// bindings — the TS shape lands at
// frontend/bindings/dappco.re/lthn/desktop/pkg/marketing/social/service.

package social

import (
	core "dappco.re/go"
)

// List returns all social posts, optionally filtered by channel or state.
//
// Usage example:
//
//	r := svc.List(social.ListInput{Channel: "mastodon"})
//	if r.OK { out := r.Value.(social.ListOutput) }
func (s *Service) List(input ListInput) core.Result {
	posts, err := loadPosts()
	if err != nil {
		return core.Fail(core.E("social.List", "scan failed", err))
	}

	// Compute ScheduledCount from the full (unfiltered) set.
	scheduledCount := 0
	for _, p := range posts {
		if p.State == "scheduled" {
			scheduledCount++
		}
	}

	filtered := posts
	if input.Channel != "" || input.State != "" {
		filtered = make([]SocialPost, 0, len(posts))
		for _, p := range posts {
			if input.Channel != "" && !containsChannel(p.Ch, input.Channel) {
				continue
			}
			if input.State != "" && p.State != input.State {
				continue
			}
			filtered = append(filtered, p)
		}
	}

	if filtered == nil {
		filtered = []SocialPost{}
	}

	return core.Ok(ListOutput{
		Posts:          filtered,
		ScheduledCount: scheduledCount,
	})
}

// Get returns a single social post by ID.
//
// Usage example:
//
//	r := svc.Get("post-20260516")
//	if r.OK { p := r.Value.(social.SocialPost) }
func (s *Service) Get(id string) core.Result {
	if id == "" {
		return core.Fail(core.E("social.Get", "id is required", nil))
	}
	dirR := socialDir()
	if !dirR.OK {
		return core.Fail(core.E("social.Get", dirR.Error(), nil))
	}
	raw := core.ReadFile(core.PathJoin(dirR.Value.(string), id+".md"))
	if !raw.OK {
		return core.Fail(core.E("social.Get", "not found: "+id, nil))
	}
	p, err := parsePost(raw.Value.([]byte))
	if err != nil {
		return core.Fail(core.E("social.Get", "parse failed", err))
	}
	return core.Ok(p)
}

// Create creates a new social post in the queue.
//
// Usage example:
//
//	r := svc.Create(social.CreateInput{
//	    Ch: []string{"mastodon", "x"}, When: "today · 16:00",
//	    Text: "Lethean v0.2 is out.",
//	})
//	if r.OK { p := r.Value.(social.SocialPost) }
func (s *Service) Create(input CreateInput) core.Result {
	if len(input.Ch) == 0 {
		return core.Fail(core.E("social.Create", "at least one channel is required", nil))
	}
	if input.Text == "" {
		return core.Fail(core.E("social.Create", "text is required", nil))
	}
	dirR := socialDir()
	if !dirR.OK {
		return core.Fail(core.E("social.Create", dirR.Error(), nil))
	}

	state := input.State
	if state == "" {
		state = "draft"
	}

	ts := core.Now().UTC().Unix()
	id := core.Sprintf("post-%d", ts)

	p := SocialPost{
		ID:     id,
		Ch:     input.Ch,
		When:   input.When,
		State:  state,
		Text:   input.Text,
		Attach: input.Attach,
	}

	if r := writePost(dirR.Value.(string), p); !r.OK {
		return core.Fail(core.E("social.Create", "write failed", nil))
	}
	s.fireSocialEvent(EventSocialCreated, id, state)
	return core.Ok(p)
}

// MarkSent transitions a post to "sent" state.
//
// Usage example:
//
//	r := svc.MarkSent("post-20260516")
//	if r.OK { p := r.Value.(social.SocialPost) }
func (s *Service) MarkSent(id string) core.Result {
	if id == "" {
		return core.Fail(core.E("social.MarkSent", "id is required", nil))
	}
	dirR := socialDir()
	if !dirR.OK {
		return core.Fail(core.E("social.MarkSent", dirR.Error(), nil))
	}

	raw := core.ReadFile(core.PathJoin(dirR.Value.(string), id+".md"))
	if !raw.OK {
		return core.Fail(core.E("social.MarkSent", "not found: "+id, nil))
	}
	p, err := parsePost(raw.Value.([]byte))
	if err != nil {
		return core.Fail(core.E("social.MarkSent", "parse failed", err))
	}

	p.State = "sent"
	if r := writePost(dirR.Value.(string), p); !r.OK {
		return core.Fail(core.E("social.MarkSent", "write failed", nil))
	}
	s.fireSocialEvent(EventSocialSent, id, "sent")
	return core.Ok(p)
}
