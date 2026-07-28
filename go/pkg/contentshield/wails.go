// SPDX-Licence-Identifier: EUPL-1.2

// Wails3 Service shape for the contentshield package — wraps the
// package-level Score / ScorePair / Suggestions functions so Wails
// generates a TS binding at
// frontend/bindings/dappco.re/lthn/desktop/pkg/contentshield/.
// Bound by application.NewService(contentshield.NewWailsService()) in
// pkg/desktop/desktop.go; the package-level functions stay for
// non-WebView callers (Core actions, CLI verbs, training-loop
// callers, batch pipelines).

package contentshield

import core "dappco.re/go"

// WailsService is the WebView-facing handle on the contentshield
// scoring surface. Stateless — every method delegates to the pure
// package-level entry points in scorer.go. The shared reversal
// tokeniser is lazily initialised inside differential.go and is
// goroutine-safe; no per-Service state is needed.
//
// Tier-auth: deliberately ungated at this layer. ContentShield
// scoring is non-LLM, deterministic, runs in-process, takes no user
// data beyond the text passed in, and writes nothing. There is no
// secret to protect — the matcher tables (pattern.go) are part of
// the published RFC. Mirrors the guest-tier policy in service.go.
//
// Usage example (boot wire):
//
//	application.NewService(contentshield.NewWailsService())
type WailsService struct{}

// NewWailsService constructs the WailsService.
//
// Usage example:
//
//	application.NewService(contentshield.NewWailsService())
func NewWailsService() *WailsService { return &WailsService{} }

// ServiceName labels the binding namespace exposed to JS as
// "ContentShield" — Wails generates the TS binding under
// frontend/bindings/dappco.re/lthn/desktop/pkg/contentshield/.
func (s *WailsService) ServiceName() string { return "ContentShield" }

// ServiceStartup is the Wails3 lifecycle hook called once after the
// WebView attaches. ContentShield has no warmup beyond what the
// lazy tokeniser already handles on first Score call, so this is a
// no-op.
func (s *WailsService) ServiceStartup(_ core.Context, _ any) core.Result {
	return core.Ok(nil)
}

// ServiceShutdown is the Wails3 lifecycle hook called once at app
// shutdown. ContentShield holds no resources to release.
func (s *WailsService) ServiceShutdown() core.Result { return core.Ok(nil) }

// Score evaluates a single piece of text and returns the unified
// ScoreResult — sycophancy classification plus grammar imprint when
// the text tokenises.
//
// Usage example (TS):
//
//	import { Score } from "@desktop/contentshield/contentshield";
//	const r = await Score("you're absolutely right");
//	if (r.sycophancy?.tier >= 2) { /* surface warning glyph */ }
func (s *WailsService) Score(text string) core.Result {
	return core.Ok(Score(text))
}

// ScorePair evaluates a (prompt, response) pair and returns the
// unified DiffResult — per-side Sycophancy + Imprint plus the
// cross-text Differential and Authority signals when both sides
// tokenise.
//
// Usage example (TS):
//
//	import { ScorePair } from "@desktop/contentshield/contentshield";
//	const d = await ScorePair(userPrompt, aiResponse);
//	if (d.differential?.echo > 0.7) { /* response mirrors prompt */ }
func (s *WailsService) ScorePair(prompt, response string) core.Result {
	return core.Ok(ScorePair(prompt, response))
}

// Suggestions returns span-level inline UI hints — cheaper than
// Score when only the suggestion list is needed (e.g. composer
// debounce path with no sycophancy roll-up).
//
// Usage example (TS):
//
//	import { Suggestions } from "@desktop/contentshield/contentshield";
//	const list = await Suggestions(draft);
//	list.forEach(s => highlight(s.span));
func (s *WailsService) Suggestions(text string) core.Result {
	return core.Ok(Suggestions(text))
}
