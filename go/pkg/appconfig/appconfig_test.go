// SPDX-Licence-Identifier: EUPL-1.2

package appconfig_test

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/appconfig"
	"github.com/wailsapp/wails/v3/pkg/application"
)

func TestApplicationOptions_Good_DesktopDefaults(t *core.T) {
	options := appconfig.ApplicationOptions()

	core.AssertEqual(t, "lthn", options.Name)
	core.AssertEqual(t, "Lethean Desktop", options.Description)
	core.AssertEqual(t, application.ActivationPolicyRegular, options.Mac.ActivationPolicy)
	core.AssertEqual(t, "WailsWebviewWindow", options.Windows.WndClass)
	core.AssertEqual(t, []string{".lthn"}, options.FileAssociations)
	core.AssertEqual(t, "localhost", options.Server.Host)
	core.AssertEqual(t, 8080, options.Server.Port)
	core.AssertEqual(t, 30*core.Second, options.Server.ReadTimeout)
	core.AssertEqual(t, 30*core.Second, options.Server.WriteTimeout)
	core.AssertEqual(t, 120*core.Second, options.Server.IdleTimeout)
	core.AssertEqual(t, 30*core.Second, options.Server.ShutdownTimeout)
}

func TestApplicationOptions_Bad_OptionalHooksStayDisabled(t *core.T) {
	options := appconfig.ApplicationOptions()

	core.AssertNil(t, options.MarshalError)
	core.AssertNil(t, options.Logger)
	core.AssertNil(t, options.PanicHandler)
	core.AssertNil(t, options.RawMessageHandler)
	core.AssertNil(t, options.WarningHandler)
	core.AssertNil(t, options.ErrorHandler)
	core.AssertNil(t, options.Server.TLS)
	core.AssertFalse(t, options.DisableDefaultSignalHandler)
}

func TestApplicationOptions_Ugly_AllPlatformGroupsInitialised(t *core.T) {
	options := appconfig.ApplicationOptions()

	core.AssertEqual(t, "ai.lthn.desktop", options.SingleInstance.UniqueID)
	core.AssertEqual(t, 0, options.SingleInstance.ExitCode)
	core.AssertEqual(t, [32]byte{}, options.SingleInstance.EncryptionKey)
	core.AssertFalse(t, options.IOS.DisableInputAccessoryView)
	core.AssertFalse(t, options.IOS.EnableNativeTabs)
	core.AssertNil(t, options.IOS.NativeTabsItems)
	core.AssertFalse(t, options.Android.DisableHardwareAcceleration)
	core.AssertEqual(t, application.RGBA{}, options.IOS.BackgroundColour)
	core.AssertEqual(t, application.RGBA{}, options.Android.BackgroundColour)
}

func TestWebviewWindowOptions_Good_MainWindow(t *core.T) {
	options := appconfig.WebviewWindowOptions("main", "app", "Lethean Desktop", "/#/")

	core.AssertEqual(t, "app", options.Name)
	core.AssertEqual(t, "Lethean Desktop", options.Title)
	core.AssertEqual(t, "/#/", options.URL)
	core.AssertEqual(t, 1440, options.Width)
	core.AssertEqual(t, 900, options.Height)
	core.AssertEqual(t, 1000, options.MinWidth)
	core.AssertEqual(t, 680, options.MinHeight)
	core.AssertTrue(t, options.Frameless)
	core.AssertTrue(t, options.EnableFileDrop)
	core.AssertTrue(t, options.DefaultContextMenuDisabled)
	core.AssertEqual(t, 40, options.Mac.InvisibleTitleBarHeight)
	core.AssertFalse(t, options.AlwaysOnTop)
	core.AssertFalse(t, options.Windows.HiddenOnTaskbar)
}

func TestWebviewWindowOptions_Bad_UnknownProfileUsesWailsDefaults(t *core.T) {
	options := appconfig.WebviewWindowOptions("unknown", "utility", "Utility", "/utility")

	core.AssertEqual(t, "utility", options.Name)
	core.AssertEqual(t, 800, options.Width)
	core.AssertEqual(t, 600, options.Height)
	core.AssertFalse(t, options.Frameless)
	core.AssertEqual(t, application.WindowStateNormal, options.StartState)
	core.AssertEqual(t, application.BackgroundTypeSolid, options.BackgroundType)
	core.AssertEqual(t, application.Auto, options.Windows.BackdropType)
	core.AssertEqual(t, application.SystemDefault, options.Windows.Theme)
	core.AssertEqual(t, application.WebviewGpuPolicyAlways, options.Linux.WebviewGpuPolicy)
	core.AssertNil(t, options.Permissions)
	core.AssertNil(t, options.Windows.Permissions)
}

func TestWebviewWindowOptions_Ugly_TrayAndTearOffPreserveProfiles(t *core.T) {
	tray := appconfig.WebviewWindowOptions(
		"tray-popover",
		"tray-panel",
		"Lethean Desktop",
		"/#/tray",
	)
	tearOff := appconfig.WebviewWindowOptions(
		"tear-off",
		"app-view-chat",
		"Lethean Desktop · chat",
		"/#/w/chat",
	)

	core.AssertEqual(t, 400, tray.Width)
	core.AssertEqual(t, 560, tray.Height)
	core.AssertEqual(t, 400, tray.MaxWidth)
	core.AssertEqual(t, 560, tray.MaxHeight)
	core.AssertTrue(t, tray.AlwaysOnTop)
	core.AssertTrue(t, tray.Hidden)
	core.AssertTrue(t, tray.DisableResize)
	core.AssertTrue(t, tray.HideOnFocusLost)
	core.AssertTrue(t, tray.HideOnEscape)
	core.AssertTrue(t, tray.Windows.HiddenOnTaskbar)
	core.AssertEqual(t, application.MacWindowLevelPopUpMenu, tray.Mac.WindowLevel)
	core.AssertTrue(t, tray.Mac.WebviewPreferences.AllowsBackForwardNavigationGestures.IsSet())
	core.AssertFalse(t, tray.Mac.WebviewPreferences.AllowsBackForwardNavigationGestures.Get())

	core.AssertEqual(t, 1440, tearOff.Width)
	core.AssertEqual(t, 900, tearOff.Height)
	core.AssertTrue(t, tearOff.Frameless)
	core.AssertTrue(t, tearOff.EnableFileDrop)
	core.AssertFalse(t, tearOff.AlwaysOnTop)
	core.AssertFalse(t, tearOff.Hidden)
}
