// SPDX-Licence-Identifier: EUPL-1.2

package modelruntime

import (
	"time"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/models"
	"dappco.re/lthn/desktop/pkg/services"
)

// Start explicitly starts the model-less LEM sidecar and waits for health.
func (service *Service) Start() core.Result {
	ctx, release, result := service.beginMutation(StateStarting, "start")
	if !result.OK {
		return result
	}
	defer release()

	managedResult := service.ensureRunning()
	if !managedResult.OK {
		return service.failMutation("start", ErrorRuntimeStartFailed, managedResult)
	}
	managed := managedResult.Value.(services.Snapshot)
	healthResult := service.waitHealth(ctx)
	if !healthResult.OK {
		return service.failMutation("start", ErrorRuntimeNotReady, healthResult)
	}
	health := healthResult.Value.(Health)
	modelViews, modelResult := service.modelViews()
	if !modelResult.OK {
		return service.failMutation("start", ErrorCatalogueUnavailable, modelResult)
	}
	snapshot := service.publishHealthy(managed, health, modelViews, nil)
	service.fireEvent("start-succeeded", snapshot.State)
	return core.Ok(snapshot)
}

// Load validates one opaque model ID, starts LEM if needed, and performs a
// confirmed reload with the trusted native path.
func (service *Service) Load(request LoadRequest) core.Result {
	ctx, release, result := service.beginMutation(StateLoading, "load")
	if !result.OK {
		return result
	}
	defer release()

	if service.options.Catalogue == nil {
		return service.failMutation(
			"load",
			ErrorCatalogueUnavailable,
			core.Fail(core.NewError("catalogue unavailable")),
		)
	}
	resolved := service.options.Catalogue.Resolve(request.ModelID)
	if !resolved.OK {
		return service.failMutation(
			"load",
			ErrorModelNotFound,
			mapCatalogueFailure("modelruntime.Service.Load", resolved),
		)
	}
	reference, ok := resolved.Value.(models.Reference)
	if !ok || reference.ID != request.ModelID || reference.NativePath == "" {
		return service.failMutation(
			"load",
			ErrorModelNotLoadable,
			core.Fail(core.NewError("invalid model reference")),
		)
	}

	managedResult := service.ensureRunning()
	if !managedResult.OK {
		return service.failMutation("load", ErrorRuntimeStartFailed, managedResult)
	}
	managed := managedResult.Value.(services.Snapshot)
	if health := service.waitHealth(ctx); !health.OK {
		return service.failMutation("load", ErrorRuntimeNotReady, health)
	}
	machineResult := service.withCredential(ctx, func(credential string) core.Result {
		return service.options.Client.Machine(ctx, credential)
	})
	if !machineResult.OK {
		return service.failMutation("load", ErrorAdminUnauthorised, machineResult)
	}
	machine, ok := machineResult.Value.(Machine)
	if !ok || machine.Hash == "" {
		return service.failMutation(
			"load",
			ErrorResponseInvalid,
			core.Fail(core.NewError("invalid machine response")),
		)
	}
	reload := service.withCredential(ctx, func(credential string) core.Result {
		return service.options.Client.Reload(ctx, credential, ReloadCommand{
			ConfirmMachine: machine.Hash,
			NativePath:     reference.NativePath,
		})
	})
	if !reload.OK {
		return service.failMutation("load", ErrorModelLoadFailed, reload)
	}
	readyResult := service.waitLoaded(ctx)
	if !readyResult.OK {
		return service.failMutation("load", ErrorModelLoadFailed, readyResult)
	}
	ready := readyResult.Value.(loadedRuntime)
	modelViews, modelResult := service.modelViews()
	if !modelResult.OK {
		return service.failMutation("load", ErrorCatalogueUnavailable, modelResult)
	}
	service.mu.Lock()
	service.activeModelID = request.ModelID
	service.mu.Unlock()
	snapshot := service.publishHealthy(
		managed,
		ready.health,
		modelViews,
		&ready.status,
	)
	service.fireEvent("load-succeeded", snapshot.State)
	return core.Ok(snapshot)
}

// Unload returns the current single-model LEM host to model-less by restarting
// the managed process without a --model argument.
func (service *Service) Unload() core.Result {
	ctx, release, result := service.beginMutation(StateStopping, "unload")
	if !result.OK {
		return result
	}
	defer release()
	if service.options.Lifecycle == nil {
		return service.failMutation(
			"unload",
			ErrorRuntimeUnavailable,
			core.Fail(core.NewError("lifecycle unavailable")),
		)
	}
	restarted := service.options.Lifecycle.Restart(inferenceServiceID)
	if !restarted.OK {
		return service.failMutation("unload", ErrorModelUnloadFailed, restarted)
	}
	managed, ok := restarted.Value.(services.Snapshot)
	if !ok {
		return service.failMutation(
			"unload",
			ErrorModelUnloadFailed,
			core.Fail(core.NewError("invalid lifecycle snapshot")),
		)
	}
	healthResult := service.waitHealth(ctx)
	if !healthResult.OK {
		return service.failMutation("unload", ErrorModelUnloadFailed, healthResult)
	}
	modelViews, modelResult := service.modelViews()
	if !modelResult.OK {
		return service.failMutation("unload", ErrorCatalogueUnavailable, modelResult)
	}
	service.mu.Lock()
	service.activeModelID = ""
	service.mu.Unlock()
	snapshot := service.publishHealthy(
		managed,
		healthResult.Value.(Health),
		modelViews,
		nil,
	)
	service.fireEvent("unload-succeeded", snapshot.State)
	return core.Ok(snapshot)
}

// Restart cycles LEM back to a healthy model-less process.
func (service *Service) Restart() core.Result {
	ctx, release, result := service.beginMutation(StateStarting, "restart")
	if !result.OK {
		return result
	}
	defer release()
	if service.options.Lifecycle == nil {
		return service.failMutation(
			"restart",
			ErrorRuntimeUnavailable,
			core.Fail(core.NewError("lifecycle unavailable")),
		)
	}
	restarted := service.options.Lifecycle.Restart(inferenceServiceID)
	if !restarted.OK {
		return service.failMutation("restart", ErrorRuntimeStartFailed, restarted)
	}
	managed, ok := restarted.Value.(services.Snapshot)
	if !ok {
		return service.failMutation(
			"restart",
			ErrorRuntimeStartFailed,
			core.Fail(core.NewError("invalid lifecycle snapshot")),
		)
	}
	healthResult := service.waitHealth(ctx)
	if !healthResult.OK {
		return service.failMutation("restart", ErrorRuntimeNotReady, healthResult)
	}
	modelViews, modelResult := service.modelViews()
	if !modelResult.OK {
		return service.failMutation("restart", ErrorCatalogueUnavailable, modelResult)
	}
	service.mu.Lock()
	service.activeModelID = ""
	service.mu.Unlock()
	snapshot := service.publishHealthy(
		managed,
		healthResult.Value.(Health),
		modelViews,
		nil,
	)
	service.fireEvent("restart-succeeded", snapshot.State)
	return core.Ok(snapshot)
}

// Stop cancels readiness work, then stops the managed sidecar.
func (service *Service) Stop() core.Result {
	if service == nil {
		return fail(
			ErrorRuntimeUnavailable,
			"modelruntime.Service.Stop",
			"The model runtime is unavailable.",
			nil,
		)
	}
	service.mu.Lock()
	if service.operationCancel != nil {
		service.operationCancel()
	}
	service.mu.Unlock()

	// Stop is the one mutation allowed to wait for a cancelled operation to
	// release the gate.
	service.operation <- struct{}{}
	defer func() { <-service.operation }()
	service.mu.Lock()
	if service.shuttingDown {
		service.mu.Unlock()
		return fail(
			ErrorRuntimeUnavailable,
			"modelruntime.Service.Stop",
			"The model runtime is shutting down.",
			nil,
		)
	}
	service.snapshot.State = StateStopping
	service.snapshot.RefreshedAt = timestamp(service.options.Now())
	service.mu.Unlock()
	service.fireEvent("stop-requested", StateStopping)

	if service.options.Lifecycle == nil {
		return service.failMutation(
			"stop",
			ErrorRuntimeUnavailable,
			core.Fail(core.NewError("lifecycle unavailable")),
		)
	}
	stopped := service.options.Lifecycle.Stop(inferenceServiceID)
	if !stopped.OK {
		return service.failMutation("stop", ErrorRuntimeStopFailed, stopped)
	}
	modelViews, modelResult := service.modelViews()
	if !modelResult.OK {
		return service.failMutation("stop", ErrorCatalogueUnavailable, modelResult)
	}
	service.stopSampler()
	service.mu.Lock()
	service.activeModelID = ""
	service.snapshot.State = StateStopped
	service.snapshot.Desired = false
	service.snapshot.ActiveModelID = ""
	service.snapshot.Models = cloneModels(modelViews)
	service.snapshot.Metrics = MetricsView{}
	service.snapshot.RefreshedAt = timestamp(service.options.Now())
	service.snapshot.Stale = false
	service.snapshot.LastError = nil
	snapshot := cloneSnapshot(service.snapshot)
	service.mu.Unlock()
	service.fireEvent("stop-succeeded", StateStopped)
	return core.Ok(snapshot)
}

func (service *Service) beginMutation(
	state State,
	reason string,
) (core.Context, func(), core.Result) {
	if service == nil {
		return nil, func() {}, fail(
			ErrorRuntimeUnavailable,
			"modelruntime.Service."+reason,
			"The model runtime is unavailable.",
			nil,
		)
	}
	select {
	case service.operation <- struct{}{}:
	default:
		return nil, func() {}, fail(
			ErrorOperationInProgress,
			"modelruntime.Service."+reason,
			"Another model-runtime operation is already running.",
			nil,
		)
	}
	service.mu.Lock()
	if service.shuttingDown {
		service.mu.Unlock()
		<-service.operation
		return nil, func() {}, fail(
			ErrorRuntimeUnavailable,
			"modelruntime.Service."+reason,
			"The model runtime is shutting down.",
			nil,
		)
	}
	ctx, cancel := core.WithTimeout(core.Background(), readinessTimeout)
	service.operationCancel = cancel
	service.operations.Add(1)
	service.snapshot.State = state
	service.snapshot.RefreshedAt = timestamp(service.options.Now())
	service.snapshot.LastError = nil
	service.mu.Unlock()
	service.fireEvent(reason+"-requested", state)
	release := func() {
		cancel()
		service.mu.Lock()
		service.operationCancel = nil
		service.mu.Unlock()
		<-service.operation
		service.operations.Done()
	}
	return ctx, release, core.Ok(nil)
}

func (service *Service) ensureRunning() core.Result {
	if service.options.Lifecycle == nil {
		return fail(
			ErrorRuntimeUnavailable,
			"modelruntime.Service.ensureRunning",
			"The managed-services runtime is unavailable.",
			nil,
		)
	}
	current := service.options.Lifecycle.Get(inferenceServiceID)
	if !current.OK {
		return fail(
			ErrorRuntimeUnavailable,
			"modelruntime.Service.ensureRunning",
			"The LEM managed service is unavailable.",
			current.Err(),
		)
	}
	managed, ok := current.Value.(services.Snapshot)
	if !ok {
		return fail(
			ErrorResponseInvalid,
			"modelruntime.Service.ensureRunning",
			"The managed-service response is invalid.",
			nil,
		)
	}
	if managed.State == services.StateRunning {
		return core.Ok(managed)
	}
	started := service.options.Lifecycle.Start(inferenceServiceID)
	if !started.OK {
		code := ErrorRuntimeStartFailed
		message := "The LEM runtime could not be started."
		if services.ErrorCodeOf(started) == services.ErrorProcessStartFailed &&
			core.Is(started.Err(), core.ErrNotExist) {
			code = ErrorBinaryMissing
			message = "The LEM runtime is not installed."
		}
		return fail(
			code,
			"modelruntime.Service.ensureRunning",
			message,
			started.Err(),
		)
	}
	managed, ok = started.Value.(services.Snapshot)
	if !ok || managed.State != services.StateRunning {
		return fail(
			ErrorRuntimeStartFailed,
			"modelruntime.Service.ensureRunning",
			"The LEM runtime did not start.",
			nil,
		)
	}
	return core.Ok(managed)
}

func (service *Service) waitHealth(ctx core.Context) core.Result {
	var last core.Result
	for attempt := 0; attempt < maxReadinessAttempts; attempt++ {
		if ctx.Err() != nil {
			return fail(
				ErrorRuntimeNotReady,
				"modelruntime.Service.waitHealth",
				"The LEM readiness check was cancelled.",
				ctx.Err(),
			)
		}
		last = service.options.Client.Health(ctx)
		if ctx.Err() != nil {
			return fail(
				ErrorRuntimeNotReady,
				"modelruntime.Service.waitHealth",
				"The LEM readiness check was cancelled.",
				ctx.Err(),
			)
		}
		if last.OK {
			health, ok := last.Value.(Health)
			if ok && health.Status == "ok" {
				return core.Ok(health)
			}
			last = fail(
				ErrorResponseInvalid,
				"modelruntime.Service.waitHealth",
				"The LEM health response is invalid.",
				nil,
			)
		}
		if attempt == maxReadinessAttempts-1 {
			break
		}
		select {
		case <-service.options.After(readinessDelay(attempt)):
		case <-ctx.Done():
			return fail(
				ErrorRuntimeNotReady,
				"modelruntime.Service.waitHealth",
				"The LEM readiness check was cancelled.",
				ctx.Err(),
			)
		}
	}
	return fail(
		ErrorRuntimeNotReady,
		"modelruntime.Service.waitHealth",
		"The LEM runtime did not become ready.",
		last.Err(),
	)
}

type loadedRuntime struct {
	health Health
	status Status
}

func (service *Service) waitLoaded(ctx core.Context) core.Result {
	var last core.Result
	for attempt := 0; attempt < maxReadinessAttempts; attempt++ {
		healthResult := service.options.Client.Health(ctx)
		if ctx.Err() != nil {
			return fail(
				ErrorModelLoadFailed,
				"modelruntime.Service.waitLoaded",
				"The model load was cancelled.",
				ctx.Err(),
			)
		}
		if healthResult.OK {
			health, ok := healthResult.Value.(Health)
			if ok && health.Status == "ok" && len(health.Models) > 0 {
				statusResult := service.adminStatus(ctx)
				if statusResult.OK {
					status, valid := statusResult.Value.(Status)
					if valid {
						return core.Ok(loadedRuntime{health: health, status: status})
					}
				}
				last = statusResult
			} else {
				last = fail(
					ErrorRuntimeNotReady,
					"modelruntime.Service.waitLoaded",
					"The selected model is not ready.",
					nil,
				)
			}
		} else {
			last = healthResult
		}
		if attempt == maxReadinessAttempts-1 {
			break
		}
		select {
		case <-service.options.After(readinessDelay(attempt)):
		case <-ctx.Done():
			return fail(
				ErrorModelLoadFailed,
				"modelruntime.Service.waitLoaded",
				"The model load was cancelled.",
				ctx.Err(),
			)
		}
	}
	return fail(
		ErrorModelLoadFailed,
		"modelruntime.Service.waitLoaded",
		"The selected model did not become ready.",
		last.Err(),
	)
}

func readinessDelay(attempt int) time.Duration {
	delay := 50 * time.Millisecond
	for index := 0; index < attempt && delay < 6400*time.Millisecond; index++ {
		delay *= 2
	}
	if delay > 6400*time.Millisecond {
		return 6400 * time.Millisecond
	}
	return delay
}

func (service *Service) adminStatus(ctx core.Context) core.Result {
	return service.withCredential(ctx, func(credential string) core.Result {
		return service.options.Client.Status(ctx, credential)
	})
}

func (service *Service) withCredential(
	ctx core.Context,
	call func(string) core.Result,
) core.Result {
	if service.options.Credentials == nil || call == nil {
		return fail(
			ErrorAdminUnauthorised,
			"modelruntime.Service.admin",
			"The LEM admin credential is unavailable.",
			nil,
		)
	}
	credentialResult := service.options.Credentials.Credential()
	if !credentialResult.OK {
		return fail(
			ErrorAdminUnauthorised,
			"modelruntime.Service.admin",
			"The LEM admin credential is unavailable.",
			credentialResult.Err(),
		)
	}
	result := call(credentialResult.String())
	if ClientErrorCodeOf(result) != ClientUnauthorised {
		return mapClientResult("modelruntime.Service.admin", result)
	}
	service.options.Credentials.Invalidate()
	if ctx.Err() != nil {
		return fail(
			ErrorAdminUnauthorised,
			"modelruntime.Service.admin",
			"LEM rejected the local admin credential.",
			ctx.Err(),
		)
	}
	refreshed := service.options.Credentials.Credential()
	if !refreshed.OK {
		return fail(
			ErrorAdminUnauthorised,
			"modelruntime.Service.admin",
			"The LEM admin credential is unavailable.",
			refreshed.Err(),
		)
	}
	return mapClientResult(
		"modelruntime.Service.admin",
		call(refreshed.String()),
	)
}

func mapClientResult(operation string, result core.Result) core.Result {
	if result.OK {
		return result
	}
	code := mapClientCode(ClientErrorCodeOf(result))
	if code == "" {
		code = ErrorRuntimeNotReady
	}
	message := "The LEM runtime request failed."
	if code == ErrorAdminUnauthorised {
		message = "LEM rejected the local admin credential."
	}
	return fail(code, operation, message, result.Err())
}

func (service *Service) publishHealthy(
	managed services.Snapshot,
	health Health,
	modelViews []ModelView,
	status *Status,
) Snapshot {
	state := StateModelLess
	activeID := ""
	if len(health.Models) > 0 && status != nil {
		state = StateReady
		service.mu.RLock()
		activeID = service.activeModelID
		service.mu.RUnlock()
	}
	metrics := service.metricsFromManaged(managed)
	now := service.options.Now()
	service.mu.Lock()
	if state == StateModelLess {
		service.activeModelID = ""
	}
	if state == StateReady {
		activeID = service.activeModelID
	}
	for index := range modelViews {
		modelViews[index].Loaded = modelViews[index].ID == activeID
		if modelViews[index].Loaded && status != nil {
			modelViews[index].Runtime = status.Runtime
			modelViews[index].ContextLength = status.Config.ContextLength
			if status.LoadedAtUnix > 0 {
				modelViews[index].LoadedAt = time.Unix(status.LoadedAtUnix, 0).
					UTC().
					Format(time.RFC3339Nano)
			}
		}
	}
	service.snapshot.State = state
	service.snapshot.Desired = managed.Desired
	service.snapshot.ActiveModelID = activeID
	service.snapshot.Models = cloneModels(modelViews)
	service.snapshot.Metrics = metrics
	service.snapshot.RefreshedAt = timestamp(now)
	service.snapshot.LastHealthyAt = timestamp(now)
	service.snapshot.Stale = false
	service.snapshot.LastError = nil
	service.appendSampleLocked(state, metrics, now)
	snapshot := cloneSnapshot(service.snapshot)
	service.mu.Unlock()
	service.ensureSampler()
	return snapshot
}

func (service *Service) metricsFromManaged(managed services.Snapshot) MetricsView {
	metrics := MetricsView{}
	if managed.StartedAt == "" {
		return metrics
	}
	startedAt, err := time.Parse(time.RFC3339Nano, managed.StartedAt)
	if err != nil {
		startedAt, err = time.Parse(time.RFC3339, managed.StartedAt)
	}
	if err == nil {
		uptime := int64(service.options.Now().Sub(startedAt).Seconds())
		if uptime < 0 {
			uptime = 0
		}
		metrics.UptimeSeconds = &uptime
	}
	return metrics
}

func (service *Service) failMutation(
	reason string,
	fallback ErrorCode,
	cause core.Result,
) core.Result {
	code := ErrorCodeOf(cause)
	if code == "" {
		code = mapClientCode(ClientErrorCodeOf(cause))
	}
	if code == "" {
		code = fallback
	}
	message := mutationFailureMessage(code)
	service.mu.Lock()
	service.snapshot.State = StateFailed
	service.snapshot.RefreshedAt = timestamp(service.options.Now())
	service.snapshot.Stale = service.snapshot.LastHealthyAt != ""
	service.snapshot.LastError = &FailureView{Code: code, Message: message}
	service.mu.Unlock()
	service.fireEvent(reason+"-failed", StateFailed)
	return fail(code, "modelruntime.Service."+reason, message, cause.Err())
}

func mutationFailureMessage(code ErrorCode) string {
	switch code {
	case ErrorModelNotFound:
		return "The selected model is unavailable."
	case ErrorModelNotLoadable:
		return "The selected model cannot be loaded locally."
	case ErrorAdminUnauthorised:
		return "LEM rejected the local admin credential."
	case ErrorOperationInProgress:
		return "Another model-runtime operation is already running."
	case ErrorRuntimeStopFailed:
		return "The LEM runtime could not be stopped."
	case ErrorModelUnloadFailed:
		return "The loaded model could not be unloaded."
	case ErrorModelLoadFailed:
		return "The selected model could not be loaded."
	case ErrorCatalogueUnavailable:
		return "The local model catalogue is unavailable."
	default:
		return "The LEM runtime operation could not be completed."
	}
}
