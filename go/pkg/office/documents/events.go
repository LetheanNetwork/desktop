// SPDX-Licence-Identifier: EUPL-1.2

// Event name constants and typed event shapes for the documents surface.
// v2 promotes EventCreated/EventUpdated from reserved to live, and adds
// EventDeleted + the DocChanged typed payload + the Subscribe helper.

package documents

import core "dappco.re/go"

// EventCreated fires when a new document is created via Create.
// Payload: DocChanged{Kind: "created"}.
const EventCreated = "documents.created"

// EventUpdated fires when an existing document is saved via Save.
// Payload: DocChanged{Kind: "updated"}.
const EventUpdated = "documents.updated"

// EventDeleted fires when a document is hard-deleted via Delete.
// Payload: DocChanged{Kind: "deleted"}.
const EventDeleted = "documents.deleted"

// DocChanged is the typed payload broadcast for all document mutation events.
// Kind is "created" | "updated" | "deleted".
// Before is zero-value when Kind == "created".
// After is zero-value when Kind == "deleted".
//
// Usage example:
//
//	documents.Subscribe(c, func(e documents.DocChanged) {
//	    core.Print("doc changed", "kind", e.Kind, "slug", e.Slug)
//	})
type DocChanged struct {
	// Kind is one of "created", "updated", "deleted".
	Kind string `json:"kind"`

	// Slug is the document identifier.
	Slug string `json:"slug"`

	// Before is the pre-mutation record. Zero-value when Kind=="created".
	Before DocRecord `json:"before"`

	// After is the post-mutation record. Zero-value when Kind=="deleted".
	After DocRecord `json:"after"`

	// At is the wall-clock time of the mutation.
	At core.Time `json:"at"`
}

// Subscribe registers fn to receive DocChanged events on c.
// All three event kinds (created / updated / deleted) route to fn.
// Mirrors the tasks.Subscribe shape so audit-log subscribers wire in
// without per-package glue.
//
// Usage example:
//
//	documents.Subscribe(c, func(e documents.DocChanged) {
//	    // audit-log e.Kind + e.Slug
//	})
func Subscribe(c *core.Core, fn func(DocChanged)) {
	c.RegisterAction(func(_ *core.Core, msg core.Message) core.Result {
		if ch, ok := msg.(DocChanged); ok {
			fn(ch)
		}
		return core.Ok(nil)
	})
}
