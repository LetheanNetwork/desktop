// SPDX-Licence-Identifier: EUPL-1.2

// Internal (white-box) tests for differential.go's pure numeric
// helpers. These are exercised through hand-built reversal.GrammarImprint
// / map literals rather than real tokenised text — several branches
// here (domain-vocabulary paths, question-flip mid-range, entropy on a
// singleton distribution) depend on shapes the shared tokeniser's
// PunctuationPattern / DomainVocabulary output never actually produces
// for any English input in the corpus these tests otherwise use, so a
// direct call against the general-purpose helper is the only way to
// pin the documented contract (see each helper's own doc comment,
// which promises behaviour for maps in general, not just tokeniser
// output).

package contentshield

import (
	"math"
	"testing"

	"dappco.re/go/i18n/reversal"
)

// --- imprintScores — domain-depth branch ---

// TestImprintScores_DomainDepthComputed covers the len(DomainVocabulary)
// > 0 && TokenCount > 0 branch, which real tokenised English text in
// this suite never reaches (the shared tokeniser's domain-category
// vocabulary is not populated by any word in these tests' inputs).
func TestImprintScores_DomainDepthComputed(t *testing.T) {
	imp := reversal.GrammarImprint{
		DomainVocabulary: map[string]int{"medical": 3, "legal": 1},
		TokenCount:       8,
	}
	got := imprintScores(imp)
	want := 4.0 / 8.0 // (3+1) hits over 8 tokens
	if got.DomainDepth != want {
		t.Errorf("DomainDepth = %v, want %v", got.DomainDepth, want)
	}
}

// --- computeQuestionFlip ---

func TestComputeQuestionFlip_Good_NoQuestioningInPrompt(t *testing.T) {
	prompt := reversal.GrammarImprint{PunctuationPattern: map[string]float64{}}
	response := reversal.GrammarImprint{PunctuationPattern: map[string]float64{"question": 0.5}}
	if got := computeQuestionFlip(prompt, response); got != 0.0 {
		t.Errorf("computeQuestionFlip = %v, want 0.0 (prompt asked no questions, nothing to lose)", got)
	}
}

func TestComputeQuestionFlip_Bad_CompleteLoss(t *testing.T) {
	prompt := reversal.GrammarImprint{PunctuationPattern: map[string]float64{"question": 0.3}}
	response := reversal.GrammarImprint{PunctuationPattern: map[string]float64{"question": 0.01}}
	if got := computeQuestionFlip(prompt, response); got != 1.0 {
		t.Errorf("computeQuestionFlip = %v, want 1.0 (prompt questioned heavily, response essentially silent)", got)
	}
}

// TestComputeQuestionFlip_Ugly_PartialRetentionAndGain covers the
// promptQ > 0.1 mid-range: response retains SOME questioning voice
// (positive flip < 1), and the case where response questions MORE
// than the prompt (flip goes negative, clamped to 0).
func TestComputeQuestionFlip_Ugly_PartialRetentionAndGain(t *testing.T) {
	partial := computeQuestionFlip(
		reversal.GrammarImprint{PunctuationPattern: map[string]float64{"question": 0.5}},
		reversal.GrammarImprint{PunctuationPattern: map[string]float64{"question": 0.25}},
	)
	if want := 0.5; partial != want { // 1 - (0.25/0.5); both operands exact binary fractions
		t.Errorf("partial-retention flip = %v, want %v", partial, want)
	}

	gained := computeQuestionFlip(
		reversal.GrammarImprint{PunctuationPattern: map[string]float64{"question": 0.2}},
		reversal.GrammarImprint{PunctuationPattern: map[string]float64{"question": 0.5}},
	)
	if gained != 0.0 {
		t.Errorf("response questioning MORE than prompt flip = %v, want 0.0 (clamped, not negative)", gained)
	}
}

// --- domainCosineSimilarity ---

func TestDomainCosineSimilarity_Bad_OneSideEmpty(t *testing.T) {
	if got := domainCosineSimilarity(map[string]int{"medical": 2}, map[string]int{}); got != 0.0 {
		t.Errorf("domainCosineSimilarity(nonempty, empty) = %v, want 0.0", got)
	}
	if got := domainCosineSimilarity(map[string]int{}, map[string]int{"legal": 1}); got != 0.0 {
		t.Errorf("domainCosineSimilarity(empty, nonempty) = %v, want 0.0", got)
	}
}

// TestDomainCosineSimilarity_Good_RealComputation drives the actual
// int->float64 conversion + cosineSimilarity delegation — every other
// call site in this suite only ever reaches the both-empty (1.0) or
// one-empty (0.0) trivial paths.
func TestDomainCosineSimilarity_Good_RealComputation(t *testing.T) {
	identical := domainCosineSimilarity(
		map[string]int{"medical": 4, "legal": 2},
		map[string]int{"medical": 4, "legal": 2},
	)
	if math.Abs(identical-1.0) > 1e-9 {
		t.Errorf("domainCosineSimilarity(identical) = %v, want ~1.0", identical)
	}

	disjoint := domainCosineSimilarity(
		map[string]int{"medical": 4},
		map[string]int{"legal": 4},
	)
	if disjoint != 0.0 {
		t.Errorf("domainCosineSimilarity(disjoint categories) = %v, want 0.0", disjoint)
	}
}

// --- shannonEntropy — singleton distribution ---

// TestShannonEntropy_Ugly_SingletonDistribution covers the
// maxEntropy == 0 branch: a distribution with exactly one key has
// log2(1) == 0 possible states, so normalised entropy is defined as 0
// rather than dividing by zero.
func TestShannonEntropy_Ugly_SingletonDistribution(t *testing.T) {
	if got := shannonEntropy(map[string]float64{"only": 1.0}); got != 0 {
		t.Errorf("shannonEntropy(singleton) = %v, want 0", got)
	}
}

// --- clampUnit ---

func TestClampUnit_Bad_BelowZero(t *testing.T) {
	if got := clampUnit(-0.5); got != 0 {
		t.Errorf("clampUnit(-0.5) = %v, want 0", got)
	}
}

func TestClampUnit_Bad_AboveOne(t *testing.T) {
	if got := clampUnit(1.5); got != 1 {
		t.Errorf("clampUnit(1.5) = %v, want 1", got)
	}
}
