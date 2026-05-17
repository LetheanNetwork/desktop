// SPDX-Licence-Identifier: EUPL-1.2

// Tests for the local-model scanner. Stubs HOME → tempdir so the scan
// runs against a controlled directory tree without touching the real
// ~/Lethean/conf/models/.

package models_test

import (
	"os"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/models"
)

// osSymlink is a thin wrapper over os.Symlink — no core helper
// exists yet (Cerberus M3 follow-on is to add core.Symlink upstream).
// Test-only usage matches the existing pattern in trust_test.go.
func osSymlink(target, link string) error { return os.Symlink(target, link) }

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

// — Import tests — covers the happy path plus every reason the import
// can refuse: missing source, wrong extension, source-is-directory,
// destination-collision. Each test plants its own source fixture so
// the assertion is exact about which bytes wound up where.

func TestModels_Import_Good(t *core.T) {
	dir := modelsFixture(t)
	src := core.PathJoin(t.TempDir(), "gemma-4-e2b-q4_k_m.gguf")
	payload := []byte("GGUF\x00\x00\x00\x03payload-bytes")
	core.AssertTrue(t, core.WriteFile(src, payload, 0o644).OK)

	r := models.Import(src)
	core.AssertTrue(t, r.OK, "Import of a valid .gguf must succeed")
	dstPath, _ := r.Value.(string)
	core.AssertEqual(t, core.PathJoin(dir, "gemma-4-e2b-q4_k_m.gguf"), dstPath)

	read := core.ReadFile(dstPath)
	core.AssertTrue(t, read.OK)
	got, _ := read.Value.([]byte)
	core.AssertEqual(t, string(payload), string(got))

	// List() should see exactly the imported file post-import.
	lst := models.List()
	core.AssertTrue(t, lst.OK)
	entries := lst.Value.([]models.Entry)
	core.AssertLen(t, entries, 1)
	core.AssertEqual(t, "gemma-4-e2b-q4_k_m.gguf", entries[0].Name)
	core.AssertEqual(t, int64(len(payload)), entries[0].Size)
}

func TestModels_Import_Bad_NonGguf(t *core.T) {
	modelsFixture(t)
	src := core.PathJoin(t.TempDir(), "weights.bin")
	core.AssertTrue(t, core.WriteFile(src, []byte("x"), 0o644).OK)

	r := models.Import(src)
	core.AssertFalse(t, r.OK, "Import must refuse non-.gguf extensions")
}

func TestModels_Import_Bad_MissingSource(t *core.T) {
	modelsFixture(t)
	src := core.PathJoin(t.TempDir(), "ghost.gguf")
	// Don't create src — Stat must fail.

	r := models.Import(src)
	core.AssertFalse(t, r.OK, "Import must Fail when source doesn't exist")
}

func TestModels_Import_Bad_SourceIsDirectory(t *core.T) {
	modelsFixture(t)
	src := core.PathJoin(t.TempDir(), "model.gguf")
	core.AssertTrue(t, core.Mkdir(src, 0o755).OK)

	r := models.Import(src)
	core.AssertFalse(t, r.OK, "Import must refuse a directory masquerading as a .gguf")
}

func TestModels_Import_Bad_DestinationExists(t *core.T) {
	dir := modelsFixture(t)
	src := core.PathJoin(t.TempDir(), "qwen-3-7b.gguf")
	core.AssertTrue(t, core.WriteFile(src, []byte("payload"), 0o644).OK)
	// Plant a colliding destination so Import sees an existing file.
	core.AssertTrue(t, core.WriteFile(core.PathJoin(dir, "qwen-3-7b.gguf"), []byte("OLD"), 0o644).OK)

	r := models.Import(src)
	core.AssertFalse(t, r.OK, "Import must refuse when destination already exists")

	// Existing file must NOT be overwritten.
	read := core.ReadFile(core.PathJoin(dir, "qwen-3-7b.gguf"))
	core.AssertTrue(t, read.OK)
	got, _ := read.Value.([]byte)
	core.AssertEqual(t, "OLD", string(got))
}

func TestModels_Import_Bad_EmptyPath(t *core.T) {
	modelsFixture(t)
	r := models.Import("")
	core.AssertFalse(t, r.OK, "Import must refuse an empty source path")
}

// — Delete tests

func TestModels_Delete_Good(t *core.T) {
	dir := modelsFixture(t)
	target := core.PathJoin(dir, "phi-4-14b.gguf")
	core.AssertTrue(t, core.WriteFile(target, []byte("payload"), 0o644).OK)
	keep := core.PathJoin(dir, "qwen-3-7b.gguf")
	core.AssertTrue(t, core.WriteFile(keep, []byte("keep"), 0o644).OK)

	r := models.Delete("phi-4-14b.gguf")
	core.AssertTrue(t, r.OK, "Delete must succeed for an existing file")

	// File should be gone, sibling untouched.
	core.AssertFalse(t, core.Stat(target).OK)
	core.AssertTrue(t, core.Stat(keep).OK)

	// List confirms.
	listR := models.List()
	core.AssertTrue(t, listR.OK)
	entries := listR.Value.([]models.Entry)
	core.AssertLen(t, entries, 1)
	core.AssertEqual(t, "qwen-3-7b.gguf", entries[0].Name)
}

func TestModels_Delete_Bad_NotFound(t *core.T) {
	modelsFixture(t)
	r := models.Delete("ghost.gguf")
	core.AssertFalse(t, r.OK, "Delete must Fail when target doesn't exist")
}

func TestModels_Delete_Bad_EmptyName(t *core.T) {
	modelsFixture(t)
	r := models.Delete("")
	core.AssertFalse(t, r.OK)
}

func TestModels_Delete_Bad_PathTraversal_Dotdot(t *core.T) {
	dir := modelsFixture(t)
	// Plant a victim file in the PARENT of the models dir so the
	// traversal attempt has a real target to allegedly delete.
	parent := core.PathDir(dir)
	victim := core.PathJoin(parent, "victim.gguf")
	core.AssertTrue(t, core.WriteFile(victim, []byte("must survive"), 0o644).OK)
	defer core.Remove(victim)

	r := models.Delete("../victim.gguf")
	core.AssertFalse(t, r.OK, "Delete must refuse a ../ name")
	// Victim must still exist.
	core.AssertTrue(t, core.Stat(victim).OK, "../-prefixed name MUST NOT touch the parent file")
}

func TestModels_Delete_Bad_PathTraversal_Slashes(t *core.T) {
	modelsFixture(t)
	r := models.Delete("subdir/inner.gguf")
	core.AssertFalse(t, r.OK, "Delete must refuse a name containing slashes")
}

func TestModels_Delete_Bad_NonGguf(t *core.T) {
	dir := modelsFixture(t)
	weights := core.PathJoin(dir, "weights.bin")
	core.AssertTrue(t, core.WriteFile(weights, []byte("not a gguf"), 0o644).OK)

	r := models.Delete("weights.bin")
	core.AssertFalse(t, r.OK, "Delete must refuse non-.gguf files for now")
	// File must still be on disk.
	core.AssertTrue(t, core.Stat(weights).OK)
}

func TestModels_Delete_Bad_Directory(t *core.T) {
	dir := modelsFixture(t)
	// A snapshot-style directory named ".gguf" extension — Delete
	// must reject it (only regular files are deletable today).
	snapshot := core.PathJoin(dir, "snapshot.gguf")
	core.AssertTrue(t, core.Mkdir(snapshot, 0o755).OK)

	r := models.Delete("snapshot.gguf")
	core.AssertFalse(t, r.OK, "Delete must refuse a directory")
	// Directory must still exist.
	statR := core.Stat(snapshot)
	core.AssertTrue(t, statR.OK)
}

// — Rename tests — same shape as Delete (path-traversal + ext +
// no-clobber). Each test validates the rename refused to touch
// anything when it should refuse, AND that the bytes survive
// untouched.

func TestModels_Rename_Good(t *core.T) {
	dir := modelsFixture(t)
	src := core.PathJoin(dir, "gemma-2b.gguf")
	payload := []byte("model-bytes")
	core.AssertTrue(t, core.WriteFile(src, payload, 0o644).OK)

	r := models.Rename("gemma-2b.gguf", "gemma-2b-prod.gguf")
	core.AssertTrue(t, r.OK, "Rename must succeed for a normal in-place rename")
	dst, _ := r.Value.(string)
	core.AssertEqual(t, core.PathJoin(dir, "gemma-2b-prod.gguf"), dst)

	// Old name gone, new name present, content preserved.
	core.AssertFalse(t, core.Stat(src).OK, "source must be removed")
	core.AssertTrue(t, core.Stat(dst).OK, "destination must exist")
	read := core.ReadFile(dst)
	core.AssertTrue(t, read.OK)
	got, _ := read.Value.([]byte)
	core.AssertEqual(t, string(payload), string(got))
}

func TestModels_Rename_Bad_EmptyName(t *core.T) {
	modelsFixture(t)
	core.AssertFalse(t, models.Rename("", "x.gguf").OK)
	core.AssertFalse(t, models.Rename("x.gguf", "").OK)
}

func TestModels_Rename_Bad_SameName(t *core.T) {
	dir := modelsFixture(t)
	src := core.PathJoin(dir, "same.gguf")
	core.AssertTrue(t, core.WriteFile(src, []byte("bytes"), 0o644).OK)

	r := models.Rename("same.gguf", "same.gguf")
	core.AssertFalse(t, r.OK, "same-name rename must fail rather than no-op")
}

func TestModels_Rename_Bad_SourceMissing(t *core.T) {
	modelsFixture(t)
	r := models.Rename("ghost.gguf", "renamed.gguf")
	core.AssertFalse(t, r.OK, "source not present → fail")
}

func TestModels_Rename_Bad_DestinationExists(t *core.T) {
	dir := modelsFixture(t)
	src := core.PathJoin(dir, "src.gguf")
	dst := core.PathJoin(dir, "dst.gguf")
	core.AssertTrue(t, core.WriteFile(src, []byte("source"), 0o644).OK)
	core.AssertTrue(t, core.WriteFile(dst, []byte("destination"), 0o644).OK)

	r := models.Rename("src.gguf", "dst.gguf")
	core.AssertFalse(t, r.OK, "destination already exists → refuse to clobber")
	// Both files survive.
	core.AssertTrue(t, core.Stat(src).OK)
	core.AssertTrue(t, core.Stat(dst).OK)
}

func TestModels_Rename_Bad_NonGguf(t *core.T) {
	dir := modelsFixture(t)
	src := core.PathJoin(dir, "data.bin")
	core.AssertTrue(t, core.WriteFile(src, []byte("bytes"), 0o644).OK)

	// Wrong source extension.
	core.AssertFalse(t, models.Rename("data.bin", "renamed.gguf").OK)
	// Wrong destination extension.
	src2 := core.PathJoin(dir, "real.gguf")
	core.AssertTrue(t, core.WriteFile(src2, []byte("real"), 0o644).OK)
	core.AssertFalse(t, models.Rename("real.gguf", "renamed.bin").OK)
}

func TestModels_Rename_Bad_PathTraversal_Slashes(t *core.T) {
	dir := modelsFixture(t)
	src := core.PathJoin(dir, "a.gguf")
	core.AssertTrue(t, core.WriteFile(src, []byte("a"), 0o644).OK)

	core.AssertFalse(t, models.Rename("a.gguf", "sub/b.gguf").OK)
	core.AssertFalse(t, models.Rename("sub/a.gguf", "b.gguf").OK)
}

func TestModels_Rename_Bad_PathTraversal_Dotdot(t *core.T) {
	dir := modelsFixture(t)
	src := core.PathJoin(dir, "stay.gguf")
	core.AssertTrue(t, core.WriteFile(src, []byte("stay"), 0o644).OK)

	parent := core.PathDir(dir)
	victim := core.PathJoin(parent, "victim.gguf")
	core.AssertTrue(t, core.WriteFile(victim, []byte("must survive"), 0o644).OK)
	defer core.Remove(victim)

	r := models.Rename("stay.gguf", "../victim.gguf")
	core.AssertFalse(t, r.OK, "../ destination must be refused")
	// Source untouched, victim untouched.
	core.AssertTrue(t, core.Stat(src).OK)
	core.AssertTrue(t, core.Stat(victim).OK)
}

func TestModels_Rename_Bad_Directory(t *core.T) {
	dir := modelsFixture(t)
	snapshot := core.PathJoin(dir, "snapshot.gguf")
	core.AssertTrue(t, core.Mkdir(snapshot, 0o755).OK)

	r := models.Rename("snapshot.gguf", "renamed.gguf")
	core.AssertFalse(t, r.OK, "Rename must refuse a directory source")
	statR := core.Stat(snapshot)
	core.AssertTrue(t, statR.OK)
}

// TestModels_Rename_Bad_SymlinkSource — Cerberus M3 / Mantis #1419.
// A symlink in the models dir pointing at a sensitive file outside
// the dir would, without Lstat, pass the "regular file" check (Stat
// follows symlinks). With the Lstat fix, the rename is refused.
//
// Uses os.Symlink directly — there's no core.Symlink helper, and
// test-only os usage matches the existing pattern in
// pkg/downloader/trust_test.go.
func TestModels_Rename_Bad_SymlinkSource(t *core.T) {
	dir := modelsFixture(t)
	// Victim target outside the models dir.
	parent := core.PathDir(dir)
	victim := core.PathJoin(parent, "victim.gguf")
	core.AssertTrue(t, core.WriteFile(victim, []byte("must not be touched"), 0o644).OK)
	defer core.Remove(victim)

	// Symlink inside the models dir pointing at the victim.
	link := core.PathJoin(dir, "link.gguf")
	if err := osSymlink(victim, link); err != nil {
		t.Fatalf("setup: symlink failed: %v", err)
	}

	// Rename must refuse the symlink source — never follow.
	r := models.Rename("link.gguf", "renamed.gguf")
	core.AssertFalse(t, r.OK, "Rename must refuse a symlink source (Cerberus M3)")

	// Victim file must survive intact.
	read := core.ReadFile(victim)
	core.AssertTrue(t, read.OK)
	got, _ := read.Value.([]byte)
	core.AssertEqual(t, "must not be touched", string(got))
}

// TestModels_Rename_Ugly_AtomicNoClobber — Mantis #1419 atomic-rename
// half. N goroutines race to rename N distinct sources to the SAME
// destination. With the link(2)-based atomicRename, exactly one
// winner succeeds; the other N-1 must fail without clobbering
// the winner's bytes. With the prior Stat(dst)+Rename pair this
// would be a race window.
func TestModels_Rename_Ugly_AtomicNoClobber(t *core.T) {
	dir := modelsFixture(t)
	// Plant N distinct sources with distinguishable contents.
	const N = 8
	for i := 0; i < N; i++ {
		name := "src-" + core.Sprintf("%d", i) + ".gguf"
		body := []byte("payload-" + core.Sprintf("%d", i))
		core.AssertTrue(t, core.WriteFile(core.PathJoin(dir, name), body, 0o644).OK)
	}

	// Race: every goroutine targets the same destination basename.
	const dstName = "winner.gguf"
	results := make(chan core.Result, N)
	start := make(chan struct{})
	for i := 0; i < N; i++ {
		go func(i int) {
			<-start
			name := "src-" + core.Sprintf("%d", i) + ".gguf"
			results <- models.Rename(name, dstName)
		}(i)
	}
	close(start)

	wins, losses := 0, 0
	for i := 0; i < N; i++ {
		r := <-results
		if r.OK {
			wins++
		} else {
			losses++
		}
	}
	core.AssertEqual(t, 1, wins, "exactly one goroutine must win the race")
	core.AssertEqual(t, N-1, losses, "all other goroutines must fail without clobbering")

	// Destination exists and is one of the source payloads.
	dst := core.PathJoin(dir, dstName)
	read := core.ReadFile(dst)
	core.AssertTrue(t, read.OK)
	got := string(read.Value.([]byte))
	core.AssertTrue(t, len(got) > 0, "winner's bytes must be present at dst")
}

// TestModels_Rename_Bad_AtomicEexistKeepsSource — Mantis #1419. When
// dst already exists, atomicRename must refuse via link(2) EEXIST
// AND leave the source intact (no half-completed rename). Distinct
// from TestModels_Rename_Bad_DestinationExists which only checks
// the refuse path; this asserts the bytes don't move.
func TestModels_Rename_Bad_AtomicEexistKeepsSource(t *core.T) {
	dir := modelsFixture(t)
	src := core.PathJoin(dir, "src.gguf")
	dst := core.PathJoin(dir, "dst.gguf")
	core.AssertTrue(t, core.WriteFile(src, []byte("src-bytes"), 0o644).OK)
	core.AssertTrue(t, core.WriteFile(dst, []byte("dst-bytes"), 0o644).OK)

	r := models.Rename("src.gguf", "dst.gguf")
	core.AssertFalse(t, r.OK)

	// Source still readable with original bytes.
	srcRead := core.ReadFile(src)
	core.AssertTrue(t, srcRead.OK)
	core.AssertEqual(t, "src-bytes", string(srcRead.Value.([]byte)))

	// Destination still has the pre-existing bytes — no clobber.
	dstRead := core.ReadFile(dst)
	core.AssertTrue(t, dstRead.OK)
	core.AssertEqual(t, "dst-bytes", string(dstRead.Value.([]byte)))
}
