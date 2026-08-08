// SPDX-Licence-Identifier: EUPL-1.2

//go:build !ios && !android

// events_test.go — end-to-end coverage for the `lthn events` verb
// surface against the go-ws Hub registered on the composed app Core
// under an isolated HOME.

package main

import (
	"testing"

	core "dappco.re/go"
)

// TestEvents_CmdEvents_Bad — missing verb is pre-boot; unknown verb
// and the publish usage error surface after the boot.
func TestEvents_CmdEvents_Bad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	core.AssertEqual(t, 2, cmdEvents(nil))
	core.AssertEqual(t, 2, cmdEvents([]string{"nonsense"}))
	core.AssertEqual(t, 2, cmdEvents([]string{"publish", "channel-only"}))
}

// TestEvents_CmdEvents_Good — stats, config, running, and a publish
// to a subscriber-less channel all complete against the registered
// Hub.
func TestEvents_CmdEvents_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	core.AssertEqual(t, 0, cmdEvents([]string{"stats"}))
	core.AssertEqual(t, 0, cmdEvents([]string{"config"}))
	core.AssertEqual(t, 0, cmdEvents([]string{"running"}))
	core.AssertEqual(t, 0, cmdEvents([]string{"publish", "test.channel", `{"hello":"world"}`}))
}
