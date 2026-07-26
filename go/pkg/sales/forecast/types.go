// SPDX-Licence-Identifier: EUPL-1.2

// Package forecast is the lthn-side quarterly forecast service. Derives
// ForecastRow and ForecastKpi values from deal files at
// ~/Lethean/sales/deals/{id}.md — no separate persistence file.
//
// Wire shapes match the ForecastRow + ForecastKpi interfaces consumed by
// the Angular forecast surface in the Sales category.
//
// Probability weights (v1 defaults):
//
//	qual     → 10%
//	engage   → 30%
//	propose  → 60%
//	close    → 85%
//	won      → 100%
//	lost     → 0%
//
// Committed = won deals only. Best case = committed + (60% of engaged
// + all proposal + close). Low case = committed + 30% of engaged.
//
// Usage example (Wails):
//
//	r := forecastSvc.Quarterly(forecast.QuarterlyInput{Quarters: 4})
//	if r.OK { out := r.Value.(forecast.QuarterlyOutput) }
package forecast

// ForecastRow is the JSON wire type for one per-quarter forecast row.
// Field names match ForecastRow in
// frontend-ng/src/app/desktop/surfaces/sales/forecast.ts exactly.
// Numbers are in £000 so the bar chart can scale all four lines against
// a single integer max.
//
// Usage example:
//
//	row := forecast.ForecastRow{
//	    Q: "Q2 · 2026", Target: 200, Committed: 142, Best: 188, Low: 108,
//	    Status: "open",
//	}
type ForecastRow struct {
	// Q is the display label ("Q2 · 2026").
	Q string `json:"q"`
	// Target is the internal quarter target in £000.
	Target int `json:"target"`
	// Committed is already-committed revenue in £000 (won deals).
	Committed int `json:"committed"`
	// Best is the best-case projection in £000.
	Best int `json:"best"`
	// Low is the low-case projection in £000.
	Low int `json:"low"`
	// Status is "open" (active quarter) or "plan" (future quarter).
	Status string `json:"status"`
}

// ForecastKpi is the JSON wire type for one headline KPI card.
// Field names match ForecastKpi in
// frontend-ng/src/app/desktop/surfaces/sales/forecast.ts exactly.
//
// Usage example:
//
//	kpi := forecast.ForecastKpi{L: "Y1 target ARR", V: "£420 K"}
type ForecastKpi struct {
	// L is the mono small-caps label ("Y1 target ARR").
	L string `json:"l"`
	// V is the pre-formatted value ("£420 K").
	V string `json:"v"`
}

// DealSummary is the lightweight internal deal shape used for forecast
// computation — parsed from frontmatter only, no body loaded.
//
// Usage example:
//
//	ds := forecast.DealSummary{AmountPence: 24000, Stage: "engage", ProbabilityPct: 60}
type DealSummary struct {
	// AmountPence is the deal value in pence.
	AmountPence int
	// Stage is the current pipeline stage id.
	Stage string
	// CloseTarget is the human close date ("14 Jun").
	CloseTarget string
	// ProbabilityPct overrides the stage default when non-zero.
	ProbabilityPct int
}

// QuarterlyInput drives the Quarterly method.
//
// Usage example:
//
//	r := svc.Quarterly(forecast.QuarterlyInput{Quarters: 4})
type QuarterlyInput struct {
	// Quarters is the number of quarters to include (default 4, max 8).
	Quarters int `json:"quarters,omitempty"`
}

// QuarterlyOutput is the Quarterly response envelope.
//
// Usage example:
//
//	out := r.Value.(forecast.QuarterlyOutput)
//	for _, row := range out.Rows { _ = row.Q }
type QuarterlyOutput struct {
	// Rows is the per-quarter forecast slice.
	Rows []ForecastRow `json:"rows"`
	// Kpis is the 4-element headline KPI slice.
	Kpis []ForecastKpi `json:"kpis"`
}
