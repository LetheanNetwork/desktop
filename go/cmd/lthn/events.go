// SPDX-Licence-Identifier: EUPL-1.2

package main

import (
	"context"

	core "dappco.re/go"
)

// cmdEvents dispatches `lthn events <verb>` — operations against the
// go-stream Hub running on the shared Core.
//
// Usage example:
//
//	rc := cmdEvents([]string{"stats"})
func cmdEvents(args []string) int {
	if len(args) == 0 {
		core.Print(core.Stderr(), "lthn events: missing verb (stats / publish / config / running)\n")
		return 2
	}
	c := newAppCore()
	if c == nil {
		return 1
	}
	defer c.ServiceShutdown(context.Background())

	switch args[0] {
	case "stats":
		return eventsStats(c, args[1:])
	case "publish":
		return eventsPublish(c, args[1:])
	case "config":
		return eventsConfig(c, args[1:])
	case "running":
		return eventsRunning(c, args[1:])
	default:
		core.Print(core.Stderr(), "lthn events: unknown verb %q\n", args[0])
		return 2
	}
}

func eventsStats(c *core.Core, _ []string) int {
	r := c.Action("stream.stats").Run(context.Background(), core.NewOptions())
	if !r.OK {
		core.Print(core.Stderr(), "lthn events stats: %s\n", r.Error())
		return 1
	}
	if jr := core.JSONMarshalIndent(r.Value, "", "  "); jr.OK {
		if b, ok := jr.Value.([]byte); ok {
			core.Print(core.Stdout(), "%s\n", string(b))
		}
	}
	return 0
}

func eventsPublish(c *core.Core, args []string) int {
	if len(args) < 2 {
		core.Print(core.Stderr(), "lthn events publish: usage: lthn events publish CHANNEL FRAME\n")
		return 2
	}
	r := c.Action("stream.publish").Run(context.Background(), core.NewOptions(
		core.Option{Key: "channel", Value: args[0]},
		core.Option{Key: "frame", Value: []byte(args[1])},
	))
	if !r.OK {
		core.Print(core.Stderr(), "lthn events publish: %s\n", r.Error())
		return 1
	}
	return 0
}

func eventsConfig(c *core.Core, _ []string) int {
	r := c.Action("stream.config").Run(context.Background(), core.NewOptions())
	if !r.OK {
		core.Print(core.Stderr(), "lthn events config: %s\n", r.Error())
		return 1
	}
	if jr := core.JSONMarshalIndent(r.Value, "", "  "); jr.OK {
		if b, ok := jr.Value.([]byte); ok {
			core.Print(core.Stdout(), "%s\n", string(b))
		}
	}
	return 0
}

func eventsRunning(c *core.Core, _ []string) int {
	r := c.Action("stream.running").Run(context.Background(), core.NewOptions())
	if !r.OK {
		core.Print(core.Stderr(), "lthn events running: %s\n", r.Error())
		return 1
	}
	if running, ok := r.Value.(bool); ok {
		if running {
			core.Println("running")
		} else {
			core.Println("stopped")
		}
	}
	return 0
}
