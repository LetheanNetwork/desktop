// SPDX-Licence-Identifier: EUPL-1.2

// Service — Core integration for the Office document catalogue. Reads
// markdown files from ~/Lethean/office/docs/ and returns typed rows to
// the Wails frontend. v2 adds Create / Save / Delete with optimistic
// locking (mtime + sha256 composite token) and live-read session-gate
// defence (RFC.stage-e-unlockgate v2 §1.2 / Mantis #1613 B.1).
//
// Lifecycle:
//   - Register(c)        wires the service into the Core container
//   - ServiceName()      returns "Documents" for the Wails namespace
//   - SetSessionGate(g)  wired post-construction in cmd/lthn/app.go
//     against *account.Service (live-read pattern — mirrors
//     office/mail.AccountProvider; no cached bool, no event bus).
//   - Stop(ctx)          nils the SessionGate reference so a draining
//     Service fails-closed on any late-arriving write.
//
// All I/O uses CoreGO wrappers (core.ReadFile / core.ReadDir /
// core.DirFS / core.MkdirAll / core.Stat / core.UserHomeDir /
// core.WriteFile / core.Rename).
// Banned stdlib imports: os, path/filepath, strings, encoding/json,
// fmt, log, errors.

package documents

import (
	"sync"
	"sync/atomic"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/office/internal/safedir"
	"dappco.re/lthn/desktop/pkg/paths"
	"gopkg.in/yaml.v3"
)

// SessionGate is the minimal consumer-defined interface satisfied by
// *account.Service. Live-read at every gate check — no cached bool, no
// subscribe/event bus (RFC.stage-e-unlockgate v2 §1.1 — Pushback 2
// CONFIRMED by Cerberus #27). When the returned slice is empty the
// session is locked; when non-empty at least one Lethean account is
// unlocked and writes may proceed.
//
// Wired in cmd/lthn/app.go (Mantis #1613 B.3, deferred to that lane):
//
//	documentsSvc.SetSessionGate(accountSvc)
//
// AX-8 compliance: this interface is defined in the consumer
// (documents) and satisfied by the producer (*account.Service). No
// pkg/account import lands in pkg/office/documents.
type SessionGate interface {
	UnlockedAccountIDs() []string
}

// SECURITY-NOTE — TIER-1 TRUSTED SURFACE (Mantis #1502, Cerberus pass-10):
//
// This Wails surface returns PII (mail threads / file paths / document bodies)
// to any in-process WebView caller. Today's caller is the trusted host SPA only.
// Post-#1421 (plugin enablement) MUST NOT inherit these methods — plugin tier
// requires explicit user consent + a stronger boundary than the in-process Wails
// IPC.
//
// Bridge bearer auth (#1423) gates only /mcp/* + /internal/* — it does NOT gate
// this Wails surface. Per design_sandbox_is_the_safety_floor + Snider's A1 floor:
// adding new endpoints here MUST be deliberate; new fields on existing endpoints
// MUST be reviewed for PII propagation.
//
// When v2 features (#1495/#1496/#1497) populate threads / docs / file index,
// re-audit this surface against the plugin-capability split.

// Service owns the Office document catalogue surface.
//
// Usage example:
//
//	svc := documents.NewService(c)
//	svc.SetSessionGate(accountSvc)
type Service struct {
	core *core.Core

	// gateMu guards reads/writes of the session gate reference. A
	// sync.RWMutex protects against the wire/Stop race where app.go
	// SetSessionGate runs concurrent with a late-arriving Wails call
	// reading the reference. Read-heavy access (every write gates
	// once) — RWMutex.RLock is microseconds.
	gateMu sync.RWMutex
	// gate is the live-read session source (RFC §1.1). nil before
	// SetSessionGate runs in app.go and after Stop nils it; the
	// nilWarned one-shot warning fires on the first nil-hit to
	// surface wire-ordering bugs without log spam (§2.2 ADD-1.5).
	gate SessionGate
	// nilWarned is the one-shot guard for the nil-gate fail-safe
	// (§2.2 / Cerberus #27 Q2). CompareAndSwap-on-first-hit emits
	// core.Warn exactly once per Service instance.
	nilWarned atomic.Bool
}

// NewService constructs the documents service against a Core container.
// Wired via core.WithName("office-documents", documents.Register) in app.go.
//
// Usage example:
//
//	svc := documents.NewService(c)
func NewService(c *core.Core) *Service {
	return &Service{core: c}
}

// Register constructs the documents service for Core registration.
//
// Usage example:
//
//	core.New(core.WithName("office-documents", documents.Register))
func Register(c *core.Core) core.Result {
	return core.Ok(NewService(c))
}

// ServiceName labels the binding namespace exposed to JS.
// Wails binds methods as "Documents.List()" etc.
func (s *Service) ServiceName() string { return "Documents" }

// docsDir resolves ~/Lethean/office/docs/ and creates it if missing.
// Mode 0o700 — documents are PII (Cerberus #1487 mandate).
//
// Symlink-pivot defence (Mantis #1499 MED, Cerberus wave-2/pass-10):
// goes through safedir.MkdirAll so an attacker who pre-creates
// ~/Lethean/office/docs as a symlink to /tmp/evil cannot redirect
// document writes — safedir refuses with
// safedir.UnsafeSymlinkParentCode.
func docsDir() core.Result {
	root := paths.Root()
	if !root.OK {
		return root
	}
	dir := core.PathJoin(root.Value.(string), "office", "docs")
	if r := safedir.MkdirAll(dir, 0o700); !r.OK {
		return r
	}
	return core.Ok(dir)
}

// docFrontmatter is the YAML schema decoded from each document's
// optional frontmatter block. v2 adds Tags, Related, UpdatedAt.
type docFrontmatter struct {
	State     string    `yaml:"state"`
	Author    string    `yaml:"author"`
	Title     string    `yaml:"title"` // optional; overridden by H1 body scan
	Tags      []string  `yaml:"tags,omitempty"`
	Related   []string  `yaml:"related,omitempty"`
	UpdatedAt core.Time `yaml:"updated_at,omitempty"`
}

// parseDoc extracts a DocRecord from raw file bytes, the OS-provided
// mtime, and size. Does not use strings/path/filepath.
func parseDoc(slug string, raw []byte, modTime core.Time, sizeB int64) DocRecord {
	fm, body := splitFrontmatter(raw)

	var meta docFrontmatter
	// Ignore parse errors — malformed frontmatter just gives us zero values.
	_ = yaml.Unmarshal(fm, &meta)

	// State validation: only allow known values.
	switch meta.State {
	case "draft", "ready", "final", "live":
	default:
		meta.State = "draft"
	}

	title := meta.Title
	if title == "" {
		title = titleFromBody(body)
	}
	if title == "" {
		title = slug
	}

	return DocRecord{
		Slug:    slug,
		Title:   title,
		State:   meta.State,
		Author:  meta.Author,
		ModTime: modTime,
		SizeB:   sizeB,
	}
}

// splitFrontmatter splits a document into its YAML frontmatter bytes
// and body bytes. The document may or may not begin with "---\n"; when
// it doesn't, all content is treated as body and the returned fm is nil.
func splitFrontmatter(raw []byte) (fm []byte, body []byte) {
	const open = "---\n"
	if len(raw) < len(open) {
		return nil, raw
	}
	// Check opening delimiter.
	for i, b := range []byte(open) {
		if raw[i] != b {
			return nil, raw
		}
	}
	content := raw[len(open):]

	// Find the closing ---
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
		// No closing delimiter — all content is body.
		return nil, raw
	}
	fm = content[:closeIdx]
	rest := content[closeIdx+3:]
	if len(rest) > 0 && rest[0] == '\n' {
		rest = rest[1:]
	}
	return fm, rest
}

// titleFromBody scans body bytes for the first H1 (`# ` at line start)
// and returns the heading text without the `# ` prefix. Returns "" when
// no H1 is found. Does not import strings.
func titleFromBody(body []byte) string {
	i := 0
	for i < len(body) {
		// Find end of current line.
		lineEnd := i
		for lineEnd < len(body) && body[lineEnd] != '\n' {
			lineEnd++
		}
		line := body[i:lineEnd]
		// H1: line starts with `# ` (hash + space).
		if len(line) >= 2 && line[0] == '#' && line[1] == ' ' {
			heading := line[2:]
			// Trim trailing whitespace / carriage-return.
			end := len(heading)
			for end > 0 && (heading[end-1] == ' ' || heading[end-1] == '\r' || heading[end-1] == '\t') {
				end--
			}
			return string(heading[:end])
		}
		i = lineEnd + 1
	}
	return ""
}

// formatSize formats byte count into human-readable file size matching
// the fixture string precision ("4.2 KB", "248 KB", "1.2 MB").
func formatSize(b int64) string {
	const kb = int64(1024)
	const mb = int64(1024 * 1024)
	switch {
	case b >= mb:
		if b%mb < mb/10 {
			return core.Sprintf("%d MB", b/mb)
		}
		return core.Sprintf("%.1f MB", float64(b)/float64(mb))
	case b >= kb:
		if b%kb < kb/10 {
			return core.Sprintf("%d KB", b/kb)
		}
		return core.Sprintf("%.1f KB", float64(b)/float64(kb))
	default:
		return core.Sprintf("%d B", b)
	}
}

// relativeEdit converts an mtime delta to the fixture strings:
// "now", "yest", "X d ago", "X w ago", "X mo ago".
func relativeEdit(t core.Time, now core.Time) string {
	if t.IsZero() {
		return ""
	}
	diff := now.Sub(t)
	if diff < 0 {
		diff = -diff
	}
	const minute = core.Minute
	const hour = core.Hour
	const day = 24 * core.Hour
	const week = 7 * day
	const month = 30 * day

	switch {
	case diff < minute:
		return "now"
	case diff < 20*hour:
		return core.Sprintf("%d h ago", int(diff/hour))
	case diff < 48*hour:
		return "yest"
	case diff < week:
		return core.Sprintf("%d d ago", int(diff/day))
	case diff < month:
		return core.Sprintf("%d w ago", int(diff/week))
	default:
		return core.Sprintf("%d mo ago", int(diff/month))
	}
}

// resolveAuthor returns "you" when raw matches the current OS username;
// otherwise returns raw. Empty raw → "you".
func resolveAuthor(raw string) string {
	if raw == "" {
		return "you"
	}
	userR := core.UserCurrent()
	if userR.OK {
		if u, _ := userR.Value.(*core.User); u != nil && u.Username == raw {
			return "you"
		}
	}
	return raw
}

// toRow converts a DocRecord to the wire type.
func toRow(r DocRecord, now core.Time) DocRow {
	return DocRow{
		Title:  r.Title,
		State:  r.State,
		Author: resolveAuthor(r.Author),
		Edited: relativeEdit(r.ModTime, now),
		Size:   formatSize(r.SizeB),
	}
}


// scanDocs reads all *.md files from docsDir() and returns a slice of
// DocRecord values sorted by ModTime descending (newest first).
// Malformed files are warned and skipped.
func scanDocs() ([]DocRecord, error) {
	dirR := docsDir()
	if !dirR.OK {
		return nil, core.E("documents.scanDocs", dirR.Error(), nil)
	}
	dir := dirR.Value.(string)

	entriesR := core.ReadDir(core.DirFS(dir), ".")
	if !entriesR.OK {
		return nil, core.E("documents.scanDocs", entriesR.Error(), nil)
	}
	entries, _ := entriesR.Value.([]core.FsDirEntry)

	now := core.Now()
	var records []DocRecord
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if len(name) < 4 || name[len(name)-3:] != ".md" {
			continue
		}
		slug := name[:len(name)-3]
		fpath := core.PathJoin(dir, name)

		statR := core.Stat(fpath)
		if !statR.OK {
			core.Warn("documents: stat failed", "path", fpath)
			continue
		}
		info, _ := statR.Value.(core.FsFileInfo)
		if info == nil {
			continue
		}

		rawR := core.ReadFile(fpath)
		if !rawR.OK {
			core.Warn("documents: read failed", "path", fpath)
			continue
		}
		raw, _ := rawR.Value.([]byte)

		rec := parseDoc(slug, raw, info.ModTime(), info.Size())
		_ = now // modTime is set from info above
		records = append(records, rec)
	}

	// Sort by ModTime descending (newest first) — insertion sort is
	// fine for the small document counts expected here.
	for i := 1; i < len(records); i++ {
		for j := i; j > 0 && records[j].ModTime.After(records[j-1].ModTime); j-- {
			records[j], records[j-1] = records[j-1], records[j]
		}
	}
	return records, nil
}

// loadDoc returns the raw bytes of the document with the given slug.
func loadDoc(slug string) ([]byte, error) {
	dirR := docsDir()
	if !dirR.OK {
		return nil, core.E("documents.loadDoc", dirR.Error(), nil)
	}
	dir := dirR.Value.(string)
	fpath := core.PathJoin(dir, slug+".md")
	rawR := core.ReadFile(fpath)
	if !rawR.OK {
		return nil, core.E("documents.loadDoc", "not found: "+slug, nil)
	}
	raw, _ := rawR.Value.([]byte)
	return raw, nil
}

// SetSessionGate wires the live-read session source. Called by
// cmd/lthn/app.go post-construction (Mantis #1613 B.3) once
// *account.Service exists.
//
// Mirrors the office/mail.AccountProvider setter pattern. Replaces
// the v1 SetLocked(bool) bool-cache with a live-read on every gate
// check — no event-bus reliability concerns, no cache coherence
// concerns (RFC.stage-e-unlockgate v2 §1.1).
//
// Usage example:
//
//	docsSvc.SetSessionGate(accountSvc)
func (s *Service) SetSessionGate(g SessionGate) {
	s.gateMu.Lock()
	s.gate = g
	s.gateMu.Unlock()
}

// Stop nils the SessionGate reference so a draining Service
// fails-closed on any late-arriving write (§B.{1,2} mirror mail's
// drain hygiene / Cerberus #27 ADD-5). Read-only methods (List, Get)
// continue to function — Stop only severs the write gate.
//
// Usage example:
//
//	_ = svc.Stop(core.Background())
func (s *Service) Stop(_ core.Context) core.Result {
	s.gateMu.Lock()
	s.gate = nil
	s.gateMu.Unlock()
	return core.Ok(nil)
}

// assertUnlocked returns a Fail result when the session is locked or
// the session gate is not wired. Called at the top of every write
// method before any FS touch.
//
// Live-read semantics (RFC §1.1): consults s.gate.UnlockedAccountIDs()
// at every call — no cached bool — so a lock transition is observable
// on the very next write attempt.
//
// Fail-safe on nil gate (§2.2 / Cerberus #27 Q2): when SetSessionGate
// has not yet wired the gate (or Stop has nilled it), the gate fails
// LOCKED rather than panicking. The first nil-hit per Service
// instance emits a one-shot core.Warn via CompareAndSwap so
// wire-ordering bugs surface in dev without log spam in production.
func (s *Service) assertUnlocked(scope string) (core.Result, bool) {
	s.gateMu.RLock()
	g := s.gate
	s.gateMu.RUnlock()
	if g == nil {
		if s.nilWarned.CompareAndSwap(false, true) {
			core.Warn("documents: session gate not wired; failing locked", "scope", scope)
		}
		return core.Fail(core.E(scope, "documents.session.locked", nil)), false
	}
	if len(g.UnlockedAccountIDs()) == 0 {
		return core.Fail(core.E(scope, "documents.session.locked", nil)), false
	}
	return core.Result{}, true
}

// sessionUnlocked is a non-failing form of assertUnlocked for the
// post-write event-emit guard. Returns true when the gate is wired
// AND reports at least one unlocked account. Used at Save/Delete
// success to suppress DocChanged emission if the lock transitioned
// mid-write (RFC §non-goals: in-flight writes complete; the emit
// gate stays best-effort, matching v1's locked.Load() check).
func (s *Service) sessionUnlocked() bool {
	s.gateMu.RLock()
	g := s.gate
	s.gateMu.RUnlock()
	if g == nil {
		return false
	}
	return len(g.UnlockedAccountIDs()) > 0
}

// reservedSlugs are route-collision candidates that must not be used as
// document filenames.
var reservedSlugs = map[string]bool{
	"new":    true,
	"search": true,
	"index":  true,
}

// validateSlug wraps paths.IsValidID and rejects reserved route names.
func validateSlug(slug string) error {
	if err := paths.IsValidID(slug); err != nil {
		return err
	}
	if reservedSlugs[slug] {
		return core.E("documents.validateSlug", "reserved slug: "+slug, nil)
	}
	return nil
}

// readBodyHash reads the document at path in a single stat+open cycle and
// returns the hex(sha256(raw file bytes)) and the mtime. Delegated to
// paths.ReadVersion (B.2 cutover) — the primitive's snapshot is the
// canonical single-stat-and-read; this helper is retained for tests that
// continue to depend on the (hash, mtime, err) tuple shape.
func readBodyHash(path string) (hashHex string, mtime core.Time, err error) {
	r := paths.ReadVersion(path)
	if !r.OK {
		return "", core.Time{}, core.E("documents.readBodyHash", r.Error(), nil)
	}
	out, _ := r.Value.(paths.ReadOutput)
	if out.Mtime.IsZero() {
		return "", core.Time{}, core.E("documents.readBodyHash",
			"file does not exist", nil)
	}
	return out.BodyHash, out.Mtime, nil
}

// composeFrontmatter produces the YAML frontmatter block for a document.
// Keys are in a deterministic order: state, author, tags, related, updated_at.
func composeFrontmatter(state, author string, tags, related []string, updatedAt core.Time) ([]byte, error) {
	fm := docFrontmatter{
		State:     state,
		Author:    author,
		Tags:      tags,
		Related:   related,
		UpdatedAt: updatedAt,
	}
	out, err := yaml.Marshal(fm)
	if err != nil {
		return nil, core.E("documents.composeFrontmatter", "yaml marshal failed", err)
	}
	return out, nil
}

// atomicWriteFile writes content to path with optimistic-lock semantics
// via paths.AtomicWriteWithVersion (B.2 PILOT cutover — Mantis #1490).
// Empty conflict-condition fields trigger the unconditional first-write
// path; populated IfMtime + IfMatchHash route through the composite
// 409-or-rename flow. Returns a typed conflict error when stale so
// callers can pattern-match the existing "documents.save.conflict" code
// without rebuilding their error envelopes.
func atomicWriteFile(path string, content []byte, ifMtime core.Time, ifMatchHash string) error {
	r := paths.AtomicWriteWithVersion(path, paths.WriteInput{
		Body:        content,
		IfMtime:     ifMtime,
		IfMatchHash: ifMatchHash,
	})
	if r.OK {
		return nil
	}
	if core.Contains(r.Error(), paths.CodeVersionStale) {
		// Wrap the primitive's typed VersionStale envelope so callers
		// keep matching on documents.save.conflict while the structured
		// payload remains accessible via paths.VersionStaleFromError.
		return core.E("documents.atomicWriteFile", "documents.save.conflict",
			versionStaleAsError(r.Value))
	}
	return core.E("documents.atomicWriteFile", "write failed: "+r.Error(), nil)
}

// versionStaleAsError extracts the structured paths.VersionStale payload
// from a Result.Value and returns it as an error whose Error() reports
// the canonical "documents.save.conflict" code. The structured envelope
// (current_version / current_mtime / current_hash + flags) stays
// accessible via paths.VersionStaleFromError(err.(*versionStaleCause)).
func versionStaleAsError(v any) error {
	if e, ok := v.(error); ok {
		return e
	}
	return core.NewCode("documents.save.conflict", "version_stale")
}

// buildFileContent assembles the full file bytes from frontmatter + body.
func buildFileContent(fm []byte, body string) []byte {
	// "---\n" + fm bytes + "---\n" + body
	out := make([]byte, 0, 4+len(fm)+4+len(body))
	out = append(out, "---\n"...)
	out = append(out, fm...)
	out = append(out, "---\n"...)
	out = append(out, body...)
	return out
}

// normaliseTags lowercases each tag. Uses core.Lower which wraps strings.ToLower.
func normaliseTags(tags []string) []string {
	if len(tags) == 0 {
		return tags
	}
	out := make([]string, len(tags))
	for i, t := range tags {
		out[i] = core.Lower(t)
	}
	return out
}

// Create writes a new document file. Fires EventCreated on success.
// Rejects if the session is locked (documents.session.locked).
// Rejects if body exceeds MaxBodyBytes (documents.body.too_large).
// Rejects if slug already exists (documents.create.exists).
//
// Usage example:
//
//	r := svc.Create(documents.CreateInput{Slug: "notes", Body: "# Notes\n"})
//	if r.OK { /* created */ }
func (s *Service) Create(input CreateInput) core.Result {
	if fail, ok := s.assertUnlocked("documents.Create"); !ok {
		return fail
	}
	if err := validateSlug(input.Slug); err != nil {
		return core.Fail(core.E("documents.Create", "documents.slug.invalid", err))
	}
	if len(input.Body) > MaxBodyBytes {
		return core.Fail(core.E("documents.Create", "documents.body.too_large", nil))
	}

	dirR := docsDir()
	if !dirR.OK {
		return core.Fail(core.E("documents.Create", dirR.Error(), nil))
	}
	dir := dirR.Value.(string)
	fpath := core.PathJoin(dir, input.Slug+".md")

	// Reject if file already exists.
	if statR := core.Stat(fpath); statR.OK {
		return core.Fail(core.E("documents.Create", "documents.create.exists", nil))
	}

	state := input.State
	switch state {
	case "draft", "ready", "final", "live":
	default:
		state = "draft"
	}

	userR := core.UserCurrent()
	author := ""
	if userR.OK {
		if u, _ := userR.Value.(*core.User); u != nil {
			author = u.Username
		}
	}

	now := core.Now()
	fm, err := composeFrontmatter(state, author, normaliseTags(input.Tags), input.Related, now)
	if err != nil {
		return core.Fail(core.E("documents.Create", "frontmatter compose failed", err))
	}

	content := buildFileContent(fm, input.Body)
	// Create is the unconditional first-write — empty IfMtime + empty
	// IfMatchHash trigger the primitive's MED-2 path. The exists-check
	// above prevents a concurrent Create racing this one.
	if err := atomicWriteFile(fpath, content, core.Time{}, ""); err != nil {
		return core.Fail(core.E("documents.Create", "write failed", err))
	}

	statR := core.Stat(fpath)
	var modTime core.Time
	var sizeB int64
	if statR.OK {
		if info, _ := statR.Value.(core.FsFileInfo); info != nil {
			modTime = info.ModTime()
			sizeB = info.Size()
		}
	}
	rec := parseDoc(input.Slug, content, modTime, sizeB)

	s.core.ACTION(DocChanged{
		Kind:  "created",
		Slug:  input.Slug,
		After: rec,
		At:    now,
	})

	return core.Ok(toRow(rec, now))
}

// Save updates the body and metadata of an existing document. Requires a
// valid composite token (IfMatch + IfMatchHash) from a prior Get — blind
// writes are rejected. Fires EventUpdated on success.
// Rejects if the session is locked (documents.session.locked).
//
// Usage example:
//
//	r := svc.Save(documents.SaveInput{
//	    Slug: "notes", Body: "# Updated\n", IfMatch: t, IfMatchHash: h,
//	})
func (s *Service) Save(input SaveInput) core.Result {
	if fail, ok := s.assertUnlocked("documents.Save"); !ok {
		return fail
	}
	if err := validateSlug(input.Slug); err != nil {
		return core.Fail(core.E("documents.Save", "documents.slug.invalid", err))
	}
	if len(input.Body) > MaxBodyBytes {
		return core.Fail(core.E("documents.Save", "documents.body.too_large", nil))
	}
	if input.IfMatch.IsZero() || input.IfMatchHash == "" {
		return core.Fail(core.E("documents.Save", "documents.save.missing_if_match", nil))
	}

	dirR := docsDir()
	if !dirR.OK {
		return core.Fail(core.E("documents.Save", dirR.Error(), nil))
	}
	dir := dirR.Value.(string)
	fpath := core.PathJoin(dir, input.Slug+".md")

	// Snapshot existing frontmatter for preservation of fields not in
	// SaveInput. paths.ReadVersion is the single-stat+read primitive —
	// using it here matches the primitive's view of "current state"
	// so the IfMtime + IfMatchHash conditions below race-free against
	// the actual disk state.
	rdR := paths.ReadVersion(fpath)
	if !rdR.OK {
		return core.Fail(core.E("documents.Save", "load failed: "+rdR.Error(), nil))
	}
	rd, _ := rdR.Value.(paths.ReadOutput)
	if rd.Mtime.IsZero() {
		return core.Fail(core.E("documents.Save", "documents.save.not_found", nil))
	}
	raw := rd.Body
	existingFm, _ := splitFrontmatter(raw)
	var existing docFrontmatter
	_ = yaml.Unmarshal(existingFm, &existing)

	state := input.State
	if state == "" {
		state = existing.State
	}
	switch state {
	case "draft", "ready", "final", "live":
	default:
		state = "draft"
	}
	tags := input.Tags
	if tags == nil {
		tags = existing.Tags
	}
	related := input.Related
	if related == nil {
		related = existing.Related
	}

	now := core.Now()
	fm, err := composeFrontmatter(state, existing.Author, normaliseTags(tags), related, now)
	if err != nil {
		return core.Fail(core.E("documents.Save", "frontmatter compose failed", err))
	}

	content := buildFileContent(fm, input.Body)
	// B.2 PILOT cutover: route through paths.AtomicWriteWithVersion
	// with the documents-v2 composite (IfMtime + IfMatchHash). The
	// primitive holds the file lock for the read-check-write sequence,
	// so the snapshot read above + this write are TOCTOU-safe under
	// concurrent writers.
	if err := atomicWriteFile(fpath, content, input.IfMatch, input.IfMatchHash); err != nil {
		return core.Fail(core.E("documents.Save", err.Error(), err))
	}

	statR := core.Stat(fpath)
	var modTime core.Time
	var sizeB int64
	if statR.OK {
		if info, _ := statR.Value.(core.FsFileInfo); info != nil {
			modTime = info.ModTime()
			sizeB = info.Size()
		}
	}
	rec := parseDoc(input.Slug, content, modTime, sizeB)
	before := parseDoc(input.Slug, raw, input.IfMatch, int64(len(raw)))

	// Suppress DocChanged emission if the session locked mid-write.
	// Best-effort live-read mirror of v1's locked.Load() guard.
	if s.sessionUnlocked() {
		s.core.ACTION(DocChanged{
			Kind:   "updated",
			Slug:   input.Slug,
			Before: before,
			After:  rec,
			At:     now,
		})
	}

	return core.Ok(toRow(rec, now))
}

// Delete hard-deletes a document. Requires a valid composite token from a
// recent Get. Fires EventDeleted on success.
// Rejects if the session is locked (documents.session.locked).
//
// Usage example:
//
//	r := svc.Delete(documents.DeleteInput{
//	    Slug: "old-draft", IfMatch: t, IfMatchHash: h,
//	})
func (s *Service) Delete(input DeleteInput) core.Result {
	if fail, ok := s.assertUnlocked("documents.Delete"); !ok {
		return fail
	}
	if err := validateSlug(input.Slug); err != nil {
		return core.Fail(core.E("documents.Delete", "documents.slug.invalid", err))
	}
	if input.IfMatch.IsZero() || input.IfMatchHash == "" {
		return core.Fail(core.E("documents.Delete", "documents.save.missing_if_match", nil))
	}

	dirR := docsDir()
	if !dirR.OK {
		return core.Fail(core.E("documents.Delete", dirR.Error(), nil))
	}
	dir := dirR.Value.(string)
	fpath := core.PathJoin(dir, input.Slug+".md")

	// Lock + read + check + unlink under one critical section. The
	// primitive's WithFileLock holds for the inner closure; concurrent
	// Delete + Save against the same slug now serialise correctly.
	var before DocRecord
	lockR := paths.WithFileLock(fpath, 0, func() core.Result {
		rdR := paths.ReadVersion(fpath)
		if !rdR.OK {
			return core.Fail(core.E("documents.Delete", "read failed: "+rdR.Error(), nil))
		}
		rd, _ := rdR.Value.(paths.ReadOutput)
		if rd.Mtime.IsZero() {
			return core.Fail(core.E("documents.Delete", "documents.delete.not_found", nil))
		}
		if rd.BodyHash != input.IfMatchHash {
			return core.Fail(core.E("documents.Delete", "documents.delete.conflict", nil))
		}
		before = parseDoc(input.Slug, rd.Body, rd.Mtime, int64(len(rd.Body)))
		if r := core.Remove(fpath); !r.OK {
			return core.Fail(core.E("documents.Delete", "remove failed: "+r.Error(), nil))
		}
		return core.Ok(nil)
	})
	if !lockR.OK {
		return lockR
	}

	now := core.Now()
	// Suppress DocChanged emission if the session locked mid-write.
	// Best-effort live-read mirror of v1's locked.Load() guard.
	if s.sessionUnlocked() {
		s.core.ACTION(DocChanged{
			Kind:   "deleted",
			Slug:   input.Slug,
			Before: before,
			At:     now,
		})
	}

	return core.Ok(nil)
}
