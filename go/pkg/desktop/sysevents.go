// SPDX-Licence-Identifier: EUPL-1.2

// System event re-broadcasting. CoreGUI lifecycle and window events
// get republished as "lthn:*" custom events so the Angular frontend has
// one consistent event-bus contract:
//
//   Events.On("lthn:app:opened-url", e => { route(e.data) })
//   Events.On("lthn:host:intent", e => { route(e.data) })
//   Events.On("lthn:window:ready", e => { /* bindings now safe */ })
//
// One subscriber pattern across theme / notification / tray / context
// / key / app / window. The frontend doesn't need to import
// wails-internal event constants — just the lthn:* string names.

package desktop

import (
	core "dappco.re/go"
	gui "dappco.re/go/render/display/webkit"
	guienvironment "dappco.re/go/render/display/webkit/pkg/environment"
	guilifecycle "dappco.re/go/render/display/webkit/pkg/lifecycle"
	guinotification "dappco.re/go/render/display/webkit/pkg/notification"
	guiwindow "dappco.re/go/render/display/webkit/pkg/window"
)

// All substrate gaps that pre-dated GUI-16..GUI-24 are closed. This
// re-broadcaster faithfully translates everything core/gui surfaces
// for app + theme + window + notification lifecycle.

// registerSystemEvents wires the cross-platform application events
// onto our lthn:* bus. Called once from desktop.Run() after the
// app is constructed.
//
// Event re-broadcasts:
//
//	guilifecycle.ActionApplicationStarted         → "lthn:app:started"
//	guilifecycle.ActionOpenedWithFile             → "lthn:host:intent" (opaque Files capability)
//	guilifecycle.ActionLaunchedWithUrl            → "lthn:app:opened-url" (full URL — lthn://… or similar)
//	guienvironment.ActionThemeChanged             → "lthn:theme"
//	guiwindow.ActionWindowFocused                 → "lthn:window:focus"
//	guiwindow.ActionWindowBlurred                 → "lthn:window:blur"
//	guiwindow.ActionWindowMoved                   → "lthn:window:move"
//	guiwindow.ActionWindowResized                 → "lthn:window:resize"
//	guiwindow.ActionFilesDropped                  → "lthn:host:intent" (opaque Files capabilities)
//	guiwindow.ActionWindowHidden                  → "lthn:window:hide"
//	guiwindow.ActionWindowShown                   → "lthn:window:show"
//	guiwindow.ActionWindowMinimised               → "lthn:window:minimise"
//	guiwindow.ActionWindowUnminimised             → "lthn:window:unminimise"
//	guiwindow.ActionWindowMaximised               → "lthn:window:maximise"
//	guiwindow.ActionWindowUnmaximised             → "lthn:window:unmaximise"
//	guiwindow.ActionWindowFullscreened            → "lthn:window:fullscreen"
//	guiwindow.ActionWindowUnfullscreened          → "lthn:window:unfullscreen"
//	guiwindow.ActionWindowRuntimeReady            → "lthn:window:ready"
//	guinotification.ActionNotificationClicked     → "lthn:notification:click" (notification id)
//	guinotification.ActionNotificationActionTriggered → "lthn:notification:action" ({notification_id, action_id})
//	guinotification.ActionNotificationDismissed   → "lthn:notification:dismiss" (notification id)
func registerSystemEvents(c *core.Core) {
	if c == nil {
		return
	}
	c.RegisterAction(func(c *core.Core, msg core.Message) core.Result {
		switch event := msg.(type) {
		case guilifecycle.ActionApplicationStarted:
			return emitCoreEvent(c, "lthn:app:started", nil)
		case guilifecycle.ActionOpenedWithFile:
			return emitHostItemIntent(
				c,
				HostIntentOpenItems,
				[]string{event.Path},
				nil,
				"",
			)
		case guilifecycle.ActionLaunchedWithUrl:
			opened := emitCoreEvent(c, "lthn:app:opened-url", event.URL)
			if handled := handleDeepLink(c, event.URL); !handled.OK {
				core.Warn("desktop deep link ignored", "err", handled.Error())
			}
			return opened
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
			return emitHostItemIntent(
				c,
				HostIntentDropItems,
				event.Paths,
				event.Target,
				event.TargetID,
			)
		case guiwindow.ActionWindowHidden:
			return emitWindowEvent(c, "hide", event.Name, nil)
		case guiwindow.ActionWindowShown:
			return emitWindowEvent(c, "show", event.Name, nil)
		case guiwindow.ActionWindowMinimised:
			return emitWindowEvent(c, "minimise", event.Name, nil)
		case guiwindow.ActionWindowUnminimised:
			return emitWindowEvent(c, "unminimise", event.Name, nil)
		case guiwindow.ActionWindowMaximised:
			return emitWindowEvent(c, "maximise", event.Name, nil)
		case guiwindow.ActionWindowUnmaximised:
			return emitWindowEvent(c, "unmaximise", event.Name, nil)
		case guiwindow.ActionWindowFullscreened:
			return emitWindowEvent(c, "fullscreen", event.Name, nil)
		case guiwindow.ActionWindowUnfullscreened:
			return emitWindowEvent(c, "unfullscreen", event.Name, nil)
		case guiwindow.ActionWindowRuntimeReady:
			return emitWindowEvent(c, "ready", event.Name, nil)
		case guinotification.ActionNotificationClicked:
			return emitCoreEvent(c, "lthn:notification:click", event.ID)
		case guinotification.ActionNotificationActionTriggered:
			return emitCoreEvent(c, "lthn:notification:action", map[string]any{
				"notification_id": event.NotificationID,
				"action_id":       event.ActionID,
			})
		case guinotification.ActionNotificationDismissed:
			return emitCoreEvent(c, "lthn:notification:dismiss", event.ID)
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
	return gui.EmitEvent(c, name, data)
}
