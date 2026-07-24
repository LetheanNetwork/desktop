// SPDX-Licence-Identifier: EUPL-1.2

//go:build ios || android

package main

import (
	core "dappco.re/go"
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

	app := application.New(application.Options{
		Name:        "Lethean",
		Description: "Local AI, sovereign by design.",
		Icon:        appIcon,
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Flags: map[string]any{
			"lthn":   true,
			"mobile": true,
		},
		IOS: application.IOSOptions{
			BackgroundColour: background,
			DisableBounce:    true,
		},
		Android: application.AndroidOptions{
			BackgroundColour: background,
		},
	})

	registerMobileRuntimeEvents(app)
	registerMobileNativeFeatures(app)
	registerIOSRuntimeEventHandlers(app)

	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "Lethean",
		BackgroundColour: background,
		URL:              "/",
	})

	if err := app.Run(); err != nil {
		return core.Fail(core.E("lthn.mobile", "run Wails application", err))
	}
	return core.Ok(nil)
}
