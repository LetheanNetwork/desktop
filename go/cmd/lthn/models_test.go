// SPDX-Licence-Identifier: EUPL-1.2

//go:build !ios && !android

// models_test.go — coverage for the `lthn models` snapshot verbs.
// Dispatch and usage errors are pre-boot; list runs against an
// isolated HOME's (empty) catalogue; the pull failure arm uses an
// unsupported URL scheme so no network is touched.

package main

import (
	"testing"

	core "dappco.re/go"
)

// TestModels_CmdModels_Bad — missing verb, unknown verb, and the
// pull usage error.
func TestModels_CmdModels_Bad(t *testing.T) {
	core.AssertEqual(t, 2, cmdModels(nil))
	core.AssertEqual(t, 2, cmdModels([]string{"nonsense"}))
	core.AssertEqual(t, 2, cmdModels([]string{"pull", "url-only"}))
}

// TestModels_ModelsList_Good — an isolated HOME has an empty
// catalogue; the list still renders as JSON and exits clean.
func TestModels_ModelsList_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	core.AssertEqual(t, 0, cmdModels([]string{"list"}))
	core.AssertEqual(t, 0, cmdModels([]string{"ls"}))
}

// TestModels_ModelsPull_Ugly — an unsupported URL scheme fails the
// fetch without touching the network.
func TestModels_ModelsPull_Ugly(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	core.AssertEqual(t, 1, cmdModels([]string{"pull", "notaurl://nowhere/m.gguf", "m"}))
}
