// SPDX-Licence-Identifier: EUPL-1.2

// Native-window declarations for the Angular desktop OS.
//
// The boot registry contains only the main OS shell at /#/. Application
// windows are owned by Angular + NgRx and render inside that shell. A
// single parameterised ad-hoc opener remains for future native tear-off
// windows; those load Angular's standalone app route at /#/w/<appId>.

package desktop

import (
	core "dappco.re/go"
	gui "dappco.re/go/render/display/webkit"
	guiwindow "dappco.re/go/render/display/webkit/pkg/window"
	"dappco.re/lthn/desktop/pkg/paths"
)

const (
	mainWindowName            = "app"
	angularShellRoute         = "/#/"
	nativeAppWindowNamePrefix = "app-view-"
	nativeAppRoutePrefix      = "/#/w/"
	desktopTitleBarHeight     = 40
)

// WindowSpec is an alias for the core/gui window type. Kept as a
// named local alias so the window declarations read as lthn-specific
// data, while the underlying type + lifecycle is owned by core/gui.
type WindowSpec = guiwindow.Window

// angularWindowSpec returns the shared native chrome for both the main
// OS shell and a future native app tear-off. Angular paints the visible
// titlebar, so the native windows stay frameless and transparent.
func angularWindowSpec(name, title, route string) *WindowSpec {
	return &WindowSpec{
		Name: name, Title: title, URL: route,
		Width: 1440, Height: 900, MinWidth: 1000, MinHeight: 680,
		Frameless: true, HideOnClose: true, EnableFileDrop: true,
		ShowDockIcon:               true,
		DefaultContextMenuDisabled: true,
		BackgroundColour:           [4]uint8{0, 0, 0, 0},
		Mac: guiwindow.MacWindow{
			InvisibleTitleBarHeight: desktopTitleBarHeight,
		},
	}
}

// windowRegistry returns the windows pre-created at application boot.
// The Angular desktop is one OS shell, so no app surface, welcome
// wizard, or tray popover is registered as a separate native window.
func windowRegistry() []*WindowSpec {
	return []*WindowSpec{
		angularWindowSpec(mainWindowName, "Lethean Desktop", angularShellRoute),
	}
}

// nativeAppWindowSpec builds the one supported native tear-off shape.
// App IDs are constrained to one safe route segment before they are
// used in either the native window name or Angular hash route.
func nativeAppWindowSpec(appID string) *WindowSpec {
	if !paths.IsValidPluginCode(appID) {
		return nil
	}
	return angularWindowSpec(
		nativeAppWindowNamePrefix+appID,
		"Lethean Desktop · "+appID,
		nativeAppRoutePrefix+appID,
	)
}

// openNativeAppWindow opens or focuses one Angular standalone app-view
// in a native frameless window. It is deliberately not wired to any
// menu, bridge, or drag event yet; it is the future tear-off seam.
func openNativeAppWindow(c *core.Core, appID string) bool {
	spec := nativeAppWindowSpec(appID)
	if c == nil || spec == nil {
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
