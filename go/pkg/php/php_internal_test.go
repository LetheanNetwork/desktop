// SPDX-Licence-Identifier: EUPL-1.2

package php

import core "dappco.re/go"

func writePHPFixtureFile(t *core.T, path, content string) {
	core.AssertTrue(t, core.MkdirAll(core.PathDir(path), 0o755).OK)
	core.AssertTrue(t, core.WriteFile(path, []byte(content), 0o644).OK)
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
