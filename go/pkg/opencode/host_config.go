// SPDX-Licence-Identifier: EUPL-1.2

// Host-side opencode.json merge — when a user runs opencode CLI/TUI
// directly on the host (not via our sandbox), this writes the lthn
// provider block into their global opencode config so opencode picks
// up the local lthn runner at http://localhost:8000/v1 without
// copy-paste.
//
// Per RFC.opencode.md §3.3 the "easy mode" UX is two buttons on the
// integrations card — Copy snippet + Merge. The Merge button calls
// this function via POST /v1/api/opencode/host-config.
//
// Merge semantics (v1):
//
//   - Read existing ~/.config/opencode/opencode.json as plain JSON
//     (JSONC support is a v2 — for now if the user has authored a
//     JSONC file with comments we surface an error so they edit it
//     manually rather than silently break their config).
//   - If missing → create with profile.Provider as the seed.
//   - If `provider.lthn` exists with same baseURL → no-op (idempotent).
//   - If `provider.lthn` exists with DIFFERENT baseURL → return
//     HostConfigConflict so the frontend prompts before overwriting.
//   - Other provider entries are left untouched.
//   - `model` / `enabled_providers` are NEVER touched on the host
//     side — those are sandbox-scope narrowing concerns. Setting
//     `enabled_providers: ["lthn"]` on a host config that uses
//     other providers would silently lock the user out of them.

package opencode

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/paths"
)

// hostConfigSubpath is opencode's canonical global config path,
// relative to $HOME.
const hostConfigSubpath = ".config/opencode/opencode.json"

// HostConfigConflict is the core error code returned when
// provider.lthn already exists with a different baseURL. Frontend
// detects this and prompts the user before retrying with force=true.
const HostConfigConflict = "opencode.host-config.conflict"

// MergeHostConfigOptions narrows the merge behaviour at call time.
type MergeHostConfigOptions struct {
	// Profile is the named profile whose Provider block is merged
	// into the host config. Empty = DefaultProfile.
	Profile string `json:"profile,omitempty"`
	// Force overwrites a conflicting provider.lthn block instead of
	// returning HostConfigConflict.
	Force bool `json:"force,omitempty"`
}

// MergeHostConfigResult is the success-shape returned to callers.
//
// Bytes carries the FULL pretty-printed opencode.json that landed on
// disk — which includes any pre-existing user provider blocks (e.g.
// OpenAI / Anthropic apiKey strings) preserved across the merge. It is
// available to in-process callers (audit-suppression decisions, internal
// reconciliation) but MUST NEVER reach the HTTP wire response or the
// audit Meta map. The `json:"-"` tag is the type-system enforcement of
// that boundary (Mantis #1617 / Cerberus #22 HIGH-2): any caller that
// JSON-encodes a MergeHostConfigResult silently drops Bytes, so a future
// handler that forgets to build a view-struct still cannot leak the
// embedded apiKey blocks. Audit Meta omits Bytes explicitly at the emit
// site (see control.go hostConfigMerge + control_test.go
// TestHostConfigMerge_AuditEmitted_Good).
type MergeHostConfigResult struct {
	// Path is the absolute path of the file that was written.
	Path string `json:"path"`
	// Profile is the profile name that was applied.
	Profile string `json:"profile"`
	// Bytes is the pretty-printed JSON that landed on disk. NEVER
	// wire-encoded (see type comment above) — `json:"-"` is the
	// load-bearing tag, not cosmetic.
	Bytes string `json:"-"`
	// Created is true when the file did not exist before this call.
	Created bool `json:"created"`
}

// MergeHostConfig merges the named profile's provider block into the
// host-side ~/.config/opencode/opencode.json file. Returns the file
// path + resulting bytes on success.
//
// Usage example:
//
//	r := svc.MergeHostConfig(opencode.MergeHostConfigOptions{})
//	if r.OK { res := r.Value.(opencode.MergeHostConfigResult); _ = res }
func (s *Service) MergeHostConfig(opts MergeHostConfigOptions) core.Result {
	profileName := core.Trim(opts.Profile)
	if profileName == "" {
		profileName = DefaultProfile
	}
	profileR := s.GetProfile(profileName)
	if !profileR.OK {
		return profileR
	}
	profile := profileR.Value.(Profile)

	homeR := core.UserHomeDir()
	if !homeR.OK {
		return homeR
	}
	path := core.PathJoin(homeR.Value.(string), hostConfigSubpath)

	// Read existing or treat as empty.
	created := true
	existing := map[string]any{}
	if r := core.ReadFile(path); r.OK {
		data, _ := r.Value.([]byte)
		if len(data) > 0 {
			created = false
			if ur := core.JSONUnmarshal(data, &existing); !ur.OK {
				return core.Fail(core.E("opencode.MergeHostConfig",
					"existing opencode.json is not valid JSON "+
						"(JSONC parsing is a v2 feature; remove comments / "+
						"trailing commas or delete the file to re-seed)", nil))
			}
		}
	}

	// Conflict detection — provider.lthn must match if it exists.
	existingProvider := map[string]any{}
	if v, ok := existing["provider"].(map[string]any); ok {
		existingProvider = v
	}
	if existingLthn, ok := existingProvider["lthn"].(map[string]any); ok {
		existingURL := nestedString(existingLthn, "options", "baseURL")
		incomingURL := ""
		if newLthn, ok := profile.Provider["lthn"].(map[string]any); ok {
			incomingURL = nestedString(newLthn, "options", "baseURL")
		}
		if existingURL != incomingURL && !opts.Force {
			return core.Fail(core.NewCode(HostConfigConflict,
				core.Sprintf("provider.lthn already exists with baseURL=%q "+
					"(incoming=%q); call again with force=true to overwrite",
					existingURL, incomingURL)))
		}
	}

	// Merge provider — entries from profile.Provider overwrite
	// matching keys in existing.provider; other keys are kept.
	//
	// `model` and `enabled_providers` are deliberately NOT merged
	// for host-config writes — those are sandbox-scope narrowing
	// fields. Writing `enabled_providers: ["lthn"]` onto the host
	// config would suppress every other provider the user has
	// configured (e.g. their own OpenAI / Anthropic keys); writing
	// `model` would change their default. T1 is purely "add the
	// lthn provider so opencode can find it" — nothing more.
	for k, v := range profile.Provider {
		existingProvider[k] = v
	}
	existing["provider"] = existingProvider

	// Ensure parent dir + write.
	//
	// Mode discipline (Cerberus #22 MED-1 / Mantis #1618):
	//
	//   - Parent dir 0o700 (user-only access). The merged file may
	//     embed pre-existing user provider apiKey blocks (OpenAI,
	//     Anthropic) preserved verbatim across the merge — see the
	//     MergeHostConfigResult.Bytes type comment. A 0o755 parent
	//     dir leaks the directory listing to other local users; an
	//     0o644 file leaks the apiKey blocks to cross-user read.
	//   - File written via paths.AtomicWriteWithVersion which uses
	//     0o600 verbatim (paths.writeFileMode) AND adds the tmp +
	//     fsync + rename atomic-write guarantee (power-failure-safe
	//     replacement, no half-written file ever visible at path).
	//   - WriteInput is left as the unconditional shape (no
	//     IfVersion / IfMtime / IfMatchHash / IfNotExist) — this
	//     surface deliberately re-writes on each merge and there is
	//     no version field in opencode.json's schema for us to pin
	//     against. Lock-serialisation under WithFileLock prevents
	//     two concurrent Merge calls from racing.
	parent := core.PathDir(path)
	if r := core.MkdirAll(parent, 0o700); !r.OK {
		return r
	}

	outR := core.JSONMarshalIndent(existing, "", "  ")
	if !outR.OK {
		return outR
	}
	outBytes, _ := outR.Value.([]byte)
	if r := paths.AtomicWriteWithVersion(path, paths.WriteInput{
		Body: outBytes,
	}); !r.OK {
		return r
	}

	return core.Ok(MergeHostConfigResult{
		Path:    path,
		Profile: profileName,
		Bytes:   string(outBytes),
		Created: created,
	})
}

// nestedString walks nested map[string]any and returns the string at
// the final key, or "" if any step is missing or the wrong type.
func nestedString(m map[string]any, keys ...string) string {
	cur := any(m)
	for _, k := range keys {
		asMap, ok := cur.(map[string]any)
		if !ok {
			return ""
		}
		cur, ok = asMap[k]
		if !ok {
			return ""
		}
	}
	if s, ok := cur.(string); ok {
		return s
	}
	return ""
}
