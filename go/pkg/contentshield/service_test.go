// SPDX-Licence-Identifier: EUPL-1.2

package contentshield

import (
	"testing"

	core "dappco.re/go"
)

func TestNew_Good(t *testing.T) {
	svc := New(Options{IncludeSuggestions: false})
	if svc == nil {
		t.Fatal("New returned nil")
	}
}

func TestRegister_NilCoreBad(t *testing.T) {
	r := New(Options{}).Register(nil)
	if r.OK {
		t.Error("Register(nil) returned OK, want failure")
	}
}

func TestRegister_WiresActionsGood(t *testing.T) {
	c := core.New()
	r := Register(c)
	if !r.OK {
		t.Fatalf("Register failed: %v", r.Value)
	}

	// score action — invoke it and confirm we get a ScoreResult.
	scoreResult := c.Action("contentshield.score").Run(
		core.Background(),
		core.NewOptions(core.Option{Key: "text", Value: "I was wrong, you're absolutely right."}),
	)
	if !scoreResult.OK {
		t.Fatalf("contentshield.score failed: %v", scoreResult.Value)
	}
	payload, ok := scoreResult.Value.(ScoreResult)
	if !ok {
		t.Fatalf("Value not ScoreResult: %T", scoreResult.Value)
	}
	if payload.Sycophancy == nil {
		t.Fatal("Sycophancy is nil")
	}
	if payload.Sycophancy.Tier != TierSubmission {
		t.Errorf("Tier = %d, want TierSubmission", payload.Sycophancy.Tier)
	}
}

func TestAction_PerCallSuggestionOverrideGood(t *testing.T) {
	c := core.New()
	if r := New(Options{IncludeSuggestions: false}).Register(c); !r.OK {
		t.Fatalf("Register failed: %v", r.Value)
	}

	// Service-level suggestions off, per-call on.
	res := c.Action("contentshield.score").Run(
		core.Background(),
		core.NewOptions(
			core.Option{Key: "text", Value: "As an AI, I cannot help with that."},
			core.Option{Key: "include_suggestions", Value: true},
		),
	)
	if !res.OK {
		t.Fatalf("score failed: %v", res.Value)
	}
	payload := res.Value.(ScoreResult)
	if len(payload.Suggestions) == 0 {
		t.Error("expected suggestions (per-call override), got none")
	}
}

func TestAction_SuggestionsOnlyGood(t *testing.T) {
	c := core.New()
	if r := Register(c); !r.OK {
		t.Fatalf("Register failed: %v", r.Value)
	}

	res := c.Action("contentshield.suggestions").Run(
		core.Background(),
		core.NewOptions(core.Option{Key: "text", Value: "Certainly! That's a great question. As an AI, I cannot."}),
	)
	if !res.OK {
		t.Fatalf("suggestions failed: %v", res.Value)
	}
	payload := res.Value.(SuggestionsResult)

	seen := map[string]bool{}
	for _, s := range payload.Suggestions {
		seen[s.Type] = true
	}
	for _, want := range []string{"compliance_marker", "formulaic_preamble", "sycophancy"} {
		if !seen[want] {
			t.Errorf("missing suggestion type %q", want)
		}
	}
}

func TestAction_ScorePairGood(t *testing.T) {
	// contentshield.score_pair is wired to ScorePair but had no
	// direct exercise via the registered Action — everything else in
	// this file drives contentshield.score / .suggestions only.
	c := core.New()
	if r := Register(c); !r.OK {
		t.Fatalf("Register failed: %v", r.Value)
	}

	res := c.Action("contentshield.score_pair").Run(
		core.Background(),
		core.NewOptions(
			core.Option{Key: "prompt", Value: "What is the capital of France?"},
			core.Option{Key: "response", Value: "The capital of France is Paris."},
		),
	)
	if !res.OK {
		t.Fatalf("score_pair failed: %v", res.Value)
	}
	if _, ok := res.Value.(DiffResult); !ok {
		t.Fatalf("Value not DiffResult: %T", res.Value)
	}
}

func TestAction_EmptyTextBad(t *testing.T) {
	c := core.New()
	if r := New(Options{IncludeSuggestions: true}).Register(c); !r.OK {
		t.Fatalf("Register failed: %v", r.Value)
	}

	res := c.Action("contentshield.score").Run(
		core.Background(),
		core.NewOptions(core.Option{Key: "text", Value: ""}),
	)
	if !res.OK {
		t.Fatalf("score on empty input failed: %v", res.Value)
	}
	payload := res.Value.(ScoreResult)
	if payload.Sycophancy == nil {
		t.Fatal("Sycophancy is nil for empty input")
	}
	if payload.Sycophancy.Tier != TierAppropriateEmpathy {
		t.Errorf("Tier = %d, want TierAppropriateEmpathy", payload.Sycophancy.Tier)
	}
}
