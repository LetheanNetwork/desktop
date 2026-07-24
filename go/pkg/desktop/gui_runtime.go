// SPDX-Licence-Identifier: EUPL-1.2

package desktop

import (
	"runtime"

	core "dappco.re/go"
	"dappco.re/go/config"
	gui "dappco.re/go/render/display/webkit"
	"dappco.re/go/render/display/webkit/pkg/contextmenu"
	"dappco.re/go/render/display/webkit/pkg/events"
	"dappco.re/go/render/display/webkit/pkg/keybinding"
	"dappco.re/go/render/display/webkit/pkg/lifecycle"
	"dappco.re/go/render/display/webkit/pkg/menu"
	"dappco.re/go/render/display/webkit/pkg/systray"
	"dappco.re/go/render/display/webkit/pkg/window"
	"dappco.re/lthn/desktop/pkg/appconfig"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// guiRuntime is the transport-aware half of the desktop GUI service.
// core/gui remains registered as the configuration facade used by its
// helpers, while this runtime owns the actual Wails application.
type guiRuntime struct {
	app *application.App
}

func newGUIRuntime(c *core.Core, cfg gui.GuiConfig, transport application.Transport) core.Result {
	if c == nil {
		return core.Fail(core.E("desktop.newGUIRuntime", "core is nil", nil))
	}
	if transport == nil {
		return core.Fail(core.E("desktop.newGUIRuntime", "transport is nil", nil))
	}

	applyGUIModeDefaults(&cfg)
	configSvc, _ := core.ServiceFor[*config.Service](c, "config")
	app := application.New(guiApplicationOptions(cfg, transport, configSvc))
	if app == nil {
		return core.Fail(core.E("desktop.newGUIRuntime", "Wails application is nil", nil))
	}
	if r := registerCoreGUIServices(c, app, cfg); !r.OK {
		return r
	}
	if r := applyGUIConfiguration(c, cfg); !r.OK {
		return r
	}
	return core.Ok(&guiRuntime{app: app})
}

func (r *guiRuntime) App() *application.App {
	if r == nil {
		return nil
	}
	return r.app
}

func (r *guiRuntime) Run() core.Result {
	if r == nil || r.app == nil {
		return core.Fail(core.E("desktop.guiRuntime.Run", "runtime is not initialised", nil))
	}
	if err := r.app.Run(); err != nil {
		return core.Fail(core.E("desktop.guiRuntime.Run", "Wails event loop failed", err))
	}
	return core.Ok(nil)
}

func guiApplicationOptions(
	cfg gui.GuiConfig,
	transport application.Transport,
	configs ...*config.Service,
) application.Options {
	name := cfg.Name
	if name == "" {
		name = "core-gui"
	}
	configSvc := firstDesktopConfig(configs)

	var middleware application.Middleware
	if cfg.Assets.Middleware != nil {
		middleware = application.Middleware(cfg.Assets.Middleware)
	}

	options := appconfig.ApplicationOptions(configSvc)
	if !desktopConfigHas(configSvc, "desktop.wails.application.name") {
		options.Name = name
	}
	if !desktopConfigHas(configSvc, "desktop.wails.application.description") {
		options.Description = cfg.Description
	}
	options.Icon = cfg.Icon
	options.Services = cfg.Bindings
	options.Transport = transport
	options.Assets.Handler = cfg.Assets.Handler
	options.Assets.Middleware = middleware
	if !desktopConfigHas(configSvc, "desktop.wails.application.assets.disable_logging") {
		options.Assets.DisableLogging = cfg.Assets.DisableLogging
	}
	if !desktopConfigHas(configSvc, "desktop.wails.application.mac.activation_policy") {
		options.Mac.ActivationPolicy = guiActivationPolicy(cfg.Mac.ActivationPolicy)
	}
	if !desktopConfigHas(
		configSvc,
		"desktop.wails.application.mac.application_should_terminate_after_last_window_closed",
	) {
		options.Mac.ApplicationShouldTerminateAfterLastWindowClosed =
			cfg.Mac.ApplicationShouldTerminateAfterLastWindowClosed
	}
	if !desktopConfigHas(
		configSvc,
		"desktop.wails.application.windows.disable_quit_on_last_window_closed",
	) {
		options.Windows.DisableQuitOnLastWindowClosed =
			cfg.Windows.DisableQuitOnLastWindowClosed
	}
	if !desktopConfigHas(configSvc, "desktop.wails.application.windows.enabled_features") {
		options.Windows.EnabledFeatures = cfg.Windows.EnabledFeatures
	}
	if !desktopConfigHas(configSvc, "desktop.wails.application.windows.disabled_features") {
		options.Windows.DisabledFeatures = cfg.Windows.DisabledFeatures
	}
	if !desktopConfigHas(configSvc, "desktop.wails.application.windows.additional_browser_args") {
		options.Windows.AdditionalBrowserArgs = cfg.Windows.AdditionalBrowserArgs
	}
	if !desktopConfigHas(configSvc, "desktop.wails.application.windows.use_visual_hosting") {
		options.Windows.UseVisualHosting = cfg.Windows.UseVisualHosting
	}
	if cfg.Windows.WndClass != "" &&
		!desktopConfigHas(configSvc, "desktop.wails.application.windows.wnd_class") {
		options.Windows.WndClass = cfg.Windows.WndClass
	}
	if !desktopConfigHas(configSvc, "desktop.wails.application.windows.webview_user_data_path") {
		options.Windows.WebviewUserDataPath = cfg.Windows.WebviewUserDataPath
	}
	if !desktopConfigHas(configSvc, "desktop.wails.application.windows.webview_browser_path") {
		options.Windows.WebviewBrowserPath = cfg.Windows.WebviewBrowserPath
	}
	options.ShouldQuit = cfg.ShouldQuit
	options.OnShutdown = cfg.OnShutdown
	options.PostShutdown = cfg.PostShutdown
	options.SingleInstance = nil
	if cfg.SingleInstance != nil {
		options.SingleInstance = guiSingleInstanceOptions(*cfg.SingleInstance, configSvc)
	}
	if cfg.OnPanic != nil {
		options.PanicHandler = func(details *application.PanicDetails) {
			if details == nil {
				cfg.OnPanic(gui.PanicDetails{})
				return
			}
			cfg.OnPanic(gui.PanicDetails{
				Error:          details.Error,
				StackTrace:     details.StackTrace,
				FullStackTrace: details.FullStackTrace,
			})
		}
	}
	return options
}

func guiActivationPolicy(policy gui.ActivationPolicy) application.ActivationPolicy {
	switch policy {
	case gui.ActivationPolicyAccessory:
		return application.ActivationPolicyAccessory
	case gui.ActivationPolicyProhibited:
		return application.ActivationPolicyProhibited
	default:
		return application.ActivationPolicyRegular
	}
}

func guiSingleInstanceOptions(
	options gui.SingleInstanceOptions,
	configs ...*config.Service,
) *application.SingleInstanceOptions {
	configSvc := firstDesktopConfig(configs)
	defaults := appconfig.ApplicationOptions(configSvc).SingleInstance
	result := &application.SingleInstanceOptions{
		UniqueID: defaults.UniqueID,
		// OnSecondInstanceLaunch is connected below when the desktop supplies a callback.
		OnSecondInstanceLaunch: nil,
		AdditionalData:         options.AdditionalData,
		// ExitCode retains the central successful hand-off default.
		ExitCode:      defaults.ExitCode,
		EncryptionKey: options.EncryptionKey,
	}
	if options.UniqueID != "" &&
		!desktopConfigHas(
			configSvc,
			"desktop.wails.application.single_instance.unique_id",
		) {
		result.UniqueID = options.UniqueID
	}
	if options.OnSecondInstanceLaunch != nil {
		result.OnSecondInstanceLaunch = func(data application.SecondInstanceData) {
			options.OnSecondInstanceLaunch(gui.SecondInstanceData{
				Args:           data.Args,
				WorkingDir:     data.WorkingDir,
				AdditionalData: data.AdditionalData,
			})
		}
	}
	return result
}

func firstDesktopConfig(configs []*config.Service) *config.Service {
	for _, candidate := range configs {
		if candidate != nil && candidate.Config() != nil {
			return candidate
		}
	}
	return nil
}

func desktopConfigHas(cfg *config.Service, key string) bool {
	if cfg == nil || cfg.Config() == nil {
		return false
	}
	var value any
	return cfg.Get(key, &value).OK
}

func desktopConfigString(cfg *config.Service, key string, fallback string) string {
	if cfg == nil || cfg.Config() == nil {
		return fallback
	}
	var value string
	if !cfg.Get(key, &value).OK {
		return fallback
	}
	return value
}

func desktopSingleInstanceEnabled(cfg *config.Service) bool {
	if cfg == nil || cfg.Config() == nil {
		return true
	}
	var enabled bool
	if !cfg.Get("desktop.single_instance.enabled", &enabled).OK {
		return true
	}
	return enabled
}

func applyGUIModeDefaults(cfg *gui.GuiConfig) {
	if cfg == nil {
		return
	}
	switch cfg.Mode {
	case gui.ModeTray:
		if cfg.Mac.ActivationPolicy == gui.ActivationPolicyRegular {
			cfg.Mac.ActivationPolicy = gui.ActivationPolicyAccessory
		}
		cfg.Windows.DisableQuitOnLastWindowClosed = true
	case gui.ModeMultiWindow:
		cfg.Windows.DisableQuitOnLastWindowClosed = true
	case gui.ModeSingleWindow:
		cfg.Mac.ApplicationShouldTerminateAfterLastWindowClosed = true
	}
}

func registerCoreGUIServices(c *core.Core, app *application.App, cfg gui.GuiConfig) core.Result {
	options := gui.BootstrapWithConfig(app, gui.BootstrapConfig{
		Chat:      cfg.Chat,
		Container: cfg.Container,
		P2P:       cfg.P2P,
	})
	if len(options) != 20 {
		return core.Fail(core.E("desktop.registerCoreGUIServices", "unexpected CoreGUI bootstrap shape", nil))
	}
	options[1] = core.WithService(window.Register(newConfiguredWailsPlatform(app)))

	// CoreGUI's declarative bootstrap registers everything before the
	// lifecycle starts. The desktop Core is already live, so start each
	// newly registered service immediately and put display ahead of
	// window because the window startup queries display configuration.
	registrations := []struct {
		name  string
		index int
	}{
		{"display", 2},
		{"window", 1},
		{"webview", 3},
		{"menu", 4},
		{"systray", 5},
		{"browser", 6},
		{"notification", 7},
		{"lifecycle", 8},
		{"dialog", 9},
		{"contextmenu", 10},
		{"keybinding", 11},
		{"dock", 12},
		{"environment", 13},
		{"screen", 14},
		{"clipboard", 15},
		{"events", 16},
		{"chat", 17},
		{"container", 18},
		{"p2p", 19},
	}
	ctx := core.Background()
	for _, registration := range registrations {
		if c.Service(registration.name).OK {
			continue
		}
		if r := options[registration.index](c); !r.OK {
			return core.Fail(core.E(
				"desktop.registerCoreGUIServices",
				"register "+registration.name+" failed",
				r.Err(),
			))
		}
		service := c.Service(registration.name)
		if !service.OK {
			return core.Fail(core.E(
				"desktop.registerCoreGUIServices",
				"registered service "+registration.name+" is unavailable",
				service.Err(),
			))
		}
		if startable, ok := service.Value.(core.Startable); ok {
			if r := startable.OnStartup(ctx); !r.OK {
				return core.Fail(core.E(
					"desktop.registerCoreGUIServices",
					"start "+registration.name+" failed",
					r.Err(),
				))
			}
		}
	}
	return core.Ok(nil)
}

func applyGUIConfiguration(c *core.Core, cfg gui.GuiConfig) core.Result {
	if cfg.WindowStatePath != "" || cfg.WindowLayoutPath != "" {
		if service, ok := core.ServiceFor[*window.Service](c, "window"); ok && service != nil {
			if cfg.WindowStatePath != "" {
				service.Manager().State().SetPath(cfg.WindowStatePath)
			}
			if cfg.WindowLayoutPath != "" {
				service.Manager().Layout().SetPath(cfg.WindowLayoutPath)
			}
		}
	}

	for _, spec := range cfg.WindowRegistry {
		if spec == nil || spec.Name == "" {
			continue
		}
		if r := registerGUIWindow(c, spec); !r.OK {
			return r
		}
	}
	applyGUITray(c, cfg.Tray)
	if cfg.Tray != nil {
		applyGUITrayRoutes(c, cfg.Tray.Routes)
	}
	applyGUIKeybindings(c, cfg.Keybindings)
	applyGUIContextMenus(c, cfg.ContextMenus)
	applyGUIAppMenu(c, cfg.AppMenu)
	return core.Ok(nil)
}

func registerGUIWindow(c *core.Core, spec *window.Window) core.Result {
	ctx := core.Background()
	if r := c.Action("window.register").Run(ctx, core.NewOptions(
		core.Option{Key: "task", Value: window.TaskRegisterWindow{Window: spec, Kind: window.KindWebview}},
	)); !r.OK {
		return core.Fail(core.E("desktop.registerGUIWindow", "register "+spec.Name+" failed", r.Err()))
	}
	hidden := *spec
	hidden.Hidden = true
	if r := c.Action("window.open").Run(ctx, core.NewOptions(
		core.Option{Key: "task", Value: window.TaskOpenWindow{Window: &hidden}},
	)); !r.OK {
		return core.Fail(core.E("desktop.registerGUIWindow", "pre-create "+spec.Name+" failed", r.Err()))
	}
	if spec.HideOnClose {
		c.Action("window.set_close_behavior").Run(ctx, core.NewOptions(
			core.Option{Key: "task", Value: window.TaskSetCloseBehavior{
				Name:     spec.Name,
				Behavior: window.CloseBehaviorHide,
			}},
		))
	}
	if spec.ContentProtection {
		c.Action("window.set_content_protection").Run(ctx, core.NewOptions(
			core.Option{Key: "task", Value: window.TaskSetContentProtection{
				Name:       spec.Name,
				Protection: true,
			}},
		))
	}
	return core.Ok(nil)
}

func applyGUITray(c *core.Core, cfg *gui.TrayConfig) {
	if c == nil || cfg == nil {
		return
	}
	ctx := core.Background()
	if cfg.Icon != nil {
		task := any(systray.TaskSetTrayIcon{Data: cfg.Icon})
		action := "systray.set_icon"
		if cfg.IconTemplate && runtime.GOOS == "darwin" {
			task = systray.TaskSetTrayTemplateIcon{Data: cfg.Icon}
			action = "systray.set_template_icon"
		}
		c.Action(action).Run(ctx, core.NewOptions(core.Option{Key: "task", Value: task}))
	}
	c.Action("systray.set_tooltip").Run(ctx, core.NewOptions(
		core.Option{Key: "task", Value: systray.TaskSetTrayTooltip{Tooltip: cfg.Tooltip}},
	))
	c.Action("systray.set_label").Run(ctx, core.NewOptions(
		core.Option{Key: "task", Value: systray.TaskSetTrayLabel{Label: cfg.Label}},
	))
	if len(cfg.Menu) > 0 {
		c.Action("systray.set_menu").Run(ctx, core.NewOptions(
			core.Option{Key: "task", Value: systray.TaskSetTrayMenu{Items: cfg.Menu}},
		))
	}
	if cfg.PopoverWindow != "" {
		c.Action("systray.attach_window").Run(ctx, core.NewOptions(
			core.Option{Key: "task", Value: systray.TaskAttachWindow{
				Name:    cfg.PopoverWindow,
				OffsetY: cfg.PopoverOffsetY,
			}},
		))
	}
}

func applyGUITrayRoutes(c *core.Core, routes []gui.TrayRoute) {
	table := make(map[string]gui.TrayRoute, len(routes))
	for _, route := range routes {
		if route.ActionID != "" {
			table[route.ActionID] = route
		}
	}
	if len(table) == 0 {
		return
	}
	c.RegisterAction(func(_ *core.Core, message core.Message) core.Result {
		click, ok := message.(systray.ActionTrayMenuItemClicked)
		if !ok {
			return core.Ok(nil)
		}
		route, ok := table[click.ActionID]
		if !ok {
			return core.Ok(nil)
		}
		if route.OpenWindow != "" {
			gui.OpenWindow(c, route.OpenWindow)
		}
		if route.EmitEvent != "" {
			gui.EmitEvent(c, route.EmitEvent, route.OpenWindow)
		}
		if route.Quit {
			c.Action("lifecycle.quit").Run(core.Background(), core.NewOptions(
				core.Option{Key: "task", Value: lifecycle.TaskQuit{}},
			))
		}
		return core.Ok(nil)
	})
}

func applyGUIKeybindings(c *core.Core, bindings []gui.Keybinding) {
	if len(bindings) == 0 {
		return
	}
	eventByAccelerator := make(map[string]string, len(bindings))
	for _, binding := range bindings {
		if binding.Accelerator == "" {
			continue
		}
		eventByAccelerator[binding.Accelerator] = binding.EventName
		c.Action("keybinding.add").Run(core.Background(), core.NewOptions(
			core.Option{Key: "task", Value: keybinding.TaskAdd{
				Accelerator: binding.Accelerator,
				Description: binding.Description,
			}},
		))
	}
	c.RegisterAction(func(_ *core.Core, message core.Message) core.Result {
		triggered, ok := message.(keybinding.ActionTriggered)
		if !ok {
			return core.Ok(nil)
		}
		eventName := eventByAccelerator[triggered.Accelerator]
		if eventName == "" {
			return core.Ok(nil)
		}
		return c.Action("events.emit").Run(core.Background(), core.NewOptions(
			core.Option{Key: "task", Value: events.TaskEmit{Name: eventName, Data: ""}},
		))
	})
}

func applyGUIContextMenus(c *core.Core, menus []gui.ContextMenu) {
	if len(menus) == 0 {
		return
	}
	type relay struct {
		template    string
		prefixStrip string
	}
	relays := make(map[string]relay, len(menus))
	for _, item := range menus {
		if item.Name == "" {
			continue
		}
		relays[item.Name] = relay{template: item.EventTemplate, prefixStrip: item.MenuPrefixStrip}
		c.Action("contextmenu.add").Run(core.Background(), core.NewOptions(
			core.Option{Key: "task", Value: contextmenu.TaskAdd{
				Name: item.Name,
				Menu: contextmenu.ContextMenuDef{Name: item.Name, Items: item.Items},
			}},
		))
	}
	c.RegisterAction(func(_ *core.Core, message core.Message) core.Result {
		click, ok := message.(contextmenu.ActionItemClicked)
		if !ok {
			return core.Ok(nil)
		}
		entry, ok := relays[click.MenuName]
		if !ok || entry.template == "" {
			return core.Ok(nil)
		}
		menuName := core.TrimPrefix(click.MenuName, entry.prefixStrip)
		eventName := core.Replace(entry.template, "{menu}", menuName)
		eventName = core.Replace(eventName, "{action}", click.ActionID)
		return c.Action("events.emit").Run(core.Background(), core.NewOptions(
			core.Option{Key: "task", Value: events.TaskEmit{Name: eventName, Data: click.Data}},
		))
	})
}

func applyGUIAppMenu(c *core.Core, items []gui.MenuItem) {
	if runtime.GOOS != "darwin" || len(items) == 0 {
		return
	}
	c.Action("menu.set_app_menu").Run(core.Background(), core.NewOptions(
		core.Option{Key: "task", Value: menu.TaskSetAppMenu{Items: items}},
	))
}
