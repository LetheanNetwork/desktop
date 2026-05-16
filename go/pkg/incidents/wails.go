// SPDX-Licence-Identifier: EUPL-1.2

// Wails surface for the incidents service. Methods are bound on the
// *Service receiver and exposed to the frontend via wails3 generate
// bindings — the TS shape lands at
// frontend/bindings/dappco.re/lthn/desktop/pkg/incidents/service.

package incidents

import (
	core "dappco.re/go"
)

// List returns the incidents in the 90-day window, optionally filtered
// by state and/or severity. Results are sorted newest-first.
//
// Usage example:
//
//	r := svc.List(incidents.ListInput{State: "investigating"})
//	if r.OK { out := r.Value.(incidents.ListOutput) }
func (s *Service) List(input ListInput) core.Result {
	records, err := list90()
	if err != nil {
		return core.Fail(core.E("incidents.List", "scan failed", err))
	}

	now := core.Now()
	all := make([]IncidentEntry, 0, len(records))
	for _, r := range records {
		all = append(all, toEntry(r, now))
	}
	total := len(all)

	// Apply filters.
	limit := input.Limit
	if limit <= 0 {
		limit = 50
	}
	filtered := all[:0]
	for _, e := range all {
		if input.State != "" && e.State != input.State {
			continue
		}
		if input.Severity != "" && e.Sev != input.Severity {
			continue
		}
		filtered = append(filtered, e)
	}

	// Sort newest-first by TS is not reliable (it's relative text);
	// we rely on list90() reading directories in reverse-creation order.
	// Cap result set.
	if len(filtered) > limit {
		filtered = filtered[:limit]
	}

	return core.Ok(ListOutput{
		Incidents: filtered,
		Total:     total,
	})
}

// Get returns a single incident by ID with the full post-mortem body.
//
// Usage example:
//
//	r := svc.Get(incidents.GetInput{ID: "2026-05-INC-001"})
//	if r.OK { detail := r.Value.(incidents.IncidentDetail) }
func (s *Service) Get(input GetInput) core.Result {
	if input.ID == "" {
		return core.Fail(core.E("incidents.Get", "id is required", nil))
	}
	rec, _, err := loadOne(input.ID)
	if err != nil {
		return core.Fail(core.E("incidents.Get", "not found: "+input.ID, err))
	}
	now := core.Now()
	return core.Ok(IncidentDetail{
		IncidentEntry: toEntry(rec, now),
		PostMortem:    rec.PostMortem,
	})
}

// Create opens a new incident, writes the Trix file to
// ~/Lethean/incidents/{YYYY}/{MM}/{id}.md, and fires the
// incidents.opened event.
//
// Usage example:
//
//	r := svc.Create(incidents.CreateInput{
//	    Title: "hub · elevated p99", Sev: "P3", Svc: "hub", Who: "Mei",
//	})
func (s *Service) Create(input CreateInput) core.Result {
	if input.Title == "" {
		return core.Fail(core.E("incidents.Create", "title is required", nil))
	}
	if input.Sev == "" {
		input.Sev = "P3"
	}

	now := core.Now().UTC()
	dirR := yearMonthDir(now)
	if !dirR.OK {
		return core.Fail(core.E("incidents.Create", dirR.Error(), nil))
	}
	dirPath := dirR.Value.(string)

	// Generate ID: {YYYY}-{MM}-INC-{NNN}
	yr := core.TimeFormat(now, "2006")
	mo := core.TimeFormat(now, "01")
	n := countMonthFiles(dirPath) + 1
	id := core.Sprintf("%s-%s-INC-%03d", yr, mo, n)

	rec := IncidentRecord{
		ID:        id,
		Title:     input.Title,
		Sev:       input.Sev,
		State:     "investigating",
		Svc:       input.Svc,
		Who:       input.Who,
		Comments:  0,
		CreatedAt: now,
	}

	raw, err := marshalRecord(rec)
	if err != nil {
		return core.Fail(core.E("incidents.Create", "marshal", err))
	}
	target := core.PathJoin(dirPath, id+".md")
	if w := core.WriteFile(target, raw, 0o644); !w.OK {
		return core.Fail(core.E("incidents.Create", w.Error(), nil))
	}

	entry := toEntry(rec, now)
	s.fireEvent(EventOpened, entry)
	return core.Ok(entry)
}

// UpdateState transitions an incident's lifecycle state. When
// transitioning to "resolved", sets resolved_at and calculates
// dur_minutes. Fires the incidents.transitioned event.
//
// Usage example:
//
//	r := svc.UpdateState(incidents.UpdateStateInput{
//	    ID: "2026-05-INC-001", State: "resolved",
//	})
func (s *Service) UpdateState(input UpdateStateInput) core.Result {
	if input.ID == "" {
		return core.Fail(core.E("incidents.UpdateState", "id is required", nil))
	}
	validStates := map[string]bool{
		"investigating": true,
		"post-mortem":   true,
		"resolved":      true,
	}
	if !validStates[input.State] {
		return core.Fail(core.E("incidents.UpdateState", "invalid state: "+input.State, nil))
	}

	rec, dirPath, err := loadOne(input.ID)
	if err != nil {
		return core.Fail(core.E("incidents.UpdateState", "not found: "+input.ID, err))
	}

	rec.State = input.State
	if input.State == "resolved" && rec.ResolvedAt.IsZero() {
		rec.ResolvedAt = core.Now().UTC()
		if !rec.CreatedAt.IsZero() {
			dur := rec.ResolvedAt.Sub(rec.CreatedAt)
			rec.DurMinutes = int(dur.Minutes())
		}
	}

	if err := writeRecord(rec, dirPath); err != nil {
		return core.Fail(core.E("incidents.UpdateState", "write", err))
	}

	entry := toEntry(rec, core.Now())
	s.fireEvent(EventTransitioned, entry)
	return core.Ok(entry)
}

// AddPostmortem attaches a markdown post-mortem to a resolved or
// post-mortem incident. Returns core.Fail when the incident is still
// in "investigating" state.
//
// Usage example:
//
//	r := svc.AddPostmortem(incidents.PostmortemInput{
//	    ID: "2026-05-INC-001", Body: "## Root cause\n\nDNS TTL too low.",
//	})
func (s *Service) AddPostmortem(input PostmortemInput) core.Result {
	if input.ID == "" {
		return core.Fail(core.E("incidents.AddPostmortem", "id is required", nil))
	}

	rec, dirPath, err := loadOne(input.ID)
	if err != nil {
		return core.Fail(core.E("incidents.AddPostmortem", "not found: "+input.ID, err))
	}
	if rec.State == "investigating" {
		return core.Fail(core.E("incidents.AddPostmortem",
			"post-mortem requires state post-mortem or resolved, got: "+rec.State, nil))
	}

	rec.PostMortem = input.Body
	if err := writeRecord(rec, dirPath); err != nil {
		return core.Fail(core.E("incidents.AddPostmortem", "write", err))
	}

	return core.Ok(toEntry(rec, core.Now()))
}
