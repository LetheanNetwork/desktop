// SPDX-Licence-Identifier: EUPL-1.2

// Tests for the local-model scanner. Stubs HOME → tempdir so the scan
// runs against a controlled directory tree without touching the real
// ~/Lethean/conf/models/.

package models_test

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/models"
)

// modelsFixture rebinds $HOME to a tempdir, creates an isolated
// ~/Lethean/conf/models/ layout, and returns the models directory
// path so tests can populate it. t.Setenv restores $HOME.
func modelsFixture(t *core.T) string {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	dir := core.PathJoin(tmp, "Lethean", "conf", "models")
	core.AssertTrue(t, core.MkdirAll(dir, 0o755).OK)
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
	core.AssertTrue(t, core.Mkdir(core.PathJoin(dir, "gemma-4-e2b"), 0o755).OK)
	core.AssertTrue(t, core.WriteFile(core.PathJoin(dir, "llama-3.2-3b.gguf"), []byte("xxxx"), 0o644).OK)

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
	blocker := core.PathJoin(tmp, "blocker")
	core.AssertTrue(t, core.WriteFile(blocker, []byte("x"), 0o644).OK)
	t.Setenv("HOME", blocker)

	r := models.List()
	core.AssertFalse(t, r.OK, "List() must Fail when ModelsDir() fails")
}
