// SPDX-Licence-Identifier: EUPL-1.2

// Service — Core integration for the v1 deploy history catalogue.
// Reads and writes Trix-style markdown files under
// ~/Lethean/deploys/{deploy-id}.md.
//
// Lifecycle:
//   - Register(c)   wires the service into the Core container
//   - ServiceName() returns "Deploys" for the Wails namespace
//
// All I/O via CoreGO wrappers: core.ReadFile / core.WriteFile /
// core.MkdirAll / core.ReadDir / core.DirFS. Banned: os, path/filepath,
// strings, encoding/json, fmt, log, errors.

package deploys

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/paths"
	"gopkg.in/yaml.v3"
)

// Service owns the deploy history catalogue surface.
//
// Usage example:
//
//	svc := deploys.NewService(c)
type Service struct {
	core *core.Core
}

// NewService constructs the deploys service against a Core container.
// Wired via core.WithName("coding-deploys", deploys.Register) in app.go.
//
// Usage example:
//
//	svc := deploys.NewService(c)
func NewService(c *core.Core) *Service {
	return &Service{core: c}
}

// Register constructs the deploys service for Core registration.
//
// Usage example:
//
//	core.New(core.WithName("coding-deploys", deploys.Register))
func Register(c *core.Core) core.Result {
	return core.Ok(NewService(c))
}

// ServiceName labels the binding namespace exposed to JS.
// Wails binds methods as "Deploys.List()" etc.
func (s *Service) ServiceName() string { return "Deploys" }

// deploysDir resolves ~/Lethean/deploys/ and creates it if missing.
// Mode 0o700 — deploy metadata is operational data (Cerberus #1487 mandate).
func deploysDir() core.Result {
	root := paths.Root()
	if !root.OK {
		return root
	}
	dir := core.PathJoin(root.Value.(string), "deploys")
	if r := core.MkdirAll(dir, 0o700); !r.OK {
		return r
	}
	return core.Ok(dir)
}

// deployFilePath returns the absolute path to a deploy record file.
// Validates id via paths.IsValidID before path join.
func deployFilePath(id string) core.Result {
	if err := paths.IsValidID(id); err != nil {
		return core.Fail(core.E("deploys.deployFilePath", "invalid deploy ID", err))
	}
	dirR := deploysDir()
	if !dirR.OK {
		return dirR
	}
	return core.Ok(core.PathJoin(dirR.Value.(string), id+".md"))
}

// generateID returns a deploy ID string from the given time.
// Format: "deploy-{YYYYMMDD}-{HHMM}".
// Deterministic — two deploys in the same minute produce identical IDs.
func generateID(t core.Time) string {
	y, mo, d := t.Date()
	h, mi, _ := t.Clock()
	return core.Sprintf("deploy-%04d%02d%02d-%02d%02d", y, int(mo), d, h, mi)
}

// trixFrontmatter is an intermediate type used to marshal DeployRecord
// into YAML without the yaml package needing to know about core.Time.
// We write via yaml.Marshal on the record directly; the struct tags handle it.

// parseFrontmatter splits a Trix file on the second "---" boundary.
// Returns the decoded DeployRecord from the YAML block and the notes body.
// Both "---\n...\n---\nnotes" and "---\n...\n---" shapes are handled.
func parseFrontmatter(raw []byte) (DeployRecord, string, error) {
	if len(raw) == 0 {
		return DeployRecord{}, "", core.E("deploys.parseFrontmatter", "empty file", nil)
	}

	// Find the opening "---".
	start := 0
	for start < len(raw) && raw[start] == '-' && start < 3 {
		start++
	}
	// Advance past the first "---\n" line.
	lineEnd := 0
	for lineEnd < len(raw) && raw[lineEnd] != '\n' {
		lineEnd++
	}
	bodyStart := lineEnd + 1

	// Find the closing "---".
	closeIdx := -1
	for i := bodyStart; i < len(raw)-2; i++ {
		if raw[i] == '-' && raw[i+1] == '-' && raw[i+2] == '-' {
			// Ensure it's at a line boundary.
			if i == 0 || raw[i-1] == '\n' {
				closeIdx = i
				break
			}
		}
	}

	var frontmatterBytes []byte
	var notes string

	if closeIdx < 0 {
		// No closing "---" — treat entire remainder as frontmatter.
		frontmatterBytes = raw[bodyStart:]
	} else {
		frontmatterBytes = raw[bodyStart:closeIdx]
		notesStart := closeIdx + 3
		if notesStart < len(raw) && raw[notesStart] == '\n' {
			notesStart++
		}
		if notesStart < len(raw) {
			notes = string(raw[notesStart:])
		}
	}

	var rec DeployRecord
	if err := yaml.Unmarshal(frontmatterBytes, &rec); err != nil {
		return DeployRecord{}, "", core.E("deploys.parseFrontmatter", "yaml decode failed", err)
	}
	return rec, notes, nil
}

// renderTrix writes a DeployRecord as a Trix markdown file:
// "---\n{yaml}\n---\n{notes}".
func renderTrix(rec DeployRecord, notes string) ([]byte, error) {
	fm, err := yaml.Marshal(rec)
	if err != nil {
		return nil, err
	}
	var buf []byte
	buf = append(buf, "---\n"...)
	buf = append(buf, fm...)
	buf = append(buf, "---\n"...)
	if notes != "" {
		buf = append(buf, notes...)
		if len(notes) > 0 && notes[len(notes)-1] != '\n' {
			buf = append(buf, '\n')
		}
	}
	return buf, nil
}

// relativeAge formats a duration (now - t) as a short human-readable string.
// Matches the fixture age values in frontend/src/app/desktop/surfaces/coding/deploys.ts.
func relativeAge(t core.Time, now core.Time) string {
	if t.IsZero() {
		return ""
	}
	diff := now.Sub(t)
	if diff < 0 {
		return "0m"
	}
	const minute = core.Minute
	const hour = core.Hour
	const day = 24 * hour
	const week = 7 * day

	switch {
	case diff < hour:
		return core.Sprintf("%dm", int(diff/minute))
	case diff < day:
		return core.Sprintf("%dh", int(diff/hour))
	case diff < 30*day:
		return core.Sprintf("%dd", int(diff/day))
	case diff < 12*week:
		return core.Sprintf("%d w", int(diff/week))
	default:
		return core.Sprintf("%d mo", int(diff/(30*day)))
	}
}

// deriveHealth maps a deploy Outcome to an EnvRow Health string.
// v1: "failed" → "failed"; everything else → "ok". No live probe.
func deriveHealth(outcome string) string {
	if outcome == "failed" {
		return "failed"
	}
	return "ok"
}

// buildEnvRows derives one EnvRow per distinct Env from the record list.
// Records must be sorted newest-first so the first hit per env is used.
// Canonical order: production → staging → preview → others alphabetically.
func buildEnvRows(records []DeployRecord, now core.Time) []EnvRow {
	if len(records) == 0 {
		return []EnvRow{}
	}
	seen := make(map[string]bool)
	byEnv := make(map[string]EnvRow)

	for _, rec := range records {
		if seen[rec.Env] {
			continue
		}
		seen[rec.Env] = true
		byEnv[rec.Env] = EnvRow{
			Name:    rec.Env,
			URL:     rec.URL,
			Version: rec.Version,
			Commit:  rec.Commit,
			Age:     relativeAge(rec.Timestamp, now),
			Health:  deriveHealth(rec.Outcome),
		}
	}

	var result []EnvRow
	// Canonical order first.
	for _, name := range envOrder {
		if row, ok := byEnv[name]; ok {
			result = append(result, row)
			delete(byEnv, name)
		}
	}
	// Remaining envs alphabetically.
	var extra []string
	for name := range byEnv {
		extra = append(extra, name)
	}
	// Simple insertion sort — the number of extra envs is always tiny.
	for i := 1; i < len(extra); i++ {
		for j := i; j > 0 && extra[j] < extra[j-1]; j-- {
			extra[j], extra[j-1] = extra[j-1], extra[j]
		}
	}
	for _, name := range extra {
		result = append(result, byEnv[name])
	}
	return result
}

// readAll reads all deploy record files from deploysDir(), parses their
// frontmatter, and returns records sorted by Timestamp descending
// (newest-first). Unreadable or malformed files are warned and skipped.
func readAll() ([]DeployRecord, error) {
	dirR := deploysDir()
	if !dirR.OK {
		return nil, core.E("deploys.readAll", dirR.Error(), nil)
	}
	dir := dirR.Value.(string)

	entriesR := core.ReadDir(core.DirFS(dir), ".")
	if !entriesR.OK {
		return nil, nil
	}
	entries, _ := entriesR.Value.([]core.FsDirEntry)

	var records []DeployRecord
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !core.HasSuffix(name, ".md") {
			continue
		}
		fpath := core.PathJoin(dir, name)
		rawR := core.ReadFile(fpath)
		if !rawR.OK {
			core.Warn("deploys: skipping unreadable file", "file", name, "err", rawR.Error())
			continue
		}
		raw, _ := rawR.Value.([]byte)
		rec, _, err := parseFrontmatter(raw)
		if err != nil {
			core.Warn("deploys: skipping malformed file", "file", name, "err", err)
			continue
		}
		records = append(records, rec)
	}

	// Sort by Timestamp descending (newest-first) via insertion sort.
	for i := 1; i < len(records); i++ {
		for j := i; j > 0 && records[j].Timestamp.After(records[j-1].Timestamp); j-- {
			records[j], records[j-1] = records[j-1], records[j]
		}
	}
	return records, nil
}

// toDeployRow converts a DeployRecord to the wire type for the history table.
func toDeployRow(rec DeployRecord, now core.Time) DeployRow {
	return DeployRow{
		Ts:      relativeAge(rec.Timestamp, now),
		Env:     rec.Env,
		By:      rec.By,
		Commit:  rec.Commit,
		Outcome: rec.Outcome,
		Dur:     rec.Dur,
	}
}
