// SPDX-Licence-Identifier: EUPL-1.2

// Package content is the lthn-side marketing content calendar service.
// Manages editorial content items at ~/Lethean/marketing/content/{id}.lthn
// — each item belongs to a pipeline column: idea → draft → review → ready
// → live.
//
// Stage E.D.B.3 (Mantis #1487 wave 3 — LAST consumer): the on-disk
// format is now the encrypted Trix envelope (`.lthn`). Legacy `.md`
// plaintext records remain readable via the lazy-migration fallthrough
// until a write promotes them to `.lthn` per RFC §3.1.
//
// Wire shapes match the ContentColumn + ContentItem interfaces consumed by
// the Angular content surface in the Marketing category.
//
// Usage example (Wails):
//
//	r := contentSvc.List(content.ListInput{})
//	if r.OK { out := r.Value.(content.ListOutput) }
package content

// ContentItem is the JSON wire type for a single content card.
// Field names match the ContentItem interface in
// frontend/src/app/desktop/surfaces/marketing/content.ts.
//
// Usage example:
//
//	item := content.ContentItem{
//	    ID: "v02-release-notes-20260516", T: "v0.2 release notes",
//	    Who: "you", Due: "today", Col: "draft",
//	}
type ContentItem struct {
	// ID is the item slug (filename key).
	ID string `json:"id"`
	// T is the content title / headline.
	T string `json:"t"`
	// Who is the author or reviewer attribution. Optional.
	Who string `json:"who,omitempty"`
	// When is the publish timestamp for live items ("3 d ago", "1 w ago"). Optional.
	When string `json:"when,omitempty"`
	// Due is the due-date label for in-flight items ("today"). Optional.
	Due string `json:"due,omitempty"`
	// Col is the pipeline column: "idea"|"draft"|"review"|"ready"|"live".
	Col string `json:"col"`
	// Body is the markdown body (content outline / notes).
	Body string `json:"body,omitempty"`

	// Version is the monotonic optimistic-lock version (Cascade W2,
	// RFC §B.3 row 4). Stamped by writeItem; 0 on legacy files
	// predating the cutover, >=1 after first write through
	// paths.AtomicWriteWithVersion. Internal-only — `json:"-"` keeps
	// it out of the Wails wire shape consumed by the frontend.
	Version int `json:"-"`
}

// ContentColumn is the JSON wire type for one pipeline stage column.
// Field names match the ContentColumn interface in
// frontend/src/app/desktop/surfaces/marketing/content.ts.
//
// Usage example:
//
//	col := content.ContentColumn{
//	    ID: "draft", Label: "Drafting", Items: []content.ContentItem{},
//	}
type ContentColumn struct {
	// ID is the stable column identifier.
	ID string `json:"id"`
	// Label is the human label rendered in the column header.
	Label string `json:"label"`
	// Items is the ordered list of content cards in this column.
	Items []ContentItem `json:"items"`
}

// ListInput drives the List method.
//
// Usage example:
//
//	r := svc.List(content.ListInput{Col: "draft"})
type ListInput struct {
	// Col filters to a single pipeline column. Empty = all columns returned.
	Col string `json:"col,omitempty"`
}

// ListOutput is the List response envelope.
//
// Usage example:
//
//	out := r.Value.(content.ListOutput)
//	for _, col := range out.Columns { _ = col.Label }
type ListOutput struct {
	// Columns is the ordered list of pipeline columns.
	Columns []ContentColumn `json:"columns"`
	// TotalInFlight is the count of items NOT in "live" state.
	TotalInFlight int `json:"totalInFlight"`
	// DueToday is the count of items with Due == "today".
	DueToday int `json:"dueToday"`
}

// CreateInput drives the Create method.
//
// Usage example:
//
//	r := svc.Create(content.CreateInput{T: "v0.2 release notes", Who: "you", Due: "today"})
type CreateInput struct {
	// T is the content title. Required.
	T string `json:"t"`
	// Col is the initial column. Defaults to "idea".
	Col string `json:"col,omitempty"`
	// Who is the author attribution. Optional.
	Who string `json:"who,omitempty"`
	// Due is the due-date label. Optional.
	Due string `json:"due,omitempty"`
	// Body is the markdown body. Optional.
	Body string `json:"body,omitempty"`
}

// UpdateInput drives the Update method. All fields except ID are optional patches.
//
// Usage example:
//
//	r := svc.Update(content.UpdateInput{ID: "v02-release-notes-20260516", Col: "review"})
type UpdateInput struct {
	// ID is the item slug. Required.
	ID string `json:"id"`
	// Col, Who, Due, When, Body — optional patch fields.
	Col  string `json:"col,omitempty"`
	Who  string `json:"who,omitempty"`
	Due  string `json:"due,omitempty"`
	When string `json:"when,omitempty"`
	Body string `json:"body,omitempty"`
}

// ContentRecord is the persistence type carried through the at-rest
// substrate as `AtRestWriter[ContentRecord]`. Stage E.D.B.3 (Mantis
// #1487 wave 3, LAST consumer) splits the wire type (ContentItem,
// json-tagged) from the persistence type (ContentRecord, yaml-tagged)
// so the encrypted body frontmatter never leaks json key names and the
// recordfile.HeaderSchema[ContentRecord] generic stays type-pinned.
//
// Per-field MUST per RFC §2.4 marketing/content row:
//
//   - `title`  → BODY (REJECT in header — PII / editorial intent).
//     The wire type calls this field T; ContentRecord renames to Title
//     for at-rest persistence so the YAML key matches RFC vocabulary.
//   - `due.at` → HEADER (MONTH-only, YYYY-MM). The wire/persisted Due
//     field is human-shaped today ("today", "next Friday"); the
//     HeaderFor projection emits `due.at` only when Due parses as
//     strict YYYY-MM (Q5-friendly per sales/deals close.target
//     pattern). Legacy free-form values stay BODY-only by being absent
//     from the header.
//
// All other fields (who / when / col / body) are BODY-only — either
// PII-adjacent (who is the author / reviewer attribution; when leaks
// publication cadence; col exposes pipeline state) or have no RFC
// ruling (col / body) and the substrate's default-body discipline
// applies. SECURITY-NOTE: brief offers `status (enum if any)` escape
// valve; RFC §2.4 does NOT name a status header key for marketing/
// content, so col stays BODY-only per [[feedback_brief_vs_rfc_
// deferral_check]] + [[feedback_beta_ticket_dont_gate]]. Absence is
// the safe no-leak default.
//
// Usage example:
//
//	rec := content.ContentRecord{
//	    ID: "v02-release-notes-1747440000", Title: "v0.2 release notes",
//	    Who: "you", Due: "2026-05", Col: "draft", Body: "## Outline\n\n...",
//	}
type ContentRecord struct {
	// ID is the canonical item identifier (matches filename slug).
	ID string `yaml:"id"`

	// Title is the content title / headline. BODY-only per RFC §2.4 —
	// MUST NEVER appear in the at-rest header. Maps to/from
	// ContentItem.T at the wire boundary.
	Title string `yaml:"title"`

	// Who is the author or reviewer attribution. BODY-only (PII-
	// adjacent; visible-while-locked would expose authorship pattern).
	Who string `yaml:"who"`

	// When is the publish timestamp label for live items. BODY-only
	// (publication cadence is intent-revealing).
	When string `yaml:"when"`

	// Due is the due-date label. Persisted in the body verbatim; the
	// HeaderFor projection emits a `due.at` HEADER key only when Due
	// parses as strict YYYY-MM (Q5-friendly).
	Due string `yaml:"due"`

	// Col is the pipeline column: "idea"|"draft"|"review"|"ready"|
	// "live". BODY-only — RFC §2.4 names no header key for content
	// pipeline state, so the substrate's default-body rule applies.
	Col string `yaml:"col"`

	// Body is the markdown body below the frontmatter delimiter — the
	// content outline / notes. BODY-only.
	Body string `yaml:"-"`

	// Version is the monotonic optimistic-lock version (Cascade W2,
	// RFC §B.3 row 4). Stamped by writeItem; 0 on legacy files
	// predating the cutover, >=1 after first write through
	// paths.AtomicWriteWithVersion / the at-rest substrate.
	//
	// omitempty keeps legacy round-trips clean (Version=0); the first
	// write stamps version=1.
	Version int `yaml:"version,omitempty"`
}
