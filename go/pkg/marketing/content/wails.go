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
	raw := core.ReadFile(core.PathJoin(dirR.Value.(string), id+".md"))
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

	if r := writeItem(dirR.Value.(string), item); !r.OK {
		return core.Fail(core.E("content.Create", "write failed", nil))
	}
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
	if err := paths.IsValidID(id); err != nil {
		return core.Fail(err)
	}
	dirR := contentDir()
	if !dirR.OK {
		return core.Fail(core.E("content.Advance", dirR.Error(), nil))
	}

	raw := core.ReadFile(core.PathJoin(dirR.Value.(string), id+".md"))
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

	item.Col = next
	// When advancing to "live", record the publish time.
	if next == "live" {
		item.When = "just now"
		item.Due = ""
	}

	if r := writeItem(dirR.Value.(string), item); !r.OK {
		return core.Fail(core.E("content.Advance", "write failed", nil))
	}
	s.fireContentEvent(EventContentAdvanced, id, next)
	return core.Ok(item)
}
