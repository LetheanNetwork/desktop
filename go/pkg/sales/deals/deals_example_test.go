// SPDX-Licence-Identifier: EUPL-1.2

package deals_test

import (
	"dappco.re/lthn/desktop/pkg/sales/deals"
)

// ExampleService_List shows the standard list-all invocation.
func ExampleService_List() {
	svc := deals.NewService(nil)
	r := svc.List(deals.ListInput{})
	if !r.OK {
		return
	}
	_ = r.Value.(deals.ListOutput)
}

// ExampleService_Create shows a new deal creation.
func ExampleService_Create() {
	svc := deals.NewService(nil)
	r := svc.Create(deals.CreateInput{
		Customer:    "Heritage Law LLP",
		AmountPence: 24000,
		Stage:       "engage",
		Owner:       "Snider",
	})
	if !r.OK {
		return
	}
	_ = r.Value.(deals.Deal)
}

// ExampleService_AddActivity shows appending an activity log entry.
func ExampleService_AddActivity() {
	svc := deals.NewService(nil)
	r := svc.AddActivity(deals.AddActivityInput{
		DealID: "202605-DEAL-001",
		K:      "call",
		Who:    "you",
		T:      "30-min call with Ada Penley · privacy posture, no blockers.",
	})
	if !r.OK {
		return
	}
	_ = r.Value.(deals.Deal)
}
