// SPDX-Licence-Identifier: EUPL-1.2

package desktop

import (
	"reflect"
	"unsafe"

	core "dappco.re/go"
	"dappco.re/go/render/display/webkit/pkg/preload"
	guiwindow "dappco.re/go/render/display/webkit/pkg/window"
	"dappco.re/lthn/desktop/pkg/appconfig"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// configuredWailsPlatform keeps CoreGUI's live-window adapter while ensuring
// the actual Wails constructor receives the complete appconfig option set.
type configuredWailsPlatform struct {
	app      *application.App
	delegate *guiwindow.WailsPlatform
}

func newConfiguredWailsPlatform(app *application.App) *configuredWailsPlatform {
	return &configuredWailsPlatform{
		app:      app,
		delegate: guiwindow.NewWailsPlatform(app),
	}
}

func (platform *configuredWailsPlatform) CreateWindow(
	source guiwindow.PlatformWindowOptions,
) guiwindow.PlatformWindow {
	if platform == nil || platform.app == nil || platform.app.Window == nil {
		return nil
	}
	if configuredWindowProfile(source.Name) == "" {
		return platform.delegate.CreateWindow(source)
	}

	var screen *application.Screen
	if source.ScreenID != "" && platform.app.Screen != nil {
		screen = platform.app.Screen.GetByID(source.ScreenID)
	}
	options := configuredWebviewWindowOptions(source, screen)
	window := platform.app.Window.NewWithOptions(options)
	subscribeConfiguredPreload(window, source.URL, source.JS)

	for _, candidate := range platform.delegate.GetWindows() {
		if candidate != nil && candidate.Name() == source.Name {
			return &configuredPlatformWindow{
				PlatformWindow: candidate,
				title:          source.Title,
				alwaysOnTop:    source.AlwaysOnTop,
			}
		}
	}
	return nil
}

func (platform *configuredWailsPlatform) GetWindows() []guiwindow.PlatformWindow {
	if platform == nil || platform.delegate == nil {
		return nil
	}
	return platform.delegate.GetWindows()
}

func (platform *configuredWailsPlatform) BindEvalReply(
	callback func(requestID string, result any, errorText string),
) {
	if platform == nil || platform.delegate == nil {
		return
	}
	platform.delegate.BindEvalReply(callback)
}

func (platform *configuredWailsPlatform) BindCustomEvent(
	name string,
	callback func(data any),
) {
	if platform == nil || platform.delegate == nil {
		return
	}
	platform.delegate.BindCustomEvent(name, callback)
}

type configuredPlatformWindow struct {
	guiwindow.PlatformWindow
	title       string
	alwaysOnTop bool
}

func (window *configuredPlatformWindow) Title() string {
	if window == nil {
		return ""
	}
	return window.title
}

func (window *configuredPlatformWindow) SetTitle(title string) {
	if window == nil || window.PlatformWindow == nil {
		return
	}
	window.title = title
	window.PlatformWindow.SetTitle(title)
}

func (window *configuredPlatformWindow) IsAlwaysOnTop() bool {
	return window != nil && window.alwaysOnTop
}

func (window *configuredPlatformWindow) SetAlwaysOnTop(alwaysOnTop bool) {
	if window == nil || window.PlatformWindow == nil {
		return
	}
	window.alwaysOnTop = alwaysOnTop
	window.PlatformWindow.SetAlwaysOnTop(alwaysOnTop)
}

func configuredWebviewWindowOptions(
	source guiwindow.PlatformWindowOptions,
	screen *application.Screen,
) application.WebviewWindowOptions {
	profile := configuredWindowProfile(source.Name)
	options := appconfig.WebviewWindowOptions(
		profile,
		source.Name,
		source.Title,
		source.URL,
	)

	options.HTML = source.HTML
	options.JS = source.JS
	options.CSS = source.CSS
	options.Width = source.Width
	options.Height = source.Height
	options.MinWidth = source.MinWidth
	options.MinHeight = source.MinHeight
	options.MaxWidth = source.MaxWidth
	options.MaxHeight = source.MaxHeight
	options.AlwaysOnTop = source.AlwaysOnTop
	options.DisableResize = source.DisableResize
	options.Frameless = source.Frameless
	options.StartState = application.WindowState(source.StartState)
	options.BackgroundType = application.BackgroundType(source.BackgroundType)
	options.BackgroundColour = application.NewRGBA(
		source.BackgroundColour[0],
		source.BackgroundColour[1],
		source.BackgroundColour[2],
		source.BackgroundColour[3],
	)
	options.X = source.X
	options.Y = source.Y
	options.InitialPosition = application.WindowCentered
	if source.X != 0 || source.Y != 0 {
		options.InitialPosition = application.WindowXY
	}
	options.Screen = screen
	options.Hidden = source.Hidden
	options.Zoom = source.Zoom
	options.ZoomControlEnabled = source.ZoomControlEnabled
	options.EnableFileDrop = source.EnableFileDrop
	options.Permissions = configuredPermissions(source.Permissions)
	options.OpenInspectorOnStartup = source.OpenInspectorOnStartup
	options.MinimiseButtonState = application.ButtonState(source.MinimiseButtonState)
	options.MaximiseButtonState = application.ButtonState(source.MaximiseButtonState)
	options.CloseButtonState = application.ButtonState(source.CloseButtonState)
	options.FullscreenButtonState = application.ButtonState(source.FullscreenButtonState)
	options.DevToolsEnabled = source.DevToolsEnabled
	options.DefaultContextMenuDisabled = source.DefaultContextMenuDisabled
	options.IgnoreMouseEvents = source.IgnoreMouseEvents
	options.ContentProtectionEnabled = source.ContentProtection
	options.HideOnFocusLost = source.HideOnFocusLost
	options.HideOnEscape = source.HideOnEscape
	options.UseApplicationMenu = source.UseApplicationMenu

	options.Mac.Backdrop = application.MacBackdrop(source.Mac.Backdrop)
	options.Mac.DisableShadow = source.Mac.DisableShadow
	options.Mac.InvisibleTitleBarHeight = source.Mac.InvisibleTitleBarHeight
	options.Mac.WindowLevel = application.MacWindowLevel(source.Mac.WindowLevel)
	options.Mac.CollectionBehavior =
		application.MacWindowCollectionBehavior(source.Mac.CollectionBehavior)
	options.Mac.TabbingMode = application.MacWindowTabbingMode(source.Mac.TabbingMode)
	options.Mac.DisableEscapeExitsFullscreen = source.Mac.DisableEscapeExitsFullscreen
	if source.Mac.DisableBackForwardNav {
		options.Mac.WebviewPreferences.AllowsBackForwardNavigationGestures.Set(false)
	}
	if source.Mac.EnableAutoplayWithoutUserAction {
		options.Mac.WebviewPreferences.EnableAutoplayWithoutUserAction.Set(true)
	}
	options.Mac.LiquidGlass.Style =
		application.MacLiquidGlassStyle(source.Mac.LiquidGlassStyle)
	options.Mac.LiquidGlass.Material =
		application.NSVisualEffectMaterial(source.Mac.LiquidGlassMaterial)
	options.Mac.LiquidGlass.CornerRadius = source.Mac.LiquidGlassCornerRadius
	options.Mac.LiquidGlass.GroupID = source.Mac.LiquidGlassGroupID
	options.Mac.LiquidGlass.GroupSpacing = source.Mac.LiquidGlassGroupSpacing
	if source.Mac.LiquidGlassTintColour != nil {
		colour := application.NewRGBA(
			source.Mac.LiquidGlassTintColour[0],
			source.Mac.LiquidGlassTintColour[1],
			source.Mac.LiquidGlassTintColour[2],
			source.Mac.LiquidGlassTintColour[3],
		)
		options.Mac.LiquidGlass.TintColor = &colour
	}

	options.Windows.HiddenOnTaskbar = source.Windows.HiddenOnTaskbar
	options.Windows.DisableMenu = source.Windows.DisableMenu
	options.Windows.Theme = application.Theme(source.Windows.Theme)
	options.Windows.NonClientRegionSupport =
		source.Windows.NonClientRegionSupport
	options.Windows.WebView2CompositionHosting =
		source.Windows.WebView2CompositionHosting
	if source.Windows.WindowDidMoveDebounceMS != 0 {
		options.Windows.WindowDidMoveDebounceMS =
			source.Windows.WindowDidMoveDebounceMS
	}

	options.Linux.Icon = source.Linux.Icon
	return options
}

func configuredWindowProfile(name string) string {
	switch {
	case name == mainWindowName:
		return "main"
	case name == trayPanelWindowName:
		return "tray-popover"
	case core.HasPrefix(name, nativeAppWindowNamePrefix):
		return "tear-off"
	default:
		return ""
	}
}

func subscribeConfiguredPreload(
	window *application.WebviewWindow,
	origin string,
	windowJS string,
) {
	if window == nil {
		return
	}
	handler := func(_ *application.WindowEvent) {
		unblockConfiguredWailsRuntime(window)
		if err := preload.InjectPreload(window, origin); err != nil {
			return
		}
		if extra := configuredPostPageLoadWindowJS(windowJS); core.Trim(extra) != "" {
			window.ExecJS(extra)
		}
	}
	for _, eventType := range []events.WindowEventType{
		events.Mac.WebViewDidFinishNavigation,
		events.Windows.WebViewNavigationCompleted,
		events.Linux.WindowLoadFinished,
	} {
		window.OnWindowEvent(eventType, handler)
	}
}

func unblockConfiguredWailsRuntime(window *application.WebviewWindow) {
	if window == nil {
		return
	}
	value := reflect.ValueOf(window)
	if value.Kind() != reflect.Pointer || value.IsNil() {
		return
	}
	structValue := value.Elem()
	if structValue.Kind() != reflect.Struct {
		return
	}
	runtimeLoaded := structValue.FieldByName("runtimeLoaded")
	if !runtimeLoaded.IsValid() || runtimeLoaded.Kind() != reflect.Bool {
		return
	}
	runtimeLoadedAddress := unsafe.Pointer(runtimeLoaded.UnsafeAddr())
	runtimeLoadedWritable := reflect.NewAt(
		runtimeLoaded.Type(),
		runtimeLoadedAddress,
	).Elem()
	if runtimeLoadedWritable.Bool() {
		return
	}
	runtimeLoadedWritable.SetBool(true)

	pendingJS := structValue.FieldByName("pendingJS")
	if !pendingJS.IsValid() || pendingJS.Kind() != reflect.Slice {
		return
	}
	pendingJSAddress := unsafe.Pointer(pendingJS.UnsafeAddr())
	pendingJSWritable := reflect.NewAt(
		pendingJS.Type(),
		pendingJSAddress,
	).Elem()
	pending := make([]string, pendingJSWritable.Len())
	for index := 0; index < pendingJSWritable.Len(); index++ {
		pending[index] = pendingJSWritable.Index(index).String()
	}
	pendingJSWritable.SetLen(0)
	for _, script := range pending {
		window.ExecJS(script)
	}
}

func configuredPostPageLoadWindowJS(raw string) string {
	if configuredLegacyDisplayPreload(raw) {
		return ""
	}
	return raw
}

func configuredLegacyDisplayPreload(raw string) bool {
	trimmed := core.Trim(raw)
	if trimmed == "" {
		return false
	}
	return core.Contains(trimmed, "const __corePageURL =") &&
		core.Contains(trimmed, "globalThis.core.ml") &&
		core.Contains(trimmed, "Document.prototype, 'cookie'")
}

func configuredPermissions(
	source map[uint8]uint8,
) map[application.PermissionType]application.Permission {
	if len(source) == 0 {
		return nil
	}
	result := make(
		map[application.PermissionType]application.Permission,
		len(source),
	)
	for permissionType, permission := range source {
		result[application.PermissionType(permissionType)] =
			application.Permission(permission)
	}
	return result
}

var _ guiwindow.Platform = (*configuredWailsPlatform)(nil)
var _ guiwindow.EvalReplyBinder = (*configuredWailsPlatform)(nil)
var _ guiwindow.CustomEventBinder = (*configuredWailsPlatform)(nil)
