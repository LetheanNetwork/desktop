// SPDX-Licence-Identifier: EUPL-1.2

// Context-menu registration for the Lit frontend. Wails routes
// right-clicks through CSS — any HTML element with
// `style="--custom-contextmenu: <name>"` shows the menu registered
// here under that name. Per-click context (which message, which
// model row) flows via `--custom-contextmenu-data: <value>` and
// reaches the Go handler via ctx.ContextMenuData().
//
// Handler shape: emit an "lthn:context:<menu>:<action>" event with
// the context data string. The originating Lit element subscribes
// and dispatches against its own state — no round-trip needed
// for the actual side effect.

package desktop

import (
	"github.com/wailsapp/wails/v3/pkg/application"
)

// registerContextMenus mounts the standard right-click surfaces.
// Called once at startup from desktop.Run(); the registered menus
// are referenced by name from the frontend CSS.
//
// Menus today:
//
//	lthn-message  — chat message bubbles (Copy / Regenerate / Edit / Delete)
//	lthn-input    — chat textarea + any editable input (Cut / Copy / Paste / Select All)
//	lthn-model    — model row in the models surface (Reveal in Finder / Remove / Info)
//	lthn-route    — provider-route entry in settings (Edit / Test / Remove / Disable)
func registerContextMenus(app *application.App) {
	registerMessageMenu(app)
	registerInputMenu(app)
	registerModelMenu(app)
	registerRouteMenu(app)
}

// emitContext is the shared handler shape — re-emits the right-click
// as an app event the frontend can listen for.
func emitContext(app *application.App, menu, action string) func(*application.Context) {
	event := "lthn:context:" + menu + ":" + action
	return func(ctx *application.Context) {
		app.Event.Emit(event, ctx.ContextMenuData())
	}
}

func registerMessageMenu(app *application.App) {
	m := app.ContextMenu.New()
	m.Add("Copy").OnClick(emitContext(app, "message", "copy"))
	m.Add("Regenerate").OnClick(emitContext(app, "message", "regenerate"))
	m.Add("Edit").OnClick(emitContext(app, "message", "edit"))
	m.AddSeparator()
	m.Add("Delete").OnClick(emitContext(app, "message", "delete"))
	app.ContextMenu.Add("lthn-message", m)
}

func registerInputMenu(app *application.App) {
	m := app.ContextMenu.New()
	m.Add("Cut").OnClick(emitContext(app, "input", "cut"))
	m.Add("Copy").OnClick(emitContext(app, "input", "copy"))
	m.Add("Paste").OnClick(emitContext(app, "input", "paste"))
	m.AddSeparator()
	m.Add("Select All").OnClick(emitContext(app, "input", "selectall"))
	app.ContextMenu.Add("lthn-input", m)
}

func registerModelMenu(app *application.App) {
	m := app.ContextMenu.New()
	m.Add("Reveal in Finder").OnClick(emitContext(app, "model", "reveal"))
	m.Add("Model Info").OnClick(emitContext(app, "model", "info"))
	m.AddSeparator()
	m.Add("Remove…").OnClick(emitContext(app, "model", "remove"))
	app.ContextMenu.Add("lthn-model", m)
}

func registerRouteMenu(app *application.App) {
	m := app.ContextMenu.New()
	m.Add("Edit").OnClick(emitContext(app, "route", "edit"))
	m.Add("Test Connection").OnClick(emitContext(app, "route", "test"))
	m.AddSeparator()
	m.Add("Disable").OnClick(emitContext(app, "route", "disable"))
	m.Add("Remove…").OnClick(emitContext(app, "route", "remove"))
	app.ContextMenu.Add("lthn-route", m)
}
