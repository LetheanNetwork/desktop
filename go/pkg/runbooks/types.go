// SPDX-Licence-Identifier: EUPL-1.2

// Package runbooks is the lthn-side runbook library service. Scans,
// indexes, and surfaces markdown runbooks from ~/Lethean/runbooks/*.md
// — Trix-style files with YAML frontmatter carrying the searchable
// header (id, title, area, tags, last_rehearsed) and the procedure
// body below.
//
// The health state ("fresh" | "aging" | "stale") is computed at read
// time from the last_rehearsed date; no separate state is persisted.
//
// Usage example (Wails):
//
//	r := runbooksSvc.List(runbooks.ListInput{})
//	if r.OK { out := r.Value.(runbooks.ListOutput) }
package runbooks

import core "dappco.re/go"

// RunbookEntry is the JSON wire type returned to the Angular frontend.
// Field names match the RunbookEntry interface in
// frontend/src/app/desktop/surfaces/operations/runbooks.ts exactly.
//
// Usage example:
//
//	entry := runbooks.RunbookEntry{
//	    ID: "R-01", Title: "Rotate runtime API keys",
//	    Area: "auth", Last: "2 d", Health: "fresh",
//	}
type RunbookEntry struct {
	// ID is the stable identifier from frontmatter (e.g. "R-01").
	ID string `json:"id"`

	// Title is the runbook headline.
	Title string `json:"title"`

	// Area is the operational domain slug (e.g. "auth", "network").
	Area string `json:"area"`

	// Last is the human-readable time since last rehearsal ("2 d",
	// "3 w", "4 mo", "never").
	Last string `json:"last"`

	// Health is the freshness state: "fresh" | "aging" | "stale".
	Health string `json:"health"`
}

// RunbookDetail extends RunbookEntry with the full markdown body.
// Returned by Get() only — List() and Search() omit the body.
//
// Usage example:
//
//	r := svc.Get(runbooks.GetInput{ID: "R-01"})
//	if r.OK { detail := r.Value.(runbooks.RunbookDetail) }
type RunbookDetail struct {
	RunbookEntry
	Body string `json:"body"`
}

// RunbookRecord is the internal persistence type — the full set of
// fields from the YAML frontmatter of a Trix file.
//
// Usage example:
//
//	rec := runbooks.RunbookRecord{
//	    ID: "R-01", Title: "Rotate runtime API keys",
//	    Area: "auth", Slug: "rotate-runtime-api-keys",
//	    LastRehearsed: core.Now(),
//	}
type RunbookRecord struct {
	// ID is the stable runbook identifier.
	ID string `yaml:"id"`

	// Title is the runbook headline.
	Title string `yaml:"title"`

	// Area is the operational domain slug.
	Area string `yaml:"area"`

	// Tags are freeform labels for search.
	Tags []string `yaml:"tags"`

	// Slug is the filename without .md — derived at parse time,
	// never written to frontmatter.
	Slug string `yaml:"-"`

	// LastRehearsed is the last time the runbook was run through.
	// Zero value = never rehearsed → health is stale.
	LastRehearsed core.Time `yaml:"last_rehearsed"`

	// CreatedAt is when the runbook was first written.
	CreatedAt core.Time `yaml:"created_at"`

	// Body is the markdown procedure below the frontmatter.
	// Populated on Get(); empty during List()/Search() scans.
	Body string `yaml:"-"`

	// Version is the monotonic optimistic-lock version (Cascade W3,
	// RFC §B.3 row 9). Stamped by writeRecord via
	// paths.AtomicWriteWithVersion; 0 on legacy files predating the
	// cutover, >=1 after first write through the primitive.
	// omitempty so legacy files round-trip cleanly as Version=0; the
	// first write stamps version=1.
	//
	// RunbookRecord is the persistence type (yaml-only) — Version
	// stays inside the package; the RunbookEntry wire type consumed
	// by the Angular frontend has no Version field by design.
	Version int `yaml:"version,omitempty"`
}

// ListInput drives the List method.
//
// Usage example:
//
//	r := svc.List(runbooks.ListInput{Health: "stale"})
type ListInput struct {
	// Health filters to a specific freshness state. Empty = all.
	Health string `json:"health,omitempty"`

	// Area filters by area slug. Empty = all.
	Area string `json:"area,omitempty"`

	// Limit caps the result count. Zero = all.
	Limit int `json:"limit,omitempty"`
}

// ListOutput is the List / Search response envelope.
//
// Usage example:
//
//	out := r.Value.(runbooks.ListOutput)
//	_ = out.Stale // stale count across the full library
type ListOutput struct {
	// Books is the filtered list of entries.
	Books []RunbookEntry `json:"books"`

	// Total is the total number of runbooks in the library (unfiltered).
	Total int `json:"total"`

	// Fresh is the count of fresh runbooks across the full library.
	Fresh int `json:"fresh"`

	// Aging is the count of aging runbooks across the full library.
	Aging int `json:"aging"`

	// Stale is the count of stale runbooks across the full library.
	Stale int `json:"stale"`
}

// GetInput drives the Get method.
//
// Usage example:
//
//	r := svc.Get(runbooks.GetInput{ID: "R-01"})
type GetInput struct {
	// ID matches the frontmatter id field (e.g. "R-01").
	ID string `json:"id,omitempty"`

	// Slug matches the filename without .md.
	Slug string `json:"slug,omitempty"`
}

// SearchInput drives the Search method.
//
// Usage example:
//
//	r := svc.Search(runbooks.SearchInput{Query: "postfix"})
type SearchInput struct {
	// Query is the search term — matched against title, area, id, tags.
	Query string `json:"query"`

	// Limit caps the result count. Zero = all matches.
	Limit int `json:"limit,omitempty"`
}

// MarkInput drives the MarkRehearsed method.
//
// Usage example:
//
//	r := svc.MarkRehearsed(runbooks.MarkInput{ID: "R-05"})
type MarkInput struct {
	// ID is the runbook to mark as rehearsed.
	ID string `json:"id"`
}
