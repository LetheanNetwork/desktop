// SPDX-Licence-Identifier: EUPL-1.2

// Package analytics is the lthn-side web analytics rollup service.
// Derives session / source / page metrics from the self-hosted Plausible
// install. Read-only: no files written to ~/Lethean/.
//
// When LTHN_PLAUSIBLE_URL is configured, proxies live Plausible Stats API
// data. When unconfigured, returns fixture data mirroring the frontend
// constants so the view renders correctly in preview mode.
//
// Wire shapes match the Source + PageRow interfaces consumed by the
// Angular analytics surface in the Marketing category.
//
// Usage example (Wails):
//
//	r := analyticsSvc.Get(analytics.GetInput{Window: "30d"})
//	if r.OK { out := r.Value.(analytics.GetOutput) }
package analytics

// AnalyticsSource is the JSON wire type for one traffic source row.
// Field names match the Source interface in
// frontend/src/app/desktop/surfaces/marketing/analytics.ts.
//
// Usage example:
//
//	src := analytics.AnalyticsSource{Label: "Direct", V: 38, N: "18.4 K"}
type AnalyticsSource struct {
	// Label is the source name ("Direct", "Hacker News", etc.).
	Label string `json:"label"`
	// V is the percentage share (0–100).
	V int `json:"v"`
	// N is the pre-formatted visitor count ("18.4 K").
	N string `json:"n"`
}

// AnalyticsPage is the JSON wire type for one top-page row.
// Field names match the PageRow interface in
// frontend/src/app/desktop/surfaces/marketing/analytics.ts.
//
// Usage example:
//
//	pg := analytics.AnalyticsPage{P: "/sovereign-compute", V: "8.8 K", B: "21%", D: "4:42"}
type AnalyticsPage struct {
	// P is the page path ("/", "/sovereign-compute", etc.).
	P string `json:"p"`
	// V is the pre-formatted view count ("18.2 K").
	V string `json:"v"`
	// B is the bounce rate percentage ("32%").
	B string `json:"b"`
	// D is the average session duration ("2:14").
	D string `json:"d"`
}

// AnalyticsWindow is the reporting window selector.
//
// Usage example:
//
//	input := analytics.GetInput{Window: analytics.Window30d}
type AnalyticsWindow = string

const (
	// Window7d is the 7-day reporting window.
	Window7d AnalyticsWindow = "7d"
	// Window30d is the 30-day reporting window.
	Window30d AnalyticsWindow = "30d"
	// Window90d is the 90-day reporting window.
	Window90d AnalyticsWindow = "90d"
)

// GetInput drives the Get method.
//
// Usage example:
//
//	r := svc.Get(analytics.GetInput{Window: "30d"})
type GetInput struct {
	// Window is the reporting window: "7d"|"30d"|"90d". Defaults to "30d".
	Window AnalyticsWindow `json:"window,omitempty"`
}

// GetOutput is the Get response envelope.
//
// Usage example:
//
//	out := r.Value.(analytics.GetOutput)
//	for _, src := range out.Sources { _ = src.Label }
type GetOutput struct {
	// Window echoes the requested window.
	Window AnalyticsWindow `json:"window"`
	// Sessions is the pre-formatted total session count ("48,412", "—").
	Sessions string `json:"sessions"`
	// Delta is the period-over-period change label ("+18 % vs prior 30 days").
	Delta string `json:"delta"`
	// Spark is the comma-separated daily session series for the sparkline.
	Spark string `json:"spark"`
	// Sources is the ordered top-sources list.
	Sources []AnalyticsSource `json:"sources"`
	// Pages is the ordered top-pages list.
	Pages []AnalyticsPage `json:"pages"`
}
