// SPDX-Licence-Identifier: EUPL-1.2

// id_validation.go — confused-deputy guard for user-supplied IDs that
// land in core.PathJoin. Closes Cerberus pass-9 #1486 (CRITICAL path
// traversal across 20+ wails surfaces).
//
// Threat model:
//
//	frontend → wails IPC → svc.Get({ID:"../../wallets/lethean-default"})
//	  → core.PathJoin(dir, ID+".md")  → reads outside the service dir.
//
// Today the only check at the wails entry points is "input.ID != \"\"".
// That lets any plugin or compromised webview pivot reads/writes to
// keystore, account.pgp, paths.json, etc.
//
// Two layered guards:
//
//   - IsValidID(id)         — shape check, rejects empty, /, .., \, NUL,
//                             leading dot, and anything > 255 bytes.
//   - WithinDir(base, full) — belt-and-braces, asserts full resolves
//                             under base after CleanPath. Catches odd
//                             cases (UNC-ish prefixes, repeated slashes
//                             collapsing to a parent) that IsValidID
//                             didn't anticipate.
//
// Threading discipline (Hephaestus 2026-05-16): every wails handler that
// pulls input.ID / input.Slug / input.DealID and concatenates it into a
// path must call paths.IsValidID(id) before the join. WithinDir is the
// belt for paths that are constructed by helpers (e.g. loadOne that
// walks year/month dirs).

package paths

import core "dappco.re/go"

// MaxIDBytes is the upper bound on an ID byte length. 255 matches the
// canonical filesystem name-byte limit on ext4/APFS/NTFS so any ID we
// accept can land as a file leaf without surprise truncation.
const MaxIDBytes = 255

// IsValidID returns nil when id is safe to concatenate into a path
// segment owned by a service directory. Returns a typed core error
// otherwise so callers can surface "paths.invalid_id" uniformly.
//
// Reject cases (Cerberus #1486):
//
//   - empty
//   - contains '/' (path separator — caller must single-segment)
//   - contains "\\" (Windows-style path separator)
//   - contains ".." (parent-dir token)
//   - contains a NUL byte (C-string truncation attack)
//   - starts with '.' (hidden-file shape AND ./ prefix)
//   - longer than MaxIDBytes
//
// Usage example:
//
//	if err := paths.IsValidID(input.ID); err != nil {
//	    return core.Fail(err)
//	}
//	fpath := core.PathJoin(dir, input.ID+".md")
func IsValidID(id string) error {
	if id == "" {
		return core.E("paths.invalid_id", "id is empty", nil)
	}
	if len(id) > MaxIDBytes {
		return core.E("paths.invalid_id", "id exceeds 255 bytes", nil)
	}
	if id[0] == '.' {
		return core.E("paths.invalid_id", "id must not start with '.'", nil)
	}
	if core.Contains(id, "..") {
		return core.E("paths.invalid_id", "id must not contain '..'", nil)
	}
	if core.Contains(id, "/") {
		return core.E("paths.invalid_id", "id must not contain '/'", nil)
	}
	if core.Contains(id, "\\") {
		return core.E("paths.invalid_id", "id must not contain '\\'", nil)
	}
	for i := 0; i < len(id); i++ {
		if id[i] == 0 {
			return core.E("paths.invalid_id", "id must not contain NUL byte", nil)
		}
	}
	return nil
}

// WithinDir reports whether candidate resolves to a path under base
// after canonical cleaning. Belt-and-braces partner of IsValidID — the
// shape check rejects obvious attack tokens, this catches resolution
// surprises (e.g. base="/a/b", candidate="/a/b/../c" → "/a/c", outside
// base).
//
// Both arguments are taken as absolute paths; relative candidates are
// rejected (return false). Equality with base alone is accepted —
// callers usually then append a leaf segment that re-anchors inside.
//
// Usage example:
//
//	fpath := core.PathJoin(dir, input.ID+".md")
//	if !paths.WithinDir(dir, fpath) {
//	    return core.Fail(core.E("paths.escape", "computed path escapes base", nil))
//	}
func WithinDir(base, candidate string) bool {
	if base == "" || candidate == "" {
		return false
	}
	if !core.PathIsAbs(base) || !core.PathIsAbs(candidate) {
		return false
	}
	cleanBase := core.CleanPath(base, "/")
	cleanCand := core.CleanPath(candidate, "/")
	if cleanCand == cleanBase {
		return true
	}
	// Append "/" to base so that "/a/b" is not treated as a prefix of
	// "/a/bc" — the candidate must be a strict child segment.
	return core.HasPrefix(cleanCand, cleanBase+"/")
}
