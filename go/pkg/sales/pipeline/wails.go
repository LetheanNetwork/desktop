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

// MoveDeal transitions a deal to a new pipeline stage. Fires the
// sales.pipeline.moved event.
//
// Usage example:
//
//	r := svc.MoveDeal(pipeline.MoveInput{DealID: "202605-DEAL-001", ToStage: "propose"})
//	if r.OK { out := r.Value.(pipeline.ListOutput) }
func (s *Service) MoveDeal(input MoveInput) core.Result {
	if err := paths.IsValidID(input.DealID); err != nil {
		return core.Fail(err)
	}

	// Validate target stage.
	knownStage := false
	for _, spec := range stageOrder() {
		if spec.ID == input.ToStage {
			knownStage = true
			break
		}
	}
	if !knownStage {
		return core.Fail(core.E("pipeline.MoveDeal", "unknown stage: "+input.ToStage, nil))
	}

	fromStage, err := writeDealStage(input.DealID, input.ToStage)
	if err != nil {
		return core.Fail(core.E("pipeline.MoveDeal", "write stage", err))
	}

	s.fireMove(input.DealID, fromStage, input.ToStage)

	// Return the updated full pipeline view.
	return s.List(ListInput{})
}
