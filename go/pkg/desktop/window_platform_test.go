// SPDX-Licence-Identifier: EUPL-1.2

package desktop

import (
	core "dappco.re/go"
	guiwindow "dappco.re/go/render/display/webkit/pkg/window"
	"github.com/wailsapp/wails/v3/pkg/application"
)

func TestWindowPlatform_ConfiguredOptions_Good_TrayProfileApplied(t *core.T) {
	options := configuredWebviewWindowOptions(
		trayPanelWindowSpec().ToPlatformOptions(),
		nil,
	)

	core.AssertEqual(t, "tray-panel", options.Name)
	core.AssertEqual(t, 400, options.Width)
	core.AssertEqual(t, 560, options.Height)
	core.AssertTrue(t, options.AlwaysOnTop)
	core.AssertTrue(t, options.Hidden)
	core.AssertTrue(t, options.Windows.HiddenOnTaskbar)
	core.AssertEqual(t, uint16(50), options.Windows.WindowDidMoveDebounceMS)
	core.AssertEqual(t, uint16(0), options.Linux.WindowDidMoveDebounceMS)
	core.AssertFalse(t, options.AllowSimpleEventEmit)
	core.AssertFalse(t, options.Windows.GeneralAutofillEnabled)
	core.AssertFalse(t, options.Windows.PasswordAutosaveEnabled)
}

func TestWindowPlatform_ConfiguredOptions_Bad_UnknownWindowUsesSafeDefaults(t *core.T) {
	options := configuredWebviewWindowOptions(guiwindow.PlatformWindowOptions{
		Name:   "utility",
		Title:  "Utility",
		URL:    "/utility",
		Width:  720,
		Height: 480,
	}, nil)

	core.AssertEqual(t, 720, options.Width)
	core.AssertEqual(t, 480, options.Height)
	core.AssertFalse(t, options.AllowSimpleEventEmit)
	core.AssertNil(t, options.Permissions)
	core.AssertNil(t, options.Screen)
	core.AssertEqual(t, application.WindowCentered, options.InitialPosition)
}

func TestWindowPlatform_ConfiguredOptions_Ugly_SavedPositionTargetsScreen(t *core.T) {
	source := angularWindowSpec("app", "Lethean Desktop", "/#/").ToPlatformOptions()
	source.X = 120
	source.Y = 80
	screen := &application.Screen{}

	options := configuredWebviewWindowOptions(source, screen)

	core.AssertEqual(t, application.WindowXY, options.InitialPosition)
	core.AssertEqual(t, 120, options.X)
	core.AssertEqual(t, 80, options.Y)
	core.AssertEqual(t, screen, options.Screen)
}

// --- configuredWindowProfile -------------------------------------------

func TestWindowPlatform_ConfiguredWindowProfile_Good_TearOffPrefix(t *core.T) {
	core.AssertEqual(t, "tear-off", configuredWindowProfile(nativeAppWindowNamePrefix+"chat"))
}

// --- configuredPermissions -------------------------------------------------

func TestWindowPlatform_ConfiguredPermissions_Bad_EmptyReturnsNil(t *core.T) {
	core.AssertNil(t, configuredPermissions(nil))
}

func TestWindowPlatform_ConfiguredPermissions_Good_TranslatesEveryEntry(t *core.T) {
	result := configuredPermissions(map[uint8]uint8{0: 1, 3: 2})

	core.AssertEqual(t, 2, len(result))
	core.AssertEqual(t, application.Permission(1), result[application.PermissionType(0)])
	core.AssertEqual(t, application.Permission(2), result[application.PermissionType(3)])
}

// --- configuredLegacyDisplayPreload / configuredPostPageLoadWindowJS ------

func TestWindowPlatform_LegacyDisplayPreload_Bad_EmptyIsFalse(t *core.T) {
	core.AssertFalse(t, configuredLegacyDisplayPreload(""))
	core.AssertFalse(t, configuredLegacyDisplayPreload("   "))
}

func TestWindowPlatform_LegacyDisplayPreload_Good_MatchesAllThreeMarkers(t *core.T) {
	js := "const __corePageURL = 'x'; globalThis.core.ml(); Object.defineProperty(Document.prototype, 'cookie', {})"
	core.AssertTrue(t, configuredLegacyDisplayPreload(js))
}

func TestWindowPlatform_LegacyDisplayPreload_Ugly_PartialMarkersIsFalse(t *core.T) {
	core.AssertFalse(t, configuredLegacyDisplayPreload("const __corePageURL = 'x';"))
}

func TestWindowPlatform_PostPageLoadWindowJS_Good_PassesThroughOrdinaryJS(t *core.T) {
	core.AssertEqual(t, "window.foo = 1;", configuredPostPageLoadWindowJS("window.foo = 1;"))
}

func TestWindowPlatform_PostPageLoadWindowJS_Bad_LegacyPreloadIsStripped(t *core.T) {
	js := "const __corePageURL = 'x'; globalThis.core.ml(); Object.defineProperty(Document.prototype, 'cookie', {})"
	core.AssertEqual(t, "", configuredPostPageLoadWindowJS(js))
}

// --- configuredPlatformWindow (Title / SetTitle / IsAlwaysOnTop / SetAlwaysOnTop) ---
//
// guiwindow.MockPlatform / MockWindow are the exported test doubles the
// pkg/bridge suite uses for the same PlatformWindow interface (real
// services over mock platforms) — reused here so configuredPlatformWindow's
// own delegation logic is exercised without any Wails dependency.

func TestWindowPlatform_ConfiguredWindow_Bad_NilReceiverIsSafe(t *core.T) {
	var w *configuredPlatformWindow
	core.AssertEqual(t, "", w.Title())
	core.AssertFalse(t, w.IsAlwaysOnTop())
	w.SetTitle("ignored")
	w.SetAlwaysOnTop(true)
}

func TestWindowPlatform_ConfiguredWindow_Good_TitleTracksLocalField(t *core.T) {
	mockPlatform := guiwindow.NewMockPlatform()
	inner := mockPlatform.CreateWindow(guiwindow.PlatformWindowOptions{Name: "widget", Title: "Widget"})
	w := &configuredPlatformWindow{PlatformWindow: inner, title: "Widget"}

	core.AssertEqual(t, "Widget", w.Title())

	w.SetTitle("Renamed")

	core.AssertEqual(t, "Renamed", w.Title())
	core.AssertEqual(t, "Renamed", inner.Title(), "delegate's own title must follow the rename too")
}

func TestWindowPlatform_ConfiguredWindow_Good_SetTitleNilDelegateIsSafe(t *core.T) {
	w := &configuredPlatformWindow{title: "Widget"}
	w.SetTitle("Renamed")
	core.AssertEqual(t, "Widget", w.title, "no delegate — the local field must stay untouched")
}

func TestWindowPlatform_ConfiguredWindow_Ugly_AlwaysOnTopTracksLocalField(t *core.T) {
	mockPlatform := guiwindow.NewMockPlatform()
	inner := mockPlatform.CreateWindow(guiwindow.PlatformWindowOptions{Name: "widget"})
	w := &configuredPlatformWindow{PlatformWindow: inner}

	core.AssertFalse(t, w.IsAlwaysOnTop())

	w.SetAlwaysOnTop(true)

	core.AssertTrue(t, w.IsAlwaysOnTop())
	core.AssertTrue(t, inner.IsAlwaysOnTop())
}

func TestWindowPlatform_ConfiguredWindow_Ugly_SetAlwaysOnTopNilDelegateIsSafe(t *core.T) {
	w := &configuredPlatformWindow{alwaysOnTop: false}
	w.SetAlwaysOnTop(true)
	core.AssertFalse(t, w.alwaysOnTop, "no delegate — the local field must stay untouched")
}

// --- configuredWailsPlatform guard clauses ---------------------------------
//
// The success paths of CreateWindow / GetWindows / BindEvalReply /
// BindCustomEvent are exercised below against a real *application.App
// (testWailsApp, gui_runtime_test.go) — application.New() is never followed
// by app.Run(), so Wails v3 defers every window/event registration onto
// app.pendingRun instead of touching native platform code
// (window_manager.go's NewWithOptions -> runOrDeferToAppRun). That is the
// same seam already proven safe by the existing
// TestGUIRuntime_RegisterCoreGUIServices_* tests.

func TestWindowPlatform_WailsPlatform_Bad_NilPlatformCreateWindowIsSafe(t *core.T) {
	var platform *configuredWailsPlatform
	core.AssertNil(t, platform.CreateWindow(guiwindow.PlatformWindowOptions{Name: "app"}))
}

func TestWindowPlatform_WailsPlatform_Bad_NilAppCreateWindowIsSafe(t *core.T) {
	platform := &configuredWailsPlatform{}
	core.AssertNil(t, platform.CreateWindow(guiwindow.PlatformWindowOptions{Name: "app"}))
}

func TestWindowPlatform_WailsPlatform_Good_UnknownProfileDelegatesToWailsPlatform(t *core.T) {
	app := testWailsApp(t)
	platform := newConfiguredWailsPlatform(app)

	window := platform.CreateWindow(guiwindow.PlatformWindowOptions{
		Name: "an-unregistered-utility-window", Title: "Utility", URL: "/utility",
	})

	core.AssertNotNil(t, window)
}

func TestWindowPlatform_WailsPlatform_Good_KnownProfileBuildsConfiguredWindow(t *core.T) {
	app := testWailsApp(t)
	platform := newConfiguredWailsPlatform(app)

	window := platform.CreateWindow(guiwindow.PlatformWindowOptions{
		Name: trayPanelWindowName, Title: "Lethean Desktop", URL: "/#/tray",
	})

	core.RequireTrue(t, window != nil)
	_, ok := window.(*configuredPlatformWindow)
	core.AssertTrue(t, ok, "a profile-known window must be wrapped as configuredPlatformWindow")
	core.AssertEqual(t, "Lethean Desktop", window.Title())
}

func TestWindowPlatform_WailsPlatform_Good_KnownProfileWithScreenIDResolvesScreen(t *core.T) {
	app := testWailsApp(t)
	platform := newConfiguredWailsPlatform(app)

	window := platform.CreateWindow(guiwindow.PlatformWindowOptions{
		Name: trayPanelWindowName, Title: "Lethean Desktop", URL: "/#/tray", ScreenID: "does-not-exist",
	})

	core.AssertNotNil(t, window)
}

func TestWindowPlatform_WailsPlatform_Good_GetWindowsGuardsAndDelegates(t *core.T) {
	var nilPlatform *configuredWailsPlatform
	core.AssertNil(t, nilPlatform.GetWindows())

	app := testWailsApp(t)
	platform := newConfiguredWailsPlatform(app)
	core.AssertNotNil(t, platform.GetWindows())
}

func TestWindowPlatform_WailsPlatform_Good_BindEvalReplyGuardsAndDelegates(t *core.T) {
	var nilPlatform *configuredWailsPlatform
	nilPlatform.BindEvalReply(func(string, any, string) {})

	app := testWailsApp(t)
	platform := newConfiguredWailsPlatform(app)
	platform.BindEvalReply(func(string, any, string) {})
}

func TestWindowPlatform_WailsPlatform_Good_BindCustomEventGuardsAndDelegates(t *core.T) {
	var nilPlatform *configuredWailsPlatform
	nilPlatform.BindCustomEvent("lthn:test", func(any) {})

	app := testWailsApp(t)
	platform := newConfiguredWailsPlatform(app)
	platform.BindCustomEvent("lthn:test", func(any) {})
}

// --- subscribeConfiguredPreload / unblockConfiguredWailsRuntime -----------

func TestWindowPlatform_SubscribeConfiguredPreload_Bad_NilWindowIsSafe(t *core.T) {
	subscribeConfiguredPreload(nil, "/#/", "")
}

func TestWindowPlatform_SubscribeConfiguredPreload_Good_RegistersHandlersOnRealWindow(t *core.T) {
	app := testWailsApp(t)
	window := app.Window.NewWithOptions(application.WebviewWindowOptions{Name: "preload-target"})

	subscribeConfiguredPreload(window, "/#/", "window.foo = 1;")
}

func TestWindowPlatform_UnblockConfiguredWailsRuntime_Bad_NilWindowIsSafe(t *core.T) {
	unblockConfiguredWailsRuntime(nil)
}

func TestWindowPlatform_UnblockConfiguredWailsRuntime_Good_FlipsRuntimeLoadedAndDrainsPendingJS(t *core.T) {
	window := application.NewWindow(application.WebviewWindowOptions{Name: "runtime-unblock-target"})

	// unblockConfiguredWailsRuntime is idempotent — a second call once the
	// flag is already set must be a no-op rather than panic on the empty
	// pendingJS slice.
	unblockConfiguredWailsRuntime(window)
	unblockConfiguredWailsRuntime(window)
}
