// SPDX-Licence-Identifier: EUPL-1.2

package desktop

import (
	core "dappco.re/go"
	"dappco.re/go/config"
	gui "dappco.re/go/render/display/webkit"
	"dappco.re/lthn/desktop/pkg/connection"
)

type guiRuntimeBindingFixture struct{}

func guiRuntimeConfigFixture(t *core.T) (*core.Core, *config.Service) {
	t.Helper()
	path := core.PathJoin(t.TempDir(), "lthn.yaml")
	c := core.New(core.WithName(
		"config",
		config.NewConfigServiceWith(config.ServiceOptions{Path: path}),
	))
	core.RequireTrue(t, c.ServiceStartup(core.Background(), nil).OK)
	t.Cleanup(func() {
		_ = c.ServiceShutdown(core.Background())
	})
	cfg, ok := core.ServiceFor[*config.Service](c, "config")
	core.RequireTrue(t, ok)
	core.AssertNotNil(t, cfg)
	return c, cfg
}

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

func TestGUIRuntime_ApplicationOptions_Good_UserConfigWinsRuntimeBaseline(t *core.T) {
	_, cfg := guiRuntimeConfigFixture(t)
	core.RequireTrue(t, cfg.Set("desktop.wails.application.name", "configured-lthn").OK)
	core.RequireTrue(t, cfg.Set(
		"desktop.wails.application.windows.use_visual_hosting",
		true,
	).OK)

	options := guiApplicationOptions(gui.GuiConfig{
		Name: "runtime-lthn",
		Windows: gui.WindowsOptions{
			UseVisualHosting: false,
		},
	}, connection.NewService(connection.Options{}).Transport(), cfg)

	core.AssertEqual(t, "configured-lthn", options.Name)
	core.AssertTrue(t, options.Windows.UseVisualHosting)
}

func TestGUIRuntime_SingleInstanceEnabled_Bad_InvalidConfigFallsBack(t *core.T) {
	_, cfg := guiRuntimeConfigFixture(t)
	core.RequireTrue(t, cfg.Set("desktop.single_instance.enabled", "sometimes").OK)

	core.AssertTrue(t, desktopSingleInstanceEnabled(cfg))
}

func TestGUIRuntime_SingleInstanceEnabled_Ugly_UserCanDisable(t *core.T) {
	_, cfg := guiRuntimeConfigFixture(t)
	core.RequireTrue(t, cfg.Set("desktop.single_instance.enabled", false).OK)

	core.AssertFalse(t, desktopSingleInstanceEnabled(cfg))
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
