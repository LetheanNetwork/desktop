// SPDX-Licence-Identifier: EUPL-1.2

// stream_test.go — real WChatStream coverage. stream_example_test.go
// only takes a method VALUE (`ref := (*subject.Service).WChatStream`)
// and Sprintf's its %T — it never calls WChatStream, so the
// echo-stub branch (no routes configured — the pre-wiring fallback)
// never ran anywhere in the suite. This file supplies a real call.

package runner_test

import (
	core "dappco.re/go"
	"dappco.re/go/inference"

	"dappco.re/lthn/desktop/pkg/runner"
)

// TestRunner_WChatStream_Good_EchoStubUsesLastUserMessage — a Service
// constructed with no routes (the documented pre-wiring fallback)
// must echo the LAST user message back through both the delta and
// done events, and the returned ChatReply. Router-less EmitEvent
// calls are safe no-ops (webkit.EmitEvent tolerates a nil Core), so
// this needs no event-capture seam.
func TestRunner_WChatStream_Good_EchoStubUsesLastUserMessage(t *core.T) {
	s := runner.NewService(runner.Options{})

	r := s.WChatStream("call-echo", []inference.Message{
		{Role: "user", Content: "first"},
		{Role: "assistant", Content: "reply"},
		{Role: "user", Content: "second and latest"},
	}, "")

	core.AssertTrue(t, r.OK, "echo-stub stream must succeed")
	reply, ok := r.Value.(runner.ChatReply)
	core.AssertTrue(t, ok, "WChatStream must return a ChatReply")
	core.AssertContains(t, reply.Text, "second and latest",
		"echo-stub reply must embed the LAST user message, not the first")
}

// TestRunner_WChatStream_Bad_EmptyCallID mirrors the WGenerate/WChat
// cap tests but for the streaming entry point's own call-id guard.
func TestRunner_WChatStream_Bad_EmptyCallID(t *core.T) {
	s := runner.NewService(runner.Options{})
	r := s.WChatStream("   ", []inference.Message{{Role: "user", Content: "hi"}}, "")
	core.AssertFalse(t, r.OK, "WChatStream must reject a blank call id")
}
