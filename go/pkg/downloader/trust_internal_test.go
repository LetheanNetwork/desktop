// SPDX-Licence-Identifier: EUPL-1.2

// Internal-package tests for the unexported `verify` digest checker.
// Lives under `package downloader` (not `downloader_test`) so the
// unexported symbol is reachable; the cross-package trust_test.go
// stays in `package downloader_test` for everything else.
//
// Background: `verify` was unexported per Cerberus #49 F-5 (Mantis
// #1675) as a confused-deputy guard — public callers must use
// FetchVerified so digest checks are bound to the quarantine +
// atomic-promote pipeline.

package downloader

import (
	core "dappco.re/go"
)

// internalHomeFixture is a package-internal twin of the cross-package
// homeFixture in downloader_test.go. Sandboxes $HOME into a t.TempDir
// so paths.ModelsDir() resolves under a disposable tree.
func internalHomeFixture(t *core.T) string {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	return tmp
}

func TestTrust_Verify_Good(t *core.T) {
	home := internalHomeFixture(t)
	dest := core.PathJoin(home, "Lethean", "conf", "models", "tiny.bin")
	core.AssertTrue(t, core.MkdirAll(core.PathDir(dest), 0o755).OK)
	payload := []byte("MOCK-PAYLOAD")
	core.AssertTrue(t, core.WriteFile(dest, payload, 0o644).OK)

	expected := core.SHA256HexString("MOCK-PAYLOAD")
	r := verify(dest, expected)
	core.AssertTrue(t, r.OK)
}

func TestTrust_Verify_Bad_Mismatch(t *core.T) {
	home := internalHomeFixture(t)
	dest := core.PathJoin(home, "Lethean", "conf", "models", "tiny.bin")
	core.AssertTrue(t, core.MkdirAll(core.PathDir(dest), 0o755).OK)
	core.AssertTrue(t, core.WriteFile(dest, []byte("ACTUAL"), 0o644).OK)

	r := verify(dest, "0000000000000000000000000000000000000000000000000000000000000000")
	core.AssertFalse(t, r.OK)
	// Error message includes both the expected and computed digests
	// for log surfacing.
	msg := r.Error()
	core.AssertTrue(t, core.Contains(msg, "expected"))
	core.AssertTrue(t, core.Contains(msg, "got"))
}

func TestTrust_Verify_Bad_EmptyDigest(t *core.T) {
	home := internalHomeFixture(t)
	dest := core.PathJoin(home, "Lethean", "conf", "models", "tiny.bin")
	core.RequireTrue(t, core.MkdirAll(core.PathDir(dest), 0o755).OK)
	core.RequireTrue(t, core.WriteFile(dest, []byte("x"), 0o644).OK)

	r := verify(dest, "")
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "sha256hex is required")
}

func TestTrust_Verify_Bad_MissingFile(t *core.T) {
	internalHomeFixture(t)
	r := verify("/nonexistent/path/nope.bin",
		"0000000000000000000000000000000000000000000000000000000000000000")
	core.AssertFalse(t, r.OK)
}

// TestTrust_QuarantineDir_Good — resolves + creates .quarantine/ under
// the models dir, idempotently (a second call against the same
// already-created dir also succeeds).
func TestTrust_QuarantineDir_Good(t *core.T) {
	home := internalHomeFixture(t)
	r := quarantineDir()
	core.RequireTrue(t, r.OK)
	qd := r.Value.(string)
	core.AssertEqual(t, core.PathJoin(home, "Lethean", "conf", "models", ".quarantine"), qd)
	core.AssertTrue(t, core.Stat(qd).OK)

	// Second call — already exists, still Ok.
	r2 := quarantineDir()
	core.RequireTrue(t, r2.OK)
	core.AssertEqual(t, qd, r2.Value.(string))
}

// TestTrust_QuarantineDir_Bad — when paths.ModelsDir() can't resolve
// (HOME pointing at a file, not a directory — mkdir fails
// deterministically regardless of platform HOME-resolution
// fallbacks), quarantineDir propagates the failure rather than
// returning a half-built path.
func TestTrust_QuarantineDir_Bad(t *core.T) {
	blocker := t.TempDir() + "/not-a-directory"
	core.RequireTrue(t, core.WriteFile(blocker, []byte("x"), 0o644).OK)
	t.Setenv("HOME", blocker)

	r := quarantineDir()
	core.AssertFalse(t, r.OK)
}

// TestTrust_CleanStaleQuarantine_Bad_NilCore — a nil Core is a safe
// no-op. (The happy-path remove-stale/keep-fresh sweep is already
// pinned end-to-end by TestTrust_QuarantineClean_Ugly in
// trust_test.go via WailsService.ServiceStartup.)
func TestTrust_CleanStaleQuarantine_Bad_NilCore(t *core.T) {
	cleanStaleQuarantine(nil) // must not panic
}

// TestTrust_CleanStaleQuarantine_Ugly_EmptyDir — an empty (freshly
// created) quarantine dir is a clean no-op sweep.
func TestTrust_CleanStaleQuarantine_Ugly_EmptyDir(t *core.T) {
	internalHomeFixture(t)
	core.RequireTrue(t, func() bool { r := quarantineDir(); return r.OK }())
	cleanStaleQuarantine(core.New()) // must not panic; nothing to remove
}

// TestQuarantineCreateFile_Bad_PermissionDenied — createQuarantineFile's
// generic-error fallback (neither ELOOP nor EEXIST) fires when the
// parent directory itself refuses writes, e.g. a read-only quarantine
// dir (disk near-full remount, permissions drift). Root-run test
// environments bypass POSIX permission bits, so this test skips
// itself when the write unexpectedly succeeds rather than asserting
// blindly.
func TestQuarantineCreateFile_Bad_PermissionDenied(t *core.T) {
	home := internalHomeFixture(t)
	qd := core.PathJoin(home, "Lethean", "conf", "models", ".quarantine")
	core.RequireTrue(t, core.MkdirAll(qd, 0o755).OK)
	core.RequireTrue(t, core.Chmod(qd, 0o500).OK) // read+execute only, no write
	t.Cleanup(func() { _ = core.Chmod(qd, 0o755) })

	target := core.PathJoin(qd, "locked-out.partial")
	r := createQuarantineFile(target)
	if r.OK {
		f, _ := r.Value.(*core.OSFile)
		if f != nil {
			_ = f.Close()
		}
		t.Skip("write succeeded despite 0500 perms — running as an account that bypasses POSIX modes")
	}
	core.AssertContains(t, r.Error(), "quarantine create failed")
}
