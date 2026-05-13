// SPDX-Licence-Identifier: EUPL-1.2

// Multi-window support. Pre-declared window specs (name → config)
// get materialised on demand — tray menu / keyboard shortcut /
// lthn:// URL handler can open them by name without the frontend
// needing to know Wails internals.
//
// Lit element loading: each window opens at `/?surface=<name>` so
// the index.html SPA router mounts the matching element. The
// surface name is the same name used to retrieve the window
// (app.GetWindowByName).

package desktop

import (
	"github.com/leaanthony/u"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

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
	// window. When true, Wails fires WindowFilesDropped with the
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
	// Wails option of the same name; surfaced here so the registry
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
	MacWindowLevel application.MacWindowLevel
	// MacCollectionBehavior sets the macOS Space + fullscreen
	// behaviour. CanJoinAllSpaces | FullScreenAuxiliary is the
	// canonical "menubar utility" combo — the popover appears on
	// every Space and overlays fullscreen apps. Zero = default.
	MacCollectionBehavior application.MacWindowCollectionBehavior
	// MacBackdrop selects the macOS visual material behind the
	// window. Translucent gives vibrancy depth behind our card.
	// Zero = MacBackdropNormal (opaque).
	MacBackdrop application.MacBackdrop
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
			Width: 760, Height: 580,
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
func preCreateWindows(app *application.App, opts Options) {
	for _, spec := range windowRegistry() {
		w := app.Window.NewWithOptions(application.WebviewWindowOptions{
			Name:           spec.Name,
			Title:          spec.Title,
			Width:          spec.Width,
			Height:         spec.Height,
			MinWidth:       spec.MinWidth,
			MinHeight:      spec.MinHeight,
			MaxWidth:       spec.MaxWidth,
			MaxHeight:      spec.MaxHeight,
			Frameless:      spec.Frameless,
			Hidden:         true,
			EnableFileDrop: spec.EnableFileDrop,
			URL:            "/?surface=" + spec.Name,
			// Transparent background so the rounded corners of the
			// renderChrome()-painted lthn-window card aren't framed by
			// an opaque OS rectangle. The Lit body has border-radius
			// 11px (Lethean-6 canon); without alpha=0 here you'd see
			// dark grey squares behind each rounded corner.
			BackgroundColour: application.NewRGBA(0, 0, 0, 0),
			// Always-on-top / hide-on-Esc / hide-on-focus-loss /
			// fixed-size knobs surface from the spec so the registry
			// is the single source of truth for window behaviour.
			AlwaysOnTop:     spec.AlwaysOnTop,
			HideOnFocusLost: spec.HideOnFocusLost,
			HideOnEscape:    spec.HideOnEscape,
			DisableResize:   spec.DisableResize,
			// ContentProtection blocks OS screen capture (wallets,
			// private chat). macOS + Windows 10+; no-op on Linux.
			ContentProtectionEnabled: spec.ContentProtection,
			// We ship our own context menus via contextmenus.go;
			// surfaces that route right-click through the Lit
			// element should disable the native WebView menu.
			DefaultContextMenuDisabled: spec.DisableNativeContextMenu,
			// macOS knobs — Wails packages every native NSWindow
			// attribute we need; setting them here means the same
			// codepath produces Floating-tray / FullScreenAuxiliary
			// / vibrancy-backdrop behaviour without a platform
			// switch in the spec or per-window cgo. iOS/Android
			// inherit zero-value defaults from this same struct.
			Mac: application.MacWindow{
				InvisibleTitleBarHeight: spec.InvisibleTitleBarHeight,
				WindowLevel:             spec.MacWindowLevel,
				CollectionBehavior:      spec.MacCollectionBehavior,
				Backdrop:                spec.MacBackdrop,
				// Disable the WebView swipe-back/forward gesture
				// app-wide — it breaks our SPA routing (the route
				// state lives in ?surface= and ?pane= query params,
				// not in the WebView's history stack). Same gesture
				// behaviour on Windows lives under EnableSwipeGestures
				// (default false) so this is the macOS-side mirror.
				WebviewPreferences: application.MacWebviewPreferences{
					AllowsBackForwardNavigationGestures: u.False,
				},
			},
			// Linux icon — Wails uses this for the minimized window
			// icon + GTK header bar. Same PNG as the macOS Dock /
			// Windows taskbar so brand identity is uniform.
			Linux: application.LinuxWindow{
				Icon: opts.AppIcon,
			},
		})

		if spec.HideOnClose {
			// Steady-state windows hide on close; the surface state
			// (chat history, settings form) persists in pkg/sessions
			// + pkg/config so re-show is just a Show() call.
			//
			// The unified `app` shell also demotes the macOS activation
			// policy back to Accessory on close so the Dock icon
			// disappears and the app returns to its tray-only steady
			// state. The close button AND the Dock-icon "close" path
			// both flow through WindowClosing, so one hook covers both.
			ws := w
			isAppShell := spec.Name == "app"
			ws.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
				ws.Hide()
				if isAppShell {
					setPolicyAccessory()
				}
				e.Cancel()
			})
		}

		// Per-window event re-broadcasts (lthn:window:ready etc.).
		registerWindowEvents(app, w)
	}
}

// openWindow shows + focuses the named window. Backend-driven so
// tray menu items / keyboard shortcuts / lthn:// URL handlers can
// open windows without round-tripping through the frontend.
//
// If the name isn't in the registry, this is a no-op (returns
// silently — the caller's tray menu shouldn't have offered the
// option in the first place).
func openWindow(app *application.App, name string) {
	w, ok := app.Window.GetByName(name)
	if !ok {
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
	w.Show()
	w.Focus()
}
