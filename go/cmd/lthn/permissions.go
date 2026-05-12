// SPDX-Licence-Identifier: EUPL-1.2

package main

import (
	"context"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/permissions"
)

// cmdPermissions dispatches `lthn permissions <verb>`. Today thin —
// reads/writes via the config service. Lives as its own verb (rather
// than under `lthn config`) so the wizard surface is discoverable.
//
// Usage example:
//
//	rc := cmdPermissions([]string{"check", "network.outbound"})
func cmdPermissions(args []string) int {
	if len(args) == 0 {
		core.Print(core.Stderr(), "lthn permissions: missing verb (check / set / list)\n")
		return 2
	}
	c := newAppCore()
	if c == nil {
		return 1
	}
	defer c.ServiceShutdown(context.Background())
	permissions.Install(c)

	switch args[0] {
	case "check":
		return permissionsCheck(c, args[1:])
	case "set":
		return permissionsSet(c, args[1:])
	case "list":
		return permissionsList(c, args[1:])
	default:
		core.Print(core.Stderr(), "lthn permissions: unknown verb %q\n", args[0])
		return 2
	}
}

func permissionsCheck(c *core.Core, args []string) int {
	if len(args) < 1 {
		core.Print(core.Stderr(), "lthn permissions check: usage: lthn permissions check ACTION\n")
		return 2
	}
	e := c.Entitled(args[0])
	type view struct {
		Allowed   bool   `json:"allowed"`
		Unlimited bool   `json:"unlimited"`
		Limit     int    `json:"limit,omitempty"`
		Used      int    `json:"used,omitempty"`
		Remaining int    `json:"remaining,omitempty"`
		Reason    string `json:"reason,omitempty"`
	}
	v := view{
		Allowed:   e.Allowed,
		Unlimited: e.Unlimited,
		Limit:     e.Limit,
		Used:      e.Used,
		Remaining: e.Remaining,
		Reason:    e.Reason,
	}
	jr := core.JSONMarshalIndent(v, "", "  ")
	if !jr.OK {
		core.Print(core.Stderr(), "lthn permissions check: %s\n", jr.Error())
		return 1
	}
	if b, ok := jr.Value.([]byte); ok {
		core.Print(core.Stdout(), "%s\n", string(b))
	}
	if !e.Allowed {
		return 1
	}
	return 0
}

func permissionsSet(c *core.Core, args []string) int {
	if len(args) < 2 {
		core.Print(core.Stderr(), "lthn permissions set: usage: lthn permissions set ACTION VALUE\n")
		return 2
	}
	key := core.Concat("permissions.", args[0])
	r := c.Action("config.set").Run(context.Background(), core.NewOptions(
		core.Option{Key: "key", Value: key},
		core.Option{Key: "value", Value: args[1]},
	))
	if !r.OK {
		core.Print(core.Stderr(), "lthn permissions set: %s\n", r.Error())
		return 1
	}
	commit := c.Action("config.commit").Run(context.Background(), core.NewOptions())
	if !commit.OK {
		core.Print(core.Stderr(), "lthn permissions set: commit failed: %s\n", commit.Error())
		return 1
	}
	return 0
}

func permissionsList(c *core.Core, _ []string) int {
	r := c.Action("config.get").Run(context.Background(), core.NewOptions(
		core.Option{Key: "key", Value: "permissions"},
	))
	if !r.OK {
		// Not set yet — print empty object so callers always get JSON.
		core.Println("{}")
		return 0
	}
	jr := core.JSONMarshalIndent(r.Value, "", "  ")
	if !jr.OK {
		core.Print(core.Stderr(), "lthn permissions list: %s\n", jr.Error())
		return 1
	}
	if b, ok := jr.Value.([]byte); ok {
		core.Print(core.Stdout(), "%s\n", string(b))
	}
	return 0
}
