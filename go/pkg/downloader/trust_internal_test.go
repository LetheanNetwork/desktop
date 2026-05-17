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

func TestTrust_Verify_Bad_MissingFile(t *core.T) {
	internalHomeFixture(t)
	r := verify("/nonexistent/path/nope.bin",
		"0000000000000000000000000000000000000000000000000000000000000000")
	core.AssertFalse(t, r.OK)
}
