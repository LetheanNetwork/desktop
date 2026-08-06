// SPDX-Licence-Identifier: EUPL-1.2

package appconfig_test

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/appconfig"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// ─── resolveNativeTheme ─────────────────────────────────────────────────

func TestWebviewWindowOptions_Good_NativeThemeDark(t *core.T) {
	_, cfg := newConfigFixture(t)
	core.RequireTrue(t, cfg.Set("desktop.theme.native", "dark").OK)

	options := appconfig.WebviewWindowOptions("main", "app", "Lethean", "/#/", cfg)

	core.AssertEqual(t, application.Dark, options.Windows.Theme)
	core.AssertEqual(t, application.NSAppearanceNameDarkAqua, options.Mac.Appearance)
}

func TestWebviewWindowOptions_Good_NativeThemeLight(t *core.T) {
	_, cfg := newConfigFixture(t)
	core.RequireTrue(t, cfg.Set("desktop.theme.native", "light").OK)

	options := appconfig.WebviewWindowOptions("main", "app", "Lethean", "/#/", cfg)

	core.AssertEqual(t, application.Light, options.Windows.Theme)
	core.AssertEqual(t, application.NSAppearanceNameAqua, options.Mac.Appearance)
}

func TestWebviewWindowOptions_Good_NativeThemeSystem(t *core.T) {
	_, cfg := newConfigFixture(t)
	core.RequireTrue(t, cfg.Set("desktop.theme.native", "system").OK)

	options := appconfig.WebviewWindowOptions("main", "app", "Lethean", "/#/", cfg)

	core.AssertEqual(t, application.SystemDefault, options.Windows.Theme)
	core.AssertEqual(t, application.DefaultAppearance, options.Mac.Appearance)
}

func TestWebviewWindowOptions_Bad_NativeThemeUnrecognisedValueLeavesWailsDefault(t *core.T) {
	_, cfg := newConfigFixture(t)
	core.RequireTrue(t, cfg.Set("desktop.theme.native", "rainbow").OK)

	options := appconfig.WebviewWindowOptions("main", "app", "Lethean", "/#/", cfg)

	core.AssertEqual(t, application.SystemDefault, options.Windows.Theme)
	core.AssertEqual(t, application.DefaultAppearance, options.Mac.Appearance)
}

func TestWebviewWindowOptions_Bad_NativeThemeUnset(t *core.T) {
	_, cfg := newConfigFixture(t)

	options := appconfig.WebviewWindowOptions("main", "app", "Lethean", "/#/", cfg)

	core.AssertEqual(t, application.SystemDefault, options.Windows.Theme)
	core.AssertEqual(t, application.DefaultAppearance, options.Mac.Appearance)
}

// ─── resolvePermissions ─────────────────────────────────────────────────

func TestWebviewWindowOptions_Good_PermissionsAllowAndDeny(t *core.T) {
	_, cfg := newConfigFixture(t)
	core.RequireTrue(t, cfg.Set("desktop.permissions.microphone", "allow").OK)
	core.RequireTrue(t, cfg.Set("desktop.permissions.camera", "deny").OK)
	core.RequireTrue(t, cfg.Set("desktop.permissions.geolocation", "allow").OK)
	core.RequireTrue(t, cfg.Set("desktop.permissions.notifications", "deny").OK)
	core.RequireTrue(t, cfg.Set("desktop.permissions.clipboard_read", "allow").OK)

	options := appconfig.WebviewWindowOptions("main", "app", "Lethean", "/#/", cfg)

	core.RequireTrue(t, options.Permissions != nil)
	core.AssertEqual(t, application.PermissionAllow, options.Permissions[application.PermissionMicrophone])
	core.AssertEqual(t, application.PermissionDeny, options.Permissions[application.PermissionCamera])
	core.AssertEqual(t, application.PermissionAllow, options.Permissions[application.PermissionGeolocation])
	core.AssertEqual(t, application.PermissionDeny, options.Permissions[application.PermissionNotifications])
	core.AssertEqual(t, application.PermissionAllow, options.Permissions[application.PermissionClipboardRead])
}

func TestWebviewWindowOptions_Bad_PermissionsDefaultAndUnrecognisedStayUnset(t *core.T) {
	_, cfg := newConfigFixture(t)
	// "default" explicitly clears any entry; an unrecognised policy value
	// is skipped entirely (neither set nor cleared).
	core.RequireTrue(t, cfg.Set("desktop.permissions.microphone", "default").OK)
	core.RequireTrue(t, cfg.Set("desktop.permissions.camera", "not-a-policy").OK)

	options := appconfig.WebviewWindowOptions("main", "app", "Lethean", "/#/", cfg)

	core.AssertNil(t, options.Permissions)
}

func TestWebviewWindowOptions_Ugly_PermissionDefaultAfterAnEarlierAllowStaysScoped(t *core.T) {
	_, cfg := newConfigFixture(t)
	// microphone resolves first in the fixed permission order and allocates
	// the map; camera's "default" then must delete only its own entry
	// (which never existed) rather than touching microphone's grant.
	core.RequireTrue(t, cfg.Set("desktop.permissions.microphone", "allow").OK)
	core.RequireTrue(t, cfg.Set("desktop.permissions.camera", "default").OK)

	options := appconfig.WebviewWindowOptions("main", "app", "Lethean", "/#/", cfg)

	core.RequireTrue(t, options.Permissions != nil)
	core.AssertEqual(t, 1, len(options.Permissions))
	core.AssertEqual(t, application.PermissionAllow, options.Permissions[application.PermissionMicrophone])
	_, cameraPresent := options.Permissions[application.PermissionCamera]
	core.AssertFalse(t, cameraPresent)
}

// ─── resolveOptionalBool / mac webview preferences ───────────────────────

func TestWebviewWindowOptions_Good_MacWebviewPreferencesOptionalBooleans(t *core.T) {
	_, cfg := newConfigFixture(t)
	core.RequireTrue(t, cfg.Set("desktop.wails.window.default.mac.webview_preferences.tab_focuses_links", true).OK)
	core.RequireTrue(t, cfg.Set("desktop.wails.window.default.mac.webview_preferences.text_interaction_enabled", false).OK)
	core.RequireTrue(t, cfg.Set("desktop.wails.window.default.mac.webview_preferences.fullscreen_enabled", true).OK)
	core.RequireTrue(t, cfg.Set("desktop.wails.window.default.mac.webview_preferences.allows_magnification", true).OK)
	core.RequireTrue(t, cfg.Set("desktop.wails.window.default.mac.webview_preferences.allows_air_play_for_media_playback", true).OK)
	core.RequireTrue(t, cfg.Set("desktop.wails.window.default.mac.webview_preferences.java_script_can_open_windows_automatically", true).OK)
	core.RequireTrue(t, cfg.Set("desktop.wails.window.default.mac.webview_preferences.enable_autoplay_without_user_action", true).OK)
	core.RequireTrue(t, cfg.Set("desktop.wails.window.default.mac.webview_preferences.minimum_font_size", float64(14)).OK)

	options := appconfig.WebviewWindowOptions("unknown", "utility", "Utility", "/utility", cfg)

	core.AssertTrue(t, options.Mac.WebviewPreferences.TabFocusesLinks.IsSet())
	core.AssertTrue(t, options.Mac.WebviewPreferences.TabFocusesLinks.Get())
	core.AssertTrue(t, options.Mac.WebviewPreferences.TextInteractionEnabled.IsSet())
	core.AssertFalse(t, options.Mac.WebviewPreferences.TextInteractionEnabled.Get())
	core.AssertTrue(t, options.Mac.WebviewPreferences.FullscreenEnabled.IsSet())
	core.AssertTrue(t, options.Mac.WebviewPreferences.AllowsMagnification.IsSet())
	core.AssertTrue(t, options.Mac.WebviewPreferences.AllowsAirPlayForMediaPlayback.IsSet())
	core.AssertTrue(t, options.Mac.WebviewPreferences.JavaScriptCanOpenWindowsAutomatically.IsSet())
	core.AssertTrue(t, options.Mac.WebviewPreferences.EnableAutoplayWithoutUserAction.IsSet())
	core.AssertEqual(t, float64(14), options.Mac.WebviewPreferences.MinimumFontSize.Get())
}

func TestWebviewWindowOptions_Bad_MacWebviewPreferencesUnsetStaysUnset(t *core.T) {
	_, cfg := newConfigFixture(t)

	options := appconfig.WebviewWindowOptions("unknown", "utility", "Utility", "/utility", cfg)

	core.AssertFalse(t, options.Mac.WebviewPreferences.TabFocusesLinks.IsSet())
	core.AssertFalse(t, options.Mac.WebviewPreferences.MinimumFontSize.IsSet())
}
