// SPDX-Licence-Identifier: EUPL-1.2

package desktop

import core "dappco.re/go"

func TestWindows_WindowRegistry_Good_AngularShell(t *core.T) {
	registry := windowRegistry()
	core.AssertEqual(t, 2, len(registry), "the desktop shell and tray panel are boot-registered")

	app := registry[0]
	core.AssertEqual(t, "app", app.Name)
	core.AssertEqual(t, "/#/", app.URL)
	core.AssertEqual(t, 1440, app.Width)
	core.AssertEqual(t, 900, app.Height)
	core.AssertTrue(t, app.Frameless)
	core.AssertTrue(t, app.ShowDockIcon)
}

func TestWindows_WindowRegistry_Good_UserConfigWinsProfileDefault(t *core.T) {
	_, cfg := guiRuntimeConfigFixture(t)
	core.RequireTrue(t, cfg.Set("desktop.wails.window.main.width", 1280).OK)
	core.RequireTrue(t, cfg.Set("desktop.wails.window.tray_popover.height", 640).OK)

	registry := windowRegistry(cfg)

	core.AssertEqual(t, 1280, registry[0].Width)
	core.AssertEqual(t, 640, registry[1].Height)
}

func TestWindows_TrayPanelWindowSpec_Good_Popover(t *core.T) {
	panel := windowRegistry()[1]

	core.AssertEqual(t, "tray-panel", panel.Name)
	core.AssertEqual(t, "/#/tray", panel.URL)
	core.AssertEqual(t, 400, panel.Width)
	core.AssertEqual(t, 560, panel.Height)
	core.AssertTrue(t, panel.Hidden)
	core.AssertTrue(t, panel.AlwaysOnTop)
	core.AssertTrue(t, panel.DisableResize)
	core.AssertTrue(t, panel.HideOnClose)
	core.AssertTrue(t, panel.HideOnEscape)
	core.AssertTrue(t, panel.HideOnFocusLost)
	core.AssertTrue(t, panel.Windows.HiddenOnTaskbar)
	core.AssertFalse(t, panel.ShowDockIcon)
}

func TestWindows_NativeAppWindowSpec_Good_HashRoute(t *core.T) {
	app := windowRegistry()[0]
	spec := nativeAppWindowSpec(AppWindowRequest{App: "chat"})

	core.AssertNotNil(t, spec)
	core.AssertEqual(t, "app-view-chat", spec.Name)
	core.AssertEqual(t, "/#/w/chat", spec.URL)
	// The shell's native chrome, painted by the same Angular; the size is one
	// application's, not the whole desktop's.
	core.AssertEqual(t, app.Frameless, spec.Frameless)
	core.AssertEqual(t, app.BackgroundColour, spec.BackgroundColour)
	core.AssertEqual(t, 900, spec.Width)
	core.AssertEqual(t, 640, spec.Height)
	// The one place the tear-off parts from the shell's chrome: its title bar
	// is a row of controls rather than a bar, so the strip is left to the
	// DOM-aware --wails-draggable, which honours their no-drag. The shell's
	// coarser invisible title bar would start a window drag on every press.
	core.AssertEqual(t, 36, app.Mac.InvisibleTitleBarHeight)
	core.AssertEqual(t, 0, spec.Mac.InvisibleTitleBarHeight)
}

// A carried size below the window's own declared minimum never arrives: the
// platform resolves the contradiction in the minimum's favour, which is how a
// 660x400 application opened at desktop size. The tear-off window's floor is
// therefore the smallest size the binding will carry.
func TestWindows_NativeAppWindowSpec_Good_MinimumIsOneApplication(t *core.T) {
	shell := windowRegistry()[0]
	spec := nativeAppWindowSpec(AppWindowRequest{App: "telemetry", Width: 660, Height: 400})

	core.AssertNotNil(t, spec)
	core.AssertEqual(t, 660, spec.Width)
	core.AssertEqual(t, 400, spec.Height)
	core.AssertEqual(t, minimumTearOffDimension, spec.MinWidth)
	core.AssertEqual(t, minimumTearOffDimension, spec.MinHeight)
	core.AssertTrue(
		t,
		spec.MinWidth <= spec.Width && spec.MinHeight <= spec.Height,
		"a window cannot open smaller than the minimum it declares",
	)
	core.AssertEqual(t, 1000, shell.MinWidth, "the desktop shell keeps its own usable minimum")
	core.AssertEqual(t, 680, shell.MinHeight)
}

func TestWindows_NativeAppWindowSpec_Good_PaneRoute(t *core.T) {
	spec := nativeAppWindowSpec(AppWindowRequest{App: "control", Pane: "models"})

	core.AssertNotNil(t, spec)
	core.AssertEqual(t, "app-view-control", spec.Name)
	core.AssertEqual(t, "/#/w/control/models", spec.URL)
}

func TestWindows_NativeAppWindowSpec_Good_CarriedGeometry(t *core.T) {
	spec := nativeAppWindowSpec(AppWindowRequest{App: "chat", Width: 720, Height: 520})

	core.AssertNotNil(t, spec)
	core.AssertEqual(t, 720, spec.Width)
	core.AssertEqual(t, 520, spec.Height)
}

func TestWindows_NativeAppWindowSpec_Bad_EmptyAppID(t *core.T) {
	core.AssertNil(t, nativeAppWindowSpec(AppWindowRequest{}))
}

func TestWindows_NativeAppWindowSpec_Ugly_PathSegment(t *core.T) {
	core.AssertNil(t, nativeAppWindowSpec(AppWindowRequest{App: "../chat"}))
}
