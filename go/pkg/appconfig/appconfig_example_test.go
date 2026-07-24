// SPDX-Licence-Identifier: EUPL-1.2

package appconfig_test

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/appconfig"
)

func ExampleApplicationOptions() {
	options := appconfig.ApplicationOptions()
	core.Println(options.Name, options.Server.Host, options.Server.Port)
	// Output:
	// lthn localhost 8080
}

func ExampleWebviewWindowOptions() {
	options := appconfig.WebviewWindowOptions(
		"tray-popover",
		"tray-panel",
		"Lethean Desktop",
		"/#/tray",
	)
	core.Println(options.Name, options.Width, options.Height, options.AlwaysOnTop)
	// Output:
	// tray-panel 400 560 true
}
