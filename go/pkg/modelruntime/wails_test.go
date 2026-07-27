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
