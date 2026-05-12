// SPDX-Licence-Identifier: EUPL-1.2

// Tests for the fresh-install detector. Drives every combination of
// the three signals (config file, state DB, routes-in-yaml) so the
// Fresh flag's truth table is fully covered.

package firstlaunch_test

import (
	"os"
	"path/filepath"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/firstlaunch"
)

// flFixture rebinds $HOME and pre-creates the ~/Lethean/conf and data
// dirs so each test can selectively populate them.
func flFixture(t *core.T) (homeDir, confDir, dataDir string) {
	t.Helper()
	tmp := t.TempDir()
	prev, hadPrev := os.LookupEnv("HOME")
	core.AssertNoError(t, os.Setenv("HOME", tmp))
	t.Cleanup(func() {
		if hadPrev {
			_ = os.Setenv("HOME", prev)
		} else {
			_ = os.Unsetenv("HOME")
		}
	})
	confDir = filepath.Join(tmp, "Lethean", "conf")
	dataDir = filepath.Join(tmp, "Lethean", "data")
	core.AssertNoError(t, os.MkdirAll(confDir, 0o755))
	core.AssertNoError(t, os.MkdirAll(dataDir, 0o755))
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
	core.AssertNoError(t, os.WriteFile(filepath.Join(conf, "lthn.yaml"), []byte("name: lthn\n"), 0o644))

	r := firstlaunch.Detect(nil)
	state := r.Value.(firstlaunch.State)
	core.AssertTrue(t, state.HasConfigFile)
	core.AssertFalse(t, state.HasRoutes, "yaml without routes: should set HasRoutes=false")
	core.AssertFalse(t, state.Fresh, "ConfigFile alone disqualifies Fresh")
}

func TestFirstLaunch_Detect_Good_RoutesConfigured(t *core.T) {
	_, conf, _ := flFixture(t)
	yaml := []byte("routes:\n  default:\n    base_url: http://localhost:11434/v1\n")
	core.AssertNoError(t, os.WriteFile(filepath.Join(conf, "lthn.yaml"), yaml, 0o644))

	r := firstlaunch.Detect(nil)
	state := r.Value.(firstlaunch.State)
	core.AssertTrue(t, state.HasConfigFile)
	core.AssertTrue(t, state.HasRoutes, "yaml with routes: should set HasRoutes=true")
	core.AssertFalse(t, state.Fresh)
}

func TestFirstLaunch_Detect_Good_StateDBPresent(t *core.T) {
	_, _, data := flFixture(t)
	core.AssertNoError(t, os.WriteFile(filepath.Join(data, "lthn.db"), []byte("sqlite"), 0o644))

	r := firstlaunch.Detect(nil)
	state := r.Value.(firstlaunch.State)
	core.AssertTrue(t, state.HasStateDB)
	core.AssertFalse(t, state.Fresh, "StateDB alone disqualifies Fresh")
}

// Bad: corrupt yaml — Detect should not panic, and HasRoutes should
// stay false rather than propagating an error.
func TestFirstLaunch_Detect_Ugly_CorruptYAML(t *core.T) {
	_, conf, _ := flFixture(t)
	core.AssertNoError(t, os.WriteFile(filepath.Join(conf, "lthn.yaml"), []byte(":::not yaml:::"), 0o644))

	r := firstlaunch.Detect(nil)
	state := r.Value.(firstlaunch.State)
	core.AssertTrue(t, state.HasConfigFile)
	core.AssertFalse(t, state.HasRoutes, "corrupt yaml should be treated as 'no routes'")
}
