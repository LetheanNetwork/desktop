// SPDX-Licence-Identifier: EUPL-1.2

// Imports — projects + provider credentials lthn has data-mined
// from external clients (opencode, codex, claude, pi) so the user
// can keep working without re-authenticating + re-finding their
// projects. The shape is source-agnostic on purpose: opencode is
// the first source wired today, but the same ImportedProject /
// ImportedProvider tables will accept rows from codex / claude /
// pi imports when those land.
//
// Credentials: per Snider's call (don't break the user's setup
// by stripping keys), provider auth tokens are imported into
// lthn-side storage so imported projects keep authenticating
// against their original providers. This is deliberate UX policy
// — the alternative (definitions only) means every imported
// project breaks until the user re-authenticates per provider.

package opencode

import (
	"time"

	"dappco.re/go/orm"
)

// Source enumerates the upstream clients lthn can import from.
// New sources add a constant; the orm types stay stable.
const (
	SourceOpenCodeHost = "opencode-host"
	// Future:
	// SourceClaudeHost = "claude-host"
	// SourceCodexHost  = "codex-host"
	// SourcePiHost     = "pi-host"
)

// ImportedProject is one project record discovered via an upstream
// client's API. Persisted so subsequent lthn sessions see the
// imported inbox without re-running the import.
//
// PK shape: "<source>:<source_id>" — collisions across sources
// can't happen, re-imports overwrite same-source rows in place.
type ImportedProject struct {
	// ID is the primary key — "<source>:<source_id>".
	ID string

	// Source is which upstream client this came from
	// (SourceOpenCodeHost / SourceClaudeHost / ...).
	Source string

	// SourceID is the upstream's own identifier — e.g. opencode's
	// sha1 hash of the worktree path.
	SourceID string

	// Name is the human-facing label, derived from the worktree
	// basename when the upstream doesn't supply one.
	Name string

	// Worktree is the absolute path the project points at on the
	// user's host. Empty when the upstream's project is virtual
	// (e.g. opencode's "global" / "current" pseudo-projects).
	Worktree string

	// VCS is the version-control type — opencode reports "git";
	// future codex / claude imports may report empty.
	VCS string

	// IconColor is the upstream's colour hint when present
	// (opencode emits "purple" / "blue" / etc.). Frontend uses
	// this as a fallback when IconDataURL is empty.
	IconColor string

	// IconDataURL is an optional base64-encoded data URL for the
	// project's favicon. Opencode emits these for projects that
	// have a custom .ico in their worktree.
	IconDataURL string

	// SandboxesJSON is a JSON-encoded []string of related worktrees
	// (opencode tracks "child" sandboxes per project — e.g. eval
	// shards, throwaway clones). Kept opaque so the orm shape
	// doesn't need a join table for what's effectively a tag list.
	SandboxesJSON string

	// UpstreamCreatedAt + UpstreamUpdatedAt are the project's
	// timestamps inside the upstream's own DB (unix-ms in the
	// case of opencode, captured here as time.Time for orm).
	UpstreamCreatedAt time.Time
	UpstreamUpdatedAt time.Time

	// ImportedAt is when lthn captured the row. Re-imports refresh
	// this to "now"; a sync timestamp distinct from the upstream's
	// own time fields.
	ImportedAt time.Time
}

// Schema declares the orm shape for ImportedProject.
//
// Usage example:
//
//	rows := orm.Of[opencode.ImportedProject](c).
//	            Where("source", "=", opencode.SourceOpenCodeHost).
//	            Order("imported_at", "desc").
//	            Get()
func (ImportedProject) Schema() orm.Schema {
	return orm.Define(func(b *orm.Builder) {
		b.Name("imported_projects")
		b.PK("id")
		// Only the keys lthn uses for routing/display are NotNull.
		// Optional upstream fields (vcs, icon, timestamps, sandboxes
		// json) stay nullable — host opencode reports projects with
		// missing fields routinely (the "global" pseudo-project has
		// no vcs, no icon URL, etc.).
		b.String("id").NotNull()
		b.String("source").NotNull()
		b.String("source_id").NotNull()
		b.String("name").NotNull()
		b.String("worktree")
		b.String("vcs")
		b.String("icon_color")
		b.String("icon_data_url")
		b.String("sandboxes_json")
		b.Time("upstream_created_at")
		b.Time("upstream_updated_at")
		b.Time("imported_at").NotNull()
		b.Index("source")
		b.Index("imported_at")
	})
}

// ImportedProvider captures a provider definition AND its
// authentication material (per the "don't break the user's flow"
// policy). The key field is sensitive — storage MUST be on the
// user's local DuckDB, never exfiltrated.
type ImportedProvider struct {
	// ID is the primary key — "<source>:<provider_id>".
	ID string

	// Source is the upstream client (SourceOpenCodeHost / ...).
	Source string

	// ProviderID is the upstream's own provider identifier —
	// "anthropic", "opencode-go", "openai", etc.
	ProviderID string

	// Name is the human-facing label (often same as ProviderID).
	Name string

	// NPM is the npm package id when the provider is an
	// @ai-sdk/openai-compatible plugin (or similar). Empty for
	// providers wired natively.
	NPM string

	// OptionsJSON is a JSON-encoded blob of provider options
	// (baseURL, custom headers, etc.). Opaque to lthn — passed
	// through to whichever client consumes the import later.
	OptionsJSON string

	// AuthType is the credential shape — "apikey", "oauth", etc.
	// Derived from the upstream's auth.json entry shape; empty
	// when the provider isn't authenticated.
	AuthType string

	// AuthKey is the actual credential string. Sensitive — only
	// ever lives in the user's local DuckDB.
	AuthKey string

	// HasAuth is true when AuthKey is non-empty. Useful in
	// queries that don't want to load AuthKey for display.
	HasAuth bool

	// ImportedAt is when lthn captured the row.
	ImportedAt time.Time
}

// Schema declares the orm shape for ImportedProvider.
//
// Usage example:
//
//	rows := orm.Of[opencode.ImportedProvider](c).
//	            Where("has_auth", "=", true).
//	            Get()
func (ImportedProvider) Schema() orm.Schema {
	return orm.Define(func(b *orm.Builder) {
		b.Name("imported_providers")
		b.PK("id")
		// Same rationale as ImportedProject — only routing keys are
		// NotNull. Most providers have no auth (HasAuth=false +
		// AuthKey=""), and many lack an npm package (native bindings).
		b.String("id").NotNull()
		b.String("source").NotNull()
		b.String("provider_id").NotNull()
		b.String("name")
		b.String("npm")
		b.String("options_json")
		b.String("auth_type")
		b.String("auth_key")
		b.Bool("has_auth").NotNull()
		b.Time("imported_at").NotNull()
		b.Index("source")
		b.Index("has_auth")
	})
}
