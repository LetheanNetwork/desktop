// SPDX-Licence-Identifier: EUPL-1.2

// Package appconfig exposes the complete hard-coded Wails control surface.
// Consumers apply their runtime dependencies after taking these defaults.
package appconfig

import (
	"time"

	"dappco.re/go/config"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// ApplicationOptions returns Lethean Desktop's complete Wails application
// defaults. Runtime services, assets, callbacks and transport are overlaid by
// the desktop or mobile entrypoint after this value is returned.
func ApplicationOptions(configs ...*config.Service) application.Options {
	options := application.Options{
		// Name labels the application in Wails-owned operating-system UI.
		Name: "lthn",
		// Description supplies the default about-box description.
		Description: "Lethean Desktop",
		// Icon leaves the about-box icon available for the embedded app artwork.
		Icon: nil,
		// Mac makes the native application lifecycle policy explicit.
		Mac: application.MacOptions{
			// ActivationPolicy uses the standard application policy until tray mode overrides it.
			ActivationPolicy: application.ActivationPolicyRegular,
			// ApplicationShouldTerminateAfterLastWindowClosed preserves Wails' non-terminating default.
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},
		// Windows makes every process-wide WebView2 option explicit.
		Windows: application.WindowsOptions{
			// WndClass uses Wails' native window-class default.
			WndClass: "WailsWebviewWindow",
			// WndProcInterceptor leaves the Windows message loop unintercepted.
			WndProcInterceptor: nil,
			// DisableQuitOnLastWindowClosed preserves normal Wails lifecycle behaviour until tray mode overrides it.
			DisableQuitOnLastWindowClosed: false,
			// WebviewUserDataPath lets WebView2 use its standard per-binary data directory.
			WebviewUserDataPath: "",
			// WebviewBrowserPath selects the installed WebView2 runtime.
			WebviewBrowserPath: "",
			// EnabledFeatures starts with no process-wide WebView2 feature overrides.
			EnabledFeatures: nil,
			// DisabledFeatures starts with no process-wide WebView2 feature suppressions.
			DisabledFeatures: nil,
			// AdditionalBrowserArgs starts WebView2 without extra command-line switches.
			AdditionalBrowserArgs: nil,
			// UseVisualHosting preserves the standard HWND-child hosting mode.
			UseVisualHosting: false,
		},
		// Linux makes the GTK application lifecycle options explicit.
		Linux: application.LinuxOptions{
			// DisableQuitOnLastWindowClosed preserves Wails' normal Linux lifecycle.
			DisableQuitOnLastWindowClosed: false,
			// ProgramName lets GTK derive the process name as Wails normally does.
			ProgramName: "",
		},
		// IOS makes every WKWebView application option explicit.
		IOS: application.IOSOptions{
			// DisableInputAccessoryView keeps the standard keyboard accessory bar.
			DisableInputAccessoryView: false,
			// DisableScroll keeps native scrolling enabled.
			DisableScroll: false,
			// DisableBounce keeps native overscroll bounce enabled.
			DisableBounce: false,
			// DisableScrollIndicators keeps native scroll indicators visible.
			DisableScrollIndicators: false,
			// EnableBackForwardNavigationGestures preserves Wails' disabled gesture default.
			EnableBackForwardNavigationGestures: false,
			// DisableLinkPreview keeps iOS link previews enabled.
			DisableLinkPreview: false,
			// EnableInlineMediaPlayback preserves Wails' user-initiated media default.
			EnableInlineMediaPlayback: false,
			// EnableAutoplayWithoutUserAction keeps autoplay gesture-gated.
			EnableAutoplayWithoutUserAction: false,
			// DisableInspectable keeps the inspector available in development builds.
			DisableInspectable: false,
			// UserAgent lets WKWebView supply its standard user agent.
			UserAgent: "",
			// ApplicationNameForUserAgent lets Wails append its standard application token.
			ApplicationNameForUserAgent: "",
			// BackgroundColour preserves Wails' zero-value launch background.
			BackgroundColour: application.RGBA{},
			// EnableNativeTabs keeps the application-owned Angular navigation.
			EnableNativeTabs: false,
			// NativeTabsItems leaves the native tab catalogue empty.
			NativeTabsItems: nil,
		},
		// Android makes every Android WebView application option explicit.
		Android: application.AndroidOptions{
			// DisableScroll keeps native scrolling enabled.
			DisableScroll: false,
			// DisableOverscroll keeps the platform overscroll effect enabled.
			DisableOverscroll: false,
			// EnableZoom preserves Wails' disabled pinch-to-zoom default.
			EnableZoom: false,
			// UserAgent lets Android WebView supply its standard user agent.
			UserAgent: "",
			// BackgroundColour preserves Wails' zero-value launch background.
			BackgroundColour: application.RGBA{},
			// DisableHardwareAcceleration keeps hardware acceleration enabled.
			DisableHardwareAcceleration: false,
		},
		// Services starts empty and is replaced with the existing Wails binding list.
		Services: nil,
		// MarshalError uses Wails' standard binding-error serialiser.
		MarshalError: nil,
		// BindAliases declares no legacy binding identifiers.
		BindAliases: nil,
		// Logger lets Wails select its development or production logger.
		Logger: nil,
		// LogLevel uses slog's information-level zero value.
		LogLevel: 0,
		// Assets makes the complete asset-server surface explicit.
		Assets: application.AssetOptions{
			// Handler is supplied by the desktop Gin engine or mobile asset filesystem.
			Handler: nil,
			// Middleware is supplied by the desktop Wails/Gin routing bridge.
			Middleware: nil,
			// DisableLogging keeps Wails asset request logging enabled.
			DisableLogging: false,
		},
		// Flags starts without application-defined frontend flags.
		Flags: nil,
		// PanicHandler uses Wails' default panic reporting until the desktop event bridge is applied.
		PanicHandler: nil,
		// DisableDefaultSignalHandler keeps Wails' SIGINT and SIGTERM handling active.
		DisableDefaultSignalHandler: false,
		// KeyBindings starts without application-level Wails shortcuts.
		KeyBindings: nil,
		// OnShutdown starts without a pre-teardown callback.
		OnShutdown: nil,
		// PostShutdown starts without a post-teardown callback.
		PostShutdown: nil,
		// ShouldQuit lets Wails accept quit requests by default.
		ShouldQuit: nil,
		// RawMessageHandler leaves raw frontend messages on Wails' normal path.
		RawMessageHandler: nil,
		// WarningHandler lets Wails log warnings through its configured logger.
		WarningHandler: nil,
		// ErrorHandler lets Wails log errors through its configured logger.
		ErrorHandler: nil,
		// FileAssociations declares the packaged Lethean configuration document.
		FileAssociations: []string{".lthn"},
		// SingleInstance exposes the complete inter-instance hand-off contract.
		SingleInstance: &application.SingleInstanceOptions{
			// UniqueID matches the packaged application identifier on every desktop platform.
			UniqueID: "ai.lthn.desktop",
			// OnSecondInstanceLaunch is connected to the desktop deep-link router at runtime.
			OnSecondInstanceLaunch: nil,
			// AdditionalData is populated with the running application identity at runtime.
			AdditionalData: nil,
			// ExitCode preserves Wails' successful second-instance exit status.
			ExitCode: 0,
			// EncryptionKey is replaced with the installation-specific key at runtime.
			EncryptionKey: [32]byte{},
		},
		// Transport is replaced with Lethean's existing WebSocket transport.
		Transport: nil,
		// Server makes Wails' headless-server defaults explicit.
		Server: application.ServerOptions{
			// Host retains Wails' loopback-only security default.
			Host: "localhost",
			// Port retains Wails' default server port.
			Port: 8080,
			// ReadTimeout retains Wails' complete-request read deadline.
			ReadTimeout: 30 * time.Second,
			// WriteTimeout retains Wails' response write deadline.
			WriteTimeout: 30 * time.Second,
			// IdleTimeout retains Wails' keep-alive idle deadline.
			IdleTimeout: 120 * time.Second,
			// ShutdownTimeout retains Wails' graceful-shutdown deadline.
			ShutdownTimeout: 30 * time.Second,
			// TLS exposes its complete defaults while remaining disabled for HTTP compatibility.
			TLS: defaultTLSOptions(false),
		},
	}
	resolveApplicationOptions(firstConfig(configs), &options)
	return options
}

func defaultTLSOptions(enabled bool) *application.TLSOptions {
	options := &application.TLSOptions{
		// CertFile is the empty PEM certificate path until TLS is enabled.
		CertFile: "",
		// KeyFile is the empty PEM private-key path until TLS is enabled.
		KeyFile: "",
	}
	if !enabled {
		return nil
	}
	return options
}

// WebviewWindowOptions returns a complete Wails window configuration for the
// "main", "tray-popover", "tear-off" or "mobile" profile. Unknown profiles
// retain Wails' own window defaults and are suitable for ad-hoc windows.
func WebviewWindowOptions(
	profile, name, title, url string,
	configs ...*config.Service,
) application.WebviewWindowOptions {
	macWebviewPreferences := defaultMacWebviewPreferences()

	options := application.WebviewWindowOptions{
		// Name is the caller's stable native-window identifier.
		Name: name,
		// Title is the caller's operating-system window title.
		Title: title,
		// Width makes Wails' 800-pixel fallback width explicit.
		Width: 800,
		// Height makes Wails' 600-pixel fallback height explicit.
		Height: 600,
		// MinWidth leaves the native minimum unconstrained.
		MinWidth: 0,
		// MinHeight leaves the native minimum unconstrained.
		MinHeight: 0,
		// MaxWidth leaves the native maximum unconstrained.
		MaxWidth: 0,
		// MaxHeight leaves the native maximum unconstrained.
		MaxHeight: 0,
		// AlwaysOnTop keeps ordinary windows in the normal stacking order.
		AlwaysOnTop: false,
		// URL loads the caller's Angular or remote route.
		URL: url,
		// HTML leaves inline-document loading disabled.
		HTML: "",
		// JS starts without per-window JavaScript injection.
		JS: "",
		// CSS starts without per-window stylesheet injection.
		CSS: "",
		// DisableResize keeps native resizing enabled.
		DisableResize: false,
		// Frameless keeps normal native chrome unless a profile owns its chrome.
		Frameless: false,
		// StartState opens the window in its normal state.
		StartState: application.WindowStateNormal,
		// BackgroundType uses Wails' solid background default.
		BackgroundType: application.BackgroundTypeSolid,
		// BackgroundColour preserves Wails' zero-value RGBA background.
		BackgroundColour: application.RGBA{},
		// AllowSimpleEventEmit keeps the privileged inline event shortcut disabled.
		AllowSimpleEventEmit: false,
		// InitialPosition centres first-launch windows.
		InitialPosition: application.WindowCentered,
		// X is unused while the initial position is centred.
		X: 0,
		// Y is unused while the initial position is centred.
		Y: 0,
		// Screen lets the operating system select the launch display.
		Screen: nil,
		// Hidden shows ordinary windows unless their lifecycle pre-creates them hidden.
		Hidden: false,
		// Zoom preserves the native WebView zoom level.
		Zoom: 0,
		// ZoomControlEnabled keeps browser-style zoom shortcuts disabled.
		ZoomControlEnabled: false,
		// EnableFileDrop keeps operating-system file drops disabled by default.
		EnableFileDrop: false,
		// Permissions delegates every capability request to the platform default.
		Permissions: nil,
		// OpenInspectorOnStartup keeps developer tools closed on launch.
		OpenInspectorOnStartup: false,
		// MinimiseButtonState keeps the native minimise button enabled.
		MinimiseButtonState: application.ButtonEnabled,
		// MaximiseButtonState keeps the native maximise button enabled.
		MaximiseButtonState: application.ButtonEnabled,
		// CloseButtonState keeps the native close button enabled.
		CloseButtonState: application.ButtonEnabled,
		// FullscreenButtonState keeps the native fullscreen button enabled.
		FullscreenButtonState: application.ButtonEnabled,
		// DevToolsEnabled preserves Wails' build-tag-controlled developer-tools policy.
		DevToolsEnabled: false,
		// DefaultContextMenuDisabled keeps the standard WebView menu available by default.
		DefaultContextMenuDisabled: false,
		// KeyBindings starts without window-local Wails shortcuts.
		KeyBindings: nil,
		// IgnoreMouseEvents keeps the native window interactive.
		IgnoreMouseEvents: false,
		// ContentProtectionEnabled leaves operating-system capture protection disabled.
		ContentProtectionEnabled: false,
		// HideOnFocusLost keeps ordinary windows visible when focus changes.
		HideOnFocusLost: false,
		// HideOnEscape keeps ordinary windows visible when Escape is pressed.
		HideOnEscape: false,
		// UseApplicationMenu keeps Windows and Linux menus window-local by default.
		UseApplicationMenu: false,
		// Mac makes every AppKit and WKWebView window option explicit.
		Mac: application.MacWindow{
			// Backdrop uses the standard opaque AppKit window treatment.
			Backdrop: application.MacBackdropNormal,
			// DisableShadow keeps the native window shadow.
			DisableShadow: false,
			// TitleBar retains Wails' complete standard title-bar configuration.
			TitleBar: application.MacTitleBar{
				// AppearsTransparent keeps the title bar opaque.
				AppearsTransparent: false,
				// Hide keeps the title bar present.
				Hide: false,
				// HideTitle keeps the native title visible.
				HideTitle: false,
				// FullSizeContent keeps content below the title bar.
				FullSizeContent: false,
				// UseToolbar keeps the standard title bar instead of an NSToolbar.
				UseToolbar: false,
				// HideToolbarSeparator keeps the standard toolbar separator visible.
				HideToolbarSeparator: false,
				// ShowToolbarWhenFullscreen preserves the normal fullscreen toolbar policy.
				ShowToolbarWhenFullscreen: false,
				// ToolbarStyle lets AppKit select its automatic toolbar style.
				ToolbarStyle: application.MacToolbarStyleAutomatic,
			},
			// Appearance follows the current macOS system appearance.
			Appearance: application.DefaultAppearance,
			// InvisibleTitleBarHeight disables an extra invisible drag region.
			InvisibleTitleBarHeight: 0,
			// EventMapping lets Wails install its standard cross-platform mapping.
			EventMapping: nil,
			// EnableFraudulentWebsiteWarnings preserves Wails' disabled warning default.
			EnableFraudulentWebsiteWarnings: false,
			// WebviewPreferences delegates unset preferences to WebKit's native defaults.
			WebviewPreferences: macWebviewPreferences,
			// WindowLevel makes AppKit's normal effective level explicit.
			WindowLevel: application.MacWindowLevelNormal,
			// CollectionBehavior preserves Wails' fullscreen-primary compatibility default.
			CollectionBehavior: application.MacWindowCollectionBehaviorDefault,
			// TabbingMode preserves Wails' disallowed tabbing compatibility default.
			TabbingMode: application.MacWindowTabbingModeDefault,
			// LiquidGlass keeps the complete inactive Liquid Glass configuration explicit.
			LiquidGlass: application.MacLiquidGlass{
				// Style lets the system choose a style if Liquid Glass is later enabled.
				Style: application.LiquidGlassStyleAutomatic,
				// Material retains the zero-value appearance-based fallback material.
				Material: application.NSVisualEffectMaterialAppearanceBased,
				// CornerRadius keeps square fallback material bounds.
				CornerRadius: 0,
				// TintColor applies no custom glass tint.
				TintColor: nil,
				// GroupID leaves the glass surface ungrouped.
				GroupID: "",
				// GroupSpacing leaves grouped-glass spacing at zero.
				GroupSpacing: 0,
			},
			// DisableEscapeExitsFullscreen keeps standard macOS Escape behaviour.
			DisableEscapeExitsFullscreen: false,
		},
		// Windows makes every per-window WebView2 and Win32 option explicit.
		Windows: application.WindowsWindow{
			// BackdropType lets Windows select the appropriate backdrop.
			BackdropType: application.Auto,
			// DisableIcon keeps the native title-bar icon.
			DisableIcon: false,
			// Theme follows Windows system theme changes.
			Theme: application.SystemDefault,
			// CustomTheme leaves every optional native colour unset.
			CustomTheme: application.ThemeSettings{
				// DarkModeActive leaves active dark-window colours system-owned.
				DarkModeActive: nil,
				// DarkModeInactive leaves inactive dark-window colours system-owned.
				DarkModeInactive: nil,
				// LightModeActive leaves active light-window colours system-owned.
				LightModeActive: nil,
				// LightModeInactive leaves inactive light-window colours system-owned.
				LightModeInactive: nil,
				// DarkModeMenuBar leaves dark menu colours system-owned.
				DarkModeMenuBar: nil,
				// LightModeMenuBar leaves light menu colours system-owned.
				LightModeMenuBar: nil,
			},
			// DisableFramelessWindowDecorations retains native shadows and rounded corners.
			DisableFramelessWindowDecorations: false,
			// WindowMask leaves the native window rectangular.
			WindowMask: nil,
			// WindowMaskDraggable leaves mask-based dragging disabled.
			WindowMaskDraggable: false,
			// NonClientRegionSupport preserves Wails' disabled WebView2 non-client regions.
			NonClientRegionSupport: false,
			// WebView2CompositionHosting preserves Wails' HWND-hosted controller.
			WebView2CompositionHosting: false,
			// WindowDidMoveDebounceMS makes Wails' effective 50-ms Windows move debounce explicit.
			WindowDidMoveDebounceMS: 50,
			// EventMapping lets Wails install its standard cross-platform mapping.
			EventMapping: nil,
			// HiddenOnTaskbar keeps ordinary windows visible on the taskbar.
			HiddenOnTaskbar: false,
			// EnableSwipeGestures preserves Wails' disabled swipe default.
			EnableSwipeGestures: false,
			// Menu leaves the native per-window menu unset.
			Menu: nil,
			// DisableMenu keeps menu support available.
			DisableMenu: false,
			// Permissions delegates WebView2 permission requests to its standard policy.
			Permissions: nil,
			// ExStyle lets Wails derive the Win32 extended style.
			ExStyle: 0,
			// GeneralAutofillEnabled preserves Wails' disabled autofill default.
			GeneralAutofillEnabled: false,
			// PasswordAutosaveEnabled preserves Wails' disabled password-saving default.
			PasswordAutosaveEnabled: false,
		},
		// Linux makes every GTK and WebKitGTK option explicit.
		Linux: application.LinuxWindow{
			// Icon leaves the platform window icon unset.
			Icon: nil,
			// WindowIsTranslucent keeps the GTK window opaque.
			WindowIsTranslucent: false,
			// WebviewGpuPolicy keeps WebKitGTK hardware acceleration enabled.
			WebviewGpuPolicy: application.WebviewGpuPolicyAlways,
			// WindowDidMoveDebounceMS preserves Wails' immediate Linux move events.
			WindowDidMoveDebounceMS: 0,
			// Menu leaves the native per-window menu unset.
			Menu: nil,
			// MenuStyle uses the traditional GTK menu bar.
			MenuStyle: application.LinuxMenuStyleMenuBar,
		},
	}

	switch profile {
	case "main", "tear-off":
		applyAngularWindowProfile(&options)
	case "tray-popover":
		applyTrayPopoverProfile(&options)
	case "mobile":
		// Mobile keeps the complete Wails defaults and applies only entrypoint-owned colour overrides.
	}
	resolveWindowOptions(firstConfig(configs), profile, &options)
	return options
}

func defaultMacWebviewPreferences() application.MacWebviewPreferences {
	result := application.MacWebviewPreferences{
		// ApplicationNameForUserAgent lets Wails append its standard user-agent token.
		ApplicationNameForUserAgent: "",
	}
	// TabFocusesLinks delegates keyboard focus policy to WebKit.
	result.TabFocusesLinks.Unset()
	// TextInteractionEnabled delegates text selection policy to WebKit.
	result.TextInteractionEnabled.Unset()
	// FullscreenEnabled delegates element-fullscreen policy to WebKit.
	result.FullscreenEnabled.Unset()
	// AllowsBackForwardNavigationGestures delegates swipe navigation policy to WebKit.
	result.AllowsBackForwardNavigationGestures.Unset()
	// AllowsMagnification delegates pinch magnification policy to WebKit.
	result.AllowsMagnification.Unset()
	// AllowsAirPlayForMediaPlayback delegates AirPlay policy to WebKit.
	result.AllowsAirPlayForMediaPlayback.Unset()
	// JavaScriptCanOpenWindowsAutomatically delegates scripted-window policy to WebKit.
	result.JavaScriptCanOpenWindowsAutomatically.Unset()
	// MinimumFontSize delegates the minimum font size to WebKit.
	result.MinimumFontSize.Unset()
	// EnableAutoplayWithoutUserAction delegates autoplay policy to WebKit.
	result.EnableAutoplayWithoutUserAction.Unset()
	return result
}

func applyAngularWindowProfile(options *application.WebviewWindowOptions) {
	// Width preserves the existing Angular desktop shell width.
	options.Width = 1440
	// Height preserves the existing Angular desktop shell height.
	options.Height = 900
	// MinWidth preserves the existing usable Angular shell minimum.
	options.MinWidth = 1000
	// MinHeight preserves the existing usable Angular shell minimum.
	options.MinHeight = 680
	// Frameless preserves the existing Angular-painted title bar.
	options.Frameless = true
	// EnableFileDrop preserves native file drops on app surfaces.
	options.EnableFileDrop = true
	// DefaultContextMenuDisabled preserves Angular-owned context menus.
	options.DefaultContextMenuDisabled = true
	// InvisibleTitleBarHeight matches the Angular bar's own height so the two
	// agree. The bar also declares --wails-draggable, which is what lets its
	// buttons opt out with no-drag; without that the window controls would
	// start a window move instead of firing.
	options.Mac.InvisibleTitleBarHeight = 36
}

func applyTrayPopoverProfile(options *application.WebviewWindowOptions) {
	// Width fixes the compact tray surface at 400 pixels.
	options.Width = 400
	// Height fixes the compact tray surface at 560 pixels.
	options.Height = 560
	// MinWidth prevents the tray surface becoming narrower.
	options.MinWidth = 400
	// MinHeight prevents the tray surface becoming shorter.
	options.MinHeight = 560
	// MaxWidth prevents the tray surface becoming wider.
	options.MaxWidth = 400
	// MaxHeight prevents the tray surface becoming taller.
	options.MaxHeight = 560
	// AlwaysOnTop keeps the tray popover above ordinary windows.
	options.AlwaysOnTop = true
	// DisableResize keeps the popover at its anchored size.
	options.DisableResize = true
	// Frameless preserves the popover's application-painted surface.
	options.Frameless = true
	// BackgroundType preserves the transparent tray-popover background.
	options.BackgroundType = application.BackgroundTypeTransparent
	// Hidden pre-creates the warm WebView without showing it.
	options.Hidden = true
	// DefaultContextMenuDisabled preserves Angular-owned context menus.
	options.DefaultContextMenuDisabled = true
	// HideOnFocusLost dismisses the transient popover on outside clicks.
	options.HideOnFocusLost = true
	// HideOnEscape dismisses the transient popover from the keyboard.
	options.HideOnEscape = true
	// WindowLevel anchors the surface at the native pop-up-menu level.
	options.Mac.WindowLevel = application.MacWindowLevelPopUpMenu
	// CollectionBehavior keeps the popover available across spaces and fullscreen.
	options.Mac.CollectionBehavior = application.MacWindowCollectionBehaviorCanJoinAllSpaces |
		application.MacWindowCollectionBehaviorFullScreenAuxiliary |
		application.MacWindowCollectionBehaviorIgnoresCycle
	// AllowsBackForwardNavigationGestures explicitly disables navigation swipes in the popover.
	options.Mac.WebviewPreferences.AllowsBackForwardNavigationGestures.Set(false)
	// HiddenOnTaskbar keeps the transient popover out of the Windows taskbar.
	options.Windows.HiddenOnTaskbar = true
}
