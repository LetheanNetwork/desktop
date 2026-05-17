// SPDX-Licence-Identifier: EUPL-1.2

// Service — Core integration for the pipeline surface. Derives a Kanban
// view by scanning ~/Lethean/sales/deals/ and grouping records by stage.
// Pipeline state is derived — deals own their stage. MoveDeal delegates
// to the deal record directly.
//
// Lifecycle:
//   - Register(c)   wires the service into the Core container
//   - ServiceName() returns "Pipeline" for the Wails namespace
//
// All I/O uses CoreGO wrappers. Banned stdlib imports: os, path/filepath,
// strings, encoding/json, fmt, log, errors.

package pipeline

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/paths"
	"gopkg.in/yaml.v3"
)

// Service owns the pipeline surface.
//
// Usage example:
//
//	svc := pipeline.NewService(c)
type Service struct {
	core *core.Core
}

// NewService constructs the pipeline service against a Core container.
//
// Usage example:
//
//	svc := pipeline.NewService(c)
func NewService(c *core.Core) *Service {
	return &Service{core: c}
}

// Register constructs the pipeline service for Core registration.
//
// Usage example:
//
//	core.New(core.WithName("sales-pipeline", pipeline.Register))
func Register(c *core.Core) core.Result {
	return core.Ok(NewService(c))
}

// ServiceName labels the binding namespace exposed to JS.
// Wails binds methods as "Pipeline.List()" etc.
func (s *Service) ServiceName() string { return "Pipeline" }

// stageSpec is the canonical stage ordered metadata.
type stageSpec struct {
	ID    string
	Label string
}

// stageOrder returns the canonical ordered stage slice.
// The order matches the fixture column order in pipeline.ts.
func stageOrder() []stageSpec {
	return []stageSpec{
		{ID: "qual", Label: "Qualifying"},
		{ID: "engage", Label: "Engaging"},
		{ID: "propose", Label: "Proposal"},
		{ID: "close", Label: "Closing"},
		{ID: "won", Label: "Won"},
		{ID: "lost", Label: "Lost"},
	}
}

// dealsDir resolves ~/Lethean/sales/deals/ and creates it if missing.
// Mode 0o700 (Cerberus #1487 PR-1): same directory as pkg/sales/deals
// — sensitive commercial data, owner-only at rest.
func dealsDir() core.Result {
	root := paths.Root()
	if !root.OK {
		return root
	}
	dir := core.PathJoin(root.Value.(string), "sales", "deals")
	if r := core.MkdirAll(dir, 0o700); !r.OK {
		return r
	}
	return core.Ok(dir)
}

// formatGBPK formats integer pence as "£N K" (above 1000p) or "£N".
func formatGBPK(pence int) string {
	if pence >= 1000 {
		k := (pence + 500) / 1000
		return core.Sprintf("£%d K", k)
	}
	return core.Sprintf("£%d", pence/100)
}

// dealFrontmatter is the minimal shape parsed from each deal file for the
// pipeline rollup — avoids importing the full deals package.
type dealFrontmatter struct {
	ID          string `yaml:"id"`
	Customer    string `yaml:"customer"`
	AmountPence int    `yaml:"amount_pence"`
	Stage       string `yaml:"stage"`
	CloseTarget string `yaml:"close_target"`
}

// parseFrontmatter extracts only the frontmatter from a Trix file.
func parseFrontmatter(raw []byte) (dealFrontmatter, error) {
	content := raw

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

	closeIdx := -1
	for i := 0; i < len(content)-3; i++ {
		if content[i] == '-' && content[i+1] == '-' && content[i+2] == '-' {
			if i == 0 || content[i-1] == '\n' {
				closeIdx = i
				break
			}
		}
	}

	var fm dealFrontmatter
	fmBytes := content
	if closeIdx >= 0 {
		fmBytes = content[:closeIdx]
	}
	if err := yaml.Unmarshal(fmBytes, &fm); err != nil {
		return dealFrontmatter{}, core.E("pipeline.parseFrontmatter", "yaml unmarshal", err)
	}
	return fm, nil
}

// loadDeals scans ~/Lethean/sales/deals/ and returns all parseable deal
// frontmatters. Skips malformed files silently.
func loadDeals() ([]dealFrontmatter, error) {
	dirR := dealsDir()
	if !dirR.OK {
		return nil, core.E("pipeline.loadDeals", dirR.Error(), nil)
	}
	dir := dirR.Value.(string)

	entriesR := core.ReadDir(core.DirFS(dir), ".")
	if !entriesR.OK {
		return nil, nil
	}
	entries, _ := entriesR.Value.([]core.FsDirEntry)

	var fms []dealFrontmatter
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if len(name) < 4 || name[len(name)-3:] != ".md" {
			continue
		}
		raw := core.ReadFile(core.PathJoin(dir, name))
		if !raw.OK {
			continue
		}
		fm, err := parseFrontmatter(raw.Value.([]byte))
		if err != nil {
			continue
		}
		fms = append(fms, fm)
	}
	return fms, nil
}

// writeDealStage reads the deal file, mutates the stage frontmatter
// field, bumps the version, and writes it back via
// paths.AtomicWriteWithVersion (Cascade W1, Mantis #1540). Returns the
// previous stage paired with a core.Result.
//
// The deal file is shared with pkg/sales/deals — both writers MUST
// route through paths.AtomicWriteWithVersion + the version frontmatter
// field for the optimistic-lock guarantee to hold across cross-package
// concurrent mutation. Pipeline reads ReadVersion (single stat+read
// under lock-friendly semantics), increments the version, edits the
// stage line in place, and writes back gated on IfVersion=priorVersion.
//
// Return shape (Mantis #1544, gating W2): on the stale-write path
// the function returns core.Fail(paths.ConflictEnvelope{...}) so the
// Wails-marshalled Result.Value carries the lowercase-json shape
// (`{code, current_version, current_hash}`) that conflict-dispatch.ts
// extractEnvelope already pattern-matches on. Legacy files without a
// version frontmatter field read as 0 and upgrade via an unconditional
// first-write that stamps version=1.
//
// Cerberus #1486: id is the load-bearing path component; reject anything
// that fails IsValidID before joining.
//
// Audit emission is automatic via paths.AuditModeForPath — sales/deals/*
// routes through AuditModeBatch per RFC §6.1.
func writeDealStage(id, toStage string) (fromStage string, result core.Result) {
	if vErr := paths.IsValidID(id); vErr != nil {
		return "", core.Fail(vErr)
	}
	dirR := dealsDir()
	if !dirR.OK {
		return "", core.Fail(core.E("pipeline.writeDealStage", dirR.Error(), nil))
	}
	dir := dirR.Value.(string)
	fpath := core.PathJoin(dir, id+".md")

	rd := paths.ReadVersion(fpath)
	if !rd.OK {
		return "", core.Fail(core.E("pipeline.writeDealStage", rd.Error(), nil))
	}
	cur := rd.Value.(paths.ReadOutput)
	if len(cur.Body) == 0 {
		return "", core.Fail(core.E("pipeline.writeDealStage", "not found: "+id, nil))
	}

	fm, err := parseFrontmatter(cur.Body)
	if err != nil {
		return "", core.Fail(err)
	}
	fromStage = fm.Stage

	// Edit the stage line in place. updateYAMLField restricts itself to
	// the frontmatter block so the body is untouched.
	updated := updateYAMLField(cur.Body, "stage", toStage)
	// Bump or insert the version frontmatter line so subsequent reads
	// see a monotonic version (matches the deals service writer shape).
	nextVersion := cur.Version + 1
	if nextVersion < 1 {
		nextVersion = 1
	}
	updated = setVersionField(updated, nextVersion)

	res := paths.AtomicWriteWithVersion(fpath, paths.WriteInput{
		Body:      updated,
		IfVersion: cur.Version,
	})
	if res.OK {
		return fromStage, res
	}
	if stale, ok := paths.VersionStaleFromError(res.Value); ok {
		return fromStage, core.Fail(paths.NewConflictEnvelope(
			"pipeline.update.conflict", stale))
	}
	return fromStage, core.Fail(core.E("pipeline.writeDealStage", res.Error(), nil))
}

// setVersionField stamps a "version: N" line into the frontmatter
// block. If the file already has a version line within the leading
// "---\n...---\n" delimiter, the value is updated in place; otherwise a
// new line is inserted immediately after the opening delimiter.
// Mirrors updateYAMLField's frontmatter-only discipline so the body
// stays untouched (Mantis #1545 — bounded discipline prevents body
// lines like "version: 1.2.3 of the spec" from being mistaken for the
// frontmatter version field).
//
// Usage example:
//
//	updated := setVersionField(raw, 2)
func setVersionField(raw []byte, v int) []byte {
	valueStr := core.Sprintf("%d", v)
	// First try in-place update via updateYAMLField (frontmatter-
	// bounded). If the field exists inside frontmatter the helper
	// returns a new slice with the value rewritten.
	if hasVersionInFrontmatter(raw) {
		return updateYAMLField(raw, "version", valueStr)
	}
	// No version field in frontmatter — insert one immediately BEFORE
	// the closing "---" line (Mantis #1550). Inserting at the top
	// would reorder Snider-ordered frontmatter on every legacy-file
	// upgrade (cosmetic but persistent churn). Tail-insert preserves
	// the author's field ordering.
	//
	// Refuse to touch raw if it lacks a frontmatter opener OR a
	// closing "---" line (matches updateYAMLField's boundary
	// discipline — never guess the frontmatter boundary).
	open := []byte("---\n")
	if len(raw) < len(open) {
		return raw
	}
	for i, b := range open {
		if raw[i] != b {
			return raw
		}
	}
	fmStart := len(open)
	closeIdx := -1
	for i := fmStart; i+2 < len(raw); i++ {
		if raw[i] == '-' && raw[i+1] == '-' && raw[i+2] == '-' {
			if i == fmStart || raw[i-1] == '\n' {
				closeIdx = i
				break
			}
		}
	}
	if closeIdx < 0 {
		return raw
	}
	line := "version: " + valueStr + "\n"
	ins := make([]byte, 0, len(raw)+len(line))
	ins = append(ins, raw[:closeIdx]...)
	ins = append(ins, []byte(line)...)
	ins = append(ins, raw[closeIdx:]...)
	return ins
}

// hasVersionInFrontmatter reports whether raw's frontmatter block
// contains a line starting with "version:" (after optional leading
// whitespace) — used by setVersionField to decide between in-place
// update and tail-insert without scanning the body (Mantis #1545).
//
// Leading-whitespace trim mirrors paths.matchVersionLine (atomic_write.go)
// so reader + writer agree on what counts as a version line. Without
// the trim, an indented "  version: 5" frontmatter line is invisible
// to the writer but visible to the reader, producing duplicate version
// keys on subsequent writes (Mantis #1549).
//
// Returns false when raw lacks the leading "---\n" delimiter or the
// closing "---" line, mirroring updateYAMLField's boundary discipline.
func hasVersionInFrontmatter(raw []byte) bool {
	const open = "---\n"
	if len(raw) < len(open) {
		return false
	}
	for i := 0; i < len(open); i++ {
		if raw[i] != open[i] {
			return false
		}
	}
	fmStart := len(open)
	closeIdx := -1
	for i := fmStart; i+2 < len(raw); i++ {
		if raw[i] == '-' && raw[i+1] == '-' && raw[i+2] == '-' {
			if i == fmStart || raw[i-1] == '\n' {
				closeIdx = i
				break
			}
		}
	}
	if closeIdx < 0 {
		return false
	}
	prefix := []byte("version:")
	lineStart := fmStart
	for i := fmStart; i <= closeIdx; i++ {
		atEnd := i == closeIdx
		if atEnd || raw[i] == '\n' {
			line := raw[lineStart:i]
			// Skip leading whitespace to mirror paths.matchVersionLine.
			ws := 0
			for ws < len(line) && (line[ws] == ' ' || line[ws] == '\t') {
				ws++
			}
			trimmed := line[ws:]
			if len(trimmed) >= len(prefix) {
				match := true
				for j := range prefix {
					if trimmed[j] != prefix[j] {
						match = false
						break
					}
				}
				if match {
					return true
				}
			}
			lineStart = i + 1
		}
	}
	return false
}

// updateYAMLField replaces the value of a YAML field key on its own line
// within a Trix file's frontmatter block ONLY. The scan is bounded to
// the bytes between the leading "---\n" delimiter and the next "---"
// line on its own; body lines after the closing delimiter are NEVER
// considered (Mantis #1545 — bounded scan stops body lines like
// "version: 1.2.3 of the spec" from corrupting frontmatter sync).
//
// Returns the original bytes unchanged when:
//   - raw lacks a leading "---\n" opening delimiter
//   - the frontmatter block has no closing "---" line
//   - the key is not present inside the bounded frontmatter block
//
// Mirrors paths.parseFrontmatterVersion's bounded-walk shape so both
// readers and writers agree on the frontmatter boundary.
func updateYAMLField(raw []byte, key, value string) []byte {
	const open = "---\n"
	if len(raw) < len(open) {
		return raw
	}
	for i := 0; i < len(open); i++ {
		if raw[i] != open[i] {
			return raw
		}
	}
	// fmStart is the first byte after the opening "---\n".
	fmStart := len(open)
	// Locate the closing "---" delimiter — must sit on its own at
	// start-of-line within the frontmatter block.
	closeIdx := -1
	for i := fmStart; i+2 < len(raw); i++ {
		if raw[i] == '-' && raw[i+1] == '-' && raw[i+2] == '-' {
			// Must be at start-of-line (previous byte is newline).
			if i == fmStart || raw[i-1] == '\n' {
				closeIdx = i
				break
			}
		}
	}
	if closeIdx < 0 {
		return raw
	}
	// Walk the frontmatter block line-by-line for an existing "key: "
	// occurrence. Leading whitespace is trimmed before prefix-matching to
	// mirror paths.matchVersionLine (Mantis #1549 — reader and writer
	// agree on indented frontmatter; without the trim, indented entries
	// are invisible to the writer and a duplicate key gets inserted on
	// the next stamp). Original indentation is preserved on rewrite.
	prefix := []byte(key + ":")
	lineStart := fmStart
	for i := fmStart; i <= closeIdx; i++ {
		atEnd := i == closeIdx
		if atEnd || raw[i] == '\n' {
			line := raw[lineStart:i]
			ws := 0
			for ws < len(line) && (line[ws] == ' ' || line[ws] == '\t') {
				ws++
			}
			trimmed := line[ws:]
			if len(trimmed) >= len(prefix) {
				match := true
				for j := range prefix {
					if trimmed[j] != prefix[j] {
						match = false
						break
					}
				}
				if match {
					indent := line[:ws]
					result := make([]byte, 0, len(raw)+len(value))
					result = append(result, raw[:lineStart]...)
					result = append(result, indent...)
					result = append(result, []byte(key+": "+value)...)
					result = append(result, raw[i:]...)
					return result
				}
			}
			lineStart = i + 1
		}
	}
	return raw // no match in frontmatter — return unchanged
}

// fireMove publishes a pipeline move event on the Core ACTION bus.
func (s *Service) fireMove(dealID, fromStage, toStage string) {
	if s == nil || s.core == nil {
		return
	}
	s.core.ACTION(PipelineMovedEvent{
		EventName: EventPipelineMoved,
		DealID:    dealID,
		FromStage: fromStage,
		ToStage:   toStage,
		At:        core.Now().UTC(),
	})
}
