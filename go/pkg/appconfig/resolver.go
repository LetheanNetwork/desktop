// SPDX-Licence-Identifier: EUPL-1.2

package appconfig

import (
	"reflect"
	"sync"

	core "dappco.re/go"
	"dappco.re/go/config"
	"github.com/wailsapp/wails/v3/pkg/application"
)

const (
	applicationConfigPrefix = "desktop.wails.application"
	windowConfigPrefix      = "desktop.wails.window"
)

var applicationRuntimeFields = map[string]bool{
	"icon":                         true,
	"services":                     true,
	"marshal_error":                true,
	"bind_aliases":                 true,
	"logger":                       true,
	"assets.handler":               true,
	"assets.middleware":            true,
	"flags":                        true,
	"panic_handler":                true,
	"key_bindings":                 true,
	"on_shutdown":                  true,
	"post_shutdown":                true,
	"should_quit":                  true,
	"raw_message_handler":          true,
	"warning_handler":              true,
	"error_handler":                true,
	"transport":                    true,
	"windows.wnd_proc_interceptor": true,
	"single_instance.on_second_instance_launch": true,
	"single_instance.additional_data":           true,
	"single_instance.encryption_key":            true,
}

var windowRuntimeFields = map[string]bool{
	"name":                  true,
	"title":                 true,
	"url":                   true,
	"html":                  true,
	"js":                    true,
	"css":                   true,
	"screen":                true,
	"key_bindings":          true,
	"mac.event_mapping":     true,
	"windows.event_mapping": true,
	"windows.menu":          true,
	"windows.window_mask":   true,
	"linux.icon":            true,
	"linux.menu":            true,
}

func firstConfig(configs []*config.Service) *config.Service {
	for _, candidate := range configs {
		if candidate != nil && candidate.Config() != nil {
			return candidate
		}
	}
	return nil
}

func resolveApplicationOptions(cfg *config.Service, options *application.Options) {
	if cfg == nil || options == nil {
		return
	}
	resolveConfigTree(cfg, applicationConfigPrefix, options, applicationRuntimeFields)
}

func resolveWindowOptions(
	cfg *config.Service,
	profile string,
	options *application.WebviewWindowOptions,
) {
	if cfg == nil || options == nil {
		return
	}
	resolveConfigTree(
		cfg,
		windowConfigPrefix+".default",
		options,
		windowRuntimeFields,
	)
	resolveWindowOptionalValues(cfg, windowConfigPrefix+".default", options)

	if segment := configSegment(profile); segment != "" && segment != "unknown" {
		prefix := windowConfigPrefix + "." + segment
		resolveConfigTree(cfg, prefix, options, windowRuntimeFields)
		resolveWindowOptionalValues(cfg, prefix, options)
	}
	resolveNativeTheme(cfg, options)
	resolvePermissions(cfg, options)
}

func resolveConfigTree(
	cfg *config.Service,
	prefix string,
	target any,
	blocked map[string]bool,
) {
	value := reflect.ValueOf(target)
	if value.Kind() != reflect.Pointer || value.IsNil() {
		return
	}
	resolveConfigValue(cfg, prefix, "", value.Elem(), blocked)
}

func resolveConfigValue(
	cfg *config.Service,
	key string,
	relative string,
	value reflect.Value,
	blocked map[string]bool,
) {
	if !value.IsValid() {
		return
	}
	if relative != "" && blocked[relative] {
		return
	}
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			if !value.CanSet() {
				return
			}
			candidate := reflect.New(value.Type().Elem())
			if !cfg.Get(key, candidate.Interface()).OK {
				return
			}
			value.Set(candidate)
		}
		resolveConfigValue(cfg, key, relative, value.Elem(), blocked)
		return
	}

	switch value.Kind() {
	case reflect.Struct:
		valueType := value.Type()
		for index := 0; index < value.NumField(); index++ {
			fieldInfo := valueType.Field(index)
			field := value.Field(index)
			if fieldInfo.PkgPath != "" || !field.CanAddr() {
				continue
			}
			name := snakeName(fieldInfo.Name)
			nextRelative := joinKey(relative, name)
			if blocked[nextRelative] {
				continue
			}
			resolveConfigValue(
				cfg,
				joinKey(key, name),
				nextRelative,
				field,
				blocked,
			)
		}
	case reflect.Bool,
		reflect.Int,
		reflect.Int8,
		reflect.Int16,
		reflect.Int32,
		reflect.Int64,
		reflect.Uint,
		reflect.Uint8,
		reflect.Uint16,
		reflect.Uint32,
		reflect.Uint64,
		reflect.Float32,
		reflect.Float64,
		reflect.String:
		resolveConfigLeaf(cfg, key, value)
	case reflect.Slice:
		if value.Type().Elem().Kind() == reflect.Uint8 {
			return
		}
		if serialisableScalar(value.Type().Elem().Kind()) {
			resolveConfigLeaf(cfg, key, value)
		}
	case reflect.Map:
		if serialisableScalar(value.Type().Key().Kind()) &&
			serialisableScalar(value.Type().Elem().Kind()) {
			resolveConfigLeaf(cfg, key, value)
		}
	}
}

func resolveConfigLeaf(cfg *config.Service, key string, value reflect.Value) {
	if !value.CanSet() {
		return
	}
	candidate := reflect.New(value.Type())
	candidate.Elem().Set(value)
	if !cfg.Get(key, candidate.Interface()).OK {
		return
	}
	value.Set(candidate.Elem())
}

func serialisableScalar(kind reflect.Kind) bool {
	switch kind {
	case reflect.Bool,
		reflect.Int,
		reflect.Int8,
		reflect.Int16,
		reflect.Int32,
		reflect.Int64,
		reflect.Uint,
		reflect.Uint8,
		reflect.Uint16,
		reflect.Uint32,
		reflect.Uint64,
		reflect.Float32,
		reflect.Float64,
		reflect.String:
		return true
	default:
		return false
	}
}

func resolveWindowOptionalValues(
	cfg *config.Service,
	prefix string,
	options *application.WebviewWindowOptions,
) {
	resolveOptionalBool(
		cfg,
		prefix+".mac.webview_preferences.tab_focuses_links",
		options.Mac.WebviewPreferences.TabFocusesLinks.Set,
	)
	resolveOptionalBool(
		cfg,
		prefix+".mac.webview_preferences.text_interaction_enabled",
		options.Mac.WebviewPreferences.TextInteractionEnabled.Set,
	)
	resolveOptionalBool(
		cfg,
		prefix+".mac.webview_preferences.fullscreen_enabled",
		options.Mac.WebviewPreferences.FullscreenEnabled.Set,
	)
	resolveOptionalBool(
		cfg,
		prefix+".mac.webview_preferences.allows_back_forward_navigation_gestures",
		options.Mac.WebviewPreferences.AllowsBackForwardNavigationGestures.Set,
	)
	resolveOptionalBool(
		cfg,
		prefix+".mac.webview_preferences.allows_magnification",
		options.Mac.WebviewPreferences.AllowsMagnification.Set,
	)
	resolveOptionalBool(
		cfg,
		prefix+".mac.webview_preferences.allows_air_play_for_media_playback",
		options.Mac.WebviewPreferences.AllowsAirPlayForMediaPlayback.Set,
	)
	resolveOptionalBool(
		cfg,
		prefix+".mac.webview_preferences.java_script_can_open_windows_automatically",
		options.Mac.WebviewPreferences.JavaScriptCanOpenWindowsAutomatically.Set,
	)
	resolveOptionalBool(
		cfg,
		prefix+".mac.webview_preferences.enable_autoplay_without_user_action",
		options.Mac.WebviewPreferences.EnableAutoplayWithoutUserAction.Set,
	)
	var minimumFontSize float64
	if cfg.Get(
		prefix+".mac.webview_preferences.minimum_font_size",
		&minimumFontSize,
	).OK {
		options.Mac.WebviewPreferences.MinimumFontSize.Set(minimumFontSize)
	}
}

func resolveOptionalBool(
	cfg *config.Service,
	key string,
	set func(bool),
) {
	var value bool
	if cfg.Get(key, &value).OK {
		set(value)
	}
}

func resolveNativeTheme(
	cfg *config.Service,
	options *application.WebviewWindowOptions,
) {
	var mode string
	if !cfg.Get("desktop.theme.native", &mode).OK {
		return
	}
	switch core.Lower(mode) {
	case "dark":
		options.Windows.Theme = application.Dark
		options.Mac.Appearance = application.NSAppearanceNameDarkAqua
	case "light":
		options.Windows.Theme = application.Light
		options.Mac.Appearance = application.NSAppearanceNameAqua
	case "system":
		options.Windows.Theme = application.SystemDefault
		options.Mac.Appearance = application.DefaultAppearance
	}
}

func resolvePermissions(
	cfg *config.Service,
	options *application.WebviewWindowOptions,
) {
	permissions := []struct {
		name string
		kind application.PermissionType
	}{
		{name: "microphone", kind: application.PermissionMicrophone},
		{name: "camera", kind: application.PermissionCamera},
		{name: "geolocation", kind: application.PermissionGeolocation},
		{name: "notifications", kind: application.PermissionNotifications},
		{name: "clipboard_read", kind: application.PermissionClipboardRead},
	}
	for _, permission := range permissions {
		var policy string
		if !cfg.Get("desktop.permissions."+permission.name, &policy).OK {
			continue
		}
		value := application.PermissionDefault
		switch core.Lower(policy) {
		case "allow":
			value = application.PermissionAllow
		case "deny":
			value = application.PermissionDeny
		case "default":
		default:
			continue
		}
		if value == application.PermissionDefault {
			if options.Permissions != nil {
				delete(options.Permissions, permission.kind)
			}
			continue
		}
		if options.Permissions == nil {
			options.Permissions = make(map[application.PermissionType]application.Permission)
		}
		options.Permissions[permission.kind] = value
	}
	if len(options.Permissions) == 0 {
		options.Permissions = nil
	}
}

func configSegment(value string) string {
	data := []byte(value)
	for index, current := range data {
		if current == '-' {
			data[index] = '_'
		}
	}
	return snakeName(string(data))
}

// snakeNameCache memoises snakeName by input — the Wails options
// struct shapes are fixed at compile time, so every resolveConfigValue
// walk re-converts the SAME field names (once per prefix: a single
// WebviewWindowOptions call already walks its struct tree twice, for
// the ".default" prefix and the profile-specific one) into the exact
// same snake_case string every time. snakeName is a pure function of
// its input, so caching by input string is always correct — never a
// behavioural change, only skips redoing the same byte-slice build. A
// plain mutex-guarded map is used rather than sync.Map: sync.Map's
// Load/Store take `key any`, which boxes the string key into an
// interface on every call and erases most of the saving; a native
// map[string]string hashes the string directly.
var (
	snakeNameCacheMu sync.RWMutex
	snakeNameCache   = make(map[string]string, 256)
)

func snakeName(value string) string {
	if value == "" {
		return ""
	}
	snakeNameCacheMu.RLock()
	cached, ok := snakeNameCache[value]
	snakeNameCacheMu.RUnlock()
	if ok {
		return cached
	}
	input := []byte(value)
	output := make([]byte, 0, len(input)+4)
	for index, current := range input {
		isUpper := current >= 'A' && current <= 'Z'
		if isUpper {
			previousLower := index > 0 &&
				((input[index-1] >= 'a' && input[index-1] <= 'z') ||
					(input[index-1] >= '0' && input[index-1] <= '9'))
			nextLower := index+1 < len(input) &&
				input[index+1] >= 'a' &&
				input[index+1] <= 'z'
			previousUpper := index > 0 &&
				input[index-1] >= 'A' &&
				input[index-1] <= 'Z'
			if len(output) > 0 && (previousLower || (previousUpper && nextLower)) {
				output = append(output, '_')
			}
			current += 'a' - 'A'
		}
		output = append(output, current)
	}
	result := string(output)
	snakeNameCacheMu.Lock()
	snakeNameCache[value] = result
	snakeNameCacheMu.Unlock()
	return result
}

func joinKey(prefix, key string) string {
	if prefix == "" {
		return key
	}
	if key == "" {
		return prefix
	}
	return prefix + "." + key
}
