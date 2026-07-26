// SPDX-Licence-Identifier: EUPL-1.2

// Package social is the lthn-side social post queue service. Manages
// social posts at ~/Lethean/marketing/social/{id}.lthn — each post carries
// a channel list, schedule time, state, body text, and optional attachment.
//
// Stage E.D.B.3 (Mantis #1487 wave 3): the on-disk format is now the
// encrypted Trix envelope (`.lthn`). Legacy `.md` plaintext records
// remain readable via the lazy-migration fallthrough until a write
// promotes them to `.lthn` per RFC §3.1.
//
// Wire shapes match the SocialPost interface consumed by the
// Angular social surface in the Marketing category.
//
// Usage example (Wails):
//
//	r := socialSvc.List(social.ListInput{})
//	if r.OK { out := r.Value.(social.ListOutput) }
package social

// SocialPost is the JSON wire type for a single social post card.
// Field names match the SocialPost interface in
// frontend-ng/src/app/desktop/surfaces/marketing/social.ts.
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

// SocialPostRecord is the persistence type carried through the at-rest
// substrate as `AtRestWriter[SocialPostRecord]`. Stage E.D.B.3 (Mantis
// #1487 wave 3) splits the wire type (SocialPost, json-tagged) from the
// persistence type (SocialPostRecord, yaml-tagged) so the encrypted body
// frontmatter never leaks json key names and the
// recordfile.HeaderSchema[SocialPostRecord] generic stays type-pinned.
//
// Per-field MUST per RFC §2.4 marketing/social row:
//
//   - `text`         → BODY (REJECT in header — post content is the
//     load-bearing content surface; brand voice + draft language must
//     never be visible while locked).
//   - `platform`     → HEADER (CONFIRM via ValidateString). The current
//     SocialPost model carries a list of channels (Ch); we project the
//     comma-joined list as `platform` to match the RFC's vocabulary
//     (channel ≅ platform for the marketing/social surface — same
//     mapping campaigns uses for its single-channel Channel field).
//   - `scheduled.at` → HEADER (MONTH-only, YYYY-MM). The current
//     SocialPost model carries When as a free-form human-friendly
//     label ("today · 16:00", "yest · 11:14") rather than a parseable
//     Unix timestamp; the schema deliberately omits this header key.
//     SECURITY-NOTE: promoting When to a structured timestamp later
//     requires adding a Unix-seconds field to SocialPostRecord AND
//     wiring a HeaderFor `scheduled.at` entry through MonthBucket.
//     Absence today is the safe default (no-leak); presence with the
//     free-form text would either leak the wrong month or break the
//     MonthBucket validator.
//
// All other fields (state / attach / body) are BODY-only — they're
// either lifecycle metadata (state) or attachment hints (attach) where
// the substrate's default-body discipline is the safe choice.
//
// Usage example:
//
//	rec := social.SocialPostRecord{
//	    ID: "post-1747440000", Ch: "mastodon,x", When: "today · 16:00",
//	    State: "scheduled", Attach: "image",
//	}
type SocialPostRecord struct {
	// ID is the canonical post identifier (matches filename slug).
	ID string `yaml:"id"`

	// Ch is the comma-separated channel list. Projected as the header
	// `platform` key per RFC §2.4 — the BODY copy round-trips for full
	// round-trip integrity but the header projection is the visible-
	// while-locked surface.
	Ch string `yaml:"ch"`

	// When is the human-friendly schedule label ("today · 16:00").
	// BODY-only — free-form text; cannot project safely to the
	// MONTH-only `scheduled.at` header key without a structured
	// timestamp (see SECURITY-NOTE in service.go atrestWriterFor doc).
	When string `yaml:"when"`

	// State is the post state: "scheduled" | "sent" | "draft". BODY-
	// only — RFC §2.4 names no header key for post-state, so the
	// substrate's default-body rule applies.
	State string `yaml:"state"`

	// Attach is the optional attachment label ("image", "video").
	// BODY-only — attachment hints are not in the §2.4 header table.
	Attach string `yaml:"attach"`

	// Text is the post body content — BODY-only by per-field MUST.
	// Carried in yaml frontmatter via `yaml:"-"` exclusion (lives in
	// the BodyText payload, not frontmatter) so the persistence type
	// mirrors the campaigns Body field discipline.
	Text string `yaml:"-"`

	// Version is the monotonic optimistic-lock version (Cascade W2,
	// RFC §B.3 row 5). Stamped by writePost; 0 on legacy files
	// predating the cutover, >=1 after first write through
	// paths.AtomicWriteWithVersion / the at-rest substrate.
	//
	// omitempty keeps legacy round-trips clean (Version=0); the first
	// write stamps version=1.
	Version int `yaml:"version,omitempty"`
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
