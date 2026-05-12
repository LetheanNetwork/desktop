// SPDX-Licence-Identifier: EUPL-1.2

// Tests for the local-model scanner. Stubs HOME → tempdir so the scan
// runs against a controlled directory tree without touching the real
// ~/Lethean/conf/models/.

package models_test

import (
	"os"
	"path/filepath"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/models"
)

// modelsFixture rebinds $HOME to a tempdir, creates an isolated
// ~/Lethean/conf/models/ layout, and returns the models directory
// path so tests can populate it. The Cleanup hook restores $HOME.
func modelsFixture(t *core.T) string {
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
	dir := filepath.Join(tmp, "Lethean", "conf", "models")
	core.AssertNoError(t, os.MkdirAll(dir, 0o755))
	return dir
}

func TestModels_List_Good_Empty(t *core.T) {
	modelsFixture(t)
	r := models.List()
	core.AssertTrue(t, r.OK)
	core.AssertLen(t, r.Value, 0, "empty models directory should produce zero entries")
}

func TestModels_List_Good_WithEntries(t *core.T) {
	dir := modelsFixture(t)
	// Plant two model artefacts — one directory (a HF-style snapshot),
	// one regular file (a single-file .gguf).
	core.AssertNoError(t, os.Mkdir(filepath.Join(dir, "gemma-4-e2b"), 0o755))
	core.AssertNoError(t, os.WriteFile(filepath.Join(dir, "llama-3.2-3b.gguf"), []byte("xxxx"), 0o644))

	r := models.List()
	core.AssertTrue(t, r.OK)
	entries := r.Value.([]models.Entry)
	core.AssertLen(t, entries, 2)

	// Index by name so order-of-iteration doesn't make the assert flaky.
	byName := map[string]models.Entry{}
	for _, e := range entries {
		byName[e.Name] = e
	}
	gemma, ok := byName["gemma-4-e2b"]
	core.AssertTrue(t, ok)
	core.AssertTrue(t, gemma.IsDir)

	llama, ok := byName["llama-3.2-3b.gguf"]
	core.AssertTrue(t, ok)
	core.AssertFalse(t, llama.IsDir)
	core.AssertEqual(t, int64(4), llama.Size)
}

// Bad: HOME unusable → ModelsDir() fails → List() propagates.
func TestModels_List_Bad_HomeIsFile(t *core.T) {
	tmp := t.TempDir()
	blocker := filepath.Join(tmp, "blocker")
	core.AssertNoError(t, os.WriteFile(blocker, []byte("x"), 0o644))
	prev, hadPrev := os.LookupEnv("HOME")
	core.AssertNoError(t, os.Setenv("HOME", blocker))
	t.Cleanup(func() {
		if hadPrev {
			_ = os.Setenv("HOME", prev)
		} else {
			_ = os.Unsetenv("HOME")
		}
	})

	r := models.List()
	core.AssertFalse(t, r.OK, "List() must Fail when ModelsDir() fails")
}
