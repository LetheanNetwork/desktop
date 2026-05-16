// SPDX-Licence-Identifier: EUPL-1.2

package deals_test

import (
	"testing"

	"dappco.re/lthn/desktop/pkg/sales/deals"
)

func TestServiceName_Good(t *testing.T) {
	svc := deals.NewService(nil)
	if svc.ServiceName() != "Deals" {
		t.Fatalf("expected Deals, got %q", svc.ServiceName())
	}
}

func TestListInput_Fields_Good(t *testing.T) {
	in := deals.ListInput{Stage: "engage", Owner: "Snider", Limit: 10}
	_ = in.Stage
	_ = in.Owner
	_ = in.Limit
}

func TestCreateInput_Fields_Good(t *testing.T) {
	in := deals.CreateInput{
		Customer: "Heritage Law", AmountPence: 24000,
		Stage: "engage", Owner: "Snider",
		Stakeholders: []string{"Ada Penley · CTO"},
	}
	_ = in.Customer
	_ = in.AmountPence
	_ = in.Stage
	_ = in.Owner
	_ = in.Stakeholders
}

func TestUpdateStageInput_Fields_Good(t *testing.T) {
	in := deals.UpdateStageInput{ID: "202605-DEAL-001", Stage: "propose"}
	_ = in.ID
	_ = in.Stage
}

func TestAddActivityInput_Fields_Good(t *testing.T) {
	in := deals.AddActivityInput{
		DealID: "202605-DEAL-001", K: "call", Who: "you",
		T: "30-min call with Ada Penley",
	}
	_ = in.DealID
	_ = in.K
	_ = in.Who
	_ = in.T
}

func TestListOutput_Fields_Good(t *testing.T) {
	out := deals.ListOutput{Deals: []deals.Deal{}, Total: 0}
	_ = out.Deals
	_ = out.Total
}
