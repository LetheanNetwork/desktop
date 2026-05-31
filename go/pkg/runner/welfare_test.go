// SPDX-Licence-Identifier: EUPL-1.2

package runner

import (
	core "dappco.re/go"
	"dappco.re/go/inference"
)

func TestWelfare_userTurns_Good(t *core.T) {
	// Latest user message + prior user turns, system/assistant turns ignored.
	msgs := []inference.Message{
		{Role: "system", Content: "you are helpful"},
		{Role: "user", Content: "first question"},
		{Role: "assistant", Content: "first answer"},
		{Role: "user", Content: "second question"},
	}
	latest, priors := userTurns(msgs)
	core.AssertEqual(t, "second question", latest)
	core.AssertEqual(t, 1, len(priors))
	core.AssertEqual(t, "first question", priors[0])
}

func TestWelfare_userTurns_Bad(t *core.T) {
	// No user turns → empty latest (the gate then no-ops on the turn).
	msgs := []inference.Message{
		{Role: "system", Content: "you are helpful"},
		{Role: "assistant", Content: "hello"},
	}
	latest, priors := userTurns(msgs)
	core.AssertEqual(t, "", latest)
	core.AssertEqual(t, 0, len(priors))
}

func TestWelfare_userTurns_Ugly(t *core.T) {
	// Empty conversation → no latest, no priors, no panic.
	latest, priors := userTurns(nil)
	core.AssertEqual(t, "", latest)
	core.AssertEqual(t, 0, len(priors))
}

func TestWelfare_withLastUser_Good(t *core.T) {
	// Replaces only the LAST user turn; earlier turns + the original slice
	// are untouched.
	orig := []inference.Message{
		{Role: "user", Content: "keep me"},
		{Role: "assistant", Content: "ok"},
		{Role: "user", Content: "replace me"},
	}
	out := withLastUser(orig, "reworded")
	core.AssertEqual(t, "reworded", out[2].Content)
	core.AssertEqual(t, "keep me", out[0].Content)
	core.AssertEqual(t, "replace me", orig[2].Content) // original not mutated
}

func TestWelfare_withLastUser_Bad(t *core.T) {
	// No user turn → nothing to replace; returns an equivalent copy.
	orig := []inference.Message{{Role: "assistant", Content: "hi"}}
	out := withLastUser(orig, "reworded")
	core.AssertEqual(t, 1, len(out))
	core.AssertEqual(t, "hi", out[0].Content)
}

func TestWelfare_withLastUser_Ugly(t *core.T) {
	// Empty input → empty output, no panic.
	out := withLastUser(nil, "reworded")
	core.AssertEqual(t, 0, len(out))
}

// TestWelfare_WChat_Good_StructuredReplyWelfareClean — Mantis #1799: WChat
// now returns the structured ChatReply shape (text + warn_user) instead of a
// bare string. On the no-router stub path the welfare gate never runs, so the
// reply MUST carry WarnUser=false (no "reworded" chip on an un-gated turn).
func TestWelfare_WChat_Good_StructuredReplyWelfareClean(t *core.T) {
	s := NewService(Options{}) // no routes → router==nil → echo stub
	r := s.WChat([]inference.Message{{Role: "user", Content: "hello"}})
	core.AssertTrue(t, r.OK, "WChat stub path must succeed")
	reply, ok := r.Value.(ChatReply)
	core.AssertTrue(t, ok, "WChat must return a structured ChatReply")
	core.AssertFalse(t, reply.WarnUser, "un-gated stub turn must not flag WarnUser")
	core.AssertContains(t, reply.Text, "hello")
}

// TestWelfare_chatCtxWelfare_Bad_NoWarnOnStub — the chatCtxWelfare seam (the
// drop-site this ticket reopened) returns warnUser=false when no welfare gate
// is attached, so HTTP / CLI callers that go through bare ChatCtx are
// unaffected by the new second return value.
func TestWelfare_chatCtxWelfare_Bad_NoWarnOnStub(t *core.T) {
	s := NewService(Options{})
	r, warn := s.chatCtxWelfare(core.Background(), []inference.Message{
		{Role: "user", Content: "ping"},
	})
	core.AssertTrue(t, r.OK, "stub chatCtxWelfare must succeed")
	core.AssertFalse(t, warn, "stub path must never flag WarnUser")
}

// TestWelfare_chatCtxWelfare_Ugly_EmptyMessages — no user turn at all: the
// stub returns its echo with no warn flag and no panic.
func TestWelfare_chatCtxWelfare_Ugly_EmptyMessages(t *core.T) {
	s := NewService(Options{})
	r, warn := s.chatCtxWelfare(core.Background(), nil)
	core.AssertTrue(t, r.OK, "empty-message stub must succeed")
	core.AssertFalse(t, warn, "empty-message path must never flag WarnUser")
}
