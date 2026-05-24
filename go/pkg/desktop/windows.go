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
	guiwindow "dappco.re/go/gui/pkg/window"
)

// TODO(snider): core/gui needs richer window creation options before this
// registry can move fully to window.open: macOS window level/collection/
// backdrop/webview preferences, hide-on-close hooks, content protection,
// native context-menu disable, hide-on-focus-lost, hide-on-escape, and
// plugin-window re-show semantics.

// WindowSpec describes one named window the app can open.
type WindowSpec struct {
	// Name is the registry key (also the ?surface= query param).
	Name string
	// Title is the OS-level window title.
	Title string
	// Width / Height in logical pixels.
	Width, Height int
	// MinWidth / MinHeight prevent shrinkage; zero = no minimum.
	MinWidth, MinHeight int
	// MaxWidth / MaxHeight cap the upper bound; zero = no maximum.
	// Pair with Width/Height (no min difference) for genuinely
	// fixed-size windows like `about` or splash screens.
	MaxWidth, MaxHeight int
	// Frameless removes the title bar — caller's CSS draws the
	// chrome via --wails-draggable.
	Frameless bool
	// HideOnClose: close button hides rather than destroys. Used
	// when the window is part of the steady-state UX (chat,
	// settings) — re-opening just shows it again.
	HideOnClose bool
	// EnableFileDrop accepts files dragged from the OS onto the
	// window. When true, the backend fires file-drop events with the
	// full filesystem paths + DropTargetDetails (which
	// data-file-drop-target element received the drop). Off by
	// default — chat and models surfaces opt in; settings/about
	// don't need it.
	EnableFileDrop bool
	// InvisibleTitleBarHeight declares the top N pixels of a
	// frameless macOS window as a native-managed drag region.
	// Layered on top of our --wails-draggable CSS — adds an OS
	// drag handler rather than replacing the chrome. Zero = off.
	InvisibleTitleBarHeight int
	// ContentProtection blocks the OS screen-capture API from
	// recording this window. Wallets / private chat / key reveal
	// surfaces should set true. macOS + Windows 10+; no-op on Linux.
	ContentProtection bool
	// DisableNativeContextMenu kills the WebView's built-in
	// right-click menu. Windows that ship their own context menu
	// (chat message actions, model card menu) should set true so
	// the native one doesn't double up.
	DisableNativeContextMenu bool
	// AlwaysOnTop pins the window above all others. Mirrors the
	// native option of the same name; surfaced here so the registry
	// is the single declaration site.
	AlwaysOnTop bool
	// HideOnFocusLost auto-hides the window when it loses focus.
	// Tray-popover pattern; not useful for steady-state windows.
	HideOnFocusLost bool
	// HideOnEscape auto-hides on Esc keypress. Tray + transient
	// helper windows; off for editor-class surfaces where Esc has
	// app meaning.
	HideOnEscape bool
	// DisableResize prevents the user from resizing the window.
	// Pairs with fixed Width/Height for tray popover, splash.
	DisableResize bool
	// MacWindowLevel sets the macOS window level — Floating for
	// the tray popover so it stays above normal windows. Leave
	// empty for the default Normal level.
	MacWindowLevel int
	// MacCollectionBehavior sets the macOS Space + fullscreen
	// behaviour. CanJoinAllSpaces | FullScreenAuxiliary is the
	// canonical "menubar utility" combo — the popover appears on
	// every Space and overlays fullscreen apps. Zero = default.
	MacCollectionBehavior int
	// MacBackdrop selects the macOS visual material behind the
	// window. Translucent gives vibrancy depth behind our card.
	// Zero = MacBackdropNormal (opaque).
	MacBackdrop int
}

// windowRegistry returns the named windows the app knows how to
// open. Today: chat (full chat surface), models (model browser),
// settings (preferences), about (about box).
func windowRegistry() []WindowSpec {
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
	return []WindowSpec{
		{
			Name: "welcome", Title: "Welcome to lthn",
			// Initial dimensions — the welcome wizard's Lit element
			// (frontend/src/lit/ops/welcome-window.ts) refines via
			// WindowService.SetSize on firstUpdated to match its own
			// declared `w`/`h`, so this value is the "before first
			// paint" floor rather than a hard cap.
			Width: 1200, Height: 800,
			Frameless: true, HideOnClose: true,
			InvisibleTitleBarHeight:  titleBarH,
			DisableNativeContextMenu: true,
		},
		// `app` is the Lethean-6 unified application shell — single
		// window holding titlebar + grouped side-nav + body that
		// auto-mounts any <lthn-*-window>. Tray buttons open this with
		// ?pane=<id> set; the side-nav swaps panes internally thereafter.
		{
			Name: "app", Title: "Lethean Desktop",
			// Bumped from 1200×800 — four-column chat layout (nav +
			// conversation list + chat body + right rail) felt cramped
			// at the previous default. 1440×900 is the standard MBP 14"+
			// retina viewport so the unified shell breathes on most
			// modern Macs without spilling off small displays.
			Width: 1440, Height: 900, MinWidth: 1000, MinHeight: 680,
			Frameless: true, HideOnClose: true, EnableFileDrop: true,
			InvisibleTitleBarHeight:  titleBarH,
			DisableNativeContextMenu: true,
		},
		{
			Name: "chat", Title: "Lethean Chat",
			Width: 900, Height: 700, MinWidth: 600, MinHeight: 400,
			Frameless: true, HideOnClose: true, EnableFileDrop: true,
			InvisibleTitleBarHeight:  titleBarH,
			DisableNativeContextMenu: true,
		},
		{
			Name: "models", Title: "Models",
			Width: 800, Height: 600, MinWidth: 500, MinHeight: 400,
			Frameless: true, HideOnClose: true, EnableFileDrop: true,
			InvisibleTitleBarHeight:  titleBarH,
			DisableNativeContextMenu: true,
		},
		{
			Name: "settings", Title: "Settings",
			Width: 700, Height: 550, MinWidth: 500, MinHeight: 400,
			Frameless: true, HideOnClose: true,
			InvisibleTitleBarHeight:  titleBarH,
			DisableNativeContextMenu: true,
		},
		{
			Name: "telemetry", Title: "Telemetry",
			Width: 880, Height: 560, MinWidth: 600, MinHeight: 400,
			Frameless: true, HideOnClose: true,
			InvisibleTitleBarHeight:  titleBarH,
			DisableNativeContextMenu: true,
		},
		{
			Name: "editor", Title: "Editor",
			Width: 1040, Height: 700, MinWidth: 600, MinHeight: 400,
			Frameless: true, HideOnClose: true, EnableFileDrop: true,
			InvisibleTitleBarHeight:  titleBarH,
			DisableNativeContextMenu: true,
		},
		{
			Name: "git", Title: "Git",
			Width: 1120, Height: 740, MinWidth: 700, MinHeight: 450,
			Frameless: true, HideOnClose: true, EnableFileDrop: true,
			InvisibleTitleBarHeight:  titleBarH,
			DisableNativeContextMenu: true,
		},
		{
			Name: "build", Title: "Build",
			Width: 1080, Height: 700, MinWidth: 700, MinHeight: 450,
			Frameless: true, HideOnClose: true, EnableFileDrop: true,
			InvisibleTitleBarHeight:  titleBarH,
			DisableNativeContextMenu: true,
		},
		{
			Name: "lint", Title: "Lint",
			Width: 1100, Height: 740, MinWidth: 700, MinHeight: 450,
			Frameless: true, HideOnClose: true, EnableFileDrop: true,
			InvisibleTitleBarHeight:  titleBarH,
			DisableNativeContextMenu: true,
		},
		{
			Name: "containers", Title: "Containers",
			Width: 1180, Height: 760, MinWidth: 720, MinHeight: 460,
			Frameless: true, HideOnClose: true, EnableFileDrop: false,
			InvisibleTitleBarHeight:  titleBarH,
			DisableNativeContextMenu: true,
		},
		{
			Name: "repos", Title: "Repos",
			Width: 1180, Height: 760, MinWidth: 720, MinHeight: 460,
			Frameless: true, HideOnClose: true, EnableFileDrop: false,
			InvisibleTitleBarHeight:  titleBarH,
			DisableNativeContextMenu: true,
		},
		{
			Name: "php", Title: "PHP",
			Width: 1200, Height: 780, MinWidth: 760, MinHeight: 480,
			Frameless: true, HideOnClose: true, EnableFileDrop: false,
			InvisibleTitleBarHeight:  titleBarH,
			DisableNativeContextMenu: true,
		},
		{
			Name: "marketplace", Title: "Marketplace",
			Width: 1180, Height: 760, MinWidth: 720, MinHeight: 460,
			Frameless: true, HideOnClose: true, EnableFileDrop: false,
			InvisibleTitleBarHeight:  titleBarH,
			DisableNativeContextMenu: true,
		},
		{
			Name: "about", Title: "About Lethean Desktop",
			// Genuinely fixed size — Min == Max == Width/Height +
			// DisableResize for the splash-card feel.
			Width: 420, Height: 320,
			MinWidth: 420, MinHeight: 320,
			MaxWidth: 420, MaxHeight: 320,
			Frameless: true, DisableResize: true,
			InvisibleTitleBarHeight:  titleBarH,
			DisableNativeContextMenu: true,
		},
	}
}

// preCreateWindows materialises the registry as hidden windows so
// the first "Open Chat…" click is instant (no cold-start render).
// Called once from desktop.Run() AFTER the tray popover window is
// constructed. opts carries the desktop service Options (currently
// only AppIcon for the Linux icon binding); pass s.opts.
func preCreateWindows(c *core.Core, opts Options) {
	if c == nil {
		return
	}
	registerAllWindows(c)
	for _, spec := range windowRegistry() {
		openWindowSpec(c, spec, opts, true)
	}
}

// registerAllWindows populates CoreGUI's WindowSpec registry from this
// package's windowRegistry() — one window.register action per entry.
// Pairs with the lazy-mount path in CoreGUI's taskSetVisibility (see
// plans/code/core/gui/RFC.window-lifecycle.md §3): a registered spec
// is what taskSetVisibility consults when set_visibility(name, true)
// arrives on a window that hasn't yet been opened.
//
// Today preCreateWindows also eager-opens each spec (hidden=true) for
// instant-show behaviour from tray clicks. The registry runs alongside
// that eager pre-creation — once a window is in the manager, the
// registry entry is dormant. The registry only fires when a consumer
// (bridge cold-start, future fleet-mode callers) issues set_visibility
// on a name whose platform window was never created.
func registerAllWindows(c *core.Core) {
	if c == nil {
		return
	}
	for _, spec := range windowRegistry() {
		kind := guiwindow.KindWebview
		// Tray windows go through pkg/systray's lifecycle, not the
		// webview registry. The current registry contains only
		// webview windows; KindTray reserved for future tray entries
		// declared here for unified lifecycle control.
		w := &guiwindow.Window{
			Name: spec.Name, Title: spec.Title,
			Width: spec.Width, Height: spec.Height,
			MinWidth: spec.MinWidth, MinHeight: spec.MinHeight,
			MaxWidth: spec.MaxWidth, MaxHeight: spec.MaxHeight,
			Frameless:        spec.Frameless,
			AlwaysOnTop:      spec.AlwaysOnTop,
			DisableResize:    spec.DisableResize,
			EnableFileDrop:   spec.EnableFileDrop,
			URL:              "/?surface=" + spec.Name,
			BackgroundColour: [4]uint8{0, 0, 0, 0},
		}
		c.Action("window.register").Run(core.Background(), core.NewOptions(
			core.Option{Key: "task", Value: guiwindow.TaskRegisterWindow{Window: w, Kind: kind}},
		))
		// Errors from window.register at boot are non-fatal — duplicate
		// registrations (the surfaced failure mode) are recoverable;
		// the registry already has the spec under that name. Log-by-
		// returning-OK keeps boot quiet; if a real consumer needs to
		// know it can call window.register directly and inspect Result.
	}
}

func openWindowSpec(c *core.Core, spec WindowSpec, _ Options, hidden bool) bool {
	if c == nil {
		return false
	}
	r := c.Action("window.open").Run(core.Background(), core.NewOptions(
		core.Option{Key: "task", Value: guiwindow.TaskOpenWindow{Window: &guiwindow.Window{
			Name: spec.Name, Title: spec.Title,
			Width: spec.Width, Height: spec.Height,
			MinWidth: spec.MinWidth, MinHeight: spec.MinHeight,
			MaxWidth: spec.MaxWidth, MaxHeight: spec.MaxHeight,
			Frameless: spec.Frameless, Hidden: hidden,
			AlwaysOnTop:      spec.AlwaysOnTop,
			DisableResize:    spec.DisableResize,
			EnableFileDrop:   spec.EnableFileDrop,
			URL:              "/?surface=" + spec.Name,
			BackgroundColour: [4]uint8{0, 0, 0, 0},
		}}},
	))
	if !r.OK {
		return false
	}
	if spec.ContentProtection {
		c.Action("window.set_content_protection").Run(core.Background(), core.NewOptions(
			core.Option{Key: "task", Value: guiwindow.TaskSetContentProtection{Name: spec.Name, Protection: true}},
		))
	}
	return true
}

// openWindow shows + focuses the named window. Backend-driven so
// tray menu items / keyboard shortcuts / lthn:// URL handlers can
// open windows without round-tripping through the frontend.
//
// If the name isn't in the registry, this is a no-op (returns
// silently — the caller's tray menu shouldn't have offered the
// option in the first place).
func openWindow(c *core.Core, name string) {
	if c == nil || !windowExists(c, name) {
		return
	}
	// The unified `app` shell is the only window that warrants a Dock
	// presence — the tray-spawned "lite" cards stay in Accessory so
	// the menubar stays the single source of truth for them. Elevate
	// BEFORE Show() so the activation policy is in place by the time
	// the window draws and the OS decides cmd+Tab eligibility.
	if name == "app" {
		setPolicyRegular()
	}
	c.Action("window.restore").Run(core.Background(), core.NewOptions(
		core.Option{Key: "task", Value: guiwindow.TaskRestore{Name: name}},
	))
	c.Action("window.set_visibility").Run(core.Background(), core.NewOptions(
		core.Option{Key: "task", Value: guiwindow.TaskSetVisibility{Name: name, Visible: true}},
	))
	c.Action("window.focus").Run(core.Background(), core.NewOptions(
		core.Option{Key: "task", Value: guiwindow.TaskFocus{Name: name}},
	))
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
	if !windowExists(typed, name) {
		return false
	}
	r := typed.Action("window.set_visibility").Run(core.Background(), core.NewOptions(
		core.Option{Key: "task", Value: guiwindow.TaskSetVisibility{Name: name, Visible: false}},
	))
	return r.OK
}

func windowExists(c *core.Core, name string) bool {
	if c == nil || name == "" {
		return false
	}
	r := c.QUERY(guiwindow.QueryWindowByName{Name: name})
	return r.OK && r.Value != nil
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
	titleBarH := 36 // matches every other Lethean-5 chromed window
	_ = titleBarH
	ok := openWindowSpec(c, WindowSpec{
		Name: wName, Title: "Plugin · " + code,
		Width: 1180, Height: 760, MinWidth: 720, MinHeight: 460,
		Frameless: true, EnableFileDrop: false,
		InvisibleTitleBarHeight:  titleBarH,
		DisableNativeContextMenu: true,
	}, Options{}, true)
	if !ok {
		return
	}
	c.Action("window.set_url").Run(core.Background(), core.NewOptions(
		core.Option{Key: "task", Value: guiwindow.TaskSetURL{Name: wName, URL: "/?surface=plugin&code=" + code}},
	))
	c.Action("window.set_visibility").Run(core.Background(), core.NewOptions(
		core.Option{Key: "task", Value: guiwindow.TaskSetVisibility{Name: wName, Visible: true}},
	))
	c.Action("window.focus").Run(core.Background(), core.NewOptions(
		core.Option{Key: "task", Value: guiwindow.TaskFocus{Name: wName}},
	))
}
