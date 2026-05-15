// SPDX-Licence-Identifier: EUPL-1.2

package main

import (
	core "dappco.re/go"
	lthnprocess "dappco.re/lthn/desktop/pkg/process"
)

// cmdProcess dispatches `lthn process <verb>` — thin shim that looks
// up the Core-registered lthn-process service and forwards verbs into
// it. No business logic; the service owns all action wrappers.
func cmdProcess(args []string) int {
	if len(args) == 0 {
		core.Print(core.Stderr(), "lthn process: missing verb (run / start / kill / list / get)\n")
		return 2
	}
	c := newAppCore()
	if c == nil {
		return 1
	}
	defer c.ServiceShutdown(core.Background())
	svc, _ := core.ServiceFor[*lthnprocess.Service](c, "lthn-process")
	if svc == nil {
		core.Print(core.Stderr(), "lthn process: service not registered\n")
		return 1
	}

	switch args[0] {
	case "run":
		return printResultJSON(svc.Run(args[1], args[2:]), "process run", len(args) < 2)
	case "start":
		return printResultJSON(svc.Start(args[1], args[2:]), "process start", len(args) < 2)
	case "kill":
		if len(args) < 2 {
			core.Print(core.Stderr(), "lthn process kill: usage: lthn process kill ID\n")
			return 2
		}
		r := svc.Kill(args[1])
		if !r.OK {
			core.Print(core.Stderr(), "lthn process kill: %s\n", r.Error())
			return 1
		}
		return 0
	case "list":
		runningOnly := false
		for _, a := range args[1:] {
			if a == "--running" {
				runningOnly = true
			}
		}
		return printResultJSON(svc.List(runningOnly), "process list", false)
	case "get":
		if len(args) < 2 {
			core.Print(core.Stderr(), "lthn process get: usage: lthn process get ID\n")
			return 2
		}
		return printResultJSON(svc.Get(args[1]), "process get", false)
	default:
		core.Print(core.Stderr(), "lthn process: unknown verb %q\n", args[0])
		return 2
	}
}

// printResultJSON renders a core.Result as indented JSON to stdout
// (success) or its error to stderr (failure). usageMissing short-
// circuits with a usage message when the caller passed too few args.
func printResultJSON(r core.Result, op string, usageMissing bool) int {
	if usageMissing {
		core.Print(core.Stderr(), "lthn %s: missing required argument\n", op)
		return 2
	}
	if !r.OK {
		core.Print(core.Stderr(), "lthn %s: %s\n", op, r.Error())
		return 1
	}
	if jr := core.JSONMarshalIndent(r.Value, "", "  "); jr.OK {
		if b, ok := jr.Value.([]byte); ok {
			core.Print(core.Stdout(), "%s\n", string(b))
		}
	}
	return 0
}
