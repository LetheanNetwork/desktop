// SPDX-Licence-Identifier: EUPL-1.2

// Internal-package tests for repos.go's unexported scan + status
// helpers (scanRoots, scanDefaultRoots, statuses) — hermetic TempDir
// fixtures only, real `git` subprocesses via dappco.re/go/process's
// package-level Run (same primitive dappco.re/go/scm/git uses).
package repos

import (
	core "dappco.re/go"
	"dappco.re/go/process"
)

// TestCollectSourcePaths_Ugly_NilService — a nil receiver is a safe
// no-op (mirrors RegisterSource's own nil-receiver guard, covered in
// sources_test.go).
func TestCollectSourcePaths_Ugly_NilService(t *core.T) {
	var s *Service
	core.AssertNil(t, s.collectSourcePaths(core.Background()))
}

// ─── scanRoots ───────────────────────────────────────────────────────

// TestScanRoots_Good — a root containing one subdirectory with a
// .git entry is returned.
func TestScanRoots_Good(t *core.T) {
	s := NewService(nil)
	root := t.TempDir()
	core.RequireTrue(t, core.MkdirAll(core.PathJoin(root, "hasgit", ".git"), 0o755).OK)

	got := s.scanRoots([]string{root})
	core.AssertLen(t, got, 1)
	core.AssertEqual(t, core.PathJoin(root, "hasgit"), got[0])
}

// TestScanRoots_Bad_NonexistentRoot — a root that doesn't exist at
// all fails ReadDir and is silently skipped (no panic, no entries).
func TestScanRoots_Bad_NonexistentRoot(t *core.T) {
	s := NewService(nil)
	got := s.scanRoots([]string{core.PathJoin(t.TempDir(), "does-not-exist")})
	core.AssertLen(t, got, 0)
}

// TestScanRoots_Ugly_MixedEntries — a plain file (not a directory)
// and a directory without a .git entry are both skipped.
func TestScanRoots_Ugly_MixedEntries(t *core.T) {
	s := NewService(nil)
	root := t.TempDir()
	core.RequireTrue(t, core.WriteFile(core.PathJoin(root, "afile.txt"), []byte("x"), 0o644).OK)
	core.RequireTrue(t, core.MkdirAll(core.PathJoin(root, "nogit"), 0o755).OK)

	got := s.scanRoots([]string{root})
	core.AssertLen(t, got, 0)
}

// ─── scanDefaultRoots ────────────────────────────────────────────────

// TestScanDefaultRoots_Good — a real (TempDir, isolated) $HOME
// resolves successfully; the canonical Code/* roots simply don't
// exist under it, so the scan comes back empty without error.
func TestScanDefaultRoots_Good(t *core.T) {
	s := NewService(nil)
	t.Setenv("HOME", t.TempDir())
	got := s.scanDefaultRoots()
	core.AssertLen(t, got, 0)
}

// TestScanDefaultRoots_Bad_NoHome — os.UserHomeDir errors when $HOME
// is unset/empty on unix, taking the early nil return.
func TestScanDefaultRoots_Bad_NoHome(t *core.T) {
	s := NewService(nil)
	t.Setenv("HOME", "")
	got := s.scanDefaultRoots()
	core.AssertNil(t, got)
}

// ─── statuses ────────────────────────────────────────────────────────

// TestStatuses_Good — a real, freshly-initialised repo probes
// cleanly: no Err on the returned Status.
func TestStatuses_Good(t *core.T) {
	core.RequireTrue(t, process.Init(core.New()).OK)
	s := NewService(nil)
	dir := t.TempDir()
	r := process.Run(core.Background(), "git", "-C", dir, "init", "-b", "main")
	core.RequireTrue(t, r.OK, r.Error())

	got := s.statuses(core.Background(), []string{dir})
	core.AssertLen(t, got, 1)
	core.AssertEqual(t, "", got[0].Err)
}

// TestStatuses_Bad_UnreadablePath — a path that was never a repo (and
// doesn't exist) makes the underlying `git status` probe fail; the
// error is captured on the Status entry rather than panicking.
func TestStatuses_Bad_UnreadablePath(t *core.T) {
	s := NewService(nil)
	got := s.statuses(core.Background(), []string{core.PathJoin(t.TempDir(), "does-not-exist")})
	core.AssertLen(t, got, 1)
	core.AssertNotEmpty(t, got[0].Err)
}
