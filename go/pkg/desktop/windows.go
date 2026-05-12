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
	// Frameless removes the title bar — caller's CSS draws the
	// chrome via --wails-draggable.
	Frameless bool
	// HideOnClose: close button hides rather than destroys. Used
	// when the window is part of the steady-state UX (chat,
	// settings) — re-opening just shows it again.
	HideOnClose bool
}

// windowRegistry returns the named windows the app knows how to
// open. Today: chat (full chat surface), models (model browser),
// settings (preferences), about (about box).
func windowRegistry() []WindowSpec {
	return []WindowSpec{
		{Name: "chat", Title: "Lethean Chat", Width: 900, Height: 700, MinWidth: 600, MinHeight: 400, HideOnClose: true},
		{Name: "models", Title: "Models", Width: 800, Height: 600, MinWidth: 500, MinHeight: 400, HideOnClose: true},
		{Name: "settings", Title: "Settings", Width: 700, Height: 550, MinWidth: 500, MinHeight: 400, HideOnClose: true},
		{Name: "about", Title: "About Lethean Desktop", Width: 420, Height: 320, Frameless: true},
	}
}

// preCreateWindows materialises the registry as hidden windows so
// the first "Open Chat…" click is instant (no cold-start render).
// Called once from desktop.Run() AFTER the tray popover window is
// constructed.
func preCreateWindows(app *application.App) {
	for _, spec := range windowRegistry() {
		w := app.Window.NewWithOptions(application.WebviewWindowOptions{
			Name:      spec.Name,
			Title:     spec.Title,
			Width:     spec.Width,
			Height:    spec.Height,
			MinWidth:  spec.MinWidth,
			MinHeight: spec.MinHeight,
			Frameless: spec.Frameless,
			Hidden:    true,
			URL:       "/?surface=" + spec.Name,
		})

		if spec.HideOnClose {
			// Steady-state windows hide on close; the surface state
			// (chat history, settings form) persists in pkg/sessions
			// + pkg/config so re-show is just a Show() call.
			ws := w
			ws.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
				ws.Hide()
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
	w.Show()
	w.Focus()
}
