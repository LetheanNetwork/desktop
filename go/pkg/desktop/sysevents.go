// SPDX-Licence-Identifier: EUPL-1.2

// System event re-broadcasting. Wails' built-in application and
// window events get republished as "lthn:*" custom events so the
// Lit frontend has one consistent event-bus contract:
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
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// registerSystemEvents wires the cross-platform application events
// onto our lthn:* bus. Called once from desktop.Run() after the
// app is constructed.
//
// Event re-broadcasts:
//
//	events.Common.ApplicationStarted          → "lthn:app:started"
//	events.Common.ApplicationOpenedWithFile   → "lthn:app:opened-file" (file path)
//	events.Common.ApplicationLaunchedWithUrl  → "lthn:app:opened-url"  (URL string)
//	events.Common.ThemeChanged                → "lthn:theme"           (already wired)
func registerSystemEvents(app *application.App) {
	app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(e *application.ApplicationEvent) {
		app.Event.Emit("lthn:app:started", nil)
	})

	app.Event.OnApplicationEvent(events.Common.ApplicationOpenedWithFile, func(e *application.ApplicationEvent) {
		app.Event.Emit("lthn:app:opened-file", e.Context().Filename())
	})

	app.Event.OnApplicationEvent(events.Common.ApplicationLaunchedWithUrl, func(e *application.ApplicationEvent) {
		app.Event.Emit("lthn:app:opened-url", e.Context().URL())
	})
}

// registerWindowEvents wires window-level events to the lthn:* bus.
// Called for each window the desktop service creates. The window's
// name is included in every emitted event so Lit subscribers can
// filter by surface.
//
// Re-broadcasts:
//
//	WindowRuntimeReady     → "lthn:window:ready"          (bindings safe)
//	WindowFocus            → "lthn:window:focus"
//	WindowLostFocus        → "lthn:window:blur"
//	WindowHide             → "lthn:window:hide"
//	WindowShow             → "lthn:window:show"
//	WindowDidResize        → "lthn:window:resize"
//	WindowFilesDropped     → "lthn:window:files-dropped"  (file paths)
func registerWindowEvents(app *application.App, window application.Window) {
	emit := func(verb string, payload any) {
		app.Event.Emit("lthn:window:"+verb, map[string]any{
			"window":  window.Name(),
			"payload": payload,
		})
	}

	window.OnWindowEvent(events.Common.WindowRuntimeReady, func(_ *application.WindowEvent) { emit("ready", nil) })
	window.OnWindowEvent(events.Common.WindowFocus, func(_ *application.WindowEvent) { emit("focus", nil) })
	window.OnWindowEvent(events.Common.WindowLostFocus, func(_ *application.WindowEvent) { emit("blur", nil) })
	window.OnWindowEvent(events.Common.WindowHide, func(_ *application.WindowEvent) { emit("hide", nil) })
	window.OnWindowEvent(events.Common.WindowShow, func(_ *application.WindowEvent) { emit("show", nil) })
	window.OnWindowEvent(events.Common.WindowDidResize, func(_ *application.WindowEvent) { emit("resize", nil) })
	window.OnWindowEvent(events.Common.WindowFilesDropped, func(e *application.WindowEvent) {
		emit("files-dropped", e.Context().DroppedFiles())
	})
}
