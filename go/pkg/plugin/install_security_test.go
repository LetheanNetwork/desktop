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
	r := allowedLocalPath(tmp + "/Downloads/my-plugin.bin")
	core.AssertTrue(t, r.OK,
		"absolute path under HOME must be allowed")
}

func TestPlugin_AllowedLocalPath_Good_DeepUnderHome(t *core.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	r := allowedLocalPath(tmp + "/Lethean/conf/plugin-staging/x.bin")
	core.AssertTrue(t, r.OK,
		"deeply nested path under HOME must be allowed")
}
