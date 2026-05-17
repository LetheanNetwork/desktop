// SPDX-Licence-Identifier: EUPL-1.2

// Wails surface for the content service. Methods are bound on the
// *Service receiver and exposed to the frontend via wails3 generate
// bindings — the TS shape lands at
// frontend/bindings/dappco.re/lthn/desktop/pkg/marketing/content/service.

package content

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/paths"
)

// List returns the content calendar grouped into ContentColumn values.
// When input.Col is non-empty, returns only that column.
//
// Usage example:
//
//	r := svc.List(content.ListInput{})
//	if r.OK { out := r.Value.(content.ListOutput) }
func (s *Service) List(input ListInput) core.Result {
	items, err := loadItems()
	if err != nil {
		return core.Fail(core.E("content.List", "scan failed", err))
	}

	// Compute totals from the full item set.
	totalInFlight := 0
	dueToday := 0
	for _, item := range items {
		if item.Col != "live" {
			totalInFlight++
		}
		if item.Due == "today" {
			dueToday++
		}
	}

	// Group by column.
	byCol := make(map[string][]ContentItem)
	for _, item := range items {
		byCol[item.Col] = append(byCol[item.Col], item)
	}

	cols := columnOrder()
	result := make([]ContentColumn, 0, len(cols))
	for _, spec := range cols {
		if input.Col != "" && spec.ID != input.Col {
			continue
		}
		colItems := byCol[spec.ID]
		if colItems == nil {
			colItems = []ContentItem{}
		}
		result = append(result, ContentColumn{
			ID:    spec.ID,
			Label: spec.Label,
			Items: colItems,
		})
	}

	return core.Ok(ListOutput{
		Columns:       result,
		TotalInFlight: totalInFlight,
		DueToday:      dueToday,
	})
}

// Get returns a single content item by ID.
//
// Usage example:
//
//	r := svc.Get("v02-release-notes-20260516")
//	if r.OK { item := r.Value.(content.ContentItem) }
func (s *Service) Get(id string) core.Result {
	if err := paths.IsValidID(id); err != nil {
		return core.Fail(err)
	}
	dirR := contentDir()
	if !dirR.OK {
		return core.Fail(core.E("content.Get", dirR.Error(), nil))
	}
	// Cerberus #1486 belt: WithinDir check after the join.
	fpath, jerr := paths.JoinAndCheck(dirR.Value.(string), id+".md")
	if jerr != nil {
		return core.Fail(jerr)
	}
	raw := core.ReadFile(fpath)
	if !raw.OK {
		return core.Fail(core.E("content.Get", "not found: "+id, nil))
	}
	item, err := parseItem(raw.Value.([]byte))
	if err != nil {
		return core.Fail(core.E("content.Get", "parse failed", err))
	}
	return core.Ok(item)
}

// Create creates a new content item in the given pipeline column.
//
// Usage example:
//
//	r := svc.Create(content.CreateInput{T: "v0.2 release notes", Who: "you", Due: "today"})
//	if r.OK { item := r.Value.(content.ContentItem) }
func (s *Service) Create(input CreateInput) core.Result {
	if fail, ok := s.assertUnlocked("content.Create"); !ok {
		return fail
	}
	if input.T == "" {
		return core.Fail(core.E("content.Create", "title (t) is required", nil))
	}
	dirR := contentDir()
	if !dirR.OK {
		return core.Fail(core.E("content.Create", dirR.Error(), nil))
	}

	col := input.Col
	if col == "" {
		col = "idea"
	}

	ts := core.Now().UTC().Unix()
	slug := slugifyContent(input.T)
	if slug == "" {
		slug = "item"
	}
	id := core.Sprintf("%s-%d", slug, ts)

	item := ContentItem{
		ID:   id,
		T:    input.T,
		Who:  input.Who,
		Due:  input.Due,
		Col:  col,
		Body: input.Body,
	}

	// Cascade W2 (RFC §B.3 row 4) — Create is an unconditional first-
	// write (ifVersion=0). writeItem stamps Version=1 into the
	// marshalled frontmatter. Conflict-path (rare on Create — only
	// fires if another goroutine races on the same slug) returns
	// core.Fail(paths.ConflictEnvelope{...}) directly via writeItem.
	if wr := writeItem(dirR.Value.(string), item, 0); !wr.OK {
		return wr
	}
	item.Version = 1
	s.fireContentEvent(EventContentCreated, id, col)
	return core.Ok(item)
}

// Advance moves a content item to the next pipeline column.
// Returns core.Fail if the item is already at "live".
//
// Usage example:
//
//	r := svc.Advance("v02-release-notes-20260516")
//	if r.OK { item := r.Value.(content.ContentItem) }
func (s *Service) Advance(id string) core.Result {
	if fail, ok := s.assertUnlocked("content.Advance"); !ok {
		return fail
	}
	if err := paths.IsValidID(id); err != nil {
		return core.Fail(err)
	}
	dirR := contentDir()
	if !dirR.OK {
		return core.Fail(core.E("content.Advance", dirR.Error(), nil))
	}

	// Cerberus #1486 belt: WithinDir check after the join.
	fpath, jerr := paths.JoinAndCheck(dirR.Value.(string), id+".md")
	if jerr != nil {
		return core.Fail(jerr)
	}
	raw := core.ReadFile(fpath)
	if !raw.OK {
		return core.Fail(core.E("content.Advance", "not found: "+id, nil))
	}
	item, err := parseItem(raw.Value.([]byte))
	if err != nil {
		return core.Fail(core.E("content.Advance", "parse failed", err))
	}

	next := nextCol(item.Col)
	if next == "" {
		return core.Fail(core.E("content.Advance", "already at live: "+id, nil))
	}

	priorVersion := item.Version
	item.Col = next
	// When advancing to "live", record the publish time.
	if next == "live" {
		item.When = "just now"
		item.Due = ""
	}

	// Cascade W2 (RFC §B.3 row 4) — IfVersion=priorVersion gates the
	// write under the primitive's optimistic-lock check.
	// priorVersion=0 (legacy file predating cutover) skips the check
	// and stamps Version=1 on the upgrade write. Conflict-path returns
	// core.Fail(paths.ConflictEnvelope{...}) directly via writeItem
	// (Mantis #1544 gating shape inherited from W1).
	if wr := writeItem(dirR.Value.(string), item, priorVersion); !wr.OK {
		return wr
	}
	item.Version = priorVersion + 1
	if item.Version < 1 {
		item.Version = 1
	}
	s.fireContentEvent(EventContentAdvanced, id, next)
	return core.Ok(item)
}
