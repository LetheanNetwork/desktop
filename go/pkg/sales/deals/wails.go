// SPDX-Licence-Identifier: EUPL-1.2

// Wails surface for the deals service. Methods are bound on the
// *Service receiver and exposed to the frontend via wails3 generate
// bindings — the TS shape lands at
// frontend/bindings/dappco.re/lthn/desktop/pkg/sales/deals/service.

package deals

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/paths"
)

// List returns all deals, optionally filtered by stage and/or owner.
// Results are sorted by CreatedAt descending. The activity log is
// omitted for performance — use Get for the full log.
//
// Usage example:
//
//	r := svc.List(deals.ListInput{Stage: "engage"})
//	if r.OK { out := r.Value.(deals.ListOutput) }
func (s *Service) List(input ListInput) core.Result {
	records, err := loadAll()
	if err != nil {
		return core.Fail(core.E("deals.List", "scan failed", err))
	}

	now := core.Now()
	all := make([]Deal, 0, len(records))
	for _, r := range records {
		all = append(all, toDeal(r, now, false))
	}
	total := len(all)

	limit := input.Limit
	if limit <= 0 {
		limit = 50
	}

	filtered := all[:0]
	rawFiltered := records[:0]
	for i, d := range all {
		if input.Stage != "" && records[i].Stage != input.Stage {
			continue
		}
		if input.Owner != "" && records[i].Owner != input.Owner {
			continue
		}
		filtered = append(filtered, d)
		rawFiltered = append(rawFiltered, records[i])
	}
	_ = rawFiltered // available for future sort-by-CreatedAt

	if len(filtered) > limit {
		filtered = filtered[:limit]
	}

	return core.Ok(ListOutput{
		Deals: filtered,
		Total: total,
	})
}

// Get returns a single deal by ID with the full activity log + docs.
//
// Usage example:
//
//	r := svc.Get(deals.GetInput{ID: "202605-DEAL-001"})
//	if r.OK { d := r.Value.(deals.Deal) }
func (s *Service) Get(input GetInput) core.Result {
	if err := paths.IsValidID(input.ID); err != nil {
		return core.Fail(err)
	}
	rec, err := loadOne(input.ID)
	if err != nil {
		return core.Fail(core.E("deals.Get", "not found: "+input.ID, err))
	}
	now := core.Now()
	return core.Ok(toDeal(rec, now, true))
}

// Create opens a new deal, writes the Trix file, and fires the
// sales.deals.created event.
//
// Usage example:
//
//	r := svc.Create(deals.CreateInput{
//	    Customer: "Heritage Law LLP", AmountPence: 24000,
//	    Stage: "engage", Owner: "Snider",
//	})
func (s *Service) Create(input CreateInput) core.Result {
	if input.Customer == "" {
		return core.Fail(core.E("deals.Create", "customer is required", nil))
	}
	if input.Stage == "" {
		input.Stage = "qual"
	}
	if !isValidStage(input.Stage) {
		return core.Fail(core.E("deals.Create", "unknown stage: "+input.Stage, nil))
	}

	dirR := dealsDir()
	if !dirR.OK {
		return core.Fail(core.E("deals.Create", dirR.Error(), nil))
	}
	dir := dirR.Value.(string)

	now := core.Now().UTC()
	// ID: {YYYYMM}-DEAL-{NNN}
	n := countDeals(dir) + 1
	id := core.Sprintf("%s-DEAL-%03d", core.TimeFormat(now, "200601"), n)

	rec := DealRecord{
		ID:             id,
		Customer:       input.Customer,
		AmountPence:    input.AmountPence,
		Stage:          input.Stage,
		ProbabilityPct: input.ProbabilityPct,
		CloseTarget:    input.CloseTarget,
		Owner:          input.Owner,
		Stakeholders:   input.Stakeholders,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	// Cascade W1 (Mantis #1540) — Create is an unconditional first-write
	// (ifVersion=0). writeRecord stamps Version=1 into the marshalled body.
	if wr := writeRecord(rec, dir, 0); !wr.OK {
		return wr
	}

	s.fireEvent(EventDealCreated, rec)
	return core.Ok(toDeal(rec, core.Now(), false))
}

// UpdateStage transitions a deal's pipeline stage. Fires the
// sales.deals.stage_changed event.
//
// Usage example:
//
//	r := svc.UpdateStage(deals.UpdateStageInput{
//	    ID: "202605-DEAL-001", Stage: "propose",
//	})
func (s *Service) UpdateStage(input UpdateStageInput) core.Result {
	if err := paths.IsValidID(input.ID); err != nil {
		return core.Fail(err)
	}
	if !isValidStage(input.Stage) {
		return core.Fail(core.E("deals.UpdateStage", "unknown stage: "+input.Stage, nil))
	}

	dirR := dealsDir()
	if !dirR.OK {
		return core.Fail(core.E("deals.UpdateStage", dirR.Error(), nil))
	}
	dir := dirR.Value.(string)

	rec, err := loadOne(input.ID)
	if err != nil {
		return core.Fail(core.E("deals.UpdateStage", "not found: "+input.ID, err))
	}

	priorVersion := rec.Version
	rec.Stage = input.Stage
	rec.UpdatedAt = core.Now().UTC()

	// Cascade W1 (Mantis #1540) — IfVersion=priorVersion gates the
	// write under the primitive's optimistic-lock check. priorVersion=0
	// (legacy file) skips the check and stamps Version=1 on the upgrade.
	// Conflict-path returns core.Fail(paths.ConflictEnvelope{...})
	// directly via writeRecord (Mantis #1544 gating W2).
	if wr := writeRecord(rec, dir, priorVersion); !wr.OK {
		return wr
	}

	s.fireEvent(EventDealStageChanged, rec)
	return core.Ok(toDeal(rec, core.Now(), true))
}

// AddActivity prepends a new activity-log entry (newest first) to the
// deal's log and writes back to disk.
//
// Usage example:
//
//	r := svc.AddActivity(deals.AddActivityInput{
//	    DealID: "202605-DEAL-001", K: "call", Who: "you",
//	    T: "30-min call · privacy posture, no blockers.",
//	})
func (s *Service) AddActivity(input AddActivityInput) core.Result {
	if err := paths.IsValidID(input.DealID); err != nil {
		return core.Fail(err)
	}
	validKinds := map[string]bool{"call": true, "email": true, "meet": true}
	if !validKinds[input.K] {
		return core.Fail(core.E("deals.AddActivity", "invalid kind: "+input.K, nil))
	}

	dirR := dealsDir()
	if !dirR.OK {
		return core.Fail(core.E("deals.AddActivity", dirR.Error(), nil))
	}
	dir := dirR.Value.(string)

	rec, err := loadOne(input.DealID)
	if err != nil {
		return core.Fail(core.E("deals.AddActivity", "not found: "+input.DealID, err))
	}

	entry := ActivityRecord{
		At:  core.Now().UTC(),
		K:   input.K,
		Who: input.Who,
		T:   input.T,
	}

	// Prepend (newest first).
	newLog := make([]ActivityRecord, 0, len(rec.Log)+1)
	newLog = append(newLog, entry)
	newLog = append(newLog, rec.Log...)
	rec.Log = newLog
	priorVersion := rec.Version
	rec.UpdatedAt = core.Now().UTC()

	// Cascade W1 (Mantis #1540) — same IfVersion discipline as
	// UpdateStage. Activity-log appends are still record-level writes.
	// Conflict-path returns core.Fail(paths.ConflictEnvelope{...})
	// directly via writeRecord (Mantis #1544 gating W2).
	if wr := writeRecord(rec, dir, priorVersion); !wr.OK {
		return wr
	}

	return core.Ok(toDeal(rec, core.Now(), true))
}
