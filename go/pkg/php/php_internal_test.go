// SPDX-Licence-Identifier: EUPL-1.2

package php

import (
	core "dappco.re/go"
	"dappco.re/go/process"
)

func writePHPFixtureFile(t *core.T, path, content string) {
	core.AssertTrue(t, core.MkdirAll(core.PathDir(path), 0o755).OK)
	core.AssertTrue(t, core.WriteFile(path, []byte(content), 0o644).OK)
}

// phpProcHarness returns a *Service backed by a real dappco.re/go/process
// Service registered under the "process" name — matches the boot-order
// contract Service.proc() expects. Mirrors pkg/bridge/process_test.go's
// processHarness — real (short-lived, no fixed-port) subprocesses only.
func phpProcHarness(t *core.T) *Service {
	t.Helper()
	c := core.New()
	r := process.NewService(process.Options{})(c)
	core.AssertTrue(t, r.OK)
	ps := r.Value.(*process.Service)
	core.AssertTrue(t, ps.OnStartup(core.Background()).OK)
	core.AssertTrue(t, c.RegisterService("process", ps).OK)
	return NewService(c)
}

func writeLaravelFixture(t *core.T, path string) {
	writePHPFixtureFile(t, core.PathJoin(path, "artisan"), "#!/usr/bin/env php\n")
	writePHPFixtureFile(t, core.PathJoin(path, "composer.json"), `{"require":{"laravel/framework":"^11.0"}}`)
	writePHPFixtureFile(t, core.PathJoin(path, ".env"), "APP_NAME=Fixture\nAPP_URL=http://fixture.test\n")
	writePHPFixtureFile(t, core.PathJoin(path, "package-lock.json"), "{}")
}

func TestPHP_detect_Good_FindsLaravelProject(t *core.T) {
	root := t.TempDir()
	project := core.PathJoin(root, "site")
	writeLaravelFixture(t, project)

	projects := NewService(nil).detect([]string{root}, 3)

	core.AssertEqual(t, 1, len(projects))
	core.AssertEqual(t, project, projects[0].Path)
	core.AssertEqual(t, "site", projects[0].Name)
	core.AssertEqual(t, "Fixture", projects[0].AppName)
	core.AssertEqual(t, "http://fixture.test", projects[0].AppURL)
	core.AssertEqual(t, "npm", projects[0].PackageMgr)
}

func TestPHP_detect_Bad_SkipsVendorDirectory(t *core.T) {
	root := t.TempDir()
	writeLaravelFixture(t, core.PathJoin(root, "vendor", "site"))

	projects := NewService(nil).detect([]string{root}, 3)

	core.AssertEqual(t, 0, len(projects))
}

func TestPHP_detect_Ugly_RespectsMaxDepth(t *core.T) {
	root := t.TempDir()
	writeLaravelFixture(t, core.PathJoin(root, "one", "two", "site"))

	projects := NewService(nil).detect([]string{root}, 1)

	core.AssertEqual(t, 0, len(projects))
}

func TestPHP_detect_Ugly_DefaultsMaxDepthWhenZero(t *core.T) {
	root := t.TempDir()
	project := core.PathJoin(root, "site")
	writeLaravelFixture(t, project)

	// maxDepth<=0 must fall back to the documented default of 3.
	projects := NewService(nil).detect([]string{root}, 0)

	core.AssertEqual(t, 1, len(projects))
}

func TestPHP_detect_Ugly_SortsAcrossMultipleRoots(t *core.T) {
	rootA := t.TempDir()
	rootB := t.TempDir()
	writeLaravelFixture(t, core.PathJoin(rootB, "zeta-site"))
	writeLaravelFixture(t, core.PathJoin(rootA, "alpha-site"))

	projects := NewService(nil).detect([]string{rootA, rootB}, 3)

	core.AssertEqual(t, 2, len(projects))
	core.AssertTrue(t, projects[0].Path < projects[1].Path,
		"expected projects sorted by Path across roots")
}

func TestPHP_detectRoot_Ugly_NonexistentRootSkippedSilently(t *core.T) {
	nonexistent := core.PathJoin(t.TempDir(), "does-not-exist")

	projects := NewService(nil).detect([]string{nonexistent}, 3)

	core.AssertEqual(t, 0, len(projects))
}

// --- dirExists ---

func TestPHP_dirExists_Good_DirectoryPresent(t *core.T) {
	dir := t.TempDir()
	core.AssertTrue(t, dirExists(dir))
}

func TestPHP_dirExists_Bad_MissingPath(t *core.T) {
	core.AssertFalse(t, dirExists(core.PathJoin(t.TempDir(), "missing")))
}

func TestPHP_dirExists_Ugly_PathIsFileNotDir(t *core.T) {
	dir := t.TempDir()
	filePath := core.PathJoin(dir, "plain.txt")
	writePHPFixtureFile(t, filePath, "x")
	core.AssertFalse(t, dirExists(filePath))
}

// --- fileExists (remaining branch: missing path) ---

func TestPHP_fileExists_Bad_MissingPath(t *core.T) {
	core.AssertFalse(t, fileExists(core.PathJoin(t.TempDir(), "missing.json")))
}

// --- defaultRoots ---

func TestPHP_defaultRoots_Good_UsesHomeCodeSubdirs(t *core.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	roots := defaultRoots()

	core.AssertEqual(t, 3, len(roots))
	core.AssertEqual(t, core.PathJoin(home, "Code", "lab"), roots[0])
	core.AssertEqual(t, core.PathJoin(home, "Code", "core"), roots[1])
	core.AssertEqual(t, core.PathJoin(home, "Code", "host-uk"), roots[2])
}

// --- Register ---

func TestPHP_Register_Good_ReturnsOKService(t *core.T) {
	c := core.New()
	r := Register(c)
	core.AssertTrue(t, r.OK)
	svc, ok := r.Value.(*Service)
	core.AssertTrue(t, ok)
	core.AssertNotNil(t, svc)
}

// --- proc() boot-order resolution ---

func TestPHP_proc_Bad_NilService(t *core.T) {
	var s *Service
	core.AssertNil(t, s.proc())
}

func TestPHP_proc_Bad_NilCore(t *core.T) {
	s := &Service{}
	core.AssertNil(t, s.proc())
}

func TestPHP_proc_Bad_ServiceNotRegistered(t *core.T) {
	s := NewService(core.New())
	core.AssertNil(t, s.proc())
}

func TestPHP_proc_Good_Resolved(t *core.T) {
	s := phpProcHarness(t)
	core.AssertNotNil(t, s.proc())
}

// --- runProc ---

func TestPHP_runProc_Bad_ServiceUnavailable(t *core.T) {
	s := NewService(core.New())
	r := s.runProc(t.TempDir(), "/bin/echo", []string{"hi"})
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "process service unavailable")
}

func TestPHP_runProc_Good_Spawns(t *core.T) {
	s := phpProcHarness(t)
	r := s.runProc(t.TempDir(), "/bin/echo", []string{"hi"})
	core.AssertTrue(t, r.OK)
	p, ok := r.Value.(*process.Process)
	core.AssertTrue(t, ok)
	core.AssertNotEmpty(t, p.ID)
}

func TestPHP_runProc_Ugly_SpawnFails(t *core.T) {
	s := phpProcHarness(t)
	r := s.runProc(t.TempDir(), "definitely-not-a-real-binary-xyz", nil)
	core.AssertFalse(t, r.OK)
}
