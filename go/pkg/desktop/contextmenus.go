// SPDX-Licence-Identifier: EUPL-1.2

// Context-menu registration for the Lit frontend. CoreGUI routes
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
	"context"

	core "dappco.re/go"
	guicontextmenu "dappco.re/go/gui/pkg/contextmenu"
	guievents "dappco.re/go/gui/pkg/events"
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
func registerContextMenus(c *core.Core) {
	if c == nil {
		return
	}
	registerContextMenuRelay(c)
	registerMessageMenu(c)
	registerInputMenu(c)
	registerModelMenu(c)
	registerRouteMenu(c)
}

func registerContextMenuRelay(c *core.Core) {
	c.RegisterAction(func(c *core.Core, msg core.Message) core.Result {
		clicked, ok := msg.(guicontextmenu.ActionItemClicked)
		if !ok {
			return core.Ok(nil)
		}
		event := "lthn:context:" + core.TrimPrefix(clicked.MenuName, "lthn-") + ":" + clicked.ActionID
		return c.Action("events.emit").Run(context.Background(), core.NewOptions(
			core.Option{Key: "task", Value: guievents.TaskEmit{Name: event, Data: clicked.Data}},
		))
	})
}

func addContextMenu(c *core.Core, name string, items []guicontextmenu.MenuItemDef) {
	c.Action("contextmenu.add").Run(context.Background(), core.NewOptions(
		core.Option{Key: "task", Value: guicontextmenu.TaskAdd{
			Name: name,
			Menu: guicontextmenu.ContextMenuDef{Name: name, Items: items},
		}},
	))
}

func menuItem(label, action string) guicontextmenu.MenuItemDef {
	return guicontextmenu.MenuItemDef{Label: label, ActionID: action}
}

func menuSeparator() guicontextmenu.MenuItemDef {
	return guicontextmenu.MenuItemDef{Type: "separator"}
}

func registerMessageMenu(c *core.Core) {
	addContextMenu(c, "lthn-message", []guicontextmenu.MenuItemDef{
		menuItem("Copy", "copy"),
		menuItem("Regenerate", "regenerate"),
		menuItem("Edit", "edit"),
		menuSeparator(),
		menuItem("Delete", "delete"),
	})
}

func registerInputMenu(c *core.Core) {
	addContextMenu(c, "lthn-input", []guicontextmenu.MenuItemDef{
		menuItem("Cut", "cut"),
		menuItem("Copy", "copy"),
		menuItem("Paste", "paste"),
		menuSeparator(),
		menuItem("Select All", "selectall"),
	})
}

func registerModelMenu(c *core.Core) {
	addContextMenu(c, "lthn-model", []guicontextmenu.MenuItemDef{
		menuItem("Reveal in Finder", "reveal"),
		menuItem("Model Info", "info"),
		menuSeparator(),
		menuItem("Remove...", "remove"),
	})
}

func registerRouteMenu(c *core.Core) {
	addContextMenu(c, "lthn-route", []guicontextmenu.MenuItemDef{
		menuItem("Edit", "edit"),
		menuItem("Test Connection", "test"),
		menuSeparator(),
		menuItem("Disable", "disable"),
		menuItem("Remove...", "remove"),
	})
}
