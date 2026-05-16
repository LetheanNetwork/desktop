// SPDX-Licence-Identifier: EUPL-1.2

// Package vi populates the Lethean Desktop mascot's Sites slot with
// real uptime probe data — the first real-data wedge of the Vi
// four-slot data contract (ViStatus / Brief / Site / Activity).
//
// Per RFC.vi.md §"Sites()": Vi consumes per-domain uptime checks
// and surfaces them as green/amber/red site cards. v1 of this
// package ships the probe loop + persistence; the Briefs / Activity
// slots remain unimplemented (separate Mantis follow-ups #1351 +
// #1352 in core/ide; mirror tickets to come for desktop).
//
// Surface (Go):
//
//	site := vi.SiteStatus{Domain: "lthn.ai", ...}
//	results := viSvc.Sites()              // []SiteStatus, newest probe first
//	results := viSvc.SitesByDomain("lthn.ai")
//
// Surface (Wails — bound to the JS runtime as "Vi"):
//
//	Vi.Sites()       → SitesOutput { Sites []SiteStatus }
//	Vi.SiteCatalogue → []SiteCatalogue (configured probe targets)
//
// Persistence: SiteProbe records (latest per-domain) live in
// ~/Lethean/data/tasks.duckdb under the `vi_site_probes` table.
//
// Scheduling: the Vi service registers a "vi-probe-site" handler
// against pkg/queue and seeds one Job per catalogued site at
// OnStart; the handler self-reschedules at the per-site interval
// after each probe so the probe cadence survives restarts cleanly.

package vi

import (
	core "dappco.re/go"
	"dappco.re/go/orm"
)

// SiteCatalogue declares one probe target — what to hit and how often.
// Mirrors the user-visible config block; one row per watched domain.
//
// Usage example:
//
//	cat := vi.SiteCatalogue{
//	    URL:      "https://lthn.ai",
//	    Name:     "Lethean",
//	    Stack:    "Lethean · CIC",
//	    Interval: 60 * core.Second,
//	}
type SiteCatalogue struct {
	// URL is the absolute https URL to probe. Must start with
	// "https://" — the probe rejects plain http to align with
	// pkg/marketplace's httpsOnlyClient discipline (Cerberus #1433).
	URL string `json:"url"`

	// Name is the display label. When empty the probe derives one
	// from the URL host.
	Name string `json:"name"`

	// Stack is the curated "Lethean · CIC" / "Forgejo · OCI" string
	// surfaced under the site card. Optional — empty renders as the
	// host name.
	Stack string `json:"stack"`

	// Interval is the per-site probe cadence. Zero defaults to 60s
	// (DefaultInterval).
	Interval core.Duration `json:"interval"`
}

// SiteProbe is one probe result persisted in the orm. Latest row per
// domain backs the Wails Sites() response — older rows linger for
// future sparkline / history work but Sites() only reads the newest.
//
// Lifecycle: the worker probe-handler inserts a fresh row each tick;
// vi.Sites() queries `ORDER BY checked_at DESC` and collapses to one
// row per Domain.
type SiteProbe struct {
	// ID is the primary key — generated at insert time.
	ID string

	// Domain is the canonical site identifier (the URL host without
	// scheme or path). Sites() aggregates by this column.
	Domain string

	// URL is the full probe URL — kept alongside Domain so a future
	// "multiple endpoints per domain" extension lands without a
	// schema migration.
	URL string

	// StatusCode is the HTTP response status (0 on transport error).
	StatusCode int

	// LatencyMs is the round-trip in milliseconds. Recorded on
	// success AND failure (timeout = LatencyMs ≈ timeout ceiling).
	LatencyMs int

	// OK reflects "site is responding healthy" — status in 200..399.
	// 4xx / 5xx / transport-error all leave OK=false.
	OK bool

	// LastError holds the transport error string (empty on success).
	// Truncated to 512 bytes at insert time to keep rows small.
	LastError string

	// CheckedAt is when the probe completed (post-response).
	CheckedAt core.Time
}

// Schema declares the orm shape for SiteProbe. Registered in
// cmd/lthn/app.go alongside the queue + tasks schemas so a fresh
// ~/Lethean/data/tasks.duckdb gets the table on first boot.
//
// Indexes target the hot Sites() query: WHERE domain = ?
// ORDER BY checked_at DESC LIMIT 1, and the rollup query that
// powers the "latest per domain" projection.
func (SiteProbe) Schema() orm.Schema {
	return orm.Define(func(b *orm.Builder) {
		b.Name("vi_site_probes")
		b.PK("id")
		// Only the routing keys are NotNull — same lesson as
		// pkg/queue/types.go: Memium treats zero-values as null, so
		// NotNull on an int column rejects every "0 status" insert
		// (transport error). Zero-times + zero-ints + empty strings
		// stay nullable.
		b.String("id").NotNull()
		b.String("domain").NotNull()
		b.String("url")
		b.Int("status_code")
		b.Int("latency_ms")
		b.Bool("ok")
		b.String("last_error")
		b.Time("checked_at")
		b.Index("domain")
		b.Index("checked_at")
	})
}

// Schemas exposes the orm schemas this package owns. Registered by
// cmd/lthn/app.go alongside opencode + tasks + queue schemas.
//
// Usage example:
//
//	schemas = append(schemas, vi.Schemas()...)
func Schemas() []orm.Schema {
	return []orm.Schema{SiteProbe{}.Schema()}
}

// SiteStatus is the JSON-shaped row the Wails Sites() response
// returns to the Lit window. Derived from the latest SiteProbe per
// domain plus the SiteCatalogue entry that named it.
//
// The shape lines up with the RFC.vi.md "Site" contract — fields the
// Lethean-3 native handoff already renders. Sparkline / LastDeploy
// land as separate follow-ups (sparkline needs a rolling window
// query that v1 doesn't ship; LastDeploy is forge-API work).
type SiteStatus struct {
	// Domain is the canonical site identifier (host only, no scheme).
	Domain string `json:"domain"`

	// Stack is the curated label from SiteCatalogue (e.g.
	// "Lethean · CIC"). Empty when the catalogue entry doesn't set
	// one — the frontend can fall back to the host.
	Stack string `json:"stack"`

	// Status is "green" (OK), "amber" (degraded — last probe failed
	// but a recent one succeeded), or "red" (last probe failed AND
	// no successful probe in the recent window). v1 derives this
	// from the latest probe only ("green" iff probe.OK else "red");
	// "amber" lands when the rolling-window query ships.
	Status string `json:"status"`

	// Response is the latest probe's round-trip in milliseconds.
	Response int `json:"response"`

	// LastChecked is the ISO timestamp of the latest probe.
	LastChecked string `json:"lastChecked"`

	// Err holds the latest probe's transport / HTTP error string
	// when OK is false. Empty on success.
	Err string `json:"error,omitempty"`
}

// SitesOutput is the Wails Sites() response shape — a top-level
// envelope so future fields (overall summary, last-refresh hint)
// can land without breaking the binding signature.
type SitesOutput struct {
	// Sites is the per-domain status list, ordered by the catalogue
	// definition (stable across calls so the UI doesn't reflow).
	Sites []SiteStatus `json:"sites"`

	// Scanned is the number of catalogued sites — useful when the
	// frontend wants to render "watching N sites" without recounting.
	Scanned int `json:"scanned"`
}
