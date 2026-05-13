// SPDX-Licence-Identifier: EUPL-1.2

// Wails3 Service shape for the lthnservices (lifecycle) package —
// wraps Install / Uninstall / Start / Stop / Restart / Status /
// Registry for the WebView. Bound by application.NewService(
// services.NewWailsService()) in pkg/desktop/desktop.go.
//
// Named Lifecycle in the TS binding (ServiceName below) so it's
// distinguishable in the WebView from other Service-named bindings.

package services

import (
	"context"

	core "dappco.re/go"
	"github.com/wailsapp/wails/v3/pkg/application"
)

type WailsService struct{}

func NewWailsService() *WailsService { return &WailsService{} }

func (s *WailsService) ServiceName() string { return "Lifecycle" }
func (s *WailsService) ServiceStartup(_ context.Context, _ application.ServiceOptions) core.Result {
	return core.Ok(nil)
}
func (s *WailsService) ServiceShutdown() core.Result { return core.Ok(nil) }

// Registry returns every known service entry — what's installable.
func (s *WailsService) Registry() []Entry { return Registry() }

// Install registers the named service with the OS service manager.
func (s *WailsService) Install(name string) core.Result {
	r := Install(name)
	if !r.OK {
		return core.Fail(core.E("services.WailsService.Install", "install service failed", r.Value.(error)))
	}
	return core.Ok(nil)
}

// Uninstall removes the named service from the OS service manager.
func (s *WailsService) Uninstall(name string) core.Result {
	r := Uninstall(name)
	if !r.OK {
		return core.Fail(core.E("services.WailsService.Uninstall", "uninstall service failed", r.Value.(error)))
	}
	return core.Ok(nil)
}

// Start starts the named service.
func (s *WailsService) Start(name string) core.Result {
	r := Start(name)
	if !r.OK {
		return core.Fail(core.E("services.WailsService.Start", "start service failed", r.Value.(error)))
	}
	return core.Ok(nil)
}

// Stop stops the named service.
func (s *WailsService) Stop(name string) core.Result {
	r := Stop(name)
	if !r.OK {
		return core.Fail(core.E("services.WailsService.Stop", "stop service failed", r.Value.(error)))
	}
	return core.Ok(nil)
}

// Restart cycles the named service.
func (s *WailsService) Restart(name string) core.Result {
	r := Restart(name)
	if !r.OK {
		return core.Fail(core.E("services.WailsService.Restart", "restart service failed", r.Value.(error)))
	}
	return core.Ok(nil)
}

// Status returns the named service's current status string —
// "running" / "stopped" / "unknown" / etc.
func (s *WailsService) Status(name string) core.Result {
	r := Status(name)
	if !r.OK {
		return core.Fail(core.E("services.WailsService.Status", "read service status failed", r.Value.(error)))
	}
	status, _ := r.Value.(string)
	return core.Ok(status)
}
