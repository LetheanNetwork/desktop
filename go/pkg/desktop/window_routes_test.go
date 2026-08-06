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
	core.AssertEqual(t, app.Width, spec.Width)
	core.AssertEqual(t, app.Height, spec.Height)
	core.AssertEqual(t, app.Frameless, spec.Frameless)
	core.AssertEqual(t, app.Mac.InvisibleTitleBarHeight, spec.Mac.InvisibleTitleBarHeight)
	core.AssertEqual(t, app.BackgroundColour, spec.BackgroundColour)
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
