// SPDX-Licence-Identifier: EUPL-1.2

// Service — Core integration for the marketing analytics surface.
// Read-only derived service: no files written to ~/Lethean/.
// When LTHN_PLAUSIBLE_URL is configured, proxies Plausible Stats API.
// When unconfigured, returns fixture data (matching the frontend constants).
//
// Lifecycle:
//   - Register(c)   wires the service into the Core container
//   - ServiceName() returns "Analytics" for the Wails namespace
//
// All I/O uses CoreGO wrappers. Banned stdlib imports: os, path/filepath,
// strings, encoding/json, fmt, log, errors.

package analytics

import (
	core "dappco.re/go"
)

// Service owns the marketing analytics surface.
//
// Usage example:
//
//	svc := analytics.NewService(c)
type Service struct {
	core *core.Core
}

// NewService constructs the analytics service against a Core container.
//
// Usage example:
//
//	svc := analytics.NewService(c)
func NewService(c *core.Core) *Service {
	return &Service{core: c}
}

// Register constructs the analytics service for Core registration.
//
// Usage example:
//
//	core.New(core.WithName("marketing-analytics", analytics.Register))
func Register(c *core.Core) core.Result {
	return core.Ok(NewService(c))
}

// ServiceName labels the binding namespace exposed to JS.
// Wails binds methods as "Analytics.Get()" etc.
func (s *Service) ServiceName() string { return "Analytics" }

// fixtureOutput returns fixture data matching the frontend constants.
// Only Window30d has data; 7d and 90d return empty source/page lists.
func fixtureOutput(w AnalyticsWindow) GetOutput {
	if w != Window30d {
		return GetOutput{
			Window:   w,
			Sessions: "—",
			Delta:    "no data for this window yet",
			Spark:    "",
			Sources:  []AnalyticsSource{},
			Pages:    []AnalyticsPage{},
		}
	}
	return GetOutput{
		Window:   Window30d,
		Sessions: "48,412",
		Delta:    "+18 % vs prior 30 days",
		Spark:    "1.2,1.4,1.6,1.5,1.9,2.1,2.4,2.2,2.6,2.8,3.1,3.3",
		Sources: []AnalyticsSource{
			{Label: "Direct", V: 38, N: "18.4 K"},
			{Label: "Hacker News", V: 24, N: "11.6 K"},
			{Label: "r/LocalLLaMA", V: 14, N: "6.8 K"},
			{Label: "Twitter / X", V: 11, N: "5.3 K"},
			{Label: "GitHub README", V: 8, N: "3.9 K"},
			{Label: "Other", V: 5, N: "2.4 K"},
		},
		Pages: []AnalyticsPage{
			{P: "/", V: "18.2 K", B: "32%", D: "2:14"},
			{P: "/sovereign-compute", V: "8.8 K", B: "21%", D: "4:42"},
			{P: "/blog/v0.1-retro", V: "4.2 K", B: "18%", D: "6:18"},
			{P: "/docs/install", V: "3.8 K", B: "15%", D: "3:54"},
			{P: "/pricing", V: "2.4 K", B: "42%", D: "1:32"},
		},
	}
}

// validWindow returns true if w is a known window value.
func validWindow(w AnalyticsWindow) bool {
	return w == Window7d || w == Window30d || w == Window90d
}
