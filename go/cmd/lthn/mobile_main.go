// SPDX-Licence-Identifier: EUPL-1.2

//go:build ios || android

package main

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/appconfig"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// main is the shared iOS and Android entrypoint. Wails' generated build
// overlays invoke it from the native application lifecycle after UIKit or the
// Android activity has started.
func main() {
	if result := runMobile(); !result.OK {
		core.Print(core.Stderr(), "lthn mobile: %s\n", result.Error())
	}
}

func runMobile() core.Result {
	assetsResult := core.Sub(frontendDist, "dist")
	if !assetsResult.OK {
		return core.Fail(core.E("lthn.mobile", "load embedded Angular assets", assetsResult.Value.(error)))
	}
	assets := assetsResult.Value.(core.FS)
	background := application.NewRGB(8, 12, 18)

	options := appconfig.ApplicationOptions()
	options.Name = "Lethean"
	options.Description = "Local AI, sovereign by design."
	options.Icon = appIcon
	options.Assets.Handler = application.AssetFileServerFS(assets)
	options.Flags = map[string]any{
		"lthn":   true,
		"mobile": true,
	}
	options.IOS.BackgroundColour = background
	options.IOS.DisableBounce = true
	options.Android.BackgroundColour = background
	// Mobile lifecycle ownership is native, so desktop instance hand-off is disabled.
	options.SingleInstance = nil

	app := application.New(options)

	registerMobileRuntimeEvents(app)
	registerMobileNativeFeatures(app)
	registerIOSRuntimeEventHandlers(app)

	windowOptions := appconfig.WebviewWindowOptions("mobile", "", "Lethean", "/")
	windowOptions.BackgroundColour = background
	app.Window.NewWithOptions(windowOptions)

	if err := app.Run(); err != nil {
		return core.Fail(core.E("lthn.mobile", "run Wails application", err))
	}
	return core.Ok(nil)
}
