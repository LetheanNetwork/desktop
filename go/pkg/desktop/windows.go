// SPDX-Licence-Identifier: EUPL-1.2

// Native-window declarations for the Angular desktop OS.
//
// The boot registry contains the main OS shell at /#/ and its compact tray
// panel at /#/tray. Application windows are owned by Angular + NgRx and render
// inside the shell. A single parameterised ad-hoc opener remains for future
// native tear-off windows; those load Angular's standalone app route at
// /#/w/<appId>.

package desktop

import (
	core "dappco.re/go"
	"dappco.re/go/config"
	gui "dappco.re/go/render/display/webkit"
	guiwindow "dappco.re/go/render/display/webkit/pkg/window"
	"dappco.re/lthn/desktop/pkg/appconfig"
	"dappco.re/lthn/desktop/pkg/paths"
	"github.com/wailsapp/wails/v3/pkg/application"
)

const (
	mainWindowName            = "app"
	angularShellRoute         = "/#/"
	trayPanelWindowName       = "tray-panel"
	trayPanelRoute            = "/#/tray"
	nativeAppWindowNamePrefix = "app-view-"
	nativeAppRoutePrefix      = "/#/w/"
)

// WindowSpec is an alias for the core/gui window type. Kept as a
// named local alias so the window declarations read as lthn-specific
// data, while the underlying type + lifecycle is owned by core/gui.
type WindowSpec = guiwindow.Window

// angularWindowSpec returns the shared native chrome for both the main
// OS shell and a future native app tear-off. Angular paints the visible
// titlebar, so the native windows stay frameless and transparent.
func angularWindowSpec(
	name, title, route string,
	configs ...*config.Service,
) *WindowSpec {
	profile := "tear-off"
	if name == mainWindowName {
		profile = "main"
	}
	spec := windowSpecFromWebviewOptions(
		appconfig.WebviewWindowOptions(
			profile,
			name,
			title,
			frontendWindowURL(route),
			configs...,
		),
	)
	spec.HideOnClose = true
	spec.ShowDockIcon = true
	return spec
}

// trayPanelWindowSpec returns the compact, single-screen surface attached to
// the system tray. It is pre-created hidden so Wails can anchor it during app
// startup, then the platform reveals and positions the same warm WebView on
// each tray click.
func trayPanelWindowSpec(configs ...*config.Service) *WindowSpec {
	spec := windowSpecFromWebviewOptions(appconfig.WebviewWindowOptions(
		"tray-popover",
		trayPanelWindowName,
		"Lethean Desktop",
		frontendWindowURL(trayPanelRoute),
		configs...,
	))
	spec.HideOnClose = true
	return spec
}

func windowSpecFromWebviewOptions(options application.WebviewWindowOptions) *WindowSpec {
	permissions := make(map[uint8]uint8, len(options.Permissions))
	for permissionType, permission := range options.Permissions {
		permissions[uint8(permissionType)] = uint8(permission)
	}
	if len(permissions) == 0 {
		permissions = nil
	}

	var liquidGlassTint *[4]uint8
	if options.Mac.LiquidGlass.TintColor != nil {
		colour := options.Mac.LiquidGlass.TintColor
		liquidGlassTint = &[4]uint8{colour.Red, colour.Green, colour.Blue, colour.Alpha}
	}

	backForwardNavigation := options.Mac.WebviewPreferences.AllowsBackForwardNavigationGestures
	autoplay := options.Mac.WebviewPreferences.EnableAutoplayWithoutUserAction

	return &WindowSpec{
		Name:                       options.Name,
		Title:                      options.Title,
		URL:                        options.URL,
		HTML:                       options.HTML,
		JS:                         options.JS,
		CSS:                        options.CSS,
		Width:                      options.Width,
		Height:                     options.Height,
		X:                          options.X,
		Y:                          options.Y,
		MinWidth:                   options.MinWidth,
		MinHeight:                  options.MinHeight,
		MaxWidth:                   options.MaxWidth,
		MaxHeight:                  options.MaxHeight,
		Frameless:                  options.Frameless,
		Hidden:                     options.Hidden,
		AlwaysOnTop:                options.AlwaysOnTop,
		BackgroundColour:           [4]uint8{options.BackgroundColour.Red, options.BackgroundColour.Green, options.BackgroundColour.Blue, options.BackgroundColour.Alpha},
		DisableResize:              options.DisableResize,
		EnableFileDrop:             options.EnableFileDrop,
		HideOnEscape:               options.HideOnEscape,
		HideOnFocusLost:            options.HideOnFocusLost,
		DefaultContextMenuDisabled: options.DefaultContextMenuDisabled,
		StartState:                 int(options.StartState),
		BackgroundType:             int(options.BackgroundType),
		ScreenID:                   "",
		Zoom:                       options.Zoom,
		ZoomControlEnabled:         options.ZoomControlEnabled,
		Permissions:                permissions,
		OpenInspectorOnStartup:     options.OpenInspectorOnStartup,
		DevToolsEnabled:            options.DevToolsEnabled,
		MinimiseButtonState:        int(options.MinimiseButtonState),
		MaximiseButtonState:        int(options.MaximiseButtonState),
		CloseButtonState:           int(options.CloseButtonState),
		FullscreenButtonState:      int(options.FullscreenButtonState),
		IgnoreMouseEvents:          options.IgnoreMouseEvents,
		UseApplicationMenu:         options.UseApplicationMenu,
		ContentProtection:          options.ContentProtectionEnabled,
		Mac: guiwindow.MacWindow{
			WindowLevel:                     guiwindow.MacWindowLevel(options.Mac.WindowLevel),
			CollectionBehavior:              guiwindow.MacCollectionBehavior(options.Mac.CollectionBehavior),
			InvisibleTitleBarHeight:         options.Mac.InvisibleTitleBarHeight,
			DisableBackForwardNav:           backForwardNavigation.IsSet() && !backForwardNavigation.Get(),
			Backdrop:                        int(options.Mac.Backdrop),
			DisableShadow:                   options.Mac.DisableShadow,
			TabbingMode:                     int(options.Mac.TabbingMode),
			DisableEscapeExitsFullscreen:    options.Mac.DisableEscapeExitsFullscreen,
			EnableAutoplayWithoutUserAction: autoplay.IsSet() && autoplay.Get(),
			LiquidGlassStyle:                int(options.Mac.LiquidGlass.Style),
			LiquidGlassMaterial:             int(options.Mac.LiquidGlass.Material),
			LiquidGlassCornerRadius:         options.Mac.LiquidGlass.CornerRadius,
			LiquidGlassTintColour:           liquidGlassTint,
			LiquidGlassGroupID:              options.Mac.LiquidGlass.GroupID,
			LiquidGlassGroupSpacing:         options.Mac.LiquidGlass.GroupSpacing,
		},
		Linux: guiwindow.LinuxWindow{
			Icon: options.Linux.Icon,
		},
		Windows: guiwindow.WindowsWindow{
			HiddenOnTaskbar:            options.Windows.HiddenOnTaskbar,
			DisableMenu:                options.Windows.DisableMenu,
			Theme:                      int(options.Windows.Theme),
			NonClientRegionSupport:     options.Windows.NonClientRegionSupport,
			WebView2CompositionHosting: options.Windows.WebView2CompositionHosting,
			WindowDidMoveDebounceMS:    options.Windows.WindowDidMoveDebounceMS,
		},
	}
}

// windowRegistry returns the windows pre-created at application boot.
// The Angular desktop remains one OS shell; the only companion is the compact
// tray panel that Wails needs pre-created before it can attach the popover.
func windowRegistry(configs ...*config.Service) []*WindowSpec {
	return []*WindowSpec{
		angularWindowSpec(mainWindowName, "Lethean Desktop", angularShellRoute, configs...),
		trayPanelWindowSpec(configs...),
	}
}

// nativeAppWindowSpec builds the one supported native tear-off shape.
// App IDs are constrained to one safe route segment before they are
// used in either the native window name or Angular hash route.
func nativeAppWindowSpec(appID string, configs ...*config.Service) *WindowSpec {
	if !paths.IsValidPluginCode(appID) {
		return nil
	}
	return angularWindowSpec(
		nativeAppWindowNamePrefix+appID,
		"Lethean Desktop · "+appID,
		nativeAppRoutePrefix+appID,
		configs...,
	)
}

// openNativeAppWindow opens or focuses one Angular standalone app-view
// in a native frameless window. It is deliberately not wired to any
// menu, bridge, or drag event yet; it is the future tear-off seam.
func openNativeAppWindow(c *core.Core, appID string) bool {
	if c == nil {
		return false
	}
	configSvc, _ := core.ServiceFor[*config.Service](c, "config")
	spec := nativeAppWindowSpec(appID, configSvc)
	if spec == nil {
		return false
	}
	if !windowExists(c, spec.Name) {
		return gui.OpenAdhocWindow(c, spec)
	}

	ctx := core.Background()
	if spec.ShowDockIcon {
		c.Action("dock.show_icon").Run(ctx, core.NewOptions())
	}
	restore := c.Action("window.restore").Run(ctx, core.NewOptions(
		core.Option{Key: "task", Value: guiwindow.TaskRestore{Name: spec.Name}},
	))
	visible := c.Action("window.set_visibility").Run(ctx, core.NewOptions(
		core.Option{Key: "task", Value: guiwindow.TaskSetVisibility{Name: spec.Name, Visible: true}},
	))
	focus := c.Action("window.focus").Run(ctx, core.NewOptions(
		core.Option{Key: "task", Value: guiwindow.TaskFocus{Name: spec.Name}},
	))
	return restore.OK && visible.OK && focus.OK
}

// windowExists reports whether the named window is in CoreGUI's live
// window directory.
func windowExists(c *core.Core, name string) bool {
	return gui.WindowExists(c, name)
}
