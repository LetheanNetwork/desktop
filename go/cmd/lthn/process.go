// SPDX-Licence-Identifier: EUPL-1.2

package main

import (
	"context"

	core "dappco.re/go"
)

// cmdProcess dispatches `lthn process <verb>` — operations against the
// go-process supervisor running on the shared Core.
//
// Usage example:
//
//	rc := cmdProcess([]string{"list"})
func cmdProcess(args []string) int {
	if len(args) == 0 {
		core.Print(core.Stderr(), "lthn process: missing verb (list / get)\n")
		return 2
	}
	c := newAppCore()
	if c == nil {
		return 1
	}
	defer c.ServiceShutdown(context.Background())

	switch args[0] {
	case "list":
		return processList(c, args[1:])
	case "get":
		return processGet(c, args[1:])
	default:
		core.Print(core.Stderr(), "lthn process: unknown verb %q\n", args[0])
		return 2
	}
}

func processList(c *core.Core, args []string) int {
	runningOnly := false
	for _, a := range args {
		if a == "--running" {
			runningOnly = true
		}
	}
	r := c.Action("process.list").Run(context.Background(), core.NewOptions(
		core.Option{Key: "runningOnly", Value: runningOnly},
	))
	if !r.OK {
		core.Print(core.Stderr(), "lthn process list: %s\n", r.Error())
		return 1
	}
	if jr := core.JSONMarshalIndent(r.Value, "", "  "); jr.OK {
		if b, ok := jr.Value.([]byte); ok {
			core.Print(core.Stdout(), "%s\n", string(b))
		}
	}
	return 0
}

func processGet(c *core.Core, args []string) int {
	if len(args) < 1 {
		core.Print(core.Stderr(), "lthn process get: usage: lthn process get ID\n")
		return 2
	}
	r := c.Action("process.get").Run(context.Background(), core.NewOptions(
		core.Option{Key: "id", Value: args[0]},
	))
	if !r.OK {
		core.Print(core.Stderr(), "lthn process get: %s\n", r.Error())
		return 1
	}
	if jr := core.JSONMarshalIndent(r.Value, "", "  "); jr.OK {
		if b, ok := jr.Value.([]byte); ok {
			core.Print(core.Stdout(), "%s\n", string(b))
		}
	}
	return 0
}
