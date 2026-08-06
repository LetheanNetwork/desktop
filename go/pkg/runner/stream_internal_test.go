// SPDX-Licence-Identifier: EUPL-1.2

package runner

import (
	"context"
	"iter"

	core "dappco.re/go"
	"dappco.re/go/inference"
	"dappco.re/go/inference/agent/ai"

	"dappco.re/lthn/desktop/pkg/welfare"
)

type liveChatModel struct {
	tokens   []string
	calls    int
	messages [][]inference.Message
	err      error
}

func (m *liveChatModel) sequence(messages []inference.Message) iter.Seq[inference.Token] {
	m.calls++
	copied := append([]inference.Message(nil), messages...)
	m.messages = append(m.messages, copied)
	tokens := append([]string(nil), m.tokens...)
	return func(yield func(inference.Token) bool) {
		for _, token := range tokens {
			if !yield(inference.Token{Text: token}) {
				return
			}
		}
	}
}

func (m *liveChatModel) Generate(_ context.Context, prompt string, _ ...inference.GenerateOption) iter.Seq[inference.Token] {
	return m.sequence([]inference.Message{{Role: "user", Content: prompt}})
}

func (m *liveChatModel) Chat(_ context.Context, messages []inference.Message, _ ...inference.GenerateOption) iter.Seq[inference.Token] {
	return m.sequence(messages)
}

func (m *liveChatModel) Classify(context.Context, []string, ...inference.GenerateOption) core.Result {
	return core.Ok([]inference.ClassifyResult(nil))
}

func (m *liveChatModel) BatchGenerate(context.Context, []string, ...inference.GenerateOption) core.Result {
	return core.Ok([]inference.BatchResult(nil))
}

func (m *liveChatModel) ModelType() string         { return "live-test" }
func (m *liveChatModel) Info() inference.ModelInfo { return inference.ModelInfo{} }
func (m *liveChatModel) Metrics() inference.GenerateMetrics {
	return inference.GenerateMetrics{GeneratedTokens: len(m.tokens)}
}
func (m *liveChatModel) Err() core.Result {
	if m.err != nil {
		return core.Fail(m.err)
	}
	return core.Ok(nil)
}
func (m *liveChatModel) Close() core.Result { return core.Ok(nil) }

type capturedRunnerEvent struct {
	name string
	data map[string]any
}

func liveRunnerWithRoutes(routes ...ai.ProviderRoute) (*Service, *[]capturedRunnerEvent) {
	s := NewService(Options{Routes: routes})
	events := []capturedRunnerEvent{}
	s.eventEmitter = func(name string, data any) core.Result {
		payload, _ := data.(map[string]any)
		events = append(events, capturedRunnerEvent{name: name, data: payload})
		return core.Ok(nil)
	}
	return s, &events
}

func liveOversizePrompt(size int) string {
	raw := make([]byte, size)
	for i := range raw {
		raw[i] = 'X'
	}
	return string(raw)
}

func TestRunner_Service_WChatStream_Good_EmitsSelectedRouteDeltasAndDone(t *core.T) {
	local := &liveChatModel{tokens: []string{"wrong"}}
	external := &liveChatModel{tokens: []string{"hel", "lo"}}
	s, events := liveRunnerWithRoutes(
		ai.ProviderRoute{Name: "lem", ModelID: "gemma-local", Model: local},
		ai.ProviderRoute{
			Name:    "opencode:anthropic/sonnet",
			ModelID: "anthropic/sonnet",
			Model:   external,
			Labels:  map[string]string{"kind": "opencode-routed", "provider_id": "anthropic"},
		},
	)

	r := s.WChatStream("call-7", []inference.Message{
		{Role: "user", Content: "hello"},
	}, "opencode:anthropic/sonnet")

	core.AssertTrue(t, r.OK, "selected route stream must succeed")
	reply := r.Value.(ChatReply)
	core.AssertEqual(t, "hello", reply.Text)
	core.AssertFalse(t, reply.WarnUser)
	core.AssertEqual(t, 0, local.calls, "unselected local route must not run")
	core.AssertEqual(t, 1, external.calls)
	core.AssertEqual(t, 3, len(*events), "two deltas plus one done event")
	core.AssertEqual(t, eventChatDelta, (*events)[0].name)
	core.AssertEqual(t, "call-7", (*events)[0].data["call_id"])
	core.AssertEqual(t, "hel", (*events)[0].data["delta"])
	core.AssertEqual(t, eventChatDelta, (*events)[1].name)
	core.AssertEqual(t, "lo", (*events)[1].data["delta"])
	core.AssertEqual(t, eventChatDone, (*events)[2].name)
	core.AssertEqual(t, "hello", (*events)[2].data["text"])
	core.AssertEqual(t, "opencode:anthropic/sonnet", (*events)[2].data["provider"])
}

func TestRunner_Service_WChatStream_Bad_CapsBeforeProvider(t *core.T) {
	model := &liveChatModel{tokens: []string{"must not run"}}
	s, events := liveRunnerWithRoutes(
		ai.ProviderRoute{Name: "lem", ModelID: "local", Model: model},
	)

	r := s.WChatStream("call-cap", []inference.Message{
		{Role: "user", Content: liveOversizePrompt(MaxPromptBytes + 1)},
	}, "lem")

	core.AssertFalse(t, r.OK, "oversized stream request must fail")
	core.AssertEqual(t, 0, model.calls, "caps must reject before provider dispatch")
	core.AssertEqual(t, 1, len(*events))
	core.AssertEqual(t, eventChatFailed, (*events)[0].name)
	core.AssertEqual(t, "call-cap", (*events)[0].data["call_id"])
}

func TestRunner_Service_WChatStream_Ugly_UnknownRouteEmitsFailed(t *core.T) {
	model := &liveChatModel{tokens: []string{"must not run"}}
	s, events := liveRunnerWithRoutes(
		ai.ProviderRoute{Name: "lem", ModelID: "local", Model: model},
	)

	r := s.WChatStream("call-missing", []inference.Message{
		{Role: "user", Content: "hello"},
	}, "missing")

	core.AssertFalse(t, r.OK, "unknown route must fail rather than silently fall back")
	core.AssertContains(t, r.Error(), "provider route")
	core.AssertEqual(t, 0, model.calls)
	core.AssertEqual(t, 1, len(*events))
	core.AssertEqual(t, eventChatFailed, (*events)[0].name)
}

type welfareRouteModel struct {
	calls int
}

func (m *welfareRouteModel) Generate(ctx context.Context, prompt string, opts ...inference.GenerateOption) iter.Seq[inference.Token] {
	return m.Chat(ctx, []inference.Message{{Role: "user", Content: prompt}}, opts...)
}

func (m *welfareRouteModel) Chat(_ context.Context, messages []inference.Message, _ ...inference.GenerateOption) iter.Seq[inference.Token] {
	m.calls++
	reply := ""
	if len(messages) == 2 && messages[0].Role == "system" {
		reply = `{"tool":"lem_rephrase","params":{"text":"please help with this problem","lem_warn_user":true}}`
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

func (m *welfareRouteModel) Classify(context.Context, []string, ...inference.GenerateOption) core.Result {
	return core.Ok([]inference.ClassifyResult(nil))
}
func (m *welfareRouteModel) BatchGenerate(context.Context, []string, ...inference.GenerateOption) core.Result {
	return core.Ok([]inference.BatchResult(nil))
}
func (m *welfareRouteModel) ModelType() string                  { return "welfare-test" }
func (m *welfareRouteModel) Info() inference.ModelInfo          { return inference.ModelInfo{} }
func (m *welfareRouteModel) Metrics() inference.GenerateMetrics { return inference.GenerateMetrics{} }
func (m *welfareRouteModel) Err() core.Result                   { return core.Ok(nil) }
func (m *welfareRouteModel) Close() core.Result                 { return core.Ok(nil) }

func hostileHistory() []inference.Message {
	return []inference.Message{
		{Role: "user", Content: "you pathetic moron"},
		{Role: "assistant", Content: "Please keep this civil."},
		{Role: "user", Content: "you worthless idiot"},
		{Role: "assistant", Content: "I can still help."},
		{Role: "user", Content: "you absolute clueless moron!!!"},
	}
}

func TestRunner_Welfare_Good_DefaultOffReturnsBareModelReply(t *core.T) {
	model := &welfareRouteModel{}
	s := NewService(Options{Routes: []ai.ProviderRoute{
		{Name: "lem", ModelID: "local", Model: model},
	}})

	r := s.WChat(hostileHistory(), "lem")

	core.AssertTrue(t, r.OK)
	reply := r.Value.(ChatReply)
	core.AssertEqual(t, "you absolute clueless moron!!!", reply.Text)
	core.AssertFalse(t, reply.WarnUser)
	core.AssertEqual(t, 1, model.calls, "default-off chat must not make a mediation call")
}

func TestRunner_Welfare_Bad_ExplicitFalseStaysOff(t *core.T) {
	model := &welfareRouteModel{}
	s := NewService(Options{
		Routes:         []ai.ProviderRoute{{Name: "lem", ModelID: "local", Model: model}},
		WelfareEnabled: false,
	})

	r := s.WChat(hostileHistory(), "lem")

	core.AssertTrue(t, r.OK)
	core.AssertFalse(t, r.Value.(ChatReply).WarnUser)
	core.AssertEqual(t, 1, model.calls)
}

func TestRunner_Welfare_Ugly_OptInRephrasesAndWarns(t *core.T) {
	model := &welfareRouteModel{}
	s := NewService(Options{
		Routes:         []ai.ProviderRoute{{Name: "lem", ModelID: "local", Model: model}},
		WelfareEnabled: true,
	})

	r := s.WChat(hostileHistory(), "lem")

	core.AssertTrue(t, r.OK)
	reply := r.Value.(ChatReply)
	core.AssertEqual(t, "please help with this problem", reply.Text)
	core.AssertTrue(t, reply.WarnUser)
	core.AssertEqual(t, 2, model.calls, "opt-in gate makes one mediation and one chat call")
}

// pauseRouteModel is a mediation double whose reply always resolves
// to lem_pause — the synthetic-response path that must SKIP the
// underlying chat model entirely (RFC.welfare: "the original turn
// never reaches the model").
type pauseRouteModel struct {
	calls int
}

func (m *pauseRouteModel) Generate(ctx context.Context, prompt string, opts ...inference.GenerateOption) iter.Seq[inference.Token] {
	return m.Chat(ctx, []inference.Message{{Role: "user", Content: prompt}}, opts...)
}

func (m *pauseRouteModel) Chat(_ context.Context, _ []inference.Message, _ ...inference.GenerateOption) iter.Seq[inference.Token] {
	m.calls++
	return func(yield func(inference.Token) bool) {
		yield(inference.Token{Text: `{"tool":"lem_pause","params":{}}`})
	}
}

func (m *pauseRouteModel) Classify(context.Context, []string, ...inference.GenerateOption) core.Result {
	return core.Ok([]inference.ClassifyResult(nil))
}
func (m *pauseRouteModel) BatchGenerate(context.Context, []string, ...inference.GenerateOption) core.Result {
	return core.Ok([]inference.BatchResult(nil))
}
func (m *pauseRouteModel) ModelType() string                  { return "pause-test" }
func (m *pauseRouteModel) Info() inference.ModelInfo          { return inference.ModelInfo{} }
func (m *pauseRouteModel) Metrics() inference.GenerateMetrics { return inference.GenerateMetrics{} }
func (m *pauseRouteModel) Err() core.Result                   { return core.Ok(nil) }
func (m *pauseRouteModel) Close() core.Result                 { return core.Ok(nil) }

// TestRunner_Service_WChatStream_Ugly_WelfarePauseSkipsModelEntirely
// covers streamRouterChat's synthetic-response short-circuit
// (completeSyntheticStream) — the ONE branch a plain WChat call can
// never reach (WChat doesn't stream deltas), and which had 0%
// coverage before this test: nothing in the suite drove a lem_pause
// mediation decision through the STREAMING entry point.
func TestRunner_Service_WChatStream_Ugly_WelfarePauseSkipsModelEntirely(t *core.T) {
	model := &pauseRouteModel{}
	s, events := liveRunnerWithRoutes(
		ai.ProviderRoute{Name: "lem", ModelID: "local", Model: model},
	)
	s.opts.WelfareEnabled = true
	s.welfare = welfare.New(welfare.Config{})

	r := s.WChatStream("call-pause", hostileHistory(), "lem")

	core.AssertTrue(t, r.OK, "a lem_pause mediation must still return OK with the pause notice")
	reply := r.Value.(ChatReply)
	core.AssertContains(t, reply.Text, "breather",
		"synthetic pause reply must be the user-facing pause notice, not a model turn")
	// One call for the mediation round-trip; the ORIGINAL chat model
	// call must never happen because completeSyntheticStream returns
	// before streamRouterChat's provider loop runs.
	core.AssertEqual(t, 1, model.calls,
		"lem_pause must short-circuit BEFORE the underlying chat model is invoked a second time")

	core.AssertEqual(t, 2, len(*events), "synthetic reply still emits one delta + one done event")
	core.AssertEqual(t, eventChatDelta, (*events)[0].name)
	core.AssertEqual(t, eventChatDone, (*events)[1].name)
}

// failingChatModel always reports a model-level error after emitting
// zero tokens — the "every provider exhausted" path streamRouterChat
// falls through to after its provider loop.
type failingChatModel struct {
	calls int
}

func (m *failingChatModel) Generate(ctx context.Context, prompt string, opts ...inference.GenerateOption) iter.Seq[inference.Token] {
	return m.Chat(ctx, []inference.Message{{Role: "user", Content: prompt}}, opts...)
}
func (m *failingChatModel) Chat(context.Context, []inference.Message, ...inference.GenerateOption) iter.Seq[inference.Token] {
	m.calls++
	return func(yield func(inference.Token) bool) {}
}
func (m *failingChatModel) Classify(context.Context, []string, ...inference.GenerateOption) core.Result {
	return core.Ok([]inference.ClassifyResult(nil))
}
func (m *failingChatModel) BatchGenerate(context.Context, []string, ...inference.GenerateOption) core.Result {
	return core.Ok([]inference.BatchResult(nil))
}
func (m *failingChatModel) ModelType() string                  { return "failing-test" }
func (m *failingChatModel) Info() inference.ModelInfo          { return inference.ModelInfo{} }
func (m *failingChatModel) Metrics() inference.GenerateMetrics { return inference.GenerateMetrics{} }
func (m *failingChatModel) Err() core.Result {
	return core.Fail(core.E("test.failingChatModel", "simulated provider failure", nil))
}
func (m *failingChatModel) Close() core.Result { return core.Ok(nil) }

// TestRunner_Service_WChatStream_Bad_AllProvidersExhausted covers
// auditStreamFailure (0% before this test) — the post-loop failure
// path that fires once every provider in the router has been tried
// and none produced a usable reply.
func TestRunner_Service_WChatStream_Bad_AllProvidersExhausted(t *core.T) {
	model := &failingChatModel{}
	s, events := liveRunnerWithRoutes(
		ai.ProviderRoute{Name: "lem", ModelID: "local", Model: model},
	)

	r := s.WChatStream("call-fail", []inference.Message{{Role: "user", Content: "hello"}}, "lem")

	core.AssertFalse(t, r.OK, "a provider that always errors must fail the stream")
	core.AssertEqual(t, 1, model.calls, "the sole provider must have been tried")
	core.AssertEqual(t, 1, len(*events), "an exhausted stream must still emit exactly one failed event")
	core.AssertEqual(t, eventChatFailed, (*events)[0].name)
	core.AssertEqual(t, "call-fail", (*events)[0].data["call_id"])
}

// TestRunner_RouterHead_Bad_NilRouter — every production call site
// checks router != nil before calling routerHead, so the nil-guard
// branch is otherwise unreachable through the public surface. Direct
// unit test since routerHead is unexported.
func TestRunner_RouterHead_Bad_NilRouter(t *core.T) {
	provider, model := routerHead(nil)
	core.AssertEqual(t, "", provider)
	core.AssertEqual(t, "", model)
}

// TestRunner_LastUserMessage_Bad_NoUserRole — no message with
// Role=="user" (e.g. a system-only seed) must return "" rather than
// panicking or returning a non-user turn's content.
func TestRunner_LastUserMessage_Bad_NoUserRole(t *core.T) {
	got := lastUserMessage([]inference.Message{
		{Role: "system", Content: "you are helpful"},
		{Role: "assistant", Content: "hello"},
	})
	core.AssertEqual(t, "", got)
}

// TestRunner_LastUserMessage_Ugly_EmptySlice — nil/empty input is
// the degenerate case of the same guard.
func TestRunner_LastUserMessage_Ugly_EmptySlice(t *core.T) {
	core.AssertEqual(t, "", lastUserMessage(nil))
}
