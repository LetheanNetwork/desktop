// SPDX-Licence-Identifier: EUPL-1.2

// Service — Core integration for the deals surface. Reads and
// writes Trix-style markdown files from ~/Lethean/sales/deals/.
// Deals are also the source-of-truth for pipeline stage — pkg/sales/pipeline
// derives its Kanban view by scanning this directory.
//
// Lifecycle:
//   - Register(c)   wires the service into the Core container
//   - ServiceName() returns "Deals" for the Wails namespace
//
// All I/O uses CoreGO wrappers (core.ReadFile / core.WriteFile /
// core.ReadDir / core.DirFS / core.MkdirAll / core.PathJoin).
// Banned stdlib imports: os, path/filepath, strings, encoding/json,
// fmt, log, errors.

package deals

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/paths"
	"dappco.re/lthn/desktop/pkg/recordfile"
	"gopkg.in/yaml.v3"
)

// Service owns the deals surface.
//
// Usage example:
//
//	svc := deals.NewService(c)
type Service struct {
	core *core.Core
}

// NewService constructs the deals service against a Core container.
// Wired via core.WithName("sales-deals", deals.Register) in app.go.
//
// Usage example:
//
//	svc := deals.NewService(c)
func NewService(c *core.Core) *Service {
	return &Service{core: c}
}

// Register constructs the deals service for Core registration.
//
// Usage example:
//
//	core.New(core.WithName("sales-deals", deals.Register))
func Register(c *core.Core) core.Result {
	return core.Ok(NewService(c))
}

// ServiceName labels the binding namespace exposed to JS.
// Wails binds methods as "Deals.List()" etc.
func (s *Service) ServiceName() string { return "Deals" }

// dealsDir resolves ~/Lethean/sales/deals/ and creates it if missing.
// Mode 0o700 (Cerberus #1487 PR-1): deal records carry sensitive
// commercial data (customer names, amounts in pence, stakeholders,
// activity log) — owner-only at rest.
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

// knownStages is the canonical ordered stage set.
// Matches the pipeline.ts fixture ids.
var knownStages = map[string]string{
	"qual":    "Qualifying",
	"engage":  "Engaging",
	"propose": "Proposal",
	"close":   "Closing",
	"won":     "Won",
	"lost":    "Lost",
}

// stageLabel maps a stage id to its human label.
func stageLabel(id string) string {
	if label, ok := knownStages[id]; ok {
		return label
	}
	return id
}

// isValidStage reports whether the given stage id is in the known set.
func isValidStage(id string) bool {
	_, ok := knownStages[id]
	return ok
}

// formatGBPK formats integer pence as "£N K" (threshold 1000p = £10).
// Values below 1000p are formatted as "£N".
func formatGBPK(pence int) string {
	if pence >= 1000 {
		k := (pence + 500) / 1000 // round to nearest £K
		return core.Sprintf("£%d K", k)
	}
	pounds := pence / 100
	return core.Sprintf("£%d", pounds)
}

// activityTS formats a timestamp relative to now for activity log display.
// Returns "today · HH:MM" for same calendar day, "yest · HH:MM" for
// yesterday, or "N d ago" otherwise.
func activityTS(t core.Time, now core.Time) string {
	if t.IsZero() {
		return ""
	}
	diff := now.Sub(t)
	if diff < 0 {
		diff = -diff
	}
	const day = 24 * core.Hour

	// Same calendar day check by comparing date parts.
	tLocal := t.UTC()
	nLocal := now.UTC()
	sameDay := core.TimeFormat(tLocal, "2006-01-02") == core.TimeFormat(nLocal, "2006-01-02")
	if sameDay {
		return core.Sprintf("today · %s", core.TimeFormat(tLocal, "15:04"))
	}
	yesterday := now.Add(-day)
	yLocal := yesterday.UTC()
	prevDay := core.TimeFormat(tLocal, "2006-01-02") == core.TimeFormat(yLocal, "2006-01-02")
	if prevDay {
		return core.Sprintf("yest · %s", core.TimeFormat(tLocal, "15:04"))
	}
	days := int(diff / day)
	if days < 7 {
		return core.Sprintf("%d d ago", days)
	}
	weeks := days / 7
	return core.Sprintf("%d w ago", weeks)
}

// slugify converts a customer name to a filesystem-safe slug.
func slugify(name string) string {
	b := []byte(name)
	out := make([]byte, 0, len(b))
	prevHyphen := false
	for _, ch := range b {
		var c byte
		switch {
		case ch >= 'A' && ch <= 'Z':
			c = ch + 32
		case ch >= 'a' && ch <= 'z':
			c = ch
		case ch >= '0' && ch <= '9':
			c = ch
		default:
			c = '-'
		}
		if c == '-' {
			if prevHyphen {
				continue
			}
			prevHyphen = true
		} else {
			prevHyphen = false
		}
		out = append(out, c)
	}
	for len(out) > 0 && out[len(out)-1] == '-' {
		out = out[:len(out)-1]
	}
	return string(out)
}

// countDeals returns the number of .md files in the deals dir.
// Used to generate NNN suffixes for new deal IDs.
func countDeals(dir string) int {
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

// parseRecord splits a Trix file into frontmatter + body via the
// shared recordfile.Split helper and decodes the YAML header into a
// DealRecord. The body is stored in the Notes field.
func parseRecord(raw []byte) (DealRecord, error) {
	fm, body := recordfile.Split(raw)
	var rec DealRecord
	if err := yaml.Unmarshal(fm, &rec); err != nil {
		return DealRecord{}, core.E("deals.parse", "yaml unmarshal", err)
	}
	rec.Notes = string(body)
	return rec, nil
}

// marshalRecord serialises a DealRecord to the Trix file format via
// the shared recordfile.Stitch helper.
func marshalRecord(r DealRecord) ([]byte, error) {
	fm, err := yaml.Marshal(r)
	if err != nil {
		return nil, core.E("deals.marshal", "yaml marshal", err)
	}
	return recordfile.Stitch(fm, []byte(r.Notes)), nil
}

// toDeal converts a DealRecord to the wire type.
// now is passed so callers control the clock reference (test-friendly).
func toDeal(r DealRecord, now core.Time, includeLog bool) Deal {
	var log []Activity
	if includeLog {
		log = make([]Activity, 0, len(r.Log))
		for _, a := range r.Log {
			log = append(log, Activity{
				TS:  activityTS(a.At, now),
				K:   a.K,
				Who: a.Who,
				T:   a.T,
			})
		}
	}

	docs := make([]DocLink, 0, len(r.Docs))
	for _, d := range r.Docs {
		docs = append(docs, DocLink{T: d.Title, S: d.State})
	}

	prob := core.Sprintf("%d%%", r.ProbabilityPct)

	// Build headline: "£24 K · 12-month hosted" — for v1, headline is the
	// formatted amount. A custom headline suffix is a follow-up.
	headline := formatGBPK(r.AmountPence)

	return Deal{
		ID:           r.ID,
		Customer:     r.Customer,
		Headline:     headline,
		Stage:        stageLabel(r.Stage),
		Probability:  prob,
		CloseTarget:  r.CloseTarget,
		Log:          log,
		Stakeholders: r.Stakeholders,
		Docs:         docs,
	}
}

// loadAll scans ~/Lethean/sales/deals/ and returns all parseable
// DealRecord values without loading the notes body.
func loadAll() ([]DealRecord, error) {
	dirR := dealsDir()
	if !dirR.OK {
		return nil, core.E("deals.loadAll", dirR.Error(), nil)
	}
	dir := dirR.Value.(string)

	entriesR := core.ReadDir(core.DirFS(dir), ".")
	if !entriesR.OK {
		return nil, nil
	}
	entries, _ := entriesR.Value.([]core.FsDirEntry)

	var records []DealRecord
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
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
		rec.Notes = ""
		records = append(records, rec)
	}
	return records, nil
}

// loadOne reads and fully parses the deal file for the given ID.
// Cerberus #1486: id is rejected unless IsValidID — defence in depth
// even though every wails entry point already gates on input.ID.
func loadOne(id string) (DealRecord, error) {
	if err := paths.IsValidID(id); err != nil {
		return DealRecord{}, err
	}
	dirR := dealsDir()
	if !dirR.OK {
		return DealRecord{}, core.E("deals.loadOne", dirR.Error(), nil)
	}
	// Cerberus #1486 belt: WithinDir check after the join.
	fpath, jerr := paths.JoinAndCheck(dirR.Value.(string), id+".md")
	if jerr != nil {
		return DealRecord{}, jerr
	}
	raw := core.ReadFile(fpath)
	if !raw.OK {
		return DealRecord{}, core.E("deals.loadOne", "not found: "+id, nil)
	}
	return parseRecord(raw.Value.([]byte))
}

// writeRecord serialises and writes a DealRecord to disk via
// paths.AtomicWriteWithVersion (Cascade W1, Mantis #1540).
//
// ifVersion is the optimistic-lock anchor — pass the Version value the
// caller observed on disk before mutating the record, or 0 for first-
// writes / legacy-file upgrades. writeRecord stamps r.Version=ifVersion+1
// into the marshalled body so subsequent reads see a monotonic version.
//
// Cerberus #1486: the record ID lands directly in the filename so
// IsValidID guards it even though Create generates the ID via Sprintf.
//
// Return shape (Mantis #1544, gating W2): on the stale-write path
// the function returns core.Fail(paths.ConflictEnvelope{...}) so the
// Wails-marshalled Result.Value carries the lowercase-json shape
// (`{code, current_version, current_hash}`) that conflict-dispatch.ts
// extractEnvelope already pattern-matches on. Non-conflict failures
// propagate via core.Fail(core.E(...)) so audit + diagnostic detail
// reach the caller unchanged.
//
// Audit emission is automatic via paths.AuditModeForPath — sales/deals/*
// routes through AuditModeBatch per RFC §6.1.
//
// Usage example:
//
//	if wr := writeRecord(rec, dir, prior.Version); !wr.OK {
//	    return wr
//	}
func writeRecord(r DealRecord, dir string, ifVersion int) core.Result {
	if err := paths.IsValidID(r.ID); err != nil {
		return core.Fail(err)
	}
	// Stamp the next monotonic version. ifVersion=0 (Create / legacy
	// upgrade) yields version=1; subsequent updates increment.
	r.Version = ifVersion + 1
	if r.Version < 1 {
		r.Version = 1
	}
	raw, err := marshalRecord(r)
	if err != nil {
		return core.Fail(err)
	}
	// Cerberus #1486 belt: WithinDir check after the join.
	target, jerr := paths.JoinAndCheck(dir, r.ID+".md")
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
			"deals.update.conflict", stale))
	}
	return core.Fail(core.E("deals.writeRecord", res.Error(), nil))
}

// fireEvent publishes a deal event on the Core ACTION bus.
func (s *Service) fireEvent(name string, rec DealRecord) {
	if s == nil || s.core == nil {
		return
	}
	s.core.ACTION(DealEvent{
		EventName: name,
		DealID:    rec.ID,
		Stage:     rec.Stage,
		Customer:  rec.Customer,
		At:        core.Now().UTC(),
	})
}
