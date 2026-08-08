// SPDX-Licence-Identifier: EUPL-1.2

// Runner shared type surface — RouteConfig schema (the on-disk
// `routes:` map entry shape) plus migration-state types lifted out of
// routes.go to keep the lifecycle file lean.
//
// Mantis #1657 / RFC.provider-creds-tier1.md v1.1 §3.1 — RouteConfig
// grammars `api_key_ref` (NEW) alongside the legacy `api_key` field
// for the auto-migration window. Loader prefers the ref when both are
// present; legacy plaintext is stripped after the first successful
// import-and-rewrite (pkg/runner/migration.go).

package runner

// RouteConfig is one entry in the `routes:` map persisted under
// ~/Lethean/conf/lthn.yaml. Two credential pathways are grammared
// simultaneously for the duration of the auto-migration window
// (RFC v1.1 §3.1, §6 Option A):
//
//   - APIKey    — DEPRECATED legacy plaintext field. Surfaced ONLY
//     so the boot-time migration scan in pkg/runner/
//     migration.go can lift the plaintext into the tier-1
//     substrate and rewrite the config with the new ref.
//     Post-migration this field is empty on disk.
//   - APIKeyRef — NEW opaque handle into pkg/keys.Service tier-1
//     (e.g. "openai-default"). The runner resolves the
//     ref via keys.GetTier1 at route-build time; the
//     plaintext only lives in the *openai.Backend
//     wrapper's cached field for the route's lifetime.
//
// When both fields are populated the ref WINS — the credential the
// operator most recently dispatched through WSetDynamicRoutes (or the
// auto-migration latch) supersedes any stale plaintext that escaped
// the strip.
//
// Precedent: cmd/lthn/fleet.go L200 already grammars `api_key_ref`
// for the agents fleet schema. Single grammar across the app.
//
// Usage example (in pkg/runner/routes.go):
//
//	for name, rc := range raw {
//	    route := buildRoute(name, rc)
//	    if route == nil { continue }
//	    out = append(out, *route)
//	}
type RouteConfig struct {
	// Kind is the provider adapter — today only "openai" (which
	// covers Ollama, vLLM, llama.cpp's OpenAI-compat endpoint, and
	// real OpenAI). Default "" is treated as "openai" by buildRoute.
	Kind string `mapstructure:"kind" yaml:"kind"`
	// BaseURL is the OpenAI-compatible endpoint base — e.g.
	// "https://api.openai.com/v1" or "http://localhost:11434/v1".
	BaseURL string `mapstructure:"base_url" yaml:"base_url"`
	// APIKey is the DEPRECATED plaintext credential. Retained for
	// the auto-migration window (RFC v1.1 §6); the boot-time scan
	// in pkg/runner/migration.go reads this field, dispatches the
	// bytes into tier-1, then rewrites the config with APIKey empty
	// and APIKeyRef populated. Post-migration this field is "".
	APIKey string `mapstructure:"api_key" yaml:"api_key,omitempty"`
	// APIKeyRef is the NEW opaque handle into pkg/keys.Service
	// tier-1. When set, buildRoute calls keys.GetTier1(APIKeyRef)
	// and pipes the resolved plaintext into the *openai.Backend.
	// When the tier-1 KEK provider is not yet live (account locked
	// at boot) the route is deferred; first successful unlock fires
	// a route-rebuild that consumes the now-resolvable ref.
	APIKeyRef string `mapstructure:"api_key_ref" yaml:"api_key_ref,omitempty"`
	// Model is the upstream model identifier — e.g. "gpt-4o-mini",
	// "llama3:8b". Used as ProviderRoute.ModelID.
	Model string `mapstructure:"model" yaml:"model"`
}

// MigrationStatus is the shape returned from
// Service.WCredentialMigrationStatus — drives the persistent banner
// in the global app-chrome per RFC v1.1 §6 ADD-4. Cerberus #1745:
// the deferral path MUST NOT be silent; a toast on the unlock screen
// is insufficient because the user-trust-vs-system-state divergence
// (operator thinks "I run a secure setup" while plaintext sits at-
// rest on disk) persists silently across boots until next unlock.
//
// The frontend lit-view subscribes via the standard lit-view-backend-
// wire pattern (60s refresh poller, fixture fallback). Banner copy
// renders when PendingMigrationCount > 0; self-dismisses when the
// count drops to zero post-unlock + migration success.
//
// Usage example (TS):
//
//	import { WCredentialMigrationStatus } from "@desktop/runner/service";
//	const s = await WCredentialMigrationStatus();
//	if (s.pending_migration_count > 0) { /* render banner */ }
type MigrationStatus struct {
	// PendingMigrationCount is the number of route entries whose
	// on-disk shape still carries a non-empty APIKey AND an empty
	// APIKeyRef at scan-time — i.e. plaintext credentials waiting
	// for the tier-1 KEK provider to become live so the auto-
	// migration latch can drain them.
	PendingMigrationCount int `json:"pending_migration_count"`
	// PlaintextOnDisk is true whenever PendingMigrationCount > 0 —
	// kept as a discrete bool so the lit-view banner can render off
	// a single flag without re-implementing the count > 0 derivation.
	PlaintextOnDisk bool `json:"plaintext_on_disk"`
	// LockedRoutes lists the route NAMES (not refs) whose migration
	// is currently deferred because the tier-1 KEK provider is not
	// live. Empty when the count is zero. Frontend may use this to
	// surface "credentials for routes openai, anthropic are waiting
	// to be migrated" in the banner detail.
	LockedRoutes []string `json:"locked_routes,omitempty"`
}

// ForceDeleteAckToken is the literal operator-confirmation token
// required by Service.WForceDeleteTier1 — the only string the server
// accepts when bypassing the referencing-route check. Exported as a
// const so the frontend can import the single canonical literal
// rather than typing it as a magic string in two places (RFC v1.1 §5
// ADD-3 — Cerberus #1744 force-delete escape).
//
// Usage example (frontend confirm dialog):
//
//	import { FORCE_DELETE_ACK_TOKEN } from "@desktop/runner/service";
//	if (input.value === FORCE_DELETE_ACK_TOKEN) { /* proceed */ }
const ForceDeleteAckToken = "operator_acknowledged_orphan"
