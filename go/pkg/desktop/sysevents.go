// SPDX-Licence-Identifier: EUPL-1.2

// System event re-broadcasting. CoreGUI lifecycle and window events
// get republished as "lthn:*" custom events so the Lit frontend has
// one consistent event-bus contract:
//
//   Events.On("lthn:app:opened-url", e => { route(e.data) })
//   Events.On("lthn:window:files-dropped", e => { import(e.data) })
//   Events.On("lthn:window:ready", e => { /* bindings now safe */ })
//
// One subscriber pattern across theme / notification / tray / context
// / key / app / window. The frontend doesn't need to import
// wails-internal event constants — just the lthn:* string names.

package desktop

import (
	core "dappco.re/go"
	guienvironment "dappco.re/go/gui/pkg/environment"
	guievents "dappco.re/go/gui/pkg/events"
	guilifecycle "dappco.re/go/gui/pkg/lifecycle"
	guiwindow "dappco.re/go/gui/pkg/window"
)

// TODO(snider): core/gui needs lifecycle/window event coverage for
// opened-url, runtime-ready, hide/show/state events, and full file-drop
// target details so these lthn:* frontend events can have exact parity
// with the previous Wails event constants.

// registerSystemEvents wires the cross-platform application events
// onto our lthn:* bus. Called once from desktop.Run() after the
// app is constructed.
//
// Event re-broadcasts:
//
//	guilifecycle.ActionApplicationStarted → "lthn:app:started"
//	guilifecycle.ActionOpenedWithFile     → "lthn:app:opened-file" (file path)
//	guienvironment.ActionThemeChanged    → "lthn:theme"
//	guiwindow.ActionWindowFocused         → "lthn:window:focus"
//	guiwindow.ActionWindowBlurred         → "lthn:window:blur"
//	guiwindow.ActionWindowMoved           → "lthn:window:move"
//	guiwindow.ActionWindowResized         → "lthn:window:resize"
//	guiwindow.ActionFilesDropped          → "lthn:window:files-dropped"
func registerSystemEvents(c *core.Core) {
	if c == nil {
		return
	}
	c.RegisterAction(func(c *core.Core, msg core.Message) core.Result {
		switch event := msg.(type) {
		case guilifecycle.ActionApplicationStarted:
			return emitCoreEvent(c, "lthn:app:started", nil)
		case guilifecycle.ActionOpenedWithFile:
			return emitCoreEvent(c, "lthn:app:opened-file", event.Path)
		case guienvironment.ActionThemeChanged:
			mode := "light"
			if event.IsDark {
				mode = "dark"
			}
			return emitCoreEvent(c, "lthn:theme", mode)
		case guiwindow.ActionWindowFocused:
			return emitWindowEvent(c, "focus", event.Name, nil)
		case guiwindow.ActionWindowBlurred:
			return emitWindowEvent(c, "blur", event.Name, nil)
		case guiwindow.ActionWindowMoved:
			return emitWindowEvent(c, "move", event.Name, map[string]any{"x": event.X, "y": event.Y})
		case guiwindow.ActionWindowResized:
			return emitWindowEvent(c, "resize", event.Name, map[string]any{"width": event.Width, "height": event.Height})
		case guiwindow.ActionFilesDropped:
			payload := map[string]any{"files": event.Paths}
			if event.TargetID != "" {
				payload["target"] = map[string]any{"id": event.TargetID}
			}
			return emitWindowEvent(c, "files-dropped", event.Name, payload)
		default:
			return core.Ok(nil)
		}
	})
}

func emitWindowEvent(c *core.Core, verb, window string, payload any) core.Result {
	return emitCoreEvent(c, "lthn:window:"+verb, map[string]any{
		"window":  window,
		"payload": payload,
	})
}

func emitCoreEvent(c *core.Core, name string, data any) core.Result {
	if c == nil {
		return core.Ok(nil)
	}
	return c.Action("events.emit").Run(core.Background(), core.NewOptions(
		core.Option{Key: "task", Value: guievents.TaskEmit{Name: name, Data: data}},
	))
}
