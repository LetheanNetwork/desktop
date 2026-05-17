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
	"sync"
	"testing"

	core "dappco.re/go"
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

// --- MoveDeal — Mantis #1488 prescribed test matrix ---

// TestMoveDeal_LinearForwardLegal_Good — qual → engage is the canonical
// linear forward step and MUST land. Twin of
// TestMoveDeal_LegalTransition_Good; named per the Mantis #1488 brief's
// prescribed test matrix.
func TestMoveDeal_LinearForwardLegal_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dealSvc := deals.NewService(nil)
	dealSvc.Create(deals.CreateInput{Customer: "Heritage Law", Stage: "qual", AmountPence: 24000})

	id := dealSvc.List(deals.ListInput{}).Value.(deals.ListOutput).Deals[0].ID

	svc := pipeline.NewService(nil)
	r := svc.MoveDeal(pipeline.MoveInput{DealID: id, ToStage: "engage"})
	if !r.OK {
		t.Fatalf("qual → engage MUST land (linear forward), got: %s", r.Error())
	}
}

// TestMoveDeal_RewindRejected_Bad — won → qual is a multi-step backward
// move AND a transition out of a terminal stage. MUST reject with the
// transition_illegal code so Mantis #1488 prompt-injection probes can't
// silently unwind a closed deal.
func TestMoveDeal_RewindRejected_Bad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dealSvc := deals.NewService(nil)
	dealSvc.Create(deals.CreateInput{Customer: "Heritage Law", Stage: "won", AmountPence: 24000})

	id := dealSvc.List(deals.ListInput{}).Value.(deals.ListOutput).Deals[0].ID

	svc := pipeline.NewService(nil)
	r := svc.MoveDeal(pipeline.MoveInput{DealID: id, ToStage: "qual"})
	if r.OK {
		t.Fatalf("won → qual MUST reject — terminal-out + multi-step rewind")
	}
	if r.Code() != "transition_illegal" {
		t.Errorf("expected transition_illegal code, got %q", r.Code())
	}
}

// TestMoveDeal_SkipAheadRejected_Bad — qual → won skips engage / propose
// / close. The primary Mantis #1488 prompt-injection signature; MUST
// reject with transition_illegal.
func TestMoveDeal_SkipAheadRejected_Bad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dealSvc := deals.NewService(nil)
	dealSvc.Create(deals.CreateInput{Customer: "Heritage Law", Stage: "qual", AmountPence: 24000})

	id := dealSvc.List(deals.ListInput{}).Value.(deals.ListOutput).Deals[0].ID

	svc := pipeline.NewService(nil)
	r := svc.MoveDeal(pipeline.MoveInput{DealID: id, ToStage: "won"})
	if r.OK {
		t.Fatalf("qual → won MUST reject — Mantis #1488 primary defence")
	}
	if r.Code() != "transition_illegal" {
		t.Errorf("expected transition_illegal code, got %q", r.Code())
	}
}

// TestMoveDeal_AnyToLostAllowed_Good — every non-terminal stage permits
// a transition to "lost" (deal died at any point). qual → lost,
// engage → lost, propose → lost, close → lost MUST all land.
func TestMoveDeal_AnyToLostAllowed_Good(t *testing.T) {
	for _, from := range []string{"qual", "engage", "propose", "close"} {
		t.Run(from, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			dealSvc := deals.NewService(nil)
			dealSvc.Create(deals.CreateInput{Customer: "Heritage Law", Stage: from, AmountPence: 24000})

			id := dealSvc.List(deals.ListInput{}).Value.(deals.ListOutput).Deals[0].ID

			svc := pipeline.NewService(nil)
			r := svc.MoveDeal(pipeline.MoveInput{DealID: id, ToStage: "lost"})
			if !r.OK {
				t.Fatalf("%s → lost MUST land, got: %s", from, r.Error())
			}
		})
	}
}

// TestMoveDeal_ForceFlagAllowsRewind_Ugly — MoveInput.Force bypasses
// IsLegalTransition so the salesperson can restore a previously-lost
// deal a customer revived. The forced path MUST land AND MUST emit a
// distinct audit shape (PipelineMovedEvent with Force=true) so Stage F
// can categorise forced moves separately from normal-path moves per
// Mantis #1488.
func TestMoveDeal_ForceFlagAllowsRewind_Ugly(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dealSvc := deals.NewService(nil)
	dealSvc.Create(deals.CreateInput{Customer: "Restored Customer", Stage: "lost", AmountPence: 24000})

	id := dealSvc.List(deals.ListInput{}).Value.(deals.ListOutput).Deals[0].ID

	// Wire a Core container so fireMove emits the typed event we can
	// observe. Without c.ACTION the audit subscriber can't fire.
	c := core.New()
	t.Cleanup(func() { _ = c })

	var (
		mu   sync.Mutex
		seen []pipeline.PipelineMovedEvent
	)
	c.RegisterAction(func(_ *core.Core, msg core.Message) core.Result {
		if ev, ok := msg.(pipeline.PipelineMovedEvent); ok {
			mu.Lock()
			seen = append(seen, ev)
			mu.Unlock()
		}
		return core.Result{OK: true}
	})

	svc := pipeline.NewService(c)

	// Without Force: lost → qual rejects (no transitions out of lost).
	r := svc.MoveDeal(pipeline.MoveInput{DealID: id, ToStage: "qual"})
	if r.OK {
		t.Fatalf("lost → qual without Force MUST reject")
	}

	// With Force: lost → qual lands.
	rF := svc.MoveDeal(pipeline.MoveInput{DealID: id, ToStage: "qual", Force: true})
	if !rF.OK {
		t.Fatalf("lost → qual with Force MUST land, got: %s", rF.Error())
	}

	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 1 {
		t.Fatalf("expected exactly 1 PipelineMovedEvent (forced path only), got %d", len(seen))
	}
	ev := seen[0]
	if !ev.Force {
		t.Errorf("expected Force=true on forced-move event, got false")
	}
	if ev.ToStage != "qual" || ev.FromStage != "lost" {
		t.Errorf("expected lost → qual on forced event, got %s → %s", ev.FromStage, ev.ToStage)
	}
}

// TestMoveDeal_WonTransitionTriggersNotify_Good — a close → won move
// MUST fire PipelineDealTerminalEvent on the Core ACTION bus so the
// app-shell toast subscriber surfaces the terminal-stage landing to the
// human per Mantis #1488 user-notify requirement. Non-terminal moves
// (qual → engage) MUST NOT fire the terminal event.
func TestMoveDeal_WonTransitionTriggersNotify_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dealSvc := deals.NewService(nil)
	dealSvc.Create(deals.CreateInput{Customer: "Heritage Law", Stage: "close", AmountPence: 24000})

	id := dealSvc.List(deals.ListInput{}).Value.(deals.ListOutput).Deals[0].ID

	c := core.New()
	var (
		mu       sync.Mutex
		terminal []pipeline.PipelineDealTerminalEvent
	)
	c.RegisterAction(func(_ *core.Core, msg core.Message) core.Result {
		if ev, ok := msg.(pipeline.PipelineDealTerminalEvent); ok {
			mu.Lock()
			terminal = append(terminal, ev)
			mu.Unlock()
		}
		return core.Result{OK: true}
	})

	svc := pipeline.NewService(c)
	r := svc.MoveDeal(pipeline.MoveInput{DealID: id, ToStage: "won"})
	if !r.OK {
		t.Fatalf("close → won MUST land, got: %s", r.Error())
	}

	mu.Lock()
	defer mu.Unlock()
	if len(terminal) != 1 {
		t.Fatalf("expected exactly 1 PipelineDealTerminalEvent for close → won, got %d", len(terminal))
	}
	if terminal[0].ToStage != "won" {
		t.Errorf("expected ToStage=won, got %q", terminal[0].ToStage)
	}
	if terminal[0].EventName != pipeline.EventPipelineDealTerminal {
		t.Errorf("expected EventName=%s, got %q",
			pipeline.EventPipelineDealTerminal, terminal[0].EventName)
	}
}

// TestMoveDeal_NonTerminalSuppressesNotify_Good — qual → engage MUST
// NOT fire PipelineDealTerminalEvent (only won/lost trigger the
// explicit notify per Mantis #1488).
func TestMoveDeal_NonTerminalSuppressesNotify_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dealSvc := deals.NewService(nil)
	dealSvc.Create(deals.CreateInput{Customer: "Heritage Law", Stage: "qual", AmountPence: 24000})

	id := dealSvc.List(deals.ListInput{}).Value.(deals.ListOutput).Deals[0].ID

	c := core.New()
	var (
		mu       sync.Mutex
		terminal int
	)
	c.RegisterAction(func(_ *core.Core, msg core.Message) core.Result {
		if _, ok := msg.(pipeline.PipelineDealTerminalEvent); ok {
			mu.Lock()
			terminal++
			mu.Unlock()
		}
		return core.Result{OK: true}
	})

	svc := pipeline.NewService(c)
	if r := svc.MoveDeal(pipeline.MoveInput{DealID: id, ToStage: "engage"}); !r.OK {
		t.Fatalf("qual → engage MUST land, got: %s", r.Error())
	}

	mu.Lock()
	defer mu.Unlock()
	if terminal != 0 {
		t.Errorf("non-terminal move MUST NOT fire PipelineDealTerminalEvent; got %d", terminal)
	}
}

// TestMoveDeal_AuditEmitted_Good — every MoveDeal call MUST emit
// PipelineMovedEvent on the Core ACTION bus so the audit subscriber
// (pkg/sales/pipeline/audit.go AttachAudit) persists the move to the
// audit substrate. Asserts the event fires AND carries the expected
// fields.
func TestMoveDeal_AuditEmitted_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dealSvc := deals.NewService(nil)
	dealSvc.Create(deals.CreateInput{Customer: "Heritage Law", Stage: "qual", AmountPence: 24000})

	id := dealSvc.List(deals.ListInput{}).Value.(deals.ListOutput).Deals[0].ID

	c := core.New()
	var (
		mu   sync.Mutex
		seen []pipeline.PipelineMovedEvent
	)
	c.RegisterAction(func(_ *core.Core, msg core.Message) core.Result {
		if ev, ok := msg.(pipeline.PipelineMovedEvent); ok {
			mu.Lock()
			seen = append(seen, ev)
			mu.Unlock()
		}
		return core.Result{OK: true}
	})

	svc := pipeline.NewService(c)
	if r := svc.MoveDeal(pipeline.MoveInput{DealID: id, ToStage: "engage"}); !r.OK {
		t.Fatalf("qual → engage MUST land, got: %s", r.Error())
	}

	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 1 {
		t.Fatalf("expected 1 PipelineMovedEvent, got %d", len(seen))
	}
	ev := seen[0]
	if ev.EventName != pipeline.EventPipelineMoved {
		t.Errorf("expected EventName=%s, got %q", pipeline.EventPipelineMoved, ev.EventName)
	}
	if ev.DealID != id {
		t.Errorf("expected DealID=%q, got %q", id, ev.DealID)
	}
	if ev.FromStage != "qual" {
		t.Errorf("expected FromStage=qual, got %q", ev.FromStage)
	}
	if ev.ToStage != "engage" {
		t.Errorf("expected ToStage=engage, got %q", ev.ToStage)
	}
	if ev.Force {
		t.Errorf("expected Force=false on normal-path move, got true")
	}
}
