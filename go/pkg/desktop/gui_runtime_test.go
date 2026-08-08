// SPDX-Licence-Identifier: EUPL-1.2

package desktop

import (
	"runtime"

	core "dappco.re/go"
	"dappco.re/go/config"
	gui "dappco.re/go/render/display/webkit"
	guicontextmenu "dappco.re/go/render/display/webkit/pkg/contextmenu"
	guievents "dappco.re/go/render/display/webkit/pkg/events"
	guikeybinding "dappco.re/go/render/display/webkit/pkg/keybinding"
	guisystray "dappco.re/go/render/display/webkit/pkg/systray"
	guiwindow "dappco.re/go/render/display/webkit/pkg/window"
	"dappco.re/lthn/desktop/pkg/connection"
	"github.com/wailsapp/wails/v3/pkg/application"
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
	transport := connection.NewService(connection.Options{Address: "127.0.0.1:0"}).Transport()
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
	}, connection.NewService(connection.Options{Address: "127.0.0.1:0"}).Transport(), cfg)

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
	transport := connection.NewService(connection.Options{Address: "127.0.0.1:0"}).Transport()
	result := newGUIRuntime(nil, gui.GuiConfig{}, transport)
	core.AssertFalse(t, result.OK)
	core.AssertContains(t, result.Error(), "core is nil")
}

// TestGUIRuntime_New_Good_ConstructsRuntimeWithoutRun proves the full
// newGUIRuntime success path — application.New, registerCoreGUIServices,
// and applyGUIConfiguration all complete against a real, empty Core —
// without ever calling the returned runtime's Run().
func TestGUIRuntime_New_Good_ConstructsRuntimeWithoutRun(t *core.T) {
	t.Setenv("HOME", t.TempDir())
	c := core.New()
	t.Cleanup(func() { _ = c.ServiceShutdown(core.Background()) })
	transport := connection.NewService(connection.Options{Address: "127.0.0.1:0"}).Transport()

	result := newGUIRuntime(c, gui.GuiConfig{}, transport)

	core.RequireTrue(t, result.OK, result.Error())
	runtime, ok := result.Value.(*guiRuntime)
	core.RequireTrue(t, ok)
	core.AssertNotNil(t, runtime.App())
}

func TestGUIRuntime_RegisterCoreGUIServices_Good_WiresWindowService(t *core.T) {
	t.Setenv("HOME", t.TempDir())
	c := core.New()
	t.Cleanup(func() {
		_ = c.ServiceShutdown(core.Background())
	})
	app := application.New(application.Options{
		Name:      "lthn-test",
		Transport: connection.NewService(connection.Options{Address: "127.0.0.1:0"}).Transport(),
	})

	result := registerCoreGUIServices(c, app, gui.GuiConfig{})

	core.RequireTrue(t, result.OK, result.Error())
	service, ok := core.ServiceFor[*guiwindow.Service](c, "window")
	core.AssertTrue(t, ok)
	core.AssertNotNil(t, service)
}

// testWailsApp returns the process-wide Wails application singleton,
// constructing it on first call. application.New caches globalApplication
// and every later call (any options) returns the same instance — safe to
// share across this file's tests. The transport binds the LTHN_WAILS_WS_LISTEN
// loopback port (the port-seam contract every go test invocation in this
// package sets); Run() is never called, so no native event loop starts and
// no window is ever actually shown — Wails v3 defers real window/tray/menu
// construction until Run(), queuing everything onto app.pendingRun instead
// (window_manager.go's runOrDeferToAppRun).
func testWailsApp(t *core.T) *application.App {
	t.Helper()
	return application.New(application.Options{
		Name:      "lthn-desktop-test",
		Transport: connection.NewService(connection.Options{Address: "127.0.0.1:0"}).Transport(),
	})
}

func TestGUIRuntime_ActivationPolicy_Good_Accessory(t *core.T) {
	core.AssertEqual(t, application.ActivationPolicyAccessory, guiActivationPolicy(gui.ActivationPolicyAccessory))
}

func TestGUIRuntime_ActivationPolicy_Bad_Prohibited(t *core.T) {
	core.AssertEqual(t, application.ActivationPolicyProhibited, guiActivationPolicy(gui.ActivationPolicyProhibited))
}

func TestGUIRuntime_ActivationPolicy_Ugly_UnknownFallsBackToRegular(t *core.T) {
	core.AssertEqual(t, application.ActivationPolicyRegular, guiActivationPolicy(gui.ActivationPolicy(99)))
}

func TestGUIRuntime_SingleInstanceOptions_Good_ExplicitUniqueIDWins(t *core.T) {
	result := guiSingleInstanceOptions(gui.SingleInstanceOptions{UniqueID: "custom.bundle.id"})
	core.AssertEqual(t, "custom.bundle.id", result.UniqueID)
}

func TestGUIRuntime_SingleInstanceOptions_Bad_EmptyUniqueIDFallsBackToDefault(t *core.T) {
	result := guiSingleInstanceOptions(gui.SingleInstanceOptions{})
	core.AssertEqual(t, "ai.lthn.desktop", result.UniqueID)
}

func TestGUIRuntime_SingleInstanceOptions_Ugly_CallbackIsWrapped(t *core.T) {
	var received gui.SecondInstanceData
	var called bool
	result := guiSingleInstanceOptions(gui.SingleInstanceOptions{
		OnSecondInstanceLaunch: func(d gui.SecondInstanceData) {
			called = true
			received = d
		},
	})

	core.AssertNotNil(t, result.OnSecondInstanceLaunch)
	result.OnSecondInstanceLaunch(application.SecondInstanceData{
		Args:           []string{"lthn", "lthn://chat"},
		WorkingDir:     "/tmp",
		AdditionalData: map[string]string{"app": "lthn-desktop"},
	})

	core.AssertTrue(t, called)
	core.AssertEqual(t, []string{"lthn", "lthn://chat"}, received.Args)
	core.AssertEqual(t, "/tmp", received.WorkingDir)
	core.AssertEqual(t, "lthn-desktop", received.AdditionalData["app"])
}

func TestGUIRuntime_SingleInstanceOptions_Ugly_NilCallbackStaysNil(t *core.T) {
	result := guiSingleInstanceOptions(gui.SingleInstanceOptions{})
	core.AssertNil(t, result.OnSecondInstanceLaunch)
}

func TestGUIRuntime_FirstDesktopConfig_Bad_EmptyReturnsNil(t *core.T) {
	core.AssertNil(t, firstDesktopConfig(nil))
}

func TestGUIRuntime_FirstDesktopConfig_Good_SkipsNilAndUnconfigured(t *core.T) {
	_, real := guiRuntimeConfigFixture(t)
	unconfigured := new(config.Service)

	result := firstDesktopConfig([]*config.Service{nil, unconfigured, real})

	core.AssertEqual(t, real, result)
}

func TestGUIRuntime_DesktopConfigString_Bad_NilConfigUsesFallback(t *core.T) {
	core.AssertEqual(t, "fallback", desktopConfigString(nil, "any.key", "fallback"))
}

func TestGUIRuntime_DesktopConfigString_Good_ConfiguredValueWins(t *core.T) {
	_, cfg := guiRuntimeConfigFixture(t)
	core.RequireTrue(t, cfg.Set("desktop.tray.tooltip", "Custom Tooltip").OK)

	core.AssertEqual(t, "Custom Tooltip", desktopConfigString(cfg, "desktop.tray.tooltip", "fallback"))
}

func TestGUIRuntime_DesktopConfigString_Ugly_UnsetKeyUsesFallback(t *core.T) {
	_, cfg := guiRuntimeConfigFixture(t)

	core.AssertEqual(t, "fallback", desktopConfigString(cfg, "desktop.tray.unset", "fallback"))
}

func TestGUIRuntime_ApplyGUIModeDefaults_Bad_NilConfigIsNoop(t *core.T) {
	applyGUIModeDefaults(nil)
}

func TestGUIRuntime_ApplyGUIModeDefaults_Good_TrayModeForcesAccessory(t *core.T) {
	cfg := &gui.GuiConfig{Mode: gui.ModeTray, Mac: gui.MacOptions{ActivationPolicy: gui.ActivationPolicyRegular}}
	applyGUIModeDefaults(cfg)
	core.AssertEqual(t, gui.ActivationPolicyAccessory, cfg.Mac.ActivationPolicy)
	core.AssertTrue(t, cfg.Windows.DisableQuitOnLastWindowClosed)
}

func TestGUIRuntime_ApplyGUIModeDefaults_Ugly_MultiWindowDisablesQuitOnClose(t *core.T) {
	cfg := &gui.GuiConfig{Mode: gui.ModeMultiWindow}
	applyGUIModeDefaults(cfg)
	core.AssertTrue(t, cfg.Windows.DisableQuitOnLastWindowClosed)
}

func TestGUIRuntime_ApplyGUIModeDefaults_Ugly_SingleWindowTerminatesAfterClose(t *core.T) {
	cfg := &gui.GuiConfig{Mode: gui.ModeSingleWindow}
	applyGUIModeDefaults(cfg)
	core.AssertTrue(t, cfg.Mac.ApplicationShouldTerminateAfterLastWindowClosed)
}

// --- registerGUIWindow / applyGUIConfiguration -----------------------

func registerGUIWindowActionFixture(t *core.T) (*core.Core, *[]string) {
	t.Helper()
	c := core.New()
	seen := []string{}
	for _, name := range []string{
		"window.register",
		"window.open",
		"window.set_close_behavior",
		"window.set_content_protection",
	} {
		actionName := name
		c.Action(actionName, func(_ core.Context, _ core.Options) core.Result {
			seen = append(seen, actionName)
			return core.Ok(nil)
		})
	}
	return c, &seen
}

func TestGUIRuntime_RegisterGUIWindow_Good_RegisterAndPreCreate(t *core.T) {
	c, seen := registerGUIWindowActionFixture(t)
	spec := &guiwindow.Window{Name: "widget"}

	result := registerGUIWindow(c, spec)

	core.RequireTrue(t, result.OK, result.Error())
	core.AssertEqual(t, []string{"window.register", "window.open"}, *seen)
}

func TestGUIRuntime_RegisterGUIWindow_Good_HideOnCloseAndContentProtection(t *core.T) {
	c, seen := registerGUIWindowActionFixture(t)
	spec := &guiwindow.Window{Name: "widget", HideOnClose: true, ContentProtection: true}

	result := registerGUIWindow(c, spec)

	core.RequireTrue(t, result.OK, result.Error())
	core.AssertEqual(t, []string{
		"window.register", "window.open", "window.set_close_behavior", "window.set_content_protection",
	}, *seen)
}

func TestGUIRuntime_RegisterGUIWindow_Bad_RegisterFailurePropagates(t *core.T) {
	c := core.New()
	result := registerGUIWindow(c, &guiwindow.Window{Name: "widget"})

	core.AssertFalse(t, result.OK)
	core.AssertContains(t, result.Error(), "register widget failed")
}

func TestGUIRuntime_RegisterGUIWindow_Ugly_OpenFailurePropagates(t *core.T) {
	c := core.New()
	c.Action("window.register", func(_ core.Context, _ core.Options) core.Result {
		return core.Ok(nil)
	})

	result := registerGUIWindow(c, &guiwindow.Window{Name: "widget"})

	core.AssertFalse(t, result.OK)
	core.AssertContains(t, result.Error(), "pre-create widget failed")
}

func TestGUIRuntime_ApplyGUIConfiguration_Good_WindowStatePathAppliedToRealWindowService(t *core.T) {
	t.Setenv("HOME", t.TempDir())
	c := core.New()
	t.Cleanup(func() { _ = c.ServiceShutdown(core.Background()) })
	app := testWailsApp(t)
	core.RequireTrue(t, registerCoreGUIServices(c, app, gui.GuiConfig{}).OK)

	statePath := core.PathJoin(t.TempDir(), "window_state.json")
	result := applyGUIConfiguration(c, gui.GuiConfig{WindowStatePath: statePath})

	core.RequireTrue(t, result.OK, result.Error())
}

func TestGUIRuntime_ApplyGUIConfiguration_Good_WindowRegistrySkipsInvalidEntries(t *core.T) {
	c, seen := registerGUIWindowActionFixture(t)

	result := applyGUIConfiguration(c, gui.GuiConfig{
		WindowRegistry: []*guiwindow.Window{nil, {Name: ""}, {Name: "widget"}},
	})

	core.RequireTrue(t, result.OK, result.Error())
	core.AssertEqual(t, []string{"window.register", "window.open"}, *seen)
}

func TestGUIRuntime_ApplyGUIConfiguration_Bad_WindowRegistryErrorPropagates(t *core.T) {
	c := core.New()

	result := applyGUIConfiguration(c, gui.GuiConfig{
		WindowRegistry: []*guiwindow.Window{{Name: "widget"}},
	})

	core.AssertFalse(t, result.OK)
}

// --- applyGUITray ------------------------------------------------------

func applyGUITrayActionFixture(t *core.T) (*core.Core, *map[string]int) {
	t.Helper()
	c := core.New()
	calls := map[string]int{}
	for _, name := range []string{
		"systray.set_icon",
		"systray.set_template_icon",
		"systray.set_tooltip",
		"systray.set_label",
		"systray.set_menu",
		"systray.attach_window",
	} {
		actionName := name
		c.Action(actionName, func(_ core.Context, _ core.Options) core.Result {
			calls[actionName]++
			return core.Ok(nil)
		})
	}
	return c, &calls
}

func TestGUIRuntime_ApplyGUITray_Bad_NilCoreIsNoop(t *core.T) {
	applyGUITray(nil, &gui.TrayConfig{})
}

func TestGUIRuntime_ApplyGUITray_Bad_NilConfigIsNoop(t *core.T) {
	applyGUITray(core.New(), nil)
}

func TestGUIRuntime_ApplyGUITray_Good_TooltipAndLabelAlwaysSet(t *core.T) {
	c, calls := applyGUITrayActionFixture(t)

	applyGUITray(c, &gui.TrayConfig{Tooltip: "Lethean Desktop", Label: "lthn"})

	core.AssertEqual(t, 1, (*calls)["systray.set_tooltip"])
	core.AssertEqual(t, 1, (*calls)["systray.set_label"])
	core.AssertEqual(t, 0, (*calls)["systray.set_icon"])
	core.AssertEqual(t, 0, (*calls)["systray.set_menu"])
	core.AssertEqual(t, 0, (*calls)["systray.attach_window"])
}

func TestGUIRuntime_ApplyGUITray_Good_IconMenuAndPopoverWindow(t *core.T) {
	c, calls := applyGUITrayActionFixture(t)

	applyGUITray(c, &gui.TrayConfig{
		Icon:          []byte{0x89, 'P', 'N', 'G'},
		IconTemplate:  false,
		Menu:          []gui.TrayItem{{Label: "Open", ActionID: "open"}},
		PopoverWindow: "tray-panel",
	})

	core.AssertEqual(t, 1, (*calls)["systray.set_icon"])
	core.AssertEqual(t, 0, (*calls)["systray.set_template_icon"])
	core.AssertEqual(t, 1, (*calls)["systray.set_menu"])
	core.AssertEqual(t, 1, (*calls)["systray.attach_window"])
}

func TestGUIRuntime_ApplyGUITray_Ugly_TemplateIconOnDarwin(t *core.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("template-icon branch is darwin-only")
	}
	c, calls := applyGUITrayActionFixture(t)

	applyGUITray(c, &gui.TrayConfig{Icon: []byte{0x89, 'P', 'N', 'G'}, IconTemplate: true})

	core.AssertEqual(t, 1, (*calls)["systray.set_template_icon"])
	core.AssertEqual(t, 0, (*calls)["systray.set_icon"])
}

// --- applyGUITrayRoutes --------------------------------------------------

func TestGUIRuntime_ApplyGUITrayRoutes_Good_EmptyRoutesRegistersNoHandler(t *core.T) {
	c := core.New()
	applyGUITrayRoutes(c, nil)

	result := c.ACTION(guisystray.ActionTrayMenuItemClicked{ActionID: "lthn:tray:quit"})
	core.AssertTrue(t, result.OK)
}

func TestGUIRuntime_ApplyGUITrayRoutes_Good_OpenWindowAndEmitEvent(t *core.T) {
	c := core.New()
	var openedWindow string
	var emittedEvent string
	c.Action("window.restore", func(_ core.Context, _ core.Options) core.Result { return core.Ok(nil) })
	c.Action("window.set_visibility", func(_ core.Context, _ core.Options) core.Result { return core.Ok(nil) })
	c.Action("window.focus", func(_ core.Context, _ core.Options) core.Result { return core.Ok(nil) })
	c.Action("dock.show_icon", func(_ core.Context, _ core.Options) core.Result { return core.Ok(nil) })
	c.Action("events.emit", func(_ core.Context, opts core.Options) core.Result {
		if task, ok := opts.Get("task").Value.(guievents.TaskEmit); ok {
			emittedEvent = task.Name
		}
		return core.Ok(nil)
	})
	applyGUITrayRoutes(c, []gui.TrayRoute{
		{ActionID: "lthn:tray:open-app", OpenWindow: "app", EmitEvent: "lthn:tray:opened"},
	})
	_ = openedWindow

	result := c.ACTION(guisystray.ActionTrayMenuItemClicked{ActionID: "lthn:tray:open-app"})

	core.RequireTrue(t, result.OK, result.Error())
	core.AssertEqual(t, "lthn:tray:opened", emittedEvent)
}

func TestGUIRuntime_ApplyGUITrayRoutes_Good_QuitRoutesToLifecycleQuit(t *core.T) {
	c := core.New()
	var quitCalled bool
	c.Action("lifecycle.quit", func(_ core.Context, _ core.Options) core.Result {
		quitCalled = true
		return core.Ok(nil)
	})
	applyGUITrayRoutes(c, []gui.TrayRoute{{ActionID: "lthn:tray:quit", Quit: true}})

	result := c.ACTION(guisystray.ActionTrayMenuItemClicked{ActionID: "lthn:tray:quit"})

	core.RequireTrue(t, result.OK, result.Error())
	core.AssertTrue(t, quitCalled)
}

func TestGUIRuntime_ApplyGUITrayRoutes_Bad_UnknownActionIDIsNoop(t *core.T) {
	c := core.New()
	applyGUITrayRoutes(c, []gui.TrayRoute{{ActionID: "lthn:tray:quit", Quit: true}})

	result := c.ACTION(guisystray.ActionTrayMenuItemClicked{ActionID: "lthn:tray:unrouted"})
	core.RequireTrue(t, result.OK, result.Error())

	result = c.ACTION(struct{ unrelated bool }{true})
	core.RequireTrue(t, result.OK, result.Error())
}

// --- applyGUIKeybindings -------------------------------------------------

func TestGUIRuntime_ApplyGUIKeybindings_Good_EmptyIsNoop(t *core.T) {
	c := core.New()
	applyGUIKeybindings(c, nil)

	result := c.ACTION(guikeybinding.ActionTriggered{Accelerator: "Cmd+K"})
	core.AssertTrue(t, result.OK)
}

func TestGUIRuntime_ApplyGUIKeybindings_Good_TriggersEmitsMappedEvent(t *core.T) {
	c := core.New()
	c.Action("keybinding.add", func(_ core.Context, _ core.Options) core.Result { return core.Ok(nil) })
	var emittedEvent string
	c.Action("events.emit", func(_ core.Context, opts core.Options) core.Result {
		if task, ok := opts.Get("task").Value.(guievents.TaskEmit); ok {
			emittedEvent = task.Name
		}
		return core.Ok(nil)
	})
	applyGUIKeybindings(c, []gui.Keybinding{
		{Accelerator: "", Description: "skipped — no accelerator"},
		{Accelerator: "Cmd+K", Description: "command palette", EventName: "lthn:key:command"},
	})

	result := c.ACTION(guikeybinding.ActionTriggered{Accelerator: "Cmd+K"})

	core.RequireTrue(t, result.OK, result.Error())
	core.AssertEqual(t, "lthn:key:command", emittedEvent)
}

func TestGUIRuntime_ApplyGUIKeybindings_Bad_UnknownAcceleratorIsNoop(t *core.T) {
	c := core.New()
	c.Action("keybinding.add", func(_ core.Context, _ core.Options) core.Result { return core.Ok(nil) })
	applyGUIKeybindings(c, []gui.Keybinding{{Accelerator: "Cmd+K", EventName: "lthn:key:command"}})

	result := c.ACTION(guikeybinding.ActionTriggered{Accelerator: "Cmd+Z"})
	core.RequireTrue(t, result.OK, result.Error())

	result = c.ACTION(struct{ unrelated bool }{true})
	core.RequireTrue(t, result.OK, result.Error())
}

// --- applyGUIContextMenus -------------------------------------------------

func TestGUIRuntime_ApplyGUIContextMenus_Good_EmptyIsNoop(t *core.T) {
	c := core.New()
	applyGUIContextMenus(c, nil)

	result := c.ACTION(guicontextmenu.ActionItemClicked{MenuName: "lthn-message", ActionID: "copy"})
	core.AssertTrue(t, result.OK)
}

func TestGUIRuntime_ApplyGUIContextMenus_Good_ClickBuildsTemplatedEvent(t *core.T) {
	c := core.New()
	c.Action("contextmenu.add", func(_ core.Context, _ core.Options) core.Result { return core.Ok(nil) })
	var emittedEvent string
	var emittedData string
	c.Action("events.emit", func(_ core.Context, opts core.Options) core.Result {
		if task, ok := opts.Get("task").Value.(guievents.TaskEmit); ok {
			emittedEvent = task.Name
			emittedData, _ = task.Data.(string)
		}
		return core.Ok(nil)
	})
	applyGUIContextMenus(c, []gui.ContextMenu{
		{Name: "", Items: nil}, // skipped — empty name
		{
			Name:            "lthn-message",
			EventTemplate:   "lthn:context:{menu}:{action}",
			MenuPrefixStrip: "lthn-",
			Items:           []gui.ContextMenuItem{{Label: "Copy", ActionID: "copy"}},
		},
	})

	result := c.ACTION(guicontextmenu.ActionItemClicked{MenuName: "lthn-message", ActionID: "copy", Data: "payload"})

	core.RequireTrue(t, result.OK, result.Error())
	core.AssertEqual(t, "lthn:context:message:copy", emittedEvent)
	core.AssertEqual(t, "payload", emittedData)
}

func TestGUIRuntime_ApplyGUIContextMenus_Bad_UnknownMenuNameIsNoop(t *core.T) {
	c := core.New()
	c.Action("contextmenu.add", func(_ core.Context, _ core.Options) core.Result { return core.Ok(nil) })
	applyGUIContextMenus(c, []gui.ContextMenu{{
		Name: "lthn-message", EventTemplate: "lthn:context:{menu}:{action}",
		Items: []gui.ContextMenuItem{{Label: "Copy", ActionID: "copy"}},
	}})

	result := c.ACTION(guicontextmenu.ActionItemClicked{MenuName: "lthn-unknown", ActionID: "copy"})
	core.RequireTrue(t, result.OK, result.Error())

	result = c.ACTION(struct{ unrelated bool }{true})
	core.RequireTrue(t, result.OK, result.Error())
}

func TestGUIRuntime_ApplyGUIContextMenus_Ugly_EmptyTemplateIsNoop(t *core.T) {
	c := core.New()
	c.Action("contextmenu.add", func(_ core.Context, _ core.Options) core.Result { return core.Ok(nil) })
	applyGUIContextMenus(c, []gui.ContextMenu{{
		Name:  "lthn-bare",
		Items: []gui.ContextMenuItem{{Label: "Copy", ActionID: "copy"}},
	}})

	result := c.ACTION(guicontextmenu.ActionItemClicked{MenuName: "lthn-bare", ActionID: "copy"})
	core.RequireTrue(t, result.OK, result.Error())
}

// --- applyGUIAppMenu -------------------------------------------------------

func TestGUIRuntime_ApplyGUIAppMenu_Bad_EmptyItemsIsNoop(t *core.T) {
	c := core.New()
	var called bool
	c.Action("menu.set_app_menu", func(_ core.Context, _ core.Options) core.Result {
		called = true
		return core.Ok(nil)
	})

	applyGUIAppMenu(c, nil)

	core.AssertFalse(t, called)
}

func TestGUIRuntime_ApplyGUIAppMenu_Good_DarwinSetsAppMenu(t *core.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("applyGUIAppMenu is darwin-only")
	}
	c := core.New()
	var called bool
	c.Action("menu.set_app_menu", func(_ core.Context, _ core.Options) core.Result {
		called = true
		return core.Ok(nil)
	})

	applyGUIAppMenu(c, []gui.MenuItem{{Role: &gui.RoleAppMenu}})

	core.AssertTrue(t, called)
}

// --- guiRuntime.App / guiRuntime.Run guard clauses -------------------------
//
// guiRuntime.Run() is deliberately never exercised past its guard clauses —
// the success path calls the real Wails event loop (application.App.Run()),
// which blocks on the native NSApp run loop. desktop_test.go documents the
// same boundary for Service.Run().

func TestGUIRuntime_App_Bad_NilReceiverReturnsNil(t *core.T) {
	var r *guiRuntime
	core.AssertNil(t, r.App())
}

func TestGUIRuntime_App_Good_ReturnsWrappedApp(t *core.T) {
	app := testWailsApp(t)
	r := &guiRuntime{app: app}
	core.AssertEqual(t, app, r.App())
}

func TestGUIRuntime_Run_Bad_NilReceiverFails(t *core.T) {
	var r *guiRuntime
	result := r.Run()
	core.AssertFalse(t, result.OK)
	core.AssertContains(t, result.Error(), "runtime is not initialised")
}

func TestGUIRuntime_Run_Ugly_NilAppFails(t *core.T) {
	r := &guiRuntime{}
	result := r.Run()
	core.AssertFalse(t, result.OK)
	core.AssertContains(t, result.Error(), "runtime is not initialised")
}
