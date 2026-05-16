// SPDX-Licence-Identifier: EUPL-1.2

package pipeline_test

import (
	"testing"

	"dappco.re/lthn/desktop/pkg/sales/deals"
	"dappco.re/lthn/desktop/pkg/sales/pipeline"
)

// TestList_Empty_Good — empty deals dir → each stage has zero deals.
func TestList_Empty_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := pipeline.NewService(nil)
	r := svc.List(pipeline.ListInput{})
	if !r.OK {
		t.Fatalf("List failed: %s", r.Error())
	}
	out := r.Value.(pipeline.ListOutput)
	// All 6 stages should be present with 0 deals each.
	if len(out.Columns) != 6 {
		t.Fatalf("expected 6 columns, got %d", len(out.Columns))
	}
	for _, col := range out.Columns {
		if len(col.Deals) != 0 {
			t.Fatalf("column %q has %d deals, expected 0", col.ID, len(col.Deals))
		}
	}
	if out.TotalDeals != 0 {
		t.Fatalf("expected 0 total deals, got %d", out.TotalDeals)
	}
}

// TestList_GroupsByStage_Good — deals in two stages appear in correct columns.
func TestList_GroupsByStage_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dealSvc := deals.NewService(nil)
	dealSvc.Create(deals.CreateInput{Customer: "A", Stage: "engage", AmountPence: 10000})
	dealSvc.Create(deals.CreateInput{Customer: "B", Stage: "engage", AmountPence: 20000})
	dealSvc.Create(deals.CreateInput{Customer: "C", Stage: "qual", AmountPence: 5000})

	svc := pipeline.NewService(nil)
	r := svc.List(pipeline.ListInput{})
	if !r.OK {
		t.Fatalf("List failed: %s", r.Error())
	}
	out := r.Value.(pipeline.ListOutput)

	var engageCol *pipeline.PipelineColumn
	var qualCol *pipeline.PipelineColumn
	for i := range out.Columns {
		switch out.Columns[i].ID {
		case "engage":
			engageCol = &out.Columns[i]
		case "qual":
			qualCol = &out.Columns[i]
		}
	}
	if engageCol == nil || len(engageCol.Deals) != 2 {
		t.Fatalf("expected 2 deals in engage column")
	}
	if qualCol == nil || len(qualCol.Deals) != 1 {
		t.Fatalf("expected 1 deal in qual column")
	}
	if out.TotalDeals != 3 {
		t.Fatalf("expected 3 total deals, got %d", out.TotalDeals)
	}
}

// TestMoveDeal_UpdatesStage_Good — MoveDeal moves a deal to the target stage.
func TestMoveDeal_UpdatesStage_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dealSvc := deals.NewService(nil)
	dealSvc.Create(deals.CreateInput{Customer: "Heritage Law", Stage: "qual", AmountPence: 24000})

	// Get the deal ID.
	lr := dealSvc.List(deals.ListInput{})
	lo := lr.Value.(deals.ListOutput)
	dealID := lo.Deals[0].ID

	svc := pipeline.NewService(nil)
	r := svc.MoveDeal(pipeline.MoveInput{DealID: dealID, ToStage: "engage"})
	if !r.OK {
		t.Fatalf("MoveDeal failed: %s", r.Error())
	}

	// Verify via List.
	r2 := svc.List(pipeline.ListInput{Stage: "engage"})
	if !r2.OK {
		t.Fatalf("List failed: %s", r2.Error())
	}
	out := r2.Value.(pipeline.ListOutput)
	if len(out.Columns) != 1 || len(out.Columns[0].Deals) != 1 {
		t.Fatalf("expected 1 deal in engage column after move")
	}
}

// TestMoveDeal_InvalidStage_Bad — unknown stage returns core.Fail.
func TestMoveDeal_InvalidStage_Bad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := pipeline.NewService(nil)
	r := svc.MoveDeal(pipeline.MoveInput{DealID: "202605-DEAL-001", ToStage: "fantasy"})
	if r.OK {
		t.Fatalf("expected failure for invalid stage, got OK")
	}
}

// TestList_ValueAggregation_Good — amount_pence sums correctly into GBP string.
func TestList_ValueAggregation_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dealSvc := deals.NewService(nil)
	// Two engage deals: 24000p (£24) + 44000p (£44) = 68000p → "£68 K"
	dealSvc.Create(deals.CreateInput{Customer: "A", Stage: "engage", AmountPence: 24000})
	dealSvc.Create(deals.CreateInput{Customer: "B", Stage: "engage", AmountPence: 44000})

	svc := pipeline.NewService(nil)
	r := svc.List(pipeline.ListInput{Stage: "engage"})
	if !r.OK {
		t.Fatalf("List failed: %s", r.Error())
	}
	out := r.Value.(pipeline.ListOutput)
	if len(out.Columns) != 1 {
		t.Fatalf("expected 1 column for stage filter, got %d", len(out.Columns))
	}
	// 24000 + 44000 = 68000 pence → £68 K
	if out.Columns[0].Value != "£68 K" {
		t.Fatalf("expected £68 K, got %q", out.Columns[0].Value)
	}
}

// TestServiceName_Good — ServiceName returns "Pipeline".
func TestServiceName_Good(t *testing.T) {
	svc := pipeline.NewService(nil)
	if svc.ServiceName() != "Pipeline" {
		t.Fatalf("expected Pipeline, got %q", svc.ServiceName())
	}
}
