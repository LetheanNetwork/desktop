// SPDX-Licence-Identifier: EUPL-1.2

package appconfig_test

import (
	"io/fs"

	core "dappco.re/go"
	"dappco.re/go/config"
	coreio "dappco.re/go/io"
	guisystray "dappco.re/go/render/display/webkit/pkg/systray"
	guiwindow "dappco.re/go/render/display/webkit/pkg/window"
	"dappco.re/lthn/desktop/pkg/appconfig"
)

func controlByKey(t *core.T, snapshot map[string]any, key string) map[string]any {
	t.Helper()
	controls, ok := snapshot["controls"].([]map[string]any)
	core.RequireTrue(t, ok)
	for _, control := range controls {
		if control["key"] == key {
			return control
		}
	}
	t.Fatalf("control %q not found", key)
	return nil
}

func TestService_Settings_Good_GroupedFallbackCatalogue(t *core.T) {
	c, _ := newConfigFixture(t)
	svc := appconfig.NewService(appconfig.Options{Core: c})

	r := svc.Settings()

	core.RequireTrue(t, r.OK)
	snapshot, ok := r.Value.(map[string]any)
	core.RequireTrue(t, ok)
	core.AssertEqual(t, "0", snapshot["revision"])
	_, exposesConfigPath := snapshot["config_path"]
	core.AssertFalse(t, exposesConfigPath)
	width := controlByKey(t, snapshot, "desktop.wails.window.main.width")
	core.AssertEqual(t, "Window", width["group"])
	core.AssertEqual(t, 1440, width["value"])
	core.AssertEqual(t, 1440, width["default"])
	core.AssertEqual(t, true, width["live"])
	core.AssertEqual(t, false, width["restart_required"])
	notifications := controlByKey(t, snapshot, "desktop.permissions.notifications")
	core.AssertEqual(t, "Notifications", notifications["group"])
	taskbar := controlByKey(t, snapshot, "desktop.shell.taskbar_edge")
	core.AssertEqual(t, "Desktop", taskbar["group"])
	core.AssertEqual(t, "bottom", taskbar["default"])
	wallpaper := controlByKey(t, snapshot, "desktop.theme.wallpaper")
	core.AssertEqual(t, "aurora", wallpaper["default"])
	language := controlByKey(t, snapshot, "desktop.locale.language")
	core.AssertEqual(t, "en", language["default"])
}

func TestService_Settings_Bad_NilCore(t *core.T) {
	svc := appconfig.NewService(appconfig.Options{})

	r := svc.Settings()

	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "core is required")
}

func TestService_Settings_Ugly_MissingConfigService(t *core.T) {
	svc := appconfig.NewService(appconfig.Options{Core: core.New()})

	r := svc.Settings()

	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "config service is required")
}

func TestService_Set_Good_PersistsThroughConfigService(t *core.T) {
	c, cfg := newConfigFixture(t)
	svc := appconfig.NewService(appconfig.Options{Core: c})
	var liveTask guiwindow.TaskSetSize
	c.Action("window.set_size", func(_ core.Context, options core.Options) core.Result {
		task := options.Get("task")
		core.RequireTrue(t, task.OK)
		liveTask = task.Value.(guiwindow.TaskSetSize)
		return core.Ok(nil)
	})

	r := svc.Set("desktop.wails.window.main.width", float64(1320))

	core.RequireTrue(t, r.OK)
	var width int
	core.RequireTrue(t, cfg.Get("desktop.wails.window.main.width", &width).OK)
	core.AssertEqual(t, 1320, width)
	body := core.ReadFile(cfg.Config().Path())
	core.RequireTrue(t, body.OK)
	core.AssertContains(t, string(body.Value.([]byte)), "width: 1320")
	core.AssertEqual(t, "app", liveTask.Name)
	core.AssertEqual(t, 1320, liveTask.Width)
	core.AssertEqual(t, 900, liveTask.Height)
}

func TestService_Set_Bad_RejectsUnknownControl(t *core.T) {
	c, cfg := newConfigFixture(t)
	svc := appconfig.NewService(appconfig.Options{Core: c})

	r := svc.Set("desktop.wails.application.single_instance.encryption_key", "unsafe")

	core.AssertFalse(t, r.OK)
	var value any
	core.AssertFalse(t, cfg.Get("desktop.wails.application.single_instance.encryption_key", &value).OK)
}

func TestService_Set_Ugly_RejectsInvalidValue(t *core.T) {
	c, cfg := newConfigFixture(t)
	svc := appconfig.NewService(appconfig.Options{Core: c})

	r := svc.Set("desktop.wails.window.main.width", "wide")

	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "invalid value")
	var width int
	core.AssertFalse(t, cfg.Get("desktop.wails.window.main.width", &width).OK)
}

func TestService_SetMany_GoodValidatesThenCommitsOneSnapshot(t *core.T) {
	medium := coreio.NewMemoryMedium()
	c, cfg := newMediumConfigFixture(t, medium)
	svc := appconfig.NewService(appconfig.Options{Core: c})
	var liveSizes []guiwindow.TaskSetSize
	c.Action("window.set_size", func(_ core.Context, options core.Options) core.Result {
		task := options.Get("task")
		core.RequireTrue(t, task.OK)
		liveSizes = append(liveSizes, task.Value.(guiwindow.TaskSetSize))
		return core.Ok(nil)
	})

	result := svc.SetMany([]appconfig.Change{
		{Key: "desktop.wails.window.main.width", Value: float64(1280)},
		{Key: "desktop.wails.window.main.height", Value: float64(760)},
		{Key: "desktop.theme.wallpaper", Value: "graphite"},
	})

	core.RequireTrue(t, result.OK, result.Error())
	var width int
	var height int
	var wallpaper string
	core.RequireTrue(t, cfg.Get("desktop.wails.window.main.width", &width).OK)
	core.RequireTrue(t, cfg.Get("desktop.wails.window.main.height", &height).OK)
	core.RequireTrue(t, cfg.Get("desktop.theme.wallpaper", &wallpaper).OK)
	core.AssertEqual(t, 1280, width)
	core.AssertEqual(t, 760, height)
	core.AssertEqual(t, "graphite", wallpaper)
	content, err := medium.Read("lthn.yaml")
	core.RequireNoError(t, err)
	core.AssertContains(t, content, "width: 1280")
	core.AssertContains(t, content, "height: 760")
	core.AssertEqual(t, 2, len(liveSizes))
}

func TestService_SetMany_BadRejectsWholeBatchBeforeMutation(t *core.T) {
	medium := coreio.NewMemoryMedium()
	c, cfg := newMediumConfigFixture(t, medium)
	svc := appconfig.NewService(appconfig.Options{Core: c})

	result := svc.SetMany([]appconfig.Change{
		{Key: "desktop.wails.window.main.width", Value: float64(1280)},
		{Key: "desktop.wails.window.main.height", Value: "tall"},
	})

	core.AssertFalse(t, result.OK)
	var width int
	core.AssertFalse(t, cfg.Get("desktop.wails.window.main.width", &width).OK)
	core.AssertFalse(t, medium.IsFile("lthn.yaml"))
}

func TestService_SetMany_BadRejectsDuplicateKeys(t *core.T) {
	c, _ := newMediumConfigFixture(t, coreio.NewMemoryMedium())
	svc := appconfig.NewService(appconfig.Options{Core: c})

	result := svc.SetMany([]appconfig.Change{
		{Key: "desktop.theme.interface", Value: "dark"},
		{Key: "desktop.theme.interface", Value: "light"},
	})

	core.AssertFalse(t, result.OK)
	core.AssertContains(t, result.Error(), "duplicate")
}

func TestService_SetMany_UglyFailedCommitRestoresStateAndSkipsLiveApply(t *core.T) {
	base := coreio.NewMemoryMedium()
	fault := &configFaultMedium{Medium: base}
	c, cfg := newMediumConfigFixture(t, fault)
	svc := appconfig.NewService(appconfig.Options{Core: c})
	core.RequireTrue(t, svc.Set(
		"desktop.wails.window.main.width",
		float64(1200),
	).OK)
	before, err := base.Read("lthn.yaml")
	core.RequireNoError(t, err)
	liveCalls := 0
	c.Action("window.set_size", func(_ core.Context, _ core.Options) core.Result {
		liveCalls++
		return core.Ok(nil)
	})
	fault.failNextCommit = true

	result := svc.SetMany([]appconfig.Change{{
		Key:   "desktop.wails.window.main.width",
		Value: float64(1400),
	}})

	core.AssertFalse(t, result.OK)
	var width int
	core.RequireTrue(t, cfg.Get("desktop.wails.window.main.width", &width).OK)
	core.AssertEqual(t, 1200, width)
	after, err := base.Read("lthn.yaml")
	core.RequireNoError(t, err)
	core.AssertEqual(t, before, after)
	core.AssertEqual(t, 0, liveCalls)
}

func TestService_NewService_Good_BindsCore(t *core.T) {
	c, _ := newConfigFixture(t)
	svc := appconfig.NewService(appconfig.Options{Core: c})
	core.AssertNotNil(t, svc)
	core.AssertTrue(t, svc.Settings().OK)
}

func TestService_NewService_Bad_NilOptionsStayFailClosed(t *core.T) {
	svc := appconfig.NewService(appconfig.Options{})
	core.AssertNotNil(t, svc)
	core.AssertFalse(t, svc.Settings().OK)
}

func TestService_NewService_Ugly_UnstartedConfigStaysFailClosed(t *core.T) {
	c := core.New(core.WithName("config", config.NewConfigService))
	svc := appconfig.NewService(appconfig.Options{Core: c})
	core.AssertFalse(t, svc.Settings().OK)
}

func TestService_Register_Good_RegistersCanonicalService(t *core.T) {
	path := core.PathJoin(t.TempDir(), "lthn.yaml")
	c := core.New(
		core.WithName("config", config.NewConfigServiceWith(config.ServiceOptions{Path: path})),
		core.WithName("appconfig", appconfig.Register),
	)
	core.RequireTrue(t, c.ServiceStartup(core.Background(), nil).OK)
	t.Cleanup(func() {
		_ = c.ServiceShutdown(core.Background())
	})

	svc, ok := core.ServiceFor[*appconfig.Service](c, "appconfig")
	core.AssertTrue(t, ok)
	core.AssertNotNil(t, svc)
}

func TestService_Register_Bad_NilCore(t *core.T) {
	r := appconfig.Register(nil)
	core.AssertFalse(t, r.OK)
}

func TestService_Register_Ugly_CoreWithoutConfigStillRegisters(t *core.T) {
	c := core.New(core.WithName("appconfig", appconfig.Register))
	svc, ok := core.ServiceFor[*appconfig.Service](c, "appconfig")
	core.AssertTrue(t, ok)
	core.AssertNotNil(t, svc)
	core.AssertFalse(t, svc.Settings().OK)
}

// TestService_Set_Good_AcceptsEveryNumericGoType drives normaliseNumber's
// numericValue helper through every integer/float Go type a caller might
// hand it (JSON decode gives float64, but direct Go callers — tests,
// internal callers — may supply any numeric kind).
func TestService_Set_Good_AcceptsEveryNumericGoType(t *core.T) {
	c, cfg := newConfigFixture(t)
	svc := appconfig.NewService(appconfig.Options{Core: c})

	values := []any{
		int(80), int8(80), int16(80), int32(80), int64(80),
		uint(80), uint8(80), uint16(80), uint32(80), uint64(80),
		float32(80), float64(80),
	}
	for _, value := range values {
		r := svc.Set("desktop.theme.custom_hue", value)
		core.RequireTrue(t, r.OK, r.Error())
		var hue int
		core.RequireTrue(t, cfg.Get("desktop.theme.custom_hue", &hue).OK)
		core.AssertEqual(t, 80, hue)
	}
}

func TestService_Set_Bad_NonNumericValueRejected(t *core.T) {
	c, _ := newConfigFixture(t)
	svc := appconfig.NewService(appconfig.Options{Core: c})

	r := svc.Set("desktop.theme.custom_hue", "not-a-number")
	core.AssertFalse(t, r.OK)
}

func TestService_Set_Bad_NonIntegralFloatRejectedForIntegerControl(t *core.T) {
	c, _ := newConfigFixture(t)
	svc := appconfig.NewService(appconfig.Options{Core: c})

	r := svc.Set("desktop.theme.custom_hue", 80.5)
	core.AssertFalse(t, r.OK)
}

// TestService_Set_Good_FloatKindControlAcceptsFractionalValue covers the
// normaliseNumber branch keyed on a float64-typed defaultValue (zoom is the
// one "number" control that isn't backed by an int).
func TestService_Set_Good_FloatKindControlAcceptsFractionalValue(t *core.T) {
	c, cfg := newConfigFixture(t)
	svc := appconfig.NewService(appconfig.Options{Core: c})

	r := svc.Set("desktop.wails.window.main.zoom", float64(1.5))
	core.RequireTrue(t, r.OK, r.Error())
	var zoom float64
	core.RequireTrue(t, cfg.Get("desktop.wails.window.main.zoom", &zoom).OK)
	core.AssertEqual(t, 1.5, zoom)
}

func TestService_Set_Bad_SelectControlRejectsNonStringValue(t *core.T) {
	c, _ := newConfigFixture(t)
	svc := appconfig.NewService(appconfig.Options{Core: c})

	r := svc.Set("desktop.theme.interface", true)
	core.AssertFalse(t, r.OK)
}

func TestService_Set_Bad_TextControlRejectsOversizedValue(t *core.T) {
	c, _ := newConfigFixture(t)
	svc := appconfig.NewService(appconfig.Options{Core: c})

	r := svc.Set("desktop.tray.label", core.Repeat("x", 129))
	core.AssertFalse(t, r.OK)
}

// TestService_SetMany_Good_AppliesEveryLiveAction exercises applyLive's
// remaining switch arms (only main.width/height was previously covered):
// both windows' size, both always-on-top toggles, zoom, content protection,
// and the tray tooltip/label live actions.
func TestService_SetMany_Good_AppliesEveryLiveAction(t *core.T) {
	c, _ := newConfigFixture(t)
	svc := appconfig.NewService(appconfig.Options{Core: c})

	var sizes []guiwindow.TaskSetSize
	var alwaysOnTop []guiwindow.TaskSetAlwaysOnTop
	var zooms []guiwindow.TaskSetZoom
	var protections []guiwindow.TaskSetContentProtection
	var tooltips []guisystray.TaskSetTrayTooltip
	var labels []guisystray.TaskSetTrayLabel

	c.Action("window.set_size", func(_ core.Context, options core.Options) core.Result {
		task := options.Get("task")
		core.RequireTrue(t, task.OK)
		sizes = append(sizes, task.Value.(guiwindow.TaskSetSize))
		return core.Ok(nil)
	})
	c.Action("window.set_always_on_top", func(_ core.Context, options core.Options) core.Result {
		task := options.Get("task")
		core.RequireTrue(t, task.OK)
		alwaysOnTop = append(alwaysOnTop, task.Value.(guiwindow.TaskSetAlwaysOnTop))
		return core.Ok(nil)
	})
	c.Action("window.set_zoom", func(_ core.Context, options core.Options) core.Result {
		task := options.Get("task")
		core.RequireTrue(t, task.OK)
		zooms = append(zooms, task.Value.(guiwindow.TaskSetZoom))
		return core.Ok(nil)
	})
	c.Action("window.set_content_protection", func(_ core.Context, options core.Options) core.Result {
		task := options.Get("task")
		core.RequireTrue(t, task.OK)
		protections = append(protections, task.Value.(guiwindow.TaskSetContentProtection))
		return core.Ok(nil)
	})
	c.Action("systray.set_tooltip", func(_ core.Context, options core.Options) core.Result {
		task := options.Get("task")
		core.RequireTrue(t, task.OK)
		tooltips = append(tooltips, task.Value.(guisystray.TaskSetTrayTooltip))
		return core.Ok(nil)
	})
	c.Action("systray.set_label", func(_ core.Context, options core.Options) core.Result {
		task := options.Get("task")
		core.RequireTrue(t, task.OK)
		labels = append(labels, task.Value.(guisystray.TaskSetTrayLabel))
		return core.Ok(nil)
	})

	result := svc.SetMany([]appconfig.Change{
		{Key: "desktop.wails.window.tray_popover.width", Value: float64(500)},
		{Key: "desktop.wails.window.tray_popover.height", Value: float64(700)},
		{Key: "desktop.wails.window.main.always_on_top", Value: true},
		{Key: "desktop.wails.window.tray_popover.always_on_top", Value: false},
		{Key: "desktop.wails.window.main.zoom", Value: float64(1.5)},
		{Key: "desktop.wails.window.main.content_protection_enabled", Value: true},
		{Key: "desktop.tray.tooltip", Value: "Lethean"},
		{Key: "desktop.tray.label", Value: "L"},
	})
	core.RequireTrue(t, result.OK, result.Error())

	core.RequireTrue(t, len(sizes) == 2)
	for _, size := range sizes {
		core.AssertEqual(t, "tray-panel", size.Name)
		core.AssertEqual(t, 500, size.Width)
		core.AssertEqual(t, 700, size.Height)
	}

	core.RequireTrue(t, len(alwaysOnTop) == 2)
	core.AssertEqual(t, "app", alwaysOnTop[0].Name)
	core.AssertTrue(t, alwaysOnTop[0].AlwaysOnTop)
	core.AssertEqual(t, "tray-panel", alwaysOnTop[1].Name)
	core.AssertFalse(t, alwaysOnTop[1].AlwaysOnTop)

	core.RequireTrue(t, len(zooms) == 1)
	core.AssertEqual(t, "app", zooms[0].Name)
	core.AssertEqual(t, 1.5, zooms[0].Magnification)

	core.RequireTrue(t, len(protections) == 1)
	core.AssertEqual(t, "app", protections[0].Name)
	core.AssertTrue(t, protections[0].Protection)

	core.RequireTrue(t, len(tooltips) == 1)
	core.AssertEqual(t, "Lethean", tooltips[0].Tooltip)

	core.RequireTrue(t, len(labels) == 1)
	core.AssertEqual(t, "L", labels[0].Label)
}

// TestService_Set_Good_ZeroZoomAppliesUnityMagnification covers applyLive's
// "zero zoom means unity, not literally zero" branch.
func TestService_Set_Good_ZeroZoomAppliesUnityMagnification(t *core.T) {
	c, _ := newConfigFixture(t)
	svc := appconfig.NewService(appconfig.Options{Core: c})
	var zoom guiwindow.TaskSetZoom
	c.Action("window.set_zoom", func(_ core.Context, options core.Options) core.Result {
		task := options.Get("task")
		core.RequireTrue(t, task.OK)
		zoom = task.Value.(guiwindow.TaskSetZoom)
		return core.Ok(nil)
	})

	r := svc.Set("desktop.wails.window.main.zoom", float64(0))

	core.RequireTrue(t, r.OK, r.Error())
	core.AssertEqual(t, float64(1), zoom.Magnification)
}

// TestService_Set_Ugly_LiveActionRunsEvenWithoutAHandlerRegistered covers
// runLive's "no listener" no-op path (Action().Run against an unregistered
// action name returns a failing Result that runLive silently swallows).
func TestService_Set_Ugly_LiveActionRunsEvenWithoutAHandlerRegistered(t *core.T) {
	c, cfg := newConfigFixture(t)
	svc := appconfig.NewService(appconfig.Options{Core: c})

	r := svc.Set("desktop.tray.tooltip", "no listener installed")

	core.RequireTrue(t, r.OK, r.Error())
	var tooltip string
	core.RequireTrue(t, cfg.Get("desktop.tray.tooltip", &tooltip).OK)
	core.AssertEqual(t, "no listener installed", tooltip)
}

// TestService_Settings_Ugly_InvalidStoredValueFallsBackToDefault seeds the
// config document directly (bypassing SetMany's validation) with a value
// whose type doesn't match its control's kind, so controlDefinition.value's
// normalise-failure branch runs: cfg.Get succeeds (the YAML decodes into
// the `any` target) but normalise rejects the type, and Settings must fall
// back to the definition's default with configured=false.
func TestService_Settings_Ugly_InvalidStoredValueFallsBackToDefault(t *core.T) {
	medium := coreio.NewMemoryMedium()
	core.RequireNoError(t, medium.WriteMode(
		"lthn.yaml",
		"desktop:\n  theme:\n    interface: 12345\n",
		fs.FileMode(0o644),
	))
	c, _ := newMediumConfigFixture(t, medium)
	svc := appconfig.NewService(appconfig.Options{Core: c})

	r := svc.Settings()

	core.RequireTrue(t, r.OK, r.Error())
	snapshot, ok := r.Value.(map[string]any)
	core.RequireTrue(t, ok)
	control := controlByKey(t, snapshot, "desktop.theme.interface")
	core.AssertEqual(t, "dark", control["value"])
	core.AssertEqual(t, false, control["configured"])
}

func newMediumConfigFixture(
	t *core.T,
	medium coreio.Medium,
) (*core.Core, *config.Service) {
	t.Helper()
	c := core.New(
		core.WithName("config-io", func(c *core.Core) core.Result {
			return core.Ok(&coreio.Service{
				ServiceRuntime: core.NewServiceRuntime(c, coreio.IOConfig{}),
				Medium:         medium,
			})
		}),
		core.WithName("config", appconfig.NewConfigService(
			appconfig.ConfigServiceOptions{
				Path:      "lthn.yaml",
				EnvPrefix: "LTHN_TEST",
			},
		)),
	)
	core.RequireTrue(t, c.ServiceStartup(core.Background(), nil).OK)
	t.Cleanup(func() {
		_ = c.ServiceShutdown(core.Background())
	})
	cfg, ok := core.ServiceFor[*config.Service](c, "config")
	core.RequireTrue(t, ok)
	core.AssertNotNil(t, cfg)
	core.AssertSame(t, medium, appconfig.BaseConfigMedium(cfg))
	core.AssertEqual(t, "lthn.yaml", cfg.Config().Path())
	return c, cfg
}

type configFaultMedium struct {
	coreio.Medium
	failNextCommit bool
}

func (medium *configFaultMedium) Rename(oldPath, newPath string) error {
	if medium.failNextCommit &&
		newPath == "lthn.yaml" &&
		core.HasSuffix(oldPath, ".new") {
		medium.failNextCommit = false
		return fs.ErrPermission
	}
	return medium.Medium.Rename(oldPath, newPath)
}
