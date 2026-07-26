// SPDX-Licence-Identifier: EUPL-1.2

// Package contacts is the lthn-side CRM contact catalogue service. Reads,
// writes, and queries Trix-style markdown files from
// ~/Lethean/sales/contacts/{slug}.md — YAML frontmatter carries the
// searchable header; the body below is free-form notes.
//
// Wire shape matches the Contact interface consumed by the
// Angular contacts surface in the Sales category.
//
// Usage example (Wails):
//
//	r := contactsSvc.List(contacts.ListInput{Warmth: "hot"})
//	if r.OK { out := r.Value.(contacts.ListOutput) }
package contacts

import core "dappco.re/go"

// Contact is the JSON wire type returned to the Angular frontend.
// Field names match the Contact interface in
// frontend-ng/src/app/desktop/surfaces/sales/contacts.ts exactly.
//
// Usage example:
//
//	c := contacts.Contact{
//	    ID: "ada-penley", Name: "Ada Penley",
//	    Role: "CTO · Heritage Law", Warmth: "hot",
//	    Last: "replied 2 d", Next: "call · Fri",
//	}
type Contact struct {
	// ID is the slug portion of the filename (e.g. "ada-penley").
	ID string `json:"id,omitempty"`

	// Name is the person's display name.
	Name string `json:"name"`

	// Role is already formatted with the org name ("CTO · Heritage Law").
	Role string `json:"role"`

	// Last is a human-readable relative time string ("replied 2 d",
	// "meeting 8 d"). Derived from LastTouch at read time.
	Last string `json:"last"`

	// Warmth is the conversation warmth bucket: "hot" | "warm" | "cool".
	Warmth string `json:"warmth"`

	// Next is the next planned action, free text.
	Next string `json:"next"`
}

// ContactRecord is the internal persistence type — the full set of
// fields stored in the YAML frontmatter of each Trix file. Converted
// to Contact for the Wails wire; the Notes field carries the markdown body.
//
// Usage example:
//
//	rec := contacts.ContactRecord{
//	    ID: "ada-penley", Name: "Ada Penley",
//	    Role: "CTO · Heritage Law", LastTouch: core.Now(),
//	    Next: "call · Fri",
//	}
type ContactRecord struct {
	// Version is the monotonic optimistic-lock version (Cascade W1,
	// Mantis #1540). Stamped into the frontmatter by atomicWriteFile;
	// 0 on legacy files predating the cutover, ≥1 after first write
	// through paths.AtomicWriteWithVersion. The IfVersion check in
	// the primitive uses this exact value.
	Version int `yaml:"version,omitempty"`

	// ID is the canonical contact identifier (matches filename slug).
	ID string `yaml:"id"`

	// Name is the person's display name.
	Name string `yaml:"name"`

	// Role is the formatted role + org string.
	Role string `yaml:"role"`

	// LastTouch is the last interaction timestamp.
	LastTouch core.Time `yaml:"last_touch"`

	// Next is the next planned action.
	Next string `yaml:"next"`

	// Notes is the markdown body below the frontmatter delimiter.
	// Populated on Get(); empty during List() scans for performance.
	Notes string `yaml:"-"`
}

// ContactDetail extends Contact with the full notes body.
// Returned by Get() only — List() omits the body for scan performance.
//
// Usage example:
//
//	r := svc.Get(contacts.GetInput{ID: "ada-penley"})
//	if r.OK { detail := r.Value.(contacts.ContactDetail) }
type ContactDetail struct {
	Contact
	Notes string `json:"notes"`
}

// ListInput drives the List method.
//
// Usage example:
//
//	r := svc.List(contacts.ListInput{Warmth: "hot", Limit: 20})
type ListInput struct {
	// Warmth filters by warmth bucket. Empty string = all.
	Warmth string `json:"warmth,omitempty"`

	// Query is a free-text filter applied to name + role.
	Query string `json:"query,omitempty"`

	// Limit caps the result count. Zero defaults to 100.
	Limit int `json:"limit,omitempty"`
}

// ListOutput is the List response envelope.
//
// Usage example:
//
//	out := r.Value.(contacts.ListOutput)
//	for _, c := range out.Contacts { _ = c.Name }
type ListOutput struct {
	// Contacts is the filtered, sorted list of contacts.
	Contacts []Contact `json:"contacts"`

	// Total is the unfiltered count.
	Total int `json:"total"`
}

// GetInput drives the Get method.
//
// Usage example:
//
//	r := svc.Get(contacts.GetInput{ID: "ada-penley"})
type GetInput struct {
	// ID is the contact slug to retrieve.
	ID string `json:"id"`
}

// CreateInput drives the Create method.
//
// Usage example:
//
//	r := svc.Create(contacts.CreateInput{
//	    Name: "Ada Penley", Role: "CTO · Heritage Law",
//	    Next: "call · Fri",
//	})
type CreateInput struct {
	// Name is the person's display name.
	Name string `json:"name"`

	// Role is the formatted role + org string.
	Role string `json:"role"`

	// LastTouch is the initial last-touch timestamp. Zero defaults to now.
	LastTouch core.Time `json:"lastTouch,omitempty"`

	// Next is the first planned action.
	Next string `json:"next,omitempty"`
}

// UpdateInput drives the Update method.
// All optional fields — only non-zero values are applied.
//
// Usage example:
//
//	r := svc.Update(contacts.UpdateInput{
//	    ID: "ada-penley", Next: "contract",
//	    LastTouch: core.Now(),
//	})
type UpdateInput struct {
	// ID is the contact slug to update.
	ID string `json:"id"`

	// Role updates the role string. Empty = no change.
	Role string `json:"role,omitempty"`

	// LastTouch updates the last-touch timestamp. Zero = no change.
	LastTouch core.Time `json:"lastTouch,omitempty"`

	// Next updates the next planned action. Empty = no change.
	Next string `json:"next,omitempty"`

	// Notes replaces the notes body. Empty = no change.
	Notes string `json:"notes,omitempty"`
}
