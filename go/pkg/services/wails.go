// SPDX-Licence-Identifier: EUPL-1.2

// Package services exposes the Desktop managed-service lifecycle to Wails.
// Native launchd/systemd management remains available only through methods
// whose names explicitly identify that separate compatibility surface.
package services

import (
	core "dappco.re/go"
)

// WailsService is the renderer-safe managed-service binding.
type WailsService struct {
	manager *Service
}

// NewWailsService wraps the Core-owned manager. A nil manager fails closed.
func NewWailsService(manager *Service) *WailsService {
	return &WailsService{manager: manager}
}

// ServiceName preserves the existing generated Wails binding name.
func (service *WailsService) ServiceName() string { return "Lifecycle" }

// ServiceStartup is a Wails binding lifecycle no-op. Core owns the manager.
func (service *WailsService) ServiceStartup(_ core.Context, _ any) core.Result {
	return core.Ok(nil)
}

// ServiceShutdown is a Wails binding lifecycle no-op. Closing a window must
// not stop Core-owned background services.
func (service *WailsService) ServiceShutdown() core.Result {
	return core.Ok(nil)
}

// Catalogue returns all renderer-safe managed-service snapshots.
func (service *WailsService) Catalogue() core.Result {
	if result := service.requireManager("services.WailsService.Catalogue"); !result.OK {
		return result
	}
	return service.manager.Catalogue()
}

// Get returns one renderer-safe managed-service snapshot.
func (service *WailsService) Get(id string) core.Result {
	if result := service.requireManager("services.WailsService.Get"); !result.OK {
		return result
	}
	return service.manager.Get(id)
}

// Start explicitly starts one known trusted managed-service definition.
func (service *WailsService) Start(id string) core.Result {
	if result := service.requireManager("services.WailsService.Start"); !result.OK {
		return result
	}
	return service.manager.Start(id)
}

// Stop explicitly stops one known managed-service process generation.
func (service *WailsService) Stop(id string) core.Result {
	if result := service.requireManager("services.WailsService.Stop"); !result.OK {
		return result
	}
	return service.manager.Stop(id)
}

// Restart explicitly cycles one known managed-service process generation.
func (service *WailsService) Restart(id string) core.Result {
	if result := service.requireManager("services.WailsService.Restart"); !result.OK {
		return result
	}
	return service.manager.Restart(id)
}

// Signal delivers a named signal to one known running managed service.
//
// The request carries a name, never a signal number — the renderer chooses
// from a fixed vocabulary for the same reason it cannot choose a command or an
// absolute path.
func (service *WailsService) Signal(request SignalRequest) core.Result {
	if result := service.requireManager("services.WailsService.Signal"); !result.OK {
		return result
	}
	// Refused here as well as in the manager. The boundary is where a
	// malformed request should stop, and the manager should not be the only
	// thing standing between a renderer and a syscall.
	if !validSignal(request.Signal) {
		return failureResult(
			ErrorSignalUnknown,
			"services.WailsService.Signal",
			"That is not a signal this manager sends.",
			nil,
		)
	}
	return service.manager.Signal(request)
}

// Kill ends one known managed-service process tree without waiting.
func (service *WailsService) Kill(id string) core.Result {
	if result := service.requireManager("services.WailsService.Kill"); !result.OK {
		return result
	}
	return service.manager.Kill(id)
}

// Output returns a bounded transient output tail for a known running service.
func (service *WailsService) Output(request OutputRequest) core.Result {
	if result := service.requireManager("services.WailsService.Output"); !result.OK {
		return result
	}
	return service.manager.Output(request)
}

// SetPolicy updates the bounded restart and graceful-stop policy for a known
// service without exposing its execution definition to the renderer.
func (service *WailsService) SetPolicy(override PolicyOverride) core.Result {
	if result := service.requireManager("services.WailsService.SetPolicy"); !result.OK {
		return result
	}
	return service.manager.SetPolicy(override)
}

func (service *WailsService) requireManager(operation string) core.Result {
	if service == nil || service.manager == nil {
		return failureResult(
			ErrorServicesUnavailable,
			operation,
			"The managed-services runtime is unavailable.",
			nil,
		)
	}
	return core.Ok(nil)
}

// NativeRegistry returns the legacy OS service-manager catalogue.
func (service *WailsService) NativeRegistry() []Entry { return Registry() }

// InstallNative installs a known legacy launchd/systemd definition.
func (service *WailsService) InstallNative(name string) core.Result {
	return Install(name)
}

// UninstallNative removes a known legacy launchd/systemd definition.
func (service *WailsService) UninstallNative(name string) core.Result {
	return Uninstall(name)
}

// StartNative starts a known legacy launchd/systemd definition.
func (service *WailsService) StartNative(name string) core.Result {
	return Start(name)
}

// StopNative stops a known legacy launchd/systemd definition.
func (service *WailsService) StopNative(name string) core.Result {
	return Stop(name)
}

// RestartNative cycles a known legacy launchd/systemd definition.
func (service *WailsService) RestartNative(name string) core.Result {
	return Restart(name)
}

// StatusNative reports a known legacy launchd/systemd definition.
func (service *WailsService) StatusNative(name string) core.Result {
	return Status(name)
}
