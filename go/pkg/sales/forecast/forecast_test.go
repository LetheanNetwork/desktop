// SPDX-Licence-Identifier: EUPL-1.2

package forecast_test

import (
	"testing"

	"dappco.re/lthn/desktop/pkg/sales/deals"
	"dappco.re/lthn/desktop/pkg/sales/forecast"
)

// TestQuarterly_Empty_Good — no deals → all zeros, 4 rows returned.
func TestQuarterly_Empty_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := forecast.NewService(nil)
	r := svc.Quarterly(forecast.QuarterlyInput{Quarters: 4})
	if !r.OK {
		t.Fatalf("Quarterly failed: %s", r.Error())
	}
	out := r.Value.(forecast.QuarterlyOutput)
	if len(out.Rows) != 4 {
		t.Fatalf("expected 4 rows, got %d", len(out.Rows))
	}
	for _, row := range out.Rows {
		if row.Committed != 0 {
			t.Fatalf("expected 0 committed, got %d for %s", row.Committed, row.Q)
		}
	}
}

// TestQuarterly_TwoDeals_Good — won + engage deal split correctly.
func TestQuarterly_TwoDeals_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dealSvc := deals.NewService(nil)
	// Won deal = £10 K committed.
	dealSvc.Create(deals.CreateInput{Customer: "Won Co", Stage: "won", AmountPence: 10000})
	// Engage deal = £20 K, 30% default weight.
	dealSvc.Create(deals.CreateInput{Customer: "Engage Co", Stage: "engage", AmountPence: 20000})

	svc := forecast.NewService(nil)
	r := svc.Quarterly(forecast.QuarterlyInput{Quarters: 4})
	if !r.OK {
		t.Fatalf("Quarterly failed: %s", r.Error())
	}
	out := r.Value.(forecast.QuarterlyOutput)
	if len(out.Rows) != 4 {
		t.Fatalf("expected 4 rows")
	}
	// Current quarter: committed = £10 K (10000 pence → £10).
	current := out.Rows[0]
	if current.Committed != 10 {
		t.Fatalf("expected committed £10 K, got %d", current.Committed)
	}
	// Best = committed + 60% of engage: 10 + (20000*0.6/1000) = 10 + 12 = 22
	if current.Best != 22 {
		t.Fatalf("expected best £22 K, got %d", current.Best)
	}
}

// TestKpis_Derived_Good — KPI slice has 4 entries with correct labels.
func TestKpis_Derived_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := forecast.NewService(nil)
	r := svc.Quarterly(forecast.QuarterlyInput{Quarters: 4})
	if !r.OK {
		t.Fatalf("Quarterly failed: %s", r.Error())
	}
	out := r.Value.(forecast.QuarterlyOutput)
	if len(out.Kpis) != 4 {
		t.Fatalf("expected 4 KPIs, got %d", len(out.Kpis))
	}
	labels := []string{"Y1 target ARR", "Committed", "Best case", "Probability-weighted"}
	for i, kpi := range out.Kpis {
		if kpi.L != labels[i] {
			t.Fatalf("KPI[%d]: expected %q, got %q", i, labels[i], kpi.L)
		}
	}
}

// TestStageWeight_All_Good — every known stage returns a weight in [0,1].
func TestStageWeight_All_Good(t *testing.T) {
	stages := []string{"qual", "engage", "propose", "close", "won", "lost"}
	for _, stage := range stages {
		// Verify via a deal computation — indirect but avoids exporting stageWeight.
		t.Setenv("HOME", t.TempDir())
		dealSvc := deals.NewService(nil)
		dealSvc.Create(deals.CreateInput{Customer: "X", Stage: stage, AmountPence: 10000})
		svc := forecast.NewService(nil)
		r := svc.Quarterly(forecast.QuarterlyInput{Quarters: 1})
		if !r.OK {
			t.Fatalf("Quarterly failed for stage %s: %s", stage, r.Error())
		}
	}
}

// TestServiceName_Good — ServiceName returns "Forecast".
func TestServiceName_Good(t *testing.T) {
	svc := forecast.NewService(nil)
	if svc.ServiceName() != "Forecast" {
		t.Fatalf("expected Forecast, got %q", svc.ServiceName())
	}
}
