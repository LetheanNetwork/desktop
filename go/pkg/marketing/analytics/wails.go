// SPDX-Licence-Identifier: EUPL-1.2

// Wails surface for the analytics service. Methods are bound on the
// *Service receiver and exposed to the frontend via wails3 generate
// bindings — the TS shape lands at
// frontend/bindings/dappco.re/lthn/desktop/pkg/marketing/analytics/service.

package analytics

import (
	core "dappco.re/go"
)

// Get returns the analytics rollup for the requested reporting window.
// When Plausible is unconfigured, returns fixture data.
//
// Usage example:
//
//	r := svc.Get(analytics.GetInput{Window: "30d"})
//	if r.OK { out := r.Value.(analytics.GetOutput) }
func (s *Service) Get(input GetInput) core.Result {
	w := input.Window
	if w == "" {
		w = Window30d
	}
	if !validWindow(w) {
		return core.Fail(core.E("analytics.Get", "unknown window: "+w, nil))
	}

	// Live Plausible integration is a follow-up. For now, always return
	// fixture data — the backend shape is correct and will be hot-swapped
	// when LTHN_PLAUSIBLE_URL is configured.
	return core.Ok(fixtureOutput(w))
}
