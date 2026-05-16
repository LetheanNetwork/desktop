// SPDX-Licence-Identifier: EUPL-1.2

// Package incidents is the lthn-side incident-log service. Reads,
// writes, and queries Trix-style markdown files from
// ~/Lethean/incidents/{YYYY}/{MM}/{id}.md — YAML frontmatter carries
// the searchable header; the body below is the post-mortem markdown.
//
// Wire shape matches the IncidentEntry interface consumed by the
// <lthn-view-incidents> Lit element in the Operations role view.
//
// Usage example (Wails):
//
//	r := incidentsSvc.List(incidents.ListInput{State: "investigating"})
//	if r.OK { out := r.Value.(incidents.ListOutput) }
package incidents

import core "dappco.re/go"

// IncidentEntry is the JSON wire type returned to the Lit frontend.
// Field names match the IncidentEntry interface in
// frontend/src/lit/views/operations/incidents.ts exactly.
//
// Usage example:
//
//	entry := incidents.IncidentEntry{
//	    ID: "2026-05-INC-001", TS: "now", Sev: "P3",
//	    State: "investigating", Title: "hub · elevated p99",
//	    Svc: "hub", Who: "Mei", Comments: 3,
//	}
type IncidentEntry struct {
	// ID is the slug portion of the filename (e.g. 2026-05-INC-001).
	ID string `json:"id"`

	// TS is a human-readable relative time string ("now", "3 d ago",
	// "2 w ago") derived from CreatedAt at read time.
	TS string `json:"ts"`

	// Sev is the severity level: "P1" | "P2" | "P3" | "P4".
	Sev string `json:"sev"`

	// State is the incident lifecycle state:
	// "investigating" | "post-mortem" | "resolved".
	State string `json:"state"`

	// Title is the one-line incident description.
	Title string `json:"title"`

	// Svc is the affected service slug (e.g. "hub", "mail", "forge").
	Svc string `json:"svc"`

	// Who is the responder's name or "all" for all-hands incidents.
	Who string `json:"who"`

	// Comments is the count of discussion comments on the incident.
	Comments int `json:"comments"`

	// Dur is the human-readable resolution duration (e.g. "42 min",
	// "2 h 14"). Omitted when the incident is not yet resolved.
	Dur string `json:"dur,omitempty"`
}

// IncidentRecord is the internal persistence type — the full set of
// fields stored in the YAML frontmatter of each Trix file. Converted
// to IncidentEntry for the Wails wire; the PostMortem field carries
// the markdown body returned by Get().
//
// Usage example:
//
//	rec := incidents.IncidentRecord{
//	    ID: "2026-05-INC-001", Title: "hub · elevated p99",
//	    Sev: "P3", State: "investigating", Svc: "hub", Who: "Mei",
//	    CreatedAt: core.Now(),
//	}
type IncidentRecord struct {
	// ID is the canonical incident identifier (matches filename slug).
	ID string `yaml:"id"`

	// Title is the one-line description.
	Title string `yaml:"title"`

	// Sev is the severity: P1 | P2 | P3 | P4.
	Sev string `yaml:"sev"`

	// State is the lifecycle state: investigating | post-mortem | resolved.
	State string `yaml:"state"`

	// Svc is the affected service slug.
	Svc string `yaml:"svc"`

	// Who is the lead responder.
	Who string `yaml:"who"`

	// Comments is the reply count.
	Comments int `yaml:"comments"`

	// CreatedAt is the incident open timestamp.
	CreatedAt core.Time `yaml:"created_at"`

	// ResolvedAt is when the incident was resolved (zero = still open).
	ResolvedAt core.Time `yaml:"resolved_at"`

	// DurMinutes is the resolution duration in minutes (0 = still open).
	DurMinutes int `yaml:"dur_minutes"`

	// PostMortem is the markdown body below the frontmatter delimiter.
	// Populated on Get(); empty during List() scans for performance.
	PostMortem string `yaml:"-"`

	// Version is the monotonic optimistic-lock version (Cascade W3,
	// RFC §B.3 row 8). Stamped by writeRecord via
	// paths.AtomicWriteWithVersion; 0 on legacy files predating the
	// cutover, >=1 after first write through the primitive.
	// omitempty so legacy files round-trip cleanly as Version=0; the
	// first write stamps version=1.
	//
	// IncidentRecord is the persistence type (yaml-only) — Version
	// stays inside the package; the IncidentEntry wire type consumed
	// by the Lit frontend has no Version field by design.
	Version int `yaml:"version,omitempty"`
}

// IncidentDetail extends IncidentEntry with the full post-mortem body.
// Returned by Get() only — List() omits the body for scan performance.
//
// Usage example:
//
//	r := svc.Get(incidents.GetInput{ID: "2026-05-INC-001"})
//	if r.OK { detail := r.Value.(incidents.IncidentDetail) }
type IncidentDetail struct {
	IncidentEntry
	PostMortem string `json:"postMortem"`
}

// ListInput drives the List method.
//
// Usage example:
//
//	r := svc.List(incidents.ListInput{State: "investigating", Limit: 20})
type ListInput struct {
	// State filters to a specific lifecycle state. Empty string = all.
	State string `json:"state,omitempty"`

	// Severity filters to a specific severity level. Empty string = all.
	Severity string `json:"severity,omitempty"`

	// Limit caps the result count. Zero defaults to 50.
	Limit int `json:"limit,omitempty"`
}

// ListOutput is the List response envelope.
//
// Usage example:
//
//	out := r.Value.(incidents.ListOutput)
//	for _, inc := range out.Incidents { _ = inc.Title }
type ListOutput struct {
	// Incidents is the filtered, sorted list of entries.
	Incidents []IncidentEntry `json:"incidents"`

	// Total is the unfiltered count in the 90-day window.
	Total int `json:"total"`
}

// GetInput drives the Get method.
//
// Usage example:
//
//	r := svc.Get(incidents.GetInput{ID: "2026-05-INC-001"})
type GetInput struct {
	// ID is the incident slug to retrieve.
	ID string `json:"id"`
}

// CreateInput drives the Create method.
//
// Usage example:
//
//	r := svc.Create(incidents.CreateInput{
//	    Title: "hub · elevated p99 latency",
//	    Sev: "P3", Svc: "hub", Who: "Mei",
//	})
type CreateInput struct {
	// Title is the one-line incident description.
	Title string `json:"title"`

	// Sev is the severity: P1 | P2 | P3 | P4.
	Sev string `json:"sev"`

	// Svc is the affected service slug.
	Svc string `json:"svc"`

	// Who is the lead responder's name.
	Who string `json:"who"`
}

// UpdateStateInput drives the UpdateState method.
//
// Usage example:
//
//	r := svc.UpdateState(incidents.UpdateStateInput{
//	    ID: "2026-05-INC-001", State: "resolved",
//	})
type UpdateStateInput struct {
	// ID is the incident slug to transition.
	ID string `json:"id"`

	// State is the target lifecycle state: "post-mortem" | "resolved".
	State string `json:"state"`
}

// PostmortemInput drives the AddPostmortem method.
//
// Usage example:
//
//	r := svc.AddPostmortem(incidents.PostmortemInput{
//	    ID: "2026-05-INC-001", Body: "## Root cause\n\n...",
//	})
type PostmortemInput struct {
	// ID is the incident slug to update.
	ID string `json:"id"`

	// Body is the markdown post-mortem text.
	Body string `json:"body"`
}
