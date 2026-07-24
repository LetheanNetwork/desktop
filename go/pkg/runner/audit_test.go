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
	"dappco.re/go/inference"
	"dappco.re/go/inference/agent/ai"

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

func (m *runnerStubModel) Classify(context.Context, []string, ...inference.GenerateOption) core.Result {
	return core.Ok([]inference.ClassifyResult(nil))
}

func (m *runnerStubModel) BatchGenerate(context.Context, []string, ...inference.GenerateOption) core.Result {
	return core.Ok([]inference.BatchResult(nil))
}

func (m *runnerStubModel) ModelType() string         { return "stub" }
func (m *runnerStubModel) Info() inference.ModelInfo { return inference.ModelInfo{} }
func (m *runnerStubModel) Metrics() inference.GenerateMetrics {
	return m.metrics
}
func (m *runnerStubModel) Err() core.Result {
	if m.err != nil {
		return core.Fail(m.err)
	}
	return core.Ok(nil)
}
func (m *runnerStubModel) Close() core.Result { return core.Ok(nil) }

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

// TestRunner_AuditMeta_ErrorCodeBoundedKeyspace_Ugly — Cerberus #60 F-1 /
// #65 ADD-2 / Mantis #1710 + #1729: STRIDE-I prose-leak canary per
// RFC.error-code-cascade.md §3 (W5). Provider error bodies (formatted
// upstream as `provider returned HTTP %d: %s` where %s is the raw
// response body) routinely echo USER INPUT (OpenAI moderation refusals,
// vLLM tokenisation errors, custom proxies that log the prompt). Putting
// resp.Error() into Meta["error_code"] would leak that prompt fragment
// into the operator forensic log (STRIDE-I / A1 surveillance shape).
//
// Drives BOTH egress sites — service.go:209 (Generate, /v1/completions)
// AND service.go:304 (Chat, /v1/chat/completions) — against a stub model
// whose Err() mirrors the openai.go providerError() shape, then asserts
// the emitted Meta["error_code"] is the BOUNDED *core.Err .Operation
// literal (audit.ErrorCode resolution) NOT the raw HTTP/body prose.
// Defends the audit.ErrorCode(r) substrate at the helper boundary so
// future regressions that revert to a raw-string error_code fall loud
// rather than silently re-leaking caller-controlled bytes.
//
// Subtest shape mirrors the marketplace / downloader / gateway / process /
// sandbox audit clusters (Cerberus #41 cluster verify) so the
// `*ErrorCodeBoundedKeyspace*` grep returns the runner alongside its
// siblings.
func TestRunner_AuditMeta_ErrorCodeBoundedKeyspace_Ugly(t *core.T) {
	// Closed bounded keyspace per audit.ErrorCode(r) resolution order.
	// Today the failure path through ai.ProviderRouter wraps with
	// Operation="ai.ProviderRouter.Chat"; a future router rewrite that
	// preserves the inner *core.Err would surface "ai.openai.provider";
	// the cluster-wide fallback per RFC W1 + FOLD-1 is "unknown_error".
	bounded := map[string]bool{
		"ai.ProviderRouter.Chat": true,
		"ai.openai.provider":     true,
		"unknown_error":          true,
	}

	cases := []struct {
		name      string
		userInput string
		httpCode  int
		drive     func(s *runner.Service) core.Result
		wantEvent string
	}{
		{
			name:      "Generate",
			userInput: "USER_TYPED_FORBIDDEN_PROMPT_FRAGMENT",
			httpCode:  400,
			drive:     func(s *runner.Service) core.Result { return s.Generate("ping") },
			wantEvent: audit.EventInferenceGenerateFailed,
		},
		{
			name:      "Chat",
			userInput: "CHAT_USER_TYPED_FORBIDDEN_FRAGMENT",
			httpCode:  429,
			drive: func(s *runner.Service) core.Result {
				return s.Chat([]inference.Message{{Role: "user", Content: "hi"}})
			},
			wantEvent: audit.EventInferenceChatFailed,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *core.T) {
			rec := installRecorder(t)
			// Mirror the exact shape of pkg/ai/go/providers/openai/openai.go
			// providerError(): core.E with prose Message embedding the body.
			providerErr := core.E("ai.openai.provider",
				core.Sprintf("provider returned HTTP %d: %s", tc.httpCode, tc.userInput), nil)
			s := newStubRunner(&runnerStubModel{err: providerErr})

			r := tc.drive(s)
			core.AssertFalse(t, r.OK, "stub provider error should propagate through router")

			events := rec.snapshot()
			core.AssertEqual(t, 2, len(events), "failed "+tc.name+" should emit Requested + Failed")
			failed := events[1]
			core.AssertEqual(t, tc.wantEvent, failed.Event)

			code, ok := failed.Meta["error_code"].(string)
			core.AssertTrue(t, ok, "error_code must be a string")
			core.AssertTrue(t, bounded[code],
				"error_code must be the bounded *core.Err.Operation literal "+
					"(audit.ErrorCode), got "+code)
			core.AssertFalse(t, core.Contains(code, tc.userInput),
				"Meta[error_code] must NOT contain raw upstream body prose")
			core.AssertFalse(t, core.Contains(code, core.Sprintf("HTTP %d", tc.httpCode)),
				"Meta[error_code] must NOT contain raw upstream HTTP-status prose")
		})
	}
}

// TestRunner_GenerateFailed_AuditHasStableCode_Good — verifies the
// positive shape: the emitted error_code IS one of the stable Lethean
// codes from audit.ErrorCode()'s mapping. Today the failure path
// through ai.ProviderRouter wraps with Operation="ai.ProviderRouter.Chat"
// and no Code field, so audit.ErrorCode() falls back to the Operation
// string — itself a structured-not-prose identifier from the flat
// keyspace agents grep on. The point: the value is bounded and
// predictable, not caller-controlled prose. RFC W7 + FOLD-1: substrate
// canon "unknown_error" replaces the runner-local "provider_error"
// fallback (was deleted from pkg/runner/service.go in the W7 cutover).
func TestRunner_GenerateFailed_AuditHasStableCode_Good(t *core.T) {
	rec := installRecorder(t)
	providerErr := core.E("ai.openai.provider",
		"provider returned HTTP 500: opaque upstream prose", nil)
	s := newStubRunner(&runnerStubModel{err: providerErr})

	r := s.Generate("ping")
	core.AssertFalse(t, r.OK)

	events := rec.snapshot()
	core.AssertEqual(t, 2, len(events))
	code, ok := events[1].Meta["error_code"].(string)
	core.AssertTrue(t, ok, "error_code must be a string")

	// Acceptable shapes per audit.ErrorCode():
	//   - "ai.ProviderRouter.Chat" (outer router Operation — current
	//     wrap path when the upstream *core.Err has no Code field)
	//   - "ai.openai.provider"     (inner provider Operation, if a
	//     future router rewrite preserves the inner *Err)
	//   - "unknown_error"          (cluster-wide canonical fallback
	//     per RFC W1 + FOLD-1; substrate replaced runner-local
	//     "provider_error" in W7).
	stable := code == "ai.ProviderRouter.Chat" ||
		code == "ai.openai.provider" ||
		code == "unknown_error"
	core.AssertTrue(t, stable,
		"error_code must be one of the stable Lethean codes, got: "+code)
}

// TestRunner_GenerateCtx_ClientDisconnect_Bad — Cerberus #60 F-2 /
// Mantis #1711: when a client disconnects mid-call (browser tab close,
// fetch abort, gin c.Request.Context() cancelled), GenerateCtx must
// propagate the cancellation into the upstream router.Chat so the
// in-flight provider RTT terminates instead of running to completion
// and burning quota. ai.ProviderRouter.Chat checks ctx.Err() before
// each route attempt and returns "request cancelled" when set.
//
// The Requested audit row MUST still commit (forensic correlation —
// abandoned requests are real events worth logging); the Failed row
// then stamps with the cancelled-ctx error_code from the router.
func TestRunner_GenerateCtx_ClientDisconnect_Bad(t *core.T) {
	rec := installRecorder(t)
	s := newStubRunner(&runnerStubModel{output: "would-be-reply"})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel — simulates client disconnect BEFORE inference completes

	r := s.GenerateCtx(ctx, "ping")
	core.AssertFalse(t, r.OK,
		"cancelled ctx should propagate into router.Chat → Failed result, not OK")

	events := rec.snapshot()
	core.AssertEqual(t, 2, len(events),
		"cancelled GenerateCtx should still emit Requested + Failed (forensic correlation)")
	core.AssertEqual(t, audit.EventInferenceGenerateRequested, events[0].Event,
		"Requested row commits BEFORE the network call — survives cancellation")
	core.AssertEqual(t, audit.EventInferenceGenerateFailed, events[1].Event)
	core.AssertEqual(t, audit.OutcomeFailed, events[1].Outcome)

	code, ok := events[1].Meta["error_code"].(string)
	core.AssertTrue(t, ok, "Failed Meta must carry error_code")
	core.AssertEqual(t, "ai.ProviderRouter.Chat", code,
		"router's cancellation error must surface as the stable router Operation code")
}

// TestRunner_ChatCtx_ClientDisconnect_Bad — same shape as the Generate
// variant but on the messages-array Chat path. Both HTTP egress sites
// (Generate via /v1/completions, Chat via /v1/chat/completions) must
// honour client-disconnect cancellation uniformly.
func TestRunner_ChatCtx_ClientDisconnect_Bad(t *core.T) {
	rec := installRecorder(t)
	s := newStubRunner(&runnerStubModel{output: "would-be-reply"})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel — simulates client disconnect

	r := s.ChatCtx(ctx, []inference.Message{{Role: "user", Content: "hi"}})
	core.AssertFalse(t, r.OK,
		"cancelled ctx should propagate into router.Chat → Failed result, not OK")

	events := rec.snapshot()
	core.AssertEqual(t, 2, len(events),
		"cancelled ChatCtx should still emit Requested + Failed")
	core.AssertEqual(t, audit.EventInferenceChatRequested, events[0].Event)
	core.AssertEqual(t, audit.EventInferenceChatFailed, events[1].Event)
	core.AssertEqual(t, audit.OutcomeFailed, events[1].Outcome)

	code, ok := events[1].Meta["error_code"].(string)
	core.AssertTrue(t, ok, "Failed Meta must carry error_code")
	core.AssertEqual(t, "ai.ProviderRouter.Chat", code)
}

// TestRunner_Generate_BackgroundCtxShim_Good — verifies the existing
// Generate(prompt) surface continues to work as a Background-context
// shim around GenerateCtx. Action-bus, CLI, and Wails callers without
// a request scope rely on this shape; the ctx variant is opt-in for
// HTTP handlers only.
func TestRunner_Generate_BackgroundCtxShim_Good(t *core.T) {
	rec := installRecorder(t)
	s := newStubRunner(&runnerStubModel{
		output:  "shim-reply",
		metrics: inference.GenerateMetrics{GeneratedTokens: 1},
	})

	r := s.Generate("ping")
	core.AssertTrue(t, r.OK, "background-ctx shim must preserve existing Generate surface")
	core.AssertEqual(t, "shim-reply", r.Value.(string))

	events := rec.snapshot()
	core.AssertEqual(t, 2, len(events))
	core.AssertEqual(t, audit.EventInferenceGenerateRequested, events[0].Event)
	core.AssertEqual(t, audit.EventInferenceGenerateCompleted, events[1].Event)
	core.AssertEqual(t, audit.OutcomeOK, events[1].Outcome)
}

// TestRunner_Chat_BackgroundCtxShim_Good — Chat-path mirror of the
// Generate background-ctx shim test. Same guarantee: existing surface
// preserved for non-HTTP callers.
func TestRunner_Chat_BackgroundCtxShim_Good(t *core.T) {
	rec := installRecorder(t)
	s := newStubRunner(&runnerStubModel{
		output:  "shim-ack",
		metrics: inference.GenerateMetrics{GeneratedTokens: 1},
	})

	r := s.Chat([]inference.Message{{Role: "user", Content: "hi"}})
	core.AssertTrue(t, r.OK, "background-ctx shim must preserve existing Chat surface")
	core.AssertEqual(t, "shim-ack", r.Value.(string))

	events := rec.snapshot()
	core.AssertEqual(t, 2, len(events))
	core.AssertEqual(t, audit.EventInferenceChatRequested, events[0].Event)
	core.AssertEqual(t, audit.EventInferenceChatCompleted, events[1].Event)
}
