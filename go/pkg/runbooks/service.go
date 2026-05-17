// SPDX-Licence-Identifier: EUPL-1.2

// Service — Core integration for the runbooks surface. Reads and
// writes Trix-style markdown files from ~/Lethean/runbooks/*.md.
// No queue, no orm — filesystem is the persistence layer.
//
// On first use (empty directory), seed runbooks are written matching
// the frontend fixture data. Seeding is idempotent — skipped when any
// *.md file already exists.
//
// All I/O uses CoreGO wrappers. Banned stdlib imports: os,
// path/filepath, strings, encoding/json, fmt, log, errors.

package runbooks

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/paths"
	"gopkg.in/yaml.v3"
)

// Freshness thresholds. Runbooks rehearsed within 30 days are "fresh";
// between 30 and 90 days are "aging"; beyond 90 days (or never) are
// "stale".
const (
	FreshThreshold = 30 * 24 * core.Hour
	AgingThreshold = 90 * 24 * core.Hour
)

// Service owns the runbooks surface.
//
// Usage example:
//
//	svc := runbooks.NewService(c)
type Service struct {
	core *core.Core
}

// NewService constructs the runbooks service against a Core container.
// Wired via core.WithName("runbooks", runbooks.Register) in app.go.
//
// Usage example:
//
//	svc := runbooks.NewService(c)
func NewService(c *core.Core) *Service {
	return &Service{core: c}
}

// Register constructs the runbooks service for Core registration.
//
// Usage example:
//
//	core.New(core.WithName("runbooks", runbooks.Register))
func Register(c *core.Core) core.Result {
	return core.Ok(NewService(c))
}

// ServiceName labels the binding namespace exposed to JS.
// Wails binds methods as "Runbooks.List()" etc.
func (s *Service) ServiceName() string { return "Runbooks" }

// OnStart seeds the runbook library when the directory is empty.
// Safe to call when core is nil (unit test paths).
func (s *Service) OnStart() core.Result {
	dir := runbooksDir()
	if !dir.OK {
		return core.Ok(nil) // best-effort — non-fatal on startup
	}
	dirPath := dir.Value.(string)
	if countMdFiles(dirPath) > 0 {
		return core.Ok(nil) // already seeded
	}
	seedAll(dirPath)
	return core.Ok(nil)
}

// runbooksDir resolves ~/Lethean/runbooks/ and creates it if missing.
// Mode 0o700 (Cerberus #1487 PR-1): runbooks contain operational
// secrets (recovery procedures, credential-rotation steps) — owner-only.
func runbooksDir() core.Result {
	root := paths.Root()
	if !root.OK {
		return root
	}
	dir := core.PathJoin(root.Value.(string), "runbooks")
	if r := core.MkdirAll(dir, 0o700); !r.OK {
		return r
	}
	return core.Ok(dir)
}

// countMdFiles returns the number of *.md files in dir.
func countMdFiles(dir string) int {
	r := core.ReadDir(core.DirFS(dir), ".")
	if !r.OK {
		return 0
	}
	entries, _ := r.Value.([]core.FsDirEntry)
	n := 0
	for _, e := range entries {
		if !e.IsDir() {
			nm := e.Name()
			if len(nm) >= 4 && nm[len(nm)-3:] == ".md" {
				n++
			}
		}
	}
	return n
}

// parseRecord splits a Trix file (slug + raw bytes) into frontmatter
// + body and decodes the YAML header into a RunbookRecord.
func parseRecord(slug string, raw []byte) (RunbookRecord, error) {
	content := raw

	// Skip the opening ---\n
	open := []byte("---\n")
	if len(content) >= len(open) {
		match := true
		for i, b := range open {
			if content[i] != b {
				match = false
				break
			}
		}
		if match {
			content = content[len(open):]
		}
	}

	// Find the closing ---
	closeIdx := -1
	for i := 0; i < len(content)-3; i++ {
		if content[i] == '-' && content[i+1] == '-' && content[i+2] == '-' {
			if i == 0 || content[i-1] == '\n' {
				closeIdx = i
				break
			}
		}
	}

	var rec RunbookRecord
	var body string
	if closeIdx < 0 {
		if err := yaml.Unmarshal(content, &rec); err != nil {
			return RunbookRecord{}, core.E("runbooks.parse", "yaml unmarshal", err)
		}
	} else {
		fm := content[:closeIdx]
		if err := yaml.Unmarshal(fm, &rec); err != nil {
			return RunbookRecord{}, core.E("runbooks.parse", "yaml unmarshal", err)
		}
		rest := content[closeIdx+3:]
		if len(rest) > 0 && rest[0] == '\n' {
			rest = rest[1:]
		}
		body = string(rest)
	}
	rec.Slug = slug
	rec.Body = body
	return rec, nil
}

// marshalRecord serialises a RunbookRecord to the Trix file format.
func marshalRecord(r RunbookRecord) ([]byte, error) {
	fm, err := yaml.Marshal(r)
	if err != nil {
		return nil, core.E("runbooks.marshal", "yaml marshal", err)
	}
	var out []byte
	out = append(out, []byte("---\n")...)
	out = append(out, fm...)
	out = append(out, []byte("---\n")...)
	if r.Body != "" {
		out = append(out, []byte(r.Body)...)
	}
	return out, nil
}

// relativeTime returns a human-readable age string ("2 d", "3 w",
// "4 mo", "never") — without "ago" suffix, matching the runbooks
// fixture's "last" field shape.
func relativeTime(t core.Time, now core.Time) string {
	if t.IsZero() {
		return "never"
	}
	diff := now.Sub(t)
	if diff < 0 {
		diff = -diff
	}
	const hour = core.Hour
	const day = 24 * hour
	const week = 7 * day
	const month = 30 * day

	switch {
	case diff < day:
		return "< 1 d"
	case diff < 2*week:
		return core.Sprintf("%d d", int(diff/day))
	case diff < month:
		return core.Sprintf("%d w", int(diff/week))
	default:
		return core.Sprintf("%d mo", int(diff/month))
	}
}

// computeHealth returns "fresh", "aging", or "stale" based on the
// last-rehearsed timestamp.
func computeHealth(r RunbookRecord, now core.Time) string {
	if r.LastRehearsed.IsZero() {
		return "stale"
	}
	diff := now.Sub(r.LastRehearsed)
	switch {
	case diff < FreshThreshold:
		return "fresh"
	case diff < AgingThreshold:
		return "aging"
	default:
		return "stale"
	}
}

// toEntry converts a RunbookRecord to the wire type.
func toEntry(r RunbookRecord, now core.Time) RunbookEntry {
	return RunbookEntry{
		ID:     r.ID,
		Title:  r.Title,
		Area:   r.Area,
		Last:   relativeTime(r.LastRehearsed, now),
		Health: computeHealth(r, now),
	}
}

// containsCI returns true when needle appears (case-insensitive) in
// the haystack byte slice. Avoids the banned `strings` package by
// lowercasing both sides via toLower.
func containsCI(haystack, needle string) bool {
	if needle == "" {
		return true
	}
	h := toLower(haystack)
	n := toLower(needle)
	if len(n) > len(h) {
		return false
	}
	for i := 0; i <= len(h)-len(n); i++ {
		match := true
		for j := 0; j < len(n); j++ {
			if h[i+j] != n[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// toLower lowercases ASCII only — sufficient for runbook search terms
// (titles, area slugs, IDs, tags). Non-ASCII passes through unchanged.
func toLower(s string) string {
	b := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		} else {
			b[i] = c
		}
	}
	return string(b)
}

// matchSearch returns true when the runbook matches the search query
// (case-insensitive substring across title, area, id, and tags).
func matchSearch(r RunbookRecord, q string) bool {
	if q == "" {
		return true
	}
	if containsCI(r.Title, q) || containsCI(r.Area, q) || containsCI(r.ID, q) {
		return true
	}
	for _, tag := range r.Tags {
		if containsCI(tag, q) {
			return true
		}
	}
	return false
}

// scanAll reads all *.md files in ~/Lethean/runbooks/ and returns
// RunbookRecord values with Body set. Returns empty slice (not error)
// when the directory is missing or empty.
func scanAll() ([]RunbookRecord, error) {
	dir := runbooksDir()
	if !dir.OK {
		return nil, core.E("runbooks.scanAll", dir.Error(), nil)
	}
	dirPath := dir.Value.(string)

	entriesR := core.ReadDir(core.DirFS(dirPath), ".")
	if !entriesR.OK {
		return nil, nil
	}
	entries, _ := entriesR.Value.([]core.FsDirEntry)

	var records []RunbookRecord
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if len(name) < 4 || name[len(name)-3:] != ".md" {
			continue
		}
		slug := name[:len(name)-3]
		fpath := core.PathJoin(dirPath, name)
		raw := core.ReadFile(fpath)
		if !raw.OK {
			continue
		}
		rec, err := parseRecord(slug, raw.Value.([]byte))
		if err != nil {
			continue
		}
		records = append(records, rec)
	}
	return records, nil
}

// loadOne finds a runbook by ID or slug and returns the full record
// with Body populated.
func loadOne(id, slug string) (RunbookRecord, string, error) {
	records, err := scanAll()
	if err != nil {
		return RunbookRecord{}, "", err
	}
	for _, r := range records {
		if (id != "" && r.ID == id) || (slug != "" && r.Slug == slug) {
			dir := runbooksDir()
			if !dir.OK {
				return RunbookRecord{}, "", core.E("runbooks.loadOne", dir.Error(), nil)
			}
			return r, dir.Value.(string), nil
		}
	}
	label := id
	if label == "" {
		label = slug
	}
	return RunbookRecord{}, "", core.E("runbooks.loadOne", "not found: "+label, nil)
}

// writeRecord serialises a RunbookRecord back to disk and writes it
// via paths.AtomicWriteWithVersion (Cascade W3, RFC §B.3 row 9).
//
// ifVersion is the optimistic-lock anchor — pass the Version the
// caller observed on disk (via parseRecord through loadOne), or 0 for
// first-writes / legacy-file upgrades. writeRecord stamps the next
// monotonic version (ifVersion+1) into the marshalled frontmatter so
// subsequent reads see version=1,2,3... monotonically.
//
// Cerberus #1486: slug lands directly in the filename; validate before
// the join even though loadOne paths route through it.
// Cerberus #1487 PR-1: 0o600 — owner-only at rest (applied by the
// primitive's atomic-rename path).
//
// Return shape (Mantis #1544 gating, inherited from W1+W2): on the
// stale-write path the function returns
// core.Fail(paths.ConflictEnvelope{...}) so the Wails-marshalled
// Result.Value carries the lowercase-json shape
// (`{code, current_version, current_hash}`) that
// conflict-dispatch.ts extractEnvelope pattern-matches on. The
// per-service Code is "runbooks.update.conflict" — the frontend
// scopes its toast + reload-listener on this exact string.
//
// Audit emission is automatic via paths.AuditModeForPath —
// runbooks/* paths route through AuditModeBatch per RFC §6.1.
//
// Usage example:
//
//	if wr := writeRecord(rec, dirPath, prior.Version); !wr.OK {
//	    return wr
//	}
func writeRecord(r RunbookRecord, dirPath string, ifVersion int) core.Result {
	if err := paths.IsValidID(r.Slug); err != nil {
		return core.Fail(err)
	}
	// Stamp the next monotonic version. ifVersion=0 (Create / legacy
	// upgrade) yields version=1; subsequent writes increment.
	nextVersion := ifVersion + 1
	if nextVersion < 1 {
		nextVersion = 1
	}
	r.Version = nextVersion
	raw, err := marshalRecord(r)
	if err != nil {
		return core.Fail(core.E("runbooks.writeRecord", "marshal", err))
	}
	target := core.PathJoin(dirPath, r.Slug+".md")
	res := paths.AtomicWriteWithVersion(target, paths.WriteInput{
		Body:      raw,
		IfVersion: ifVersion,
	})
	if res.OK {
		return res
	}
	if stale, ok := paths.VersionStaleFromError(res.Value); ok {
		return core.Fail(paths.NewConflictEnvelope(
			"runbooks.update.conflict", stale))
	}
	return core.Fail(core.E("runbooks.writeRecord",
		"write failed: "+res.Error(), nil))
}

// fireEvent publishes a runbook event on the Core ACTION bus.
func (s *Service) fireEvent(name string, entry RunbookEntry) {
	if s == nil || s.core == nil {
		return
	}
	s.core.ACTION(RunbookEvent{
		EventName: name,
		Entry:     entry,
		At:        core.Now().UTC(),
	})
}

// buildCounts tallies health across all records.
func buildCounts(records []RunbookRecord, now core.Time) (fresh, aging, stale int) {
	for _, r := range records {
		switch computeHealth(r, now) {
		case "fresh":
			fresh++
		case "aging":
			aging++
		default:
			stale++
		}
	}
	return
}

// seedAll writes the seven default runbooks derived from the frontend
// fixture data. Called once on startup when ~/Lethean/runbooks/ is empty.
func seedAll(dirPath string) {
	now := core.Now().UTC()
	// Build seed records matching the fixture data in runbooks.ts.
	seeds := []struct {
		rec          RunbookRecord
		rehearsedAgo core.Duration // negative = in past
	}{
		{RunbookRecord{ID: "R-01", Title: "Rotate runtime API keys", Area: "auth", Tags: []string{"auth", "api-keys", "security"}, Slug: "rotate-runtime-api-keys", CreatedAt: now}, 2 * 24 * core.Hour},
		{RunbookRecord{ID: "R-02", Title: "Recover from corrupt model directory", Area: "runtime", Tags: []string{"runtime", "model", "recovery"}, Slug: "recover-from-corrupt-model-directory", CreatedAt: now}, 3 * 7 * 24 * core.Hour},
		{RunbookRecord{ID: "R-03", Title: "Rollback bad deploy · production", Area: "deploy", Tags: []string{"deploy", "rollback", "production"}, Slug: "rollback-bad-deploy-production", CreatedAt: now}, 5 * 24 * core.Hour},
		{RunbookRecord{ID: "R-04", Title: "Reset DNS · cache poisoning incident", Area: "network", Tags: []string{"network", "dns", "security"}, Slug: "reset-dns-cache-poisoning-incident", CreatedAt: now}, 2 * 7 * 24 * core.Hour},
		{RunbookRecord{ID: "R-05", Title: "Drain a stuck Postfix queue", Area: "mail", Tags: []string{"mail", "postfix", "queue"}, Slug: "drain-a-stuck-postfix-queue", CreatedAt: now}, 4 * 30 * 24 * core.Hour},
		{RunbookRecord{ID: "R-06", Title: "Trigger emergency model unload", Area: "runtime", Tags: []string{"runtime", "model", "emergency"}, Slug: "trigger-emergency-model-unload", CreatedAt: now}, 30 * 24 * core.Hour},
		{RunbookRecord{ID: "R-07", Title: "Restore tray-app from corrupt config", Area: "client", Tags: []string{"client", "config", "recovery"}, Slug: "restore-tray-app-from-corrupt-config", CreatedAt: now}, 6 * 30 * 24 * core.Hour},
	}
	for _, s := range seeds {
		rec := s.rec
		rec.LastRehearsed = now.Add(-s.rehearsedAgo)
		rec.Body = "# " + rec.Title + "\n\nThis is a seed runbook. " +
			"Replace this with your organisation's actual procedure.\n"
		// Stamp the seed at version 1 so subsequent edits see the
		// monotonic anchor immediately (Cascade W3, RFC §B.3 row 9).
		rec.Version = 1
		raw, err := marshalRecord(rec)
		if err != nil {
			continue
		}
		target := core.PathJoin(dirPath, rec.Slug+".md")
		// Cascade W3 (RFC §B.3 row 9) — seed-path replaces the prior
		// bare `core.WriteFile` with an unconditional first-write
		// through paths.AtomicWriteWithVersion. Per RFC §3.2 lazy-
		// migration semantics, an empty WriteInput (no IfVersion /
		// IfMtime / IfMatchHash) treats this as a legitimate
		// unconditional first-write. seedAll() is invoked once on
		// first start when the runbooks directory is empty (see
		// OnStart + List entry-points) so there's no concurrent
		// writer to race against.
		// 0o600 (Cerberus #1487 PR-1): runbooks carry operational
		// secrets; even seed content uses the production mode,
		// applied inside the primitive's atomic-rename path.
		// Mantis #1572 (Cerberus #12 F5): log on !OK rather than
		// silently swallow — seedAll is best-effort (called from
		// OnStart, single caller) so an individual seed failure
		// shouldn't abort the rest, but it MUST surface.
		if wr := paths.AtomicWriteWithVersion(target, paths.WriteInput{
			Body: raw,
		}); !wr.OK {
			core.Print(core.Stderr(),
				"runbooks: seedAll write failed for %s: %s\n",
				rec.Slug, wr.Error())
		}
	}
}
