// SPDX-Licence-Identifier: EUPL-1.2

package php_test

import (
	core "dappco.re/go"
	"dappco.re/go/process"
	subject "dappco.re/lthn/desktop/pkg/php"
)

func writeWailsPHPFixtureFile(t *core.T, path, content string) {
	core.AssertTrue(t, core.MkdirAll(core.PathDir(path), 0o755).OK)
	core.AssertTrue(t, core.WriteFile(path, []byte(content), 0o644).OK)
}

// phpWailsProcHarness returns a subject.Service backed by a real
// dappco.re/go/process Service registered under the "process" name —
// mirrors pkg/bridge/process_test.go's processHarness. Real (short-lived,
// no fixed-port) subprocesses only.
func phpWailsProcHarness(t *core.T) *subject.Service {
	t.Helper()
	c := core.New()
	r := process.NewService(process.Options{})(c)
	core.AssertTrue(t, r.OK)
	ps := r.Value.(*process.Service)
	core.AssertTrue(t, ps.OnStartup(core.Background()).OK)
	core.AssertTrue(t, c.RegisterService("process", ps).OK)
	return subject.NewService(c)
}

func writeWailsLaravelFixture(t *core.T, path string) {
	writeWailsPHPFixtureFile(t, core.PathJoin(path, "artisan"), "#!/usr/bin/env php\n")
	writeWailsPHPFixtureFile(t, core.PathJoin(path, "composer.json"), `{"require":{"laravel/framework":"^11.0"}}`)
	writeWailsPHPFixtureFile(t, core.PathJoin(path, ".env"), "APP_NAME=Fixture\nAPP_URL=http://fixture.test\n")
}

func TestWails_Service_Scripts_Good_ComposerScripts(t *core.T) {
	root := t.TempDir()
	writeWailsPHPFixtureFile(t, core.PathJoin(root, "artisan"), "#!/usr/bin/env php\n")
	writeWailsPHPFixtureFile(t, core.PathJoin(root, "composer.json"), `{
		"scripts": {
			"dev": "vite --host 0.0.0.0",
			"post-root-package-install": [
				"Composer\\\\Config::disableProcessTimeout",
				"php artisan key:generate"
			]
		}
	}`)

	r := subject.NewService(nil).Scripts(root)

	core.AssertTrue(t, r.OK)
	out := r.Value.(subject.ScriptsOutput)
	core.AssertTrue(t, out.HasArtisan)
	core.AssertTrue(t, out.HasComposer)
	core.AssertEqual(t, 2, len(out.ComposerScripts))
	core.AssertEqual(t, "dev", out.ComposerScripts[0].Name)
	core.AssertEqual(t, "vite --host 0.0.0.0", out.ComposerScripts[0].Command)
	core.AssertEqual(t, "php artisan key:generate", out.ComposerScripts[1].Command)
	core.AssertGreater(t, len(out.ArtisanScripts), 0)
}

func TestWails_Service_Scripts_Bad_EmptyPath(t *core.T) {
	r := subject.NewService(nil).Scripts("  ")

	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "path required")
}

func TestWails_Service_Scripts_Ugly_PseudoDirectiveFallback(t *core.T) {
	root := t.TempDir()
	writeWailsPHPFixtureFile(t, core.PathJoin(root, "composer.json"), `{
		"scripts": {
			"post-install-cmd": ["Composer\\\\Config::disableProcessTimeout"]
		}
	}`)

	r := subject.NewService(nil).Scripts(root)

	core.AssertTrue(t, r.OK)
	out := r.Value.(subject.ScriptsOutput)
	core.AssertFalse(t, out.HasArtisan)
	core.AssertEqual(t, 1, len(out.ComposerScripts))
	core.AssertEqual(t, "Composer\\\\Config::disableProcessTimeout", out.ComposerScripts[0].Command)
	core.AssertEqual(t, 1, out.ComposerScripts[0].Lines)
}

func TestWails_Service_Scripts_Ugly_NoComposerJSON(t *core.T) {
	root := t.TempDir()
	writeWailsPHPFixtureFile(t, core.PathJoin(root, "artisan"), "#!/usr/bin/env php\n")

	r := subject.NewService(nil).Scripts(root)

	core.AssertTrue(t, r.OK)
	out := r.Value.(subject.ScriptsOutput)
	core.AssertFalse(t, out.HasComposer)
	core.AssertEqual(t, 0, len(out.ComposerScripts))
	core.AssertTrue(t, out.HasArtisan)
	core.AssertGreater(t, len(out.ArtisanScripts), 0)
}

func TestWails_Service_Scripts_Ugly_InvalidJSON(t *core.T) {
	root := t.TempDir()
	writeWailsPHPFixtureFile(t, core.PathJoin(root, "composer.json"), `{not valid json`)

	r := subject.NewService(nil).Scripts(root)

	core.AssertTrue(t, r.OK)
	out := r.Value.(subject.ScriptsOutput)
	core.AssertTrue(t, out.HasComposer)
	core.AssertEqual(t, 0, len(out.ComposerScripts))
}

func TestWails_Service_Scripts_Ugly_NoScriptsKey(t *core.T) {
	root := t.TempDir()
	writeWailsPHPFixtureFile(t, core.PathJoin(root, "composer.json"), `{"require":{}}`)

	r := subject.NewService(nil).Scripts(root)

	core.AssertTrue(t, r.OK)
	out := r.Value.(subject.ScriptsOutput)
	core.AssertEqual(t, 0, len(out.ComposerScripts))
}

func TestWails_Service_Scripts_Ugly_ScriptsNotAnObject(t *core.T) {
	root := t.TempDir()
	writeWailsPHPFixtureFile(t, core.PathJoin(root, "composer.json"), `{"scripts": "not-an-object"}`)

	r := subject.NewService(nil).Scripts(root)

	core.AssertTrue(t, r.OK)
	out := r.Value.(subject.ScriptsOutput)
	core.AssertEqual(t, 0, len(out.ComposerScripts))
}

// --- Detect ---

func TestWails_Service_Detect_Good_ExplicitRoots(t *core.T) {
	root := t.TempDir()
	project := core.PathJoin(root, "site")
	writeWailsLaravelFixture(t, project)

	r := subject.NewService(nil).Detect([]string{root}, 3)

	core.AssertTrue(t, r.OK)
	out := r.Value.(subject.DetectOutput)
	core.AssertEqual(t, 1, len(out.Projects))
	core.AssertEqual(t, 1, out.Count)
	core.AssertEqual(t, []string{root}, out.Roots)
}

func TestWails_Service_Detect_Good_DefaultRootsWhenEmpty(t *core.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	project := core.PathJoin(home, "Code", "lab", "site")
	writeWailsLaravelFixture(t, project)

	r := subject.NewService(nil).Detect(nil, 3)

	core.AssertTrue(t, r.OK)
	out := r.Value.(subject.DetectOutput)
	core.AssertEqual(t, 3, len(out.Roots))
	core.AssertEqual(t, 1, out.Count)
	core.AssertEqual(t, project, out.Projects[0].Path)
}

// --- Project ---

func TestWails_Service_Project_Bad_EmptyPath(t *core.T) {
	r := subject.NewService(nil).Project("")
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "path is required")
}

func TestWails_Service_Project_Bad_NotLaravelProject(t *core.T) {
	root := t.TempDir()
	r := subject.NewService(nil).Project(root)
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "not a Laravel project")
}

func TestWails_Service_Project_Good_FullDetail(t *core.T) {
	root := t.TempDir()
	writeWailsLaravelFixture(t, root)
	writeWailsPHPFixtureFile(t, core.PathJoin(root, ".env.example"), "APP_NAME=Fixture\n")
	writeWailsPHPFixtureFile(t, core.PathJoin(root, "composer.lock"), "{}")
	writeWailsPHPFixtureFile(t, core.PathJoin(root, "package-lock.json"), "{}")
	writeWailsPHPFixtureFile(t, core.PathJoin(root, "vendor", "autoload.php"), "<?php\n")
	writeWailsPHPFixtureFile(t, core.PathJoin(root, "node_modules", ".bin", "x"), "")

	r := subject.NewService(nil).Project(root)

	core.AssertTrue(t, r.OK)
	out := r.Value.(subject.ProjectOutput)
	core.AssertEqual(t, root, out.Detail.Path)
	core.AssertTrue(t, out.Detail.HasEnv)
	core.AssertTrue(t, out.Detail.HasEnvExample)
	core.AssertTrue(t, out.Detail.HasVendor)
	core.AssertTrue(t, out.Detail.HasComposerLock)
	core.AssertTrue(t, out.Detail.HasNodeModules)
	core.AssertTrue(t, out.Detail.HasPackageLock)
}

func TestWails_Service_Project_Ugly_MinimalDetailNoOptionalFiles(t *core.T) {
	root := t.TempDir()
	writeWailsLaravelFixture(t, root)

	r := subject.NewService(nil).Project(root)

	core.AssertTrue(t, r.OK)
	out := r.Value.(subject.ProjectOutput)
	core.AssertFalse(t, out.Detail.HasEnvExample)
	core.AssertFalse(t, out.Detail.HasVendor)
	core.AssertFalse(t, out.Detail.HasComposerLock)
	core.AssertFalse(t, out.Detail.HasNodeModules)
	core.AssertFalse(t, out.Detail.HasPackageLock)
}

// --- Run ---

func TestWails_Service_Run_Bad_MissingPathOrMode(t *core.T) {
	svc := subject.NewService(nil)
	r := svc.Run(subject.RunInput{})
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "path and mode required")
}

func TestWails_Service_Run_Bad_ComposerMissingName(t *core.T) {
	svc := subject.NewService(nil)
	r := svc.Run(subject.RunInput{Path: t.TempDir(), Mode: "composer"})
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "name required for composer mode")
}

func TestWails_Service_Run_Bad_ArtisanMissingArgs(t *core.T) {
	svc := subject.NewService(nil)
	r := svc.Run(subject.RunInput{Path: t.TempDir(), Mode: "artisan"})
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "args required for artisan mode")
}

func TestWails_Service_Run_Bad_RawMissingCommand(t *core.T) {
	svc := subject.NewService(nil)
	r := svc.Run(subject.RunInput{Path: t.TempDir(), Mode: "raw"})
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "command required for raw mode")
}

func TestWails_Service_Run_Bad_UnknownMode(t *core.T) {
	svc := subject.NewService(nil)
	r := svc.Run(subject.RunInput{Path: t.TempDir(), Mode: "sorcery"})
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "unknown mode")
}

func TestWails_Service_Run_Ugly_ProcessServiceUnavailable(t *core.T) {
	svc := subject.NewService(core.New())
	r := svc.Run(subject.RunInput{Path: t.TempDir(), Mode: "raw", Command: "echo hi"})
	core.AssertFalse(t, r.OK)
}

func TestWails_Service_Run_Good_ComposerMode(t *core.T) {
	svc := phpWailsProcHarness(t)
	r := svc.Run(subject.RunInput{Path: t.TempDir(), Mode: "composer", Name: "dev"})
	core.AssertTrue(t, r.OK)
	out := r.Value.(subject.RunOutput)
	core.AssertEqual(t, "composer", out.Command)
	core.AssertEqual(t, []string{"run-script", "dev"}, out.Args)
	core.AssertNotEmpty(t, out.ID)
}

func TestWails_Service_Run_Good_ArtisanMode(t *core.T) {
	svc := phpWailsProcHarness(t)
	r := svc.Run(subject.RunInput{Path: t.TempDir(), Mode: "artisan", Args: []string{"route:list"}})
	core.AssertTrue(t, r.OK)
	out := r.Value.(subject.RunOutput)
	core.AssertEqual(t, "php", out.Command)
	core.AssertEqual(t, []string{"artisan", "route:list"}, out.Args)
}

func TestWails_Service_Run_Good_RawMode(t *core.T) {
	svc := phpWailsProcHarness(t)
	r := svc.Run(subject.RunInput{Path: t.TempDir(), Mode: "RAW", Command: "echo hi"})
	core.AssertTrue(t, r.OK)
	out := r.Value.(subject.RunOutput)
	core.AssertEqual(t, "sh", out.Command)
	core.AssertEqual(t, []string{"-c", "echo hi"}, out.Args)
}
