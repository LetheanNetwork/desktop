// SPDX-Licence-Identifier: EUPL-1.2

package modelruntime

import (
	"sync"
	"time"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/models"
	"dappco.re/lthn/desktop/pkg/services"
)

type fakeLifecycle struct {
	mu             sync.Mutex
	snapshot       services.Snapshot
	calls          []string
	startResult    core.Result
	hasStartResult bool
	// getResult/hasGetResult override Get's normal Ok(snapshot) reply —
	// used to provoke Snapshot()'s publishUnavailable branches (a
	// failed lifecycle read, or one that returns an unexpected Value
	// type) that the snapshot-state-driven scenarios can't reach.
	getResult    core.Result
	hasGetResult bool
	// restartResult/hasRestartResult override Restart's normal success
	// reply — used to provoke Unload/Restart's "lifecycle restart
	// failed" branches.
	restartResult    core.Result
	hasRestartResult bool
}

func (lifecycle *fakeLifecycle) Get(id string) core.Result {
	lifecycle.mu.Lock()
	defer lifecycle.mu.Unlock()
	lifecycle.calls = append(lifecycle.calls, "lifecycle.get")
	if id != inferenceServiceID {
		return core.Fail(core.NewError("unknown service"))
	}
	if lifecycle.hasGetResult {
		return lifecycle.getResult
	}
	return core.Ok(lifecycle.snapshot)
}

func (lifecycle *fakeLifecycle) Start(id string) core.Result {
	lifecycle.mu.Lock()
	defer lifecycle.mu.Unlock()
	lifecycle.calls = append(lifecycle.calls, "lifecycle.start")
	if id != inferenceServiceID {
		return core.Fail(core.NewError("unknown service"))
	}
	if lifecycle.hasStartResult {
		return lifecycle.startResult
	}
	lifecycle.snapshot.State = services.StateRunning
	lifecycle.snapshot.Desired = true
	lifecycle.snapshot.StartedAt = "2026-07-27T12:00:00Z"
	return core.Ok(lifecycle.snapshot)
}

func (lifecycle *fakeLifecycle) Stop(id string) core.Result {
	lifecycle.mu.Lock()
	defer lifecycle.mu.Unlock()
	lifecycle.calls = append(lifecycle.calls, "lifecycle.stop")
	if id != inferenceServiceID {
		return core.Fail(core.NewError("unknown service"))
	}
	lifecycle.snapshot.State = services.StateStopped
	lifecycle.snapshot.Desired = false
	lifecycle.snapshot.StartedAt = ""
	return core.Ok(lifecycle.snapshot)
}

func (lifecycle *fakeLifecycle) Restart(id string) core.Result {
	lifecycle.mu.Lock()
	defer lifecycle.mu.Unlock()
	lifecycle.calls = append(lifecycle.calls, "lifecycle.restart")
	if id != inferenceServiceID {
		return core.Fail(core.NewError("unknown service"))
	}
	if lifecycle.hasRestartResult {
		return lifecycle.restartResult
	}
	lifecycle.snapshot.State = services.StateRunning
	lifecycle.snapshot.Desired = true
	lifecycle.snapshot.StartedAt = "2026-07-27T12:00:00Z"
	return core.Ok(lifecycle.snapshot)
}

func (lifecycle *fakeLifecycle) callSnapshot() []string {
	lifecycle.mu.Lock()
	defer lifecycle.mu.Unlock()
	return append([]string(nil), lifecycle.calls...)
}

type fakeRuntimeClient struct {
	mu            sync.Mutex
	calls         []string
	healthResults []core.Result
	statusResults []core.Result
	machineResult core.Result
	reloadResult  core.Result
	reloadCommand ReloadCommand
	healthBlock   chan struct{}
	ignoreCancel  bool
	reloaded      bool
}

func (client *fakeRuntimeClient) Health(ctx core.Context) core.Result {
	client.mu.Lock()
	client.calls = append(client.calls, "client.health")
	block := client.healthBlock
	var result core.Result
	if len(client.healthResults) > 0 {
		result = client.healthResults[0]
		client.healthResults = client.healthResults[1:]
	} else {
		models := []string{}
		if client.reloaded {
			models = []string{"gemma-4-e2b"}
		}
		result = core.Ok(Health{
			Status:  "ok",
			Runtime: "go-inference",
			Models:  models,
			Time:    1785146400,
		})
	}
	client.mu.Unlock()
	if block != nil {
		if client.ignoreCancel {
			<-block
		} else {
			select {
			case <-block:
			case <-ctx.Done():
				return core.Fail(ctx.Err())
			}
		}
	}
	return result
}

func (client *fakeRuntimeClient) Status(_ core.Context, _ string) core.Result {
	client.mu.Lock()
	defer client.mu.Unlock()
	client.calls = append(client.calls, "client.status")
	if len(client.statusResults) > 0 {
		result := client.statusResults[0]
		client.statusResults = client.statusResults[1:]
		return result
	}
	return core.Ok(Status{
		Runtime:      "metal",
		LoadedAtUnix: 1785146400,
	})
}

func (client *fakeRuntimeClient) Machine(_ core.Context, _ string) core.Result {
	client.mu.Lock()
	defer client.mu.Unlock()
	client.calls = append(client.calls, "client.machine")
	if !client.machineResult.OK && client.machineResult.Err() != nil {
		return client.machineResult
	}
	return core.Ok(Machine{Hash: "lem-machine", Runtime: "go-inference"})
}

func (client *fakeRuntimeClient) Reload(
	_ core.Context,
	_ string,
	command ReloadCommand,
) core.Result {
	client.mu.Lock()
	defer client.mu.Unlock()
	client.calls = append(client.calls, "client.reload")
	client.reloadCommand = command
	if !client.reloadResult.OK && client.reloadResult.Err() != nil {
		return client.reloadResult
	}
	client.reloaded = true
	return core.Ok(nil)
}

func (client *fakeRuntimeClient) callSnapshot() []string {
	client.mu.Lock()
	defer client.mu.Unlock()
	return append([]string(nil), client.calls...)
}

type fakeCredentialProvider struct {
	mu          sync.Mutex
	token       string
	reads       int
	invalidates int
	clears      int
	// results is an optional FIFO override queue — when non-empty,
	// Credential() pops and returns the head instead of Ok(token).
	// Used to provoke withCredential's "refreshed credential also
	// unavailable" branch.
	results []core.Result
}

func (provider *fakeCredentialProvider) Credential() core.Result {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	provider.reads++
	if len(provider.results) > 0 {
		next := provider.results[0]
		provider.results = provider.results[1:]
		return next
	}
	return core.Ok(provider.token)
}

func (provider *fakeCredentialProvider) Invalidate() {
	provider.mu.Lock()
	provider.invalidates++
	provider.mu.Unlock()
}

func (provider *fakeCredentialProvider) Clear() {
	provider.mu.Lock()
	provider.clears++
	provider.token = ""
	provider.mu.Unlock()
}

type fakeModelCatalogue struct {
	mu          sync.Mutex
	entry       models.CatalogueEntry
	reference   models.Reference
	resolveCode models.CatalogueErrorCode
	listCalls   int
	resolves    int
}

func (catalogue *fakeModelCatalogue) List() core.Result {
	catalogue.mu.Lock()
	defer catalogue.mu.Unlock()
	catalogue.listCalls++
	return core.Ok([]models.CatalogueEntry{catalogue.entry})
}

func (catalogue *fakeModelCatalogue) Resolve(id string) core.Result {
	catalogue.mu.Lock()
	defer catalogue.mu.Unlock()
	catalogue.resolves++
	if catalogue.resolveCode != "" {
		return core.Fail(&models.CatalogueFailure{
			Code:    catalogue.resolveCode,
			Message: "unavailable",
		})
	}
	if id != catalogue.reference.ID {
		return core.Fail(&models.CatalogueFailure{
			Code:    models.CatalogueModelNotFound,
			Message: "not found",
		})
	}
	return core.Ok(catalogue.reference)
}

type runtimeFixture struct {
	runtime     *Service
	lifecycle   *fakeLifecycle
	client      *fakeRuntimeClient
	credentials *fakeCredentialProvider
	catalogue   *fakeModelCatalogue
	core        *core.Core
	now         time.Time
}

func newRuntimeFixture(t *core.T) *runtimeFixture {
	t.Helper()
	now := time.Date(2026, 7, 27, 13, 0, 0, 0, time.UTC)
	lifecycle := &fakeLifecycle{
		snapshot: services.Snapshot{
			Definition: services.DefinitionView{ID: inferenceServiceID},
			State:      services.StateStopped,
		},
	}
	client := &fakeRuntimeClient{
		machineResult: core.Ok(nil),
		reloadResult:  core.Ok(nil),
	}
	credentials := &fakeCredentialProvider{
		token: "lthn-mlx_abcdefghijklmnopqrstuvwx",
	}
	catalogue := &fakeModelCatalogue{
		entry: models.CatalogueEntry{
			ID:          "model-1111111111111111",
			DisplayName: "gemma-4-e2b",
			Format:      "snapshot",
			Loadable:    true,
		},
		reference: models.Reference{
			ID:           "model-1111111111111111",
			DisplayName:  "gemma-4-e2b",
			RelativePath: "lem/models/gemma-4-e2b",
			NativePath:   "/trusted/Lethean/lem/models/gemma-4-e2b",
			Format:       "snapshot",
		},
	}
	coreApp := core.New()
	runtime := NewService(Options{
		Lifecycle:   lifecycle,
		Client:      client,
		Credentials: credentials,
		Catalogue:   catalogue,
		Now:         func() time.Time { return now },
		After: func(time.Duration) <-chan time.Time {
			ready := make(chan time.Time, 1)
			ready <- now
			return ready
		},
		SampleInterval: time.Hour,
		DisableSampler: true,
	})
	registered := runtime.Register(coreApp)
	core.RequireTrue(t, registered.OK, registered.Error())
	t.Cleanup(func() { _ = runtime.OnShutdown(core.Background()) })
	return &runtimeFixture{
		runtime:     runtime,
		lifecycle:   lifecycle,
		client:      client,
		credentials: credentials,
		catalogue:   catalogue,
		core:        coreApp,
		now:         now,
	}
}

func TestService_Snapshot_GoodStoppedRuntimeNeverStartsOrCallsLEM(t *core.T) {
	fixture := newRuntimeFixture(t)

	result := fixture.runtime.Snapshot()

	core.RequireTrue(t, result.OK, result.Error())
	snapshot := result.Value.(Snapshot)
	core.AssertEqual(t, StateStopped, snapshot.State)
	core.AssertFalse(t, snapshot.Desired)
	core.RequireTrue(t, len(snapshot.Models) == 1)
	core.AssertEqual(t, "gemma-4-e2b", snapshot.Models[0].DisplayName)
	core.AssertEqual(t, []string{"lifecycle.get"}, fixture.lifecycle.callSnapshot())
	core.AssertLen(t, fixture.client.callSnapshot(), 0)
}

func TestService_Start_GoodStartsThenWaitsForModelLessHealth(t *core.T) {
	fixture := newRuntimeFixture(t)

	result := fixture.runtime.Start()

	core.RequireTrue(t, result.OK, result.Error())
	snapshot := result.Value.(Snapshot)
	core.AssertEqual(t, StateModelLess, snapshot.State)
	core.AssertTrue(t, snapshot.Desired)
	core.AssertEqual(t, []string{
		"lifecycle.get",
		"lifecycle.start",
	}, fixture.lifecycle.callSnapshot())
	core.AssertEqual(t, []string{"client.health"}, fixture.client.callSnapshot())
}

func TestService_Start_BadMapsMissingSidecarWithoutLeakingNativePath(t *core.T) {
	fixture := newRuntimeFixture(t)
	fixture.lifecycle.startResult = core.Fail(&services.Failure{
		Code:      services.ErrorProcessStartFailed,
		Operation: "services.Service.Start",
		Message:   "could not start /private/secret/lem",
		Cause:     core.ErrNotExist,
	})
	fixture.lifecycle.hasStartResult = true

	result := fixture.runtime.Start()

	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ErrorBinaryMissing, ErrorCodeOf(result))
	core.AssertNotContains(t, result.Error(), "/private/secret/lem")
	core.AssertLen(t, fixture.client.callSnapshot(), 0)
}

func TestService_Load_GoodResolvesBeforeStartAndReturnsReadySnapshot(t *core.T) {
	fixture := newRuntimeFixture(t)
	fixture.client.healthResults = []core.Result{
		core.Ok(Health{Status: "ok", Runtime: "go-inference"}),
		core.Ok(Health{
			Status:  "ok",
			Runtime: "go-inference",
			Models:  []string{"gemma-4-e2b"},
		}),
	}
	fixture.client.statusResults = []core.Result{
		core.Ok(Status{
			ModelPath:    "/trusted/Lethean/lem/models/gemma-4-e2b",
			Runtime:      "metal",
			LoadedAtUnix: 1785146400,
		}),
	}

	result := fixture.runtime.Load(LoadRequest{ModelID: "model-1111111111111111"})

	core.RequireTrue(t, result.OK, result.Error())
	snapshot := result.Value.(Snapshot)
	core.AssertEqual(t, StateReady, snapshot.State)
	core.AssertEqual(t, "model-1111111111111111", snapshot.ActiveModelID)
	core.AssertTrue(t, snapshot.Models[0].Loaded)
	core.AssertEqual(t, []string{
		"lifecycle.get",
		"lifecycle.start",
	}, fixture.lifecycle.callSnapshot())
	core.AssertEqual(t, []string{
		"client.health",
		"client.machine",
		"client.reload",
		"client.health",
		"client.status",
	}, fixture.client.callSnapshot())
	core.AssertEqual(t, ReloadCommand{
		ConfirmMachine: "lem-machine",
		NativePath:     "/trusted/Lethean/lem/models/gemma-4-e2b",
	}, fixture.client.reloadCommand)
}

func TestService_Load_BadUnknownModelFailsBeforeProcessStart(t *core.T) {
	fixture := newRuntimeFixture(t)

	result := fixture.runtime.Load(LoadRequest{ModelID: "model-0000000000000000"})

	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ErrorModelNotFound, ErrorCodeOf(result))
	core.AssertLen(t, fixture.lifecycle.callSnapshot(), 0)
	core.AssertLen(t, fixture.client.callSnapshot(), 0)
}

func TestService_Load_BadNonLoadableModelFailsBeforeProcessStart(t *core.T) {
	fixture := newRuntimeFixture(t)
	fixture.catalogue.resolveCode = models.CatalogueModelNotLoadable

	result := fixture.runtime.Load(LoadRequest{ModelID: "model-1111111111111111"})

	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ErrorModelNotLoadable, ErrorCodeOf(result))
	core.AssertLen(t, fixture.lifecycle.callSnapshot(), 0)
	core.AssertLen(t, fixture.client.callSnapshot(), 0)
}

func TestService_Unload_GoodRestartsBackToModelLessWithoutReload(t *core.T) {
	fixture := newRuntimeFixture(t)
	fixture.lifecycle.snapshot.State = services.StateRunning
	fixture.lifecycle.snapshot.Desired = true

	result := fixture.runtime.Unload()

	core.RequireTrue(t, result.OK, result.Error())
	snapshot := result.Value.(Snapshot)
	core.AssertEqual(t, StateModelLess, snapshot.State)
	core.AssertEqual(t, []string{"lifecycle.restart"}, fixture.lifecycle.callSnapshot())
	core.AssertEqual(t, []string{"client.health"}, fixture.client.callSnapshot())
}

func TestService_Mutation_BadRejectsConcurrentOperation(t *core.T) {
	fixture := newRuntimeFixture(t)
	fixture.client.healthBlock = make(chan struct{})
	started := make(chan core.Result, 1)
	go func() {
		started <- fixture.runtime.Start()
	}()
	for {
		if len(fixture.client.callSnapshot()) > 0 {
			break
		}
		time.Sleep(time.Millisecond)
	}

	concurrent := fixture.runtime.Restart()

	core.AssertFalse(t, concurrent.OK)
	core.AssertEqual(t, ErrorOperationInProgress, ErrorCodeOf(concurrent))
	close(fixture.client.healthBlock)
	core.RequireTrue(t, (<-started).OK)
}

func TestService_Snapshot_UglyRetainsLastGoodWhenHealthDegrades(t *core.T) {
	fixture := newRuntimeFixture(t)
	fixture.lifecycle.snapshot.State = services.StateRunning
	fixture.lifecycle.snapshot.Desired = true
	fixture.client.healthResults = []core.Result{
		core.Ok(Health{
			Status:  "ok",
			Runtime: "go-inference",
			Models:  []string{"gemma-4-e2b"},
		}),
		core.Fail(&ClientFailure{
			Code:    ClientUnavailable,
			Message: "down",
		}),
	}
	fixture.client.statusResults = []core.Result{
		core.Ok(Status{Runtime: "metal", LoadedAtUnix: 1785146400}),
	}
	fixture.runtime.mu.Lock()
	fixture.runtime.activeModelID = "model-1111111111111111"
	fixture.runtime.mu.Unlock()
	first := fixture.runtime.Snapshot()
	core.RequireTrue(t, first.OK, first.Error())
	core.AssertEqual(t, StateReady, first.Value.(Snapshot).State)

	second := fixture.runtime.Snapshot()

	core.RequireTrue(t, second.OK, second.Error())
	degraded := second.Value.(Snapshot)
	core.AssertEqual(t, StateDegraded, degraded.State)
	core.AssertTrue(t, degraded.Stale)
	core.AssertEqual(t, "model-1111111111111111", degraded.ActiveModelID)
	core.RequireTrue(t, len(degraded.Models) == 1)
	core.AssertTrue(t, degraded.Models[0].Loaded)
	core.AssertEqual(t, "metal", degraded.Models[0].Runtime)
	core.RequireTrue(t, degraded.LastError != nil)
	core.AssertEqual(t, ErrorRuntimeNotReady, degraded.LastError.Code)
}

func TestService_Snapshot_GoodRetriesRotatedCredentialExactlyOnce(t *core.T) {
	fixture := newRuntimeFixture(t)
	fixture.lifecycle.snapshot.State = services.StateRunning
	fixture.lifecycle.snapshot.Desired = true
	fixture.client.healthResults = []core.Result{
		core.Ok(Health{
			Status:  "ok",
			Runtime: "go-inference",
			Models:  []string{"gemma-4-e2b"},
		}),
	}
	fixture.client.statusResults = []core.Result{
		core.Fail(&ClientFailure{
			Code:    ClientUnauthorised,
			Message: "unauthorised",
		}),
		core.Ok(Status{Runtime: "metal", LoadedAtUnix: 1785146400}),
	}
	fixture.runtime.mu.Lock()
	fixture.runtime.activeModelID = "model-1111111111111111"
	fixture.runtime.mu.Unlock()

	result := fixture.runtime.Snapshot()

	core.RequireTrue(t, result.OK, result.Error())
	core.AssertEqual(t, StateReady, result.Value.(Snapshot).State)
	core.AssertEqual(t, 2, fixture.credentials.reads)
	core.AssertEqual(t, 1, fixture.credentials.invalidates)
	core.AssertEqual(t, []string{
		"client.health",
		"client.status",
		"client.status",
	}, fixture.client.callSnapshot())
}

func TestService_Restart_GoodNeverReloadsAModel(t *core.T) {
	fixture := newRuntimeFixture(t)
	fixture.lifecycle.snapshot.State = services.StateRunning
	fixture.lifecycle.snapshot.Desired = true

	result := fixture.runtime.Restart()

	core.RequireTrue(t, result.OK, result.Error())
	core.AssertEqual(t, StateModelLess, result.Value.(Snapshot).State)
	core.AssertEqual(t, []string{"lifecycle.restart"}, fixture.lifecycle.callSnapshot())
	core.AssertEqual(t, []string{"client.health"}, fixture.client.callSnapshot())
}

func TestService_Stop_GoodCancelsLoadPollingBeforeStoppingLifecycle(t *core.T) {
	fixture := newRuntimeFixture(t)
	fixture.client.healthBlock = make(chan struct{})
	loadDone := make(chan core.Result, 1)
	go func() {
		loadDone <- fixture.runtime.Load(
			LoadRequest{ModelID: "model-1111111111111111"},
		)
	}()
	waitForClientCalls(t, fixture.client, 1)

	stopped := fixture.runtime.Stop()
	loadResult := <-loadDone

	core.RequireTrue(t, stopped.OK, stopped.Error())
	core.AssertEqual(t, StateStopped, stopped.Value.(Snapshot).State)
	core.AssertFalse(t, loadResult.OK)
	core.AssertEqual(t, ErrorRuntimeNotReady, ErrorCodeOf(loadResult))
	core.AssertEqual(t, []string{
		"lifecycle.get",
		"lifecycle.start",
		"lifecycle.stop",
	}, fixture.lifecycle.callSnapshot())
	core.AssertEqual(t, []string{"client.health"}, fixture.client.callSnapshot())
}

func TestService_OnShutdown_UglyWaitsForInFlightOperationAndClearsCredential(t *core.T) {
	fixture := newRuntimeFixture(t)
	fixture.client.healthBlock = make(chan struct{})
	fixture.client.ignoreCancel = true
	loadDone := make(chan core.Result, 1)
	go func() {
		loadDone <- fixture.runtime.Load(
			LoadRequest{ModelID: "model-1111111111111111"},
		)
	}()
	waitForClientCalls(t, fixture.client, 1)

	shutdownDone := make(chan core.Result, 1)
	go func() {
		shutdownDone <- fixture.runtime.OnShutdown(core.Background())
	}()
	select {
	case <-shutdownDone:
		t.Fatal("shutdown returned before the in-flight operation ended")
	case <-time.After(20 * time.Millisecond):
	}

	close(fixture.client.healthBlock)
	loadResult := <-loadDone
	shutdownResult := <-shutdownDone

	core.AssertFalse(t, loadResult.OK)
	core.RequireTrue(t, shutdownResult.OK, shutdownResult.Error())
	core.AssertEqual(t, 1, fixture.credentials.clears)
	core.AssertEqual(t, []string{"client.health"}, fixture.client.callSnapshot())
}

func TestService_Snapshot_UglyBoundsHistoryToLatestHour(t *core.T) {
	fixture := newRuntimeFixture(t)
	fixture.lifecycle.snapshot.State = services.StateRunning
	fixture.lifecycle.snapshot.Desired = true
	for index := 0; index < maxHistorySamples+5; index++ {
		result := fixture.runtime.Snapshot()
		core.RequireTrue(t, result.OK, result.Error())
	}

	result := fixture.runtime.Snapshot()
	history := result.Value.(Snapshot).History
	core.AssertEqual(t, maxHistorySamples, len(history))
}

func TestService_Event_GoodContainsInvalidationMetadataOnly(t *core.T) {
	fixture := newRuntimeFixture(t)
	events := []Event{}
	Subscribe(fixture.core, func(_ *core.Core, event Event) {
		events = append(events, event)
	})

	result := fixture.runtime.Load(LoadRequest{ModelID: "model-1111111111111111"})

	core.RequireTrue(t, result.OK, result.Error())
	core.RequireTrue(t, len(events) >= 2)
	encoded := core.JSONMarshal(events)
	core.RequireTrue(t, encoded.OK, encoded.Error())
	payload := string(encoded.Bytes())
	for _, forbidden := range []string{
		"path", "command", "arguments", "environment", "url", "token",
		"credential", "/trusted/", "lthn-mlx_",
	} {
		core.AssertNotContains(t, payload, forbidden)
	}
}

func TestService_Snapshot_UglyReturnsIndependentHistoryAndMetricPointers(t *core.T) {
	prompt := 12.5
	fixture := newRuntimeFixture(t)
	fixture.runtime.mu.Lock()
	fixture.runtime.snapshot.Metrics.PromptTokensPerSecond = &prompt
	fixture.runtime.snapshot.History = []SampleView{{
		State:                 StateReady,
		At:                    "2026-07-27T13:00:00Z",
		PromptTokensPerSecond: &prompt,
	}}
	first := cloneSnapshot(fixture.runtime.snapshot)
	fixture.runtime.mu.Unlock()

	*first.Metrics.PromptTokensPerSecond = 99
	*first.History[0].PromptTokensPerSecond = 88

	fixture.runtime.mu.RLock()
	core.AssertEqual(t, 12.5, *fixture.runtime.snapshot.Metrics.PromptTokensPerSecond)
	core.AssertEqual(t, 12.5, *fixture.runtime.snapshot.History[0].PromptTokensPerSecond)
	fixture.runtime.mu.RUnlock()
}

func TestService_Sampler_GoodStartsOneLoopAndStopsWithManagedRuntime(t *core.T) {
	fixture := newRuntimeFixture(t)
	fixture.lifecycle.snapshot.State = services.StateRunning
	fixture.lifecycle.snapshot.Desired = true
	ticks := make(chan time.Time, 2)
	fixture.runtime.options.DisableSampler = false
	fixture.runtime.options.After = func(time.Duration) <-chan time.Time {
		return ticks
	}

	first := fixture.runtime.Snapshot()
	core.RequireTrue(t, first.OK, first.Error())
	ticks <- fixture.now
	waitForLifecycleCalls(t, fixture.lifecycle, 2)

	fixture.lifecycle.mu.Lock()
	fixture.lifecycle.snapshot.State = services.StateStopped
	fixture.lifecycle.snapshot.Desired = false
	fixture.lifecycle.mu.Unlock()
	ticks <- fixture.now
	waitForLifecycleCalls(t, fixture.lifecycle, 3)

	for attempt := 0; attempt < 100; attempt++ {
		fixture.runtime.mu.RLock()
		stopped := fixture.runtime.samplerCancel == nil
		fixture.runtime.mu.RUnlock()
		if stopped {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("sampler did not stop after the managed runtime stopped")
}

func waitForClientCalls(t *core.T, client *fakeRuntimeClient, count int) {
	t.Helper()
	for attempt := 0; attempt < 1_000; attempt++ {
		if len(client.callSnapshot()) >= count {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("client did not reach %d calls", count)
}

func waitForLifecycleCalls(t *core.T, lifecycle *fakeLifecycle, count int) {
	t.Helper()
	for attempt := 0; attempt < 1_000; attempt++ {
		if len(lifecycle.callSnapshot()) >= count {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("lifecycle did not reach %d calls", count)
}
