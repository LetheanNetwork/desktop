// SPDX-Licence-Identifier: EUPL-1.2

// sweep_test.go — Mantis #1594 acceptance: SweepTmpOrphans covers
// the age-threshold gate (both sides), the orphan-pattern matcher
// (positive + negative shapes), the under-Lethean-root safety
// boundary, and recursive traversal across nested subdirectories.

package paths_test

import (
	"os"
	"testing"
	"time"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/paths"
)

// validTmpSuffix returns a deterministic 16-hex tmp suffix so each
// test can construct an orphan whose name matches the production
// AtomicWriteWithVersion staging shape (`<basename>.tmp.<16-hex>`)
// without relying on core.RandomString.
const validTmpSuffix = "deadbeefcafef00d"

// writeOrphan creates a `<base>.tmp.<hex>` file under dir with the
// requested age (computed back from time.Now). Returns the file path.
func writeOrphan(t *testing.T, dir, base string, age time.Duration) string {
	t.Helper()
	name := base + ".tmp." + validTmpSuffix
	full := core.PathJoin(dir, name)
	if err := os.WriteFile(full, []byte("staging"), 0o600); err != nil {
		t.Fatalf("WriteFile %s: %s", full, err)
	}
	when := time.Now().Add(-age)
	if err := os.Chtimes(full, when, when); err != nil {
		t.Fatalf("Chtimes %s: %s", full, err)
	}
	return full
}

// pathExists is a tiny test-side helper — production code uses
// core.Stat / core.Lstat. Keeping it inline avoids dragging in a
// new fixture surface.
func pathExists(t *testing.T, p string) bool {
	t.Helper()
	_, err := os.Lstat(p)
	return err == nil
}

// homeFixtureT is the testing.T-typed twin of paths_test.homeFixture.
// homeFixture takes *core.T (which is an alias for *testing.T but the
// existing helper is written against *core.T). Re-using the same body
// here keeps sweep_test self-contained against the standard testing
// API, mirroring atomic_write_test.go's pattern of mixing the two
// forms when convenient.
func homeFixtureT(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	// Force Root() to materialise so subsequent SweepTmpOrphans calls
	// resolve the new ~/Lethean/ rather than a cached stale path.
	r := paths.Root()
	if !r.OK {
		t.Fatalf("Root: %s", r.Error())
	}
	return r.Value.(string)
}

func TestSweepTmpOrphans_AgesBelowThreshold_Skipped_Good(t *testing.T) {
	root := homeFixtureT(t)
	// 5 minutes < 1h threshold — must NOT be deleted.
	orphan := writeOrphan(t, root, "fresh.md", 5*time.Minute)

	r := paths.SweepTmpOrphans(root)
	if !r.OK {
		t.Fatalf("SweepTmpOrphans: %s", r.Error())
	}
	if got := r.Value.(int); got != 0 {
		t.Fatalf("count: got %d, want 0", got)
	}
	if !pathExists(t, orphan) {
		t.Fatalf("recent orphan was deleted; should be preserved: %s", orphan)
	}
}

func TestSweepTmpOrphans_AgesAboveThreshold_Deleted_Good(t *testing.T) {
	root := homeFixtureT(t)
	// 2h > 1h threshold — must be deleted.
	orphan := writeOrphan(t, root, "stale.md", 2*time.Hour)

	// Subscribe to LockEvents so the test can assert the audit emit
	// fired for the deletion. Cleared via the package's test-only
	// reset to avoid bleeding into sibling tests.
	defer paths.ClearLockEventSubscribersForTest()
	var sawSwept bool
	paths.SubscribeLockEvents(func(ev paths.LockEvent) {
		if ev.Kind == paths.EventOrphanTmpSwept {
			sawSwept = true
		}
	})

	r := paths.SweepTmpOrphans(root)
	if !r.OK {
		t.Fatalf("SweepTmpOrphans: %s", r.Error())
	}
	if got := r.Value.(int); got != 1 {
		t.Fatalf("count: got %d, want 1", got)
	}
	if pathExists(t, orphan) {
		t.Fatalf("stale orphan was preserved; should be deleted: %s", orphan)
	}
	// Audit emit requires an injected audit secret to actually fire
	// through the subscriber path (the F2 degraded-mode drop returns
	// Ok without emitting). Provide a fixed secret for the duration
	// of this assertion.
	// NOTE: emit happens INSIDE the sweep above — by the time we get
	// here, sawSwept reflects whether emission fired with the
	// previously-installed (no-op) secret. Re-run a second sweep with
	// a fresh orphan + injected secret to exercise the live emit
	// path.
	if sawSwept {
		// Sub-test environment already had a secret installed —
		// nothing more to verify.
		return
	}

	paths.SetAuditSecretProvider(func() []byte { return []byte("test-secret") })
	t.Cleanup(func() { paths.SetAuditSecretProvider(nil) })
	orphan2 := writeOrphan(t, root, "stale2.md", 2*time.Hour)
	r2 := paths.SweepTmpOrphans(root)
	if !r2.OK {
		t.Fatalf("SweepTmpOrphans (with secret): %s", r2.Error())
	}
	if got := r2.Value.(int); got != 1 {
		t.Fatalf("count (with secret): got %d, want 1", got)
	}
	if pathExists(t, orphan2) {
		t.Fatalf("stale orphan2 was preserved: %s", orphan2)
	}
	if !sawSwept {
		t.Fatalf("EventOrphanTmpSwept never observed by subscriber")
	}
}

func TestSweepTmpOrphans_NonTmpPattern_Ignored_Good(t *testing.T) {
	root := homeFixtureT(t)
	// Files that look orphan-adjacent but should be preserved:
	//   - plain regular file
	//   - `.tmp` with no hex suffix
	//   - `.tmp.draft` (suffix is not hex)
	//   - `.tmp.<short-hex>` (suffix wrong length)
	//   - `.tmp.<long-hex>` (suffix wrong length)
	// All set to 2h old so the age gate would delete them IF the
	// pattern matched.
	old := time.Now().Add(-2 * time.Hour)
	mustWrite := func(name string) string {
		full := core.PathJoin(root, name)
		if err := os.WriteFile(full, []byte("x"), 0o600); err != nil {
			t.Fatalf("WriteFile %s: %s", full, err)
		}
		if err := os.Chtimes(full, old, old); err != nil {
			t.Fatalf("Chtimes %s: %s", full, err)
		}
		return full
	}
	preserved := []string{
		mustWrite("notes.md"),
		mustWrite("notes.tmp"),
		mustWrite("notes.tmp.draft"),
		mustWrite("notes.tmp.abcd1234"),                // 8 hex — wrong length
		mustWrite("notes.tmp.deadbeefcafef00d00"),      // 20 hex — wrong length
		mustWrite("notes.tmp.deadbeefcafef00g"),        // 16 chars but 'g' is non-hex
	}

	r := paths.SweepTmpOrphans(root)
	if !r.OK {
		t.Fatalf("SweepTmpOrphans: %s", r.Error())
	}
	if got := r.Value.(int); got != 0 {
		t.Fatalf("count: got %d, want 0 (no orphans should match)", got)
	}
	for _, p := range preserved {
		if !pathExists(t, p) {
			t.Fatalf("non-orphan file was deleted: %s", p)
		}
	}
}

func TestSweepTmpOrphans_RootOutsideLethean_Refused_Bad(t *testing.T) {
	// Rebind HOME so paths.Root() points at a known temp tree.
	_ = homeFixtureT(t)
	// /tmp is NOT under ~/Lethean/ — sweep MUST refuse.
	r := paths.SweepTmpOrphans("/tmp/some-unrelated-dir")
	if r.OK {
		t.Fatalf("SweepTmpOrphans should have refused /tmp root; got Ok(%v)", r.Value)
	}
	if !core.Contains(r.Error(), paths.CodeSweepUnsafeRoot) {
		t.Fatalf("expected error containing %s, got: %s",
			paths.CodeSweepUnsafeRoot, r.Error())
	}
}

func TestSweepTmpOrphans_EmptyRoot_Refused_Bad(t *testing.T) {
	_ = homeFixtureT(t)
	r := paths.SweepTmpOrphans("")
	if r.OK {
		t.Fatalf("SweepTmpOrphans('') should have failed")
	}
	if !core.Contains(r.Error(), paths.CodeSweepUnsafeRoot) {
		t.Fatalf("expected error containing %s, got: %s",
			paths.CodeSweepUnsafeRoot, r.Error())
	}
}

func TestSweepTmpOrphans_Recursive_Good(t *testing.T) {
	root := homeFixtureT(t)
	// Build a 3-level nested fixture with one orphan at each level.
	nested := []string{
		root,
		core.PathJoin(root, "wallets"),
		core.PathJoin(root, "account", "abc"),
		core.PathJoin(root, "office", "mail", "inbox"),
	}
	for _, d := range nested {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("MkdirAll %s: %s", d, err)
		}
	}
	var orphans []string
	for i, d := range nested {
		// Vary basenames so each path is distinct.
		orphans = append(orphans,
			writeOrphan(t, d, core.Sprintf("file%d.enc", i), 3*time.Hour))
	}

	r := paths.SweepTmpOrphans(root)
	if !r.OK {
		t.Fatalf("SweepTmpOrphans: %s", r.Error())
	}
	if got, want := r.Value.(int), len(orphans); got != want {
		t.Fatalf("count: got %d, want %d", got, want)
	}
	for _, p := range orphans {
		if pathExists(t, p) {
			t.Fatalf("nested orphan was preserved; should be deleted: %s", p)
		}
	}
}

func TestSweepTmpOrphans_MixedAges_OnlyOldDeleted_Ugly(t *testing.T) {
	root := homeFixtureT(t)
	// Mixed ages in a single tree — only those older than the
	// threshold should disappear.
	fresh := writeOrphan(t, root, "fresh.md", 5*time.Minute)
	old := writeOrphan(t, root, "old.md", 90*time.Minute)
	veryOld := writeOrphan(t, root, "veryold.md", 24*time.Hour)

	r := paths.SweepTmpOrphans(root)
	if !r.OK {
		t.Fatalf("SweepTmpOrphans: %s", r.Error())
	}
	if got := r.Value.(int); got != 2 {
		t.Fatalf("count: got %d, want 2", got)
	}
	if !pathExists(t, fresh) {
		t.Fatalf("fresh orphan was deleted: %s", fresh)
	}
	if pathExists(t, old) {
		t.Fatalf("old orphan was preserved: %s", old)
	}
	if pathExists(t, veryOld) {
		t.Fatalf("veryOld orphan was preserved: %s", veryOld)
	}
}

func TestSweepTmpOrphans_ThresholdBoundary_Good(t *testing.T) {
	root := homeFixtureT(t)
	// Just-under-threshold and just-over-threshold edge cases.
	// Subtract a small fudge to avoid the test racing the clock as
	// SweepTmpOrphans recomputes core.Since(modTime) on each entry.
	justUnder := writeOrphan(t, root, "justunder.md",
		paths.OrphanTmpAgeThreshold-5*time.Second)
	justOver := writeOrphan(t, root, "justover.md",
		paths.OrphanTmpAgeThreshold+5*time.Second)

	r := paths.SweepTmpOrphans(root)
	if !r.OK {
		t.Fatalf("SweepTmpOrphans: %s", r.Error())
	}
	if got := r.Value.(int); got != 1 {
		t.Fatalf("count: got %d, want 1", got)
	}
	if !pathExists(t, justUnder) {
		t.Fatalf("justUnder orphan deleted; should be preserved")
	}
	if pathExists(t, justOver) {
		t.Fatalf("justOver orphan preserved; should be deleted")
	}
}
