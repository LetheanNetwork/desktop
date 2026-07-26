// SPDX-Licence-Identifier: EUPL-1.2

// Package campaigns is the lthn-side marketing campaigns service. Manages
// campaign threads at ~/Lethean/marketing/campaigns/{slug}.lthn — each
// campaign carries name, state, reach, conversion rate, spend, and channel.
//
// Stage E.D.B.3 (Mantis #1487 wave 3): the on-disk format is now the
// encrypted Trix envelope (`.lthn`). Legacy `.md` plaintext records
// remain readable via the lazy-migration fallthrough until a write
// promotes them to `.lthn` per RFC §3.1.
//
// Wire shapes match the Campaign interface consumed by the
// Angular campaigns surface in the Marketing category.
//
// Usage example (Wails):
//
//	r := campaignsSvc.List(campaigns.ListInput{})
//	if r.OK { out := r.Value.(campaigns.ListOutput) }
package campaigns

// Campaign is the JSON wire type for a single campaign row.
// Field names match the Campaign interface in
// frontend-ng/src/app/desktop/surfaces/marketing/campaigns.ts.
//
// Usage example:
//
//	c := campaigns.Campaign{
//	    ID: "v02-launch-20260516", Name: "v0.2 launch · 'sovereign compute'",
//	    State: "live", Reach: "42 K", Convert: "3.2%", Spend: "£0", Channel: "earned",
//	}
type Campaign struct {
	// ID is the campaign slug (filename key).
	ID string `json:"id"`
	// Name is the campaign display name.
	Name string `json:"name"`
	// State is the lifecycle state: "live" | "scheduled" | "draft" | "complete".
	State string `json:"state"`
	// Reach is the pre-formatted audience reach ("42 K", "38", "~180 K", "—").
	Reach string `json:"reach"`
	// Convert is the pre-formatted conversion rate ("3.2%", "21%", "—").
	Convert string `json:"convert"`
	// Spend is the pre-formatted spend with currency symbol ("£0", "£800").
	Spend string `json:"spend"`
	// Channel is the distribution channel: "earned"|"direct"|"email"|"paid".
	Channel string `json:"channel"`
	// Body is the markdown body below the frontmatter (campaign brief / notes).
	Body string `json:"body,omitempty"`

	// Version is the monotonic optimistic-lock version (Cascade W2,
	// RFC §B.3 row 7). Stamped by writeCampaign; 0 on legacy files
	// predating the cutover, >=1 after first write through
	// paths.AtomicWriteWithVersion. Internal-only — `json:"-"` keeps
	// it out of the Wails wire shape consumed by the frontend.
	Version int `json:"-"`
}

// ListInput drives the List method.
//
// Usage example:
//
//	r := svc.List(campaigns.ListInput{State: "live"})
type ListInput struct {
	// State filters to a single state. Empty = all campaigns returned.
	State string `json:"state,omitempty"`
}

// ListOutput is the List response envelope.
//
// Usage example:
//
//	out := r.Value.(campaigns.ListOutput)
//	for _, c := range out.Campaigns { _ = c.Name }
type ListOutput struct {
	// Campaigns is the ordered list of campaigns.
	Campaigns []Campaign `json:"campaigns"`
	// LiveCount is the count of campaigns in "live" state.
	LiveCount int `json:"liveCount"`
	// ScheduledCount is the count of campaigns in "scheduled" state.
	ScheduledCount int `json:"scheduledCount"`
}

// CreateInput drives the Create method.
//
// Usage example:
//
//	r := svc.Create(campaigns.CreateInput{Name: "Product Hunt launch", Channel: "earned"})
type CreateInput struct {
	// Name is the campaign display name. Required.
	Name string `json:"name"`
	// State is the initial state. Defaults to "draft".
	State string `json:"state,omitempty"`
	// Reach is the pre-formatted reach. Defaults to "—".
	Reach string `json:"reach,omitempty"`
	// Convert is the pre-formatted conversion rate. Defaults to "—".
	Convert string `json:"convert,omitempty"`
	// Spend is the pre-formatted spend. Defaults to "£0".
	Spend string `json:"spend,omitempty"`
	// Channel is the distribution channel. Defaults to "earned".
	Channel string `json:"channel,omitempty"`
	// Body is the markdown body (campaign brief / notes).
	Body string `json:"body,omitempty"`
}

// CampaignRecord is the persistence type carried through the at-rest
// substrate as `AtRestWriter[CampaignRecord]`. Stage E.D.B.3 (Mantis
// #1487 wave 3) splits the wire type (Campaign, json-tagged) from the
// persistence type (CampaignRecord, yaml-tagged) so the encrypted body
// frontmatter never leaks json key names and the
// recordfile.HeaderSchema[CampaignRecord] generic stays type-pinned.
//
// Per-field MUST per RFC §2.4 marketing/campaigns row:
//
//   - `name`     → BODY (REJECT in header — PII / campaign branding)
//   - `platform` → HEADER (= Channel field; CONFIRM via ValidateString)
//   - `scheduled.at` → HEADER (MONTH-only, YYYY-MM); the current
//     Campaign type has no scheduled-time field so the schema omits this
//     header key entirely. Adding it later requires an additive field on
//     CampaignRecord plus a HeaderFor entry; absence is correct
//     (no-leak) for v1. See SECURITY-NOTE in service.go atrestWriterFor.
//
// All other fields (state / reach / convert / spend / body) are BODY-
// only — they're either sensitive metrics (reach/convert/spend) or have
// no RFC ruling (state) and the substrate's default-body discipline is
// the safe choice.
//
// Usage example:
//
//	rec := campaigns.CampaignRecord{
//	    ID: "v02-launch-1747440000", Name: "v0.2 launch",
//	    State: "live", Channel: "earned", Body: "## Brief\n\n...",
//	}
type CampaignRecord struct {
	// ID is the canonical campaign identifier (matches filename slug).
	ID string `yaml:"id"`

	// Name is the campaign display name. BODY-only per RFC §2.4 —
	// MUST NEVER appear in the at-rest header.
	Name string `yaml:"name"`

	// State is the lifecycle state: "live" | "scheduled" | "draft" |
	// "complete". BODY-only — RFC §2.4 names no header key for
	// campaign-state, so the substrate's default-body rule applies.
	State string `yaml:"state"`

	// Reach is the pre-formatted audience reach. BODY-only (audience
	// metric is sensitive — visible-while-locked would leak progress).
	Reach string `yaml:"reach"`

	// Convert is the pre-formatted conversion rate. BODY-only.
	Convert string `yaml:"convert"`

	// Spend is the pre-formatted spend with currency symbol. BODY-only
	// (financial metric).
	Spend string `yaml:"spend"`

	// Channel is the distribution channel: "earned"|"direct"|"email"
	// |"paid". Projects into the header `platform` key per RFC §2.4 —
	// channel ≅ platform for the marketing/campaigns surface.
	Channel string `yaml:"channel"`

	// Body is the markdown body below the frontmatter delimiter — the
	// campaign brief / notes. BODY-only.
	Body string `yaml:"-"`

	// Version is the monotonic optimistic-lock version (Cascade W2,
	// RFC §B.3 row 7). Stamped by writeRecord; 0 on legacy files
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
//	r := svc.Update(campaigns.UpdateInput{ID: "v02-launch-20260516", State: "complete"})
type UpdateInput struct {
	// ID is the campaign slug. Required.
	ID string `json:"id"`
	// State, Reach, Convert, Spend, Channel, Body — optional patch fields.
	State   string `json:"state,omitempty"`
	Reach   string `json:"reach,omitempty"`
	Convert string `json:"convert,omitempty"`
	Spend   string `json:"spend,omitempty"`
	Channel string `json:"channel,omitempty"`
	Body    string `json:"body,omitempty"`
}
