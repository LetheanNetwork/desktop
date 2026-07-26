// SPDX-Licence-Identifier: EUPL-1.2

// Wails surface for the social service. Methods are bound on the
// *Service receiver and exposed to the frontend via wails3 generate
// bindings — the TS shape lands at
// frontend-ng/bindings/dappco.re/lthn/desktop/pkg/marketing/social/service.

package social

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/paths"
)

// List returns all social posts, optionally filtered by channel or state.
//
// Dual-format aware (RFC.stage-e-encrypt-at-rest v2 §4.1, Wave 3):
// .lthn files project HEADER-ONLY entries (When / State / Text / Attach
// empty — the frontend renders an "encrypted" placeholder); .md legacy
// files round-trip the full plaintext entry. Reads-while-locked stays
// open — header MAC verify needs only the public key (no unlock
// required).
//
// Usage example:
//
//	r := svc.List(social.ListInput{Channel: "mastodon"})
//	if r.OK { out := r.Value.(social.ListOutput) }
func (s *Service) List(input ListInput) core.Result {
	posts, err := s.loadPosts()
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
// Stage E.D.B.3 surface (RFC.stage-e-encrypt-at-rest v2 §3.1, Wave 3):
// .lthn records require an unlocked session to decrypt the body. When
// loadOne reports the typed "social.session.locked" code (either
// the gate is unwired or no account is unlocked), Get forwards the
// session-locked failure verbatim so the frontend can distinguish
// "encrypted record needs unlock" from a true not-found.
//
// Usage example:
//
//	r := svc.Get("post-20260516")
//	if r.OK { p := r.Value.(social.SocialPost) }
func (s *Service) Get(id string) core.Result {
	if err := paths.IsValidID(id); err != nil {
		return core.Fail(err)
	}
	p, _, err := s.loadOne(id)
	if err != nil {
		// Forward session.locked verbatim so the frontend can render
		// the unlock-prompt distinct from not-found.
		if core.Contains(err.Error(), "social.session.locked") {
			return core.Fail(err)
		}
		return core.Fail(core.E("social.Get", "not found: "+id, err))
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
	if fail, ok := s.assertUnlocked("social.Create"); !ok {
		return fail
	}
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

	// Cascade W2 (RFC §B.3 row 5) — Create is an unconditional first-
	// write (ifVersion=0). writePost stamps Version=1 into the
	// persisted record. Conflict-path (rare on Create — only fires if
	// another goroutine races on the same slug) returns
	// core.Fail(paths.ConflictEnvelope{...}) directly via writePost.
	if wr := s.writePost(dirR.Value.(string), p, 0); !wr.OK {
		return wr
	}
	p.Version = 1
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
	if fail, ok := s.assertUnlocked("social.MarkSent"); !ok {
		return fail
	}
	if err := paths.IsValidID(id); err != nil {
		return core.Fail(err)
	}

	p, dir, err := s.loadOne(id)
	if err != nil {
		if core.Contains(err.Error(), "social.session.locked") {
			return core.Fail(err)
		}
		return core.Fail(core.E("social.MarkSent", "not found: "+id, err))
	}

	priorVersion := p.Version
	p.State = "sent"

	// Cascade W2 (RFC §B.3 row 5) — IfVersion=priorVersion gates the
	// write under the primitive's optimistic-lock check.
	// priorVersion=0 (legacy file predating cutover) skips the check
	// and stamps Version=1 on the upgrade write. Conflict-path returns
	// core.Fail(paths.ConflictEnvelope{...}) directly via writePost
	// (Mantis #1544 gating shape inherited from W1).
	if wr := s.writePost(dir, p, priorVersion); !wr.OK {
		return wr
	}
	p.Version = priorVersion + 1
	if p.Version < 1 {
		p.Version = 1
	}
	s.fireSocialEvent(EventSocialSent, id, "sent")
	return core.Ok(p)
}
