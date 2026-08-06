// SPDX-Licence-Identifier: EUPL-1.2

package build

import (
	core "dappco.re/go"
	"dappco.re/go/process"
)

func buildDetectFixture(t *core.T) string {
	t.Setenv("PATH", "")
	return t.TempDir()
}

// buildDetectFixtureWithCore mirrors buildDetectFixture but puts a fake
// executable `core` binary on PATH so hasCore() reports true — the
// house pattern for exec-boundary tests (no real `core` invocation,
// just an executable-bit probe via core.App{}.Find).
func buildDetectFixtureWithCore(t *core.T) string {
	binDir := t.TempDir()
	core.AssertTrue(t, core.WriteFile(core.PathJoin(binDir, "core"), []byte("#!/bin/sh\nexit 0\n"), 0o755).OK)
	t.Setenv("PATH", binDir)
	return t.TempDir()
}

func writeBuildMarker(t *core.T, path string) {
	core.AssertTrue(t, core.MkdirAll(core.PathDir(path), 0o755).OK)
	core.AssertTrue(t, core.WriteFile(path, []byte("{}"), 0o644).OK)
}

// buildProcHarness returns a *Service backed by a real dappco.re/go/process
// Service registered under the "process" name, matching the boot-order
// contract Service.proc() expects. Mirrors pkg/bridge/process_test.go's
// processHarness — real (short-lived, no fixed-port) subprocesses only.
func buildProcHarness(t *core.T) *Service {
	t.Helper()
	c := coreNewWithProcess(t)
	return NewService(c)
}

func coreNewWithProcess(t *core.T) *core.Core {
	t.Helper()
	c := core.New()
	r := process.NewService(process.Options{})(c)
	core.AssertTrue(t, r.OK)
	ps := r.Value.(*process.Service)
	core.AssertTrue(t, ps.OnStartup(core.Background()).OK)
	core.AssertTrue(t, c.RegisterService("process", ps).OK)
	return c
}

func TestBuild_detectProject_Good_WailsFallback(t *core.T) {
	root := buildDetectFixture(t)
	writeBuildMarker(t, core.PathJoin(root, "wails.json"))

	detected := detectProject(root)

	core.AssertEqual(t, "wails", detected.ProjectType)
	core.AssertEqual(t, "wails", detected.Command)
	core.AssertEqual(t, []string{"build"}, detected.Args)
	core.AssertFalse(t, detected.HasCoreBin)
}

func TestBuild_detectProject_Bad_ConfigWithoutCore(t *core.T) {
	root := buildDetectFixture(t)
	writeBuildMarker(t, core.PathJoin(root, ".core", "build.yaml"))

	detected := detectProject(root)

	core.AssertEqual(t, "config", detected.ProjectType)
	core.AssertEqual(t, "", detected.Command)
	core.AssertEqual(t, 0, len(detected.Args))
	core.AssertFalse(t, detected.HasCoreBin)
}

func TestBuild_detectProject_Ugly_GoSubdirFallback(t *core.T) {
	root := buildDetectFixture(t)
	goRoot := core.PathJoin(root, "go")
	writeBuildMarker(t, core.PathJoin(goRoot, "go.mod"))

	detected := detectProject(root)

	core.AssertEqual(t, "go-subdir", detected.ProjectType)
	core.AssertEqual(t, "sh", detected.Command)
	core.AssertEqual(t, []string{"-c", "cd " + shellQuote(goRoot) + goBuildNoOutputSuffix}, detected.Args)
	core.AssertFalse(t, detected.HasCoreBin)
}

// --- remaining detectProject marker-priority branches ---

func TestBuild_detectProject_Good_ConfigWithCore(t *core.T) {
	root := buildDetectFixtureWithCore(t)
	writeBuildMarker(t, core.PathJoin(root, ".core", "build.yaml"))

	detected := detectProject(root)

	core.AssertEqual(t, "config", detected.ProjectType)
	core.AssertEqual(t, "core", detected.Command)
	core.AssertEqual(t, []string{"build"}, detected.Args)
	core.AssertTrue(t, detected.HasCoreBin)
}

func TestBuild_detectProject_Good_WailsWithCore(t *core.T) {
	root := buildDetectFixtureWithCore(t)
	writeBuildMarker(t, core.PathJoin(root, "wails.json"))

	detected := detectProject(root)

	core.AssertEqual(t, "wails", detected.ProjectType)
	core.AssertEqual(t, "core", detected.Command)
	core.AssertEqual(t, []string{"build"}, detected.Args)
	core.AssertTrue(t, detected.HasCoreBin)
}

func TestBuild_detectProject_Bad_GoModNoCore(t *core.T) {
	root := buildDetectFixture(t)
	writeBuildMarker(t, core.PathJoin(root, "go.mod"))

	detected := detectProject(root)

	core.AssertEqual(t, "go", detected.ProjectType)
	core.AssertEqual(t, "sh", detected.Command)
	core.AssertEqual(t, []string{"-c", "cd " + shellQuote(root) + goBuildNoOutputSuffix}, detected.Args)
}

func TestBuild_detectProject_Bad_GoModWithCore(t *core.T) {
	root := buildDetectFixtureWithCore(t)
	writeBuildMarker(t, core.PathJoin(root, "go.mod"))

	detected := detectProject(root)

	core.AssertEqual(t, "go", detected.ProjectType)
	core.AssertEqual(t, "core", detected.Command)
}

func TestBuild_detectProject_Good_GoWorkNoCore(t *core.T) {
	root := buildDetectFixture(t)
	writeBuildMarker(t, core.PathJoin(root, "go.work"))

	detected := detectProject(root)

	core.AssertEqual(t, "go-work", detected.ProjectType)
	core.AssertEqual(t, "sh", detected.Command)
	core.AssertEqual(t, []string{"-c", "cd " + shellQuote(root) + goBuildNoOutputSuffix}, detected.Args)
}

func TestBuild_detectProject_Good_GoWorkWithCore(t *core.T) {
	root := buildDetectFixtureWithCore(t)
	writeBuildMarker(t, core.PathJoin(root, "go.work"))

	detected := detectProject(root)

	core.AssertEqual(t, "go-work", detected.ProjectType)
	core.AssertEqual(t, "core", detected.Command)
}

func TestBuild_detectProject_Bad_DockerNoCore(t *core.T) {
	root := buildDetectFixture(t)
	writeBuildMarker(t, core.PathJoin(root, "Dockerfile"))

	detected := detectProject(root)

	core.AssertEqual(t, "docker", detected.ProjectType)
	core.AssertEqual(t, "docker", detected.Command)
	core.AssertEqual(t, []string{"build", "."}, detected.Args)
}

func TestBuild_detectProject_Bad_DockerWithCore(t *core.T) {
	root := buildDetectFixtureWithCore(t)
	writeBuildMarker(t, core.PathJoin(root, "Dockerfile"))

	detected := detectProject(root)

	core.AssertEqual(t, "docker", detected.ProjectType)
	core.AssertEqual(t, "core", detected.Command)
}

func TestBuild_detectProject_Ugly_CMake(t *core.T) {
	root := buildDetectFixture(t)
	writeBuildMarker(t, core.PathJoin(root, "CMakeLists.txt"))

	detected := detectProject(root)

	core.AssertEqual(t, "cpp", detected.ProjectType)
	core.AssertEqual(t, "cmake", detected.Command)
	core.AssertEqual(t, []string{"--build", "build"}, detected.Args)
}

func TestBuild_detectProject_Ugly_TaskfileYaml(t *core.T) {
	root := buildDetectFixture(t)
	writeBuildMarker(t, core.PathJoin(root, "Taskfile.yaml"))

	detected := detectProject(root)

	core.AssertEqual(t, "taskfile", detected.ProjectType)
	core.AssertEqual(t, "task", detected.Command)
	core.AssertEqual(t, []string{"build"}, detected.Args)
}

func TestBuild_detectProject_Ugly_TaskfileYml(t *core.T) {
	root := buildDetectFixture(t)
	writeBuildMarker(t, core.PathJoin(root, "Taskfile.yml"))

	detected := detectProject(root)

	core.AssertEqual(t, "taskfile", detected.ProjectType)
	core.AssertEqual(t, "task", detected.Command)
}

func TestBuild_detectProject_Bad_Composer(t *core.T) {
	root := buildDetectFixture(t)
	writeBuildMarker(t, core.PathJoin(root, "composer.json"))

	detected := detectProject(root)

	core.AssertEqual(t, "php", detected.ProjectType)
	core.AssertEqual(t, "composer", detected.Command)
	core.AssertEqual(t, []string{"install"}, detected.Args)
}

func TestBuild_detectProject_Bad_Package(t *core.T) {
	root := buildDetectFixture(t)
	writeBuildMarker(t, core.PathJoin(root, "package.json"))

	detected := detectProject(root)

	core.AssertEqual(t, "node", detected.ProjectType)
	core.AssertEqual(t, "npm", detected.Command)
	core.AssertEqual(t, []string{"run", "build"}, detected.Args)
}

func TestBuild_detectProject_Ugly_Unknown(t *core.T) {
	root := buildDetectFixture(t)

	detected := detectProject(root)

	core.AssertEqual(t, "unknown", detected.ProjectType)
	core.AssertEqual(t, "", detected.Command)
}

// --- hasCore / exists ---

func TestBuild_hasCore_Good_FindsBinaryOnPath(t *core.T) {
	_ = buildDetectFixtureWithCore(t)
	core.AssertTrue(t, hasCore())
}

func TestBuild_hasCore_Bad_NoBinaryOnPath(t *core.T) {
	t.Setenv("PATH", "")
	core.AssertFalse(t, hasCore())
}

// --- shellQuote ---

func TestBuild_shellQuote_Good_PlainPath(t *core.T) {
	core.AssertEqual(t, "'/tmp/site'", shellQuote("/tmp/site"))
}

func TestBuild_shellQuote_Bad_Empty(t *core.T) {
	core.AssertEqual(t, "''", shellQuote(""))
}

func TestBuild_shellQuote_Ugly_EmbeddedSingleQuote(t *core.T) {
	core.AssertEqual(t, `'/tmp/o'\''brien'`, shellQuote("/tmp/o'brien"))
}

// --- proc() boot-order resolution ---

func TestBuild_proc_Bad_NilService(t *core.T) {
	var s *Service
	core.AssertNil(t, s.proc())
}

func TestBuild_proc_Bad_NilCore(t *core.T) {
	s := &Service{}
	core.AssertNil(t, s.proc())
}

func TestBuild_proc_Bad_ServiceNotRegistered(t *core.T) {
	s := NewService(core.New())
	core.AssertNil(t, s.proc())
}

func TestBuild_proc_Good_Resolved(t *core.T) {
	s := buildProcHarness(t)
	core.AssertNotNil(t, s.proc())
}

// --- startProc ---

func TestBuild_startProc_Bad_ServiceUnavailable(t *core.T) {
	s := NewService(core.New())
	r := s.startProc(t.TempDir(), "/bin/echo", []string{"hi"})
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), processServiceUnavailable)
}

func TestBuild_startProc_Good_Spawns(t *core.T) {
	s := buildProcHarness(t)
	r := s.startProc(t.TempDir(), "/bin/echo", []string{"hi"})
	core.AssertTrue(t, r.OK)
	p, ok := r.Value.(*process.Process)
	core.AssertTrue(t, ok)
	core.AssertNotEmpty(t, p.ID)
}

func TestBuild_startProc_Ugly_SpawnFails(t *core.T) {
	s := buildProcHarness(t)
	r := s.startProc(t.TempDir(), "definitely-not-a-real-binary-xyz", nil)
	core.AssertFalse(t, r.OK)
}
