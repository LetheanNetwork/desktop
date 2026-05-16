// SPDX-Licence-Identifier: EUPL-1.2

// Wails surface for the Office document catalogue service. Methods are
// bound on the *Service receiver and exposed to the frontend via wails3
// generate bindings — the TS shape lands at
// frontend/bindings/dappco.re/lthn/desktop/pkg/office/documents/.

package documents

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/paths"
)

// List returns the documents in ~/Lethean/office/docs/, optionally
// filtered by state. Results are sorted newest-first by modification
// time.
//
// Usage example:
//
//	r := svc.List(documents.ListInput{State: "draft"})
//	if r.OK { out := r.Value.(documents.ListOutput) }
func (s *Service) List(input ListInput) core.Result {
	records, err := scanDocs()
	if err != nil {
		return core.Fail(core.E("documents.List", "scan failed", err))
	}

	now := core.Now()
	all := make([]DocRow, 0, len(records))
	for _, r := range records {
		all = append(all, toRow(r, now))
	}
	total := len(all)

	limit := input.Limit
	if limit <= 0 {
		limit = 50
	}

	filtered := all[:0]
	for _, d := range all {
		if input.State != "" && d.State != input.State {
			continue
		}
		filtered = append(filtered, d)
	}
	if len(filtered) > limit {
		filtered = filtered[:limit]
	}

	return core.Ok(ListOutput{
		Docs:  filtered,
		Total: total,
	})
}

// Get returns a single document by slug with its full markdown body.
//
// Usage example:
//
//	r := svc.Get(documents.GetInput{Slug: "release-notes"})
//	if r.OK { d := r.Value.(documents.DocDetail) }
func (s *Service) Get(input GetInput) core.Result {
	if err := paths.IsValidID(input.Slug); err != nil {
		return core.Fail(core.E("documents.Get", "invalid slug", err))
	}

	raw, err := loadDoc(input.Slug)
	if err != nil {
		return core.Fail(core.E("documents.Get", "not found: "+input.Slug, err))
	}

	// Re-derive the record from raw bytes. We need the stat for mtime;
	// fetch it separately rather than keeping it from the scan.
	dirR := docsDir()
	if !dirR.OK {
		return core.Fail(core.E("documents.Get", dirR.Error(), nil))
	}
	fpath := core.PathJoin(dirR.Value.(string), input.Slug+".md")
	statR := core.Stat(fpath)
	var modTime core.Time
	var sizeB int64
	if statR.OK {
		if info, _ := statR.Value.(core.FsFileInfo); info != nil {
			modTime = info.ModTime()
			sizeB = info.Size()
		}
	}

	_, body := splitFrontmatter(raw)
	rec := parseDoc(input.Slug, raw, modTime, sizeB)

	return core.Ok(DocDetail{
		DocRow: toRow(rec, core.Now()),
		Body:   string(body),
	})
}
