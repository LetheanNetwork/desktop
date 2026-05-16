// SPDX-Licence-Identifier: EUPL-1.2

// Cerberus Mantis #1426 — defence-in-depth caps on the
// WGenerate / WChat Wails surface. Threat is a compromised renderer
// or future plugin-tier caller submitting billion-message arrays /
// multi-GiB payloads. These tests verify the cap rejects on the
// over-size paths before any allocator work happens.

package runner_test

import (
	core "dappco.re/go"
	"dappco.re/go/inference"

	"dappco.re/lthn/desktop/pkg/runner"
)

// oversizePrompt returns a string strictly larger than the WGenerate
// per-call cap (1 MiB). Used to drive the cap-rejection paths.
func oversizePrompt(n int) string {
	buf := make([]byte, n)
	for i := range buf {
		buf[i] = 'X'
	}
	return string(buf)
}

func TestRunner_WGenerate_Bad_OversizedPrompt(t *core.T) {
	s := runner.NewService(runner.Options{})
	// 1 MiB cap + 1 byte = guaranteed reject.
	r := s.WGenerate(oversizePrompt(1<<20 + 1))
	core.AssertFalse(t, r.OK,
		"WGenerate must refuse prompts larger than the cap")
}

func TestRunner_WChat_Bad_TooManyMessages(t *core.T) {
	s := runner.NewService(runner.Options{})
	// 2001 messages exceeds the 2000 cap. Tiny content so the count
	// check fails before the per-message + total checks.
	msgs := make([]inference.Message, 2001)
	for i := range msgs {
		msgs[i] = inference.Message{Role: "user", Content: "x"}
	}
	r := s.WChat(msgs)
	core.AssertFalse(t, r.OK,
		"WChat must refuse message arrays beyond the count cap")
}

func TestRunner_WChat_Bad_OversizedSingleMessage(t *core.T) {
	s := runner.NewService(runner.Options{})
	// Single message whose Role+Content exceeds 1 MiB.
	r := s.WChat([]inference.Message{
		{Role: "user", Content: oversizePrompt(1<<20 + 1)},
	})
	core.AssertFalse(t, r.OK,
		"WChat must refuse a single message larger than the per-msg cap")
}

func TestRunner_WChat_Bad_OversizedCumulative(t *core.T) {
	s := runner.NewService(runner.Options{})
	// Nine messages, each just under the per-msg cap, total exceeds
	// the 8 MiB cumulative cap. 9 × ~1 MiB = ~9 MiB > 8 MiB.
	near := oversizePrompt(1 << 20)
	msgs := make([]inference.Message, 9)
	for i := range msgs {
		// Role consumes a few bytes but Content stays under 1 MiB so
		// each individual message passes the per-msg cap; the
		// cumulative is what fails.
		msgs[i] = inference.Message{Role: "u", Content: near}
	}
	r := s.WChat(msgs)
	core.AssertFalse(t, r.OK,
		"WChat must refuse a message array whose total bytes exceed the cumulative cap")
}

// TestRunner_WChat_Good_AtCapNoReject — a request right at the
// boundary on EVERY axis (message count, per-msg size, total bytes)
// must NOT be rejected by the cap layer. The downstream Chat call
// may still fail (no router configured in this test), so we don't
// assert OK; we assert that the failure mode is NOT one of the
// cap-rejection paths. This catches off-by-one in the comparisons.
func TestRunner_WChat_Good_AtCapNoReject(t *core.T) {
	s := runner.NewService(runner.Options{})
	// 8 messages, each exactly 1 MiB Content + 0-byte Role.
	// Hits the per-msg cap exactly (1 MiB == cap, not > cap) and the
	// total cap exactly (8 × 1 MiB == 8 MiB == total cap).
	// `size > cap` semantics means equality must pass.
	atMsg := oversizePrompt(1 << 20)
	msgs := make([]inference.Message, 8)
	for i := range msgs {
		msgs[i] = inference.Message{Role: "", Content: atMsg}
	}
	r := s.WChat(msgs)
	// Either OK (no router → echo stub on Chat path) or a Chat-layer
	// failure — but the error string must NOT contain "cap" /
	// "exceeds" / "count exceeds", which are the cap-rejection sentinels.
	if !r.OK {
		msg := r.Error()
		core.AssertFalse(t, core.Contains(msg, "cap"),
			"at-cap WChat must not trip the cap layer; got: "+msg)
		core.AssertFalse(t, core.Contains(msg, "exceeds"),
			"at-cap WChat must not trip the cap layer; got: "+msg)
	}
}

// TestRunner_WGenerate_Good_AtCapNoReject — same boundary check on
// the single-prompt path.
func TestRunner_WGenerate_Good_AtCapNoReject(t *core.T) {
	s := runner.NewService(runner.Options{})
	// Exactly at the 1 MiB cap; must NOT be rejected by the cap layer.
	r := s.WGenerate(oversizePrompt(1 << 20))
	if !r.OK {
		msg := r.Error()
		core.AssertFalse(t, core.Contains(msg, "exceeds"),
			"at-cap WGenerate must not trip the cap layer; got: "+msg)
	}
}
