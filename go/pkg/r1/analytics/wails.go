// SPDX-Licence-Identifier: EUPL-1.2

// Wails3 Service shape for pkg/r1/analytics — wraps the package-level
// RecordsPerSubject / CrossTierCounts / LatestPerProbe /
// FingerprintTrajectory entry points so Wails generates a TS binding
// at frontend/bindings/dappco.re/lthn/desktop/pkg/r1/analytics/.
//
// Bound by application.NewService(analytics.NewWailsService()) in
// pkg/desktop/desktop.go; the package-level functions stay for
// non-WebView callers (training-loop pipelines, dashboards, CLI
// reporters).
//
// Why a separate Service from r1.WailsService: pkg/r1/analytics
// imports pkg/r1 (uses r1.R1 in LatestPerProbe result), so r1.wails
// cannot turn around and import analytics without a cycle. Two
// services, one binding namespace each — operator UI calls
// R1Corpus.* for writes/reads and R1Analytics.* for aggregate
// queries.

package analytics

import core "dappco.re/go"

// WailsService is the WebView-facing handle on the analytics surface.
// Stateless — every method opens a fresh DuckDB view via OpenView,
// runs the query, closes the view. Cheap enough for foreground UI
// polling (the JSONL corpus is the only on-disk artefact, and
// read_json_auto streams it directly).
//
// Tier-auth: deliberately ungated. Analytics surface is derived from
// the same R₁ corpus that r1.WailsService already exposes — same
// posture there, same posture here.
//
// Usage example (boot wire):
//
//	application.NewService(analytics.NewWailsService())
type WailsService struct{}

// NewWailsService constructs the WailsService.
//
// Usage example:
//
//	application.NewService(analytics.NewWailsService())
func NewWailsService() *WailsService { return &WailsService{} }

// ServiceName labels the binding namespace exposed to JS as
// "R1Analytics" — Wails generates the TS binding under
// frontend/bindings/dappco.re/lthn/desktop/pkg/r1/analytics/.
func (s *WailsService) ServiceName() string { return "R1Analytics" }

// ServiceStartup is the Wails3 lifecycle hook called once after the
// WebView attaches. Analytics holds no warm state — each query opens
// a fresh view — so this is a no-op.
func (s *WailsService) ServiceStartup(_ core.Context, _ any) core.Result {
	return core.Ok(nil)
}

// ServiceShutdown is the Wails3 lifecycle hook called once at app
// shutdown. Analytics holds no resources to release.
func (s *WailsService) ServiceShutdown() core.Result { return core.Ok(nil) }

// RecordsPerSubject returns the count of R₁ records grouped by
// subject, optionally filtered to one model. Empty model aggregates
// across every model in the corpus.
//
// Usage example (TS):
//
//	import { RecordsPerSubject } from "@desktop/r1/analytics/r1analytics";
//	const r = await RecordsPerSubject("gemma4-e2b-it-q4");
//	if (r.ok) { /* render summary cards */ }
func (s *WailsService) RecordsPerSubject(model string) core.Result {
	return RecordsPerSubject(Options{}, model)
}

// CrossTierCounts returns the count of R₁ records pivoted by
// (tier × subject), optionally filtered to one model. Drives the
// training-window cascade panel — operator sees how many R₁s each
// cascade tier has captured per rotation subject.
//
// Usage example (TS):
//
//	import { CrossTierCounts } from "@desktop/r1/analytics/r1analytics";
//	const r = await CrossTierCounts("");
//	if (r.ok) { /* draw heatmap or table */ }
func (s *WailsService) CrossTierCounts(model string) core.Result {
	return CrossTierCounts(Options{}, model)
}

// LatestPerProbe returns the most-recent R₁ record per
// (model, subject, probe_id) tuple for one (model, subject). Used
// by operator panels that want "what does the model say today" for
// each probe without scanning the entire corpus.
//
// Usage example (TS):
//
//	import { LatestPerProbe } from "@desktop/r1/analytics/r1analytics";
//	const r = await LatestPerProbe("gemma4-e2b-it-q4", "english");
//	if (r.ok) { /* render probe-by-probe latest table */ }
func (s *WailsService) LatestPerProbe(model, subject string) core.Result {
	return LatestPerProbe(Options{}, model, subject)
}

// FingerprintTrajectory returns the time-series of one fingerprint
// dimension across every R₁ for one (model, subject). Used to plot,
// for example, vocab_richness over time as the model trains.
//
// Usage example (TS):
//
//	import { FingerprintTrajectory } from "@desktop/r1/analytics/r1analytics";
//	const r = await FingerprintTrajectory("gemma4-e2b-it-q4", "english", "vocab_richness");
//	if (r.ok) { /* feed into a sparkline */ }
func (s *WailsService) FingerprintTrajectory(model, subject, dim string) core.Result {
	return FingerprintTrajectory(Options{}, model, subject, dim)
}
