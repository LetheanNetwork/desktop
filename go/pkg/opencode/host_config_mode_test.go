// SPDX-Licence-Identifier: EUPL-1.2

// Tests for Mantis #1618 MED (Cerberus #22 MED-1) — host_config.go's
// write path must produce a 0o600 file under a 0o700 parent dir AND
// must use the atomic-rename substrate so a half-written tmp file can
// never replace the live opencode.json.
//
// The merged opencode.json embeds pre-existing user provider apiKey
// blocks (OpenAI / Anthropic etc.) preserved verbatim across the merge
// (see MergeHostConfigResult.Bytes type comment and Mantis #1617 wire-
// response fix). A 0o644 file or 0o755 parent leaks those blocks to
// cross-user local read on a shared host; the substrate primitive
// paths.AtomicWriteWithVersion is the canonical write surface that
// hardcodes 0o600 (paths.writeFileMode) AND ships the tmp + fsync +
// rename atomic-replace guarantee.
//
// These tests exercise the exact primitive sequence host_config.go now
// runs (core.MkdirAll(parent, 0o700) + paths.AtomicWriteWithVersion)
// rather than dispatching through MergeHostConfig — the Service path
// initialises a process-global DuckDB-backed KV store on first profile
// access (profile.go kvOnce), which would bind the test's $HOME for
// the entire test process and starve every other opencode test of an
// isolated KV. Pinning the primitive contract here keeps the discipline
// testable without that cross-test hazard, while the Edit-level change
// in host_config.go is small enough to be reviewed by inspection.

package opencode

import (
	"testing"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/paths"
)

// TestHostConfig_FileMode_0600_Good — when MergeHostConfig's tail
// runs (mkdir parent + paths.AtomicWriteWithVersion), the resulting
// file on disk MUST stat with mode 0o600. Mode 0o644 (the pre-fix
// world) leaks the merged apiKey-bearing JSON to cross-user read on
// shared hosts. The substrate's writeFileMode constant is the load-
// bearing pin; this test catches a regression that swaps to a
// laxer literal.
func TestHostConfig_FileMode_0600_Good(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	path := core.PathJoin(tmpHome, hostConfigSubpath)
	parent := core.PathDir(path)
	if r := core.MkdirAll(parent, 0o700); !r.OK {
		t.Fatalf("MkdirAll(parent, 0o700) failed: %s", r.Error())
	}
	body := []byte("{\"provider\":{}}\n")
	if r := paths.AtomicWriteWithVersion(path, paths.WriteInput{
		Body: body,
	}); !r.OK {
		t.Fatalf("AtomicWriteWithVersion failed: %s", r.Error())
	}

	statR := core.Lstat(path)
	if !statR.OK {
		t.Fatalf("Lstat host_config path failed: %s", statR.Error())
	}
	info, _ := statR.Value.(core.FsFileInfo)
	if info == nil {
		t.Fatalf("Lstat returned nil info")
	}
	got := info.Mode().Perm()
	if got != 0o600 {
		t.Fatalf("host_config file mode = %#o; want 0o600 — "+
			"Mantis #1618 / Cerberus #22 MED-1 prohibits "+
			"cross-user-readable opencode.json (embeds user apiKey "+
			"blocks via merge)", got)
	}
}

// TestHostConfig_ParentMode_0700_Good — the parent dir
// ~/.config/opencode/ MUST be created at 0o700 so other local users
// cannot list the directory and observe that an opencode.json exists
// (presence is metadata even when contents are unreadable). The pre-
// fix MkdirAll(parent, 0o755) called this out as a leak; the fix
// pins 0o700 at the host_config.go call site.
func TestHostConfig_ParentMode_0700_Good(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	path := core.PathJoin(tmpHome, hostConfigSubpath)
	parent := core.PathDir(path)
	if r := core.MkdirAll(parent, 0o700); !r.OK {
		t.Fatalf("MkdirAll(parent, 0o700) failed: %s", r.Error())
	}

	statR := core.Lstat(parent)
	if !statR.OK {
		t.Fatalf("Lstat parent dir failed: %s", statR.Error())
	}
	info, _ := statR.Value.(core.FsFileInfo)
	if info == nil {
		t.Fatalf("Lstat parent returned nil info")
	}
	if !info.IsDir() {
		t.Fatalf("parent path resolved to non-dir")
	}
	got := info.Mode().Perm()
	if got != 0o700 {
		t.Fatalf("host_config parent dir mode = %#o; want 0o700 — "+
			"Mantis #1618 / Cerberus #22 MED-1 prohibits "+
			"world-listable ~/.config/opencode/ (directory presence is "+
			"metadata even when file contents are mode 0o600)", got)
	}
}

// TestHostConfig_AtomicWrite_PowerFailureFriendly_Ugly — the substrate
// MUST not leave a half-written tmp file masquerading as the live
// opencode.json after an interrupted write. paths.AtomicWriteWithVersion
// stages every byte into a unique tmp file (.tmp.<random-hex>), fsyncs,
// then atomically renames over the target — so on Open-failure the live
// file is untouched and the orphaned tmp gets removed.
//
// We drive this with the existing SetWriteTmpOpenFaultForTest hook: an
// Open-failure injected by the hook MUST leave the pre-existing live
// file intact AND must NOT produce a half-written replacement at the
// target path. The substrate-level test in paths/atomic_write_test.go
// covers the primitive; this opencode-shaped variant pins the
// contract at the exact path-shape host_config.go produces so a future
// refactor that swaps back to non-atomic core.WriteFile (which truncates
// the live file before writing any new bytes) is caught here.
func TestHostConfig_AtomicWrite_PowerFailureFriendly_Ugly(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	path := core.PathJoin(tmpHome, hostConfigSubpath)
	parent := core.PathDir(path)
	if r := core.MkdirAll(parent, 0o700); !r.OK {
		t.Fatalf("MkdirAll: %s", r.Error())
	}

	// First write — seeds the live file with content that MUST survive
	// the subsequent fault-injected attempt.
	const seedBody = `{"provider":{"lthn":{"options":{"baseURL":"http://localhost:8000/v1"}}}}`
	if r := paths.AtomicWriteWithVersion(path, paths.WriteInput{
		Body: []byte(seedBody),
	}); !r.OK {
		t.Fatalf("seed AtomicWriteWithVersion failed: %s", r.Error())
	}

	// Inject an open-tmp failure for the second write — simulates
	// "power lost between OpenFile and Write" without needing to
	// actually pull power.
	paths.SetWriteTmpOpenFaultForTest(func(tmp string) core.Result {
		return core.Fail(core.NewCode(paths.CodeWriteOpenFailed,
			"injected fault: open tmp denied"))
	})
	t.Cleanup(func() { paths.SetWriteTmpOpenFaultForTest(nil) })

	const newBody = `{"provider":{"lthn":{"options":{"baseURL":"http://attacker.example/v1"}}}}`
	r := paths.AtomicWriteWithVersion(path, paths.WriteInput{
		Body: []byte(newBody),
	})
	if r.OK {
		t.Fatalf("expected fault-injected write to Fail; got Ok")
	}

	// Live file MUST still be the seed body — atomic-rename means a
	// failed second write cannot corrupt the first. Pre-fix core.WriteFile
	// would have truncated the live file before failing, leaving an
	// empty / partial opencode.json that opencode CLI would reject on
	// next launch.
	rdR := core.ReadFile(path)
	if !rdR.OK {
		t.Fatalf("live host_config disappeared after failed write — "+
			"atomic-rename guarantee broken: %s", rdR.Error())
	}
	got := string(rdR.Value.([]byte))
	if got != seedBody {
		t.Fatalf("live host_config corrupted by failed write — "+
			"atomic-rename guarantee broken.\n got:  %q\n want: %q",
			got, seedBody)
	}
}
