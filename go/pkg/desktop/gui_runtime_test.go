// SPDX-Licence-Identifier: EUPL-1.2

package desktop

import (
	core "dappco.re/go"
	gui "dappco.re/go/render/display/webkit"
	"dappco.re/lthn/desktop/pkg/connection"
)

type guiRuntimeBindingFixture struct{}

func TestGUIRuntime_ApplicationOptions_Good_PreservesTransport(t *core.T) {
	transport := connection.NewService(connection.Options{}).Transport()
	bindingFixture := &guiRuntimeBindingFixture{}
	binding := gui.Bind(bindingFixture)
	options := guiApplicationOptions(gui.GuiConfig{
		Name:     "lthn-test",
		Bindings: []gui.Binding{binding},
		SingleInstance: &gui.SingleInstanceOptions{
			UniqueID: "ai.lthn.desktop",
		},
	}, transport)

	core.AssertEqual(t, "lthn-test", options.Name)
	core.AssertEqual(t, transport, options.Transport)
	core.AssertEqual(t, 1, len(options.Services))
	core.AssertEqual(t, bindingFixture, options.Services[0].Instance())
	core.AssertEqual(t, "ai.lthn.desktop", options.SingleInstance.UniqueID)
}

func TestGUIRuntime_New_Bad_NilTransport(t *core.T) {
	result := newGUIRuntime(core.New(), gui.GuiConfig{}, nil)
	core.AssertFalse(t, result.OK)
	core.AssertContains(t, result.Error(), "transport is nil")
}

func TestGUIRuntime_New_Ugly_NilCore(t *core.T) {
	transport := connection.NewService(connection.Options{}).Transport()
	result := newGUIRuntime(nil, gui.GuiConfig{}, transport)
	core.AssertFalse(t, result.OK)
	core.AssertContains(t, result.Error(), "core is nil")
}
