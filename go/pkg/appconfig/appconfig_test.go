// SPDX-Licence-Identifier: EUPL-1.2

package appconfig_test

import (
	core "dappco.re/go"
	"dappco.re/go/config"
	"dappco.re/lthn/desktop/pkg/appconfig"
	"github.com/wailsapp/wails/v3/pkg/application"
)

func newConfigFixture(t *core.T) (*core.Core, *config.Service) {
	t.Helper()
	path := core.PathJoin(t.TempDir(), "lthn.yaml")
	c := core.New(
		core.WithName("config", config.NewConfigServiceWith(config.ServiceOptions{
			Path: path,
		})),
	)
	core.RequireTrue(t, c.ServiceStartup(core.Background(), nil).OK)
	t.Cleanup(func() {
		_ = c.ServiceShutdown(core.Background())
	})
	cfg, ok := core.ServiceFor[*config.Service](c, "config")
	core.RequireTrue(t, ok)
	core.AssertNotNil(t, cfg)
	return c, cfg
}

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
	// 36 matches the Angular menu bar's own height — the two have to agree or
	// the native drag region and the painted bar cover different pixels.
	core.AssertEqual(t, 36, options.Mac.InvisibleTitleBarHeight)
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

	// One application, not a second desktop: the shell's chrome, the shell's
	// invisible title bar, and a minimum an application window can actually
	// honour — a carried size below the declared minimum never arrives.
	core.AssertEqual(t, 900, tearOff.Width)
	core.AssertEqual(t, 640, tearOff.Height)
	core.AssertEqual(t, 320, tearOff.MinWidth)
	core.AssertEqual(t, 320, tearOff.MinHeight)
	core.AssertTrue(t, tearOff.Frameless)
	core.AssertTrue(t, tearOff.EnableFileDrop)
	core.AssertTrue(t, tearOff.DefaultContextMenuDisabled)
	core.AssertEqual(t, 36, tearOff.Mac.InvisibleTitleBarHeight)
	core.AssertFalse(t, tearOff.AlwaysOnTop)
	core.AssertFalse(t, tearOff.Hidden)
}

func TestApplicationOptions_Good_ConfigOverride(t *core.T) {
	_, cfg := newConfigFixture(t)
	core.RequireTrue(t, cfg.Set("desktop.wails.application.server.port", 9245).OK)
	core.RequireTrue(t, cfg.Set("desktop.wails.application.windows.use_visual_hosting", true).OK)
	core.RequireTrue(t, cfg.Set(
		"desktop.wails.application.server.tls.cert_file",
		"/tmp/lthn-cert.pem",
	).OK)

	options := appconfig.ApplicationOptions(cfg)

	core.AssertEqual(t, 9245, options.Server.Port)
	core.AssertTrue(t, options.Windows.UseVisualHosting)
	core.AssertEqual(t, "localhost", options.Server.Host)
	core.AssertNotNil(t, options.Server.TLS)
	core.AssertEqual(t, "/tmp/lthn-cert.pem", options.Server.TLS.CertFile)
}

func TestApplicationOptions_Bad_InvalidConfigUsesFallback(t *core.T) {
	_, cfg := newConfigFixture(t)
	core.RequireTrue(t, cfg.Set("desktop.wails.application.server.port", "not-a-port").OK)

	options := appconfig.ApplicationOptions(cfg)

	core.AssertEqual(t, 8080, options.Server.Port)
}

func TestApplicationOptions_Ugly_RuntimeOwnedValuesStayInternal(t *core.T) {
	_, cfg := newConfigFixture(t)
	core.RequireTrue(t, cfg.Set("desktop.wails.application.single_instance.encryption_key", "unsafe").OK)
	core.RequireTrue(t, cfg.Set("desktop.wails.application.services", []string{"unsafe"}).OK)

	options := appconfig.ApplicationOptions(cfg)

	core.AssertEqual(t, [32]byte{}, options.SingleInstance.EncryptionKey)
	core.AssertNil(t, options.Services)
}

func TestWebviewWindowOptions_Good_ConfigOverride(t *core.T) {
	_, cfg := newConfigFixture(t)
	core.RequireTrue(t, cfg.Set("desktop.wails.window.main.width", 1280).OK)
	core.RequireTrue(t, cfg.Set("desktop.wails.window.main.always_on_top", true).OK)

	options := appconfig.WebviewWindowOptions(
		"main",
		"app",
		"Lethean Desktop",
		"/#/",
		cfg,
	)

	core.AssertEqual(t, 1280, options.Width)
	core.AssertTrue(t, options.AlwaysOnTop)
	core.AssertEqual(t, 900, options.Height)
}

func TestWebviewWindowOptions_Bad_InvalidConfigUsesProfileFallback(t *core.T) {
	_, cfg := newConfigFixture(t)
	core.RequireTrue(t, cfg.Set("desktop.wails.window.tray_popover.width", "wide").OK)

	options := appconfig.WebviewWindowOptions(
		"tray-popover",
		"tray-panel",
		"Lethean Desktop",
		"/#/tray",
		cfg,
	)

	core.AssertEqual(t, 400, options.Width)
}

func TestWebviewWindowOptions_Ugly_ProfileOverrideWinsCommonOverride(t *core.T) {
	_, cfg := newConfigFixture(t)
	core.RequireTrue(t, cfg.Set("desktop.wails.window.default.width", 1111).OK)
	core.RequireTrue(t, cfg.Set("desktop.wails.window.main.width", 1222).OK)

	main := appconfig.WebviewWindowOptions("main", "app", "Lethean", "/#/", cfg)
	unknown := appconfig.WebviewWindowOptions("unknown", "utility", "Utility", "/", cfg)

	core.AssertEqual(t, 1222, main.Width)
	core.AssertEqual(t, 1111, unknown.Width)
}
