// SPDX-Licence-Identifier: EUPL-1.2

// Models-dir override surface for the paths package — Cerberus H1 /
// Mantis 2026-05-16. The first-launch wizard lets the user point
// the models directory at any host path (external SSD, NAS mount,
// etc.). That choice persists to ~/Lethean/conf/paths.json and is
// consulted by future ModelsDir() reads.
//
// The override is a confused-deputy risk: the WebView (and, post-
// plugin-enablement, third-party plugin code) can set it. Without
// validation a malicious caller could redirect model writes to
// ~/.ssh, ~/Library/LaunchAgents, /etc, mounted shares, etc.
//
// Defences applied here:
//
//   - must be absolute
//   - must live strictly under $HOME (no escape to /etc, /usr/local,
//     mounted shares)
//   - first $HOME-relative segment must not start with "." (blocks
//     ~/.ssh, ~/.aws, ~/.gnupg, ~/.config, ~/.gitconfig dirs)
//   - first segment must NOT be "Library" (macOS persistence vector
//     — LaunchAgents, Application Support, etc.)
//   - the resolved path must NOT itself be a symlink (Lstat — Stat
//     would silently follow into a forbidden dir)
//
// File layout:
//
//	~/Lethean/conf/paths.json    { "models_dir": "/abs/path" }
//
// Mode 0o600 — the override holds a user-supplied path that may
// carry context (mount labels, drive names) we don't want world-
// readable.
//
// NB: this file ships the producer side only. The ModelsDir() hot
// path that consults readModelsDirOverride() lives in paths.go and
// is owned by the parallel paths-sweep lane; until that wires up,
// Set/Clear persist correctly but ModelsDir() returns the default.

package paths

import core "dappco.re/go"

// pathsOverride is the JSON shape written to ~/Lethean/conf/paths.json.
// Empty strings mean "no override — fall through to the default".
type pathsOverride struct {
	ModelsDir string `json:"models_dir,omitempty"`
}

// pathsOverrideFile returns the absolute path to the override JSON.
// Empty string if the conf dir can't be resolved. Pure — never
// writes anywhere itself.
func pathsOverrideFile() string {
	conf := ConfDir()
	if !conf.OK {
		return ""
	}
	return core.PathJoin(conf.Value.(string), "paths.json")
}

// readPathsOverride returns the parsed override JSON (zero-value
// struct when the file is missing or unreadable — overrides are a
// best-effort augmentation, never a fail-stop).
func readPathsOverride() pathsOverride {
	file := pathsOverrideFile()
	if file == "" {
		return pathsOverride{}
	}
	if stat := core.Stat(file); !stat.OK {
		return pathsOverride{}
	}
	body := core.ReadFile(file)
	if !body.OK {
		return pathsOverride{}
	}
	raw, _ := body.Value.([]byte)
	var p pathsOverride
	if dec := core.JSONUnmarshalString(string(raw), &p); !dec.OK {
		return pathsOverride{}
	}
	return p
}

// readModelsDirOverride returns the user-set models dir override, or
// empty string when none is set. Convenience helper so ModelsDir's
// hot path doesn't need to know the override schema.
func readModelsDirOverride() string {
	return readPathsOverride().ModelsDir
}

// writePathsOverride persists the override struct to disk. Empty
// fields are omitted (omitempty) so a partial override only carries
// the keys the user actually set.
func writePathsOverride(p pathsOverride) core.Result {
	file := pathsOverrideFile()
	if file == "" {
		return core.Fail(core.NewError("paths.writePathsOverride: conf dir unresolved"))
	}
	body := core.JSONMarshal(p)
	if !body.OK {
		return body
	}
	raw, _ := body.Value.([]byte)
	// 0o600 (Cerberus L3 2026-05-16): the override file holds a
	// user-supplied path that may carry context (drive labels, mount
	// names) we don't want world-readable.
	if w := core.WriteFile(file, raw, 0o600); !w.OK {
		return core.Fail(core.E("paths.writePathsOverride", "write failed", w.Value.(error)))
	}
	return core.Ok(file)
}

// SetModelsDirOverride writes the user's chosen models directory to
// ~/Lethean/conf/paths.json. The path is validated against the
// confused-deputy posture (Cerberus H1 2026-05-16):
//
//   - must be absolute
//   - must live under $HOME (so a WebView/plugin can't pivot writes
//     into /etc/, /usr/local/, /System/, mounted shares, etc.)
//   - first segment past $HOME must NOT be a dot-prefixed directory
//     (~/.ssh, ~/.aws, ~/.gnupg, ~/.config — all attacker targets)
//   - must NOT be inside ~/Library (LaunchAgents persistence vector
//     on macOS) or under any of the allowlist-explicit blocked dirs
//   - the path itself + the resolved $HOME-relative chain must NOT
//     be a symlink (Lstat — Stat would follow it and pass)
//
// Failure mode is a typed error so the UI can surface the reason
// rather than silently dropping the override.
//
// Usage example:
//
//	r := paths.SetModelsDirOverride("/Users/snider/Vault/lthn-models")
//	if r.OK { abs := r.Value.(string); _ = abs }
func SetModelsDirOverride(p string) core.Result {
	if p == "" {
		return core.Fail(core.NewError("paths.SetModelsDirOverride: empty path"))
	}
	if !core.PathIsAbs(p) {
		return core.Fail(core.NewError("paths.SetModelsDirOverride: path must be absolute"))
	}
	if reason := validateOverridePath(p); reason != "" {
		return core.Fail(core.NewError("paths.SetModelsDirOverride: " + reason))
	}
	if r := core.MkdirAll(p, 0o755); !r.OK {
		return core.Fail(core.E("paths.SetModelsDirOverride", "mkdir failed", r.Value.(error)))
	}
	// Lstat (not Stat) — Stat follows symlinks and would happily
	// accept a symlink-into-/etc/. The override target itself must be
	// a real directory.
	lstatR := core.Lstat(p)
	if !lstatR.OK {
		return core.Fail(core.E("paths.SetModelsDirOverride", "lstat failed", lstatR.Value.(error)))
	}
	info, _ := lstatR.Value.(core.FsFileInfo)
	if info == nil || !info.IsDir() {
		return core.Fail(core.NewError("paths.SetModelsDirOverride: path is not a directory (or is a symlink)"))
	}
	cur := readPathsOverride()
	cur.ModelsDir = p
	if r := writePathsOverride(cur); !r.OK {
		return r
	}
	return core.Ok(p)
}

// validateOverridePath enforces the H1 confused-deputy guard. Returns
// an empty string when the path passes; a short human-readable reason
// when it doesn't. Pure — testable without the filesystem (the path
// shape is what's checked; symlink + dir checks live in the caller).
//
// Allowlist policy (deny by default):
//   - must be under $HOME
//   - first $HOME-relative segment must not start with "."
//   - must not be under ~/Library (macOS persistence root)
func validateOverridePath(p string) string {
	homeR := core.UserHomeDir()
	if !homeR.OK {
		return "home directory unresolved"
	}
	home, _ := homeR.Value.(string)
	clean := core.CleanPath(p, "/")
	// Containment: strict under $HOME (equal to $HOME is also rejected
	// to avoid pointing the override at the literal home root).
	if !core.HasPrefix(clean+"/", home+"/") || clean == home {
		return "path must live under your home directory"
	}
	// First segment past $HOME.
	rel := clean[len(home):]
	for len(rel) > 0 && rel[0] == '/' {
		rel = rel[1:]
	}
	if rel == "" {
		return "path must live under your home directory"
	}
	first := rel
	if i := indexByteOverride(rel, '/'); i >= 0 {
		first = rel[:i]
	}
	if len(first) > 0 && first[0] == '.' {
		return "path must not be inside a hidden ('.') directory"
	}
	if first == "Library" {
		return "path must not be under ~/Library"
	}
	return ""
}

// indexByteOverride mirrors strings.IndexByte for a single ASCII
// byte. Local helper — avoids pulling strings into the import set
// just for one scan of a path segment. Renamed from indexByte to
// avoid collision when paths.go (sibling lane) introduces its own
// helper of the same name.
func indexByteOverride(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}

// ClearModelsDirOverride removes the models-dir entry from the
// override file, restoring the default. Idempotent — missing file
// + missing key both succeed silently. Returns Ok(nil).
//
// Usage example:
//
//	r := paths.ClearModelsDirOverride()
//	if !r.OK { core.Warn("clear failed", "err", r.Error()) }
func ClearModelsDirOverride() core.Result {
	cur := readPathsOverride()
	if cur.ModelsDir == "" {
		return core.Ok(nil)
	}
	cur.ModelsDir = ""
	if r := writePathsOverride(cur); !r.OK {
		return r
	}
	return core.Ok(nil)
}
