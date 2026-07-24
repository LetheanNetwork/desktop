// SPDX-Licence-Identifier: EUPL-1.2

//go:build ios

package main

import "github.com/wailsapp/wails/v3/pkg/application"

func registerMobileNativeFeatures(app *application.App) {
	app.Event.On("common:share", func(event *application.CustomEvent) {
		application.IOS.Share(mobilePayloadJSON(event.Data))
	})
	app.Event.On("common:openURL", func(event *application.CustomEvent) {
		application.IOS.OpenURL(mobileEventString(event.Data, "url"))
	})
	app.Event.On("common:keepAwake", func(event *application.CustomEvent) {
		application.IOS.SetKeepAwake(mobileEventBool(event.Data, "enabled", false))
	})
	app.Event.On("common:torch", func(event *application.CustomEvent) {
		if payload := mobileFirstMap(event.Data); payload != nil {
			if _, result := payload["available"]; result {
				return
			}
		}
		application.IOS.SetTorch(mobileEventBool(event.Data, "enabled", false))
	})
	app.Event.On("common:getSafeArea", func(*application.CustomEvent) {
		app.Event.Emit("common:safeArea", mobileJSONMap(application.IOS.SafeAreaJSON()))
	})
	app.Event.On("common:setBrightness", func(event *application.CustomEvent) {
		application.IOS.SetBrightness(mobileEventFloat(event.Data, "value", 0.5))
	})
	app.Event.On("common:getBrightness", func(*application.CustomEvent) {
		app.Event.Emit("common:brightness", map[string]any{"value": application.IOS.GetBrightness()})
	})
	app.Event.On("common:getAppInfo", func(*application.CustomEvent) {
		app.Event.Emit("common:appInfo", mobileJSONMap(application.IOS.AppInfoJSON()))
	})
	app.Event.On("common:setOrientation", func(event *application.CustomEvent) {
		application.IOS.SetOrientation(mobileEventString(event.Data, "mode"))
	})
	app.Event.On("common:getOrientation", func(*application.CustomEvent) {
		app.Event.Emit("common:orientation", map[string]any{"orientation": application.IOS.GetOrientation()})
	})
	app.Event.On("common:setStatusBar", func(event *application.CustomEvent) {
		application.IOS.SetStatusBar(mobilePayloadJSON(event.Data))
	})
	app.Event.On("common:authenticate", func(event *application.CustomEvent) {
		reason := mobileEventString(event.Data, "reason")
		if reason == "" {
			reason = "Authenticate to continue"
		}
		application.IOS.BiometricAuthenticate(reason)
	})
	app.Event.On("common:notify", func(event *application.CustomEvent) {
		application.IOS.PostNotification(mobilePayloadJSON(event.Data))
	})
	app.Event.On("common:secureSet", func(event *application.CustomEvent) {
		application.IOS.SecureSet(
			mobileEventString(event.Data, "key"),
			mobileEventString(event.Data, "value"),
		)
	})
	app.Event.On("common:secureGet", func(event *application.CustomEvent) {
		key := mobileEventString(event.Data, "key")
		app.Event.Emit("common:secureValue", map[string]any{
			"key": key, "value": application.IOS.SecureGet(key),
		})
	})
	app.Event.On("common:secureDelete", func(event *application.CustomEvent) {
		application.IOS.SecureDelete(mobileEventString(event.Data, "key"))
	})
	app.Event.On("common:haptic", func(event *application.CustomEvent) {
		application.IOS.Haptic(mobileEventString(event.Data, "type"))
	})
	app.Event.On("common:getLocation", func(*application.CustomEvent) {
		application.IOS.GetLocation()
	})
	app.Event.On("common:watchMotion", func(event *application.CustomEvent) {
		application.IOS.SetMotion(mobileEventBool(event.Data, "enabled", false))
	})
	app.Event.On("common:watchProximity", func(event *application.CustomEvent) {
		application.IOS.SetProximity(mobileEventBool(event.Data, "enabled", false))
	})
	app.Event.On("common:speak", func(event *application.CustomEvent) {
		application.IOS.Speak(mobileEventString(event.Data, "text"))
	})
	app.Event.On("common:stopSpeak", func(*application.CustomEvent) {
		application.IOS.StopSpeak()
	})
	app.Event.On("common:getStorage", func(*application.CustomEvent) {
		app.Event.Emit("common:storage", mobileJSONMap(application.IOS.StorageJSON()))
	})
	app.Event.On("common:getPower", func(*application.CustomEvent) {
		app.Event.Emit("common:power", mobileJSONMap(application.IOS.PowerJSON()))
	})
	app.Event.On("common:getNetwork", func(*application.CustomEvent) {
		app.Event.Emit("common:network", mobileJSONMap(application.IOS.NetworkJSON()))
	})
	app.Event.On("common:watchKeyboard", func(event *application.CustomEvent) {
		application.IOS.SetKeyboardWatch(mobileEventBool(event.Data, "enabled", false))
	})
	app.Event.On("common:setScreenProtect", func(event *application.CustomEvent) {
		application.IOS.SetScreenProtect(mobileEventBool(event.Data, "enabled", false))
	})
	app.Event.On("common:capturePhoto", func(*application.CustomEvent) {
		application.IOS.CapturePhoto()
	})
	app.Event.On("common:captureVideo", func(*application.CustomEvent) {
		application.IOS.CaptureVideo()
	})
	app.Event.On("ios:beginBackgroundTask", func(event *application.CustomEvent) {
		application.IOS.BeginBackgroundTask(int(mobileEventFloat(event.Data, "seconds", 20)))
	})
	app.Event.On("common:startForegroundService", func(*application.CustomEvent) {
		application.IOS.BeginBackgroundTask(30)
	})
	app.Event.On("common:stopForegroundService", func(*application.CustomEvent) {
		application.IOS.EndBackgroundTask()
	})
}

func registerIOSRuntimeEventHandlers(app *application.App) {
	app.Event.On("ios:setScrollEnabled", func(event *application.CustomEvent) {
		application.IOS.SetScrollEnabled(mobileEventBool(event.Data, "enabled", true))
	})
	app.Event.On("ios:setBounceEnabled", func(event *application.CustomEvent) {
		application.IOS.SetBounceEnabled(mobileEventBool(event.Data, "enabled", true))
	})
	app.Event.On("ios:setScrollIndicatorsEnabled", func(event *application.CustomEvent) {
		application.IOS.SetScrollIndicatorsEnabled(mobileEventBool(event.Data, "enabled", true))
	})
	app.Event.On("ios:setBackForwardGesturesEnabled", func(event *application.CustomEvent) {
		application.IOS.SetBackForwardGesturesEnabled(mobileEventBool(event.Data, "enabled", false))
	})
	app.Event.On("ios:setLinkPreviewEnabled", func(event *application.CustomEvent) {
		application.IOS.SetLinkPreviewEnabled(mobileEventBool(event.Data, "enabled", true))
	})
	app.Event.On("ios:setInspectableEnabled", func(event *application.CustomEvent) {
		application.IOS.SetInspectableEnabled(mobileEventBool(event.Data, "enabled", true))
	})
	app.Event.On("ios:setCustomUserAgent", func(event *application.CustomEvent) {
		application.IOS.SetCustomUserAgent(mobileEventString(event.Data, "ua"))
	})
}
