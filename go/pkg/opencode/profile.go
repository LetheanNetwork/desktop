// SPDX-Licence-Identifier: EUPL-1.2

// Per-task profile substrate — each profile is a partial OpenCode
// Config (the JSON shape from https://opencode.ai/config.json).
// Stored as JSON blobs in the lthn-side go-store under group
// "opencode.profile". On sandbox Start, the named profile is fetched
// + PATCHed onto opencode-serve's /config so the model only loads
// the tools / skills / hooks / provider config needed for the task.
//
// Why narrow per task: every loaded MCP tool, skill, and hook eats
// context window. The model is sharper + cheaper + faster when its
// surface matches the job. We know the job in advance — bake the
// curation into the spawn.

package opencode

import (
	"sync"

	core "dappco.re/go"
	goiostore "dappco.re/go/io/store"
)

// Profile names the canonical default profile + the store group.
const (
	profileStoreGroup = "opencode.profile"
	DefaultProfile    = "default"
)

// Profile is a partial opencode Config — only the fields lthn cares
// about narrowing. Marshalled as JSON and sent to opencode-serve's
// PATCH /config endpoint after spawn.
//
// Fields use omitempty so unset keys aren't sent — opencode-serve's
// PATCH semantics merge non-nil keys + leave nil keys untouched.
//
// Usage example:
//
//	p := opencode.Profile{Model: "anthropic/claude-sonnet-4-5"}
type Profile struct {
	// Name is the lookup key in go-store. Required.
	Name string `json:"name"`

	// Description is human-facing — what task this profile is for.
	Description string `json:"description,omitempty"`

	// Model is the default model in `provider/model` form.
	Model string `json:"model,omitempty"`

	// SmallModel is used for title generation + lightweight tasks.
	SmallModel string `json:"small_model,omitempty"`

	// Provider maps provider-id → provider config. The opencode
	// PATCH /config takes the whole `provider` block; lthn's spawn
	// path always seeds `lthn` here pointing at the local runner.
	Provider map[string]any `json:"provider,omitempty"`

	// Tools enables/disables individual tool ids. Narrowing here
	// is the cheapest context-window saving.
	Tools map[string]bool `json:"tools,omitempty"`

	// DisabledProviders is the explicit deny-list — anything in
	// this list won't be loaded even if the user has credentials.
	DisabledProviders []string `json:"disabled_providers,omitempty"`

	// EnabledProviders is the explicit allow-list — when non-empty,
	// ONLY these providers load. Strongest narrowing.
	EnabledProviders []string `json:"enabled_providers,omitempty"`

	// Permission narrows what the agent can do without asking.
	Permission map[string]any `json:"permission,omitempty"`

	// Agent maps agent-id → agent config — used to wire the
	// `lthn app <name>` pattern (build / plan / review / etc.).
	Agent map[string]any `json:"agent,omitempty"`

	// MCP maps mcp-server-id → mcp config — narrowing the MCP
	// surface to just the servers this task needs.
	MCP map[string]any `json:"mcp,omitempty"`
}

// ToOpenCodeWire serialises the profile to the wire shape opencode
// expects — strips lthn-only metadata fields (Name, Description)
// that aren't part of the upstream Config schema. opencode-serve
// rejects unrecognised keys via ConfigInvalidError, so the strip
// is load-bearing for OPENCODE_CONFIG_CONTENT + PATCH /global/config.
//
// Usage example:
//
//	wire := p.ToOpenCodeWire()
//	env := "OPENCODE_CONFIG_CONTENT=" + wire
func (p Profile) ToOpenCodeWire() string {
	raw := core.JSONMarshalString(p)
	var m map[string]any
	_ = core.JSONUnmarshalString(raw, &m)
	delete(m, "name")
	delete(m, "description")
	return core.JSONMarshalString(m)
}

// DefaultLthnProfile returns the baseline profile seeded at first
// boot — points opencode at the local lthn runner via
// host.docker.internal:8000/v1 so the in-container opencode can
// reach the host-side lthn server (localhost inside the container
// would resolve to the container itself).
//
// Users / tasks layer narrower profiles on top via SaveProfile.
func DefaultLthnProfile() Profile {
	return Profile{
		Name: DefaultProfile,
		Description: "Baseline — local lthn runner; full tools + permissions inside the sandbox " +
			"(the container is the safety boundary, not the permission system).",
		Provider: map[string]any{
			"lthn": map[string]any{
				"npm":  "@ai-sdk/openai-compatible",
				"name": "Lethean Local",
				"options": map[string]any{
					"baseURL": "http://host.docker.internal:8000/v1",
				},
				"models": map[string]any{
					"lthn-local": map[string]any{
						"name": "Lethean Local",
					},
				},
			},
		},
		EnabledProviders: []string{"lthn"},
		// All tools enabled — the sandbox isolates the host from
		// whatever the agent does inside.
		Tools: map[string]bool{
			"bash":     true,
			"edit":     true,
			"webfetch": true,
		},
		// All permissions auto-allow — there's no operator-in-the-loop
		// inside the sandbox; "ask" stalls non-interactive workflows.
		// Tasks that want stricter behaviour ship their own profile.
		Permission: map[string]any{
			"bash":               "allow",
			"edit":               "allow",
			"webfetch":           "allow",
			"doom_loop":          "allow",
			"external_directory": "allow",
		},
	}
}

// profileKVPath is the DuckDB file used for profile storage. Lives
// under the visible ~/Lethean/data/ layout per design_no_hidden_user_bloat.
// Backed by dappco.re/go/io/store (DuckDB-driven KeyValueStore).
const profileKVPath = "Lethean/data/opencode.duckdb"

// kvOnce + kvStore are lazily initialised on first profile access.
// One per Service instance — wrapped in sync.Once so concurrent
// callers don't race the DuckDB file open.
var (
	kvOnce sync.Once
	kvErr  error
	kvInst *goiostore.KeyValueStore
)

// kv lazily opens the DuckDB-backed KV store at ~/Lethean/data/opencode.duckdb.
// Returns the store + a Result wrapping any open error.
func kv() (*goiostore.KeyValueStore, core.Result) {
	kvOnce.Do(func() {
		homeR := core.UserHomeDir()
		if !homeR.OK {
			kvErr = core.E("opencode.kv", "home dir resolve failed", nil)
			return
		}
		path := core.PathJoin(homeR.Value.(string), profileKVPath)
		// Ensure parent dir exists — store.New won't mkdir.
		parent := core.PathDir(path)
		_ = core.MkdirAll(parent, 0o755)
		store, err := goiostore.New(goiostore.Options{Path: path})
		if err != nil {
			kvErr = err
			return
		}
		kvInst = store
	})
	if kvErr != nil {
		return nil, core.Fail(kvErr)
	}
	if kvInst == nil {
		return nil, core.Fail(core.E("opencode.kv", "store not initialised", nil))
	}
	return kvInst, core.Ok(nil)
}

// GetProfile fetches a profile by name. Returns Fail with
// core code "opencode.profile.notfound" when missing.
func (s *Service) GetProfile(name string) core.Result {
	if core.Trim(name) == "" {
		return core.Fail(core.E("opencode.GetProfile", "name is required", nil))
	}
	st, r := kv()
	if !r.OK {
		return r
	}
	raw, err := st.Get(profileStoreGroup, name)
	if err != nil {
		if core.Is(err, goiostore.NotFoundError) {
			return core.Fail(core.NewCode("opencode.profile.notfound", "profile not found: "+name))
		}
		return core.Fail(err)
	}
	var p Profile
	if r := core.JSONUnmarshalString(raw, &p); !r.OK {
		return r
	}
	return core.Ok(p)
}

// SaveProfile persists a profile by name. Idempotent — overwrites
// any existing entry under the same name.
func (s *Service) SaveProfile(p Profile) core.Result {
	if core.Trim(p.Name) == "" {
		return core.Fail(core.E("opencode.SaveProfile", "profile name is required", nil))
	}
	st, r := kv()
	if !r.OK {
		return r
	}
	if err := st.Set(profileStoreGroup, p.Name, core.JSONMarshalString(p)); err != nil {
		return core.Fail(err)
	}
	return core.Ok(nil)
}

// ListProfiles returns all stored profiles.
func (s *Service) ListProfiles() core.Result {
	st, r := kv()
	if !r.OK {
		return r
	}
	all, err := st.GetAll(profileStoreGroup)
	if err != nil {
		return core.Fail(err)
	}
	out := make([]Profile, 0, len(all))
	for _, raw := range all {
		var p Profile
		if r := core.JSONUnmarshalString(raw, &p); r.OK {
			out = append(out, p)
		}
	}
	return core.Ok(out)
}

// DeleteProfile drops a profile by name. Cannot delete the
// "default" profile — it's the safety floor.
func (s *Service) DeleteProfile(name string) core.Result {
	if core.Trim(name) == "" {
		return core.Fail(core.E("opencode.DeleteProfile", "name is required", nil))
	}
	if name == DefaultProfile {
		return core.Fail(core.E("opencode.DeleteProfile", "cannot delete the default profile", nil))
	}
	st, r := kv()
	if !r.OK {
		return r
	}
	if err := st.Delete(profileStoreGroup, name); err != nil {
		return core.Fail(err)
	}
	return core.Ok(nil)
}

// SeedDefaultProfile installs the baseline profile if no "default"
// is stored yet. Called from NewService so a fresh install always
// has a usable spawn target.
func (s *Service) SeedDefaultProfile() core.Result {
	if r := s.GetProfile(DefaultProfile); r.OK {
		return core.Ok(nil)
	}
	return s.SaveProfile(DefaultLthnProfile())
}
