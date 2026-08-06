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
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/paths"
)

// homeFixture rebinds $HOME to a t-scoped temp dir for the duration of
// the test. t.Setenv restores the prior value during cleanup.
func homeFixture(t *core.T) string {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	return tmp
}

func TestPaths_Root_Good(t *core.T) {
	home := homeFixture(t)
	r := paths.Root()
	core.AssertTrue(t, r.OK, "Root() should succeed under a writable HOME")
	got := r.Value.(string)
	core.AssertEqual(t, core.PathJoin(home, "Lethean"), got)
	// MkdirAll side effect: directory exists after the call.
	stat := core.Stat(got)
	core.AssertTrue(t, stat.OK)
	info := stat.Value.(core.FsFileInfo)
	core.AssertTrue(t, info.IsDir(), "Root path should be a directory")
}

func TestPaths_ConfDir_Good(t *core.T) {
	home := homeFixture(t)
	r := paths.ConfDir()
	core.AssertTrue(t, r.OK)
	core.AssertEqual(t, core.PathJoin(home, "Lethean", "conf"), r.Value.(string))
	stat := core.Stat(r.Value.(string))
	core.AssertTrue(t, stat.OK)
	info := stat.Value.(core.FsFileInfo)
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
	stat := core.Stat(r.Value.(string))
	core.AssertTrue(t, stat.OK)
	info := stat.Value.(core.FsFileInfo)
	core.AssertTrue(t, info.IsDir())
}

func TestPaths_ConfigFile_Good(t *core.T) {
	home := homeFixture(t)
	r := paths.ConfigFile()
	core.AssertTrue(t, r.OK)
	core.AssertEqual(t, core.PathJoin(home, "Lethean", "conf", "lthn.yaml"), r.Value.(string))
	// ConfigFile is path-only — the file should NOT exist after the call.
	stat := core.Stat(r.Value.(string))
	core.AssertTrue(t, core.IsNotExist(stat.Value.(error)), "ConfigFile should not create the file itself")
}

func TestPaths_StoreDB_Good(t *core.T) {
	home := homeFixture(t)
	r := paths.StoreDB()
	core.AssertTrue(t, r.OK)
	core.AssertEqual(t, core.PathJoin(home, "Lethean", "data", "lthn.db"), r.Value.(string))
	// Path-only: file should not exist after the call.
	stat := core.Stat(r.Value.(string))
	core.AssertTrue(t, core.IsNotExist(stat.Value.(error)), "StoreDB should not create the file itself")
}

func TestPaths_MasterDB_Good(t *core.T) {
	home := homeFixture(t)
	r := paths.MasterDB()
	core.AssertTrue(t, r.OK)
	core.AssertEqual(t, core.PathJoin(home, "Lethean", "data", "lthn.duckdb"), r.Value.(string))
	// Path-only: file should not exist after the call (store.OpenDuckDB creates it).
	stat := core.Stat(r.Value.(string))
	core.AssertTrue(t, core.IsNotExist(stat.Value.(error)), "MasterDB should not create the file itself")
}

func TestPaths_KeysDir_Good(t *core.T) {
	home := homeFixture(t)
	r := paths.KeysDir()
	core.AssertTrue(t, r.OK)
	core.AssertEqual(t, core.PathJoin(home, "Lethean", "data", "keys"), r.Value.(string))
	stat := core.Stat(r.Value.(string))
	core.AssertTrue(t, stat.OK)
	info := stat.Value.(core.FsFileInfo)
	core.AssertTrue(t, info.IsDir())
}

func TestPaths_WorkspaceDir_Good(t *core.T) {
	home := homeFixture(t)
	r := paths.WorkspaceDir()
	core.AssertTrue(t, r.OK)
	core.AssertEqual(t, core.PathJoin(home, "Lethean", "data", "workspace"), r.Value.(string))
	stat := core.Stat(r.Value.(string))
	core.AssertTrue(t, stat.OK)
	info := stat.Value.(core.FsFileInfo)
	core.AssertTrue(t, info.IsDir())
}

// Bad-case: HOME points at a path that already exists as a regular file,
// so MkdirAll fails. Every public helper should propagate the Fail
// Result rather than panic.
func TestPaths_Root_Bad_HomeIsFile(t *core.T) {
	tmp := t.TempDir()
	filePath := core.PathJoin(tmp, "not-a-dir")
	core.AssertTrue(t, core.WriteFile(filePath, []byte("blocker"), 0o644).OK)
	t.Setenv("HOME", filePath)

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
	core.AssertTrue(t, core.WriteFile(filePath, []byte("x"), 0o644).OK)
	t.Setenv("HOME", filePath)

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
		{"MasterDB", paths.MasterDB},
		{"KeysDir", paths.KeysDir},
		{"DesktopDir", paths.DesktopDir},
		{"AIDB", paths.AIDB},
		{"MLDB", paths.MLDB},
		{"R1Dir", paths.R1Dir},
		{"TrainingCheckpointDir", paths.TrainingCheckpointDir},
		{"WelfareDir", paths.WelfareDir},
	}
	for _, c := range calls {
		r := c.fn()
		core.AssertFalse(t, r.OK, c.name+" must Fail when HOME is unusable")
	}
}

// --- Desktop per-app namespace + subsystem DBs -----------------------
//
// DesktopDir / AIDB / MLDB / R1Dir / TrainingCheckpointDir / WelfareDir
// had zero test coverage before this block — nothing else in the
// suite happened to call them.

func TestPaths_DesktopDir_Good(t *core.T) {
	home := homeFixture(t)
	r := paths.DesktopDir()
	core.AssertTrue(t, r.OK)
	core.AssertEqual(t, core.PathJoin(home, "Lethean", "data", "desktop"), r.Value.(string))
	stat := core.Stat(r.Value.(string))
	core.AssertTrue(t, stat.OK)
	core.AssertTrue(t, stat.Value.(core.FsFileInfo).IsDir())
}

func TestPaths_AIDB_Good(t *core.T) {
	home := homeFixture(t)
	r := paths.AIDB()
	core.AssertTrue(t, r.OK)
	core.AssertEqual(t, core.PathJoin(home, "Lethean", "data", "desktop", "ai.duckdb"), r.Value.(string))
	// Path-only: file should not exist after the call.
	stat := core.Stat(r.Value.(string))
	core.AssertTrue(t, core.IsNotExist(stat.Value.(error)), "AIDB should not create the file itself")
}

func TestPaths_MLDB_Good(t *core.T) {
	home := homeFixture(t)
	r := paths.MLDB()
	core.AssertTrue(t, r.OK)
	core.AssertEqual(t, core.PathJoin(home, "Lethean", "data", "desktop", "ml.duckdb"), r.Value.(string))
	stat := core.Stat(r.Value.(string))
	core.AssertTrue(t, core.IsNotExist(stat.Value.(error)), "MLDB should not create the file itself")
}

func TestPaths_R1Dir_Good(t *core.T) {
	home := homeFixture(t)
	r := paths.R1Dir()
	core.AssertTrue(t, r.OK)
	core.AssertEqual(t, core.PathJoin(home, "Lethean", "data", "r1"), r.Value.(string))
	stat := core.Stat(r.Value.(string))
	core.AssertTrue(t, stat.OK)
	core.AssertTrue(t, stat.Value.(core.FsFileInfo).IsDir())
}

func TestPaths_TrainingCheckpointDir_Good(t *core.T) {
	home := homeFixture(t)
	r := paths.TrainingCheckpointDir()
	core.AssertTrue(t, r.OK)
	core.AssertEqual(t, core.PathJoin(home, "Lethean", "data", "training", "checkpoints"), r.Value.(string))
	stat := core.Stat(r.Value.(string))
	core.AssertTrue(t, stat.OK)
	core.AssertTrue(t, stat.Value.(core.FsFileInfo).IsDir())
}

func TestPaths_WelfareDir_Good(t *core.T) {
	home := homeFixture(t)
	r := paths.WelfareDir()
	core.AssertTrue(t, r.OK)
	core.AssertEqual(t, core.PathJoin(home, "Lethean", "data", "welfare"), r.Value.(string))
	stat := core.Stat(r.Value.(string))
	core.AssertTrue(t, stat.OK)
	core.AssertTrue(t, stat.Value.(core.FsFileInfo).IsDir())
}

// TestPaths_KeysDir_Bad_ParentDenied — DataDir succeeds (created
// earlier) but is then made read-only, so KeysDir's own MkdirAll for
// the "keys" leaf fails. Distinct from the HOME-is-a-file propagation
// case above: here Root()/DataDir() both succeed and only the final
// per-function MkdirAll denies.
func TestPaths_KeysDir_Bad_ParentDenied(t *core.T) {
	homeFixture(t)
	data := paths.DataDir()
	core.AssertTrue(t, data.OK, "fixture DataDir must succeed")
	dataDir := data.Value.(string)

	if r := core.Chmod(dataDir, 0o500); !r.OK {
		t.Skipf("chmod unsupported on this fs: %v", r.Error())
	}
	defer func() { core.Chmod(dataDir, 0o755) }() // restore before t.TempDir() cleanup

	r := paths.KeysDir()
	core.AssertFalse(t, r.OK, "KeysDir must fail when its parent directory denies write")
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
		core.AssertTrue(t, core.Contains(path, "Lethean"), c.name+" path should contain 'Lethean'")
	}
}
