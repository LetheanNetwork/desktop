// SPDX-Licence-Identifier: EUPL-1.2

// PR Activity — Vi.Activity slot populated by watching open pull
// requests across Forgejo (forge.lthn.sh + siblings) and GitHub
// repos. Mirrors the Sites pattern: a queue-driven fetcher persists
// each tick as PRActivity rows; LatestPerPR rolls up to one entry
// per (provider, owner, repo, pr_number) for the Wails Activity()
// response.
//
// Per RFC.vi.md §"Activity()": the Activity slot is a chronological
// feed of operator + Vi observations. Open PRs are the first wedge —
// every PR a watched repo carries is a thing a human (Vi) cares
// about right now ("five PRs open against lthn/desktop, oldest is
// 4d old"). Forgejo + GitHub backends share one persistence shape
// so the surface stays uniform regardless of where the PR lives.
//
// Surface (Go):
//
//	prs := viSvc.Activity()              // []ActivityEntry, newest first
//
// Surface (Wails — bound to "Vi"):
//
//	Vi.Activity()    → ActivityOutput { Items []ActivityEntry, Scanned int }
//	Vi.Repos()       → []PRRepo (configured watched repos)
//
// Persistence: PRActivity rows in ~/Lethean/data/tasks.duckdb under
// the `vi_pr_activity` table. One row per (repo, pr_number) per tick
// — LatestPerPR collapses to the newest snapshot per PR.

package vi

import (
	core "dappco.re/go"
	"dappco.re/go/orm"
)

// PRRepo declares one watched repository — provider + owner + name.
// Mirrors the user-visible config block; one row per watched repo.
//
// Usage example:
//
//	r := vi.PRRepo{
//	    Provider: "forge",
//	    Owner:    "lthn",
//	    Name:     "desktop",
//	    Interval: 300 * core.Second,
//	}
type PRRepo struct {
	// Provider is the backend name — "forge" (Forgejo) or "github".
	// Other Forgejo instances reuse the "forge" provider with a
	// different BaseURL; "github" pins api.github.com.
	Provider string `json:"provider"`

	// BaseURL overrides the provider default — used for Forgejo
	// instances other than forge.lthn.sh (e.g. forge.lthn.ai,
	// codeberg.org). Empty inherits the provider default
	// (https://forge.lthn.sh for forge, https://api.github.com for
	// github).
	BaseURL string `json:"baseUrl,omitempty"`

	// Owner is the org / user that owns the repo.
	Owner string `json:"owner"`

	// Name is the repository name (no .git suffix).
	Name string `json:"name"`

	// Interval is the per-repo poll cadence. Zero defaults to
	// DefaultPRInterval (300s). PR state changes are less frequent
	// than uptime probes; the slower cadence keeps us under
	// GitHub's anonymous + token rate limits comfortably.
	Interval core.Duration `json:"interval"`
}

// PRActivity is one persisted snapshot of an open pull request.
// One row inserted per (provider, owner, repo, pr_number) per tick;
// LatestPerPR collapses to the newest per PR for the Wails surface.
//
// Lifecycle: the worker poll-handler fetches the open-PR list per
// repo and inserts one row per PR. Closed PRs simply stop appearing
// in upstream responses — their old rows linger in the table but
// LatestPerPR's "open-only" filter drops them from the rollup.
type PRActivity struct {
	// ID is the primary key — generated at insert time.
	ID string

	// Provider is the backend that produced this row — "forge" or
	// "github". Lets Activity() render a per-provider icon / label.
	Provider string

	// Owner is the repo owner (org / user).
	Owner string

	// Repo is the repository name.
	Repo string

	// PRNumber is the upstream PR number (per-repo, not globally
	// unique).
	PRNumber int

	// Title is the PR title at this snapshot.
	Title string

	// Author is the PR author's login.
	Author string

	// State is the upstream state — "open" today; the table will
	// retain "closed" / "merged" rows once the poll covers state
	// transitions (v1 only fetches open PRs).
	State string

	// OpenedAt is when the PR was created upstream.
	OpenedAt core.Time

	// UpdatedAt is the last upstream update timestamp.
	UpdatedAt core.Time

	// URL is the upstream PR URL — what Activity() links to.
	URL string

	// CheckedAt is when this snapshot was fetched.
	CheckedAt core.Time
}

// PRSchema declares the orm shape for PRActivity. Registered in
// cmd/lthn/app.go alongside the SiteProbe schema so a fresh
// ~/Lethean/data/tasks.duckdb gets the table on first boot.
//
// Indexes target the hot Activity() query: rollup latest per
// (provider, owner, repo, pr_number) ordered by checked_at desc.
func (PRActivity) Schema() orm.Schema {
	return orm.Define(func(b *orm.Builder) {
		b.Name("vi_pr_activity")
		b.PK("id")
		// Only the routing keys are NotNull — same reasoning as
		// SiteProbe: Memium treats zero-values as null, so NotNull
		// on PRNumber would reject any PR that happens to be #0.
		b.String("id").NotNull()
		b.String("provider").NotNull()
		b.String("owner").NotNull()
		b.String("repo").NotNull()
		b.Int("pr_number")
		b.String("title")
		b.String("author")
		b.String("state")
		b.Time("opened_at")
		b.Time("updated_at")
		b.String("url")
		b.Time("checked_at")
		b.Index("provider")
		b.Index("owner")
		b.Index("repo")
		b.Index("checked_at")
	})
}

// ActivityEntry is the JSON-shaped row the Wails Activity() response
// returns to the Lit window. Derived from the latest PRActivity per
// (provider, owner, repo, pr_number).
//
// The shape lines up with the RFC.vi.md "ActivityItem" contract —
// fields the Lethean-3 native handoff already renders. v1 ships PR
// rows only; later sources (git commits, Mantis closures) plug into
// the same surface as additional Kind values.
type ActivityEntry struct {
	// Kind is the event class — "pr" for v1. Future kinds (commit,
	// mantis, deploy, …) reuse this field so the frontend can fan
	// out by kind without bespoke surfaces.
	Kind string `json:"kind"`

	// Provider is "forge" or "github".
	Provider string `json:"provider"`

	// Repo is the human-friendly "<owner>/<name>" identifier — what
	// the UI shows next to the PR title.
	Repo string `json:"repo"`

	// Number is the PR number.
	Number int `json:"number"`

	// Title is the PR title at last snapshot.
	Title string `json:"title"`

	// Author is the PR author's login.
	Author string `json:"author"`

	// State is the upstream state — "open" today.
	State string `json:"state"`

	// URL is the upstream PR URL — Activity panel renders this as a
	// clickable link.
	URL string `json:"url"`

	// OpenedAt is the ISO timestamp when the PR was created.
	OpenedAt string `json:"openedAt"`

	// UpdatedAt is the ISO timestamp of the last upstream update.
	UpdatedAt string `json:"updatedAt"`

	// CheckedAt is the ISO timestamp of the snapshot fetch.
	CheckedAt string `json:"checkedAt"`
}

// ActivityOutput is the Wails Activity() response shape — a top-level
// envelope so future fields (per-kind totals, last-refresh hint) can
// land without breaking the binding signature.
type ActivityOutput struct {
	// Items is the chronological list of activity entries, ordered
	// by UpdatedAt descending (most-recently-touched first).
	Items []ActivityEntry `json:"items"`

	// Scanned is the count of watched repos — useful when the
	// frontend wants to render "watching N repos" without
	// recounting.
	Scanned int `json:"scanned"`
}
