// SPDX-Licence-Identifier: EUPL-1.2

//go:build !ios && !android

// ai_test.go — coverage for the `lthn ai` verb surface. Usage errors
// and printReply run without any Core; the verb bodies boot the real
// composed app Core under an isolated HOME, where no routes are
// configured, and assert the documented degraded behaviour.

package main

import (
	"testing"

	core "dappco.re/go"
)

// TestAi_AiChat_Bad — a missing message is a usage error before boot.
func TestAi_AiChat_Bad(t *testing.T) {
	core.AssertEqual(t, 2, aiChat(nil))
}

// TestAi_AiGenerate_Bad — a missing prompt is a usage error before
// boot.
func TestAi_AiGenerate_Bad(t *testing.T) {
	core.AssertEqual(t, 2, aiGenerate(nil))
}

// TestAi_AiModels_Bad — `pull` redirects to the Lemma admin surface
// (with and without a repo argument) and unknown verbs are rejected,
// all before boot.
func TestAi_AiModels_Bad(t *testing.T) {
	core.AssertEqual(t, 2, aiModels([]string{"pull"}))
	core.AssertEqual(t, 2, aiModels([]string{"pull", "org/repo"}))
	core.AssertEqual(t, 2, aiModels([]string{"nonsense"}))
}

// TestAi_AiModels_Good — with no routes configured under an isolated
// HOME the list is empty and exits clean; the legacy `ls` spelling
// takes the same path after printing its note.
func TestAi_AiModels_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	core.AssertEqual(t, 0, aiModels(nil))
	core.AssertEqual(t, 0, aiModels([]string{"ls"}))
}

// TestAi_AiChat_Good — a real boot with no routes configured serves
// the documented echo stub ("Empty list = the runner serves the echo
// stub"), so the one-shot chat still exits clean.
func TestAi_AiChat_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	core.AssertEqual(t, 0, aiChat([]string{"hello"}))
}

// TestAi_AiGenerate_Good — same echo-stub composition for the
// completion verb.
func TestAi_AiGenerate_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	core.AssertEqual(t, 0, aiGenerate([]string{"write a haiku"}))
}

// TestAi_PrintReply_Good — a string reply prints and exits 0; a
// non-string OK value stays silent but still exits 0.
func TestAi_PrintReply_Good(t *testing.T) {
	core.AssertEqual(t, 0, printReply(core.Ok("hi"), "ai chat"))
	core.AssertEqual(t, 0, printReply(core.Ok(123), "ai chat"))
}

// TestAi_PrintReply_Bad — a failed Result reports with the op prefix
// and exits 1.
func TestAi_PrintReply_Bad(t *testing.T) {
	r := core.Fail(core.E("test.printReply", "boom", nil))
	core.AssertEqual(t, 1, printReply(r, "ai generate"))
}
