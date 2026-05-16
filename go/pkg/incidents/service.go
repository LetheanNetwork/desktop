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
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/paths"
	"gopkg.in/yaml.v3"
)

// Service owns the incidents surface.
//
// Usage example:
//
//	svc := incidents.NewService(c)
type Service struct {
	core *core.Core
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
func incidentsDir() core.Result {
	root := paths.Root()
	if !root.OK {
		return root
	}
	dir := core.PathJoin(root.Value.(string), "incidents")
	if r := core.MkdirAll(dir, 0o755); !r.OK {
		return r
	}
	return core.Ok(dir)
}

// yearMonthDir resolves ~/Lethean/incidents/{YYYY}/{MM}/ and creates it.
func yearMonthDir(t core.Time) core.Result {
	base := incidentsDir()
	if !base.OK {
		return base
	}
	yr := core.TimeFormat(t, "2006")
	mo := core.TimeFormat(t, "01")
	dir := core.PathJoin(base.Value.(string), yr, mo)
	if r := core.MkdirAll(dir, 0o755); !r.OK {
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

// parseRecord splits a Trix file into frontmatter + body and decodes
// the YAML header into an IncidentRecord. The body is stored in the
// PostMortem field.
func parseRecord(raw []byte) (IncidentRecord, error) {
	// Find the closing --- of the frontmatter block.
	// The file starts with "---\n"; the closing delimiter is the next
	// "---" on its own line. We scan via byte search rather than the
	// banned `strings` package.
	const delim = "---"
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
			// Verify it's on its own line (preceded by \n or start)
			if i == 0 || content[i-1] == '\n' {
				closeIdx = i
				break
			}
		}
	}

	var rec IncidentRecord
	var body string
	if closeIdx < 0 {
		// No closing delimiter — treat entire content as frontmatter.
		if err := yaml.Unmarshal(content, &rec); err != nil {
			return IncidentRecord{}, core.E("incidents.parse", "yaml unmarshal", err)
		}
	} else {
		fm := content[:closeIdx]
		if err := yaml.Unmarshal(fm, &rec); err != nil {
			return IncidentRecord{}, core.E("incidents.parse", "yaml unmarshal", err)
		}
		// Body follows the closing --- and an optional newline.
		rest := content[closeIdx+3:]
		if len(rest) > 0 && rest[0] == '\n' {
			rest = rest[1:]
		}
		body = string(rest)
	}
	rec.PostMortem = body
	return rec, nil
}

// marshalRecord serialises an IncidentRecord to the Trix file format:
// YAML frontmatter block followed by the post-mortem markdown body.
func marshalRecord(r IncidentRecord) ([]byte, error) {
	// Marshal the struct fields (excluding PostMortem, which is body).
	fm, err := yaml.Marshal(r)
	if err != nil {
		return nil, core.E("incidents.marshal", "yaml marshal", err)
	}
	var out []byte
	out = append(out, []byte("---\n")...)
	out = append(out, fm...)
	out = append(out, []byte("---\n")...)
	if r.PostMortem != "" {
		out = append(out, []byte(r.PostMortem)...)
	}
	return out, nil
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
func loadOne(id string) (IncidentRecord, string, error) {
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
			target := core.PathJoin(dirPath, id+".md")
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

// writeRecord serialises a record back to disk. dirPath must be the
// directory that already contains the file.
func writeRecord(r IncidentRecord, dirPath string) error {
	raw, err := marshalRecord(r)
	if err != nil {
		return err
	}
	target := core.PathJoin(dirPath, r.ID+".md")
	if w := core.WriteFile(target, raw, 0o644); !w.OK {
		return core.E("incidents.writeRecord", w.Error(), nil)
	}
	return nil
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
