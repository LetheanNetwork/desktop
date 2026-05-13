// SPDX-Licence-Identifier: EUPL-1.2

package build

import core "dappco.re/go"

func buildDetectFixture(t *core.T) string {
	t.Setenv("PATH", "")
	return t.TempDir()
}

func writeBuildMarker(t *core.T, path string) {
	core.AssertTrue(t, core.MkdirAll(core.PathDir(path), 0o755).OK)
	core.AssertTrue(t, core.WriteFile(path, []byte("{}"), 0o644).OK)
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
