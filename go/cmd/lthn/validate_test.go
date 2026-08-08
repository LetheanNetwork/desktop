// SPDX-Licence-Identifier: EUPL-1.2

//go:build !ios && !android

// validate_test.go — coverage for `lthn validate URL`. The usage
// error is free; the probe arm targets a loopback port nothing
// listens on, so the connection refuses instantly without external
// network traffic.

package main

import (
	"testing"

	core "dappco.re/go"
)

// TestValidate_CmdValidate_Bad — a missing URL is a usage error.
func TestValidate_CmdValidate_Bad(t *testing.T) {
	core.AssertEqual(t, 2, cmdValidate(nil))
}

// TestValidate_CmdValidate_Ugly — an unreachable loopback endpoint
// fails the probe and exits 1.
func TestValidate_CmdValidate_Ugly(t *testing.T) {
	core.AssertEqual(t, 1, cmdValidate([]string{"http://127.0.0.1:1/v1"}))
}
