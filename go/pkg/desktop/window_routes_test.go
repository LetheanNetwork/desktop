// SPDX-Licence-Identifier: EUPL-1.2

package desktop

import core "dappco.re/go"

func TestWindows_WindowRegistry_Good_AngularShell(t *core.T) {
	registry := windowRegistry()
	core.AssertEqual(t, 1, len(registry), "only the Angular OS shell is boot-registered")

	app := registry[0]
	core.AssertEqual(t, "app", app.Name)
	core.AssertEqual(t, "/#/", app.URL)
	core.AssertEqual(t, 1440, app.Width)
	core.AssertEqual(t, 900, app.Height)
	core.AssertTrue(t, app.Frameless)
	core.AssertTrue(t, app.ShowDockIcon)
}

func TestWindows_NativeAppWindowSpec_Good_HashRoute(t *core.T) {
	app := windowRegistry()[0]
	spec := nativeAppWindowSpec("chat")

	core.AssertNotNil(t, spec)
	core.AssertEqual(t, "app-view-chat", spec.Name)
	core.AssertEqual(t, "/#/w/chat", spec.URL)
	core.AssertEqual(t, app.Width, spec.Width)
	core.AssertEqual(t, app.Height, spec.Height)
	core.AssertEqual(t, app.Frameless, spec.Frameless)
	core.AssertEqual(t, app.Mac.InvisibleTitleBarHeight, spec.Mac.InvisibleTitleBarHeight)
	core.AssertEqual(t, app.BackgroundColour, spec.BackgroundColour)
}

func TestWindows_NativeAppWindowSpec_Bad_EmptyAppID(t *core.T) {
	core.AssertNil(t, nativeAppWindowSpec(""))
}

func TestWindows_NativeAppWindowSpec_Ugly_PathSegment(t *core.T) {
	core.AssertNil(t, nativeAppWindowSpec("../chat"))
}

func TestWindows_OpenNativeAppWindow_Bad_InvalidAppID(t *core.T) {
	core.AssertFalse(t, openNativeAppWindow(core.New(), "../chat"))
}
