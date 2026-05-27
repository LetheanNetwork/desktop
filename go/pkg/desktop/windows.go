// SPDX-Licence-Identifier: EUPL-1.2

// Multi-window support. Pre-declared window specs (name → config)
// get materialised on demand — tray menu / keyboard shortcut /
// lthn:// URL handler can open them by name without the frontend
// needing to know Wails internals.
//
// Lit element loading: each window opens at `/?surface=<name>` so
// the index.html SPA router mounts the matching element. The
// surface name is the same name used to retrieve the window
// from CoreGUI's window registry.

package desktop

import (
	core "dappco.re/go"
	gui "dappco.re/go/gui"
	guiwindow "dappco.re/go/gui/pkg/window"
)

// WindowSpec is an alias for the core/gui window type. Kept as a
// named local alias so the windowRegistry() reads as lthn-specific
// data, while the underlying type + lifecycle is owned by core/gui.
type WindowSpec = guiwindow.Window

// windowRegistry returns the named windows the app knows how to
// open. Today: chat (full chat surface), models (model browser),
// settings (preferences), about (about box).
//
// The slice is passed to gui.GuiConfig.WindowRegistry at boot;
// core/gui handles register + pre-create + HideOnClose / ContentProtection
// auto-wiring + state persistence via the window service.
func windowRegistry() []*WindowSpec {
	// All windows ship frameless — renderChrome() / lthn-app-shell
	// paint their own titlebar + traffic-lights, so the native macOS
	// chrome would be a second, redundant set (Snider confirmed in
	// the Lethean-6 handover: "frameless yes, rounded edges yes").
	// The body's border-radius:11px from renderChrome provides the
	// rounded card; window.BackgroundIsTransparent (set on app
	// construction in desktop.go) lets the rounded corners actually
	// show transparency at the four corners.
	// Shared per-window defaults — InvisibleTitleBarHeight matches
	// the 40px titlebar strip painted by renderChrome() so the OS
	// drag handler aligns exactly with the chrome's visible drag
	// region. DisableNativeContextMenu defers right-click to the
	// app's context menu registry (see contextmenus.go).
	const titleBarH = 40
	// Shared per-window defaults — InvisibleTitleBarHeight matches the
	// 40px titlebar strip painted by renderChrome() so the OS drag
	// handler aligns with the chrome's visible drag region.
	// DefaultContextMenuDisabled defers right-click to the app's own
	// context menu registry (see contextmenus.go).
	mac := guiwindow.MacWindow{InvisibleTitleBarHeight: titleBarH}
	return []*WindowSpec{
		{
			Name: "welcome", Title: "Welcome to lthn",
			URL: "/?surface=welcome",
			// Initial dimensions — the welcome wizard's Lit element
			// (frontend/src/lit/ops/welcome-window.ts) refines via
			// WindowService.SetSize on firstUpdated to match its own
			// declared `w`/`h`, so this value is the "before first
			// paint" floor rather than a hard cap.
			Width: 1200, Height: 800,
			Frameless: true, HideOnClose: true,
			DefaultContextMenuDisabled: true,
			BackgroundColour:           [4]uint8{0, 0, 0, 0},
			Mac:                        mac,
		},
		// `app` is the Lethean-6 unified application shell — single
		// window holding titlebar + grouped side-nav + body that
		// auto-mounts any <lthn-*-window>. Tray buttons open this with
		// ?pane=<id> set; the side-nav swaps panes internally thereafter.
		{
			Name: "app", Title: "Lethean Desktop",
			URL: "/?surface=app",
			// 1440×900 mirrors the MBP 14"+ retina viewport so the
			// unified shell breathes on most modern Macs without
			// spilling off small displays.
			Width: 1440, Height: 900, MinWidth: 1000, MinHeight: 680,
			Frameless: true, HideOnClose: true, EnableFileDrop: true,
			DefaultContextMenuDisabled: true,
			BackgroundColour:           [4]uint8{0, 0, 0, 0},
			Mac:                        mac,
		},
		{
			Name: "chat", Title: "Lethean Chat",
			URL:   "/?surface=chat",
			Width: 900, Height: 700, MinWidth: 600, MinHeight: 400,
			Frameless: true, HideOnClose: true, EnableFileDrop: true,
			DefaultContextMenuDisabled: true,
			BackgroundColour:           [4]uint8{0, 0, 0, 0},
			Mac:                        mac,
		},
		{
			Name: "models", Title: "Models",
			URL:   "/?surface=models",
			Width: 800, Height: 600, MinWidth: 500, MinHeight: 400,
			Frameless: true, HideOnClose: true, EnableFileDrop: true,
			DefaultContextMenuDisabled: true,
			BackgroundColour:           [4]uint8{0, 0, 0, 0},
			Mac:                        mac,
		},
		{
			Name: "settings", Title: "Settings",
			URL:   "/?surface=settings",
			Width: 700, Height: 550, MinWidth: 500, MinHeight: 400,
			Frameless: true, HideOnClose: true,
			DefaultContextMenuDisabled: true,
			BackgroundColour:           [4]uint8{0, 0, 0, 0},
			Mac:                        mac,
		},
		{
			Name: "telemetry", Title: "Telemetry",
			URL:   "/?surface=telemetry",
			Width: 880, Height: 560, MinWidth: 600, MinHeight: 400,
			Frameless: true, HideOnClose: true,
			DefaultContextMenuDisabled: true,
			BackgroundColour:           [4]uint8{0, 0, 0, 0},
			Mac:                        mac,
		},
		{
			Name: "editor", Title: "Editor",
			URL:   "/?surface=editor",
			Width: 1040, Height: 700, MinWidth: 600, MinHeight: 400,
			Frameless: true, HideOnClose: true, EnableFileDrop: true,
			DefaultContextMenuDisabled: true,
			BackgroundColour:           [4]uint8{0, 0, 0, 0},
			Mac:                        mac,
		},
		{
			Name: "git", Title: "Git",
			URL:   "/?surface=git",
			Width: 1120, Height: 740, MinWidth: 700, MinHeight: 450,
			Frameless: true, HideOnClose: true, EnableFileDrop: true,
			DefaultContextMenuDisabled: true,
			BackgroundColour:           [4]uint8{0, 0, 0, 0},
			Mac:                        mac,
		},
		{
			Name: "build", Title: "Build",
			URL:   "/?surface=build",
			Width: 1080, Height: 700, MinWidth: 700, MinHeight: 450,
			Frameless: true, HideOnClose: true, EnableFileDrop: true,
			DefaultContextMenuDisabled: true,
			BackgroundColour:           [4]uint8{0, 0, 0, 0},
			Mac:                        mac,
		},
		{
			Name: "lint", Title: "Lint",
			URL:   "/?surface=lint",
			Width: 1100, Height: 740, MinWidth: 700, MinHeight: 450,
			Frameless: true, HideOnClose: true, EnableFileDrop: true,
			DefaultContextMenuDisabled: true,
			BackgroundColour:           [4]uint8{0, 0, 0, 0},
			Mac:                        mac,
		},
		{
			Name: "containers", Title: "Containers",
			URL:   "/?surface=containers",
			Width: 1180, Height: 760, MinWidth: 720, MinHeight: 460,
			Frameless: true, HideOnClose: true,
			DefaultContextMenuDisabled: true,
			BackgroundColour:           [4]uint8{0, 0, 0, 0},
			Mac:                        mac,
		},
		{
			Name: "repos", Title: "Repos",
			URL:   "/?surface=repos",
			Width: 1180, Height: 760, MinWidth: 720, MinHeight: 460,
			Frameless: true, HideOnClose: true,
			DefaultContextMenuDisabled: true,
			BackgroundColour:           [4]uint8{0, 0, 0, 0},
			Mac:                        mac,
		},
		{
			Name: "php", Title: "PHP",
			URL:   "/?surface=php",
			Width: 1200, Height: 780, MinWidth: 760, MinHeight: 480,
			Frameless: true, HideOnClose: true,
			DefaultContextMenuDisabled: true,
			BackgroundColour:           [4]uint8{0, 0, 0, 0},
			Mac:                        mac,
		},
		{
			Name: "marketplace", Title: "Marketplace",
			URL:   "/?surface=marketplace",
			Width: 1180, Height: 760, MinWidth: 720, MinHeight: 460,
			Frameless: true, HideOnClose: true,
			DefaultContextMenuDisabled: true,
			BackgroundColour:           [4]uint8{0, 0, 0, 0},
			Mac:                        mac,
		},
		{
			Name: "ml-lab", Title: "ML Lab",
			URL:   "/?surface=ml-lab",
			Width: 1440, Height: 900, MinWidth: 1100, MinHeight: 660,
			Frameless: true, HideOnClose: true, EnableFileDrop: true,
			DefaultContextMenuDisabled: true,
			BackgroundColour:           [4]uint8{0, 0, 0, 0},
			Mac:                        mac,
		},
		{
			Name: "about", Title: "About Lethean Desktop",
			URL: "/?surface=about",
			// Genuinely fixed size — Min == Max == Width/Height +
			// DisableResize for the splash-card feel.
			Width: 420, Height: 320,
			MinWidth: 420, MinHeight: 320,
			MaxWidth: 420, MaxHeight: 320,
			Frameless: true, DisableResize: true,
			DefaultContextMenuDisabled: true,
			BackgroundColour:           [4]uint8{0, 0, 0, 0},
			Mac:                        mac,
		},
	}
}

// openWindow shows + focuses the named window. Backend-driven so
// tray menu items / keyboard shortcuts / lthn:// URL handlers can
// open windows without round-tripping through the frontend.
//
// If the name isn't in the registry, this is a no-op (returns
// silently — the caller's tray menu shouldn't have offered the
// option in the first place).
func openWindow(c *core.Core, name string) {
	// The unified `app` shell is the only window that warrants a Dock
	// presence — the tray-spawned "lite" cards stay in Accessory so
	// the menubar stays the single source of truth for them. Elevate
	// BEFORE the show actions so the activation policy is in place by
	// the time the window draws and the OS decides cmd+Tab eligibility.
	if name == "app" {
		setPolicyRegular()
	}
	gui.OpenWindow(c, name)
}

func openWindowHandle(app any, name string) {
	if typed, ok := app.(*core.Core); ok {
		openWindow(typed, name)
	}
}

func hideWindowHandle(app any, name string) bool {
	typed, ok := app.(*core.Core)
	if !ok || typed == nil {
		return false
	}
	return gui.HideWindow(typed, name)
}

func windowExists(c *core.Core, name string) bool {
	return gui.WindowExists(c, name)
}

// openPluginWindow creates (or re-shows) a CoreGUI window for one
// installed plugin. Each plugin code keys its own window — clicking
// the same plugin twice focuses the existing window rather than
// opening a duplicate, but plugins A and B can be open at the same
// time. Multi-window matches the rest of lthn's tray-driven UX.
//
// The URL is /?surface=plugin&code=<code>; main.ts dispatches into
// the plugin-window Lit element which renders an iframe pointed at
// /v1/api/plugin/<namespace>/<ui.entrypoint> when the manifest
// declares a UI surface, or a status card otherwise.
func openPluginWindow(c *core.Core, code string) {
	wName := "plugin-" + code
	if windowExists(c, wName) {
		openWindow(c, wName)
		return
	}
	r := c.Action("window.open").Run(core.Background(), core.NewOptions(
		core.Option{Key: "task", Value: guiwindow.TaskOpenWindow{Window: &guiwindow.Window{
			Name: wName, Title: "Plugin · " + code,
			URL:   "/?surface=plugin&code=" + code,
			Width: 1180, Height: 760, MinWidth: 720, MinHeight: 460,
			Frameless: true, Hidden: true,
			DefaultContextMenuDisabled: true,
			BackgroundColour:           [4]uint8{0, 0, 0, 0},
			Mac:                        guiwindow.MacWindow{InvisibleTitleBarHeight: 36},
		}}},
	))
	if !r.OK {
		return
	}
	c.Action("window.set_visibility").Run(core.Background(), core.NewOptions(
		core.Option{Key: "task", Value: guiwindow.TaskSetVisibility{Name: wName, Visible: true}},
	))
	c.Action("window.focus").Run(core.Background(), core.NewOptions(
		core.Option{Key: "task", Value: guiwindow.TaskFocus{Name: wName}},
	))
}
