// SPDX-Licence-Identifier: EUPL-1.2

package services

import (
	"context"
	"time"

	core "dappco.re/go"
	coreio "dappco.re/go/io"
	coreprocess "dappco.re/go/process"
)

type scheduledRestart struct {
	delay time.Duration
	fire  chan time.Time
}

type fakeRestartClock struct {
	scheduled chan scheduledRestart
}

func newFakeRestartClock() *fakeRestartClock {
	return &fakeRestartClock{scheduled: make(chan scheduledRestart, 8)}
}

func (clock *fakeRestartClock) After(delay time.Duration) <-chan time.Time {
	fire := make(chan time.Time, 1)
	clock.scheduled <- scheduledRestart{delay: delay, fire: fire}
	return fire
}

type restartFixture struct {
	service *Service
	runtime *fakeProcessRuntime
	first   *fakeManagedProcess
	second  *fakeManagedProcess
	third   *fakeManagedProcess
	clock   *fakeRestartClock
}

func newRestartFixture(
	t *core.T,
	policy RestartPolicy,
	restartLimit int,
) *restartFixture {
	t.Helper()
	medium := coreio.NewMemoryMedium()
	catalogue := NewMediumCatalogue(medium, testCataloguePath, DefaultLimits())
	definition := validDefinition()
	definition.RestartPolicy = policy
	core.RequireTrue(t, catalogue.Save(CatalogueDocument{
		Version:         CatalogueVersion,
		Definitions:     []Definition{definition},
		PolicyOverrides: []PolicyOverride{},
		UpdatedAt:       "2026-07-27T12:00:00Z",
	}).OK)
	first := newFakeManagedProcess("proc-1", 4101)
	second := newFakeManagedProcess("proc-2", 4102)
	third := newFakeManagedProcess("proc-3", 4103)
	runtime := &fakeProcessRuntime{
		startResults: []core.Result{
			core.Ok(ProcessHandle(first)),
			core.Ok(ProcessHandle(second)),
			core.Ok(ProcessHandle(third)),
		},
		processes: map[string]ProcessHandle{
			"proc-1": first,
			"proc-2": second,
			"proc-3": third,
		},
		startSignal: make(chan struct{}, 8),
	}
	limits := DefaultLimits()
	limits.RestartLimit = restartLimit
	clock := newFakeRestartClock()
	service := NewService(Options{
		Process:   runtime,
		Catalogue: catalogue,
		Limits:    limits,
		After:     clock.After,
		Audit:     &managedServiceAuditRecorder{},
	})
	core.RequireTrue(t, service.Register(core.New()).OK)
	core.RequireTrue(t, service.OnStartup(core.Background()).OK)
	t.Cleanup(func() {
		result := service.Get(definition.ID)
		if result.OK {
			snapshot := result.Value.(Snapshot)
			if snapshot.State == StateRunning || snapshot.Desired {
				_ = service.Stop(definition.ID)
			}
		}
	})
	return &restartFixture{
		service: service,
		runtime: runtime,
		first:   first,
		second:  second,
		third:   third,
		clock:   clock,
	}
}

func receiveStart(t *core.T, runtime *fakeProcessRuntime) {
	t.Helper()
	select {
	case <-runtime.startSignal:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for process start")
	}
}

func receiveRestart(t *core.T, clock *fakeRestartClock) scheduledRestart {
	t.Helper()
	select {
	case scheduled := <-clock.scheduled:
		return scheduled
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for restart schedule")
		return scheduledRestart{}
	}
}

func TestService_RestartOnFailure_GoodRestartsDesiredService(t *core.T) {
	fixture := newRestartFixture(t, RestartOnFailure, 5)
	core.RequireTrue(t, fixture.service.Start("local-api").OK)
	receiveStart(t, fixture.runtime)

	fixture.first.complete(
		7,
		coreprocess.StatusExited,
		core.Fail(core.E("fake.Wait", "exit 7", nil)),
	)
	scheduled := receiveRestart(t, fixture.clock)
	core.AssertEqual(t, DefaultLimits().RestartBaseDelay, scheduled.delay)
	scheduled.fire <- time.Now()
	receiveStart(t, fixture.runtime)

	snapshot := fixture.service.Get("local-api").Value.(Snapshot)
	core.AssertEqual(t, StateRunning, snapshot.State)
	core.AssertTrue(t, snapshot.Desired)
	core.AssertEqual(t, "proc-2", snapshot.ProcessID)
	core.AssertEqual(t, 1, snapshot.RestartCount)
}

func TestService_RestartNever_BadDoesNotRespawnFailedProcess(t *core.T) {
	fixture := newRestartFixture(t, RestartNever, 5)
	core.RequireTrue(t, fixture.service.Start("local-api").OK)
	receiveStart(t, fixture.runtime)

	fixture.first.complete(
		7,
		coreprocess.StatusExited,
		core.Fail(core.E("fake.Wait", "exit 7", nil)),
	)
	select {
	case <-fixture.clock.scheduled:
		t.Fatal("restart-never scheduled a restart")
	case <-time.After(50 * time.Millisecond):
	}

	snapshot := fixture.service.Get("local-api").Value.(Snapshot)
	core.AssertEqual(t, StateFailed, snapshot.State)
	core.AssertFalse(t, snapshot.Desired)
	core.AssertEqual(t, 1, fixture.runtime.startCount())
}

func TestService_RestartAlways_GoodRestartsSuccessfulExit(t *core.T) {
	fixture := newRestartFixture(t, RestartAlways, 5)
	core.RequireTrue(t, fixture.service.Start("local-api").OK)
	receiveStart(t, fixture.runtime)

	fixture.first.complete(0, coreprocess.StatusExited, core.Ok(nil))
	scheduled := receiveRestart(t, fixture.clock)
	scheduled.fire <- time.Now()
	receiveStart(t, fixture.runtime)

	core.AssertEqual(t, "proc-2", fixture.service.Get("local-api").Value.(Snapshot).ProcessID)
}

func TestService_Stop_UglyCancelsPendingAutomaticRestart(t *core.T) {
	fixture := newRestartFixture(t, RestartOnFailure, 5)
	core.RequireTrue(t, fixture.service.Start("local-api").OK)
	receiveStart(t, fixture.runtime)
	fixture.first.complete(
		7,
		coreprocess.StatusExited,
		core.Fail(core.E("fake.Wait", "exit 7", nil)),
	)
	scheduled := receiveRestart(t, fixture.clock)

	stopped := fixture.service.Stop("local-api")
	scheduled.fire <- time.Now()

	core.RequireTrue(t, stopped.OK, stopped.Error())
	select {
	case <-fixture.runtime.startSignal:
		t.Fatal("stop did not cancel the pending restart")
	case <-time.After(50 * time.Millisecond):
	}
	core.AssertEqual(t, StateStopped, fixture.service.Get("local-api").Value.(Snapshot).State)
}

func TestService_RestartBudget_UglyStopsCrashLoop(t *core.T) {
	fixture := newRestartFixture(t, RestartOnFailure, 1)
	core.RequireTrue(t, fixture.service.Start("local-api").OK)
	receiveStart(t, fixture.runtime)
	fixture.first.complete(
		7,
		coreprocess.StatusExited,
		core.Fail(core.E("fake.Wait", "exit 7", nil)),
	)
	scheduled := receiveRestart(t, fixture.clock)
	scheduled.fire <- time.Now()
	receiveStart(t, fixture.runtime)

	fixture.second.complete(
		8,
		coreprocess.StatusExited,
		core.Fail(core.E("fake.Wait", "exit 8", nil)),
	)
	select {
	case <-fixture.clock.scheduled:
		t.Fatal("exhausted restart budget scheduled another restart")
	case <-time.After(50 * time.Millisecond):
	}

	snapshot := fixture.service.Get("local-api").Value.(Snapshot)
	core.AssertEqual(t, StateFailed, snapshot.State)
	core.AssertFalse(t, snapshot.Desired)
	core.AssertEqual(t, ErrorRestartBudgetExhausted, snapshot.LastError.Code)
}

func TestService_Restart_GoodSerialisesStopThenStart(t *core.T) {
	fixture := newServiceFixture(t)
	core.RequireTrue(t, fixture.service.Start("local-api").OK)
	second := newFakeManagedProcess("proc-2", 4202)
	fixture.runtime.mu.Lock()
	fixture.runtime.startResults = []core.Result{core.Ok(ProcessHandle(second))}
	fixture.runtime.processes["proc-2"] = second
	fixture.runtime.mu.Unlock()

	result := fixture.service.Restart("local-api")

	core.RequireTrue(t, result.OK, result.Error())
	snapshot := result.Value.(Snapshot)
	core.AssertEqual(t, StateRunning, snapshot.State)
	core.AssertEqual(t, "proc-2", snapshot.ProcessID)
	core.AssertEqual(t, 1, fixture.process.shutdownCalls)
	core.AssertEqual(t, 2, fixture.runtime.startCount())
}

func TestService_OnShutdown_GoodStopsEveryProcessAndPreventsNewStarts(t *core.T) {
	fixture := newServiceFixture(t)
	secondDefinition := validDefinition()
	secondDefinition.ID = "indexer"
	secondDefinition.DisplayName = "Workspace indexer"
	secondDefinition.Arguments = []string{"index"}
	core.RequireTrue(t, fixture.service.EnsureDefinition(secondDefinition).OK)
	second := newFakeManagedProcess("proc-2", 4302)
	fixture.runtime.mu.Lock()
	fixture.runtime.startResults = []core.Result{
		core.Ok(ProcessHandle(fixture.process)),
		core.Ok(ProcessHandle(second)),
	}
	fixture.runtime.processes["proc-2"] = second
	fixture.runtime.mu.Unlock()
	core.RequireTrue(t, fixture.service.Start("local-api").OK)
	core.RequireTrue(t, fixture.service.Start("indexer").OK)

	result := fixture.service.OnShutdown(context.Background())

	core.RequireTrue(t, result.OK, result.Error())
	core.AssertEqual(t, 1, fixture.process.shutdownCalls)
	core.AssertEqual(t, 1, second.shutdownCalls)
	core.AssertFalse(t, fixture.service.Start("local-api").OK)
}

func TestService_OnShutdown_UglyAggregatesFailureWithoutSkippingAnother(t *core.T) {
	fixture := newServiceFixture(t)
	secondDefinition := validDefinition()
	secondDefinition.ID = "indexer"
	secondDefinition.DisplayName = "Workspace indexer"
	secondDefinition.Arguments = []string{"index"}
	core.RequireTrue(t, fixture.service.EnsureDefinition(secondDefinition).OK)
	second := newFakeManagedProcess("proc-2", 4402)
	fixture.process.shutdownResult = core.Fail(core.E("fake.Shutdown", "stuck", nil))
	fixture.runtime.mu.Lock()
	fixture.runtime.startResults = []core.Result{
		core.Ok(ProcessHandle(fixture.process)),
		core.Ok(ProcessHandle(second)),
	}
	fixture.runtime.processes["proc-2"] = second
	fixture.runtime.mu.Unlock()
	core.RequireTrue(t, fixture.service.Start("local-api").OK)
	core.RequireTrue(t, fixture.service.Start("indexer").OK)

	result := fixture.service.OnShutdown(context.Background())

	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ErrorShutdownIncomplete, ErrorCodeOf(result))
	core.AssertEqual(t, 1, fixture.process.shutdownCalls)
	core.AssertEqual(t, 1, second.shutdownCalls)
}
