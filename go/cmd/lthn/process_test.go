// SPDX-Licence-Identifier: EUPL-1.2

//go:build !ios && !android

// process_test.go — end-to-end coverage for the `lthn process`
// supervisor verbs on the composed app Core under an isolated HOME.
// No test spawns a real process: the run arm uses a nonexistent
// binary so the failure surfaces before any exec.

package main

import (
	"testing"

	core "dappco.re/go"
)

// TestProcess_CmdProcess_Bad — missing verb is pre-boot; unknown
// verb and every per-verb usage guard surface after the boot.
func TestProcess_CmdProcess_Bad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	core.AssertEqual(t, 2, cmdProcess(nil))
	core.AssertEqual(t, 2, cmdProcess([]string{"nonsense"}))
	core.AssertEqual(t, 2, cmdProcess([]string{"run"}))
	core.AssertEqual(t, 2, cmdProcess([]string{"start"}))
	core.AssertEqual(t, 2, cmdProcess([]string{"kill"}))
	core.AssertEqual(t, 2, cmdProcess([]string{"get"}))
}

// TestProcess_CmdProcess_Good — listing an empty supervisor renders
// JSON and exits clean, with and without the --running filter.
func TestProcess_CmdProcess_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	core.AssertEqual(t, 0, cmdProcess([]string{"list"}))
	core.AssertEqual(t, 0, cmdProcess([]string{"list", "--running"}))
}

// TestProcess_CmdProcess_Ugly — unknown ids and a nonexistent binary
// surface the service failures as exit 1.
func TestProcess_CmdProcess_Ugly(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	core.AssertEqual(t, 1, cmdProcess([]string{"kill", "no-such-id"}))
	core.AssertEqual(t, 1, cmdProcess([]string{"get", "no-such-id"}))
	core.AssertEqual(t, 1, cmdProcess([]string{"run", "/definitely/not/a/binary-xyz"}))
}
