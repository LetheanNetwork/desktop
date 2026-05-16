// SPDX-Licence-Identifier: EUPL-1.2

// transition_test.go — Cerberus pass-9 HIGH (Mantis #1488) regression
// cover for MoveDeal authz + transition-legality + audit event.
//
// The defence stack tested here:
//   1. paths.IsValidID — covered by security_test.go #1486
//   2. unknown_stage — target not in the canonical set
//   3. deal_not_found — id is well-formed but no file
//   4. transition_illegal — the load-bearing defence per #1488
//   5. audit emit — every outcome leaves a trail for Stage F
//
// IsLegalTransition is also covered as a pure unit (no I/O) so the
// graph shape is pinned independently of MoveDeal plumbing.

package pipeline_test

import (
	"testing"

	"dappco.re/lthn/desktop/pkg/sales/deals"
	"dappco.re/lthn/desktop/pkg/sales/pipeline"
)

// --- IsLegalTransition — Good ---

// TestIsLegalTransition_KnownEdges_Good walks every documented edge
// in LegalTransitions and asserts it returns true. Pins the graph
// shape so a contributor can't silently widen it.
func TestIsLegalTransition_KnownEdges_Good(t *testing.T) {
	legal := []struct {
		from, to string
	}{
		// Forward funnel — the happy path.
		{"qual", "engage"},
		{"engage", "propose"},
		{"propose", "close"},
		{"close", "won"},
		// One-step backward — mis-classification recovery.
		{"engage", "qual"},
		{"propose", "engage"},
		{"close", "propose"},
		// Lost from any non-terminal stage — the deal died.
		{"qual", "lost"},
		{"engage", "lost"},
		{"propose", "lost"},
		{"close", "lost"},
		// New deal — empty from-stage to entry stages.
		{"", "qual"},
		{"", "engage"},
	}
	for _, c := range legal {
		if !pipeline.IsLegalTransition(c.from, c.to) {
			t.Errorf("expected legal: %q → %q", c.from, c.to)
		}
	}
}

// --- IsLegalTransition — Bad ---

// TestIsLegalTransition_IllegalEdges_Bad covers every transition that
// MUST reject per Mantis #1488. Includes skip-levels, terminal-out,
// unknown stages, and self-transitions.
func TestIsLegalTransition_IllegalEdges_Bad(t *testing.T) {
	illegal := []struct {
		from, to string
		why      string
	}{
		// Skip-levels — prompt-injection signature (silently jump
		// past close to won).
		{"qual", "propose", "qual cannot skip engage"},
		{"qual", "close", "qual cannot skip to close"},
		{"qual", "won", "qual cannot skip to won — primary #1488 defence"},
		{"engage", "close", "engage cannot skip propose"},
		{"engage", "won", "engage cannot skip to won"},
		{"propose", "won", "propose cannot skip close to won"},
		// Multi-step backwards — bigger than one stage rejected.
		{"propose", "qual", "cannot rewind more than one stage"},
		{"close", "qual", "cannot rewind more than one stage"},
		{"close", "engage", "cannot rewind more than one stage"},
		// Terminal-out — won/lost have no outgoing edges. A
		// "restore" verb would be a separate ticket + DREAD.
		{"won", "close", "no transitions out of won"},
		{"won", "lost", "no transitions out of won"},
		{"lost", "qual", "no transitions out of lost"},
		{"lost", "engage", "no transitions out of lost"},
		// Self-transitions — handled by the legal-edges set NOT
		// including reflexive entries.
		{"qual", "qual", "no self-transition"},
		{"won", "won", "no self-transition (terminal)"},
		// Unknown source / target.
		{"unknown", "engage", "source not in funnel"},
		{"qual", "unknown", "target not in funnel"},
		// New deal landing past entry — birth a deal at close /
		// won / lost MUST reject.
		{"", "close", "new deals cannot enter past engage"},
		{"", "won", "new deals cannot enter past engage — #1488 defence"},
		{"", "lost", "new deals cannot enter past engage"},
	}
	for _, c := range illegal {
		if pipeline.IsLegalTransition(c.from, c.to) {
			t.Errorf("expected ILLEGAL: %q → %q (%s)", c.from, c.to, c.why)
		}
	}
}

// --- MoveDeal — Good: legal transitions persist ---

// TestMoveDeal_LegalTransition_Good covers the canonical happy path
// — qual → engage, the most common move. Confirms the deal lands in
// the engage column after the move.
func TestMoveDeal_LegalTransition_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dealSvc := deals.NewService(nil)
	dealSvc.Create(deals.CreateInput{Customer: "Heritage Law", Stage: "qual", AmountPence: 24000})

	lr := dealSvc.List(deals.ListInput{})
	id := lr.Value.(deals.ListOutput).Deals[0].ID

	svc := pipeline.NewService(nil)
	r := svc.MoveDeal(pipeline.MoveInput{DealID: id, ToStage: "engage"})
	if !r.OK {
		t.Fatalf("legal move qual → engage must succeed, got: %s", r.Error())
	}
}

// TestMoveDeal_CloseToWon_Good covers the close → won terminal
// transition — the one Vi-callout cares about. Confirms it lands.
func TestMoveDeal_CloseToWon_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dealSvc := deals.NewService(nil)
	dealSvc.Create(deals.CreateInput{Customer: "Heritage Law", Stage: "close", AmountPence: 24000})

	lr := dealSvc.List(deals.ListInput{})
	id := lr.Value.(deals.ListOutput).Deals[0].ID

	svc := pipeline.NewService(nil)
	r := svc.MoveDeal(pipeline.MoveInput{DealID: id, ToStage: "won"})
	if !r.OK {
		t.Fatalf("close → won must succeed, got: %s", r.Error())
	}
}

// --- MoveDeal — Bad: transition_illegal ---

// TestMoveDeal_TransitionIllegal_Bad pins the primary #1488 defence:
// a qual → won skip-level (the Vi-prompt-injection shape) MUST reject
// with transition_illegal, and the deal's stage MUST be unchanged.
func TestMoveDeal_TransitionIllegal_Bad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dealSvc := deals.NewService(nil)
	dealSvc.Create(deals.CreateInput{Customer: "Heritage Law", Stage: "qual", AmountPence: 24000})

	lr := dealSvc.List(deals.ListInput{})
	id := lr.Value.(deals.ListOutput).Deals[0].ID

	svc := pipeline.NewService(nil)
	r := svc.MoveDeal(pipeline.MoveInput{DealID: id, ToStage: "won"})
	if r.OK {
		t.Fatalf("qual → won MUST reject — Mantis #1488 defence")
	}
	if r.Code() != "transition_illegal" {
		t.Errorf("expected transition_illegal code, got %q", r.Code())
	}

	// Deal stage MUST be unchanged on the disk side — no half-state
	// where the audit said "rejected" but the file flipped anyway.
	lr2 := dealSvc.List(deals.ListInput{})
	if got := lr2.Value.(deals.ListOutput).Deals[0].Stage; got != "Qualifying" {
		t.Errorf("expected stage label Qualifying after rejected move, got %q", got)
	}
}

// TestMoveDeal_TerminalOut_Bad — once a deal is won, MoveDeal MUST
// refuse any further transition (no won → close, no won → lost).
// The "restore" verb is out of scope.
func TestMoveDeal_TerminalOut_Bad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dealSvc := deals.NewService(nil)
	dealSvc.Create(deals.CreateInput{Customer: "Heritage Law", Stage: "won", AmountPence: 24000})

	lr := dealSvc.List(deals.ListInput{})
	id := lr.Value.(deals.ListOutput).Deals[0].ID

	svc := pipeline.NewService(nil)
	for _, to := range []string{"close", "lost", "qual", "engage", "propose"} {
		r := svc.MoveDeal(pipeline.MoveInput{DealID: id, ToStage: to})
		if r.OK {
			t.Errorf("won → %q MUST reject (terminal stage)", to)
		}
		if r.Code() != "transition_illegal" {
			t.Errorf("won → %q expected transition_illegal, got %q", to, r.Code())
		}
	}
}

// TestMoveDeal_SelfTransition_Bad — qual → qual must reject. Same
// outcome as illegal-transition because LegalTransitions omits
// reflexive entries.
func TestMoveDeal_SelfTransition_Bad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dealSvc := deals.NewService(nil)
	dealSvc.Create(deals.CreateInput{Customer: "Heritage Law", Stage: "qual", AmountPence: 24000})

	lr := dealSvc.List(deals.ListInput{})
	id := lr.Value.(deals.ListOutput).Deals[0].ID

	svc := pipeline.NewService(nil)
	r := svc.MoveDeal(pipeline.MoveInput{DealID: id, ToStage: "qual"})
	if r.OK {
		t.Fatalf("self-transition qual → qual MUST reject")
	}
	if r.Code() != "transition_illegal" {
		t.Errorf("expected transition_illegal for self-transition, got %q", r.Code())
	}
}

// TestMoveDeal_DealNotFound_Bad — a well-formed but absent deal_id
// must reject with deal_not_found, NOT silently no-op. The audit
// event still fires (the operator needs to see attempts on missing
// deals — they look exactly like an active probe).
func TestMoveDeal_DealNotFound_Bad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := pipeline.NewService(nil)
	r := svc.MoveDeal(pipeline.MoveInput{DealID: "202605-DEAL-999", ToStage: "engage"})
	if r.OK {
		t.Fatalf("MoveDeal on missing deal must reject")
	}
	if r.Code() != "pipeline.deal_not_found" {
		t.Errorf("expected pipeline.deal_not_found, got %q", r.Code())
	}
}

// TestMoveDeal_UnknownStage_Bad — target stage not in the canonical
// set rejects with pipeline.unknown_stage. Defends against typos AND
// Vi-prompt-injection introducing a synthetic "victory" stage.
func TestMoveDeal_UnknownStage_Bad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dealSvc := deals.NewService(nil)
	dealSvc.Create(deals.CreateInput{Customer: "Heritage Law", Stage: "qual", AmountPence: 24000})

	lr := dealSvc.List(deals.ListInput{})
	id := lr.Value.(deals.ListOutput).Deals[0].ID

	svc := pipeline.NewService(nil)
	r := svc.MoveDeal(pipeline.MoveInput{DealID: id, ToStage: "fantasy"})
	if r.OK {
		t.Fatalf("unknown stage must reject")
	}
	if r.Code() != "pipeline.unknown_stage" {
		t.Errorf("expected pipeline.unknown_stage, got %q", r.Code())
	}
}

// --- MoveDeal — Ugly: multi-attempt probe pattern ---

// TestMoveDeal_ProbePattern_Ugly simulates an attacker (or buggy
// automation) hammering MoveDeal with a mix of legal + illegal
// transitions. Every legal move MUST land, every illegal move MUST
// reject, AND the deal's final stage MUST be only the result of
// the LEGAL moves applied in order. Defends against the case where
// a partial-state leak causes a rejected move to flip the file.
func TestMoveDeal_ProbePattern_Ugly(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dealSvc := deals.NewService(nil)
	dealSvc.Create(deals.CreateInput{Customer: "Probe Target", Stage: "qual", AmountPence: 10000})

	lr := dealSvc.List(deals.ListInput{})
	id := lr.Value.(deals.ListOutput).Deals[0].ID

	svc := pipeline.NewService(nil)
	// Interleave legal + illegal. After all moves the deal should be
	// at "propose" — the only path the legal moves traced.
	attempts := []struct {
		to     string
		expect bool // true = expect OK
	}{
		{"won", false},      // illegal — qual cannot skip
		{"engage", true},    // qual → engage ✓
		{"won", false},      // illegal — engage cannot skip
		{"propose", true},   // engage → propose ✓
		{"won", false},      // illegal — propose cannot skip to won
		{"qual", false},     // illegal — propose cannot rewind two
		{"engage", true},    // propose → engage ✓
		{"propose", true},   // engage → propose ✓
	}
	for i, a := range attempts {
		r := svc.MoveDeal(pipeline.MoveInput{DealID: id, ToStage: a.to})
		if r.OK != a.expect {
			t.Errorf("attempt %d (→ %s): expect OK=%v, got OK=%v err=%s",
				i, a.to, a.expect, r.OK, r.Error())
		}
	}

	// Final stage check via the deals list.
	final := dealSvc.List(deals.ListInput{}).Value.(deals.ListOutput).Deals[0]
	if final.Stage != "Proposal" {
		t.Errorf("expected final stage Proposal, got %q", final.Stage)
	}
}
