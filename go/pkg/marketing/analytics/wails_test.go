// SPDX-Licence-Identifier: EUPL-1.2

package analytics_test

import (
	"testing"

	"dappco.re/lthn/desktop/pkg/marketing/analytics"
)

func TestGetInput_Fields_Good(t *testing.T) {
	in := analytics.GetInput{Window: analytics.Window30d}
	_ = in.Window
}

func TestGetOutput_Fields_Good(t *testing.T) {
	out := analytics.GetOutput{
		Window:   analytics.Window30d,
		Sessions: "48,412",
		Delta:    "+18 % vs prior 30 days",
		Spark:    "1.2,1.4,1.6",
		Sources:  []analytics.AnalyticsSource{},
		Pages:    []analytics.AnalyticsPage{},
	}
	_ = out.Window
	_ = out.Sessions
	_ = out.Delta
	_ = out.Spark
	_ = out.Sources
	_ = out.Pages
}

func TestAnalyticsSource_Fields_Good(t *testing.T) {
	src := analytics.AnalyticsSource{Label: "Direct", V: 38, N: "18.4 K"}
	_ = src.Label
	_ = src.V
	_ = src.N
}

func TestAnalyticsPage_Fields_Good(t *testing.T) {
	pg := analytics.AnalyticsPage{P: "/sovereign-compute", V: "8.8 K", B: "21%", D: "4:42"}
	_ = pg.P
	_ = pg.V
	_ = pg.B
	_ = pg.D
}
