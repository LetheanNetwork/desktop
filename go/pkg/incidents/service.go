// SPDX-Licence-Identifier: EUPL-1.2

// Service — Core integration for the incidents surface. Reads and
// writes Trix-style markdown files from ~/Lethean/incidents/{YYYY}/{MM}/.
// No queue, no orm — filesystem is the persistence layer.
//
// Lifecycle:
//   - Register(c)   wires the service into the Core container
//   - ServiceName() returns "Incidents" for the Wails namespace
//
// All I/O uses CoreGO wrappers (core.ReadFile / core.WriteFile /
// core.ReadDir / core.DirFS / core.MkdirAll / core.PathJoin).
// Banned stdlib imports: os, path/filepath, strings, encoding/json,
// fmt, log, errors.

package incidents

import (
	"sync"
	"sync/atomic"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/paths"
	"dappco.re/lthn/desktop/pkg/recordfile"
	"gopkg.in/yaml.v3"
)

// SessionGate is the minimal consumer-defined interface satisfied by
// *account.Service. Live-read at every gate check — no cached bool, no
// subscribe/event bus (RFC.stage-e-unlockgate v2 §1.1 — Pushback 2
// CONFIRMED by Cerberus #27, B.2 inherits the same shape per Cerberus
// #28 confirm). When the returned slice is empty the session is locked;
// when non-empty at least one Lethean account is unlocked and writes
// may proceed.
//
// Wired in cmd/lthn/app.go (Mantis #1613 B.3, deferred to that lane):
//
//	incidentsSvc.SetSessionGate(accountSvc)
//
// AX-8 compliance: this interface is defined in the consumer
// (incidents) and satisfied by the producer (*account.Service). No
// pkg/account import lands in pkg/incidents. Each writer pkg defines
// its OWN interface (consumer-defines per Pushback 1 / H#147+H#148+H#149
// canonical pattern) — no shared types package.
type SessionGate interface {
	UnlockedAccountIDs() []string
}

// Service owns the incidents surface.
//
// Usage example:
//
//	svc := incidents.NewService(c)
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
	// (§2.2 / Cerberus #27 Q2, B.2 mirror confirmed by Cerberus #28).
	// CompareAndSwap-on-first-hit emits core.Warn exactly once per
	// Service instance. Uses stdlib sync/atomic.Bool to mirror H#147
	// documents pattern (codebase convention — not core.AtomicBool).
	nilWarned atomic.Bool
}

// NewService constructs the incidents service against a Core container.
// Wired via core.WithName("incidents", incidents.Register) in app.go.
//
// Usage example:
//
//	svc := incidents.NewService(c)
func NewService(c *core.Core) *Service {
	return &Service{core: c}
}

// Register constructs the incidents service for Core registration.
//
// Usage example:
//
//	core.New(core.WithName("incidents", incidents.Register))
func Register(c *core.Core) core.Result {
	return core.Ok(NewService(c))
}

// ServiceName labels the binding namespace exposed to JS.
// Wails binds methods as "Incidents.List()" etc.
func (s *Service) ServiceName() string { return "Incidents" }

// incidentsDir resolves ~/Lethean/incidents/ and creates it if missing.
// Mode 0o700 (Cerberus #1487 PR-1): post-mortem bodies + service names
// + people-attribution are sensitive — owner-only at rest.
func incidentsDir() core.Result {
	root := paths.Root()
	if !root.OK {
		return root
	}
	dir := core.PathJoin(root.Value.(string), "incidents")
	if r := core.MkdirAll(dir, 0o700); !r.OK {
		return r
	}
	return core.Ok(dir)
}

// yearMonthDir resolves ~/Lethean/incidents/{YYYY}/{MM}/ and creates it.
// Mode 0o700 (Cerberus #1487 PR-1): mirrors the parent.
func yearMonthDir(t core.Time) core.Result {
	base := incidentsDir()
	if !base.OK {
		return base
	}
	yr := core.TimeFormat(t, "2006")
	mo := core.TimeFormat(t, "01")
	dir := core.PathJoin(base.Value.(string), yr, mo)
	if r := core.MkdirAll(dir, 0o700); !r.OK {
		return r
	}
	return core.Ok(dir)
}

// filePath returns the absolute path for a given time + id.
func filePath(t core.Time, id string) core.Result {
	dir := yearMonthDir(t)
	if !dir.OK {
		return dir
	}
	return core.Ok(core.PathJoin(dir.Value.(string), id+".md"))
}

// parseRecord splits a Trix file into frontmatter + body via the
// shared recordfile.Split helper and decodes the YAML header into an
// IncidentRecord. The body is stored in the PostMortem field.
func parseRecord(raw []byte) (IncidentRecord, error) {
	fm, body := recordfile.Split(raw)
	var rec IncidentRecord
	if err := yaml.Unmarshal(fm, &rec); err != nil {
		return IncidentRecord{}, core.E("incidents.parse", "yaml unmarshal", err)
	}
	rec.PostMortem = string(body)
	return rec, nil
}

// marshalRecord serialises an IncidentRecord to the Trix file format
// via the shared recordfile.Stitch helper: YAML frontmatter block
// followed by the post-mortem markdown body.
func marshalRecord(r IncidentRecord) ([]byte, error) {
	fm, err := yaml.Marshal(r)
	if err != nil {
		return nil, core.E("incidents.marshal", "yaml marshal", err)
	}
	return recordfile.Stitch(fm, []byte(r.PostMortem)), nil
}

// relativeTime returns a human-readable relative duration string
// matching the fixture data shape: "now", "X min ago", "X h ago",
// "X d ago", "X w ago", "X mo ago".
func relativeTime(t core.Time, now core.Time) string {
	if t.IsZero() {
		return ""
	}
	diff := core.Since(t)
	if diff < 0 {
		diff = -diff
	}
	const minute = core.Minute
	const hour = core.Hour
	const day = 24 * core.Hour
	const week = 7 * day
	const month = 30 * day
	// Switch to "X w ago" after 2 weeks (14 days); below that use "X d ago".
	// This matches the fixture data: "11 d ago" (< 2 weeks), "2 w ago" (≥ 2 weeks).
	const weekThreshold = 2 * week

	switch {
	case diff < minute:
		return "now"
	case diff < hour:
		return core.Sprintf("%d min ago", int(diff/minute))
	case diff < day:
		return core.Sprintf("%d h ago", int(diff/hour))
	case diff < weekThreshold:
		return core.Sprintf("%d d ago", int(diff/day))
	case diff < month:
		return core.Sprintf("%d w ago", int(diff/week))
	default:
		return core.Sprintf("%d mo ago", int(diff/month))
	}
}

// durString converts a resolved incident's DurMinutes to a display
// string ("42 min", "1 h 02", "2 h 14").
func durString(minutes int) string {
	if minutes <= 0 {
		return ""
	}
	if minutes < 60 {
		return core.Sprintf("%d min", minutes)
	}
	h := minutes / 60
	m := minutes % 60
	return core.Sprintf("%d h %02d", h, m)
}

// toEntry converts an IncidentRecord to the wire type. now is passed
// in so callers control the clock reference (test-friendly).
func toEntry(r IncidentRecord, now core.Time) IncidentEntry {
	return IncidentEntry{
		ID:       r.ID,
		TS:       relativeTime(r.CreatedAt, now),
		Sev:      r.Sev,
		State:    r.State,
		Title:    r.Title,
		Svc:      r.Svc,
		Who:      r.Who,
		Comments: r.Comments,
		Dur:      durString(r.DurMinutes),
	}
}

// list90 scans the last two YYYY/MM directories in ~/Lethean/incidents/
// and returns all parseable IncidentRecord values. Malformed files are
// skipped with a warning. The body (PostMortem) is NOT loaded — use
// loadOne for full records.
func list90() ([]IncidentRecord, error) {
	base := incidentsDir()
	if !base.OK {
		return nil, core.E("incidents.list90", base.Error(), nil)
	}
	baseDir := base.Value.(string)

	now := core.Now()
	yrNow := core.TimeFormat(now, "2006")
	moNow := core.TimeFormat(now, "01")

	// Build the month-directory candidates: current + previous month.
	prev := now.Add(-32 * 24 * core.Hour)
	yrPrev := core.TimeFormat(prev, "2006")
	moPrev := core.TimeFormat(prev, "01")

	candidates := []string{
		core.PathJoin(baseDir, yrNow, moNow),
		core.PathJoin(baseDir, yrPrev, moPrev),
	}

	var records []IncidentRecord
	for _, dir := range candidates {
		entriesR := core.ReadDir(core.DirFS(dir), ".")
		if !entriesR.OK {
			// Directory may not exist yet — normal for a new install.
			continue
		}
		entries, _ := entriesR.Value.([]core.FsDirEntry)
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			// Only process .md files.
			if len(name) < 4 || name[len(name)-3:] != ".md" {
				continue
			}
			fpath := core.PathJoin(dir, name)
			raw := core.ReadFile(fpath)
			if !raw.OK {
				continue
			}
			rec, err := parseRecord(raw.Value.([]byte))
			if err != nil {
				continue
			}
			// Strip the body for list performance — Get() loads it.
			rec.PostMortem = ""
			records = append(records, rec)
		}
	}
	return records, nil
}

// loadOne finds and fully parses the incident file for the given ID,
// returning the record with the PostMortem body populated.
// Cerberus #1486: id is validated before any directory walk so a
// traversal payload cannot inflate ReadDir attempts across the tree.
func loadOne(id string) (IncidentRecord, string, error) {
	if err := paths.IsValidID(id); err != nil {
		return IncidentRecord{}, "", err
	}
	base := incidentsDir()
	if !base.OK {
		return IncidentRecord{}, "", core.E("incidents.loadOne", base.Error(), nil)
	}
	baseDir := base.Value.(string)

	// Walk all YYYY/MM directories under the base.
	yearsR := core.ReadDir(core.DirFS(baseDir), ".")
	if !yearsR.OK {
		return IncidentRecord{}, "", core.E("incidents.loadOne", yearsR.Error(), nil)
	}
	years, _ := yearsR.Value.([]core.FsDirEntry)
	for _, yEntry := range years {
		if !yEntry.IsDir() {
			continue
		}
		yrPath := core.PathJoin(baseDir, yEntry.Name())
		monthsR := core.ReadDir(core.DirFS(yrPath), ".")
		if !monthsR.OK {
			continue
		}
		months, _ := monthsR.Value.([]core.FsDirEntry)
		for _, mEntry := range months {
			if !mEntry.IsDir() {
				continue
			}
			dirPath := core.PathJoin(yrPath, mEntry.Name())
			// Cerberus #1486 belt-and-braces (Mantis #1607, forward-arc
			// from H#85): WithinDir check after the join. IsValidID at
			// the top of loadOne is the cheap shape gate; JoinAndCheck
			// catches cousin-validator drift if a future regression
			// loosens that shape or a non-wails caller bypasses it.
			target, jerr := paths.JoinAndCheck(dirPath, id+".md")
			if jerr != nil {
				return IncidentRecord{}, "", jerr
			}
			raw := core.ReadFile(target)
			if !raw.OK {
				continue
			}
			rec, err := parseRecord(raw.Value.([]byte))
			if err != nil {
				return IncidentRecord{}, "", err
			}
			return rec, dirPath, nil
		}
	}
	return IncidentRecord{}, "", core.E("incidents.loadOne", "incident not found: "+id, nil)
}

// writeRecord serialises a record back to disk and writes it via
// paths.AtomicWriteWithVersion (Cascade W3, RFC §B.3 row 8).
//
// ifVersion is the optimistic-lock anchor — pass the Version the
// caller observed on disk (via parseRecord through loadOne), or 0 for
// first-writes / legacy-file upgrades. writeRecord stamps the next
// monotonic version (ifVersion+1) into the marshalled frontmatter so
// subsequent reads see version=1,2,3... monotonically.
//
// Cerberus #1486: rec.ID lands in the filename — validate before join.
// Cerberus #1487 PR-1: 0o600 — owner-only at rest (applied by the
// primitive's atomic-rename path).
//
// Return shape (Mantis #1544 gating, inherited from W1+W2): on the
// stale-write path the function returns
// core.Fail(paths.ConflictEnvelope{...}) so the Wails-marshalled
// Result.Value carries the lowercase-json shape
// (`{code, current_version, current_hash}`) that
// conflict-dispatch.ts extractEnvelope pattern-matches on. The
// per-service Code is "incidents.update.conflict" — the frontend
// scopes its toast + reload-listener on this exact string.
//
// Audit emission is automatic via paths.AuditModeForPath —
// incidents/* paths route through AuditModeBatch per RFC §6.1.
//
// Usage example:
//
//	if wr := writeRecord(rec, dirPath, prior.Version); !wr.OK {
//	    return wr
//	}
func writeRecord(r IncidentRecord, dirPath string, ifVersion int) core.Result {
	if err := paths.IsValidID(r.ID); err != nil {
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
		return core.Fail(core.E("incidents.writeRecord", "marshal", err))
	}
	// Cerberus #1486 belt-and-braces (Mantis #1607, forward-arc from
	// H#85): WithinDir check after the join. IsValidID above is the
	// cheap shape gate; JoinAndCheck refuses any path that resolves
	// outside dirPath even if cousin-validator drift weakens IsValidID.
	target, jerr := paths.JoinAndCheck(dirPath, r.ID+".md")
	if jerr != nil {
		return core.Fail(jerr)
	}
	res := paths.AtomicWriteWithVersion(target, paths.WriteInput{
		Body:      raw,
		IfVersion: ifVersion,
	})
	if res.OK {
		return res
	}
	if stale, ok := paths.VersionStaleFromError(res.Value); ok {
		return core.Fail(paths.NewConflictEnvelope(
			"incidents.update.conflict", stale))
	}
	return core.Fail(core.E("incidents.writeRecord",
		"write failed: "+res.Error(), nil))
}

// countMonthFiles returns the number of .md files in a month directory.
// Used to derive the NNN suffix for a new incident ID.
func countMonthFiles(dirPath string) int {
	r := core.ReadDir(core.DirFS(dirPath), ".")
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

// IncidentEvent is the Core ACTION bus payload for incident lifecycle
// transitions.
//
// Usage example:
//
//	c.RegisterAction(func(c *core.Core, msg core.Message) core.Result {
//	    if ev, ok := msg.(incidents.IncidentEvent); ok {
//	        _ = ev.Entry.ID
//	    }
//	    return core.Result{OK: true}
//	})
type IncidentEvent struct {
	// EventName is the event constant that triggered this message
	// (EventOpened or EventTransitioned).
	EventName string `json:"event"`

	// Entry is the incident state after the change.
	Entry IncidentEntry `json:"entry"`

	// At is the event timestamp.
	At core.Time `json:"at"`
}

// fireEvent publishes an incident event on the Core ACTION bus.
func (s *Service) fireEvent(name string, entry IncidentEntry) {
	if s == nil || s.core == nil {
		return
	}
	s.core.ACTION(IncidentEvent{
		EventName: name,
		Entry:     entry,
		At:        core.Now().UTC(),
	})
}

// SetSessionGate wires the live-read session source. Called by
// cmd/lthn/app.go post-construction (Mantis #1613 B.3) once
// *account.Service exists.
//
// Mirrors the H#147 documents.SetSessionGate setter pattern. Live-read
// on every gate check — no event-bus reliability concerns, no cache
// coherence concerns (RFC.stage-e-unlockgate v2 §1.1).
//
// Usage example:
//
//	incidentsSvc.SetSessionGate(accountSvc)
func (s *Service) SetSessionGate(g SessionGate) {
	s.gateMu.Lock()
	s.gate = g
	s.gateMu.Unlock()
}

// Stop nils the SessionGate reference so a draining Service
// fails-closed on any late-arriving write (Cerberus #28 ADD-5 / H#147
// documents.Stop mirror). Read-only methods (List, Get) continue to
// function — Stop only severs the write gate.
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
// Fail-safe on nil gate (§2.2 / Cerberus #28 Q2): when SetSessionGate
// has not yet wired the gate (or Stop has nilled it), the gate fails
// LOCKED rather than panicking. The first nil-hit per Service
// instance emits a one-shot core.Warn via CompareAndSwap so
// wire-ordering bugs surface in dev without log spam in production.
//
// Usage example:
//
//	if fail, ok := s.assertUnlocked("incidents.Create"); !ok {
//	    return fail
//	}
func (s *Service) assertUnlocked(scope string) (core.Result, bool) {
	s.gateMu.RLock()
	g := s.gate
	s.gateMu.RUnlock()
	if g == nil {
		if s.nilWarned.CompareAndSwap(false, true) {
			core.Warn("incidents: session gate not wired; failing locked", "scope", scope)
		}
		return core.Fail(core.E(scope, "incidents.session.locked", nil)), false
	}
	if len(g.UnlockedAccountIDs()) == 0 {
		return core.Fail(core.E(scope, "incidents.session.locked", nil)), false
	}
	return core.Result{}, true
}
