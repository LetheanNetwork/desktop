// SPDX-Licence-Identifier: EUPL-1.2

// Benchmarks for the streaming chat load path — the per-token
// multiplier: WChatStream's provider loop runs once PER TOKEN of every
// streamed reply, so a few bytes of allocation here is the whole GB
// story across a session. Companions: pkg/connection's
// BenchmarkDispatchWailsEvent_* (the transport hop the SAME token
// takes one step further out) and pkg/gateway's
// BenchmarkGatewayHandle_* (the per-request, not per-token, sibling
// load path).
//
// Run:
//
//	go test -run='^$' -bench=. -benchmem -benchtime=20x ./pkg/runner/
package runner

import (
	"context"
	"iter"

	core "dappco.re/go"
	"dappco.re/go/inference"
	"dappco.re/go/inference/agent/ai"

	"dappco.re/lthn/desktop/pkg/audit"
)

// benchNoopRecorder discards every audit event with zero allocation.
// streamRouterChat fires two audit.Default().Record calls per turn
// (requested + completed), and the production Default() is a real
// NDJSON file sink with an fsync per call (pkg/audit/recorder.go) —
// installing this for the bench keeps the numbers about the runner's
// own per-token cost, not disk latency, and keeps the benchmark from
// writing real files to ~/Lethean/audit/ on every run.
type benchNoopRecorder struct{}

func (benchNoopRecorder) Record(audit.Event) core.Result { return core.Ok(nil) }

// benchChatModel streams a fixed token slice with no per-call
// bookkeeping (contrast stream_internal_test.go's liveChatModel, which
// records calls/messages for assertions) — the bench should measure
// streamRouterChat's allocations, not the fake's.
type benchChatModel struct {
	tokens []string
}

func (m *benchChatModel) Generate(ctx context.Context, prompt string, _ ...inference.GenerateOption) iter.Seq[inference.Token] {
	return m.Chat(ctx, []inference.Message{{Role: "user", Content: prompt}})
}

func (m *benchChatModel) Chat(_ context.Context, _ []inference.Message, _ ...inference.GenerateOption) iter.Seq[inference.Token] {
	tokens := m.tokens
	return func(yield func(inference.Token) bool) {
		for _, tok := range tokens {
			if !yield(inference.Token{Text: tok}) {
				return
			}
		}
	}
}

func (m *benchChatModel) Classify(context.Context, []string, ...inference.GenerateOption) core.Result {
	return core.Ok([]inference.ClassifyResult(nil))
}

func (m *benchChatModel) BatchGenerate(context.Context, []string, ...inference.GenerateOption) core.Result {
	return core.Ok([]inference.BatchResult(nil))
}

func (m *benchChatModel) ModelType() string         { return "bench" }
func (m *benchChatModel) Info() inference.ModelInfo { return inference.ModelInfo{} }
func (m *benchChatModel) Metrics() inference.GenerateMetrics {
	return inference.GenerateMetrics{GeneratedTokens: len(m.tokens)}
}
func (m *benchChatModel) Err() core.Result   { return core.Ok(nil) }
func (m *benchChatModel) Close() core.Result { return core.Ok(nil) }

// benchTokens builds n short word-shaped tokens ("The ", "quick ", …) —
// representative of a real streamed reply where each SSE/event chunk
// is a handful of bytes, not a whole sentence.
func benchTokens(n int) []string {
	words := []string{
		"The ", "quick ", "brown ", "fox ", "jumps ", "over ",
		"the ", "lazy ", "dog", ". ", "It ", "runs ", "again ", "soon",
	}
	out := make([]string, n)
	for i := range out {
		out[i] = words[i%len(words)]
	}
	return out
}

// benchStreamService builds a Service with one live route streaming
// tokenCount tokens through a discard event emitter — hermetic (no
// wails core, no audit I/O once benchNoopRecorder is installed),
// isolating streamRouterChat's own per-token allocation cost.
func benchStreamService(tokenCount int) (*Service, []inference.Message) {
	model := &benchChatModel{tokens: benchTokens(tokenCount)}
	s := NewService(Options{Routes: []ai.ProviderRoute{
		{Name: "lem", ModelID: "bench-model", Model: model},
	}})
	s.eventEmitter = func(string, any) core.Result { return core.Ok(nil) }
	return s, []inference.Message{{Role: "user", Content: "tell me a story about a fox"}}
}

// BenchmarkWChatStream_TokenLoop_40Tokens measures the full per-token
// stream step (streamRouterChat's provider loop: channel range +
// builder.WriteString + the eventChatDelta map[string]any construction
// + emitChatEvent dispatch) for a representative 40-token streamed
// reply. This is the dominant per-token multiplier path named in the
// perf brief: calls-per-user-action == tokens-per-reply.
func BenchmarkWChatStream_TokenLoop_40Tokens(b *core.B) {
	audit.SetDefault(benchNoopRecorder{})
	b.Cleanup(func() { audit.SetDefault(nil) })
	s, messages := benchStreamService(40)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r := s.WChatStream("bench-call", messages, "")
		if !r.OK {
			b.Fatal(r.Error())
		}
	}
}

// BenchmarkEmitChatEvent_Delta isolates ONE per-token emit — the
// map[string]any{4 keys} construction plus the eventEmitter dispatch —
// from the surrounding channel-range/builder cost that
// BenchmarkWChatStream_TokenLoop_40Tokens measures together. Every
// token in a stream calls this exact code path once.
//
// The four map values are read through indirected vars (not string
// literals): a literal string constant boxed into `any` is a case the
// compiler can statically intern (zero extra allocation beyond the
// map itself), which is NOT what streamRouterChat's real call site
// does — there every value is a runtime variable (callID, token.Text,
// providerRoute.Name/ModelID). Using literals here undercounts by
// ~4 allocs/op; the indirection keeps this bench honest.
func BenchmarkEmitChatEvent_Delta(b *core.B) {
	s := &Service{eventEmitter: func(string, any) core.Result { return core.Ok(nil) }}
	callID, delta, provider, model := benchDeltaFields()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r := s.emitChatEvent(eventChatDelta, map[string]any{
			"call_id":  callID,
			"delta":    delta,
			"provider": provider,
			"model":    model,
		})
		if !r.OK {
			b.Fatal(r.Error())
		}
	}
}

// benchDeltaFields returns the four field values as runtime variables
// (never inlined back to constants at the call site) — see
// BenchmarkEmitChatEvent_Delta's comment for why this matters.
//
//go:noinline
func benchDeltaFields() (callID, delta, provider, model string) {
	return "bench-call", "quick ", "lem", "bench-model"
}

// BenchmarkRouterForSelector_Empty measures the fast path every
// WChatStream call takes when the caller doesn't narrow to a specific
// route (the common case: the frontend's default "send" leaves route
// ""). One call per chat TURN, not per token, but it still runs on
// every request.
func BenchmarkRouterForSelector_Empty(b *core.B) {
	s, _ := benchStreamService(1)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r := s.routerForSelector("")
		if !r.OK {
			b.Fatal(r.Error())
		}
	}
}

// BenchmarkRouterForSelector_NarrowedByName measures the route-
// narrowing path (matchingProviderRoutes + ai.NewProviderRouter) the
// frontend takes when the operator pins a specific model/provider —
// still once per turn, but does real allocation work the empty-
// selector path skips entirely.
func BenchmarkRouterForSelector_NarrowedByName(b *core.B) {
	s, _ := benchStreamService(1)
	s.router = ai.NewProviderRouter(
		ai.ProviderRoute{Name: "lem", ModelID: "local", Model: &benchChatModel{}},
		ai.ProviderRoute{
			Name: "opencode:anthropic/sonnet", ModelID: "anthropic/sonnet",
			Model:  &benchChatModel{},
			Labels: map[string]string{"kind": "opencode-routed", "provider_id": "anthropic"},
		},
	).Value.(*ai.ProviderRouter)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r := s.routerForSelector("opencode:anthropic/sonnet")
		if !r.OK {
			b.Fatal(r.Error())
		}
	}
}

// welfareBenchModel is a two-shape chat double for the welfare-enabled
// bench: a mediation-opener turn ({system, user}) gets a bare "proceed"
// reply (no tool call) so applyWelfare's Guard call round-trips once
// without triggering rephrase/pause, then the real turn streams
// normally — isolating welfareGuard's per-turn overhead (userTurns
// slice build + the mediation Chat call) on the opt-in path.
type welfareBenchModel struct {
	tokens []string
}

func (m *welfareBenchModel) Generate(ctx context.Context, prompt string, opts ...inference.GenerateOption) iter.Seq[inference.Token] {
	return m.Chat(ctx, []inference.Message{{Role: "user", Content: prompt}}, opts...)
}

func (m *welfareBenchModel) Chat(_ context.Context, messages []inference.Message, _ ...inference.GenerateOption) iter.Seq[inference.Token] {
	if len(messages) == 2 && messages[0].Role == "system" {
		return func(yield func(inference.Token) bool) {
			yield(inference.Token{Text: `{"tool":"lem_ok","params":{}}`})
		}
	}
	tokens := m.tokens
	return func(yield func(inference.Token) bool) {
		for _, tok := range tokens {
			if !yield(inference.Token{Text: tok}) {
				return
			}
		}
	}
}

func (m *welfareBenchModel) Classify(context.Context, []string, ...inference.GenerateOption) core.Result {
	return core.Ok([]inference.ClassifyResult(nil))
}
func (m *welfareBenchModel) BatchGenerate(context.Context, []string, ...inference.GenerateOption) core.Result {
	return core.Ok([]inference.BatchResult(nil))
}
func (m *welfareBenchModel) ModelType() string                  { return "welfare-bench" }
func (m *welfareBenchModel) Info() inference.ModelInfo          { return inference.ModelInfo{} }
func (m *welfareBenchModel) Metrics() inference.GenerateMetrics { return inference.GenerateMetrics{} }
func (m *welfareBenchModel) Err() core.Result                   { return core.Ok(nil) }
func (m *welfareBenchModel) Close() core.Result                 { return core.Ok(nil) }

// BenchmarkWChatStream_WelfareGuard_OptIn measures one full turn with
// RFC.welfare's opt-in gate live — the mediation Guard call
// (userTurns' priors-slice build + a synchronous round-trip Chat call)
// that runs before every provider dispatch when WelfareEnabled. Default
// production shape has welfare OFF (nil *welfare.Service short-
// circuits with zero allocation, already covered implicitly by
// BenchmarkWChatStream_TokenLoop_40Tokens above); this bench is the
// opt-in cost for GUI sessions that turn it on.
func BenchmarkWChatStream_WelfareGuard_OptIn(b *core.B) {
	audit.SetDefault(benchNoopRecorder{})
	b.Cleanup(func() { audit.SetDefault(nil) })
	model := &welfareBenchModel{tokens: benchTokens(10)}
	s := NewService(Options{
		Routes:         []ai.ProviderRoute{{Name: "lem", ModelID: "bench-model", Model: model}},
		WelfareEnabled: true,
	})
	s.eventEmitter = func(string, any) core.Result { return core.Ok(nil) }
	messages := []inference.Message{
		{Role: "user", Content: "hostile-shaped bench prompt one"},
		{Role: "assistant", Content: "a reply"},
		{Role: "user", Content: "hostile-shaped bench prompt two"},
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r := s.WChatStream("bench-call", messages, "")
		if !r.OK {
			b.Fatal(r.Error())
		}
	}
}
