// SPDX-Licence-Identifier: EUPL-1.2

package appconfig

import (
	core "dappco.re/go"
	"dappco.re/go/config"
	guisystray "dappco.re/go/render/display/webkit/pkg/systray"
	guiwindow "dappco.re/go/render/display/webkit/pkg/window"
)

const (
	mainWindowControlName = "app"
	trayWindowControlName = "tray-panel"
)

// Options configures the desktop-control settings service.
type Options struct {
	Core *core.Core
}

// Service exposes the curated desktop-control catalogue to Wails while using
// the Core-registered config service as its only persistence substrate.
type Service struct {
	core *core.Core
}

// NewService constructs the desktop-control settings service.
//
// Usage example:
//
//	svc := appconfig.NewService(appconfig.Options{Core: c})
func NewService(options Options) *Service {
	return &Service{core: options.Core}
}

// Register provides the canonical zero-option Core service factory.
//
// Usage example:
//
//	core.WithName("appconfig", appconfig.Register)
func Register(c *core.Core) core.Result {
	if c == nil {
		return core.Fail(core.E("appconfig.Register", "core is required", nil))
	}
	return core.Ok(NewService(Options{Core: c}))
}

// Settings returns the grouped, user-facing control catalogue with effective
// resolve-with-fallback values and the canonical persistence path.
func (s *Service) Settings() core.Result {
	cfgResult := s.configService()
	if !cfgResult.OK {
		return cfgResult
	}
	cfg := cfgResult.Value.(*config.Service)
	controls := make([]map[string]any, 0, len(controlDefinitions))
	for _, definition := range controlDefinitions {
		value, configured := definition.value(cfg)
		control := map[string]any{
			"key":              definition.key,
			"group":            definition.group,
			"label":            definition.label,
			"description":      definition.description,
			"kind":             definition.kind,
			"value":            value,
			"default":          definition.defaultValue,
			"configured":       configured,
			"live":             definition.live,
			"restart_required": definition.restartRequired,
		}
		if len(definition.choices) > 0 {
			control["choices"] = definition.choices
		}
		if definition.minimum != nil {
			control["minimum"] = *definition.minimum
		}
		if definition.maximum != nil {
			control["maximum"] = *definition.maximum
		}
		if definition.step != nil {
			control["step"] = *definition.step
		}
		controls = append(controls, control)
	}
	return core.Ok(map[string]any{
		"controls":    controls,
		"config_path": cfg.Config().Path(),
	})
}

// Set validates and persists one user-facing control through config.Service.
// Live-capable CoreGUI actions are dispatched after the durable commit.
func (s *Service) Set(key string, value any) core.Result {
	cfgResult := s.configService()
	if !cfgResult.OK {
		return cfgResult
	}
	definition, ok := controlDefinitionFor(key)
	if !ok {
		return core.Fail(core.E(
			"appconfig.Service.Set",
			"unknown desktop control: "+key,
			nil,
		))
	}
	normalised, valid := definition.normalise(value)
	if !valid {
		return core.Fail(core.E(
			"appconfig.Service.Set",
			"invalid value for "+key,
			nil,
		))
	}
	cfg := cfgResult.Value.(*config.Service)
	if r := cfg.Set(key, normalised); !r.OK {
		return core.Fail(core.E(
			"appconfig.Service.Set",
			"set config value",
			r.Err(),
		))
	}
	if r := cfg.Commit(); !r.OK {
		return core.Fail(core.E(
			"appconfig.Service.Set",
			"persist config value",
			r.Err(),
		))
	}
	s.applyLive(key, normalised, cfg)
	return s.Settings()
}

func (s *Service) configService() core.Result {
	if s == nil || s.core == nil {
		return core.Fail(core.E(
			"appconfig.Service",
			"core is required",
			nil,
		))
	}
	cfg, ok := core.ServiceFor[*config.Service](s.core, "config")
	if !ok || cfg == nil {
		return core.Fail(core.E(
			"appconfig.Service",
			"config service is required",
			nil,
		))
	}
	if cfg.Config() == nil {
		return core.Fail(core.E(
			"appconfig.Service",
			"config service is not started",
			nil,
		))
	}
	return core.Ok(cfg)
}

type controlDefinition struct {
	key             string
	group           string
	label           string
	description     string
	kind            string
	defaultValue    any
	choices         []string
	minimum         *float64
	maximum         *float64
	step            *float64
	live            bool
	restartRequired bool
}

func number(value float64) *float64 {
	return &value
}

var controlDefinitions = []controlDefinition{
	{
		key: "desktop.wails.window.main.width", group: "Window",
		label: "Window width", description: "Width of the main desktop window in pixels.",
		kind: "number", defaultValue: 1440, minimum: number(800), maximum: number(3840),
		step: number(10), live: true,
	},
	{
		key: "desktop.wails.window.main.height", group: "Window",
		label: "Window height", description: "Height of the main desktop window in pixels.",
		kind: "number", defaultValue: 900, minimum: number(600), maximum: number(2160),
		step: number(10), live: true,
	},
	{
		key: "desktop.wails.window.main.always_on_top", group: "Window",
		label: "Always on top", description: "Keep the main window above ordinary windows.",
		kind: "toggle", defaultValue: false, live: true,
	},
	{
		key: "desktop.wails.window.main.zoom", group: "Window",
		label: "WebView zoom", description: "Magnification for the main desktop WebView; zero uses the platform default.",
		kind: "number", defaultValue: float64(0), minimum: number(0), maximum: number(3),
		step: number(0.1), live: true,
	},
	{
		key: "desktop.wails.window.main.content_protection_enabled", group: "Window",
		label: "Content protection", description: "Request operating-system screen-capture protection.",
		kind: "toggle", defaultValue: false, live: true,
	},
	{
		key: "desktop.wails.window.main.enable_file_drop", group: "Window",
		label: "File drop", description: "Allow files to be dropped from the operating system into the desktop.",
		kind: "toggle", defaultValue: true, restartRequired: true,
	},
	{
		key: "desktop.wails.window.main.default_context_menu_disabled", group: "Window",
		label: "Application context menus", description: "Use the Angular context menus instead of the WebView default.",
		kind: "toggle", defaultValue: true, restartRequired: true,
	},
	{
		key: "desktop.tray.tooltip", group: "Tray",
		label: "Tooltip", description: "Base text shown before the tray's live model status.",
		kind: "text", defaultValue: "Lethean Desktop", live: true,
	},
	{
		key: "desktop.tray.label", group: "Tray",
		label: "Menu-bar label", description: "Optional text displayed beside the tray icon.",
		kind: "text", defaultValue: "", live: true,
	},
	{
		key: "desktop.wails.window.tray_popover.width", group: "Tray",
		label: "Popover width", description: "Width of the compact tray panel in pixels.",
		kind: "number", defaultValue: 400, minimum: number(320), maximum: number(800),
		step: number(10), live: true,
	},
	{
		key: "desktop.wails.window.tray_popover.height", group: "Tray",
		label: "Popover height", description: "Height of the compact tray panel in pixels.",
		kind: "number", defaultValue: 560, minimum: number(360), maximum: number(1000),
		step: number(10), live: true,
	},
	{
		key: "desktop.wails.window.tray_popover.always_on_top", group: "Tray",
		label: "Popover always on top", description: "Keep the tray panel above ordinary windows.",
		kind: "toggle", defaultValue: true, live: true,
	},
	{
		key: "desktop.wails.window.tray_popover.hide_on_focus_lost", group: "Tray",
		label: "Dismiss on focus loss", description: "Hide the tray panel after clicking elsewhere.",
		kind: "toggle", defaultValue: true, restartRequired: true,
	},
	{
		key: "desktop.wails.window.tray_popover.hide_on_escape", group: "Tray",
		label: "Dismiss with Escape", description: "Hide the tray panel when Escape is pressed.",
		kind: "toggle", defaultValue: true, restartRequired: true,
	},
	{
		key: "desktop.theme.interface", group: "Theme",
		label: "Interface theme", description: "Apply the Angular desktop theme immediately.",
		kind: "select", defaultValue: "dark", choices: []string{"dark", "light"}, live: true,
	},
	{
		key: "desktop.theme.native", group: "Theme",
		label: "Native window theme", description: "Follow the system or force native window chrome to dark or light.",
		kind: "select", defaultValue: "system", choices: []string{"system", "dark", "light"},
		restartRequired: true,
	},
	{
		key: "desktop.theme.reduce_motion", group: "Theme",
		label: "Reduce motion", description: "Disable dock magnification and interface transitions.",
		kind: "toggle", defaultValue: false, live: true,
	},
	{
		key: "permissions.network.outbound", group: "Permissions",
		label: "Outbound network", description: "Allow desktop services to make outbound network requests.",
		kind: "toggle", defaultValue: true, live: true,
	},
	{
		key: "permissions.network.inbound", group: "Permissions",
		label: "Inbound network", description: "Allow desktop services to accept inbound network requests.",
		kind: "toggle", defaultValue: true, live: true,
	},
	{
		key: "desktop.permissions.microphone", group: "Permissions",
		label: "Microphone", description: "Policy for WebView microphone requests.",
		kind: "select", defaultValue: "default", choices: []string{"default", "allow", "deny"},
		restartRequired: true,
	},
	{
		key: "desktop.permissions.camera", group: "Permissions",
		label: "Camera", description: "Policy for WebView camera requests.",
		kind: "select", defaultValue: "default", choices: []string{"default", "allow", "deny"},
		restartRequired: true,
	},
	{
		key: "desktop.permissions.geolocation", group: "Permissions",
		label: "Location", description: "Policy for WebView location requests.",
		kind: "select", defaultValue: "default", choices: []string{"default", "allow", "deny"},
		restartRequired: true,
	},
	{
		key: "desktop.permissions.clipboard_read", group: "Permissions",
		label: "Clipboard read", description: "Policy for WebView clipboard-read requests.",
		kind: "select", defaultValue: "default", choices: []string{"default", "allow", "deny"},
		restartRequired: true,
	},
	{
		key: "desktop.permissions.notifications", group: "Notifications",
		label: "Web notifications", description: "Policy for notification requests from WebView content.",
		kind: "select", defaultValue: "default", choices: []string{"default", "allow", "deny"},
		restartRequired: true,
	},
	{
		key: "desktop.single_instance.enabled", group: "Single instance",
		label: "Single-instance hand-off", description: "Send subsequent launches to the running desktop process.",
		kind: "toggle", defaultValue: true, restartRequired: true,
	},
	{
		key: "desktop.updater.check_on_startup", group: "Updater",
		label: "Startup update check", description: "Choose whether startup checks or also installs a newer release.",
		kind: "select", defaultValue: "never",
		choices: []string{"never", "check", "check-and-install"}, restartRequired: true,
	},
	{
		key: "desktop.updater.channel", group: "Updater",
		label: "Release channel", description: "Track stable releases or prerelease beta builds.",
		kind: "select", defaultValue: "stable", choices: []string{"stable", "beta"},
		restartRequired: true,
	},
}

func controlDefinitionFor(key string) (controlDefinition, bool) {
	for _, definition := range controlDefinitions {
		if definition.key == key {
			return definition, true
		}
	}
	return controlDefinition{}, false
}

func (definition controlDefinition) value(cfg *config.Service) (any, bool) {
	value := definition.defaultValue
	if !cfg.Get(definition.key, &value).OK {
		return definition.defaultValue, false
	}
	normalised, ok := definition.normalise(value)
	if !ok {
		return definition.defaultValue, false
	}
	return normalised, true
}

func (definition controlDefinition) normalise(value any) (any, bool) {
	switch definition.kind {
	case "toggle":
		result, ok := value.(bool)
		return result, ok
	case "text":
		result, ok := value.(string)
		return result, ok && len(result) <= 128
	case "select":
		result, ok := value.(string)
		if !ok {
			return nil, false
		}
		for _, choice := range definition.choices {
			if result == choice {
				return result, true
			}
		}
		return nil, false
	case "number":
		return definition.normaliseNumber(value)
	default:
		return nil, false
	}
}

func (definition controlDefinition) normaliseNumber(value any) (any, bool) {
	numeric, ok := numericValue(value)
	if !ok {
		return nil, false
	}
	if definition.minimum != nil && numeric < *definition.minimum {
		return nil, false
	}
	if definition.maximum != nil && numeric > *definition.maximum {
		return nil, false
	}
	switch definition.defaultValue.(type) {
	case int:
		integer := int(numeric)
		if float64(integer) != numeric {
			return nil, false
		}
		return integer, true
	case float64:
		return numeric, true
	default:
		return nil, false
	}
}

func numericValue(value any) (float64, bool) {
	switch typed := value.(type) {
	case int:
		return float64(typed), true
	case int8:
		return float64(typed), true
	case int16:
		return float64(typed), true
	case int32:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case uint:
		return float64(typed), true
	case uint8:
		return float64(typed), true
	case uint16:
		return float64(typed), true
	case uint32:
		return float64(typed), true
	case uint64:
		return float64(typed), true
	case float32:
		return float64(typed), true
	case float64:
		return typed, true
	default:
		return 0, false
	}
}

func (s *Service) applyLive(key string, value any, cfg *config.Service) {
	if s == nil || s.core == nil {
		return
	}
	switch key {
	case "desktop.wails.window.main.width",
		"desktop.wails.window.main.height":
		s.applyWindowSize(cfg, mainWindowControlName,
			"desktop.wails.window.main.width", 1440,
			"desktop.wails.window.main.height", 900)
	case "desktop.wails.window.tray_popover.width",
		"desktop.wails.window.tray_popover.height":
		s.applyWindowSize(cfg, trayWindowControlName,
			"desktop.wails.window.tray_popover.width", 400,
			"desktop.wails.window.tray_popover.height", 560)
	case "desktop.wails.window.main.always_on_top":
		s.runLive("window.set_always_on_top", guiwindow.TaskSetAlwaysOnTop{
			Name: mainWindowControlName, AlwaysOnTop: value.(bool),
		})
	case "desktop.wails.window.tray_popover.always_on_top":
		s.runLive("window.set_always_on_top", guiwindow.TaskSetAlwaysOnTop{
			Name: trayWindowControlName, AlwaysOnTop: value.(bool),
		})
	case "desktop.wails.window.main.zoom":
		magnification := value.(float64)
		if magnification == 0 {
			magnification = 1
		}
		s.runLive("window.set_zoom", guiwindow.TaskSetZoom{
			Name: mainWindowControlName, Magnification: magnification,
		})
	case "desktop.wails.window.main.content_protection_enabled":
		s.runLive("window.set_content_protection", guiwindow.TaskSetContentProtection{
			Name: mainWindowControlName, Protection: value.(bool),
		})
	case "desktop.tray.tooltip":
		s.runLive("systray.set_tooltip", guisystray.TaskSetTrayTooltip{
			Tooltip: value.(string),
		})
	case "desktop.tray.label":
		s.runLive("systray.set_label", guisystray.TaskSetTrayLabel{
			Label: value.(string),
		})
	}
}

func (s *Service) applyWindowSize(
	cfg *config.Service,
	name string,
	widthKey string,
	defaultWidth int,
	heightKey string,
	defaultHeight int,
) {
	width := defaultWidth
	height := defaultHeight
	if result := cfg.Get(widthKey, &width); !result.OK {
		width = defaultWidth
	}
	if result := cfg.Get(heightKey, &height); !result.OK {
		height = defaultHeight
	}
	s.runLive("window.set_size", guiwindow.TaskSetSize{
		Name: name, Width: width, Height: height,
	})
}

func (s *Service) runLive(action string, task any) {
	result := s.core.Action(action).Run(core.Background(), core.NewOptions(
		core.Option{Key: "task", Value: task},
	))
	if !result.OK {
		return
	}
}
