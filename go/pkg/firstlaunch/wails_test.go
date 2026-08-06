// SPDX-Licence-Identifier: EUPL-1.2

// Real tests for wails.go's WailsService (Detect / Build / Paths /
// SetModelsDir / ClearModelsDir / lifecycle). firstlaunch_test.go
// already drives the package-level Detect() function hard; this file
// covers the Wails-bound wrapper the pre-existing wails_example_test.go
// only reflected on (method VALUES via core.Sprintf("%T", ref), never
// invoked). flFixture is shared from firstlaunch_test.go (same test
// package, different file).

package firstlaunch_test

import (
	"os"

	core "dappco.re/go"
	lthn "dappco.re/lthn/desktop"
	"dappco.re/lthn/desktop/pkg/firstlaunch"
	"dappco.re/lthn/desktop/pkg/paths"
)

// TestWails_WailsService_Build_Good_UsesSharedDesktopVersion was
// migrated from wails_example_test.go (the fake-mechanism file's one
// genuine test, previously stranded alongside its non-invoking
// _Good/_Bad/_Ugly triplet) — this is Build's proper home.
func TestWails_WailsService_Build_Good_UsesSharedDesktopVersion(t *core.T) {
	original := lthn.Version
	t.Cleanup(func() { lthn.Version = original })
	lthn.Version = "v8.7.6-test"

	r := firstlaunch.NewWailsService().Build()

	core.AssertTrue(t, r.OK)
	info := r.Value.(firstlaunch.BuildInfo)
	core.AssertEqual(t, "8.7.6-test", info.Version)
}

func TestWails_WailsService_ServiceLifecycle_Good(t *core.T) {
	svc := firstlaunch.NewWailsService()
	core.AssertEqual(t, "FirstLaunch", svc.ServiceName())

	r := svc.ServiceStartup(core.Background(), nil)
	core.AssertTrue(t, r.OK)

	r = svc.ServiceShutdown()
	core.AssertTrue(t, r.OK)
}

func TestWails_WailsService_Detect_Good_ReturnsFreshState(t *core.T) {
	flFixture(t)
	r := firstlaunch.NewWailsService().Detect()
	core.RequireTrue(t, r.OK, r.Error())
	state := r.Value.(firstlaunch.State)
	core.AssertTrue(t, state.Fresh)
}

func TestWails_WailsService_Detect_Good_ReturnsConfiguredState(t *core.T) {
	_, conf, _ := flFixture(t)
	yaml := []byte("routes:\n  default:\n    base_url: http://localhost:11434/v1\n")
	core.AssertTrue(t, core.WriteFile(core.PathJoin(conf, lthnYAML), yaml, 0o644).OK)

	r := firstlaunch.NewWailsService().Detect()
	core.RequireTrue(t, r.OK, r.Error())
	state := r.Value.(firstlaunch.State)
	core.AssertFalse(t, state.Fresh)
	core.AssertTrue(t, state.HasRoutes)
}

func TestWails_WailsService_Paths_Good_ReturnsExpandedPaths(t *core.T) {
	home, conf, data := flFixture(t)

	r := firstlaunch.NewWailsService().Paths()
	core.RequireTrue(t, r.OK, r.Error())
	p := r.Value.(firstlaunch.LetheanPaths)

	core.AssertEqual(t, core.PathJoin(home, "Lethean"), p.Root)
	core.AssertEqual(t, conf, p.ConfDir)
	core.AssertEqual(t, data, p.DataDir)
	core.AssertEqual(t, core.PathJoin(home, "Lethean", "wallets"), p.WalletsDir)
	core.AssertEqual(t, core.PathJoin(conf, "models"), p.ModelsDir)
	core.AssertEqual(t, core.PathJoin(conf, "lthn.yaml"), p.ConfigFile)
}

// TestWails_WailsService_Paths_Ugly_PermissionDeniedSubdirsStayEmpty
// is real fault injection (chmod) rather than a fake: Root() succeeds
// because ~/Lethean already exists (MkdirAll no-ops on an existing
// dir regardless of writability), but ConfDir/DataDir/WalletsDir/
// ModelsDir/ConfigFile all need to MkdirAll a NEW subdirectory under
// a now-read-only parent, so each of Paths()'s per-field `if r.OK`
// guards takes the false arm and leaves that field empty rather than
// propagating an error (Paths() always returns core.Ok, even
// partially populated). Permissions are restored in cleanup so
// t.TempDir()'s own removal doesn't trip over them.
func TestWails_WailsService_Paths_Ugly_PermissionDeniedSubdirsStayEmpty(t *core.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := core.PathJoin(home, "Lethean")
	core.RequireTrue(t, core.MkdirAll(root, 0o755).OK)
	core.RequireTrue(t, core.Chmod(root, 0o500).OK)
	t.Cleanup(func() { _ = core.Chmod(root, 0o755) })

	r := firstlaunch.NewWailsService().Paths()
	core.RequireTrue(t, r.OK, r.Error())
	p := r.Value.(firstlaunch.LetheanPaths)

	core.AssertEqual(t, root, p.Root)
	core.AssertEqual(t, "", p.ConfDir)
	core.AssertEqual(t, "", p.DataDir)
	core.AssertEqual(t, "", p.WalletsDir)
	core.AssertEqual(t, "", p.ModelsDir)
	core.AssertEqual(t, "", p.ConfigFile)
}

func TestWails_WailsService_SetModelsDir_Good_PersistsOverride(t *core.T) {
	home, _, _ := flFixture(t)
	custom := core.PathJoin(home, "Vault", "lthn-models")

	r := firstlaunch.NewWailsService().SetModelsDir(custom)
	core.RequireTrue(t, r.OK, r.Error())
	core.AssertEqual(t, custom, r.Value.(string))

	modelsR := paths.ModelsDir()
	core.RequireTrue(t, modelsR.OK)
	core.AssertEqual(t, custom, modelsR.Value.(string))
}

func TestWails_WailsService_SetModelsDir_Bad_RelativePathRejected(t *core.T) {
	flFixture(t)
	r := firstlaunch.NewWailsService().SetModelsDir("relative/models")
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "set override failed")
}

func TestWails_WailsService_SetModelsDir_Bad_OutsideHomeRejected(t *core.T) {
	flFixture(t)
	r := firstlaunch.NewWailsService().SetModelsDir(string(os.PathSeparator) + "etc" + string(os.PathSeparator) + "lthn-evil")
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "set override failed")
}

func TestWails_WailsService_ClearModelsDir_Good_RemovesOverride(t *core.T) {
	home, conf, _ := flFixture(t)
	custom := core.PathJoin(home, "Vault", "lthn-models")

	setR := firstlaunch.NewWailsService().SetModelsDir(custom)
	core.RequireTrue(t, setR.OK, setR.Error())

	r := firstlaunch.NewWailsService().ClearModelsDir()
	core.RequireTrue(t, r.OK, r.Error())

	modelsR := paths.ModelsDir()
	core.RequireTrue(t, modelsR.OK)
	core.AssertEqual(t, core.PathJoin(conf, "models"), modelsR.Value.(string))
}

// TestWails_WailsService_ClearModelsDir_Ugly_WriteFailsOnReadOnlyConfDir
// drives ClearModelsDir's error-wrap branch: an override is set first
// (succeeds, conf dir still writable), then the conf dir is made
// read-only before Clear runs. writePathsOverride's tmp-file write
// fails, so ClearModelsDirOverride fails and WailsService.ClearModelsDir
// wraps it. Real fault injection via chmod; permission restored in
// cleanup so t.TempDir()'s own removal doesn't trip over it.
func TestWails_WailsService_ClearModelsDir_Ugly_WriteFailsOnReadOnlyConfDir(t *core.T) {
	home, conf, _ := flFixture(t)
	custom := core.PathJoin(home, "Vault", "lthn-models")
	setR := firstlaunch.NewWailsService().SetModelsDir(custom)
	core.RequireTrue(t, setR.OK, setR.Error())

	core.RequireTrue(t, core.Chmod(conf, 0o500).OK)
	t.Cleanup(func() { _ = core.Chmod(conf, 0o755) })

	r := firstlaunch.NewWailsService().ClearModelsDir()
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "clear override failed")
}

func TestWails_WailsService_ClearModelsDir_Ugly_IdempotentWhenNoOverride(t *core.T) {
	flFixture(t)
	r1 := firstlaunch.NewWailsService().ClearModelsDir()
	core.RequireTrue(t, r1.OK, r1.Error())
	r2 := firstlaunch.NewWailsService().ClearModelsDir()
	core.RequireTrue(t, r2.OK, r2.Error())
}
