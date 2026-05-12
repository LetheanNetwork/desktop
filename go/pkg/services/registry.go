// SPDX-Licence-Identifier: EUPL-1.2

// Package services manages the lifecycle of long-running lthn
// daemons at the OS level — launchd on macOS, systemd on Linux,
// Windows Service on Windows. Wraps github.com/kardianos/service so
// the platform-specific install/start/stop logic stays out of the
// CLI surface.
//
// The registry maps human names to the argv that runs them as a
// foreground process. install converts that argv into a native
// service definition; start/stop/uninstall delegate to the platform
// manager via kardianos.
//
// Usage example:
//
//	r := services.Install("serve")
//	if !r.OK { return r }
//	r = services.Start("serve")
//	if !r.OK { return r }
//
//	r = services.Stop("serve")
//	r = services.Uninstall("serve")
package services

import (
	core "dappco.re/go"
)

// Entry describes one lthn-named background service.
type Entry struct {
	// Name is the registry key (e.g. "serve", "tray").
	Name string
	// DisplayName is the user-facing name in launchctl / systemctl.
	DisplayName string
	// Description is shown by the OS service manager UI.
	Description string
	// Arguments are the argv tail passed to the lthn binary
	// (e.g. ["serve", "--port", "8000"]). The executable is the
	// running lthn binary path itself.
	Arguments []string
}

// Registry returns the known services. The registry is static today
// — entries are compiled in. Future: config-driven custom services
// via `services.<name>.arguments` in lthn.yaml.
func Registry() []Entry {
	return []Entry{
		{
			Name:        "serve",
			DisplayName: "Lethean Desktop API",
			Description: "OpenAI-compatible HTTP API for the local Lethean runner.",
			Arguments:   []string{"serve"},
		},
		{
			Name:        "tray",
			DisplayName: "Lethean Desktop Tray",
			Description: "System-tray runner for Lethean Desktop.",
			Arguments:   []string{"tray"},
		},
	}
}

// Lookup returns the entry for name, or core.Fail when unknown.
//
// Usage example:
//
//	r := services.Lookup("serve")
//	if r.OK { entry := r.Value.(Entry); _ = entry }
func Lookup(name string) core.Result {
	for _, e := range Registry() {
		if e.Name == name {
			return core.Ok(e)
		}
	}
	return core.Fail(core.E("services.Lookup",
		core.Concat("unknown service: ", name, " — known: ", knownNames()),
		nil))
}

// Names returns the names of registered services.
//
// Usage example:
//
//	names := services.Names()
func Names() []string {
	entries := Registry()
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.Name
	}
	return out
}

func knownNames() string {
	entries := Registry()
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name
	}
	return core.Join(", ", names...)
}
