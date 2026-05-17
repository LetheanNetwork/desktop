// SPDX-Licence-Identifier: EUPL-1.2

// Tests for the Stage F egress-audit emit added by Mantis #1658 /
// Cerberus #45 Shape A per H#179. Drives a router-backed Service with
// a stub TextModel so the router.Chat path runs without network egress;
// asserts the Requested + Completed + Failed audit events land via
// the audit.Default() singleton with the correct Meta shape and
// secret-shape discipline (no prompt body, no API key).
//
// Mirror of pkg/account / pkg/serverkey recordingRecorder pattern —
// SetDefault(fake) at test start, t.Cleanup(SetDefault(nil)) at end
// so concurrent test packages do not see each other's recorders.

package runner_test

import (
	"context"
	"iter"
	"sync"

	core "dappco.re/go"
	"dappco.re/go/ai/ai"
	"dappco.re/go/inference"

	"dappco.re/lthn/desktop/pkg/audit"
	"dappco.re/lthn/desktop/pkg/runner"
)

// recordingRecorder captures every audit.Event the runner emits during
// a test. Mirror of the same-named fixture in pkg/account/service_test.go.
type recordingRecorder struct {
	mu     sync.Mutex
	events []audit.Event
}

func (r *recordingRecorder) Record(ev audit.Event) core.Result {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, ev)
	return core.Ok(nil)
}

func (r *recordingRecorder) snapshot() []audit.Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]audit.Event, len(r.events))
	copy(out, r.events)
	return out
}

// installRecorder wires a recordingRecorder as audit.Default and
// restores the prior default at test end.
//
// Usage example:
//
//	rec := installRecorder(t)
//	... drive service ...
//	events := rec.snapshot()
func installRecorder(t *core.T) *recordingRecorder {
	t.Helper()
	rec := &recordingRecorder{}
	audit.SetDefault(rec)
	t.Cleanup(func() { audit.SetDefault(nil) })
	return rec
}

// runnerStubModel implements inference.TextModel with a fixed token
// stream — enough surface area for ai.ProviderRouter.Chat to return a
// successful ProviderChatResponse without touching the network.
type runnerStubModel struct {
	output  string
	err     error
	metrics inference.GenerateMetrics
}

func (m *runnerStubModel) seq() iter.Seq[inference.Token] {
	out := m.output
	err := m.err
	return func(yield func(inference.Token) bool) {
		if err != nil || out == "" {
			return
		}
		yield(inference.Token{Text: out})
	}
}

func (m *runnerStubModel) Generate(ctx context.Context, prompt string, opts ...inference.GenerateOption) iter.Seq[inference.Token] {
	return m.seq()
}

func (m *runnerStubModel) Chat(ctx context.Context, messages []inference.Message, opts ...inference.GenerateOption) iter.Seq[inference.Token] {
	return m.seq()
}

func (m *runnerStubModel) Classify(context.Context, []string, ...inference.GenerateOption) ([]inference.ClassifyResult, error) {
	return nil, nil
}

func (m *runnerStubModel) BatchGenerate(context.Context, []string, ...inference.GenerateOption) ([]inference.BatchResult, error) {
	return nil, nil
}

func (m *runnerStubModel) ModelType() string         { return "stub" }
func (m *runnerStubModel) Info() inference.ModelInfo { return inference.ModelInfo{} }
func (m *runnerStubModel) Metrics() inference.GenerateMetrics {
	return m.metrics
}
func (m *runnerStubModel) Err() error   { return m.err }
func (m *runnerStubModel) Close() error { return nil }

// newStubRunner constructs a Service backed by a single ai.ProviderRoute
// with the given stub model. Exercises the router.Chat egress path the
// audit emits wrap.
func newStubRunner(model *runnerStubModel) *runner.Service {
	return runner.NewService(runner.Options{
		Routes: []ai.ProviderRoute{
			{
				Name:    "stub-provider",
				ModelID: "stub-model",
				Model:   model,
			},
		},
	})
}

func TestRunner_Generate_EmitsRequestedAndCompleted_Good(t *core.T) {
	rec := installRecorder(t)
	s := newStubRunner(&runnerStubModel{
		output:  "pong",
		metrics: inference.GenerateMetrics{GeneratedTokens: 1},
	})

	r := s.Generate("ping")
	core.AssertTrue(t, r.OK, "router-backed Generate should succeed with stub model")
	// Mantis #1669 (fixed): router.Chat returns ProviderChatResponse by
	// value, so the service-side type assertion is `(ai.ProviderChatResponse)`
	// not `(*ai.ProviderChatResponse)`. With the fix the assistant reply
	// propagates to the caller and the audit Completed emit stamps the
	// actually-selected provider/model + real token count.
	core.AssertEqual(t, "pong", r.Value.(string))

	events := rec.snapshot()
	core.AssertEqual(t, 2, len(events), "Generate should emit Requested + Completed")

	core.AssertEqual(t, audit.EventInferenceGenerateRequested, events[0].Event)
	core.AssertEqual(t, "inference", events[0].Scope)
	core.AssertEqual(t, audit.OutcomeOK, events[0].Outcome)
	core.AssertEqual(t, "stub-provider", events[0].Meta["provider"])
	core.AssertEqual(t, "stub-model", events[0].Meta["model"])
	core.AssertEqual(t, 1, events[0].Meta["msg_count"])

	core.AssertEqual(t, audit.EventInferenceGenerateCompleted, events[1].Event)
	core.AssertEqual(t, "inference", events[1].Scope)
	core.AssertEqual(t, audit.OutcomeOK, events[1].Outcome)
	core.AssertEqual(t, "stub-provider", events[1].Meta["provider"])
	core.AssertEqual(t, "stub-model", events[1].Meta["model"])
	core.AssertEqual(t, 1, events[1].Meta["tokens"], "Completed tokens reflects router-returned GeneratedTokens")
	_, hasLatency := events[1].Meta["latency_ms"]
	core.AssertTrue(t, hasLatency, "Completed Meta must carry latency_ms key")
}

func TestRunner_Chat_EmitsRequestedAndCompleted_Good(t *core.T) {
	rec := installRecorder(t)
	s := newStubRunner(&runnerStubModel{
		output:  "ack",
		metrics: inference.GenerateMetrics{GeneratedTokens: 2},
	})

	r := s.Chat([]inference.Message{
		{Role: "system", Content: "be brief"},
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: "hello"},
		{Role: "user", Content: "again"},
	})
	core.AssertTrue(t, r.OK, "router-backed Chat should succeed with stub model")
	// Mantis #1669 (fixed): see Generate test — reply propagates with
	// real assistant text and Completed Meta carries real token count.
	core.AssertEqual(t, "ack", r.Value.(string))

	events := rec.snapshot()
	core.AssertEqual(t, 2, len(events), "Chat should emit Requested + Completed")

	core.AssertEqual(t, audit.EventInferenceChatRequested, events[0].Event)
	core.AssertEqual(t, "inference", events[0].Scope)
	core.AssertEqual(t, audit.OutcomeOK, events[0].Outcome)
	core.AssertEqual(t, "stub-provider", events[0].Meta["provider"])
	core.AssertEqual(t, "stub-model", events[0].Meta["model"])
	core.AssertEqual(t, 4, events[0].Meta["msg_count"], "msg_count tracks full conversation length")

	core.AssertEqual(t, audit.EventInferenceChatCompleted, events[1].Event)
	core.AssertEqual(t, audit.OutcomeOK, events[1].Outcome)
	core.AssertEqual(t, "stub-provider", events[1].Meta["provider"])
	core.AssertEqual(t, "stub-model", events[1].Meta["model"])
	core.AssertEqual(t, 2, events[1].Meta["tokens"], "Completed tokens reflects router-returned GeneratedTokens")
	_, hasLatency := events[1].Meta["latency_ms"]
	core.AssertTrue(t, hasLatency, "Completed Meta must carry latency_ms key")
}

func TestRunner_GenerateFailure_EmitsFailed_Bad(t *core.T) {
	rec := installRecorder(t)
	// A model whose Err() returns non-nil makes the router's response
	// path mark the attempt as failed; with only one configured route
	// the router itself returns a non-OK Result.
	s := newStubRunner(&runnerStubModel{
		err: core.E("stub.fail", "deliberate stub failure", nil),
	})

	r := s.Generate("ping")
	core.AssertFalse(t, r.OK, "stub model error should propagate through router")

	events := rec.snapshot()
	core.AssertEqual(t, 2, len(events), "failed Generate should emit Requested + Failed")
	core.AssertEqual(t, audit.EventInferenceGenerateRequested, events[0].Event)
	core.AssertEqual(t, audit.EventInferenceGenerateFailed, events[1].Event)
	core.AssertEqual(t, audit.OutcomeFailed, events[1].Outcome)
	core.AssertEqual(t, "stub-provider", events[1].Meta["provider"])
	core.AssertEqual(t, "stub-model", events[1].Meta["model"])
	_, hasErrorCode := events[1].Meta["error_code"]
	core.AssertTrue(t, hasErrorCode, "Failed Meta must carry error_code key")
}

// TestRunner_AuditMeta_NoPromptOrKey_Good asserts the Cerberus #1465
// closure-only-scope discipline at the inference egress: neither the
// prompt body NOR any provider-credential bytes ever appear in the
// audit Meta payload. The redactor in pkg/audit also enforces this at
// the sink layer, but the emit-site MUST NOT supply them in the first
// place — defence in depth.
func TestRunner_AuditMeta_NoPromptOrKey_Good(t *core.T) {
	rec := installRecorder(t)
	const sensitivePrompt = "supersecretpromptdonotpersist"
	s := newStubRunner(&runnerStubModel{
		output:  "ok",
		metrics: inference.GenerateMetrics{GeneratedTokens: 1},
	})

	r := s.Generate(sensitivePrompt)
	core.AssertTrue(t, r.OK)

	events := rec.snapshot()
	core.AssertEqual(t, 2, len(events))

	for _, ev := range events {
		// Top-level fields must not carry the prompt body.
		core.AssertFalse(t, core.Contains(ev.Event, sensitivePrompt))
		core.AssertFalse(t, core.Contains(ev.AccountID, sensitivePrompt))
		core.AssertFalse(t, core.Contains(ev.Scope, sensitivePrompt))
		// Meta values must not carry the prompt body — walk every
		// string value.
		for k, v := range ev.Meta {
			if s, ok := v.(string); ok {
				core.AssertFalse(t, core.Contains(s, sensitivePrompt),
					"Meta["+k+"] must not echo the prompt body")
			}
		}
		// Meta MUST NOT contain a "prompt" or "api_key" or "messages"
		// key — those are reserved as smuggle-surface canaries; the
		// emit-site is the only place to enforce this so an audit
		// reader can trust the schema.
		_, hasPrompt := ev.Meta["prompt"]
		core.AssertFalse(t, hasPrompt, "Meta must never carry a `prompt` key")
		_, hasAPIKey := ev.Meta["api_key"]
		core.AssertFalse(t, hasAPIKey, "Meta must never carry an `api_key` key")
		_, hasMessages := ev.Meta["messages"]
		core.AssertFalse(t, hasMessages, "Meta must never carry a `messages` key")
	}
}
