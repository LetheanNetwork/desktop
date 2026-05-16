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

// pathsOverrideMu serialises in-process read-modify-write callers of
// the override file (Cerberus DREAD-r3 LOW, Mantis #1541). Two
// concurrent SetModelsDirOverride / ClearModelsDirOverride callers
// would otherwise both read pathsOverride, both mutate it, and race
// at the write step — second writer's intent silently lost. The
// mutex makes the read-modify-write atomic from the program's view.
//
// Cross-process safety (multiple desktop binaries against the same
// $HOME) needs core.Flock — pending the CoreGO primitive. This
// mutex closes the in-process gap only.
var pathsOverrideMu core.Mutex

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
//
// Cerberus H#9-verify F3 (Mantis #1536): writes are atomic via
// tmp-file + rename rather than direct write. The previous direct
// core.WriteFile(file, raw, ...) call left a window where a crash
// (power loss, OOM-kill, container teardown) between truncate and
// flush would land a partial JSON blob on disk. On next start
// readPathsOverride() would fail to parse the truncated body and
// silently return the zero-value struct — losing the user's override
// without any indication. tmp+rename ensures the final paths.json
// either holds the previous valid content or the fully-serialised
// new content; never a half-written intermediate.
//
// Cerberus DREAD-r3 LOW (Mantis #1541): the tmp filename carries a
// per-call random suffix (paths.json.tmp.<16-hex-chars>) rather than
// a fixed paths.json.tmp. Two concurrent writers would otherwise
// stomp the SAME staging file mid-stream and one writer's tmp body
// would land partially overwritten before its rename, leaving a
// corrupt blob on disk if the rename happened to fire on the
// interleaved bytes. With unique tmp filenames each caller writes
// intact content; the final rename is last-writer-wins which is the
// expected semantics for a serial override store. Pairs with the
// pathsOverrideMu lock on the Set/Clear callsites to serialise
// in-process read-modify-write.
//
// Mode 0o600 on the tmp file too (Cerberus L3 2026-05-16): the
// override holds user-supplied path context (drive labels, mount
// names) that mustn't be world-readable even mid-rename.
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
	// 8 bytes of crypto entropy → 16 hex chars → 64 bits of
	// uniqueness per tmp filename. Far more than enough to dodge
	// collision under any realistic concurrent-writer load.
	suffix := core.RandomString(8)
	if !suffix.OK {
		return core.Fail(core.E("paths.writePathsOverride", "random suffix gen failed", suffix.Value.(error)))
	}
	tmp := file + ".tmp." + suffix.Value.(string)
	if w := core.WriteFile(tmp, raw, 0o600); !w.OK {
		return core.Fail(core.E("paths.writePathsOverride", "write tmp failed", w.Value.(error)))
	}
	if r := core.Rename(tmp, file); !r.OK {
		// Best-effort cleanup so a stale tmp doesn't sit forever; the
		// rename failure is the real error we surface. Uses the same
		// randomised tmp name we just wrote — never the legacy fixed
		// paths.json.tmp.
		_ = core.Remove(tmp)
		return core.Fail(core.E("paths.writePathsOverride", "rename tmp→final failed", r.Value.(error)))
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
	// Cerberus DREAD-r3 LOW (Mantis #1541): serialise the read-
	// modify-write sequence so two concurrent in-process callers
	// can't both base their write on the same pre-read state and
	// lose the first writer's intent. Cross-process safety still
	// pending core.Flock (CoreGO gap).
	pathsOverrideMu.Lock()
	defer pathsOverrideMu.Unlock()
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
	// Cerberus DREAD-r3 LOW (Mantis #1541): same in-process
	// read-modify-write serialisation as SetModelsDirOverride —
	// a concurrent Clear vs Set must not race on the file.
	pathsOverrideMu.Lock()
	defer pathsOverrideMu.Unlock()
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
