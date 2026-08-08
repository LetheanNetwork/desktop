// SPDX-Licence-Identifier: EUPL-1.2

//go:build !ios && !android

// state_test.go — end-to-end coverage for the `lthn state` verb
// surface. Every call drives the real dispatcher (which boots the
// composed app Core) under an isolated HOME; the go-store KV persists
// across invocations in that HOME, so the roundtrip asserts real
// on-disk state, not handler plumbing.

package main

import (
	"testing"

	core "dappco.re/go"
)

// TestState_CmdState_Bad — missing verb is pre-boot; unknown verb and
// per-verb usage errors surface after the boot.
func TestState_CmdState_Bad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	core.AssertEqual(t, 2, cmdState(nil))
	core.AssertEqual(t, 2, cmdState([]string{"nonsense"}))
	core.AssertEqual(t, 2, cmdState([]string{"get"}))
	core.AssertEqual(t, 2, cmdState([]string{"set", "group", "key"}))
	core.AssertEqual(t, 2, cmdState([]string{"delete", "group"}))
	core.AssertEqual(t, 2, cmdState([]string{"list"}))
}

// TestState_CmdState_Good — the full KV roundtrip: set persists,
// get / list / groups observe it, delete removes it.
func TestState_CmdState_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	core.AssertEqual(t, 0, cmdState([]string{"set", "config", "host", "homelab"}))
	core.AssertEqual(t, 0, cmdState([]string{"get", "config", "host"}))
	core.AssertEqual(t, 0, cmdState([]string{"list", "config"}))
	core.AssertEqual(t, 0, cmdState([]string{"groups"}))
	core.AssertEqual(t, 0, cmdState([]string{"delete", "config", "host"}))
}

// TestState_StateGet_Ugly — a key that was never set surfaces the
// store's failure as exit 1.
func TestState_StateGet_Ugly(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	core.AssertEqual(t, 1, cmdState([]string{"get", "ghost", "missing"}))
}
