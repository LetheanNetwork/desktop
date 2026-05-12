// SPDX-Licence-Identifier: EUPL-1.2

package services

import (
	"os"

	core "dappco.re/go"
	nativeservice "github.com/kardianos/service"
)

// labelPrefix is prepended to the entry name to form the OS service
// label (com.lethean.<name>). Per the bundle-id `ai.lthn.desktop`
// memory, the user-facing identifier root is `ai.lthn` —
// "ai.lthn.serve" is the launchctl label.
const labelPrefix = "ai.lthn."

// noopProgram satisfies kardianos.service.Interface with no-ops.
// Used only because nativeservice.New requires it; we never call
// .Run() — the kardianos library uses the interface for the
// `lthn service run` codepath which we don't expose (the binary
// is already daemon-friendly through `lthn serve` etc).
type noopProgram struct{}

func (noopProgram) Start(_ nativeservice.Service) error { return nil }
func (noopProgram) Stop(_ nativeservice.Service) error  { return nil }

// controller returns a kardianos.Service for entry, configured to
// run THIS binary with entry.Arguments.
func controller(entry Entry) (nativeservice.Service, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	cfg := &nativeservice.Config{
		Name:        labelPrefix + entry.Name,
		DisplayName: entry.DisplayName,
		Description: entry.Description,
		Executable:  exe,
		Arguments:   append([]string{}, entry.Arguments...),
		Option: nativeservice.KeyValue{
			"KeepAlive": true,
			"RunAtLoad": true,
		},
	}
	return nativeservice.New(noopProgram{}, cfg)
}

// Install registers the service with the OS service manager. On
// macOS this writes ~/Library/LaunchAgents/ai.lthn.<name>.plist and
// loads it. On Linux ~/.config/systemd/user/ai.lthn.<name>.service.
// Idempotent at the OS level — calling twice is fine.
//
// Usage example:
//
//	r := services.Install("serve")
func Install(name string) core.Result {
	r := Lookup(name)
	if !r.OK {
		return r
	}
	entry := r.Value.(Entry)
	svc, err := controller(entry)
	if err != nil {
		return core.Fail(core.E("services.Install", "controller failed", err))
	}
	if err := svc.Install(); err != nil {
		return core.Fail(core.E("services.Install", "install failed", err))
	}
	return core.Ok(nil)
}

// Uninstall removes the service definition from the OS service
// manager. Stops first if running.
//
// Usage example:
//
//	r := services.Uninstall("serve")
func Uninstall(name string) core.Result {
	r := Lookup(name)
	if !r.OK {
		return r
	}
	entry := r.Value.(Entry)
	svc, err := controller(entry)
	if err != nil {
		return core.Fail(core.E("services.Uninstall", "controller failed", err))
	}
	// Best-effort stop before uninstall — ignore errors so users can
	// uninstall services that are already stopped.
	_ = svc.Stop()
	if err := svc.Uninstall(); err != nil {
		return core.Fail(core.E("services.Uninstall", "uninstall failed", err))
	}
	return core.Ok(nil)
}

// Start asks the OS service manager to start the service. The
// service must be installed first.
//
// Usage example:
//
//	r := services.Start("serve")
func Start(name string) core.Result {
	r := Lookup(name)
	if !r.OK {
		return r
	}
	entry := r.Value.(Entry)
	svc, err := controller(entry)
	if err != nil {
		return core.Fail(core.E("services.Start", "controller failed", err))
	}
	if err := svc.Start(); err != nil {
		return core.Fail(core.E("services.Start", "start failed", err))
	}
	return core.Ok(nil)
}

// Stop asks the OS service manager to stop the service. No-op when
// already stopped.
//
// Usage example:
//
//	r := services.Stop("serve")
func Stop(name string) core.Result {
	r := Lookup(name)
	if !r.OK {
		return r
	}
	entry := r.Value.(Entry)
	svc, err := controller(entry)
	if err != nil {
		return core.Fail(core.E("services.Stop", "controller failed", err))
	}
	if err := svc.Stop(); err != nil {
		return core.Fail(core.E("services.Stop", "stop failed", err))
	}
	return core.Ok(nil)
}

// Restart stops then starts the service. Convenience for the common
// "reload config" path.
//
// Usage example:
//
//	r := services.Restart("serve")
func Restart(name string) core.Result {
	r := Lookup(name)
	if !r.OK {
		return r
	}
	entry := r.Value.(Entry)
	svc, err := controller(entry)
	if err != nil {
		return core.Fail(core.E("services.Restart", "controller failed", err))
	}
	if err := svc.Restart(); err != nil {
		return core.Fail(core.E("services.Restart", "restart failed", err))
	}
	return core.Ok(nil)
}

// Status returns the OS service-manager view of the service —
// running / stopped / unknown.
//
// Usage example:
//
//	r := services.Status("serve")
//	if r.OK { state := r.Value.(string); _ = state }
func Status(name string) core.Result {
	r := Lookup(name)
	if !r.OK {
		return r
	}
	entry := r.Value.(Entry)
	svc, err := controller(entry)
	if err != nil {
		return core.Fail(core.E("services.Status", "controller failed", err))
	}
	state, err := svc.Status()
	if err != nil {
		// Treat not-installed as a distinct state rather than failure.
		if err == nativeservice.ErrNotInstalled {
			return core.Ok("not_installed")
		}
		return core.Fail(core.E("services.Status", "status failed", err))
	}
	switch state {
	case nativeservice.StatusRunning:
		return core.Ok("running")
	case nativeservice.StatusStopped:
		return core.Ok("stopped")
	default:
		return core.Ok("unknown")
	}
}
