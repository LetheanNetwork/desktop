// SPDX-Licence-Identifier: EUPL-1.2

// Wails surface for the pipeline service. Methods are bound on the
// *Service receiver and exposed to the frontend via wails3 generate
// bindings — the TS shape lands at
// frontend/bindings/dappco.re/lthn/desktop/pkg/sales/pipeline/service.

package pipeline

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/paths"
)

// List returns the pipeline grouped into PipelineColumn values, one per
// stage. When input.Stage is non-empty, returns only that column.
//
// Usage example:
//
//	r := svc.List(pipeline.ListInput{})
//	if r.OK { out := r.Value.(pipeline.ListOutput) }
func (s *Service) List(input ListInput) core.Result {
	fms, err := loadDeals()
	if err != nil {
		return core.Fail(core.E("pipeline.List", "scan failed", err))
	}

	stages := stageOrder()

	// Group deals by stage.
	byStage := make(map[string][]dealFrontmatter, len(stages))
	for _, spec := range stages {
		byStage[spec.ID] = nil
	}
	for _, fm := range fms {
		byStage[fm.Stage] = append(byStage[fm.Stage], fm)
	}

	// Build columns.
	cols := make([]PipelineColumn, 0, len(stages))
	totalPence := 0
	totalDeals := 0

	for _, spec := range stages {
		if input.Stage != "" && spec.ID != input.Stage {
			continue
		}
		dms := byStage[spec.ID]
		pipelineDeals := make([]PipelineDeal, 0, len(dms))
		stagePence := 0
		for _, fm := range dms {
			stagePence += fm.AmountPence
			pipelineDeals = append(pipelineDeals, PipelineDeal{
				C:  fm.Customer,
				V:  formatGBPK(fm.AmountPence),
				T:  fm.CloseTarget,
				ID: fm.ID,
			})
		}
		if spec.ID != "lost" {
			totalPence += stagePence
			totalDeals += len(dms)
		}
		cols = append(cols, PipelineColumn{
			ID:    spec.ID,
			Label: spec.Label,
			Value: formatGBPK(stagePence),
			Deals: pipelineDeals,
		})
	}

	return core.Ok(ListOutput{
		Columns:    cols,
		TotalValue: formatGBPK(totalPence),
		TotalDeals: totalDeals,
	})
}

// MoveDeal transitions a deal to a new pipeline stage. Cerberus
// pass-9 HIGH (Mantis #1488) — every move is validated against
// LegalTransitions to defend against prompt-injection silently
// flipping deals to won/lost via Vi-driven signals. Every call
// emits a structured audit event via core.Print regardless of
// outcome so Stage F log-tailing has a complete trail.
//
// Validation order (each step short-circuits on failure):
//
//  1. paths.IsValidID(dealId) — defends against path traversal
//     (Cerberus #1486).
//  2. Target stage MUST exist in stageOrder() (unknown_stage).
//  3. Read current stage from disk (deal_not_found if missing).
//  4. IsLegalTransition(from, to) — defends against illegal
//     transitions (transition_illegal). Self-transitions and
//     skip-levels reject here.
//  5. Persist via writeDealStage + fire the typed event.
//
// Audit event schema (reserved per RFC.stage-f §6 M4 conventions):
//
//	event=sales.pipeline.move_attempted
//	  deal_id=... from_stage=... to_stage=... outcome=... reason=...
//
// outcome ∈ {ok, transition_illegal, deal_not_found,
// unknown_stage, invalid_id}. The audit ALWAYS emits — successful
// AND rejected moves both leave a trail so Stage F can spot a
// pattern of rejected transitions (likely-bug or active probe).
//
// Usage example:
//
//	r := svc.MoveDeal(pipeline.MoveInput{DealID: "202605-DEAL-001", ToStage: "propose"})
//	if r.OK { out := r.Value.(pipeline.ListOutput) }
func (s *Service) MoveDeal(input MoveInput) core.Result {
	if err := paths.IsValidID(input.DealID); err != nil {
		s.auditMoveAttempt(input.DealID, "", input.ToStage, "invalid_id", err.Error())
		return core.Fail(err)
	}

	// Validate target stage exists in the canonical list.
	knownStage := false
	for _, spec := range stageOrder() {
		if spec.ID == input.ToStage {
			knownStage = true
			break
		}
	}
	if !knownStage {
		s.auditMoveAttempt(input.DealID, "", input.ToStage, "unknown_stage",
			"target stage is not in the canonical stage set")
		return core.Fail(core.NewCode("pipeline.unknown_stage",
			"unknown stage: "+input.ToStage))
	}

	// Read the current stage BEFORE the legality check — we need
	// the from-stage to make the decision.
	fromStage, readErr := readDealStage(input.DealID)
	if readErr != nil {
		s.auditMoveAttempt(input.DealID, "", input.ToStage, "deal_not_found", readErr.Error())
		return core.Fail(core.NewCode("pipeline.deal_not_found",
			"deal not found: "+input.DealID))
	}

	// Legality check — the core defence per Mantis #1488. Rejects
	// every transition not enumerated in LegalTransitions, including
	// self-transitions, skip-levels, and any move out of a terminal
	// (won/lost) stage.
	if !IsLegalTransition(fromStage, input.ToStage) {
		s.auditMoveAttempt(input.DealID, fromStage, input.ToStage,
			"transition_illegal", "from→to not in LegalTransitions")
		return core.Fail(core.NewCode("transition_illegal",
			core.Sprintf("illegal transition: %s → %s (Mantis #1488)",
				fromStage, input.ToStage)))
	}

	// Persist + fire the typed event. writeDealStage re-reads the
	// frontmatter to mutate-in-place; we already have the prior
	// fromStage so the returned value is redundant but kept for
	// signature symmetry.
	if _, err := writeDealStage(input.DealID, input.ToStage); err != nil {
		s.auditMoveAttempt(input.DealID, fromStage, input.ToStage,
			"write_failed", err.Error())
		return core.Fail(core.E("pipeline.MoveDeal", "write stage", err))
	}

	s.fireMove(input.DealID, fromStage, input.ToStage)
	s.auditMoveAttempt(input.DealID, fromStage, input.ToStage, "ok", "")

	// Return the updated full pipeline view.
	return s.List(ListInput{})
}

// auditMoveAttempt emits the sales.pipeline.move_attempted event
// per the Cerberus #1488 audit-prep contract. The schema is
// reserved (Stage F log-tailing depends on the literal field
// names); changes after Stage 5 ships break the contract.
//
// reason is empty for outcome=ok; non-empty for every rejection
// path so the log-tailer can categorise without re-deriving from
// the response code.
func (s *Service) auditMoveAttempt(dealID, fromStage, toStage, outcome, reason string) {
	core.Print(core.Stderr(),
		"event=sales.pipeline.move_attempted deal_id=%s from_stage=%s to_stage=%s ts=%d outcome=%s reason=%s\n",
		dealID, fromStage, toStage, core.Now().UTC().Unix(), outcome, reason)
}

// readDealStage reads the current stage frontmatter field for the
// supplied deal id. Returns ("", err) when the file is missing or
// unparseable. paths.IsValidID MUST run before this — the function
// joins the id into a path without re-validating.
func readDealStage(id string) (string, error) {
	dirR := dealsDir()
	if !dirR.OK {
		return "", core.E("pipeline.readDealStage", dirR.Error(), nil)
	}
	dir := dirR.Value.(string)
	fpath := core.PathJoin(dir, id+".md")

	raw := core.ReadFile(fpath)
	if !raw.OK {
		return "", core.E("pipeline.readDealStage", "not found: "+id, nil)
	}
	fm, err := parseFrontmatter(raw.Value.([]byte))
	if err != nil {
		return "", err
	}
	return fm.Stage, nil
}
