// SPDX-Licence-Identifier: EUPL-1.2

package modelruntime

import core "dappco.re/go"

// WailsService is the exact renderer-safe ModelRuntime binding.
type WailsService struct {
	runtime *Service
}

// NewWailsService wraps the Core-owned runtime. A nil service fails closed.
func NewWailsService(runtime *Service) *WailsService {
	return &WailsService{runtime: runtime}
}

// ServiceName fixes the generated Wails namespace.
func (service *WailsService) ServiceName() string { return "ModelRuntime" }

// ServiceStartup is inert because Core owns runtime lifecycle.
func (service *WailsService) ServiceStartup(core.Context, any) core.Result {
	return core.Ok(nil)
}

// ServiceShutdown does not stop the Core-owned runtime when a window closes.
func (service *WailsService) ServiceShutdown() core.Result { return core.Ok(nil) }

// Snapshot returns one immutable renderer-safe runtime observation.
func (service *WailsService) Snapshot() core.Result {
	if result := service.requireRuntime("modelruntime.WailsService.Snapshot"); !result.OK {
		return result
	}
	return service.runtime.Snapshot()
}

// Start explicitly starts the model-less runtime.
func (service *WailsService) Start() core.Result {
	if result := service.requireRuntime("modelruntime.WailsService.Start"); !result.OK {
		return result
	}
	return service.runtime.Start()
}

// Load selects one opaque catalogue model ID.
func (service *WailsService) Load(request LoadRequest) core.Result {
	if result := service.requireRuntime("modelruntime.WailsService.Load"); !result.OK {
		return result
	}
	return service.runtime.Load(request)
}

// Unload restarts the single-model host back to model-less.
func (service *WailsService) Unload() core.Result {
	if result := service.requireRuntime("modelruntime.WailsService.Unload"); !result.OK {
		return result
	}
	return service.runtime.Unload()
}

// Restart cycles the managed runtime without reloading a model.
func (service *WailsService) Restart() core.Result {
	if result := service.requireRuntime("modelruntime.WailsService.Restart"); !result.OK {
		return result
	}
	return service.runtime.Restart()
}

// Stop explicitly stops the managed runtime.
func (service *WailsService) Stop() core.Result {
	if result := service.requireRuntime("modelruntime.WailsService.Stop"); !result.OK {
		return result
	}
	return service.runtime.Stop()
}

func (service *WailsService) requireRuntime(operation string) core.Result {
	if service == nil || service.runtime == nil {
		return fail(
			ErrorRuntimeUnavailable,
			operation,
			"The model runtime is unavailable.",
			nil,
		)
	}
	return core.Ok(nil)
}
