// SPDX-Licence-Identifier: EUPL-1.2

// Global keyboard shortcut registration through CoreGUI.
//
// Pattern: each binding emits an "lthn:key:<verb>" event with the
// originating window's name. Lit elements subscribe via Events.On
// and dispatch state — no shortcut handler needs to know about
// the actual app state.
//
// macOS uses Cmd; Windows/Linux use Ctrl. CoreGUI accepts both forms,
// so we register both for cross-platform parity (the inactive
// modifier is just dead key on each platform). Cmd+Q / Cmd+W /
// Cmd+M / Cmd+H / clipboard shortcuts come from the AppMenu /
// EditMenu / WindowMenu roles registered in desktop.go — we don't
// re-register those here.

package desktop

import (
	"context"

	core "dappco.re/go"
	guievents "dappco.re/go/gui/pkg/events"
	guikeybinding "dappco.re/go/gui/pkg/keybinding"
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
func registerKeyBindings(c *core.Core) {
	if c == nil {
		return
	}
	bindings := []struct{ accel, verb string }{
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
	}
	verbByAccelerator := map[string]string{}
	for _, b := range bindings {
		verbByAccelerator[b.accel] = b.verb
		c.Action("keybinding.add").Run(context.Background(), core.NewOptions(
			core.Option{Key: "task", Value: guikeybinding.TaskAdd{
				Accelerator: b.accel,
				Description: "lthn:" + b.verb,
			}},
		))
	}

	c.RegisterAction(func(c *core.Core, msg core.Message) core.Result {
		triggered, ok := msg.(guikeybinding.ActionTriggered)
		if !ok {
			return core.Ok(nil)
		}
		verb := verbByAccelerator[triggered.Accelerator]
		if verb == "" {
			return core.Ok(nil)
		}
		// TODO(snider): core/gui needs keybinding callbacks to include
		// the originating window name. Until then the frontend receives
		// an empty payload, matching the old type but not the full detail.
		return c.Action("events.emit").Run(context.Background(), core.NewOptions(
			core.Option{Key: "task", Value: guievents.TaskEmit{Name: "lthn:key:" + verb, Data: ""}},
		))
	})
}
