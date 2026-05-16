// SPDX-Licence-Identifier: EUPL-1.2

// Wails surface for the runbooks service. Methods are bound on the
// *Service receiver and exposed to the frontend via wails3 generate
// bindings — the TS shape lands at
// frontend/bindings/dappco.re/lthn/desktop/pkg/runbooks/service.

package runbooks

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/paths"
)

// List returns all runbooks, optionally filtered by health or area.
// The ListOutput always carries fresh/aging/stale counts for the
// FULL library (not the filtered subset) so the sidebar health
// widget stays accurate regardless of the active filter.
//
// Usage example:
//
//	r := svc.List(runbooks.ListInput{Health: "stale"})
//	if r.OK { out := r.Value.(runbooks.ListOutput) }
func (s *Service) List(input ListInput) core.Result {
	records, err := scanAll()
	if err != nil {
		return core.Fail(core.E("runbooks.List", "scan failed", err))
	}

	now := core.Now()
	// Seed on first empty scan so a fresh install immediately has content.
	if len(records) == 0 {
		if dir := runbooksDir(); dir.OK {
			seedAll(dir.Value.(string))
			// Rescan after seeding.
			records, _ = scanAll()
		}
	}

	fresh, aging, stale := buildCounts(records, now)
	total := len(records)

	// Filter.
	var filtered []RunbookEntry
	for _, r := range records {
		entry := toEntry(r, now)
		if input.Health != "" && entry.Health != input.Health {
			continue
		}
		if input.Area != "" && r.Area != input.Area {
			continue
		}
		filtered = append(filtered, entry)
	}
	if filtered == nil {
		filtered = []RunbookEntry{}
	}

	if input.Limit > 0 && len(filtered) > input.Limit {
		filtered = filtered[:input.Limit]
	}

	return core.Ok(ListOutput{
		Books: filtered,
		Total: total,
		Fresh: fresh,
		Aging: aging,
		Stale: stale,
	})
}

// Get returns a single runbook with the full markdown body.
//
// Usage example:
//
//	r := svc.Get(runbooks.GetInput{ID: "R-01"})
//	if r.OK { detail := r.Value.(runbooks.RunbookDetail) }
func (s *Service) Get(input GetInput) core.Result {
	if input.ID == "" && input.Slug == "" {
		return core.Fail(core.E("runbooks.Get", "id or slug is required", nil))
	}
	// Cerberus #1486: id + slug are matched against in-memory records
	// today but the slug also lands in filenames via writeRecord — gate
	// any non-empty value through IsValidID before it can propagate.
	if input.ID != "" {
		if err := paths.IsValidID(input.ID); err != nil {
			return core.Fail(err)
		}
	}
	if input.Slug != "" {
		if err := paths.IsValidID(input.Slug); err != nil {
			return core.Fail(err)
		}
	}
	rec, _, err := loadOne(input.ID, input.Slug)
	if err != nil {
		label := input.ID
		if label == "" {
			label = input.Slug
		}
		return core.Fail(core.E("runbooks.Get", "not found: "+label, err))
	}
	now := core.Now()
	return core.Ok(RunbookDetail{
		RunbookEntry: toEntry(rec, now),
		Body:         rec.Body,
	})
}

// Search performs a case-insensitive substring search across title,
// area, ID, and tags. An empty query returns all runbooks.
//
// Usage example:
//
//	r := svc.Search(runbooks.SearchInput{Query: "postfix"})
//	if r.OK { out := r.Value.(runbooks.ListOutput) }
func (s *Service) Search(input SearchInput) core.Result {
	records, err := scanAll()
	if err != nil {
		return core.Fail(core.E("runbooks.Search", "scan failed", err))
	}

	now := core.Now()
	fresh, aging, stale := buildCounts(records, now)
	total := len(records)

	var matched []RunbookEntry
	for _, r := range records {
		if !matchSearch(r, input.Query) {
			continue
		}
		matched = append(matched, toEntry(r, now))
	}
	if matched == nil {
		matched = []RunbookEntry{}
	}
	if input.Limit > 0 && len(matched) > input.Limit {
		matched = matched[:input.Limit]
	}

	return core.Ok(ListOutput{
		Books: matched,
		Total: total,
		Fresh: fresh,
		Aging: aging,
		Stale: stale,
	})
}

// MarkRehearsed updates the LastRehearsed timestamp to now, rewrites
// the Trix file, and fires the runbooks.rehearsed event.
//
// Usage example:
//
//	r := svc.MarkRehearsed(runbooks.MarkInput{ID: "R-05"})
//	if r.OK { entry := r.Value.(runbooks.RunbookEntry) }
func (s *Service) MarkRehearsed(input MarkInput) core.Result {
	if err := paths.IsValidID(input.ID); err != nil {
		return core.Fail(err)
	}
	rec, dirPath, err := loadOne(input.ID, "")
	if err != nil {
		return core.Fail(core.E("runbooks.MarkRehearsed", "not found: "+input.ID, err))
	}
	priorVersion := rec.Version
	rec.LastRehearsed = core.Now().UTC()
	// Cascade W3 (RFC §B.3 row 9) — IfVersion=priorVersion gates the
	// write under the primitive's optimistic-lock check.
	// priorVersion=0 (legacy file predating cutover) upgrades via an
	// unconditional first-write that stamps Version=1. Conflict-path
	// returns core.Fail(paths.ConflictEnvelope{...}) directly via
	// writeRecord (Mantis #1544 gating shape inherited from W1+W2).
	if wr := writeRecord(rec, dirPath, priorVersion); !wr.OK {
		return wr
	}
	rec.Version = priorVersion + 1
	if rec.Version < 1 {
		rec.Version = 1
	}
	entry := toEntry(rec, core.Now())
	s.fireEvent(EventRehearsed, entry)
	return core.Ok(entry)
}
