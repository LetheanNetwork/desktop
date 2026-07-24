// SPDX-Licence-Identifier: EUPL-1.2

//go:build !ios && !android

package main

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/services"
)

// cmdService dispatches `lthn service <verb> NAME` — lifecycle
// management of OS-level daemonisation. Each verb wraps a
// pkg/services entry-point that delegates to
// github.com/kardianos/service (launchd / systemd / Windows
// Service backends).
//
// Verbs:
//
//	lthn service install NAME    — register with the OS service mgr
//	lthn service uninstall NAME  — remove the registration
//	lthn service start NAME      — ask the OS to start the service
//	lthn service stop NAME       — ask the OS to stop the service
//	lthn service restart NAME    — stop + start
//	lthn service reload NAME     — alias for restart (future: SIGHUP)
//	lthn service status NAME     — running / stopped / not_installed
//	lthn service list            — show the registry
//
// install/uninstall are the verbs called by the macOS .pkg installer
// during postinstall / preuninstall scripts.
//
// Usage example:
//
//	rc := cmdService([]string{"install", "serve"})
func cmdService(args []string) int {
	if len(args) == 0 {
		core.Print(core.Stderr(),
			"lthn service: missing verb (install / uninstall / start / stop / restart / reload / status / list)\n")
		return 2
	}
	switch args[0] {
	case "list":
		return serviceList(args[1:])
	case "install":
		return serviceCall(args[0], args[1:], services.Install)
	case "uninstall":
		return serviceCall(args[0], args[1:], services.Uninstall)
	case "start":
		return serviceCall(args[0], args[1:], services.Start)
	case "stop":
		return serviceCall(args[0], args[1:], services.Stop)
	case "restart", "reload":
		return serviceCall(args[0], args[1:], services.Restart)
	case "status":
		return serviceStatus(args[1:])
	default:
		core.Print(core.Stderr(), "lthn service: unknown verb %q\n", args[0])
		return 2
	}
}

// serviceCall is the shared shape for install/uninstall/start/stop/
// restart — each takes a NAME and returns core.Ok(nil) on success.
func serviceCall(verb string, args []string, fn func(string) core.Result) int {
	if len(args) < 1 {
		core.Print(core.Stderr(), "lthn service %s: usage: lthn service %s NAME\n", verb, verb)
		return 2
	}
	r := fn(args[0])
	if !r.OK {
		core.Print(core.Stderr(), "lthn service %s: %s\n", verb, r.Error())
		return 1
	}
	return 0
}

func serviceStatus(args []string) int {
	if len(args) < 1 {
		core.Print(core.Stderr(), "lthn service status: usage: lthn service status NAME\n")
		return 2
	}
	r := services.Status(args[0])
	if !r.OK {
		core.Print(core.Stderr(), "lthn service status: %s\n", r.Error())
		return 1
	}
	if s, ok := r.Value.(string); ok {
		core.Println(s)
	}
	return 0
}

func serviceList(_ []string) int {
	entries := services.Registry()
	jr := core.JSONMarshalIndent(entries, "", "  ")
	if !jr.OK {
		core.Print(core.Stderr(), "lthn service list: %s\n", jr.Error())
		return 1
	}
	if b, ok := jr.Value.([]byte); ok {
		core.Print(core.Stdout(), "%s\n", string(b))
	}
	return 0
}
