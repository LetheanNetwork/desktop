// SPDX-Licence-Identifier: EUPL-1.2

// atomic_write.go — paths.AtomicWriteWithVersion + paths.ReadVersion
// per RFC.atomic-write.md §2.
//
// The primitive is a thin, opaque-byte write surface: callers pass
// the final body; the primitive applies a composite optimistic-lock
// check, writes via tmp + fsync + rename, and surfaces a typed
// VersionStale payload on conflict. Version-frontmatter encode /
// decode is the caller's responsibility — the primitive only READS
// the version field from existing files for the IfVersion gate, and
// never rewrites it.
//
// Flow per §2:
//
//   1. paths.WithFileLock(path, timeout, fn)
//   2. Inside fn: ReadVersion(path) under lock → current state
//      (version, mtime, hash, body).
//   3. Composite-or-pick-one check — any non-zero IfVersion /
//      IfMtime / IfMatchHash supplied that does NOT match → fail
//      with VersionStale envelope.
//   4. WriteFile(<path>.tmp, Body, 0o600) + fsync + Rename(.tmp).
//   5. Lock released by WithFileLock defer.
//
// CRIT-1 enforcement (§4.2):
//
//   - VersionStale.CurrentBody is omitempty + opt-in only via
//     WriteInput.IncludeBody (default false).
//   - AtRestEncryptedPrefixes() listed paths NEVER include body
//     regardless of IncludeBody.
//   - When body IS included it is hard-capped at
//     CurrentBodyMaxBytes (1 MiB); larger files surface
//     CurrentBodyTooLarge=true and omit the bytes.
//
// MED-2 (legacy first-write): when the caller leaves all three
// conditions zero AND the file does not exist on disk, the primitive
// treats this as a legitimate unconditional first-write. ReadVersion
// of a missing file returns Version=0, zero Mtime, empty Body, empty
// Hash — the primitive's conflict-check sees nothing to compare and
// proceeds.

package paths

import (
	core "dappco.re/go"
)

// Write-path error codes. Reserved schema — pattern-matched by
// HTTP envelope construction and frontend conflict-UI handlers.
const (
	CodeWriteInvalidPath = "paths.write.invalid_path"
	CodeWriteOpenFailed  = "paths.write.open_failed"
	CodeWriteFsync       = "paths.write.fsync_failed"
	CodeWriteRename      = "paths.write.rename_failed"
	CodeVersionStale     = "paths.write.version_stale"
)

// CurrentBodyMaxBytes is the §4.2 CRIT-1 cap on VersionStale
// CurrentBody payloads. 1 MiB matches the documents-v2 Q2 ruling;
// larger files force the client through the proper GET path.
const CurrentBodyMaxBytes = 1 << 20

// CurrentBodyTooLargeFlag is the sentinel that flags an omitted
// body that exceeded the 1 MiB cap. Surfaced in VersionStale.Flags
// so the frontend can distinguish "body omitted because opt-out"
// from "body omitted because too large".
const CurrentBodyTooLargeFlag = "body_too_large_for_inline"

// CurrentBodyAtRestOmitFlag flags an omission because the path
// matched AtRestEncryptedPrefixes(). The client falls back to the
// proper authed GET to retrieve the bytes.
const CurrentBodyAtRestOmitFlag = "body_omitted_at_rest_path"

// File mode for new + replaced files. 0o600 matches the
// ~/Lethean/wallets/ + ~/Lethean/account/<id>/ discipline.
const writeFileMode core.FileMode = 0o600

// WriteInput is the call payload for AtomicWriteWithVersion.
//
// Composite-or-pick-one (HIGH-1) — populate whichever subset the
// caller can compute; ANY supplied condition that fails triggers a
// VersionStale response. Leaving all three zero is the
// unconditional-write shape (used for first-write to a non-existent
// path; MED-2).
//
// Usage example:
//
//	r := paths.AtomicWriteWithVersion(fpath, paths.WriteInput{
//	    Body:        composed,
//	    IfMtime:     prior.Mtime,
//	    IfMatchHash: prior.BodyHash,
//	    IncludeBody: false,
//	})
type WriteInput struct {
	// Body is the new file content, written verbatim.
	Body []byte

	// IfVersion expects ReadVersion(path).Version == IfVersion when
	// >0. 0 = skip this check.
	IfVersion int

	// IfMtime expects ReadVersion(path).Mtime to compare <= IfMtime
	// when non-zero. Zero = skip this check.
	IfMtime core.Time

	// IfMatchHash expects ReadVersion(path).BodyHash == IfMatchHash
	// when non-empty. Empty = skip this check.
	IfMatchHash string

	// NewVersion is the version the caller intends to stamp. Today
	// reserved for forward-compatible frontmatter helpers — the
	// primitive does not encode this into the body itself; callers
	// embed it in their composed frontmatter before submission. Kept
	// in the struct so the field exists across the upcoming sweep
	// of services that gain a version field.
	NewVersion int

	// Timeout caps the wait for the file lock. 0 = LockTimeoutDefault.
	Timeout core.Duration

	// IncludeBody opts into CurrentBody on VersionStale responses
	// (§4.2 CRIT-1). Default false. At-rest-encrypted prefixes
	// silently ignore opt-in.
	IncludeBody bool
}

// WriteOutput is the success-path payload (returned in Result.Value
// of an OK Result).
type WriteOutput struct {
	Version int        `json:"version"`
	Mtime   core.Time  `json:"mtime"`
	Hash    string     `json:"hash"`
}

// VersionStale is the conflict-path payload. Surfaced via
// Result.Fail with a typed core error whose embedded payload (use
// VersionStaleFrom on the returned Result) carries the structured
// fields the HTTP envelope renders into a 409 response.
type VersionStale struct {
	CurrentVersion int       `json:"current_version"`
	CurrentMtime   core.Time `json:"current_mtime"`
	CurrentHash    string    `json:"current_hash"`           // always present
	CurrentBody    []byte    `json:"current_body,omitempty"` // §4.2 opt-in only
	Flags          []string  `json:"flags,omitempty"`        // body_too_large_for_inline / body_omitted_at_rest_path
}

// ReadOutput is the payload returned by ReadVersion.
type ReadOutput struct {
	Version  int       `json:"version"`  // 0 when file has no version frontmatter OR does not exist
	Mtime    core.Time `json:"mtime"`    // zero when file does not exist
	Body     []byte    `json:"body"`     // empty when file does not exist
	BodyHash string    `json:"body_hash"`// SHA-256 hex of body, empty when file does not exist
}

// AtRestEncryptedPrefixes returns the workspace-relative prefix list
// (Stage E §4 C2) whose contents are encrypted at rest. Paths that
// match a prefix here are NEVER eligible for CurrentBody inclusion
// in VersionStale responses regardless of IncludeBody (CRIT-1).
//
// Matching is path-segment-relative-to-paths.Root() — both
// "~/Lethean/wallets/server.key" and the absolute equivalent return
// true.
//
// Usage example:
//
//	if paths.IsAtRestEncryptedPath(somePath) { ... }
func AtRestEncryptedPrefixes() []string {
	return []string{
		"wallets/",
		"account/",
		"office/mail/_accounts.enc",
		"sales/",
		"incidents",
		"runbooks/",
	}
}

// IsAtRestEncryptedPath reports whether p (absolute or
// workspace-relative) names a file under one of the
// AtRestEncryptedPrefixes() entries.
//
// Usage example:
//
//	if paths.IsAtRestEncryptedPath(fpath) { ... }
func IsAtRestEncryptedPath(p string) bool {
	rel := workspaceRel(p)
	for _, prefix := range AtRestEncryptedPrefixes() {
		// Trailing-slash prefixes match path-segment prefixes.
		if core.HasSuffix(prefix, "/") {
			if core.HasPrefix(rel, prefix) {
				return true
			}
			continue
		}
		// Non-trailing entries match exact path OR path with
		// trailing segment (e.g. "incidents" → "incidents",
		// "incidents/2026.md", "incidents.md" is NOT a match).
		if rel == prefix {
			return true
		}
		if core.HasPrefix(rel, prefix+"/") {
			return true
		}
	}
	return false
}

// workspaceRel reduces p to its workspace-relative form (strip
// paths.Root() prefix) for prefix matching. Falls back to p
// verbatim when Root() resolution fails (boot-time edge case).
func workspaceRel(p string) string {
	root := Root()
	if !root.OK {
		return p
	}
	r := root.Value.(string)
	if core.HasPrefix(p, r+"/") {
		return p[len(r)+1:]
	}
	return p
}

// ReadVersion atomically reads the file's current state in one
// stat+read cycle. Callers MUST NOT split into separate stat-then-
// read — race window opens.
//
// A missing file returns Version=0, zero Mtime, empty Body + Hash,
// Result.OK=true. Other I/O errors (permission, IO) surface as
// Result.Fail.
//
// Usage example:
//
//	r := paths.ReadVersion(fpath)
//	if !r.OK { return r }
//	cur := r.Value.(paths.ReadOutput)
func ReadVersion(path string) core.Result {
	statR := core.Lstat(path)
	if !statR.OK {
		if core.IsNotExist(unwrapErr(statR)) {
			return core.Ok(ReadOutput{})
		}
		return core.Fail(core.E("paths.ReadVersion", "stat failed: "+statR.Error(), nil))
	}
	info, _ := statR.Value.(core.FsFileInfo)
	if info == nil {
		return core.Fail(core.NewCode("paths.ReadVersion",
			"stat returned nil info"))
	}
	if info.IsDir() {
		return core.Fail(core.NewCode("paths.ReadVersion",
			"path resolves to a directory"))
	}
	rawR := core.ReadFile(path)
	if !rawR.OK {
		return core.Fail(core.E("paths.ReadVersion", "read failed: "+rawR.Error(), nil))
	}
	raw, _ := rawR.Value.([]byte)
	return core.Ok(ReadOutput{
		Version:  parseFrontmatterVersion(raw),
		Mtime:    info.ModTime(),
		Body:     raw,
		BodyHash: core.SHA256Hex(raw),
	})
}

// AtomicWriteWithVersion performs the lock + read + composite-check
// + tmp + fsync + rename sequence per §2. Returns Ok(WriteOutput)
// on success or Fail(VersionStale-bearing error) on optimistic-lock
// failure.
//
// Usage example:
//
//	r := paths.AtomicWriteWithVersion(fpath, paths.WriteInput{
//	    Body:        newBytes,
//	    IfMtime:     prior.Mtime,
//	    IfMatchHash: prior.BodyHash,
//	})
//	if !r.OK { return r }
//	out := r.Value.(paths.WriteOutput)
func AtomicWriteWithVersion(path string, input WriteInput) core.Result {
	if path == "" {
		return core.Fail(core.NewCode(CodeWriteInvalidPath,
			"AtomicWriteWithVersion requires a non-empty path"))
	}
	return WithFileLock(path, input.Timeout, func() core.Result {
		// Read current state under lock.
		rdR := ReadVersion(path)
		if !rdR.OK {
			return rdR
		}
		cur, _ := rdR.Value.(ReadOutput)

		// Composite-or-pick-one (HIGH-1).
		stale := false
		if input.IfVersion > 0 && cur.Version != input.IfVersion {
			stale = true
		}
		if !input.IfMtime.IsZero() && !cur.Mtime.IsZero() {
			// mtime check is "current mtime <= IfMtime" — the
			// expected-prior-snapshot semantics. Any newer mtime
			// is stale.
			if cur.Mtime.After(input.IfMtime) {
				stale = true
			}
		}
		if input.IfMatchHash != "" && cur.BodyHash != input.IfMatchHash {
			stale = true
		}
		if stale {
			return failVersionStale(path, cur, input.IncludeBody)
		}

		// Two-phase write: tmp + fsync + rename.
		//
		// Mantis #1552 — per-call randomised tmp suffix mirrors the
		// #1541 paths.json fix. Concurrent in-process writers to the
		// same path used to race on a fixed ".tmp" staging path
		// (one writer's truncate landing mid-stream of another's
		// content). Per-call hex suffix removes that footgun
		// structurally; each writer owns its own staging file.
		//
		// Random source is core.RandomString(8) → 16 hex chars; if
		// crypto/rand is unavailable the surface returns a typed
		// open_failed Fail rather than silently fall back to a
		// predictable suffix.
		var tmp string
		if rs := core.RandomString(8); rs.OK {
			tmp = path + ".tmp." + rs.Value.(string)
		} else {
			emitWriteFailed(path, CodeWriteOpenFailed)
			return core.Fail(core.E(CodeWriteOpenFailed,
				"random suffix: "+rs.Error(), nil))
		}
		var openR core.Result
		if writeTmpOpenFaultForTest != nil {
			openR = writeTmpOpenFaultForTest(tmp)
		} else {
			openR = core.OpenFile(tmp,
				core.O_CREATE|core.O_WRONLY|core.O_TRUNC, writeFileMode)
		}
		if !openR.OK {
			emitWriteFailed(path, CodeWriteOpenFailed)
			return core.Fail(core.E(CodeWriteOpenFailed,
				"open tmp: "+openR.Error(), nil))
		}
		f, _ := openR.Value.(*core.OSFile)
		if f == nil {
			emitWriteFailed(path, CodeWriteOpenFailed)
			return core.Fail(core.NewCode(CodeWriteOpenFailed,
				"open tmp returned nil file"))
		}
		if _, err := f.Write(input.Body); err != nil {
			_ = f.Close()
			_ = core.Remove(tmp)
			emitWriteFailed(path, CodeWriteOpenFailed)
			return core.Fail(core.E(CodeWriteOpenFailed,
				"write tmp", err))
		}
		if err := f.Sync(); err != nil {
			_ = f.Close()
			_ = core.Remove(tmp)
			emitWriteFailed(path, CodeWriteFsync)
			return core.Fail(core.E(CodeWriteFsync,
				"fsync tmp", err))
		}
		if err := f.Close(); err != nil {
			_ = core.Remove(tmp)
			emitWriteFailed(path, CodeWriteOpenFailed)
			return core.Fail(core.E(CodeWriteOpenFailed,
				"close tmp", err))
		}
		if r := core.Rename(tmp, path); !r.OK {
			_ = core.Remove(tmp)
			emitWriteFailed(path, CodeWriteRename)
			return core.Fail(core.E(CodeWriteRename,
				"rename: "+r.Error(), nil))
		}

		// Post-write stat for the success envelope.
		newStat := core.Lstat(path)
		out := WriteOutput{Hash: core.SHA256Hex(input.Body)}
		if newStat.OK {
			if info, _ := newStat.Value.(core.FsFileInfo); info != nil {
				out.Mtime = info.ModTime()
			}
		}
		out.Version = parseFrontmatterVersion(input.Body)

		// Emit audit + typed event (lock-acquired-and-write-succeeded
		// is collapsed into one event for the writer-success case).
		emitWriteSucceeded(path, out.Version)

		return core.Ok(out)
	})
}

// failVersionStale builds the VersionStale envelope per §4.2 rules
// (opt-in body, at-rest never-include, 1 MiB cap) and returns it as
// a typed Fail Result.
func failVersionStale(path string, cur ReadOutput, includeBody bool) core.Result {
	vs := VersionStale{
		CurrentVersion: cur.Version,
		CurrentMtime:   cur.Mtime,
		CurrentHash:    cur.BodyHash,
	}
	switch {
	case !includeBody:
		// Default omit — no flag, plain omission.
	case IsAtRestEncryptedPath(path):
		vs.Flags = append(vs.Flags, CurrentBodyAtRestOmitFlag)
	case len(cur.Body) > CurrentBodyMaxBytes:
		vs.Flags = append(vs.Flags, CurrentBodyTooLargeFlag)
	default:
		vs.CurrentBody = cur.Body
	}
	emitVersionStale(path, cur.Version)
	return core.Fail(versionStaleError{
		stale: vs,
	})
}

// versionStaleError carries the typed VersionStale payload so HTTP
// envelope construction can extract it via VersionStaleFromError.
type versionStaleError struct {
	stale VersionStale
}

// Error returns the canonical error code so log-tailers + tests
// can pattern-match on it via core.Contains.
func (e versionStaleError) Error() string {
	return CodeVersionStale
}

// VersionStaleFromError extracts the structured VersionStale payload
// from a Result returned by AtomicWriteWithVersion. Returns
// (VersionStale, true) when the error is a typed stale envelope;
// (zero, false) otherwise. HTTP envelope construction calls this to
// shape the 409 body.
//
// Usage example:
//
//	if vs, ok := paths.VersionStaleFromError(r.Value); ok {
//	    return ctx.JSON(409, vs)
//	}
func VersionStaleFromError(v any) (VersionStale, bool) {
	if err, ok := v.(error); ok {
		if vse, ok := err.(versionStaleError); ok {
			return vse.stale, true
		}
	}
	return VersionStale{}, false
}

// parseFrontmatterVersion scans body for a leading "---\n" YAML
// block and returns the int value of any "version: <int>" line
// inside. Returns 0 when no frontmatter / no version field /
// unparseable value. NOT a full YAML parser — only the version
// field is needed by the primitive, and a single-pass byte scan is
// faster + has no error surface to thread.
func parseFrontmatterVersion(raw []byte) int {
	const open = "---\n"
	if len(raw) < len(open) {
		return 0
	}
	for i := 0; i < len(open); i++ {
		if raw[i] != open[i] {
			return 0
		}
	}
	content := raw[len(open):]
	// Find closing "---" delimiter.
	closeIdx := -1
	for i := 0; i < len(content)-2; i++ {
		if content[i] == '-' && content[i+1] == '-' && content[i+2] == '-' {
			if i == 0 || content[i-1] == '\n' {
				closeIdx = i
				break
			}
		}
	}
	if closeIdx < 0 {
		return 0
	}
	fm := content[:closeIdx]
	// Walk line-by-line for "version: <int>".
	start := 0
	for i := 0; i <= len(fm); i++ {
		if i == len(fm) || fm[i] == '\n' {
			line := fm[start:i]
			if v, ok := matchVersionLine(line); ok {
				return v
			}
			start = i + 1
		}
	}
	return 0
}

// matchVersionLine extracts the int after "version:" on a single
// frontmatter line. Returns (0, false) when the line is not a
// version assignment or the value is non-int.
func matchVersionLine(line []byte) (int, bool) {
	// trim leading whitespace
	i := 0
	for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
		i++
	}
	const key = "version:"
	if i+len(key) > len(line) {
		return 0, false
	}
	for k := 0; k < len(key); k++ {
		if line[i+k] != key[k] {
			return 0, false
		}
	}
	rest := line[i+len(key):]
	// trim leading whitespace after colon
	j := 0
	for j < len(rest) && (rest[j] == ' ' || rest[j] == '\t') {
		j++
	}
	val := rest[j:]
	// trim trailing whitespace / comment
	end := len(val)
	for end > 0 && (val[end-1] == ' ' || val[end-1] == '\t' || val[end-1] == '\r') {
		end--
	}
	if end == 0 {
		return 0, false
	}
	r := core.Atoi(string(val[:end]))
	if !r.OK {
		return 0, false
	}
	return r.Value.(int), true
}

// Event-emission stubs — overridden in events.go (B.1.c) once the
// LockEvent typed-bus + audit wiring lands. Stubbed here so the
// write surface ships independently and the events plumbing can
// land in its own commit without forcing this file to grow a
// circular dependency.
var emitWriteSucceeded = func(path string, version int) {}
var emitVersionStale = func(path string, currentVersion int) {}

// emitWriteFailed reports a typed write-failure to the audit
// recorder. Wired in events.go init() once the LockEvent bus is
// constructed. Mantis #1551. code is one of CodeWriteOpenFailed /
// CodeWriteFsync / CodeWriteRename so log readers can pattern-match
// the failure step without parsing the free-form message.
var emitWriteFailed = func(path string, code string) {}

// writeTmpOpenFaultForTest is a fault-injection hook used by
// Mantis #1551 coverage to force a deterministic write-step failure
// without filesystem-permission trickery (which collides with the
// lock sentinel's parent-dir sensitivity). When non-nil it overrides
// the OpenFile call that stages the tmp file inside
// AtomicWriteWithVersion. Production code MUST NOT touch this —
// pair every test setter with t.Cleanup that resets it to nil.
var writeTmpOpenFaultForTest func(tmp string) core.Result

// SetWriteTmpOpenFaultForTest installs a fault-injection callback
// that AtomicWriteWithVersion consults in place of the tmp-stage
// OpenFile. Pass nil to disable. Test-only.
func SetWriteTmpOpenFaultForTest(fn func(tmp string) core.Result) {
	writeTmpOpenFaultForTest = fn
}

// unwrapErr coaxes a Result's Value into an error for IsNotExist
// classification. core.E + core.Result conventions wrap errors in
// the Value field on !OK; this helper does the type-assertion in
// one place.
func unwrapErr(r core.Result) error {
	if e, ok := r.Value.(error); ok {
		return e
	}
	return nil
}
