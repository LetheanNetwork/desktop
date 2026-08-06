// SPDX-Licence-Identifier: EUPL-1.2

// Tests for the fresh-install detector. Drives every combination of
// the three signals (config file, state DB, routes-in-yaml) so the
// Fresh flag's truth table is fully covered.

package firstlaunch_test

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/firstlaunch"
)

const lthnYAML = "lthn.yaml"

// flFixture rebinds $HOME and pre-creates the ~/Lethean/conf and data
// dirs so each test can selectively populate them.
func flFixture(t *core.T) (homeDir, confDir, dataDir string) {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	confDir = core.PathJoin(tmp, "Lethean", "conf")
	dataDir = core.PathJoin(tmp, "Lethean", "data")
	core.AssertTrue(t, core.MkdirAll(confDir, 0o755).OK)
	core.AssertTrue(t, core.MkdirAll(dataDir, 0o755).OK)
	return tmp, confDir, dataDir
}

func TestFirstLaunch_Detect_Good_Fresh(t *core.T) {
	flFixture(t)
	r := firstlaunch.Detect(nil)
	core.AssertTrue(t, r.OK)
	state := r.Value.(firstlaunch.State)
	core.AssertTrue(t, state.Fresh, "no config + no db = fresh")
	core.AssertFalse(t, state.HasConfigFile)
	core.AssertFalse(t, state.HasStateDB)
	core.AssertFalse(t, state.HasRoutes)
}

func TestFirstLaunch_Detect_Good_ConfigFilePresent(t *core.T) {
	_, conf, _ := flFixture(t)
	// Config exists but has no `routes:` key.
	core.AssertTrue(t, core.WriteFile(core.PathJoin(conf, lthnYAML), []byte("name: lthn\n"), 0o644).OK)

	r := firstlaunch.Detect(nil)
	state := r.Value.(firstlaunch.State)
	core.AssertTrue(t, state.HasConfigFile)
	core.AssertFalse(t, state.HasRoutes, "yaml without routes: should set HasRoutes=false")
	core.AssertFalse(t, state.Fresh, "ConfigFile alone disqualifies Fresh")
}

func TestFirstLaunch_Detect_Good_RoutesConfigured(t *core.T) {
	_, conf, _ := flFixture(t)
	yaml := []byte("routes:\n  default:\n    base_url: http://localhost:11434/v1\n")
	core.AssertTrue(t, core.WriteFile(core.PathJoin(conf, lthnYAML), yaml, 0o644).OK)

	r := firstlaunch.Detect(nil)
	state := r.Value.(firstlaunch.State)
	core.AssertTrue(t, state.HasConfigFile)
	core.AssertTrue(t, state.HasRoutes, "yaml with routes: should set HasRoutes=true")
	core.AssertFalse(t, state.Fresh)
}

func TestFirstLaunch_Detect_Good_StateDBPresent(t *core.T) {
	_, _, data := flFixture(t)
	core.AssertTrue(t, core.WriteFile(core.PathJoin(data, "lthn.db"), []byte("sqlite"), 0o644).OK)

	r := firstlaunch.Detect(nil)
	state := r.Value.(firstlaunch.State)
	core.AssertTrue(t, state.HasStateDB)
	core.AssertFalse(t, state.Fresh, "StateDB alone disqualifies Fresh")
}

// Bad: corrupt yaml — Detect should not panic, and HasRoutes should
// stay false rather than propagating an error.
func TestFirstLaunch_Detect_Ugly_CorruptYAML(t *core.T) {
	_, conf, _ := flFixture(t)
	core.AssertTrue(t, core.WriteFile(core.PathJoin(conf, lthnYAML), []byte(":::not yaml:::"), 0o644).OK)

	r := firstlaunch.Detect(nil)
	state := r.Value.(firstlaunch.State)
	core.AssertTrue(t, state.HasConfigFile)
	core.AssertFalse(t, state.HasRoutes, "corrupt yaml should be treated as 'no routes'")
}

// TestFirstLaunch_Detect_Ugly_UnreadableYAML is real fault injection
// (chmod, not a fake): the file EXISTS (pathExists/Stat sees it, so
// HasConfigFile stays true) but is unreadable, driving yamlHasRoutes's
// core.ReadFile !r.OK branch — distinct from the corrupt-parse branch
// above. The content deliberately HAS routes so a false positive
// (falling through to the parse path) would be caught by HasRoutes
// wrongly flipping true. Permission restored in cleanup so
// t.TempDir()'s own removal doesn't trip over it.
func TestFirstLaunch_Detect_Ugly_UnreadableYAML(t *core.T) {
	_, conf, _ := flFixture(t)
	path := core.PathJoin(conf, lthnYAML)
	yaml := []byte("routes:\n  default:\n    base_url: http://localhost:11434/v1\n")
	core.AssertTrue(t, core.WriteFile(path, yaml, 0o644).OK)
	core.RequireTrue(t, core.Chmod(path, 0o000).OK)
	t.Cleanup(func() { _ = core.Chmod(path, 0o644) })

	r := firstlaunch.Detect(nil)
	state := r.Value.(firstlaunch.State)
	core.AssertTrue(t, state.HasConfigFile, "Stat still sees the file — only read access is denied")
	core.AssertFalse(t, state.HasRoutes, "unreadable yaml should be treated as 'no routes'")
}
