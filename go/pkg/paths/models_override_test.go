// SPDX-Licence-Identifier: EUPL-1.2

// Tests for the models-dir override surface — Cerberus H1 / L3
// hardening (Mantis 2026-05-16).
//
// Scope: producer-side only — Set, Clear, paths.json persistence +
// permissions, confused-deputy guards. The ModelsDir() read-side
// integration (override honoured by the hot path) is covered by
// paths_test.go in the parallel paths-sweep lane.
//
// Pattern matches core/go canon: external `_test` package, t-scoped
// HOME via homeFixture (defined in paths_test.go) so every test
// runs against an isolated ~/.

package paths_test

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/paths"
)

func TestPathsOverride_SetModelsDirOverride_Good(t *core.T) {
	home := homeFixture(t)
	override := core.PathJoin(home, "external-models")

	r := paths.SetModelsDirOverride(override)
	core.AssertTrue(t, r.OK, "SetModelsDirOverride must succeed for a valid abs path")
	core.AssertEqual(t, override, r.Value.(string))

	// Override directory should exist on disk (MkdirAll was called).
	stat := core.Stat(override)
	core.AssertTrue(t, stat.OK)
	info := stat.Value.(core.FsFileInfo)
	core.AssertTrue(t, info.IsDir(), "override path must be a directory")

	// The paths.json file should exist and carry the value.
	confDir := paths.ConfDir().Value.(string)
	overrideFile := core.PathJoin(confDir, "paths.json")
	body := core.ReadFile(overrideFile)
	core.AssertTrue(t, body.OK, "paths.json must be written")
	raw, _ := body.Value.([]byte)
	core.AssertContains(t, string(raw), override, "paths.json must carry the override path")
}

func TestPathsOverride_SetModelsDirOverride_Bad_EmptyPath(t *core.T) {
	homeFixture(t)
	r := paths.SetModelsDirOverride("")
	core.AssertFalse(t, r.OK)
}

func TestPathsOverride_SetModelsDirOverride_Bad_OutsideHome(t *core.T) {
	homeFixture(t)
	r := paths.SetModelsDirOverride("/etc/lthn-models")
	core.AssertFalse(t, r.OK, "paths outside $HOME must be refused")
}

func TestPathsOverride_SetModelsDirOverride_Bad_HiddenDir(t *core.T) {
	home := homeFixture(t)
	r := paths.SetModelsDirOverride(core.PathJoin(home, ".ssh", "models"))
	core.AssertFalse(t, r.OK, "paths inside hidden ~/. directories must be refused")
}

func TestPathsOverride_SetModelsDirOverride_Bad_LibraryDir(t *core.T) {
	home := homeFixture(t)
	r := paths.SetModelsDirOverride(core.PathJoin(home, "Library", "lthn"))
	core.AssertFalse(t, r.OK, "paths under ~/Library must be refused (macOS persistence vector)")
}

func TestPathsOverride_SetModelsDirOverride_Bad_HomeRoot(t *core.T) {
	home := homeFixture(t)
	r := paths.SetModelsDirOverride(home)
	core.AssertFalse(t, r.OK, "the home root itself must be refused")
}

func TestPathsOverride_SetModelsDirOverride_Bad_RelativePath(t *core.T) {
	homeFixture(t)
	r := paths.SetModelsDirOverride("relative/path")
	core.AssertFalse(t, r.OK, "relative paths must be refused — overrides are absolute")
}

func TestPathsOverride_PathsJsonIsMode0600(t *core.T) {
	home := homeFixture(t)
	core.AssertTrue(t, paths.SetModelsDirOverride(core.PathJoin(home, "vault")).OK)

	stat := core.Stat(core.PathJoin(paths.ConfDir().Value.(string), "paths.json"))
	core.AssertTrue(t, stat.OK)
	info := stat.Value.(core.FsFileInfo)
	mode := info.Mode().Perm()
	core.AssertEqual(t, 0o600, int(mode), "paths.json must be mode 0o600 (Cerberus L3)")
}

func TestPathsOverride_ClearModelsDirOverride_Good(t *core.T) {
	home := homeFixture(t)
	override := core.PathJoin(home, "external")
	core.AssertTrue(t, paths.SetModelsDirOverride(override).OK)

	r := paths.ClearModelsDirOverride()
	core.AssertTrue(t, r.OK)

	// After clear, paths.json should no longer carry the override.
	overrideFile := core.PathJoin(paths.ConfDir().Value.(string), "paths.json")
	body := core.ReadFile(overrideFile)
	core.AssertTrue(t, body.OK, "paths.json must still be readable post-clear")
	raw, _ := body.Value.([]byte)
	core.AssertNotContains(t, string(raw), override, "paths.json must no longer carry the cleared path")
}

func TestPathsOverride_ClearModelsDirOverride_Good_Idempotent(t *core.T) {
	homeFixture(t)
	r := paths.ClearModelsDirOverride()
	core.AssertTrue(t, r.OK, "clearing when no override is set must succeed silently")
}
