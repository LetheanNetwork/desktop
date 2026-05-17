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

// MaxPluginCodeBytes is the upper bound on a plugin code byte length.
// 64 matches the pkg/plugin original isValidPluginCode cap — plugin
// codes flow through URL routes + install directory basenames so a
// tighter cap than MaxIDBytes keeps the surface conservative.
const MaxPluginCodeBytes = 64

// CodePluginCodeCollision is the typed error code Install-time
// duplicate-code rejections raise (Mantis #1580). Stable for HTTP
// envelope matching + frontend toast routing. Same shape discipline
// as the cascade conflict codes — service-prefix + dot-separated.
const CodePluginCodeCollision = "plugin.code.collision"

// IsValidPluginCode reports whether code is a safe basename for the
// plugin install directory + URL route. Shared between pkg/plugin
// (binary plugins) and pkg/marketplace (lthn-vm bundle plugin block)
// per Mantis #1580 — cousin-validator drift was the original gap
// (pkg/marketplace had no equivalent gate so a marketplace-declared
// plugin.code could bypass the path-traversal hardening pkg/plugin
// got from Cerberus Mantis #1436).
//
// Rules (mirror of the original pkg/plugin.isValidPluginCode):
//
//   - Length 1..MaxPluginCodeBytes (64).
//   - First character: ASCII alphanumeric (so `--malicious` flags
//     can't appear as a route segment).
//   - Remaining characters: alphanumeric or `_` or `-`.
//   - No `/`, no `.`, no `..`, no whitespace, no control chars.
//
// Shape mirrors sandbox.IsValidVolumeName (same primitive on
// adjacent surface — Cerberus's pass-3 "shape observation"). Dots
// are allowed in volume names but NOT plugin codes; plugin codes
// flow through URL paths too and a leading dot would surprise the
// router.
//
// Usage example:
//
//	if !paths.IsValidPluginCode(input.Code) {
//	    return core.Fail(core.E(op,
//	        "invalid plugin code (must be alphanumeric basename, 1-64 chars, leading alnum): "+input.Code, nil))
//	}
func IsValidPluginCode(code string) bool {
	if code == "" || len(code) > MaxPluginCodeBytes {
		return false
	}
	for i, ch := range code {
		alnum := ('a' <= ch && ch <= 'z') ||
			('A' <= ch && ch <= 'Z') ||
			('0' <= ch && ch <= '9')
		if i == 0 {
			if !alnum {
				return false
			}
			continue
		}
		if !(alnum || ch == '_' || ch == '-') {
			return false
		}
	}
	return true
}

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

// IsValidAbsoluteUnderRoot validates that a user-supplied candidate
// path is safe to use as a destination under a known root directory.
// Used by Office v2.B.3 dispatches (mail-v2 attachment download +
// files-v2 location addition) where the user picks a target path
// the backend then writes to / reads from — the helper closes the
// confused-deputy gap between "the user typed a path" and "the
// service is about to touch it".
//
// Contract (mail-v2 §3.1):
//
//  1. candidate is non-empty + contains no NUL bytes
//  2. tilde expansion (`~/foo` → `<HOME>/foo`) applied BEFORE
//     absoluteness + clean-path checks. Bare `~` (no slash) expands
//     to HOME itself.
//  3. candidate (post-tilde) is absolute — starts with `/` on POSIX
//  4. CleanPath(candidate) is byte-equal to candidate post-tilde
//     — defends against `..`, `.`, and double-slash sequences
//     that Clean would collapse. Caller MUST submit a pre-cleaned
//     path; we don't silently rewrite (a Clean that changes the
//     bytes signals the caller's intent diverges from the resolved
//     path, which is exactly the prompt-injection signal we want
//     to surface as an error).
//  5. root MUST be absolute. Relative roots reject — a relative
//     root is a programming bug, not a user error.
//  6. CleanPath(candidate) MUST live under CleanPath(root) — same
//     WithinDir semantics (equal-or-strict-child).
//
// Returns nil when all rules pass; a typed core error with code
// `paths.invalid_absolute` otherwise so callers can surface a
// uniform machine-readable failure.
//
// Usage example:
//
//	if err := paths.IsValidAbsoluteUnderRoot(workspaceRoot, input.Destination); err != nil {
//	    return core.Fail(err)
//	}
//	// safe to: core.WriteFile(input.Destination, payload, 0o600)
func IsValidAbsoluteUnderRoot(root, candidate string) error {
	if candidate == "" {
		return core.E("paths.invalid_absolute", "candidate is empty", nil)
	}
	// NUL-byte guard before any other processing — a stray NUL would
	// truncate downstream C-string consumers (e.g. open syscalls).
	for i := 0; i < len(candidate); i++ {
		if candidate[i] == 0 {
			return core.E("paths.invalid_absolute", "candidate must not contain NUL byte", nil)
		}
	}
	for i := 0; i < len(root); i++ {
		if root[i] == 0 {
			return core.E("paths.invalid_absolute", "root must not contain NUL byte", nil)
		}
	}
	// Tilde expansion — `~` or `~/foo`. Anything past the first
	// character isn't an alias for another user's home (we don't
	// support `~alice/...`) so `~alice` rejects on the absoluteness
	// check below.
	expanded := candidate
	if candidate == "~" || (len(candidate) >= 2 && candidate[0] == '~' && candidate[1] == '/') {
		homeR := core.UserHomeDir()
		if !homeR.OK {
			return core.E("paths.invalid_absolute", "tilde expansion failed: HOME unavailable", nil)
		}
		home, _ := homeR.Value.(string)
		if home == "" {
			return core.E("paths.invalid_absolute", "tilde expansion failed: HOME empty", nil)
		}
		if candidate == "~" {
			expanded = home
		} else {
			expanded = home + candidate[1:]
		}
	}
	if !core.PathIsAbs(expanded) {
		return core.E("paths.invalid_absolute", "candidate is not absolute after tilde expansion", nil)
	}
	cleanCand := core.CleanPath(expanded, "/")
	if cleanCand != expanded {
		return core.E("paths.invalid_absolute",
			"candidate is not path-clean — collapse '..' / '.' / '//' before submission", nil)
	}
	if root == "" {
		return core.E("paths.invalid_absolute", "root is empty", nil)
	}
	if !core.PathIsAbs(root) {
		return core.E("paths.invalid_absolute", "root must be absolute", nil)
	}
	cleanRoot := core.CleanPath(root, "/")
	if cleanCand == cleanRoot {
		return nil
	}
	if !core.HasPrefix(cleanCand, cleanRoot+"/") {
		return core.E("paths.invalid_absolute", "candidate escapes root after cleaning", nil)
	}
	return nil
}
