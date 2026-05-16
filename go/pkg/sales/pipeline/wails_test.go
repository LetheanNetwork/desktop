// SPDX-Licence-Identifier: EUPL-1.2

package pipeline_test

import (
	"testing"

	"dappco.re/lthn/desktop/pkg/sales/pipeline"
)

func TestListInput_Fields_Good(t *testing.T) {
	in := pipeline.ListInput{Stage: "engage"}
	_ = in.Stage
}

func TestMoveInput_Fields_Good(t *testing.T) {
	in := pipeline.MoveInput{DealID: "202605-DEAL-001", ToStage: "propose"}
	_ = in.DealID
	_ = in.ToStage
}

func TestListOutput_Fields_Good(t *testing.T) {
	out := pipeline.ListOutput{
		Columns:    []pipeline.PipelineColumn{},
		TotalValue: "£0",
		TotalDeals: 0,
	}
	_ = out.Columns
	_ = out.TotalValue
	_ = out.TotalDeals
}

func TestPipelineColumn_Fields_Good(t *testing.T) {
	col := pipeline.PipelineColumn{
		ID:    "engage",
		Label: "Engaging",
		Value: "£128 K",
		Deals: []pipeline.PipelineDeal{},
	}
	_ = col.ID
	_ = col.Label
	_ = col.Value
	_ = col.Deals
}

func TestPipelineDeal_Fields_Good(t *testing.T) {
	d := pipeline.PipelineDeal{
		C:  "Heritage Law LLP",
		V:  "£24 K",
		T:  "GDPR + privilege",
		ID: "202605-DEAL-001",
	}
	_ = d.C
	_ = d.V
	_ = d.T
	_ = d.ID
}
