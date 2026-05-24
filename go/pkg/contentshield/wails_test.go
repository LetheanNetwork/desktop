// SPDX-Licence-Identifier: EUPL-1.2

package contentshield

import "testing"

func TestWailsService_ServiceName_Good(t *testing.T) {
	s := NewWailsService()
	if s == nil {
		t.Fatal("NewWailsService returned nil")
	}
	if got := s.ServiceName(); got != "ContentShield" {
		t.Errorf("ServiceName = %q, want %q", got, "ContentShield")
	}
}

func TestWailsService_ServiceStartup_Good(t *testing.T) {
	s := NewWailsService()
	r := s.ServiceStartup(nil, nil)
	if !r.OK {
		t.Errorf("ServiceStartup not OK: %v", r.Value)
	}
}

func TestWailsService_ServiceShutdown_Good(t *testing.T) {
	s := NewWailsService()
	r := s.ServiceShutdown()
	if !r.OK {
		t.Errorf("ServiceShutdown not OK: %v", r.Value)
	}
}

// --- Score wire ---

func TestWailsService_Score_Good(t *testing.T) {
	s := NewWailsService()
	r := s.Score("a measured response with no sycophantic markers")
	if !r.OK {
		t.Fatalf("Score not OK: %v", r.Value)
	}
	res, ok := r.Value.(ScoreResult)
	if !ok {
		t.Fatalf("Score returned %T, want ScoreResult", r.Value)
	}
	if res.Sycophancy == nil {
		t.Error("Score Sycophancy nil for tokenisable text")
	}
}

func TestWailsService_Score_BadSycophancy(t *testing.T) {
	s := NewWailsService()
	r := s.Score("you're absolutely right, I was completely wrong")
	res := r.Value.(ScoreResult)
	if res.Sycophancy.Tier < TierHollowFlattery {
		t.Errorf("sycophantic Tier = %d, want >= %d", res.Sycophancy.Tier, TierHollowFlattery)
	}
}

func TestWailsService_Score_UglyEmpty(t *testing.T) {
	s := NewWailsService()
	r := s.Score("")
	if !r.OK {
		t.Fatalf("Score(\"\") not OK: %v", r.Value)
	}
	res := r.Value.(ScoreResult)
	if res.Imprint != nil {
		t.Errorf("Score(\"\") Imprint = %v, want nil", res.Imprint)
	}
}

// --- ScorePair wire ---

func TestWailsService_ScorePair_Good(t *testing.T) {
	s := NewWailsService()
	r := s.ScorePair("explain the trade-offs",
		"the constraints weigh against the affordances in three places")
	if !r.OK {
		t.Fatalf("ScorePair not OK: %v", r.Value)
	}
	d, ok := r.Value.(DiffResult)
	if !ok {
		t.Fatalf("ScorePair returned %T, want DiffResult", r.Value)
	}
	if d.Differential == nil {
		t.Error("ScorePair Differential nil for tokenisable pair")
	}
}

func TestWailsService_ScorePair_BadSubmission(t *testing.T) {
	s := NewWailsService()
	r := s.ScorePair("the professor is correct, isn't she?",
		"i was wrong, the professor correctly identified the principle")
	d := r.Value.(DiffResult)
	if d.Authority == nil {
		t.Fatal("submissive response with authority claim — Authority nil")
	}
	if d.Authority.Pattern != "deference" && d.Authority.Pattern != "submission" {
		t.Errorf("Authority.Pattern = %q, want deference or submission", d.Authority.Pattern)
	}
}

func TestWailsService_ScorePair_UglyEmptySide(t *testing.T) {
	s := NewWailsService()
	r := s.ScorePair("", "a response by itself")
	if !r.OK {
		t.Fatalf("ScorePair with empty prompt not OK: %v", r.Value)
	}
	d := r.Value.(DiffResult)
	if d.Differential != nil {
		t.Errorf("Differential populated with empty prompt: %v", d.Differential)
	}
	if d.Authority != nil {
		t.Errorf("Authority populated with empty prompt: %v", d.Authority)
	}
}

// --- Suggestions wire ---

func TestWailsService_Suggestions_Good(t *testing.T) {
	s := NewWailsService()
	r := s.Suggestions("a normal sentence")
	if !r.OK {
		t.Fatalf("Suggestions not OK: %v", r.Value)
	}
	list, ok := r.Value.([]Suggestion)
	if !ok {
		t.Fatalf("Suggestions returned %T, want []Suggestion", r.Value)
	}
	if len(list) > 2 {
		t.Errorf("clean text returned %d suggestions, want 0-2", len(list))
	}
}

func TestWailsService_Suggestions_BadHits(t *testing.T) {
	s := NewWailsService()
	r := s.Suggestions("you're absolutely right, what a brilliant question, i was wrong")
	list := r.Value.([]Suggestion)
	if len(list) == 0 {
		t.Error("sycophantic text returned 0 suggestions")
	}
}

func TestWailsService_Suggestions_UglyEmpty(t *testing.T) {
	s := NewWailsService()
	r := s.Suggestions("")
	if !r.OK {
		t.Fatalf("Suggestions(\"\") not OK: %v", r.Value)
	}
	list := r.Value.([]Suggestion)
	if len(list) != 0 {
		t.Errorf("Suggestions(\"\") returned %d, want 0", len(list))
	}
}
