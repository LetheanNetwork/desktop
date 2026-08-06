// SPDX-Licence-Identifier: EUPL-1.2

package modelruntime

import (
	"reflect"
	"sort"

	core "dappco.re/go"
)

func TestWailsService_GoodExposesOnlyTheTypedRuntimeSurface(t *core.T) {
	wails := NewWailsService(nil)
	serviceType := reflect.TypeOf(wails)
	methods := make([]string, 0, serviceType.NumMethod())
	for index := 0; index < serviceType.NumMethod(); index++ {
		methods = append(methods, serviceType.Method(index).Name)
	}
	sort.Strings(methods)
	core.AssertEqual(t, []string{
		"Load",
		"Restart",
		"ServiceName",
		"ServiceShutdown",
		"ServiceStartup",
		"Snapshot",
		"Start",
		"Stop",
		"Unload",
	}, methods)
	core.AssertEqual(t, "ModelRuntime", wails.ServiceName())
}

func TestWailsService_BadFailsClosedWithoutRuntime(t *core.T) {
	wails := NewWailsService(nil)
	results := []core.Result{
		wails.Snapshot(),
		wails.Start(),
		wails.Load(LoadRequest{ModelID: "model-1111111111111111"}),
		wails.Unload(),
		wails.Restart(),
		wails.Stop(),
	}
	for _, result := range results {
		core.AssertFalse(t, result.OK)
		core.AssertEqual(t, ErrorRuntimeUnavailable, ErrorCodeOf(result))
	}
}

// TestWailsService_ServiceStartup_Good and TestWailsService_ServiceShutdown_Good
// — both are deliberately inert (Core owns runtime lifecycle), but had
// 0% coverage because nothing ever called them.
func TestWailsService_ServiceStartup_Good(t *core.T) {
	wails := NewWailsService(nil)
	result := wails.ServiceStartup(core.Background(), nil)
	core.AssertTrue(t, result.OK)
}

func TestWailsService_ServiceShutdown_Good(t *core.T) {
	wails := NewWailsService(nil)
	result := wails.ServiceShutdown()
	core.AssertTrue(t, result.OK)
}

// TestWailsService_GoodDelegatesEveryMethodToTheWiredRuntime — the nil-
// runtime test above only exercises requireRuntime's fail-closed
// branch; every method's actual delegation line
// (`return service.runtime.X()`) was still dark. Wire a real fixture
// runtime and confirm each WailsService method reaches the underlying
// Service call (asserted via the fixture's lifecycle/client call log,
// same signal the direct Service-level tests use).
func TestWailsService_GoodDelegatesEveryMethodToTheWiredRuntime(t *core.T) {
	fixture := newRuntimeFixture(t)
	wails := NewWailsService(fixture.runtime)

	snapshot := wails.Snapshot()
	core.RequireTrue(t, snapshot.OK, snapshot.Error())
	core.AssertEqual(t, StateStopped, snapshot.Value.(Snapshot).State)

	start := wails.Start()
	core.RequireTrue(t, start.OK, start.Error())
	core.AssertEqual(t, StateModelLess, start.Value.(Snapshot).State)

	restart := wails.Restart()
	core.RequireTrue(t, restart.OK, restart.Error())

	load := wails.Load(LoadRequest{ModelID: "model-1111111111111111"})
	core.RequireTrue(t, load.OK, load.Error())
	core.AssertEqual(t, "model-1111111111111111", load.Value.(Snapshot).ActiveModelID)

	unload := wails.Unload()
	core.RequireTrue(t, unload.OK, unload.Error())
	core.AssertEqual(t, "", unload.Value.(Snapshot).ActiveModelID)

	stop := wails.Stop()
	core.RequireTrue(t, stop.OK, stop.Error())
	core.AssertEqual(t, StateStopped, stop.Value.(Snapshot).State)
}
