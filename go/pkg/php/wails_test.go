// SPDX-Licence-Identifier: EUPL-1.2

package php_test

import (
	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/php"
)

func writeWailsPHPFixtureFile(t *core.T, path, content string) {
	core.AssertTrue(t, core.MkdirAll(core.PathDir(path), 0o755).OK)
	core.AssertTrue(t, core.WriteFile(path, []byte(content), 0o644).OK)
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
