// SPDX-Licence-Identifier: EUPL-1.2

package appconfig_test

import (
	"io/fs"

	core "dappco.re/go"
	"dappco.re/go/config"
	coreio "dappco.re/go/io"
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
