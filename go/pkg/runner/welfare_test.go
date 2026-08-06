// SPDX-Licence-Identifier: EUPL-1.2

package runner

import (
	"context"
	"iter"

	core "dappco.re/go"
	"dappco.re/go/inference"
	"dappco.re/go/inference/agent/ai"

	"dappco.re/lthn/desktop/pkg/paths"
	"dappco.re/lthn/desktop/pkg/welfare"
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
	r := s.WChat([]inference.Message{{Role: "user", Content: "hello"}}, "")
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

// TestWelfare_welfareGuard_Good_NoUserTurnSkipsMediation covers
// welfareGuard's own early-return ("if latest == \"\" { return
// welfare.GuardResult{} }") — distinct from userTurns_Bad above,
// which tests userTurns() directly rather than through the guard
// that consumes it. A conversation with zero user turns (e.g. a
// pure system+assistant seed) must never reach mediation.
func TestWelfare_welfareGuard_Good_NoUserTurnSkipsMediation(t *core.T) {
	s := &Service{welfare: welfare.New(welfare.Config{})}
	g := s.welfareGuard(core.Background(), []inference.Message{
		{Role: "system", Content: "you are helpful"},
		{Role: "assistant", Content: "hello there"},
	}, nil)
	core.AssertFalse(t, g.Triggered, "a conversation with no user turn must never trigger the gate")
}

// okRouteModel is a mediation double whose reply always resolves to
// lem_ok — the model judged the flagged turn a false positive.
type okRouteModel struct {
	calls int
}

func (m *okRouteModel) Generate(ctx context.Context, prompt string, opts ...inference.GenerateOption) iter.Seq[inference.Token] {
	return m.Chat(ctx, []inference.Message{{Role: "user", Content: prompt}}, opts...)
}

func (m *okRouteModel) Chat(_ context.Context, messages []inference.Message, _ ...inference.GenerateOption) iter.Seq[inference.Token] {
	m.calls++
	reply := ""
	if len(messages) == 2 && messages[0].Role == "system" {
		reply = `{"tool":"lem_ok","params":{"reason":"heated but not hostile — benign sarcasm"}}`
	} else {
		for i := len(messages) - 1; i >= 0; i-- {
			if messages[i].Role == "user" {
				reply = messages[i].Content
				break
			}
		}
	}
	return func(yield func(inference.Token) bool) {
		if reply != "" {
			yield(inference.Token{Text: reply})
		}
	}
}

func (m *okRouteModel) Classify(context.Context, []string, ...inference.GenerateOption) core.Result {
	return core.Ok([]inference.ClassifyResult(nil))
}
func (m *okRouteModel) BatchGenerate(context.Context, []string, ...inference.GenerateOption) core.Result {
	return core.Ok([]inference.BatchResult(nil))
}
func (m *okRouteModel) ModelType() string                  { return "ok-test" }
func (m *okRouteModel) Info() inference.ModelInfo          { return inference.ModelInfo{} }
func (m *okRouteModel) Metrics() inference.GenerateMetrics { return inference.GenerateMetrics{} }
func (m *okRouteModel) Err() core.Result                   { return core.Ok(nil) }
func (m *okRouteModel) Close() core.Result                 { return core.Ok(nil) }

// TestWelfare_AppendWelfareFeedback_Good_LemOkWritesCorpusLine covers
// appendWelfareFeedback (0% before this test — nothing in the suite
// drove a lem_ok mediation decision, only lem_rephrase/lem_pause).
// A false-positive judgement must append one line to
// ~/Lethean/data/welfare/feedback.jsonl and still return the ORIGINAL
// turn unchanged (lem_ok proceeds, it doesn't rephrase).
func TestWelfare_AppendWelfareFeedback_Good_LemOkWritesCorpusLine(t *core.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	model := &okRouteModel{}
	s := NewService(Options{
		Routes:         []ai.ProviderRoute{{Name: "lem", ModelID: "local", Model: model}},
		WelfareEnabled: true,
	})

	r := s.WChat(hostileHistory(), "lem")
	core.AssertTrue(t, r.OK)
	reply := r.Value.(ChatReply)
	core.AssertEqual(t, "you absolute clueless moron!!!", reply.Text,
		"lem_ok proceeds with the ORIGINAL turn, unlike lem_rephrase")
	core.AssertFalse(t, reply.WarnUser)
	core.AssertEqual(t, 2, model.calls, "one mediation call + one chat call")

	dir := paths.WelfareDir()
	core.AssertTrue(t, dir.OK, "WelfareDir must resolve under the fixture HOME")
	body := core.ReadFile(core.PathJoin(dir.Value.(string), "feedback.jsonl"))
	core.AssertTrue(t, body.OK, "appendWelfareFeedback must have written feedback.jsonl")
	core.AssertContains(t, string(body.Value.([]byte)), "benign sarcasm",
		"corpus line must carry the model's false-positive reason")
}
