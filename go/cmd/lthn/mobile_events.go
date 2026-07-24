// SPDX-Licence-Identifier: EUPL-1.2

//go:build ios || android

package main

import (
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// registerMobileRuntimeEvents translates Wails application events into the
// lthn:* contract consumed by Angular on desktop, iOS and Android.
func registerMobileRuntimeEvents(app *application.App) {
	if app == nil {
		return
	}

	forward := func(name string) func(*application.ApplicationEvent) {
		return func(event *application.ApplicationEvent) {
			app.Event.Emit(name, mobileApplicationEventData(event))
		}
	}

	app.Event.OnApplicationEvent(events.Common.ApplicationStarted, forward("lthn:app:started"))
	app.Event.OnApplicationEvent(events.Common.BatteryChanged, forward("lthn:system:battery"))
	app.Event.OnApplicationEvent(events.Common.NetworkChanged, forward("lthn:system:network"))
	app.Event.OnApplicationEvent(events.Common.ThemeChanged, forward("lthn:system:theme"))
	app.Event.OnApplicationEvent(events.Common.LowMemory, func(*application.ApplicationEvent) {
		app.Event.Emit("lthn:system:low-memory", map[string]any{})
	})
	app.Event.OnApplicationEvent(events.Common.ScreenLocked, func(*application.ApplicationEvent) {
		app.Event.Emit("lthn:system:lock", map[string]any{"locked": true})
	})
	app.Event.OnApplicationEvent(events.Common.ScreenUnlocked, func(*application.ApplicationEvent) {
		app.Event.Emit("lthn:system:lock", map[string]any{"locked": false})
	})
	app.Event.OnApplicationEvent(events.Common.ApplicationOpenedWithFile, func(event *application.ApplicationEvent) {
		if event == nil {
			return
		}
		app.Event.Emit("lthn:app:opened-file", event.Context().Filename())
	})
	app.Event.OnApplicationEvent(events.Common.ApplicationLaunchedWithUrl, func(event *application.ApplicationEvent) {
		if event == nil {
			return
		}
		app.Event.Emit("lthn:app:opened-url", event.Context().URL())
	})

	app.Event.OnApplicationEvent(events.IOS.ApplicationDidBecomeActive, func(*application.ApplicationEvent) {
		app.Event.Emit("lthn:app:active", map[string]any{"platform": "ios"})
	})
	app.Event.OnApplicationEvent(events.IOS.ApplicationWillResignActive, func(*application.ApplicationEvent) {
		app.Event.Emit("lthn:app:inactive", map[string]any{"platform": "ios"})
	})
	app.Event.OnApplicationEvent(events.IOS.ApplicationDidEnterBackground, func(*application.ApplicationEvent) {
		app.Event.Emit("lthn:app:background", map[string]any{"platform": "ios"})
	})
	app.Event.OnApplicationEvent(events.IOS.ApplicationWillEnterForeground, func(*application.ApplicationEvent) {
		app.Event.Emit("lthn:app:foreground", map[string]any{"platform": "ios"})
	})
	app.Event.OnApplicationEvent(events.IOS.ApplicationWillTerminate, func(*application.ApplicationEvent) {
		app.Event.Emit("lthn:app:terminate", map[string]any{"platform": "ios"})
	})

	app.Event.OnApplicationEvent(events.Android.ActivityResumed, func(*application.ApplicationEvent) {
		app.Event.Emit("lthn:app:active", map[string]any{"platform": "android"})
	})
	app.Event.OnApplicationEvent(events.Android.ActivityPaused, func(*application.ApplicationEvent) {
		app.Event.Emit("lthn:app:inactive", map[string]any{"platform": "android"})
	})
	app.Event.OnApplicationEvent(events.Android.ActivityStopped, func(*application.ApplicationEvent) {
		app.Event.Emit("lthn:app:background", map[string]any{"platform": "android"})
	})
	app.Event.OnApplicationEvent(events.Android.ActivityStarted, func(*application.ApplicationEvent) {
		app.Event.Emit("lthn:app:foreground", map[string]any{"platform": "android"})
	})
	app.Event.OnApplicationEvent(events.Android.ActivityDestroyed, func(*application.ApplicationEvent) {
		app.Event.Emit("lthn:app:terminate", map[string]any{"platform": "android"})
	})
}

func mobileApplicationEventData(event *application.ApplicationEvent) map[string]any {
	if event == nil {
		return map[string]any{}
	}
	data := event.Context().Data()
	if data == nil {
		return map[string]any{}
	}
	return data
}
