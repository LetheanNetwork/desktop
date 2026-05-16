// SPDX-Licence-Identifier: EUPL-1.2

// Package content is the lthn-side marketing content calendar service.
// Manages editorial content items at ~/Lethean/marketing/content/{id}.md
// — each item belongs to a pipeline column: idea → draft → review → ready
// → live.
//
// Wire shapes match the ContentColumn + ContentItem interfaces consumed by
// the <lthn-view-content> Lit element in the Marketing role view.
//
// Usage example (Wails):
//
//	r := contentSvc.List(content.ListInput{})
//	if r.OK { out := r.Value.(content.ListOutput) }
package content

// ContentItem is the JSON wire type for a single content card.
// Field names match the ContentItem interface in
// frontend/src/lit/views/marketing/content.ts.
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
}

// ContentColumn is the JSON wire type for one pipeline stage column.
// Field names match the ContentColumn interface in
// frontend/src/lit/views/marketing/content.ts.
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
