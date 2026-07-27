// SPDX-Licence-Identifier: EUPL-1.2

package services

import (
	"io/fs"
	"sync"
	"time"

	core "dappco.re/go"
	coreio "dappco.re/go/io"
	coreprocess "dappco.re/go/process"
	"dappco.re/lthn/desktop/pkg/audit"
)

type fakeManagedProcess struct {
	mu             sync.Mutex
	info           coreprocess.Info
	output         string
	done           chan struct{}
	waitResult     core.Result
	shutdownResult core.Result
	shutdownCalls  int
	closeOnce      sync.Once
}

func newFakeManagedProcess(id string, pid int) *fakeManagedProcess {
	return &fakeManagedProcess{
		info: coreprocess.Info{
			ID:        id,
			StartedAt: time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC),
			Running:   true,
			Status:    coreprocess.StatusRunning,
			PID:       pid,
		},
		done:           make(chan struct{}),
		waitResult:     core.Ok(nil),
		shutdownResult: core.Ok(nil),
	}
}

func (process *fakeManagedProcess) Info() coreprocess.Info {
	process.mu.Lock()
	defer process.mu.Unlock()
	return process.info
}

func (process *fakeManagedProcess) Output() string {
	process.mu.Lock()
	defer process.mu.Unlock()
	return process.output
}

func (process *fakeManagedProcess) Wait() core.Result {
	<-process.done
	process.mu.Lock()
	defer process.mu.Unlock()
	return process.waitResult
}

func (process *fakeManagedProcess) Shutdown() core.Result {
	process.mu.Lock()
	process.shutdownCalls++
	result := process.shutdownResult
	if result.OK {
		process.info.Running = false
		process.info.Status = coreprocess.StatusKilled
		process.info.ExitCode = -1
	}
	process.mu.Unlock()
	if result.OK {
		process.closeOnce.Do(func() { close(process.done) })
	}
	return result
}

func (process *fakeManagedProcess) complete(
	exitCode int,
	status coreprocess.Status,
	result core.Result,
) {
	process.mu.Lock()
	process.info.Running = false
	process.info.Status = status
	process.info.ExitCode = exitCode
	process.waitResult = result
	process.mu.Unlock()
	process.closeOnce.Do(func() { close(process.done) })
}

type fakeProcessRuntime struct {
	mu           sync.Mutex
	starts       []coreprocess.RunOptions
	startResult  core.Result
	startResults []core.Result
	processes    map[string]ProcessHandle
	getCalls     []string
	startSignal  chan struct{}
}

func (runtime *fakeProcessRuntime) StartWithOptions(
	_ core.Context,
	options coreprocess.RunOptions,
) core.Result {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	options.Args = append([]string(nil), options.Args...)
	runtime.starts = append(runtime.starts, options)
	if runtime.startSignal != nil {
		runtime.startSignal <- struct{}{}
	}
	if len(runtime.startResults) > 0 {
		result := runtime.startResults[0]
		runtime.startResults = runtime.startResults[1:]
		return result
	}
	return runtime.startResult
}

func (runtime *fakeProcessRuntime) Get(id string) core.Result {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	runtime.getCalls = append(runtime.getCalls, id)
	process, ok := runtime.processes[id]
	if !ok {
		return core.Fail(fs.ErrNotExist)
	}
	return core.Ok(process)
}

func (runtime *fakeProcessRuntime) startCount() int {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	return len(runtime.starts)
}

func (runtime *fakeProcessRuntime) firstStart() coreprocess.RunOptions {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	return runtime.starts[0]
}

type managedServiceAuditRecorder struct {
	mu     sync.Mutex
	events []audit.Event
}

func (recorder *managedServiceAuditRecorder) Record(event audit.Event) core.Result {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	recorder.events = append(recorder.events, event)
	return core.Ok(nil)
}

func (recorder *managedServiceAuditRecorder) snapshot() []audit.Event {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	events := make([]audit.Event, len(recorder.events))
	copy(events, recorder.events)
	return events
}

type serviceFixture struct {
	service    *Service
	runtime    *fakeProcessRuntime
	process    *fakeManagedProcess
	medium     *coreio.MemoryMedium
	definition Definition
	now        time.Time
	core       *core.Core
	audit      *managedServiceAuditRecorder
}

func newServiceFixture(t *core.T) *serviceFixture {
	t.Helper()
	now := time.Date(2026, 7, 27, 12, 30, 0, 0, time.UTC)
	medium := coreio.NewMemoryMedium()
	catalogue := NewMediumCatalogue(medium, testCataloguePath, DefaultLimits())
	definition := validDefinition()
	document := CatalogueDocument{
		Version:         CatalogueVersion,
		Definitions:     []Definition{definition},
		PolicyOverrides: []PolicyOverride{},
		UpdatedAt:       "2026-07-27T12:00:00Z",
	}
	core.RequireTrue(t, catalogue.Save(document).OK)
	process := newFakeManagedProcess("proc-1", 4242)
	runtime := &fakeProcessRuntime{
		startResult: core.Ok(ProcessHandle(process)),
		processes:   map[string]ProcessHandle{"proc-1": process},
	}
	recorder := &managedServiceAuditRecorder{}
	service := NewService(Options{
		Process:   runtime,
		Catalogue: catalogue,
		Limits:    DefaultLimits(),
		Now:       func() time.Time { return now },
		Audit:     recorder,
	})
	coreApp := core.New()
	core.RequireTrue(t, service.Register(coreApp).OK)
	core.RequireTrue(t, service.OnStartup(core.Background()).OK)
	fixture := &serviceFixture{
		service:    service,
		runtime:    runtime,
		process:    process,
		medium:     medium,
		definition: definition,
		now:        now,
		core:       coreApp,
		audit:      recorder,
	}
	t.Cleanup(func() {
		result := fixture.service.Get(definition.ID)
		if !result.OK {
			return
		}
		snapshot := result.Value.(Snapshot)
		if snapshot.State == StateRunning || snapshot.State == StateStarting ||
			snapshot.State == StateStopping {
			_ = fixture.service.Stop(definition.ID)
		}
	})
	return fixture
}

func (fixture *serviceFixture) snapshot(t *core.T) Snapshot {
	t.Helper()
	result := fixture.service.Get(fixture.definition.ID)
	core.RequireTrue(t, result.OK, result.Error())
	return result.Value.(Snapshot)
}

func TestService_OnStartup_GoodLoadsDefinitionsWithoutStarting(t *core.T) {
	fixture := newServiceFixture(t)

	view := fixture.service.Catalogue()

	core.RequireTrue(t, view.OK, view.Error())
	core.AssertEqual(t, 0, fixture.runtime.startCount())
	services := view.Value.(CatalogueView).Services
	core.RequireTrue(t, len(services) == 1)
	core.AssertEqual(t, StateStopped, services[0].State)
	core.AssertFalse(t, services[0].Desired)
}

func TestService_OnStartup_BadCatalogueFailureLeavesManagerUnavailableWithoutStart(t *core.T) {
	medium := coreio.NewMemoryMedium()
	core.RequireNoError(t, medium.Write(testCataloguePath, `{"version":1`))
	runtime := &fakeProcessRuntime{processes: map[string]ProcessHandle{}}
	service := NewService(Options{
		Process: runtime,
		Catalogue: NewMediumCatalogue(
			medium,
			testCataloguePath,
			DefaultLimits(),
		),
		Limits: DefaultLimits(),
		Audit:  &managedServiceAuditRecorder{},
	})

	startup := service.OnStartup(core.Background())
	view := service.Catalogue()

	core.AssertTrue(t, startup.OK, "manager failure must not abort unrelated Core services")
	core.AssertFalse(t, view.OK)
	core.AssertEqual(t, ErrorCatalogueInvalid, ErrorCodeOf(view))
	core.AssertEqual(t, 0, runtime.startCount())
}

func TestService_Start_GoodUsesExpectedGoProcessOptions(t *core.T) {
	fixture := newServiceFixture(t)

	result := fixture.service.Start(fixture.definition.ID)

	core.RequireTrue(t, result.OK, result.Error())
	snapshot := result.Value.(Snapshot)
	core.AssertEqual(t, StateRunning, snapshot.State)
	core.AssertTrue(t, snapshot.Desired)
	core.AssertEqual(t, "proc-1", snapshot.ProcessID)
	core.AssertEqual(t, 4242, snapshot.PID)
	core.AssertEqual(t, 1, fixture.runtime.startCount())
	options := fixture.runtime.firstStart()
	core.AssertEqual(t, fixture.definition.Command, options.Command)
	core.AssertEqual(t, fixture.definition.Arguments, options.Args)
	core.AssertEqual(t, "", options.Dir)
	core.AssertEqual(t, 0, len(options.Env))
	core.AssertFalse(t, options.DisableCapture)
	core.AssertTrue(t, options.Detach)
	core.AssertTrue(t, options.KillGroup)
	core.AssertEqual(t, 5*time.Second, options.GracePeriod)
}

func TestService_Start_GoodIsIdempotentWhileRunning(t *core.T) {
	fixture := newServiceFixture(t)
	core.RequireTrue(t, fixture.service.Start(fixture.definition.ID).OK)

	again := fixture.service.Start(fixture.definition.ID)

	core.RequireTrue(t, again.OK, again.Error())
	core.AssertEqual(t, StateRunning, again.Value.(Snapshot).State)
	core.AssertEqual(t, 1, fixture.runtime.startCount())
}

func TestService_Start_BadUnknownIDNeverReachesProcessRuntime(t *core.T) {
	fixture := newServiceFixture(t)

	result := fixture.service.Start("unknown")

	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ErrorDefinitionNotFound, ErrorCodeOf(result))
	core.AssertEqual(t, 0, fixture.runtime.startCount())
}

func TestService_Start_UglyFailedSpawnClearsDesiredState(t *core.T) {
	fixture := newServiceFixture(t)
	fixture.runtime.startResult = core.Fail(core.E("fake.Start", "boom", nil))

	result := fixture.service.Start(fixture.definition.ID)

	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ErrorProcessStartFailed, ErrorCodeOf(result))
	snapshot := fixture.snapshot(t)
	core.AssertEqual(t, StateFailed, snapshot.State)
	core.AssertFalse(t, snapshot.Desired)
	core.AssertEqual(t, ErrorProcessStartFailed, snapshot.LastError.Code)
}

func TestService_Stop_GoodResolvesIdentityAndDelegatesGracefulShutdown(t *core.T) {
	fixture := newServiceFixture(t)
	core.RequireTrue(t, fixture.service.Start(fixture.definition.ID).OK)

	result := fixture.service.Stop(fixture.definition.ID)

	core.RequireTrue(t, result.OK, result.Error())
	core.AssertEqual(t, StateStopped, result.Value.(Snapshot).State)
	core.AssertFalse(t, result.Value.(Snapshot).Desired)
	core.AssertEqual(t, 1, fixture.process.shutdownCalls)
	core.AssertEqual(t, []string{"proc-1"}, fixture.runtime.getCalls)
}

func TestService_Stop_GoodIsIdempotentWhenAlreadyStopped(t *core.T) {
	fixture := newServiceFixture(t)

	result := fixture.service.Stop(fixture.definition.ID)

	core.RequireTrue(t, result.OK, result.Error())
	core.AssertEqual(t, StateStopped, result.Value.(Snapshot).State)
	core.AssertEqual(t, 0, fixture.process.shutdownCalls)
	core.AssertEqual(t, 0, len(fixture.runtime.getCalls))
}

func TestService_Stop_UglyLookupMismatchNeverSignalsUnverifiedProcess(t *core.T) {
	fixture := newServiceFixture(t)
	core.RequireTrue(t, fixture.service.Start(fixture.definition.ID).OK)
	fixture.runtime.processes = map[string]ProcessHandle{}

	result := fixture.service.Stop(fixture.definition.ID)

	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ErrorProcessLookupFailed, ErrorCodeOf(result))
	core.AssertEqual(t, 0, fixture.process.shutdownCalls)
	core.AssertFalse(t, fixture.snapshot(t).Desired)
}

func TestService_Output_GoodReturnsUTF8SafeBoundedTail(t *core.T) {
	fixture := newServiceFixture(t)
	fixture.process.output = "ready: αβγ"
	core.RequireTrue(t, fixture.service.Start(fixture.definition.ID).OK)

	result := fixture.service.Output(OutputRequest{
		ID:    fixture.definition.ID,
		Limit: 5,
	})

	core.RequireTrue(t, result.OK, result.Error())
	output := result.Value.(OutputView)
	core.AssertEqual(t, "βγ", output.Output)
	core.AssertTrue(t, output.Truncated)
	core.AssertEqual(t, "proc-1", output.ProcessID)
	core.AssertEqual(t, uint64(1), output.Generation)
}

func TestService_Output_BadRejectsUnboundedOrStoppedReads(t *core.T) {
	fixture := newServiceFixture(t)

	for _, request := range []OutputRequest{
		{ID: fixture.definition.ID, Limit: 0},
		{ID: fixture.definition.ID, Limit: DefaultLimits().MaxOutputBytes + 1},
	} {
		result := fixture.service.Output(request)
		core.AssertFalse(t, result.OK)
		core.AssertEqual(t, ErrorDefinitionInvalid, ErrorCodeOf(result))
	}
	stopped := fixture.service.Output(OutputRequest{ID: fixture.definition.ID, Limit: 100})
	core.AssertFalse(t, stopped.OK)
	core.AssertEqual(t, ErrorProcessLookupFailed, ErrorCodeOf(stopped))
}

func TestService_EnsureDefinition_GoodPersistsBeforePublishing(t *core.T) {
	fixture := newServiceFixture(t)
	definition := validDefinition()
	definition.ID = "indexer"
	definition.DisplayName = "Workspace indexer"
	definition.Arguments = []string{"index"}

	result := fixture.service.EnsureDefinition(definition)

	core.RequireTrue(t, result.OK, result.Error())
	core.AssertEqual(t, "indexer", result.Value.(Snapshot).Definition.ID)
	content, err := fixture.medium.Read(testCataloguePath)
	core.RequireNoError(t, err)
	core.AssertContains(t, content, `"id":"indexer"`)
}

func TestService_EnsureDefinition_BadRejectsOwnershipConflict(t *core.T) {
	fixture := newServiceFixture(t)
	definition := fixture.definition
	definition.Owner = "another-owner"

	result := fixture.service.EnsureDefinition(definition)

	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ErrorDefinitionConflict, ErrorCodeOf(result))
}

func TestService_RemoveDefinition_GoodPersistsRemoval(t *core.T) {
	fixture := newServiceFixture(t)

	result := fixture.service.RemoveDefinition(
		fixture.definition.Owner,
		fixture.definition.ID,
	)

	core.RequireTrue(t, result.OK, result.Error())
	core.AssertEqual(t, ErrorDefinitionNotFound, ErrorCodeOf(fixture.service.Get(fixture.definition.ID)))
	content, err := fixture.medium.Read(testCataloguePath)
	core.RequireNoError(t, err)
	core.AssertNotContains(t, content, `"id":"local-api"`)
}

func TestService_SetPolicy_GoodPersistsOnlyBoundedPolicy(t *core.T) {
	fixture := newServiceFixture(t)
	override := PolicyOverride{
		ID:                fixture.definition.ID,
		RestartPolicy:     RestartAlways,
		GracePeriodMillis: 8_000,
	}

	result := fixture.service.SetPolicy(override)

	core.RequireTrue(t, result.OK, result.Error())
	snapshot := result.Value.(Snapshot)
	core.AssertEqual(t, RestartAlways, snapshot.Definition.RestartPolicy)
	core.AssertEqual(t, int64(8_000), snapshot.Definition.GracePeriodMillis)
	content, err := fixture.medium.Read(testCataloguePath)
	core.RequireNoError(t, err)
	core.AssertContains(t, content, `"restartPolicy":"always"`)
}

func TestService_Catalogue_UglyReturnsImmutableCopies(t *core.T) {
	fixture := newServiceFixture(t)
	first := fixture.service.Catalogue().Value.(CatalogueView)
	first.Services[0].Definition.DisplayName = "changed"

	second := fixture.service.Catalogue().Value.(CatalogueView)

	core.AssertEqual(t, "Lethean API", second.Services[0].Definition.DisplayName)
}
