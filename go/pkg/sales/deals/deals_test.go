// SPDX-Licence-Identifier: EUPL-1.2

package deals_test

import (
	"testing"

	"dappco.re/lthn/desktop/pkg/sales/deals"
)

func TestCreate_WritesFile_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := deals.NewService(nil)
	r := svc.Create(deals.CreateInput{
		Customer:    "Heritage Law LLP",
		AmountPence: 24000,
		Stage:       "engage",
		Owner:       "Snider",
	})
	if !r.OK {
		t.Fatalf("Create failed: %s", r.Error())
	}
	d := r.Value.(deals.Deal)
	if d.Customer != "Heritage Law LLP" {
		t.Fatalf("expected Heritage Law LLP, got %q", d.Customer)
	}
	if d.Stage != "Engaging" {
		t.Fatalf("expected Engaging, got %q", d.Stage)
	}
}

func TestList_Empty_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := deals.NewService(nil)
	r := svc.List(deals.ListInput{})
	if !r.OK {
		t.Fatalf("List failed: %s", r.Error())
	}
	out := r.Value.(deals.ListOutput)
	if len(out.Deals) != 0 {
		t.Fatalf("expected 0 deals, got %d", len(out.Deals))
	}
}

func TestList_FiltersByStage_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := deals.NewService(nil)
	svc.Create(deals.CreateInput{Customer: "A", Stage: "engage", AmountPence: 10000})
	svc.Create(deals.CreateInput{Customer: "B", Stage: "qual", AmountPence: 5000})

	r := svc.List(deals.ListInput{Stage: "engage"})
	if !r.OK {
		t.Fatalf("List failed: %s", r.Error())
	}
	out := r.Value.(deals.ListOutput)
	if len(out.Deals) != 1 {
		t.Fatalf("expected 1 deal, got %d", len(out.Deals))
	}
	if out.Deals[0].Customer != "A" {
		t.Fatalf("expected A, got %q", out.Deals[0].Customer)
	}
}

func TestUpdateStage_Transitions_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := deals.NewService(nil)
	svc.Create(deals.CreateInput{Customer: "Heritage Law", Stage: "engage", AmountPence: 24000})

	// Find the created deal ID via List.
	lr := svc.List(deals.ListInput{})
	if !lr.OK {
		t.Fatalf("List failed: %s", lr.Error())
	}
	out := lr.Value.(deals.ListOutput)
	if len(out.Deals) != 1 {
		t.Fatalf("expected 1 deal")
	}
	id := out.Deals[0].ID

	r := svc.UpdateStage(deals.UpdateStageInput{ID: id, Stage: "propose"})
	if !r.OK {
		t.Fatalf("UpdateStage failed: %s", r.Error())
	}
	d := r.Value.(deals.Deal)
	if d.Stage != "Proposal" {
		t.Fatalf("expected Proposal, got %q", d.Stage)
	}
}

func TestUpdateStage_InvalidStage_Bad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := deals.NewService(nil)
	svc.Create(deals.CreateInput{Customer: "X", Stage: "engage", AmountPence: 1000})

	lr := svc.List(deals.ListInput{})
	out := lr.Value.(deals.ListOutput)
	id := out.Deals[0].ID

	r := svc.UpdateStage(deals.UpdateStageInput{ID: id, Stage: "magic"})
	if r.OK {
		t.Fatalf("expected failure for invalid stage, got OK")
	}
}

func TestAddActivity_Prepends_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := deals.NewService(nil)
	svc.Create(deals.CreateInput{Customer: "Heritage Law", Stage: "engage", AmountPence: 24000})

	lr := svc.List(deals.ListInput{})
	out := lr.Value.(deals.ListOutput)
	id := out.Deals[0].ID

	svc.AddActivity(deals.AddActivityInput{DealID: id, K: "email", Who: "you", T: "first"})
	r := svc.AddActivity(deals.AddActivityInput{DealID: id, K: "call", Who: "you", T: "second"})
	if !r.OK {
		t.Fatalf("AddActivity failed: %s", r.Error())
	}
	d := r.Value.(deals.Deal)
	if len(d.Log) != 2 {
		t.Fatalf("expected 2 log entries, got %d", len(d.Log))
	}
	if d.Log[0].T != "second" {
		t.Fatalf("expected newest first (second), got %q", d.Log[0].T)
	}
}

func TestGet_ReturnsFullLog_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := deals.NewService(nil)
	svc.Create(deals.CreateInput{Customer: "Heritage Law", Stage: "engage", AmountPence: 24000})

	lr := svc.List(deals.ListInput{})
	out := lr.Value.(deals.ListOutput)
	id := out.Deals[0].ID

	svc.AddActivity(deals.AddActivityInput{DealID: id, K: "call", Who: "you", T: "call one"})

	r := svc.Get(deals.GetInput{ID: id})
	if !r.OK {
		t.Fatalf("Get failed: %s", r.Error())
	}
	d := r.Value.(deals.Deal)
	if len(d.Log) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(d.Log))
	}
}

func TestAddActivity_InvalidKind_Bad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := deals.NewService(nil)
	svc.Create(deals.CreateInput{Customer: "X", Stage: "engage", AmountPence: 1000})
	lr := svc.List(deals.ListInput{})
	out := lr.Value.(deals.ListOutput)
	id := out.Deals[0].ID

	r := svc.AddActivity(deals.AddActivityInput{DealID: id, K: "tweet", Who: "you", T: "bad"})
	if r.OK {
		t.Fatalf("expected failure for invalid kind, got OK")
	}
}

func TestCreate_EmptyCustomer_Ugly(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := deals.NewService(nil)
	r := svc.Create(deals.CreateInput{Customer: "", Stage: "engage"})
	if r.OK {
		t.Fatalf("expected failure for empty customer, got OK")
	}
}
