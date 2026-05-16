// SPDX-Licence-Identifier: EUPL-1.2

// Cerberus Mantis #1432 — internal-package tests for the
// install-time security gates. Internal pack (package plugin, not
// plugin_test) so we can drive verifyChecksum + allowedLocalPath
// directly without needing a full Service + manifest fixture.

package plugin

import core "dappco.re/go"

func TestPlugin_VerifyChecksum_Bad_EmptyRejected(t *core.T) {
	r := verifyChecksum([]byte("payload"), "")
	core.AssertFalse(t, r.OK,
		"empty checksum must fail (was previously a no-op pre-#1432)")
}

func TestPlugin_VerifyChecksum_Bad_WhitespaceRejected(t *core.T) {
	r := verifyChecksum([]byte("payload"), "   \t\n")
	core.AssertFalse(t, r.OK,
		"whitespace-only checksum trims to empty and must fail")
}

func TestPlugin_VerifyChecksum_Bad_UnsupportedAlgorithm(t *core.T) {
	r := verifyChecksum([]byte("payload"), "md5:abc123")
	core.AssertFalse(t, r.OK,
		"non-sha256 algorithms must be rejected")
}

func TestPlugin_VerifyChecksum_Bad_HashMismatch(t *core.T) {
	r := verifyChecksum([]byte("payload"),
		"sha256:0000000000000000000000000000000000000000000000000000000000000000")
	core.AssertFalse(t, r.OK,
		"wrong sha256 must be rejected")
}

func TestPlugin_VerifyChecksum_Good_MatchingHash(t *core.T) {
	// sha256("payload") = 239f59ed55e737c77147cf55ad0c1b030b6d7ee748a7426952f9b852d5a935e5
	r := verifyChecksum([]byte("payload"),
		"sha256:239f59ed55e737c77147cf55ad0c1b030b6d7ee748a7426952f9b852d5a935e5")
	core.AssertTrue(t, r.OK,
		"matching sha256 must succeed")
}

func TestPlugin_AllowedLocalPath_Bad_DockerSocket(t *core.T) {
	r := allowedLocalPath("/var/run/docker.sock")
	core.AssertFalse(t, r.OK,
		"/var/run/docker.sock must be rejected")
}

func TestPlugin_AllowedLocalPath_Bad_EtcPasswd(t *core.T) {
	r := allowedLocalPath("/etc/passwd")
	core.AssertFalse(t, r.OK,
		"/etc/passwd must be rejected")
}

func TestPlugin_AllowedLocalPath_Bad_UsrBinCurl(t *core.T) {
	r := allowedLocalPath("/usr/bin/curl")
	core.AssertFalse(t, r.OK,
		"/usr/bin/curl must be rejected")
}

func TestPlugin_AllowedLocalPath_Bad_Empty(t *core.T) {
	r := allowedLocalPath("")
	core.AssertFalse(t, r.OK,
		"empty local_path must be rejected")
}

func TestPlugin_AllowedLocalPath_Bad_Relative(t *core.T) {
	r := allowedLocalPath("relative/path/binary")
	core.AssertFalse(t, r.OK,
		"relative paths must be rejected (CWD-dependent)")
}

func TestPlugin_AllowedLocalPath_Bad_Traversal(t *core.T) {
	// Set HOME to a tempdir so the test isolates from the real home.
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	// "/Users/snider/Lethean/../../etc/passwd" cleans to "/etc/passwd".
	r := allowedLocalPath(tmp + "/lthn/../../etc/passwd")
	core.AssertFalse(t, r.OK,
		"path-traversal that escapes HOME via .. must be rejected")
}

func TestPlugin_AllowedLocalPath_Good_UnderHome(t *core.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	// Cerberus #1447 — non-existent paths now reject; create the
	// file first so EvalSymlinks can resolve it.
	core.RequireTrue(t, core.MkdirAll(tmp+"/Downloads", 0o755).OK)
	core.RequireTrue(t, core.WriteFile(tmp+"/Downloads/my-plugin.bin", []byte("ok"), 0o644).OK)
	r := allowedLocalPath(tmp + "/Downloads/my-plugin.bin")
	core.AssertTrue(t, r.OK,
		"absolute path under HOME that resolves to an existing file must be allowed")
}

func TestPlugin_AllowedLocalPath_Good_DeepUnderHome(t *core.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	core.RequireTrue(t, core.MkdirAll(tmp+"/Lethean/conf/plugin-staging", 0o755).OK)
	core.RequireTrue(t, core.WriteFile(tmp+"/Lethean/conf/plugin-staging/x.bin", []byte("ok"), 0o644).OK)
	r := allowedLocalPath(tmp + "/Lethean/conf/plugin-staging/x.bin")
	core.AssertTrue(t, r.OK,
		"deeply nested existing path under HOME must be allowed")
}

// TestPlugin_AllowedLocalPath_Bad_NonExistent — Cerberus #1447: the
// previous #1444 implementation proceeded when EvalSymlinks failed
// (path doesn't exist yet), creating a TOCTOU window between the
// gate and the caller's ReadFile. Now fails-closed: non-existent
// paths reject. A legitimate caller would have a real file ready.
func TestPlugin_AllowedLocalPath_Bad_NonExistent(t *core.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	r := allowedLocalPath(tmp + "/Downloads/does-not-exist.bin")
	core.AssertFalse(t, r.OK,
		"non-existent path must reject (closes #1447 TOCTOU)")
}

// Cerberus Mantis #1436 — plugin code + binary-path traversal gates.
// Shape mirrors sandbox.IsValidVolumeName tests (same primitive on
// adjacent surface).

func TestPlugin_IsValidPluginCode_Good(t *core.T) {
	core.AssertTrue(t, isValidPluginCode("opencode"))
	core.AssertTrue(t, isValidPluginCode("ollama"))
	core.AssertTrue(t, isValidPluginCode("forgejo_runner"))
	core.AssertTrue(t, isValidPluginCode("phpmyadmin-5"))
	core.AssertTrue(t, isValidPluginCode("a"))
	core.AssertTrue(t, isValidPluginCode("z123"))
}

func TestPlugin_IsValidPluginCode_Bad_PathTraversal(t *core.T) {
	// The exact attack pattern Cerberus flagged.
	core.AssertFalse(t, isValidPluginCode("../../etc"))
	core.AssertFalse(t, isValidPluginCode(".."))
	core.AssertFalse(t, isValidPluginCode("../foo"))
}

func TestPlugin_IsValidPluginCode_Bad_LeadingSlash(t *core.T) {
	core.AssertFalse(t, isValidPluginCode("/etc/passwd"))
	core.AssertFalse(t, isValidPluginCode("/"))
}

func TestPlugin_IsValidPluginCode_Bad_LeadingDot(t *core.T) {
	core.AssertFalse(t, isValidPluginCode(".hidden"))
}

func TestPlugin_IsValidPluginCode_Bad_LeadingDash(t *core.T) {
	core.AssertFalse(t, isValidPluginCode("-rf"))
	core.AssertFalse(t, isValidPluginCode("--privileged"))
}

func TestPlugin_IsValidPluginCode_Bad_Empty(t *core.T) {
	core.AssertFalse(t, isValidPluginCode(""))
}

func TestPlugin_IsValidPluginCode_Bad_TooLong(t *core.T) {
	long := "a"
	for i := 0; i < 64; i++ {
		long += "x"
	}
	core.AssertFalse(t, isValidPluginCode(long))
}

func TestPlugin_IsValidPluginCode_Bad_SpecialChars(t *core.T) {
	core.AssertFalse(t, isValidPluginCode("code/sub"))
	core.AssertFalse(t, isValidPluginCode("code.with.dot"))
	core.AssertFalse(t, isValidPluginCode("code space"))
	core.AssertFalse(t, isValidPluginCode("code;rm"))
	core.AssertFalse(t, isValidPluginCode("code$(pwd)"))
}

func TestPlugin_IsValidBinaryPath_Good(t *core.T) {
	core.AssertTrue(t, isValidBinaryPath("bin/foo"))
	core.AssertTrue(t, isValidBinaryPath("bin/opencode"))
	core.AssertTrue(t, isValidBinaryPath("relative/path/to/binary"))
	core.AssertTrue(t, isValidBinaryPath("standalone"))
}

func TestPlugin_IsValidBinaryPath_Bad_Traversal(t *core.T) {
	// The exact attack pattern.
	core.AssertFalse(t, isValidBinaryPath("../../etc/passwd"))
	core.AssertFalse(t, isValidBinaryPath("bin/../../escape"))
	core.AssertFalse(t, isValidBinaryPath(".."))
}

func TestPlugin_IsValidBinaryPath_Bad_LeadingSlash(t *core.T) {
	core.AssertFalse(t, isValidBinaryPath("/etc/passwd"))
	core.AssertFalse(t, isValidBinaryPath("/usr/bin/curl"))
}

func TestPlugin_IsValidBinaryPath_Bad_BackslashTraversal(t *core.T) {
	// Windows-flavoured path-traversal trick.
	core.AssertFalse(t, isValidBinaryPath("bin\\..\\escape"))
}

func TestPlugin_IsValidBinaryPath_Bad_NullByte(t *core.T) {
	// NUL byte injection — some platforms truncate paths at NUL.
	core.AssertFalse(t, isValidBinaryPath("bin/foo\x00bar"))
}

func TestPlugin_IsValidBinaryPath_Bad_Empty(t *core.T) {
	core.AssertFalse(t, isValidBinaryPath(""))
}
