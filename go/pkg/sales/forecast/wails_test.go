// SPDX-Licence-Identifier: EUPL-1.2

package forecast_test

import (
	"testing"

	"dappco.re/lthn/desktop/pkg/sales/forecast"
)

func TestQuarterlyInput_Fields_Good(t *testing.T) {
	in := forecast.QuarterlyInput{Quarters: 4}
	_ = in.Quarters
}

func TestQuarterlyOutput_Fields_Good(t *testing.T) {
	out := forecast.QuarterlyOutput{
		Rows: []forecast.ForecastRow{},
		Kpis: []forecast.ForecastKpi{},
	}
	_ = out.Rows
	_ = out.Kpis
}

func TestForecastRow_Fields_Good(t *testing.T) {
	row := forecast.ForecastRow{
		Q: "Q2 · 2026", Target: 200, Committed: 142,
		Best: 188, Low: 108, Status: "open",
	}
	_ = row.Q
	_ = row.Target
	_ = row.Committed
	_ = row.Best
	_ = row.Low
	_ = row.Status
}

func TestForecastKpi_Fields_Good(t *testing.T) {
	kpi := forecast.ForecastKpi{L: "Y1 target ARR", V: "£420 K"}
	_ = kpi.L
	_ = kpi.V
}
