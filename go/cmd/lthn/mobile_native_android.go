// SPDX-Licence-Identifier: EUPL-1.2

//go:build android

package main

import "github.com/wailsapp/wails/v3/pkg/application"

func registerMobileNativeFeatures(app *application.App) {
	app.Event.On("common:share", func(event *application.CustomEvent) {
		application.Android.Share(mobilePayloadJSON(event.Data))
	})
	app.Event.On("common:openURL", func(event *application.CustomEvent) {
		application.Android.OpenURL(mobileEventString(event.Data, "url"))
	})
	app.Event.On("common:keepAwake", func(event *application.CustomEvent) {
		application.Android.SetKeepAwake(mobileEventBool(event.Data, "enabled", false))
	})
	app.Event.On("common:torch", func(event *application.CustomEvent) {
		if payload := mobileFirstMap(event.Data); payload != nil {
			if _, result := payload["available"]; result {
				return
			}
		}
		application.Android.SetTorch(mobileEventBool(event.Data, "enabled", false))
	})
	app.Event.On("common:getSafeArea", func(*application.CustomEvent) {
		app.Event.Emit("common:safeArea", mobileJSONMap(application.Android.SafeAreaJSON()))
	})
	app.Event.On("common:setBrightness", func(event *application.CustomEvent) {
		application.Android.SetBrightness(int(mobileEventFloat(event.Data, "value", 0.5) * 100))
	})
	app.Event.On("common:getBrightness", func(*application.CustomEvent) {
		app.Event.Emit("common:brightness", mobileJSONMap(application.Android.BrightnessJSON()))
	})
	app.Event.On("common:getAppInfo", func(*application.CustomEvent) {
		app.Event.Emit("common:appInfo", mobileJSONMap(application.Android.AppInfoJSON()))
	})
	app.Event.On("common:setOrientation", func(event *application.CustomEvent) {
		application.Android.SetOrientation(mobileEventString(event.Data, "mode"))
	})
	app.Event.On("common:getOrientation", func(*application.CustomEvent) {
		app.Event.Emit("common:orientation", mobileJSONMap(application.Android.OrientationJSON()))
	})
	app.Event.On("common:setStatusBar", func(event *application.CustomEvent) {
		application.Android.SetStatusBar(mobilePayloadJSON(event.Data))
	})
	app.Event.On("common:authenticate", func(event *application.CustomEvent) {
		reason := mobileEventString(event.Data, "reason")
		if reason == "" {
			reason = "Authenticate to continue"
		}
		application.Android.BiometricAuthenticate(reason)
	})
	app.Event.On("common:notify", func(event *application.CustomEvent) {
		application.Android.Notify(mobilePayloadJSON(event.Data))
	})
	app.Event.On("common:secureSet", func(event *application.CustomEvent) {
		application.Android.SecureSet(mobilePayloadJSON(event.Data))
	})
	app.Event.On("common:secureGet", func(event *application.CustomEvent) {
		key := mobileEventString(event.Data, "key")
		app.Event.Emit("common:secureValue", map[string]any{
			"key": key, "value": application.Android.SecureGet(key),
		})
	})
	app.Event.On("common:secureDelete", func(event *application.CustomEvent) {
		application.Android.SecureDelete(mobileEventString(event.Data, "key"))
	})
	app.Event.On("common:haptic", func(event *application.CustomEvent) {
		application.Android.Haptic(mobileEventString(event.Data, "type"))
	})
	app.Event.On("common:getLocation", func(*application.CustomEvent) {
		application.Android.GetLocation()
	})
	app.Event.On("common:watchMotion", func(event *application.CustomEvent) {
		application.Android.SetMotion(mobileEventBool(event.Data, "enabled", false))
	})
	app.Event.On("common:watchProximity", func(event *application.CustomEvent) {
		application.Android.SetProximity(mobileEventBool(event.Data, "enabled", false))
	})
	app.Event.On("common:speak", func(event *application.CustomEvent) {
		application.Android.Speak(mobileEventString(event.Data, "text"))
	})
	app.Event.On("common:stopSpeak", func(*application.CustomEvent) {
		application.Android.StopSpeak()
	})
	app.Event.On("common:getStorage", func(*application.CustomEvent) {
		app.Event.Emit("common:storage", mobileJSONMap(application.Android.StorageJSON()))
	})
	app.Event.On("common:getPower", func(*application.CustomEvent) {
		app.Event.Emit("common:power", mobileJSONMap(application.Android.PowerJSON()))
	})
	app.Event.On("common:getNetwork", func(*application.CustomEvent) {
		app.Event.Emit("common:network", mobileJSONMap(application.Android.NetworkJSON()))
	})
	app.Event.On("common:watchKeyboard", func(event *application.CustomEvent) {
		application.Android.SetKeyboardWatch(mobileEventBool(event.Data, "enabled", false))
	})
	app.Event.On("common:setScreenProtect", func(event *application.CustomEvent) {
		application.Android.SetScreenProtect(mobileEventBool(event.Data, "enabled", false))
	})
	app.Event.On("common:capturePhoto", func(*application.CustomEvent) {
		application.Android.CapturePhoto()
	})
	app.Event.On("common:captureVideo", func(*application.CustomEvent) {
		application.Android.CaptureVideo()
	})
	app.Event.On("common:startForegroundService", func(event *application.CustomEvent) {
		application.Android.StartForegroundService(mobilePayloadJSON(event.Data))
	})
	app.Event.On("common:stopForegroundService", func(*application.CustomEvent) {
		application.Android.StopForegroundService()
	})
}

func registerIOSRuntimeEventHandlers(*application.App) {}
