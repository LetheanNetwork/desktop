// SPDX-Licence-Identifier: EUPL-1.2

// Package modelruntime coordinates the optional LEM sidecar without exposing
// its execution or credential boundary to a renderer.
package modelruntime

import (
	"sync"
	"time"

	core "dappco.re/go"
	coreio "dappco.re/go/io"
	"dappco.re/lthn/desktop/pkg/models"
	"dappco.re/lthn/desktop/pkg/services"
)

const (
	defaultSampleInterval = 5 * time.Second
	maxReadinessAttempts  = 9
	readinessTimeout      = 30 * time.Second
)

// Lifecycle is the narrow managed-services boundary used by the runtime.
type Lifecycle interface {
	Get(string) core.Result
	Start(string) core.Result
	Stop(string) core.Result
	Restart(string) core.Result
}

// ModelCatalogue is the Medium-backed model lookup boundary.
type ModelCatalogue interface {
	List() core.Result
	Resolve(string) core.Result
}

// Options injects every external side effect for deterministic tests.
type Options struct {
	Lifecycle      Lifecycle
	Client         Client
	Credentials    CredentialProvider
	Catalogue      ModelCatalogue
	Now            func() time.Time
	After          func(time.Duration) <-chan time.Time
	SampleInterval time.Duration
	DisableSampler bool
}

// Service owns model-runtime state, readiness, operations, and bounded history.
type Service struct {
	options Options
	core    *core.Core

	mu              sync.RWMutex
	snapshot        Snapshot
	activeModelID   string
	shuttingDown    bool
	operation       chan struct{}
	operationCancel core.CancelFunc
	samplerCancel   core.CancelFunc
	operations      sync.WaitGroup
	work            sync.WaitGroup
}

// NewService constructs a runtime without starting a process or sampler.
func NewService(options Options) *Service {
	if options.Now == nil {
		options.Now = core.Now
	}
	if options.After == nil {
		options.After = time.After
	}
	if options.SampleInterval <= 0 {
		options.SampleInterval = defaultSampleInterval
	}
	now := timestamp(options.Now())
	return &Service{
		options:   options,
		operation: make(chan struct{}, 1),
		snapshot: Snapshot{
			State:       StateUnavailable,
			Models:      []ModelView{},
			History:     []SampleView{},
			RefreshedAt: now,
		},
	}
}

// Register resolves the named managed-services and LEM Medium, then registers
// the canonical model-runtime service.
//
// Usage example:
//
//	core.New(core.WithName("modelruntime", modelruntime.Register))
func Register(c *core.Core) core.Result {
	if c == nil {
		return fail(
			ErrorRuntimeUnavailable,
			"modelruntime.Register",
			"The model runtime is unavailable.",
			nil,
		)
	}
	lifecycle, ok := core.ServiceFor[*services.Service](c, "services")
	if !ok || lifecycle == nil {
		return fail(
			ErrorRuntimeUnavailable,
			"modelruntime.Register",
			"The managed-services runtime is unavailable.",
			nil,
		)
	}
	ioService, ok := core.ServiceFor[*coreio.Service](c, "lem-io")
	if !ok ||
		ioService == nil ||
		ioService.Medium == nil ||
		ioService.ServiceRuntime == nil ||
		ioService.Options().Root == "" {
		return fail(
			ErrorCatalogueUnavailable,
			"modelruntime.Register",
			"The LEM I/O Medium is unavailable.",
			nil,
		)
	}
	nativeRoot := core.PathJoin(ioService.Options().Root, "lem", "models")
	service := NewService(Options{
		Lifecycle:   lifecycle,
		Client:      NewClient(),
		Credentials: NewMediumCredentialProvider(ioService.Medium),
		Catalogue: models.NewCatalogue(
			ioService.Medium,
			"lem/models",
			nativeRoot,
		),
	})
	return service.Register(c)
}

// Register attaches an explicitly composed service to Core.
func (service *Service) Register(c *core.Core) core.Result {
	if service == nil || c == nil {
		return fail(
			ErrorRuntimeUnavailable,
			"modelruntime.Service.Register",
			"The model runtime is unavailable.",
			nil,
		)
	}
	service.mu.Lock()
	service.core = c
	service.mu.Unlock()
	return core.Ok(service)
}

// OnStartup is deliberately inert: boot must never start or probe LEM.
func (service *Service) OnStartup(core.Context) core.Result {
	return core.Ok(nil)
}

// OnShutdown cancels work and releases the cached credential. The managed
// services owner stops the child process.
func (service *Service) OnShutdown(core.Context) core.Result {
	if service == nil {
		return core.Ok(nil)
	}
	service.mu.Lock()
	if service.shuttingDown {
		service.mu.Unlock()
		return core.Ok(nil)
	}
	service.shuttingDown = true
	if service.operationCancel != nil {
		service.operationCancel()
		service.operationCancel = nil
	}
	if service.samplerCancel != nil {
		service.samplerCancel()
		service.samplerCancel = nil
	}
	service.mu.Unlock()
	service.operations.Wait()
	service.work.Wait()
	if service.options.Credentials != nil {
		service.options.Credentials.Clear()
	}
	return core.Ok(nil)
}

// Snapshot reconciles one immutable renderer-safe observation.
func (service *Service) Snapshot() core.Result {
	if service == nil || service.options.Lifecycle == nil {
		return fail(
			ErrorRuntimeUnavailable,
			"modelruntime.Service.Snapshot",
			"The model runtime is unavailable.",
			nil,
		)
	}
	service.mu.RLock()
	shuttingDown := service.shuttingDown
	service.mu.RUnlock()
	if shuttingDown {
		return fail(
			ErrorRuntimeUnavailable,
			"modelruntime.Service.Snapshot",
			"The model runtime is shutting down.",
			nil,
		)
	}
	lifecycleResult := service.options.Lifecycle.Get(inferenceServiceID)
	if !lifecycleResult.OK {
		return service.publishUnavailable(lifecycleResult)
	}
	managed, ok := lifecycleResult.Value.(services.Snapshot)
	if !ok {
		return service.publishUnavailable(core.Fail(core.NewError("invalid lifecycle snapshot")))
	}
	modelViews, modelResult := service.modelViews()
	if !modelResult.OK {
		return service.publishCatalogueFailure(managed, modelResult)
	}
	switch managed.State {
	case services.StateStopped, services.StateExited:
		service.stopSampler()
		return service.publishManagedState(StateStopped, managed.Desired, modelViews)
	case services.StateStarting:
		return service.publishManagedState(StateStarting, managed.Desired, modelViews)
	case services.StateStopping:
		return service.publishManagedState(StateStopping, managed.Desired, modelViews)
	case services.StateFailed:
		service.stopSampler()
		return service.publishFailure(
			StateFailed,
			managed.Desired,
			modelViews,
			ErrorRuntimeStartFailed,
			"The LEM runtime failed.",
		)
	case services.StateRunning:
		return service.refreshRunning(core.Background(), managed, modelViews, true)
	default:
		return service.publishUnavailable(core.Fail(core.NewError("unknown lifecycle state")))
	}
}

func (service *Service) modelViews() ([]ModelView, core.Result) {
	if service.options.Catalogue == nil {
		return nil, fail(
			ErrorCatalogueUnavailable,
			"modelruntime.Service.models",
			"The local model catalogue is unavailable.",
			nil,
		)
	}
	result := service.options.Catalogue.List()
	if !result.OK {
		return nil, mapCatalogueFailure("modelruntime.Service.models", result)
	}
	entries, ok := result.Value.([]models.CatalogueEntry)
	if !ok || len(entries) > 512 {
		return nil, fail(
			ErrorResponseInvalid,
			"modelruntime.Service.models",
			"The local model catalogue response is invalid.",
			nil,
		)
	}
	service.mu.RLock()
	activeID := service.activeModelID
	service.mu.RUnlock()
	views := make([]ModelView, len(entries))
	for index, entry := range entries {
		views[index] = ModelView{
			ID:          entry.ID,
			DisplayName: entry.DisplayName,
			Format:      entry.Format,
			Loadable:    entry.Loadable,
			Loaded:      entry.ID == activeID,
		}
	}
	return views, core.Ok(nil)
}

func (service *Service) refreshRunning(
	ctx core.Context,
	managed services.Snapshot,
	modelsView []ModelView,
	appendHistory bool,
) core.Result {
	if service.options.Client == nil {
		return service.publishFailure(
			StateFailed,
			managed.Desired,
			modelsView,
			ErrorRuntimeUnavailable,
			"The LEM runtime client is unavailable.",
		)
	}
	healthResult := service.options.Client.Health(ctx)
	if !healthResult.OK {
		return service.publishDegraded(modelsView, healthResult)
	}
	health, ok := healthResult.Value.(Health)
	if !ok || health.Status != "ok" {
		return service.publishDegraded(
			modelsView,
			fail(
				ErrorResponseInvalid,
				"modelruntime.Service.refresh",
				"The LEM health response is invalid.",
				nil,
			),
		)
	}
	state := StateModelLess
	status := Status{Runtime: health.Runtime}
	if len(health.Models) > 0 {
		state = StateReady
		statusResult := service.adminStatus(ctx)
		if !statusResult.OK {
			return service.publishDegraded(modelsView, statusResult)
		}
		status, ok = statusResult.Value.(Status)
		if !ok {
			return service.publishDegraded(
				modelsView,
				fail(
					ErrorResponseInvalid,
					"modelruntime.Service.refresh",
					"The LEM status response is invalid.",
					nil,
				),
			)
		}
	}
	metrics := service.metricsFromManaged(managed)
	now := service.options.Now()
	service.mu.Lock()
	activeID := service.activeModelID
	for index := range modelsView {
		modelsView[index].Loaded = modelsView[index].ID == activeID && state == StateReady
		if modelsView[index].Loaded {
			modelsView[index].Runtime = status.Runtime
			modelsView[index].ContextLength = status.Config.ContextLength
			if status.LoadedAtUnix > 0 {
				modelsView[index].LoadedAt = time.Unix(status.LoadedAtUnix, 0).
					UTC().
					Format(time.RFC3339Nano)
			}
		}
	}
	service.snapshot.State = state
	service.snapshot.Desired = managed.Desired
	service.snapshot.ActiveModelID = activeID
	if state == StateModelLess {
		service.snapshot.ActiveModelID = ""
		service.activeModelID = ""
	}
	service.snapshot.Models = cloneModels(modelsView)
	service.snapshot.Metrics = metrics
	service.snapshot.RefreshedAt = timestamp(now)
	service.snapshot.LastHealthyAt = timestamp(now)
	service.snapshot.Stale = false
	service.snapshot.LastError = nil
	if appendHistory {
		service.appendSampleLocked(state, metrics, now)
	}
	snapshot := cloneSnapshot(service.snapshot)
	service.mu.Unlock()
	service.ensureSampler()
	return core.Ok(snapshot)
}

func (service *Service) publishManagedState(
	state State,
	desired bool,
	modelsView []ModelView,
) core.Result {
	service.mu.Lock()
	service.snapshot.State = state
	service.snapshot.Desired = desired
	if state == StateStopped {
		service.activeModelID = ""
		service.snapshot.ActiveModelID = ""
		service.snapshot.Metrics = MetricsView{}
	}
	service.snapshot.Models = cloneModels(modelsView)
	service.snapshot.RefreshedAt = timestamp(service.options.Now())
	service.snapshot.Stale = false
	service.snapshot.LastError = nil
	snapshot := cloneSnapshot(service.snapshot)
	service.mu.Unlock()
	return core.Ok(snapshot)
}

func (service *Service) publishFailure(
	state State,
	desired bool,
	modelsView []ModelView,
	code ErrorCode,
	message string,
) core.Result {
	service.mu.Lock()
	service.snapshot.State = state
	service.snapshot.Desired = desired
	service.snapshot.Models = cloneModels(modelsView)
	service.snapshot.RefreshedAt = timestamp(service.options.Now())
	service.snapshot.Stale = false
	service.snapshot.LastError = &FailureView{Code: code, Message: message}
	snapshot := cloneSnapshot(service.snapshot)
	service.mu.Unlock()
	return core.Ok(snapshot)
}

func (service *Service) publishDegraded(
	modelsView []ModelView,
	result core.Result,
) core.Result {
	code := ErrorCodeOf(result)
	if code == "" {
		code = mapClientCode(ClientErrorCodeOf(result))
	}
	if code == "" {
		code = ErrorRuntimeNotReady
	}
	service.mu.Lock()
	service.snapshot.State = StateDegraded
	if service.snapshot.LastHealthyAt == "" {
		service.snapshot.Models = cloneModels(modelsView)
	}
	service.snapshot.RefreshedAt = timestamp(service.options.Now())
	service.snapshot.Stale = true
	service.snapshot.LastError = &FailureView{
		Code:    code,
		Message: "The LEM runtime is temporarily unavailable.",
	}
	snapshot := cloneSnapshot(service.snapshot)
	service.mu.Unlock()
	return core.Ok(snapshot)
}

func (service *Service) publishUnavailable(cause core.Result) core.Result {
	service.mu.Lock()
	service.snapshot.State = StateUnavailable
	service.snapshot.Desired = false
	service.snapshot.RefreshedAt = timestamp(service.options.Now())
	service.snapshot.Stale = service.snapshot.LastHealthyAt != ""
	service.snapshot.LastError = &FailureView{
		Code:    ErrorRuntimeUnavailable,
		Message: "The model runtime is unavailable.",
	}
	service.mu.Unlock()
	return fail(
		ErrorRuntimeUnavailable,
		"modelruntime.Service.Snapshot",
		"The model runtime is unavailable.",
		cause.Err(),
	)
}

func (service *Service) publishCatalogueFailure(
	managed services.Snapshot,
	cause core.Result,
) core.Result {
	service.mu.Lock()
	service.snapshot.State = managedState(managed.State)
	service.snapshot.Desired = managed.Desired
	service.snapshot.RefreshedAt = timestamp(service.options.Now())
	service.snapshot.Stale = service.snapshot.LastHealthyAt != ""
	service.snapshot.LastError = &FailureView{
		Code:    ErrorCatalogueUnavailable,
		Message: "The local model catalogue is unavailable.",
	}
	service.mu.Unlock()
	return mapCatalogueFailure("modelruntime.Service.Snapshot", cause)
}

func managedState(state services.State) State {
	switch state {
	case services.StateStopped, services.StateExited:
		return StateStopped
	case services.StateStarting:
		return StateStarting
	case services.StateRunning:
		return StateModelLess
	case services.StateStopping:
		return StateStopping
	default:
		return StateFailed
	}
}

func (service *Service) appendSampleLocked(
	state State,
	metrics MetricsView,
	at time.Time,
) {
	service.snapshot.History = append(service.snapshot.History, SampleView{
		State:                 state,
		At:                    timestamp(at),
		PromptTokensPerSecond: cloneFloat(metrics.PromptTokensPerSecond),
		DecodeTokensPerSecond: cloneFloat(metrics.DecodeTokensPerSecond),
		ActiveMemoryBytes:     cloneUint(metrics.ActiveMemoryBytes),
		PeakMemoryBytes:       cloneUint(metrics.PeakMemoryBytes),
		KVCacheBytes:          cloneUint(metrics.KVCacheBytes),
	})
	if excess := len(service.snapshot.History) - maxHistorySamples; excess > 0 {
		service.snapshot.History = append(
			[]SampleView(nil),
			service.snapshot.History[excess:]...,
		)
	}
}

func cloneSnapshot(snapshot Snapshot) Snapshot {
	clone := snapshot
	clone.Models = cloneModels(snapshot.Models)
	clone.History = cloneSamples(snapshot.History)
	if snapshot.LastError != nil {
		lastError := *snapshot.LastError
		clone.LastError = &lastError
	}
	clone.Metrics = cloneMetrics(snapshot.Metrics)
	return clone
}

func cloneSamples(values []SampleView) []SampleView {
	clones := make([]SampleView, len(values))
	for index, value := range values {
		clones[index] = value
		clones[index].PromptTokensPerSecond = cloneFloat(value.PromptTokensPerSecond)
		clones[index].DecodeTokensPerSecond = cloneFloat(value.DecodeTokensPerSecond)
		clones[index].ActiveMemoryBytes = cloneUint(value.ActiveMemoryBytes)
		clones[index].PeakMemoryBytes = cloneUint(value.PeakMemoryBytes)
		clones[index].KVCacheBytes = cloneUint(value.KVCacheBytes)
	}
	return clones
}

func cloneModels(values []ModelView) []ModelView {
	return append([]ModelView(nil), values...)
}

func cloneMetrics(value MetricsView) MetricsView {
	return MetricsView{
		PromptTokensPerSecond: cloneFloat(value.PromptTokensPerSecond),
		DecodeTokensPerSecond: cloneFloat(value.DecodeTokensPerSecond),
		ActiveMemoryBytes:     cloneUint(value.ActiveMemoryBytes),
		PeakMemoryBytes:       cloneUint(value.PeakMemoryBytes),
		KVCacheBytes:          cloneUint(value.KVCacheBytes),
		UptimeSeconds:         cloneInt(value.UptimeSeconds),
	}
}

func cloneFloat(value *float64) *float64 {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

func cloneUint(value *uint64) *uint64 {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

func cloneInt(value *int64) *int64 {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

func mapCatalogueFailure(operation string, result core.Result) core.Result {
	switch models.CatalogueErrorCodeOf(result) {
	case models.CatalogueModelNotFound:
		return fail(ErrorModelNotFound, operation, "The selected model is unavailable.", result.Err())
	case models.CatalogueModelNotLoadable:
		return fail(
			ErrorModelNotLoadable,
			operation,
			"The selected model cannot be loaded locally.",
			result.Err(),
		)
	default:
		return fail(
			ErrorCatalogueUnavailable,
			operation,
			"The local model catalogue is unavailable.",
			result.Err(),
		)
	}
}

func mapClientCode(code ClientErrorCode) ErrorCode {
	switch code {
	case ClientUnauthorised:
		return ErrorAdminUnauthorised
	case ClientResponseInvalid:
		return ErrorResponseInvalid
	case ClientResponseTooLarge:
		return ErrorResponseTooLarge
	case ClientRequestTimeout:
		return ErrorRequestTimeout
	case ClientUnavailable:
		return ErrorRuntimeNotReady
	default:
		return ""
	}
}
