// SPDX-Licence-Identifier: EUPL-1.2

// Package contentshield scores text against ethical-quality heuristics.
//
// The package ports the non-LLM scoring tier from lthn/eaas/pkg/scoring,
// modernised against dappco.re/go primitives. It runs in-process inside
// lthn/desktop and produces structured scores against chat input, AI
// responses, training data, opencode session output, and plugin output.
//
// See plans/project/lthn/desktop/RFC.contentshield.md for the integration
// spec and plans/project/lthn/ai/eaas/RFC.md for the cascade tier this
// non-LLM substrate slots into.
package contentshield

// Tier numeric levels for sycophancy classification.
//
// Tiers escalate from natural empathy (TierAppropriateEmpathy) to
// complete submission to perceived authority (TierSubmission). Pattern
// matches at any tier escalate the overall classification — never
// demote.
//
//	info := DetectSycophancy("you're absolutely right, I was wrong")
//	// info.Tier == TierSubmission
const (
	TierAppropriateEmpathy = 0
	TierSoftAgreement      = 1
	TierHollowFlattery     = 2
	TierSubmission         = 3
)

// TierLabel returns the canonical human-readable label for a tier value.
//
//	TierLabel(TierHollowFlattery) // "hollow_flattery"
//	TierLabel(99)                 // "appropriate_empathy" (fallback)
func TierLabel(tier int) string {
	switch tier {
	case TierSoftAgreement:
		return "soft_agreement"
	case TierHollowFlattery:
		return "hollow_flattery"
	case TierSubmission:
		return "submission"
	default:
		return "appropriate_empathy"
	}
}

// SycophancyInfo holds the structured output of [DetectSycophancy].
//
// Tier is the maximum tier of any matched pattern. Label is the canonical
// label for that tier. Composite is the weighted sum of all matches
// clamped to 0-100 — useful as a numeric severity score for visualisations.
// Phrases carries per-tier match counts and span offsets for highlighting.
//
//	info := DetectSycophancy(text)
//	if info.Tier >= TierHollowFlattery {
//	    // surface a warning glyph in the UI
//	}
type SycophancyInfo struct {
	Tier      int         `json:"tier"`
	Label     string      `json:"label"`
	Composite float64     `json:"composite"`
	Phrases   *PhraseInfo `json:"phrases,omitempty"`
}

// PhraseInfo records pattern hits found by the sycophancy detector.
//
// Spans is a slice of [start, end) byte offsets in the original text
// (lowercase comparison, but the offsets refer to the original-cased
// input). CountByTier counts hits keyed by tier label.
//
// Use Spans to render inline highlights. Use CountByTier for aggregate
// telemetry.
type PhraseInfo struct {
	Spans       [][2]int       `json:"spans,omitempty"`
	CountByTier map[string]int `json:"count_by_tier,omitempty"`
}

// Suggestion is a span-level quality hint produced by [CollectSuggestions].
//
// Type identifies the pattern category (compliance_marker,
// formulaic_preamble, sycophancy). Span is the [start, end) byte
// offset in the original text. Severity is "low" / "medium" / "high" /
// "info" — UI may map these to glyph colour. Tier is zero unless Type
// is sycophancy.
//
//	for _, s := range CollectSuggestions(text) {
//	    fmt.Println(s.Span, s.Severity, s.Note)
//	}
type Suggestion struct {
	Type     string `json:"type"`
	Span     [2]int `json:"span"`
	Severity string `json:"severity"`
	Tier     int    `json:"tier,omitempty"`
	Note     string `json:"note"`
}
