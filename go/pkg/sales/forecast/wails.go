// SPDX-Licence-Identifier: EUPL-1.2

// Wails surface for the forecast service. Methods are bound on the
// *Service receiver and exposed to the frontend via wails3 generate
// bindings — the TS shape lands at
// frontend/bindings/dappco.re/lthn/desktop/pkg/sales/forecast/service.

package forecast

import (
	core "dappco.re/go"
)

// Quarterly derives a probability-weighted quarterly forecast from the
// deal files in ~/Lethean/sales/deals/. Returns ForecastRow[] + ForecastKpi[].
// Fires no events — forecast is read-only.
//
// Usage example:
//
//	r := svc.Quarterly(forecast.QuarterlyInput{Quarters: 4})
//	if r.OK { out := r.Value.(forecast.QuarterlyOutput) }
func (s *Service) Quarterly(input QuarterlyInput) core.Result {
	n := input.Quarters
	if n <= 0 {
		n = 4
	}
	if n > 8 {
		n = 8
	}

	summaries, err := loadAllDeals()
	if err != nil {
		return core.Fail(core.E("forecast.Quarterly", "load deals", err))
	}

	now := core.Now()
	rows := buildRows(summaries, n, now)
	kpis := buildKpis(rows, summaries)

	return core.Ok(QuarterlyOutput{
		Rows: rows,
		Kpis: kpis,
	})
}
