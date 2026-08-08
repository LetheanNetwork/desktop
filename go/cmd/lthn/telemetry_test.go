// SPDX-Licence-Identifier: EUPL-1.2

//go:build !ios && !android

package main

import core "dappco.re/go"

func TestCmdTelemetry_Host_Good(t *core.T) {
	core.AssertEqual(t, 0, cmdTelemetry([]string{"host"}))
}

func TestCmdTelemetry_BadRejectsMissingVerb(t *core.T) {
	core.AssertEqual(t, 2, cmdTelemetry(nil))
}

func TestCmdTelemetry_UglyRejectsUnknownVerb(t *core.T) {
	core.AssertEqual(t, 2, cmdTelemetry([]string{"unknown"}))
}

func TestCmdTelemetry_Sample_Good(t *core.T) {
	core.AssertEqual(t, 0, cmdTelemetry([]string{"sample"}))
}
