// SPDX-Licence-Identifier: EUPL-1.2

package desktop

import (
	core "dappco.re/go"
	guiwindow "dappco.re/go/render/display/webkit/pkg/window"
	"github.com/wailsapp/wails/v3/pkg/application"
)

func TestWindowPlatform_ConfiguredOptions_Good_TrayProfileApplied(t *core.T) {
	options := configuredWebviewWindowOptions(
		trayPanelWindowSpec().ToPlatformOptions(),
		nil,
	)

	core.AssertEqual(t, "tray-panel", options.Name)
	core.AssertEqual(t, 400, options.Width)
	core.AssertEqual(t, 560, options.Height)
	core.AssertTrue(t, options.AlwaysOnTop)
	core.AssertTrue(t, options.Hidden)
	core.AssertTrue(t, options.Windows.HiddenOnTaskbar)
	core.AssertEqual(t, uint16(50), options.Windows.WindowDidMoveDebounceMS)
	core.AssertEqual(t, uint16(0), options.Linux.WindowDidMoveDebounceMS)
	core.AssertFalse(t, options.AllowSimpleEventEmit)
	core.AssertFalse(t, options.Windows.GeneralAutofillEnabled)
	core.AssertFalse(t, options.Windows.PasswordAutosaveEnabled)
}

func TestWindowPlatform_ConfiguredOptions_Bad_UnknownWindowUsesSafeDefaults(t *core.T) {
	options := configuredWebviewWindowOptions(guiwindow.PlatformWindowOptions{
		Name:   "utility",
		Title:  "Utility",
		URL:    "/utility",
		Width:  720,
		Height: 480,
	}, nil)

	core.AssertEqual(t, 720, options.Width)
	core.AssertEqual(t, 480, options.Height)
	core.AssertFalse(t, options.AllowSimpleEventEmit)
	core.AssertNil(t, options.Permissions)
	core.AssertNil(t, options.Screen)
	core.AssertEqual(t, application.WindowCentered, options.InitialPosition)
}

func TestWindowPlatform_ConfiguredOptions_Ugly_SavedPositionTargetsScreen(t *core.T) {
	source := angularWindowSpec("app", "Lethean Desktop", "/#/").ToPlatformOptions()
	source.X = 120
	source.Y = 80
	screen := &application.Screen{}

	options := configuredWebviewWindowOptions(source, screen)

	core.AssertEqual(t, application.WindowXY, options.InitialPosition)
	core.AssertEqual(t, 120, options.X)
	core.AssertEqual(t, 80, options.Y)
	core.AssertEqual(t, screen, options.Screen)
}
