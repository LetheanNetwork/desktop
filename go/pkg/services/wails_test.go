// SPDX-Licence-Identifier: EUPL-1.2

package services

import (
	core "dappco.re/go"
)

func TestWailsService_GoodDelegatesManagedLifecycle(t *core.T) {
	fixture := newServiceFixture(t)
	wails := NewWailsService(fixture.service)

	catalogue := wails.Catalogue()
	started := wails.Start("local-api")
	got := wails.Get("local-api")
	output := wails.Output(OutputRequest{ID: "local-api", Limit: 100})
	stopped := wails.Stop("local-api")

	core.RequireTrue(t, catalogue.OK, catalogue.Error())
	core.RequireTrue(t, started.OK, started.Error())
	core.AssertEqual(t, StateRunning, started.Value.(Snapshot).State)
	core.RequireTrue(t, got.OK, got.Error())
	core.AssertEqual(t, "proc-1", got.Value.(Snapshot).ProcessID)
	core.RequireTrue(t, output.OK, output.Error())
	core.RequireTrue(t, stopped.OK, stopped.Error())
	core.AssertEqual(t, StateStopped, stopped.Value.(Snapshot).State)
}

func TestWailsService_GoodDelegatesPolicyAndRestart(t *core.T) {
	fixture := newServiceFixture(t)
	wails := NewWailsService(fixture.service)
	core.RequireTrue(t, wails.SetPolicy(PolicyOverride{
		ID:                "local-api",
		RestartPolicy:     RestartAlways,
		GracePeriodMillis: 7_000,
	}).OK)
	core.RequireTrue(t, wails.Start("local-api").OK)
	second := newFakeManagedProcess("proc-2", 4502)
	fixture.runtime.mu.Lock()
	fixture.runtime.startResults = []core.Result{core.Ok(ProcessHandle(second))}
	fixture.runtime.processes["proc-2"] = second
	fixture.runtime.mu.Unlock()

	restarted := wails.Restart("local-api")

	core.RequireTrue(t, restarted.OK, restarted.Error())
	core.AssertEqual(t, "proc-2", restarted.Value.(Snapshot).ProcessID)
	core.AssertEqual(
		t,
		RestartAlways,
		restarted.Value.(Snapshot).Definition.RestartPolicy,
	)
}

func TestWailsService_BadNilManagerFailsWithoutNativeFallback(t *core.T) {
	wails := NewWailsService(nil)

	for _, result := range []core.Result{
		wails.Catalogue(),
		wails.Get("serve"),
		wails.Start("serve"),
		wails.Stop("serve"),
		wails.Restart("serve"),
		wails.Output(OutputRequest{ID: "serve", Limit: 100}),
		wails.SetPolicy(PolicyOverride{
			ID:                "serve",
			RestartPolicy:     RestartNever,
			GracePeriodMillis: 5_000,
		}),
	} {
		core.AssertFalse(t, result.OK)
		core.AssertEqual(t, ErrorServicesUnavailable, ErrorCodeOf(result))
	}
}

func TestWailsService_GoodKeepsNativeControllerExplicit(t *core.T) {
	wails := NewWailsService(nil)

	core.AssertEqual(t, "Lifecycle", wails.ServiceName())
	core.AssertGreater(t, len(wails.NativeRegistry()), 0)
	for _, result := range []core.Result{
		wails.InstallNative("unknown"),
		wails.UninstallNative("unknown"),
		wails.StartNative("unknown"),
		wails.StopNative("unknown"),
		wails.RestartNative("unknown"),
		wails.StatusNative("unknown"),
	} {
		core.AssertFalse(t, result.OK)
	}
}

func TestWailsService_UglyBindingLifecycleDoesNotOwnCoreShutdown(t *core.T) {
	fixture := newServiceFixture(t)
	wails := NewWailsService(fixture.service)
	core.RequireTrue(t, wails.Start("local-api").OK)

	core.AssertTrue(t, wails.ServiceStartup(core.Background(), nil).OK)
	core.AssertTrue(t, wails.ServiceShutdown().OK)

	core.AssertEqual(t, StateRunning, fixture.snapshot(t).State)
}

func TestWailsService_Signal_GoodDelegatesToTheManager(t *core.T) {
	fixture := newServiceFixture(t)
	wails := NewWailsService(fixture.service)
	core.RequireTrue(t, wails.Start(fixture.definition.ID).OK)

	result := wails.Signal(SignalRequest{ID: fixture.definition.ID, Signal: SignalHangup})

	core.RequireTrue(t, result.OK, result.Error())
	core.AssertEqual(t, 1, len(fixture.runtime.recordedSignals()))
}

func TestWailsService_Kill_GoodDelegatesToTheManager(t *core.T) {
	fixture := newServiceFixture(t)
	wails := NewWailsService(fixture.service)
	core.RequireTrue(t, wails.Start(fixture.definition.ID).OK)

	result := wails.Kill(fixture.definition.ID)

	core.RequireTrue(t, result.OK, result.Error())
	core.AssertEqual(t, []string{"proc-1"}, fixture.runtime.recordedKills())
}

// The boundary refuses before the manager is reached. The manager checks too,
// but it should not be the only thing between a renderer and a syscall.
func TestWailsService_Signal_BadRefusesAnUnknownNameAtTheBoundary(t *core.T) {
	fixture := newServiceFixture(t)
	wails := NewWailsService(fixture.service)
	core.RequireTrue(t, wails.Start(fixture.definition.ID).OK)

	for _, name := range []Signal{"", "9", "SIGTERM", "obliterate"} {
		result := wails.Signal(SignalRequest{ID: fixture.definition.ID, Signal: name})

		core.AssertFalse(t, result.OK, core.Concat("expected ", string(name), " to be refused"))
		core.AssertEqual(t, ErrorSignalUnknown, ErrorCodeOf(result))
	}

	core.AssertEqual(t, 0, len(fixture.runtime.recordedSignals()))
}

func TestWailsService_Ugly_UnboundManagerFailsRatherThanPanicking(t *core.T) {
	wails := NewWailsService(nil)

	signalled := wails.Signal(SignalRequest{ID: "anything", Signal: SignalTerminate})
	killed := wails.Kill("anything")

	core.AssertFalse(t, signalled.OK, "an unbound manager must fail typed")
	core.AssertFalse(t, killed.OK, "an unbound manager must fail typed")
}
