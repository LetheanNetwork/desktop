// SPDX-Licence-Identifier: EUPL-1.2

// Service — Core integration for the Office document catalogue. Reads
// markdown files from ~/Lethean/office/docs/ and returns typed rows to
// the Wails frontend.
//
// Lifecycle:
//   - Register(c)   wires the service into the Core container
//   - ServiceName() returns "Documents" for the Wails namespace
//
// All I/O uses CoreGO wrappers (core.ReadFile / core.ReadDir /
// core.DirFS / core.MkdirAll / core.Stat / core.UserHomeDir).
// Banned stdlib imports: os, path/filepath, strings, encoding/json,
// fmt, log, errors.

package documents

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/paths"
	"gopkg.in/yaml.v3"
)

// Service owns the Office document catalogue surface.
//
// Usage example:
//
//	svc := documents.NewService(c)
type Service struct {
	core *core.Core
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
func docsDir() core.Result {
	root := paths.Root()
	if !root.OK {
		return root
	}
	dir := core.PathJoin(root.Value.(string), "office", "docs")
	if r := core.MkdirAll(dir, 0o700); !r.OK {
		return r
	}
	return core.Ok(dir)
}

// docFrontmatter is the YAML schema decoded from each document's
// optional frontmatter block.
type docFrontmatter struct {
	State  string `yaml:"state"`
	Author string `yaml:"author"`
	Title  string `yaml:"title"` // optional; overridden by H1 body scan
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
