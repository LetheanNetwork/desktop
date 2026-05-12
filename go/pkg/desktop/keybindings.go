// SPDX-Licence-Identifier: EUPL-1.2

// Global keyboard shortcut registration. Wails routes accelerators
// through `app.KeyBinding.Add(...)` — once registered, the shortcut
// works from anywhere in the app (any window focused, or tray popover
// visible).
//
// Pattern: each binding emits an "lthn:key:<verb>" event with the
// originating window's name. Lit elements subscribe via Events.On
// and dispatch state — no shortcut handler needs to know about
// the actual app state.
//
// macOS uses Cmd; Windows/Linux use Ctrl. Wails accepts both forms,
// so we register both for cross-platform parity (the inactive
// modifier is just dead key on each platform). Cmd+Q / Cmd+W /
// Cmd+M / Cmd+H / clipboard shortcuts come from the AppMenu /
// EditMenu / WindowMenu roles registered in desktop.go — we don't
// re-register those here.

package desktop

import (
	"github.com/wailsapp/wails/v3/pkg/application"
)

// registerKeyBindings mounts the default Lethean Desktop accelerators.
// Called once from desktop.Run().
//
// Defaults:
//
//	Cmd/Ctrl+J            → "lthn:key:popover"       (toggle tray popover)
//	Cmd/Ctrl+N            → "lthn:key:new-session"   (new chat)
//	Cmd/Ctrl+K            → "lthn:key:command"       (command palette)
//	Cmd/Ctrl+,            → "lthn:key:settings"      (open settings)
//	Cmd/Ctrl+Shift+M      → "lthn:key:models"        (open models)
//	Cmd/Ctrl+/            → "lthn:key:help"          (shortcuts help)
//	Escape                → "lthn:key:dismiss"       (close active modal)
//
// All emit with the active window's name as the payload so a Lit
// element scoped to one window doesn't react to another's key event.
func registerKeyBindings(app *application.App) {
	bind := func(accel, verb string) {
		handler := emitKey(app, verb)
		app.KeyBinding.Add(accel, handler)
	}

	for _, b := range []struct{ accel, verb string }{
		{"Cmd+J", "popover"},
		{"Ctrl+J", "popover"},
		{"Cmd+N", "new-session"},
		{"Ctrl+N", "new-session"},
		{"Cmd+K", "command"},
		{"Ctrl+K", "command"},
		{"Cmd+,", "settings"},
		{"Ctrl+,", "settings"},
		{"Cmd+Shift+M", "models"},
		{"Ctrl+Shift+M", "models"},
		{"Cmd+/", "help"},
		{"Ctrl+/", "help"},
		{"Escape", "dismiss"},
	} {
		bind(b.accel, b.verb)
	}
}

// emitKey builds a key-handler that re-emits to the app event bus
// with the active window's name. Lit elements filter by window in
// their Events.On callbacks so cross-window keys don't bleed.
func emitKey(app *application.App, verb string) func(application.Window) {
	event := "lthn:key:" + verb
	return func(window application.Window) {
		name := ""
		if window != nil {
			name = window.Name()
		}
		app.Event.Emit(event, name)
	}
}
