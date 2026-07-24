// SPDX-Licence-Identifier: EUPL-1.2

//go:build !ios && !android

package main

import (
	core "dappco.re/go"
)

// cmdState dispatches `lthn state <verb>` — operations against the
// go-store backed runtime KV at ~/Lethean/data/lthn.db.
//
// Usage example:
//
//	rc := cmdState([]string{"set", "config", "host", "homelab"})
func cmdState(args []string) int {
	if len(args) == 0 {
		core.Print(core.Stderr(), "lthn state: missing verb (get / set / delete / list / groups)\n")
		return 2
	}
	c := newAppCore()
	if c == nil {
		return 1
	}
	defer c.ServiceShutdown(core.Background())

	switch args[0] {
	case "get":
		return stateGet(c, args[1:])
	case "set":
		return stateSet(c, args[1:])
	case "delete":
		return stateDelete(c, args[1:])
	case "list":
		return stateList(c, args[1:])
	case "groups":
		return stateGroups(c, args[1:])
	default:
		core.Print(core.Stderr(), "lthn state: unknown verb %q\n", args[0])
		return 2
	}
}

func stateGet(c *core.Core, args []string) int {
	if len(args) < 2 {
		core.Print(core.Stderr(), "lthn state get: usage: lthn state get GROUP KEY\n")
		return 2
	}
	r := c.Action("store.get").Run(core.Background(), core.NewOptions(
		core.Option{Key: "group", Value: args[0]},
		core.Option{Key: "key", Value: args[1]},
	))
	if !r.OK {
		core.Print(core.Stderr(), "lthn state get: %s\n", r.Error())
		return 1
	}
	if v, ok := r.Value.(string); ok {
		core.Println(v)
	}
	return 0
}

func stateSet(c *core.Core, args []string) int {
	if len(args) < 3 {
		core.Print(core.Stderr(), "lthn state set: usage: lthn state set GROUP KEY VALUE\n")
		return 2
	}
	r := c.Action("store.set").Run(core.Background(), core.NewOptions(
		core.Option{Key: "group", Value: args[0]},
		core.Option{Key: "key", Value: args[1]},
		core.Option{Key: "value", Value: args[2]},
	))
	if !r.OK {
		core.Print(core.Stderr(), "lthn state set: %s\n", r.Error())
		return 1
	}
	return 0
}

func stateDelete(c *core.Core, args []string) int {
	if len(args) < 2 {
		core.Print(core.Stderr(), "lthn state delete: usage: lthn state delete GROUP KEY\n")
		return 2
	}
	r := c.Action("store.delete").Run(core.Background(), core.NewOptions(
		core.Option{Key: "group", Value: args[0]},
		core.Option{Key: "key", Value: args[1]},
	))
	if !r.OK {
		core.Print(core.Stderr(), "lthn state delete: %s\n", r.Error())
		return 1
	}
	return 0
}

func stateList(c *core.Core, args []string) int {
	if len(args) < 1 {
		core.Print(core.Stderr(), "lthn state list: usage: lthn state list GROUP\n")
		return 2
	}
	r := c.Action("store.get_all").Run(core.Background(), core.NewOptions(
		core.Option{Key: "group", Value: args[0]},
	))
	if !r.OK {
		core.Print(core.Stderr(), "lthn state list: %s\n", r.Error())
		return 1
	}
	if m, ok := r.Value.(map[string]string); ok {
		for k, v := range m {
			core.Print(core.Stdout(), "%s\t%s\n", k, v)
		}
	}
	return 0
}

func stateGroups(c *core.Core, _ []string) int {
	r := c.Action("store.groups").Run(core.Background(), core.NewOptions())
	if !r.OK {
		core.Print(core.Stderr(), "lthn state groups: %s\n", r.Error())
		return 1
	}
	if groups, ok := r.Value.([]string); ok {
		for _, g := range groups {
			core.Println(g)
		}
	}
	return 0
}
