// SPDX-Licence-Identifier: EUPL-1.2

// Package documents is the lthn-side Office document catalogue service.
// Reads and indexes local markdown files from ~/Lethean/office/docs/.
// YAML frontmatter carries the searchable header (state, author); the
// body is the document text — Trix-style split.
//
// Wire shape matches the DocRow interface consumed by the
// Angular documents surface in the Office category.
//
// Usage example (Wails):
//
//	r := docsSvc.List(documents.ListInput{State: "draft"})
//	if r.OK { out := r.Value.(documents.ListOutput) }
package documents

import core "dappco.re/go"

// DocRow is the JSON wire type returned to the Angular frontend.
// Field names match the DocRow interface in
// frontend-ng/src/app/desktop/surfaces/office/documents.ts exactly.
//
// Usage example:
//
//	row := documents.DocRow{
//	    Title: "Release notes", State: "draft",
//	    Author: "you", Edited: "now", Size: "4.2 KB",
//	}
type DocRow struct {
	// Title is the first H1 heading in the markdown body; falls back
	// to the filename stem when no H1 is found.
	Title string `json:"title"`

	// State is the document lifecycle state:
	// "draft" | "ready" | "final" | "live".
	// Defaults to "draft" when frontmatter is absent.
	State string `json:"state"`

	// Author is the display name of the document author. Resolves to
	// "you" when the frontmatter author field matches the current OS
	// user; otherwise passes the field value through verbatim.
	Author string `json:"author"`

	// Edited is a human-readable last-edit timestamp:
	// "now", "yest", "3 d ago", "2 w ago".
	Edited string `json:"edited"`

	// Size is a human-readable file size: "4.2 KB", "248 KB", "1.2 MB".
	Size string `json:"size"`
}

// DocDetail extends DocRow with the full markdown body. Returned by
// Get() only — List() omits the body for scan performance.
//
// Usage example:
//
//	r := svc.Get(documents.GetInput{Slug: "release-notes"})
//	if r.OK { d := r.Value.(documents.DocDetail); _ = d.Body }
type DocDetail struct {
	DocRow
	// Body is the full markdown text of the document.
	Body string `json:"body"`
}

// DocRecord is the internal persistence type — the full set of fields
// derived from the file's frontmatter and OS stat. Converted to DocRow
// for the Wails wire.
//
// Usage example:
//
//	rec := documents.DocRecord{
//	    Slug: "release-notes", Title: "Release notes",
//	    State: "draft", Author: "Snider",
//	    ModTime: core.Now(), SizeB: 4300,
//	}
type DocRecord struct {
	// Slug is the filename stem (no extension), used as the document ID.
	Slug string

	// Title extracted from frontmatter or first H1; empty when neither
	// is present (caller falls back to Slug).
	Title string

	// State is the lifecycle state: draft | ready | final | live.
	State string

	// Author is the raw frontmatter author field (pre-resolution).
	Author string

	// ModTime is the file modification time from the OS stat.
	ModTime core.Time

	// SizeB is the raw file size in bytes.
	SizeB int64
}

// ListInput drives the List method.
//
// Usage example:
//
//	r := svc.List(documents.ListInput{State: "draft", Limit: 20})
type ListInput struct {
	// State filters to a specific lifecycle state. Empty string = all.
	State string `json:"state,omitempty"`

	// Limit caps the result count. Zero defaults to 50.
	Limit int `json:"limit,omitempty"`
}

// ListOutput is the List response envelope.
//
// Usage example:
//
//	out := r.Value.(documents.ListOutput)
//	for _, d := range out.Docs { _ = d.Title }
type ListOutput struct {
	// Docs is the filtered, sorted list of document rows.
	Docs []DocRow `json:"docs"`

	// Total is the unfiltered count of all documents in the directory.
	Total int `json:"total"`
}

// GetInput drives the Get method.
//
// Usage example:
//
//	r := svc.Get(documents.GetInput{Slug: "release-notes"})
type GetInput struct {
	// Slug is the filename stem of the document to retrieve.
	Slug string `json:"slug"`
}

// MaxBodyBytes is the maximum permitted document body size (1 MB). Enforced
// server-side before any disk touch. Defence against DoS (Q2 ruling).
const MaxBodyBytes = 1 << 20

// CreateInput drives the Create method.
//
// Usage example:
//
//	r := svc.Create(documents.CreateInput{
//	    Slug: "release-notes", Title: "Release Notes",
//	    Body: "# Release Notes\n\nContent.", State: "draft",
//	})
type CreateInput struct {
	// Slug is the filename stem; MUST pass paths.IsValidID.
	Slug string `json:"slug"`

	// Title is the first H1 line. Empty → falls back to Slug.
	Title string `json:"title"`

	// Body is the markdown body. MUST be ≤ MaxBodyBytes.
	Body string `json:"body"`

	// State is the lifecycle state. Defaults to "draft".
	State string `json:"state"`

	// Tags are free-form labels; lowercase-normalised at write.
	Tags []string `json:"tags,omitempty"`

	// Related lists sibling document slugs. Existence is NOT validated (Q5).
	Related []string `json:"related,omitempty"`
}

// SaveInput drives the Save method. The IfMatch + IfMatchHash composite
// token is REQUIRED — blind writes are rejected (Q1 ruling).
//
// Usage example:
//
//	r := svc.Save(documents.SaveInput{
//	    Slug: "release-notes", Body: "# Updated\n\nContent.",
//	    IfMatch: lastMtime, IfMatchHash: lastBodyHash,
//	})
type SaveInput struct {
	// Slug identifies the document to update.
	Slug string `json:"slug"`

	// Body is the new markdown body. MUST be ≤ MaxBodyBytes.
	Body string `json:"body"`

	// State updates the lifecycle state. Empty → preserve existing.
	State string `json:"state,omitempty"`

	// Tags replaces the tag list. Nil → preserve existing.
	Tags []string `json:"tags,omitempty"`

	// Related replaces the related-doc list. Nil → preserve existing.
	Related []string `json:"related,omitempty"`

	// IfMatch is the mtime from the last Get (half of composite lock token).
	IfMatch core.Time `json:"if_match"`

	// IfMatchHash is the hex(sha256(body bytes on disk)) from the last Get.
	IfMatchHash string `json:"if_match_hash"`
}

// DeleteInput drives the Delete method. Requires the composite token from a
// recent Get to prevent accidental deletion of a stale view (Q4 ruling).
//
// Usage example:
//
//	r := svc.Delete(documents.DeleteInput{
//	    Slug: "old-draft", IfMatch: lastMtime, IfMatchHash: lastBodyHash,
//	})
type DeleteInput struct {
	// Slug identifies the document to remove.
	Slug string `json:"slug"`

	// IfMatch is the mtime from the last Get.
	IfMatch core.Time `json:"if_match"`

	// IfMatchHash is the hex(sha256(body bytes on disk)) from the last Get.
	IfMatchHash string `json:"if_match_hash"`
}
