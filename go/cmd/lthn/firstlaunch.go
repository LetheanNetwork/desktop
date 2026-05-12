// SPDX-Licence-Identifier: EUPL-1.2

package main

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/firstlaunch"
)

// cmdFirstLaunch handles `lthn firstlaunch` — detects whether the
// app is in a fresh state and reports the diagnostic JSON. Exits 0
// either way; the wizard caller branches on the `fresh` field.
//
// Does NOT call newAppCore — booting the Core would create
// ~/Lethean/data/lthn.db before the path check runs, defeating the
// "fresh install?" question. Pure path inspection here; the routes
// check is path-based too (read the yaml directly).
//
// Usage example:
//
//	rc := cmdFirstLaunch(nil)
func cmdFirstLaunch(_ []string) int {
	r := firstlaunch.Detect(nil)
	if !r.OK {
		core.Print(core.Stderr(), "lthn firstlaunch: %s\n", r.Error())
		return 1
	}
	jr := core.JSONMarshalIndent(r.Value, "", "  ")
	if !jr.OK {
		core.Print(core.Stderr(), "lthn firstlaunch: %s\n", jr.Error())
		return 1
	}
	if b, ok := jr.Value.([]byte); ok {
		core.Print(core.Stdout(), "%s\n", string(b))
	}
	return 0
}
