// SPDX-Licence-Identifier: EUPL-1.2

//go:build !ios && !android

package main

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/telemetry"
)

// cmdTelemetry handles `lthn telemetry <verb>` — process-level
// metrics sampling. Today: `sample` (one-shot JSON reading).
//
// Usage example:
//
//	rc := cmdTelemetry([]string{"sample"})
func cmdTelemetry(args []string) int {
	if len(args) == 0 {
		core.Print(core.Stderr(), "lthn telemetry: missing verb (sample)\n")
		return 2
	}
	switch args[0] {
	case "sample":
		return telemetrySample(args[1:])
	default:
		core.Print(core.Stderr(), "lthn telemetry: unknown verb %q\n", args[0])
		return 2
	}
}

func telemetrySample(_ []string) int {
	r := telemetry.Sample()
	if !r.OK {
		core.Print(core.Stderr(), "lthn telemetry sample: %s\n", r.Error())
		return 1
	}
	jr := core.JSONMarshalIndent(r.Value, "", "  ")
	if !jr.OK {
		core.Print(core.Stderr(), "lthn telemetry sample: %s\n", jr.Error())
		return 1
	}
	if b, ok := jr.Value.([]byte); ok {
		core.Print(core.Stdout(), "%s\n", string(b))
	}
	return 0
}
