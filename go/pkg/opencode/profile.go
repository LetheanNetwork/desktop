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
	core "dappco.re/go"
	goiostore "dappco.re/go/io/store"
)

// Profile names the canonical default profile + the store group.
const (
	profileStoreGroup = "opencode.profile"
	DefaultProfile    = "default"
)

// Profile-schema validation error codes. Returned by SaveProfile when
// a caller-supplied Profile blob carries keys / values outside the
// closed schema. Mantis #1603 HIGH (Cerberus #22) — the unvalidated
// opaque-map pass-through let any LocalKey-bearer caller install an
// adversarial MCP command (executed inside the sandbox on every
// query), swap providers to attacker URLs, or rewrite the agent
// system-prompt by overwriting the "default" profile.
//
// The codes are reserved schema — renaming a literal without a spec
// bump breaks the audit log-tailer's facet chrome.
//
// Usage example:
//
//	r := svc.SaveProfile(p)
//	if r.Code() == ProfileInvalidSchema { /* surface user-facing */ }
const (
	ProfileInvalidSchema = "opencode.profile.invalid_schema"
	ProfileDefaultGuard  = "opencode.profile.default_guard"
)

// profileAllowedProviderKeys is the closed set of provider IDs the
// control surface accepts. Sized to current opencode-serve provider
// catalogue — extend deliberately, not opportunistically.
var profileAllowedProviderKeys = map[string]bool{
	"lthn":           true,
	"openai":         true,
	"anthropic":      true,
	"ollama":         true,
	"openrouter":     true,
	"google":         true,
	"groq":           true,
	"mistral":        true,
	"deepseek":       true,
	"xai":            true,
	"github-copilot": true,
}

// profileAllowedProviderSubKeys is the closed set of per-provider
// keys. Mirrors opencode-serve's provider config shape; extend only
// when opencode-serve adds a documented key.
var profileAllowedProviderSubKeys = map[string]bool{
	"npm":     true,
	"name":    true,
	"options": true,
	"models":  true,
}

// profileAllowedProviderOptionsKeys is the closed set of nested
// `options` keys. baseURL is shape-validated separately as a URL.
var profileAllowedProviderOptionsKeys = map[string]bool{
	"baseURL": true,
	"apiKey":  true,
	"headers": true,
}

// profileAllowedMCPKeys is the closed set of MCP server keys. The
// substrate accepts EITHER a `command + args` shape (local stdio MCP)
// OR a `url` shape (HTTP MCP) — never both for the same record.
// Command + args carry the heaviest review (arbitrary execution inside
// the sandbox); url variants are URL-shape-validated.
var profileAllowedMCPKeys = map[string]bool{
	"type":    true,
	"command": true,
	"args":    true,
	"url":     true,
	"enabled": true,
	"env":     true,
}

// profileAllowedAgentKeys is the closed set of agent keys.
var profileAllowedAgentKeys = map[string]bool{
	"description":   true,
	"mode":          true,
	"model":         true,
	"temperature":   true,
	"tools":         true,
	"permission":    true,
	"system_prompt": true,
	"prompt":        true,
}

// profileAllowedPermissionVerbs is the closed set of permission
// surface verbs. Mirrors opencode-serve's permission-grant shape +
// the DefaultLthnProfile permission keys.
var profileAllowedPermissionVerbs = map[string]bool{
	"bash":               true,
	"edit":               true,
	"webfetch":           true,
	"doom_loop":          true,
	"external_directory": true,
}

// profileAllowedPermissionValues is the closed set of permission
// dispositions opencode-serve recognises. Anything else is silently
// reinterpreted by opencode-serve as "ask" — explicit-reject prevents
// the silent-downgrade smuggling vector.
var profileAllowedPermissionValues = map[string]bool{
	"allow": true,
	"ask":   true,
	"deny":  true,
}

// profileShellMetacharacters are bytes that, if present in an MCP
// `command` string or args value, indicate a shell-injection attempt.
// opencode-serve's MCP runtime spawns command directly (no shell), but
// callers that interpolate the value into a shell elsewhere would be
// burned. Reject at the substrate boundary.
const profileShellMetacharacters = ";&|`$<>(){}*?!\"'\\\n\r\t"

// profileMaxStringLen caps any single string value in the profile
// blob. Defends against the "1MB system_prompt smuggled into the
// audit log + the spawn env var" amplification.
const profileMaxStringLen = 8192

// profileDefaultGuardedFields names the keys that, when present on a
// mutation of the "default" profile, surface a Meta warning to the
// caller — they change spawn behaviour for EVERY future spawn, not
// just an explicitly-named one. Per the brief: this is a surfacing,
// not a hard reject; the caller may legitimately want this.
var profileDefaultGuardedFields = []string{"mcp", "agent", "permission"}

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
// One per Service instance — wrapped in core.Once so concurrent
// callers don't race the DuckDB file open.
var (
	kvOnce core.Once
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
//
// Mantis #1603 HIGH (Cerberus #22) — Validates Profile.Provider /
// .MCP / .Agent / .Permission against a closed schema before
// persisting. The opaque map[string]any fields previously let any
// LocalKey-bearer caller install adversarial MCP commands, swap
// providers to attacker URLs, or rewrite the agent system-prompt by
// overwriting "default". Validation runs at the Service layer so the
// HTTP control surface, future CLI, and any other caller (orm
// migration, import) all inherit the boundary.
//
// On success when the profile name is "default" AND the body mutates
// any of profileDefaultGuardedFields (mcp / agent / permission), the
// Result.Value is a map carrying a "warning" key naming the guarded
// fields touched — the caller can surface this to the user but the
// write still lands (sandbox-internal blast radius per
// design_sandbox_is_the_safety_floor; the surface is informational).
//
// Returns Fail with ProfileInvalidSchema on schema violation, with
// the offending key path in the error message.
//
// Usage example:
//
//	r := svc.SaveProfile(opencode.Profile{Name: "tight-loop", Tools: map[string]bool{"bash": true}})
//	if r.Code() == opencode.ProfileInvalidSchema { /* user-facing */ }
func (s *Service) SaveProfile(p Profile) core.Result {
	if core.Trim(p.Name) == "" {
		return core.Fail(core.E("opencode.SaveProfile", "profile name is required", nil))
	}
	if err := validateProfileSchema(p); err != nil {
		return core.Fail(err)
	}
	st, r := kv()
	if !r.OK {
		return r
	}
	if err := st.Set(profileStoreGroup, p.Name, core.JSONMarshalString(p)); err != nil {
		return core.Fail(err)
	}
	// Default-profile guard surfacing — the write lands either way,
	// but the caller learns which spawn-affecting fields were touched
	// so the UI can render a "this changes every future spawn" notice.
	if p.Name == DefaultProfile {
		touched := defaultGuardedTouched(p)
		if len(touched) > 0 {
			return core.Ok(map[string]any{
				"warning":         ProfileDefaultGuard,
				"guarded_fields":  touched,
				"warning_message": "default profile mutation affects every future spawn",
			})
		}
	}
	return core.Ok(nil)
}

// validateProfileSchema enforces the closed-schema contract on a
// Profile blob. Returns nil when the blob is acceptable; returns an
// error coded ProfileInvalidSchema with a human-readable key-path
// when not. Pure function — no Service state touched, so the test
// suite hits it without DuckDB ceremony.
//
// Usage example:
//
//	if err := validateProfileSchema(p); err != nil { return core.Fail(err) }
func validateProfileSchema(p Profile) error {
	if err := validateProfileProvider(p.Provider); err != nil {
		return err
	}
	if err := validateProfileMCP(p.MCP); err != nil {
		return err
	}
	if err := validateProfileAgent(p.Agent); err != nil {
		return err
	}
	if err := validateProfilePermission(p.Permission); err != nil {
		return err
	}
	if err := validateProfileStringValue("model", p.Model); err != nil {
		return err
	}
	if err := validateProfileStringValue("small_model", p.SmallModel); err != nil {
		return err
	}
	return nil
}

// defaultGuardedTouched returns the subset of profileDefaultGuardedFields
// that the supplied profile actually carries a non-nil value for. The
// "default" profile warning fires only when at least one such field is
// present; pure-Tools / pure-EnabledProviders mutations of default are
// silent.
func defaultGuardedTouched(p Profile) []string {
	out := []string{}
	for _, f := range profileDefaultGuardedFields {
		switch f {
		case "mcp":
			if len(p.MCP) > 0 {
				out = append(out, f)
			}
		case "agent":
			if len(p.Agent) > 0 {
				out = append(out, f)
			}
		case "permission":
			if len(p.Permission) > 0 {
				out = append(out, f)
			}
		}
	}
	return out
}

// validateProfileProvider walks the provider map; rejects unknown
// provider IDs + unknown per-provider sub-keys + shell-metacharacters
// in any string value + over-long strings.
func validateProfileProvider(provider map[string]any) error {
	for providerID, raw := range provider {
		if !profileAllowedProviderKeys[providerID] {
			return core.NewCode(ProfileInvalidSchema,
				"unknown provider id: "+providerID)
		}
		sub, ok := raw.(map[string]any)
		if !ok {
			return core.NewCode(ProfileInvalidSchema,
				"provider."+providerID+" must be a map")
		}
		for k, v := range sub {
			if !profileAllowedProviderSubKeys[k] {
				return core.NewCode(ProfileInvalidSchema,
					"unknown provider key: provider."+providerID+"."+k)
			}
			if k == "options" {
				if err := validateProfileProviderOptions(providerID, v); err != nil {
					return err
				}
				continue
			}
			if err := validateProfileAnyValue("provider."+providerID+"."+k, v); err != nil {
				return err
			}
		}
	}
	return nil
}

// validateProfileProviderOptions walks the nested options map.
// baseURL is shape-validated as a URL; other keys go through the
// generic any-value validator.
func validateProfileProviderOptions(providerID string, raw any) error {
	opts, ok := raw.(map[string]any)
	if !ok {
		return core.NewCode(ProfileInvalidSchema,
			"provider."+providerID+".options must be a map")
	}
	for k, v := range opts {
		if !profileAllowedProviderOptionsKeys[k] {
			return core.NewCode(ProfileInvalidSchema,
				"unknown provider options key: provider."+providerID+".options."+k)
		}
		if k == "baseURL" {
			s, ok := v.(string)
			if !ok {
				return core.NewCode(ProfileInvalidSchema,
					"provider."+providerID+".options.baseURL must be a string")
			}
			if !profileIsValidURL(s) {
				return core.NewCode(ProfileInvalidSchema,
					"provider."+providerID+".options.baseURL is not a valid http(s) URL")
			}
			continue
		}
		if err := validateProfileAnyValue("provider."+providerID+".options."+k, v); err != nil {
			return err
		}
	}
	return nil
}

// validateProfileMCP walks the MCP map. Per-record: closed key set,
// command + args carry shell-metacharacter rejection, url is URL-shape
// validated. A record must declare either command OR url, not both.
func validateProfileMCP(mcp map[string]any) error {
	for serverID, raw := range mcp {
		if err := validateProfileIdentifier("mcp", serverID); err != nil {
			return err
		}
		sub, ok := raw.(map[string]any)
		if !ok {
			return core.NewCode(ProfileInvalidSchema,
				"mcp."+serverID+" must be a map")
		}
		hasCommand := false
		hasURL := false
		for k, v := range sub {
			if !profileAllowedMCPKeys[k] {
				return core.NewCode(ProfileInvalidSchema,
					"unknown mcp key: mcp."+serverID+"."+k)
			}
			switch k {
			case "command":
				hasCommand = true
				s, ok := v.(string)
				if !ok {
					return core.NewCode(ProfileInvalidSchema,
						"mcp."+serverID+".command must be a string")
				}
				if err := validateProfileNoShellMetachars("mcp."+serverID+".command", s); err != nil {
					return err
				}
			case "args":
				arr, ok := v.([]any)
				if !ok {
					return core.NewCode(ProfileInvalidSchema,
						"mcp."+serverID+".args must be an array of strings")
				}
				for i, item := range arr {
					s, ok := item.(string)
					if !ok {
						return core.NewCode(ProfileInvalidSchema,
							"mcp."+serverID+".args["+core.Sprintf("%d", i)+"] must be a string")
					}
					if err := validateProfileNoShellMetachars("mcp."+serverID+".args", s); err != nil {
						return err
					}
				}
			case "url":
				hasURL = true
				s, ok := v.(string)
				if !ok {
					return core.NewCode(ProfileInvalidSchema,
						"mcp."+serverID+".url must be a string")
				}
				if !profileIsValidURL(s) {
					return core.NewCode(ProfileInvalidSchema,
						"mcp."+serverID+".url is not a valid http(s) URL")
				}
			default:
				if err := validateProfileAnyValue("mcp."+serverID+"."+k, v); err != nil {
					return err
				}
			}
		}
		if hasCommand && hasURL {
			return core.NewCode(ProfileInvalidSchema,
				"mcp."+serverID+" cannot declare both command and url")
		}
	}
	return nil
}

// validateProfileAgent walks the agent map; closed key set + generic
// string-shape validation on values (length cap + no NULs).
func validateProfileAgent(agent map[string]any) error {
	for agentID, raw := range agent {
		if err := validateProfileIdentifier("agent", agentID); err != nil {
			return err
		}
		sub, ok := raw.(map[string]any)
		if !ok {
			return core.NewCode(ProfileInvalidSchema,
				"agent."+agentID+" must be a map")
		}
		for k, v := range sub {
			if !profileAllowedAgentKeys[k] {
				return core.NewCode(ProfileInvalidSchema,
					"unknown agent key: agent."+agentID+"."+k)
			}
			if err := validateProfileAnyValue("agent."+agentID+"."+k, v); err != nil {
				return err
			}
		}
	}
	return nil
}

// validateProfilePermission walks the permission map. Both keys
// (verbs) and values (allow / ask / deny) are closed sets.
func validateProfilePermission(perm map[string]any) error {
	for verb, raw := range perm {
		if !profileAllowedPermissionVerbs[verb] {
			return core.NewCode(ProfileInvalidSchema,
				"unknown permission verb: permission."+verb)
		}
		s, ok := raw.(string)
		if !ok {
			return core.NewCode(ProfileInvalidSchema,
				"permission."+verb+" must be one of allow|ask|deny (got non-string)")
		}
		if !profileAllowedPermissionValues[s] {
			return core.NewCode(ProfileInvalidSchema,
				"permission."+verb+" must be one of allow|ask|deny (got: "+s+")")
		}
	}
	return nil
}

// validateProfileAnyValue is the generic any-typed value validator.
// Walks nested maps + arrays; rejects strings with NUL bytes or over
// the length cap. Used for provider sub-values (npm, name, models)
// and agent sub-values (description, system_prompt, etc.).
func validateProfileAnyValue(path string, v any) error {
	switch t := v.(type) {
	case string:
		return validateProfileStringValue(path, t)
	case map[string]any:
		for k, child := range t {
			if err := validateProfileAnyValue(path+"."+k, child); err != nil {
				return err
			}
		}
		return nil
	case []any:
		for i, child := range t {
			if err := validateProfileAnyValue(path+"["+core.Sprintf("%d", i)+"]", child); err != nil {
				return err
			}
		}
		return nil
	case nil, bool, float64, int, int32, int64:
		return nil
	default:
		// Unknown JSON-shape: numbers come through as float64 from
		// encoding/json, so the cases above cover the legitimate
		// shapes. Anything else is suspect (a function, channel,
		// etc. from a non-JSON caller path).
		return core.NewCode(ProfileInvalidSchema,
			path+" has unsupported value type")
	}
}

// validateProfileStringValue caps a string value at profileMaxStringLen
// and rejects NUL bytes (defence against truncation attacks against
// downstream consumers that interpret NUL as terminator).
func validateProfileStringValue(path, s string) error {
	if len(s) > profileMaxStringLen {
		return core.NewCode(ProfileInvalidSchema,
			path+" exceeds max length "+core.Sprintf("%d", profileMaxStringLen))
	}
	if core.Contains(s, "\x00") {
		return core.NewCode(ProfileInvalidSchema,
			path+" must not contain NUL bytes")
	}
	return nil
}

// validateProfileNoShellMetachars rejects strings carrying any byte
// from profileShellMetacharacters. Use for MCP command + args where
// a downstream consumer that mis-shells the value would be burned.
func validateProfileNoShellMetachars(path, s string) error {
	if err := validateProfileStringValue(path, s); err != nil {
		return err
	}
	for i := 0; i < len(s); i++ {
		if core.Contains(profileShellMetacharacters, string(s[i])) {
			return core.NewCode(ProfileInvalidSchema,
				path+" contains forbidden shell metacharacter")
		}
	}
	return nil
}

// validateProfileIdentifier enforces the identifier shape used for
// MCP server IDs + Agent IDs: ASCII alphanumeric + dash + underscore,
// 1-64 bytes. Defends against keys carrying path-traversal sequences
// or other smuggling shapes.
func validateProfileIdentifier(scope, id string) error {
	if id == "" {
		return core.NewCode(ProfileInvalidSchema,
			scope+" identifier must be non-empty")
	}
	if len(id) > 64 {
		return core.NewCode(ProfileInvalidSchema,
			scope+"."+id+" identifier exceeds 64 bytes")
	}
	for i := 0; i < len(id); i++ {
		c := id[i]
		ok := (c >= 'a' && c <= 'z') ||
			(c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') ||
			c == '-' || c == '_' || c == '.'
		if !ok {
			return core.NewCode(ProfileInvalidSchema,
				scope+"."+id+" identifier has invalid character")
		}
	}
	return nil
}

// profileIsValidURL shape-validates a URL string for the provider /
// MCP url fields. Accepts http and https only — schemes like file://
// or javascript: would smuggle local-file reads or downstream eval.
func profileIsValidURL(s string) bool {
	if s == "" {
		return false
	}
	if !core.HasPrefix(s, "http://") && !core.HasPrefix(s, "https://") {
		return false
	}
	// No control characters; no shell metachars that would matter if a
	// downstream consumer interpolates the URL into a shell command.
	for i := 0; i < len(s); i++ {
		if s[i] < 0x20 || s[i] == 0x7f {
			return false
		}
	}
	return true
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
