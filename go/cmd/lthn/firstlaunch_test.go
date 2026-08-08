// SPDX-Licence-Identifier: EUPL-1.2

//go:build !ios && !android

// firstlaunch_test.go — coverage for `lthn firstlaunch`, the pure
// path-inspection diagnostic (deliberately Core-free so the check
// itself cannot create ~/Lethean state).

package main

import (
	"testing"

	core "dappco.re/go"
)

// TestFirstlaunch_CmdFirstLaunch_Good — a fresh isolated HOME
// reports its state as JSON and exits clean.
func TestFirstlaunch_CmdFirstLaunch_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	core.AssertEqual(t, 0, cmdFirstLaunch(nil))
}
