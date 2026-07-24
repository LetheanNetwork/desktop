// SPDX-Licence-Identifier: EUPL-1.2

//go:build !ios && !android

package main

import (
	core "dappco.re/go"
)

// cmdConfig dispatches `lthn config <verb>` — operations against
// dappco.re/go/config running on the shared Core. Config file lives
// at ~/Lethean/conf/lthn.yaml.
//
// Usage example:
//
//	rc := cmdConfig([]string{"set", "transport.port", "8000"})
func cmdConfig(args []string) int {
	if len(args) == 0 {
		core.Print(core.Stderr(), "lthn config: missing verb (get / set / list / commit / path)\n")
		return 2
	}
	c := newAppCore()
	if c == nil {
		return 1
	}
	defer c.ServiceShutdown(core.Background())

	switch args[0] {
	case "get":
		return configGet(c, args[1:])
	case "set":
		return configSet(c, args[1:])
	case "list", "all":
		return configAll(c, args[1:])
	case "commit":
		return configCommit(c, args[1:])
	case "path":
		return configPath(c, args[1:])
	default:
		core.Print(core.Stderr(), "lthn config: unknown verb %q\n", args[0])
		return 2
	}
}

func configGet(c *core.Core, args []string) int {
	if len(args) < 1 {
		core.Print(core.Stderr(), "lthn config get: usage: lthn config get KEY\n")
		return 2
	}
	r := c.Action("config.get").Run(core.Background(), core.NewOptions(
		core.Option{Key: "key", Value: args[0]},
	))
	if !r.OK {
		core.Print(core.Stderr(), "lthn config get: %s\n", r.Error())
		return 1
	}
	if jr := core.JSONMarshalIndent(r.Value, "", "  "); jr.OK {
		if b, ok := jr.Value.([]byte); ok {
			core.Print(core.Stdout(), "%s\n", string(b))
		}
	}
	return 0
}

func configSet(c *core.Core, args []string) int {
	if len(args) < 2 {
		core.Print(core.Stderr(), "lthn config set: usage: lthn config set KEY VALUE\n")
		return 2
	}
	r := c.Action("config.set").Run(core.Background(), core.NewOptions(
		core.Option{Key: "key", Value: args[0]},
		core.Option{Key: "value", Value: args[1]},
	))
	if !r.OK {
		core.Print(core.Stderr(), "lthn config set: %s\n", r.Error())
		return 1
	}
	// Persist after every set — CLI flow.
	commit := c.Action("config.commit").Run(core.Background(), core.NewOptions())
	if !commit.OK {
		core.Print(core.Stderr(), "lthn config set: commit failed: %s\n", commit.Error())
		return 1
	}
	return 0
}

func configAll(c *core.Core, _ []string) int {
	r := c.Action("config.all").Run(core.Background(), core.NewOptions())
	if !r.OK {
		core.Print(core.Stderr(), "lthn config list: %s\n", r.Error())
		return 1
	}
	if jr := core.JSONMarshalIndent(r.Value, "", "  "); jr.OK {
		if b, ok := jr.Value.([]byte); ok {
			core.Print(core.Stdout(), "%s\n", string(b))
		}
	}
	return 0
}

func configCommit(c *core.Core, _ []string) int {
	r := c.Action("config.commit").Run(core.Background(), core.NewOptions())
	if !r.OK {
		core.Print(core.Stderr(), "lthn config commit: %s\n", r.Error())
		return 1
	}
	return 0
}

func configPath(c *core.Core, _ []string) int {
	r := c.Action("config.path").Run(core.Background(), core.NewOptions())
	if !r.OK {
		core.Print(core.Stderr(), "lthn config path: %s\n", r.Error())
		return 1
	}
	if s, ok := r.Value.(string); ok {
		core.Println(s)
	}
	return 0
}
