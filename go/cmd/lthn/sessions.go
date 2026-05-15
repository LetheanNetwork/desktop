// SPDX-Licence-Identifier: EUPL-1.2

package main

import (

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/sessions"
)

// cmdSessions dispatches `lthn sessions <verb>` — operations against
// the persisted chat history backed by go-store.
//
// Usage example:
//
//	rc := cmdSessions([]string{"create", "first chat"})
func cmdSessions(args []string) int {
	if len(args) == 0 {
		core.Print(core.Stderr(), "lthn sessions: missing verb (create / list / read / append)\n")
		return 2
	}
	c := newAppCore()
	if c == nil {
		return 1
	}
	defer c.ServiceShutdown(core.Background())

	switch args[0] {
	case "create":
		return sessionsCreate(c, args[1:])
	case "list":
		return sessionsList(c, args[1:])
	case "read":
		return sessionsRead(c, args[1:])
	case "append":
		return sessionsAppend(c, args[1:])
	default:
		core.Print(core.Stderr(), "lthn sessions: unknown verb %q\n", args[0])
		return 2
	}
}

func sessionsCreate(c *core.Core, args []string) int {
	if len(args) < 1 {
		core.Print(core.Stderr(), "lthn sessions create: usage: lthn sessions create TITLE\n")
		return 2
	}
	r := sessions.Create(c, args[0])
	if !r.OK {
		core.Print(core.Stderr(), "lthn sessions create: %s\n", r.Error())
		return 1
	}
	if s, ok := r.Value.(string); ok {
		core.Println(s)
	}
	return 0
}

func sessionsList(c *core.Core, _ []string) int {
	r := sessions.List(c)
	if !r.OK {
		core.Print(core.Stderr(), "lthn sessions list: %s\n", r.Error())
		return 1
	}
	infos, ok := r.Value.([]sessions.SessionInfo)
	if !ok {
		core.Print(core.Stderr(), "lthn sessions list: unexpected value type\n")
		return 1
	}
	jr := core.JSONMarshalIndent(infos, "", "  ")
	if !jr.OK {
		core.Print(core.Stderr(), "lthn sessions list: %s\n", jr.Error())
		return 1
	}
	if b, ok := jr.Value.([]byte); ok {
		core.Print(core.Stdout(), "%s\n", string(b))
	}
	return 0
}

func sessionsRead(c *core.Core, args []string) int {
	if len(args) < 1 {
		core.Print(core.Stderr(), "lthn sessions read: usage: lthn sessions read ID\n")
		return 2
	}
	r := sessions.Read(c, args[0])
	if !r.OK {
		core.Print(core.Stderr(), "lthn sessions read: %s\n", r.Error())
		return 1
	}
	jr := core.JSONMarshalIndent(r.Value, "", "  ")
	if !jr.OK {
		core.Print(core.Stderr(), "lthn sessions read: %s\n", jr.Error())
		return 1
	}
	if b, ok := jr.Value.([]byte); ok {
		core.Print(core.Stdout(), "%s\n", string(b))
	}
	return 0
}

func sessionsAppend(c *core.Core, args []string) int {
	if len(args) < 3 {
		core.Print(core.Stderr(), "lthn sessions append: usage: lthn sessions append ID ROLE CONTENT\n")
		return 2
	}
	r := sessions.Append(c, args[0], args[1], args[2])
	if !r.OK {
		core.Print(core.Stderr(), "lthn sessions append: %s\n", r.Error())
		return 1
	}
	return 0
}
