// SPDX-Licence-Identifier: EUPL-1.2

// Tests for the lthn filesystem-layout surface.
//
// Pattern matches core/go canon: external `_test` package, dot-import
// of `dappco.re/go` so AssertEqual / AssertTrue / *T resolve without
// a separate `import "testing"` line. Each function gets the AX
// Good / Bad / Ugly triplet where applicable; pure path-construction
// helpers cover the Good case alone since there's no failure path
// short of os.MkdirAll panicking, which AssertNotPanics guards.
//
// Tests isolate via HOME override + t.TempDir() so they never touch
// the real ~/Lethean/.

package paths_test

import (
	"os"
	"strings"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/paths"
)

// homeFixture rebinds $HOME to a t-scoped temp dir for the duration of
// the test. The deferred restore runs at t.Cleanup so nested t.Run
// children share the same isolated home unless they call homeFixture
// themselves.
func homeFixture(t *core.T) string {
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
	return tmp
}

func TestPaths_Root_Good(t *core.T) {
	home := homeFixture(t)
	r := paths.Root()
	core.AssertTrue(t, r.OK, "Root() should succeed under a writable HOME")
	got := r.Value.(string)
	core.AssertEqual(t, core.PathJoin(home, "Lethean"), got)
	// MkdirAll side effect: directory exists after the call.
	info, err := os.Stat(got)
	core.AssertNoError(t, err)
	core.AssertTrue(t, info.IsDir(), "Root path should be a directory")
}

func TestPaths_ConfDir_Good(t *core.T) {
	home := homeFixture(t)
	r := paths.ConfDir()
	core.AssertTrue(t, r.OK)
	core.AssertEqual(t, core.PathJoin(home, "Lethean", "conf"), r.Value.(string))
	info, err := os.Stat(r.Value.(string))
	core.AssertNoError(t, err)
	core.AssertTrue(t, info.IsDir())
}

func TestPaths_DataDir_Good(t *core.T) {
	home := homeFixture(t)
	r := paths.DataDir()
	core.AssertTrue(t, r.OK)
	core.AssertEqual(t, core.PathJoin(home, "Lethean", "data"), r.Value.(string))
}

func TestPaths_WalletsDir_Good(t *core.T) {
	home := homeFixture(t)
	r := paths.WalletsDir()
	core.AssertTrue(t, r.OK)
	core.AssertEqual(t, core.PathJoin(home, "Lethean", "wallets"), r.Value.(string))
}

func TestPaths_CliDir_Good(t *core.T) {
	home := homeFixture(t)
	r := paths.CliDir()
	core.AssertTrue(t, r.OK)
	core.AssertEqual(t, core.PathJoin(home, "Lethean", "cli"), r.Value.(string))
}

func TestPaths_ModelsDir_Good(t *core.T) {
	home := homeFixture(t)
	r := paths.ModelsDir()
	core.AssertTrue(t, r.OK)
	core.AssertEqual(t, core.PathJoin(home, "Lethean", "conf", "models"), r.Value.(string))
	info, err := os.Stat(r.Value.(string))
	core.AssertNoError(t, err)
	core.AssertTrue(t, info.IsDir())
}

func TestPaths_ConfigFile_Good(t *core.T) {
	home := homeFixture(t)
	r := paths.ConfigFile()
	core.AssertTrue(t, r.OK)
	core.AssertEqual(t, core.PathJoin(home, "Lethean", "conf", "lthn.yaml"), r.Value.(string))
	// ConfigFile is path-only — the file should NOT exist after the call.
	_, err := os.Stat(r.Value.(string))
	core.AssertTrue(t, os.IsNotExist(err), "ConfigFile should not create the file itself")
}

func TestPaths_StoreDB_Good(t *core.T) {
	home := homeFixture(t)
	r := paths.StoreDB()
	core.AssertTrue(t, r.OK)
	core.AssertEqual(t, core.PathJoin(home, "Lethean", "data", "lthn.db"), r.Value.(string))
	// Path-only: file should not exist after the call.
	_, err := os.Stat(r.Value.(string))
	core.AssertTrue(t, os.IsNotExist(err), "StoreDB should not create the file itself")
}

func TestPaths_WorkspaceDir_Good(t *core.T) {
	home := homeFixture(t)
	r := paths.WorkspaceDir()
	core.AssertTrue(t, r.OK)
	core.AssertEqual(t, core.PathJoin(home, "Lethean", "data", "workspace"), r.Value.(string))
	info, err := os.Stat(r.Value.(string))
	core.AssertNoError(t, err)
	core.AssertTrue(t, info.IsDir())
}

// Bad-case: HOME points at a path that already exists as a regular file,
// so MkdirAll fails. Every public helper should propagate the Fail
// Result rather than panic.
func TestPaths_Root_Bad_HomeIsFile(t *core.T) {
	tmp := t.TempDir()
	filePath := core.PathJoin(tmp, "not-a-dir")
	core.AssertNoError(t, os.WriteFile(filePath, []byte("blocker"), 0o644))
	prev, hadPrev := os.LookupEnv("HOME")
	core.AssertNoError(t, os.Setenv("HOME", filePath))
	t.Cleanup(func() {
		if hadPrev {
			_ = os.Setenv("HOME", prev)
		} else {
			_ = os.Unsetenv("HOME")
		}
	})

	r := paths.Root()
	core.AssertFalse(t, r.OK, "Root() must Fail when HOME is a regular file")
}

// Bad: HOME is a regular file, so Root() fails and every helper that
// goes through subdir() must propagate the Fail rather than panic.
// Run all in one test so each helper's early-return branch counts
// toward coverage without ballooning the test file.
func TestPaths_Subdir_Bad_PropagatesRootFail(t *core.T) {
	tmp := t.TempDir()
	filePath := core.PathJoin(tmp, "blocker")
	core.AssertNoError(t, os.WriteFile(filePath, []byte("x"), 0o644))
	prev, hadPrev := os.LookupEnv("HOME")
	core.AssertNoError(t, os.Setenv("HOME", filePath))
	t.Cleanup(func() {
		if hadPrev {
			_ = os.Setenv("HOME", prev)
		} else {
			_ = os.Unsetenv("HOME")
		}
	})

	calls := []struct {
		name string
		fn   func() core.Result
	}{
		{"ConfDir", paths.ConfDir},
		{"DataDir", paths.DataDir},
		{"WalletsDir", paths.WalletsDir},
		{"CliDir", paths.CliDir},
		{"ModelsDir", paths.ModelsDir},
		{"ConfigFile", paths.ConfigFile},
		{"StoreDB", paths.StoreDB},
		{"WorkspaceDir", paths.WorkspaceDir},
	}
	for _, c := range calls {
		r := c.fn()
		core.AssertFalse(t, r.OK, c.name+" must Fail when HOME is unusable")
	}
}

// Ugly: every helper exercised back-to-back under the same HOME so the
// idempotency guarantee (calling twice doesn't break) is covered. Run
// in a single test to keep the table-driven shape visible at the top
// of the file.
func TestPaths_Idempotent(t *core.T) {
	homeFixture(t)
	calls := []struct {
		name string
		fn   func() core.Result
	}{
		{"Root", paths.Root},
		{"ConfDir", paths.ConfDir},
		{"DataDir", paths.DataDir},
		{"WalletsDir", paths.WalletsDir},
		{"CliDir", paths.CliDir},
		{"ModelsDir", paths.ModelsDir},
		{"ConfigFile", paths.ConfigFile},
		{"StoreDB", paths.StoreDB},
		{"WorkspaceDir", paths.WorkspaceDir},
	}
	for _, c := range calls {
		r1 := c.fn()
		core.AssertTrue(t, r1.OK, c.name+" first call")
		r2 := c.fn()
		core.AssertTrue(t, r2.OK, c.name+" second call")
		core.AssertEqual(t, r1.Value, r2.Value, c.name+" should be idempotent")
		path := r1.Value.(string)
		core.AssertTrue(t, strings.Contains(path, "Lethean"), c.name+" path should contain 'Lethean'")
	}
}
