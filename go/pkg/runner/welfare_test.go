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
