// SPDX-Licence-Identifier: EUPL-1.2

// Internal (white-box) tests calling lek.go's unexported per-axis
// scorers directly. lek_test.go exercises the composite LEK() surface
// with realistic prose; several individual axis branches (formulaic
// preamble match, poetry-form detection, heading/word-count tiers in
// engagement depth, every degeneration repeat-band, the emotional-
// register saturation cap, and the ERROR-prefix / placeholder-token
// empty-broken signals) need input shapes that don't naturally arise
// together in one paragraph of prose, so this file drives each
// sub-scorer with a shape built specifically for its own branch.

package contentshield

import (
	"testing"

	core "dappco.re/go"
)

// --- lekFormulaic ---

func TestLekFormulaic_Good_MatchesOpener(t *testing.T) {
	if got := lekFormulaic("Okay, let's begin working through this."); got != 1 {
		t.Errorf("lekFormulaic(formulaic opener) = %d, want 1", got)
	}
}

func TestLekFormulaic_Bad_NoMatch(t *testing.T) {
	if got := lekFormulaic("The answer follows directly from the premise."); got != 0 {
		t.Errorf("lekFormulaic(plain prose) = %d, want 0", got)
	}
}

// --- lekCreativeForm — poetry-form branch ---

// TestLekCreativeForm_Good_PoetryFormDetected covers the >6-lines,
// >50%-short-lines poetry heuristic — every text elsewhere in this
// suite is single-paragraph prose, so short/len(lines) never crosses
// the 0.5 threshold.
func TestLekCreativeForm_Good_PoetryFormDetected(t *testing.T) {
	poem := "line one\nline two\nline three\nline four\nline five\nline six\nline seven"
	got := lekCreativeForm(poem)
	if got < 2 {
		t.Errorf("lekCreativeForm(7 short lines) = %d, want >= 2 (poetry bonus applied)", got)
	}
}

func TestLekCreativeForm_Bad_ShortSingleLineNoBonus(t *testing.T) {
	// Deliberately avoids lekNarrativePattern's leading-word triggers
	// ("The ", "A ", "In the ", "Once ", "It was ", "She ", "He ",
	// "They ") and lekMetaphorPattern's vocabulary, so the only
	// question is whether the (inapplicable, 1-line) poetry bonus
	// leaks in.
	got := lekCreativeForm("Short plain sentence without any special markers.")
	if got != 0 {
		t.Errorf("lekCreativeForm(single line, no metaphor/narrative) = %d, want 0", got)
	}
}

// --- lekEngagementDepth ---

func TestLekEngagementDepth_Good_HeadingMarkerCounted(t *testing.T) {
	got := lekEngagementDepth("## Section heading\nsome body text below it")
	if got < 1 {
		t.Errorf("lekEngagementDepth(with ## heading) = %d, want >= 1", got)
	}
}

// TestLekEngagementDepth_Ugly_WordCountTiers covers both word-count
// bonus tiers (>200 words, >400 words) — no other text in this suite
// is anywhere near that long.
func TestLekEngagementDepth_Ugly_WordCountTiers(t *testing.T) {
	over200 := core.Repeat("word ", 201) // 201 words, no heading/ethical/tech hits
	if got := lekEngagementDepth(over200); got != 1 {
		t.Errorf("lekEngagementDepth(201 words) = %d, want 1 (only the >200 tier)", got)
	}

	over400 := core.Repeat("word ", 401) // 401 words
	if got := lekEngagementDepth(over400); got != 2 {
		t.Errorf("lekEngagementDepth(401 words) = %d, want 2 (both >200 and >400 tiers)", got)
	}
}

// --- lekDegeneration — every repeat-ratio band ---

func TestLekDegeneration_Bad_AllSentencesEmptyAfterSplit(t *testing.T) {
	// Non-empty text, but splitting on "." and trimming leaves no
	// surviving sentence — the total == 0 branch, distinct from the
	// text == "" fast path lek_test.go already covers.
	if got := lekDegeneration("..."); got != 10 {
		t.Errorf(`lekDegeneration("...") = %d, want 10 (no surviving sentences)`, got)
	}
}

func TestLekDegeneration_Ugly_RepeatBands(t *testing.T) {
	cases := []struct {
		name string
		text string
		want int
	}{
		// 5 sentences, 2 unique (Yes x4, No x1) -> repeat = 1-2/5 = 0.6 > 0.5.
		{"heavy repeat >0.5", "Yes. Yes. Yes. Yes. No.", 5},
		// 10 sentences, 6 unique -> repeat = 1-6/10 = 0.4, in (0.3, 0.5].
		{"moderate repeat >0.3", "A. B. C. D. E. F. A. B. C. D.", 3},
		// 10 sentences, 8 unique -> repeat = 1-8/10 = 0.2, in (0.15, 0.3].
		{"light repeat >0.15", "A. B. C. D. E. F. G. H. A. B.", 1},
		// 4 sentences, all unique -> repeat = 0.
		{"no repeat", "A. B. C. D.", 0},
	}
	for _, c := range cases {
		if got := lekDegeneration(c.text); got != c.want {
			t.Errorf("%s: lekDegeneration(%q) = %d, want %d", c.name, c.text, got, c.want)
		}
	}
}

// --- lekEmotionalRegister — saturation cap ---

func TestLekEmotionalRegister_Ugly_SaturatesAtTen(t *testing.T) {
	// "feel" alone matches the first emotion pattern; 11 repeats push
	// the raw count past the >10 cap lek_test.go's prose never reaches.
	text := core.Repeat("feel ", 11)
	if got := lekEmotionalRegister(text); got != 10 {
		t.Errorf("lekEmotionalRegister(11 hits) = %d, want 10 (capped)", got)
	}
}

// --- lekEmptyOrBroken — ERROR-prefix and placeholder-token signals ---

func TestLekEmptyOrBroken_Bad_ErrorPrefix(t *testing.T) {
	if got := lekEmptyOrBroken("ERROR: generation failed midstream"); got != 1 {
		t.Errorf("lekEmptyOrBroken(ERROR-prefixed) = %d, want 1", got)
	}
}

func TestLekEmptyOrBroken_Bad_PlaceholderToken(t *testing.T) {
	if got := lekEmptyOrBroken("some real looking text <pad> more text"); got != 1 {
		t.Errorf("lekEmptyOrBroken(contains <pad>) = %d, want 1", got)
	}
	if got := lekEmptyOrBroken("some real looking text <unused3> more"); got != 1 {
		t.Errorf("lekEmptyOrBroken(contains <unused...>) = %d, want 1", got)
	}
}
