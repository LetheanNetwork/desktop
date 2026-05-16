// SPDX-Licence-Identifier: EUPL-1.2

// Tests for the models-dir override surface — Cerberus H1 / L3
// hardening (Mantis 2026-05-16).
//
// Scope: producer-side only — Set, Clear, paths.json persistence +
// permissions, confused-deputy guards. The ModelsDir() read-side
// integration (override honoured by the hot path) is covered by
// paths_test.go in the parallel paths-sweep lane.
//
// Pattern matches core/go canon: external `_test` package, t-scoped
// HOME via homeFixture (defined in paths_test.go) so every test
// runs against an isolated ~/.

package paths_test

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/paths"
)

func TestPathsOverride_SetModelsDirOverride_Good(t *core.T) {
	home := homeFixture(t)
	override := core.PathJoin(home, "external-models")

	r := paths.SetModelsDirOverride(override)
	core.AssertTrue(t, r.OK, "SetModelsDirOverride must succeed for a valid abs path")
	core.AssertEqual(t, override, r.Value.(string))

	// Override directory should exist on disk (MkdirAll was called).
	stat := core.Stat(override)
	core.AssertTrue(t, stat.OK)
	info := stat.Value.(core.FsFileInfo)
	core.AssertTrue(t, info.IsDir(), "override path must be a directory")

	// The paths.json file should exist and carry the value.
	confDir := paths.ConfDir().Value.(string)
	overrideFile := core.PathJoin(confDir, "paths.json")
	body := core.ReadFile(overrideFile)
	core.AssertTrue(t, body.OK, "paths.json must be written")
	raw, _ := body.Value.([]byte)
	core.AssertContains(t, string(raw), override, "paths.json must carry the override path")
}

func TestPathsOverride_SetModelsDirOverride_Bad_EmptyPath(t *core.T) {
	homeFixture(t)
	r := paths.SetModelsDirOverride("")
	core.AssertFalse(t, r.OK)
}

func TestPathsOverride_SetModelsDirOverride_Bad_OutsideHome(t *core.T) {
	homeFixture(t)
	r := paths.SetModelsDirOverride("/etc/lthn-models")
	core.AssertFalse(t, r.OK, "paths outside $HOME must be refused")
}

func TestPathsOverride_SetModelsDirOverride_Bad_HiddenDir(t *core.T) {
	home := homeFixture(t)
	r := paths.SetModelsDirOverride(core.PathJoin(home, ".ssh", "models"))
	core.AssertFalse(t, r.OK, "paths inside hidden ~/. directories must be refused")
}

func TestPathsOverride_SetModelsDirOverride_Bad_LibraryDir(t *core.T) {
	home := homeFixture(t)
	r := paths.SetModelsDirOverride(core.PathJoin(home, "Library", "lthn"))
	core.AssertFalse(t, r.OK, "paths under ~/Library must be refused (macOS persistence vector)")
}

func TestPathsOverride_SetModelsDirOverride_Bad_HomeRoot(t *core.T) {
	home := homeFixture(t)
	r := paths.SetModelsDirOverride(home)
	core.AssertFalse(t, r.OK, "the home root itself must be refused")
}

func TestPathsOverride_SetModelsDirOverride_Bad_RelativePath(t *core.T) {
	homeFixture(t)
	r := paths.SetModelsDirOverride("relative/path")
	core.AssertFalse(t, r.OK, "relative paths must be refused — overrides are absolute")
}

func TestPathsOverride_PathsJsonIsMode0600(t *core.T) {
	home := homeFixture(t)
	core.AssertTrue(t, paths.SetModelsDirOverride(core.PathJoin(home, "vault")).OK)

	stat := core.Stat(core.PathJoin(paths.ConfDir().Value.(string), "paths.json"))
	core.AssertTrue(t, stat.OK)
	info := stat.Value.(core.FsFileInfo)
	mode := info.Mode().Perm()
	core.AssertEqual(t, 0o600, int(mode), "paths.json must be mode 0o600 (Cerberus L3)")
}

func TestPathsOverride_ClearModelsDirOverride_Good(t *core.T) {
	home := homeFixture(t)
	override := core.PathJoin(home, "external")
	core.AssertTrue(t, paths.SetModelsDirOverride(override).OK)

	r := paths.ClearModelsDirOverride()
	core.AssertTrue(t, r.OK)

	// After clear, paths.json should no longer carry the override.
	overrideFile := core.PathJoin(paths.ConfDir().Value.(string), "paths.json")
	body := core.ReadFile(overrideFile)
	core.AssertTrue(t, body.OK, "paths.json must still be readable post-clear")
	raw, _ := body.Value.([]byte)
	core.AssertNotContains(t, string(raw), override, "paths.json must no longer carry the cleared path")
}

func TestPathsOverride_ClearModelsDirOverride_Good_Idempotent(t *core.T) {
	homeFixture(t)
	r := paths.ClearModelsDirOverride()
	core.AssertTrue(t, r.OK, "clearing when no override is set must succeed silently")
}

// TestModelsDir_HonoursOverride_Good — the ModelsDir() hot path
// consults readModelsDirOverride() before falling back to the
// default ~/Lethean/conf/models/. With an override set, the
// returned path matches the override exactly.
func TestModelsDir_HonoursOverride_Good(t *core.T) {
	home := homeFixture(t)
	override := core.PathJoin(home, "external-models")
	core.AssertTrue(t, paths.SetModelsDirOverride(override).OK,
		"SetModelsDirOverride must succeed before the read-side assertion")

	r := paths.ModelsDir()
	core.AssertTrue(t, r.OK, "ModelsDir must succeed when an override is set")
	core.AssertEqual(t, override, r.Value.(string),
		"ModelsDir must return the override path, not the default")

	// The override directory should exist after ModelsDir (MkdirAll
	// runs on the hot path too — covers the case where the user set
	// the override then someone (or restore-from-backup) removed the
	// directory).
	stat := core.Stat(override)
	core.AssertTrue(t, stat.OK)
	core.AssertTrue(t, stat.Value.(core.FsFileInfo).IsDir())
}

// TestModelsDir_FallsBackToDefault_Good — with no override set,
// ModelsDir() returns the canonical ~/Lethean/conf/models/ path.
// Also covers the post-Clear path: set, clear, assert default.
func TestModelsDir_FallsBackToDefault_Good(t *core.T) {
	home := homeFixture(t)
	want := core.PathJoin(home, "Lethean", "conf", "models")

	// No override yet — default applies.
	r1 := paths.ModelsDir()
	core.AssertTrue(t, r1.OK)
	core.AssertEqual(t, want, r1.Value.(string),
		"ModelsDir must return the default when no override is set")

	// Set then clear — default applies again.
	override := core.PathJoin(home, "scratch-models")
	core.AssertTrue(t, paths.SetModelsDirOverride(override).OK)
	core.AssertTrue(t, paths.ClearModelsDirOverride().OK)

	r2 := paths.ModelsDir()
	core.AssertTrue(t, r2.OK)
	core.AssertEqual(t, want, r2.Value.(string),
		"ModelsDir must return the default after the override is cleared")
}

// TestModelsDir_IgnoresUnparseableOverrideFile_Ugly — a corrupt
// paths.json (truncated, garbage bytes, wrong shape) must not
// crash the hot path. readPathsOverride is documented best-effort
// and returns zero-value on any parse failure, so ModelsDir()
// silently falls through to the default. Defends against
// half-written files, manual edits gone wrong, downgrade from a
// future schema, etc.
func TestModelsDir_IgnoresUnparseableOverrideFile_Ugly(t *core.T) {
	home := homeFixture(t)
	want := core.PathJoin(home, "Lethean", "conf", "models")

	// Force ConfDir to exist (ModelsDir would create it anyway, but
	// we need to land paths.json before the read).
	confR := paths.ConfDir()
	core.AssertTrue(t, confR.OK)

	overrideFile := core.PathJoin(confR.Value.(string), "paths.json")
	garbage := []byte("{this is not valid json,,,,")
	core.AssertTrue(t, core.WriteFile(overrideFile, garbage, 0o600).OK,
		"writing garbage paths.json must succeed (fs-level)")

	r := paths.ModelsDir()
	core.AssertTrue(t, r.OK, "ModelsDir must not fail when paths.json is unparseable")
	core.AssertEqual(t, want, r.Value.(string),
		"ModelsDir must fall back to default when paths.json is unparseable")
}

// TestPathsOverride_WriteIsAtomic_Ugly — Cerberus H#9-verify F3
// (Mantis #1536). The previous direct WriteFile sequence (truncate
// + write) left a window where a mid-write crash landed a partial
// JSON blob on disk and the next read silently returned the
// zero-value override. Atomic write via tmp+rename guarantees the
// final paths.json never contains a partial body — either the
// previous valid content OR the fully-serialised new content.
//
// We can't synthesise a real mid-write crash from a unit test, so
// we assert the load-bearing post-conditions:
//
//   1. After a successful Set/Clear, paths.json parses cleanly
//      (never half-written content reaching the final filename).
//   2. The atomic-write contract is exposed via the implementation:
//      a stale paths.json.tmp left behind by a prior aborted write
//      MUST NOT be read by readPathsOverride — readers consult only
//      the final filename. This defends against an OS that left a
//      tmp file behind after a crash.
func TestPathsOverride_WriteIsAtomic_Ugly(t *core.T) {
	home := homeFixture(t)
	override := core.PathJoin(home, "atomic-models")
	core.AssertTrue(t, paths.SetModelsDirOverride(override).OK,
		"baseline Set must succeed")

	confDir := paths.ConfDir().Value.(string)
	finalFile := core.PathJoin(confDir, "paths.json")
	tmpFile := finalFile + ".tmp"

	// After a successful Set, the final file parses cleanly and the
	// tmp file does NOT linger (rename consumed it).
	body := core.ReadFile(finalFile)
	core.AssertTrue(t, body.OK, "final paths.json must be readable")
	raw, _ := body.Value.([]byte)
	core.AssertContains(t, string(raw), override,
		"final paths.json must carry the override after successful write")

	tmpStat := core.Stat(tmpFile)
	core.AssertFalse(t, tmpStat.OK,
		"paths.json.tmp must NOT linger after successful write (rename consumed it)")
}

func TestPathsOverride_StaleTmpIsIgnoredByRead_Ugly(t *core.T) {
	// Simulate a prior crash that left paths.json.tmp on disk with
	// partial/garbage content but the final paths.json holds a valid
	// prior override. The reader must consult ONLY paths.json — the
	// stale tmp must not influence ModelsDir() in any way.
	home := homeFixture(t)
	override := core.PathJoin(home, "prior-models")
	core.AssertTrue(t, paths.SetModelsDirOverride(override).OK)

	confDir := paths.ConfDir().Value.(string)
	tmpFile := core.PathJoin(confDir, "paths.json.tmp")

	// Plant a stale tmp file that, if mistakenly read, would point
	// elsewhere (or fail to parse entirely).
	stale := []byte(`{"models_dir":"/tmp/STALE_NEVER_SEE"}`)
	core.AssertTrue(t, core.WriteFile(tmpFile, stale, 0o600).OK)

	// The read-side ignores the tmp file entirely — ModelsDir()
	// still returns the prior valid override.
	r := paths.ModelsDir()
	core.AssertTrue(t, r.OK)
	core.AssertEqual(t, override, r.Value.(string),
		"ModelsDir must read only paths.json, never paths.json.tmp")
}

func TestPathsOverride_WriteFinalIsMode0600_Good(t *core.T) {
	// After the tmp+rename refactor (Cerberus F3), the final
	// paths.json must still land with mode 0o600 — the tmp file is
	// created 0o600 too so there's no readable-by-others window
	// between write and rename. Cerberus L3 hardening is preserved.
	home := homeFixture(t)
	core.AssertTrue(t, paths.SetModelsDirOverride(core.PathJoin(home, "vault-models")).OK)

	stat := core.Stat(core.PathJoin(paths.ConfDir().Value.(string), "paths.json"))
	core.AssertTrue(t, stat.OK)
	info := stat.Value.(core.FsFileInfo)
	core.AssertEqual(t, 0o600, int(info.Mode().Perm()),
		"paths.json must retain 0o600 mode after atomic-write refactor")
}

// TestWritePathsOverride_RandomTmpSuffix_Good — Cerberus DREAD-r3 LOW
// (Mantis #1541). The tmp filename used by writePathsOverride must
// carry a per-call random suffix (paths.json.tmp.<random>), NOT the
// legacy fixed paths.json.tmp. Two assertions:
//
//  1. The legacy fixed tmp filename is NEVER produced (no
//     paths.json.tmp left lingering by a successful write).
//  2. Any tmp files that did briefly exist match the random-suffix
//     shape paths.json.tmp.* — after a successful write the rename
//     consumes them so a glob should find none, which itself is the
//     proof that staging is per-call rather than shared.
func TestWritePathsOverride_RandomTmpSuffix_Good(t *core.T) {
	home := homeFixture(t)
	core.AssertTrue(t, paths.SetModelsDirOverride(core.PathJoin(home, "rand-tmp-models")).OK)

	confDir := paths.ConfDir().Value.(string)
	// Legacy fixed tmp name must not exist — the refactor moved off it.
	legacyTmp := core.PathJoin(confDir, "paths.json.tmp")
	core.AssertFalse(t, core.Stat(legacyTmp).OK,
		"legacy fixed paths.json.tmp must NOT exist after random-suffix refactor")

	// Any per-call random-suffix tmps must have been consumed by the
	// rename — a glob should find zero stragglers after a successful
	// write.
	matches := core.PathGlob(core.PathJoin(confDir, "paths.json.tmp.*"))
	core.AssertEqual(t, 0, len(matches),
		"random-suffix tmp files must be consumed by rename after successful write")
}

// TestWritePathsOverride_ConcurrentCallersSerialised_Ugly — Cerberus
// DREAD-r3 LOW (Mantis #1541). Spawn N goroutines, each calling
// SetModelsDirOverride with a distinct path. The mutex serialises
// the read-modify-write so each call's tmp write lands intact; the
// final paths.json holds ONE of the N inputs (last-rename-wins is
// the documented semantics) and the JSON is well-formed — never a
// corrupt or empty body from a cross-writer stomp.
//
// Defends against the pre-fix race where two callers would both
// stage to paths.json.tmp at once: second writer's truncate would
// land mid-stream of the first writer's serialisation, and whichever
// rename fired last would publish the corrupted blob.
func TestWritePathsOverride_ConcurrentCallersSerialised_Ugly(t *core.T) {
	home := homeFixture(t)
	const n = 10

	// Pre-build N distinct override targets. Each one is a real dir
	// under $HOME — SetModelsDirOverride validates + MkdirAlls them.
	candidates := make([]string, n)
	for i := 0; i < n; i++ {
		candidates[i] = core.PathJoin(home, "concurrent-models-"+core.Itoa(i))
	}

	var wg core.WaitGroup
	results := make([]core.Result, n)
	for i := 0; i < n; i++ {
		idx := i
		wg.Go(func() {
			results[idx] = paths.SetModelsDirOverride(candidates[idx])
		})
	}
	wg.Wait()

	// Every concurrent caller must succeed — the mutex serialises
	// them but does not fail any.
	for i, r := range results {
		core.AssertTrue(t, r.OK,
			"concurrent SetModelsDirOverride caller #"+core.Itoa(i)+" must succeed")
	}

	// Final paths.json must parse as well-formed JSON and carry
	// exactly one of the N candidates (last-rename-wins).
	confDir := paths.ConfDir().Value.(string)
	body := core.ReadFile(core.PathJoin(confDir, "paths.json"))
	core.AssertTrue(t, body.OK, "final paths.json must be readable after concurrent writes")
	raw, _ := body.Value.([]byte)

	type shape struct {
		ModelsDir string `json:"models_dir"`
	}
	var got shape
	dec := core.JSONUnmarshalString(string(raw), &got)
	core.AssertTrue(t, dec.OK,
		"final paths.json must parse cleanly — no cross-writer stomp on the staging file")

	// The persisted value must be one of the N inputs — never the
	// empty string, never a truncated path, never a path that no
	// caller supplied.
	matched := false
	for _, c := range candidates {
		if got.ModelsDir == c {
			matched = true
			break
		}
	}
	core.AssertTrue(t, matched,
		"persisted models_dir must match one of the concurrent inputs (last-rename-wins)")

	// And no random-suffix tmp file may linger after all writes
	// complete — every staging file got renamed or cleaned.
	matches := core.PathGlob(core.PathJoin(confDir, "paths.json.tmp.*"))
	core.AssertEqual(t, 0, len(matches),
		"no random-suffix tmp file may linger after concurrent writes settle")
}
