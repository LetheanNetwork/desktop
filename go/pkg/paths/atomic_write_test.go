// SPDX-Licence-Identifier: EUPL-1.2

// atomic_write_test.go — RFC.atomic-write.md §10 mandatory tests for
// AtomicWriteWithVersion + ReadVersion + AtomicAppendLine. Test
// naming follows AX-10 Good/Bad/Ugly; the canonical list from §10
// is realised here (one Go test per RFC test name).

package paths_test

import (
	"sync"
	"testing"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/paths"
)

// helper — relative-under-root file path.
func tmpFile(t *core.T, name string) string {
	t.Helper()
	root := paths.Root()
	if !root.OK {
		t.Fatalf("Root: %s", root.Error())
	}
	return core.PathJoin(root.Value.(string), name)
}

func TestAtomicWrite_Good(t *core.T) {
	homeFixture(t)
	fp := tmpFile(t, "good.md")
	r := paths.AtomicWriteWithVersion(fp, paths.WriteInput{
		Body: []byte("hello world"),
	})
	core.AssertTrue(t, r.OK, "first write should succeed: "+r.Error())
	out := r.Value.(paths.WriteOutput)
	core.AssertEqual(t, core.SHA256Hex([]byte("hello world")), out.Hash)
	// File landed on disk.
	rd := paths.ReadVersion(fp)
	core.AssertTrue(t, rd.OK)
	cur := rd.Value.(paths.ReadOutput)
	core.AssertEqual(t, "hello world", string(cur.Body))
}

func TestAtomicWrite_Bad(t *core.T) {
	homeFixture(t)
	r := paths.AtomicWriteWithVersion("", paths.WriteInput{Body: []byte("x")})
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), paths.CodeWriteInvalidPath)
}

func TestAtomicWrite_VersionStale_Ugly(t *core.T) {
	homeFixture(t)
	fp := tmpFile(t, "stale.md")

	// Seed file with version 1.
	body1 := []byte("---\nversion: 1\n---\nfirst\n")
	r := paths.AtomicWriteWithVersion(fp, paths.WriteInput{Body: body1})
	core.AssertTrue(t, r.OK)

	rd := paths.ReadVersion(fp)
	core.AssertTrue(t, rd.OK)
	cur := rd.Value.(paths.ReadOutput)
	core.AssertEqual(t, 1, cur.Version)

	// Writer A: expects version 1, writes 2 → succeeds.
	body2 := []byte("---\nversion: 2\n---\nsecond\n")
	rA := paths.AtomicWriteWithVersion(fp, paths.WriteInput{
		Body:      body2,
		IfVersion: 1,
	})
	core.AssertTrue(t, rA.OK)

	// Writer B: still expects version 1 → conflict.
	rB := paths.AtomicWriteWithVersion(fp, paths.WriteInput{
		Body:      []byte("---\nversion: 2\n---\nlost\n"),
		IfVersion: 1,
	})
	core.AssertFalse(t, rB.OK)
	core.AssertContains(t, rB.Error(), paths.CodeVersionStale)
	vs, ok := paths.VersionStaleFromError(rB.Value)
	core.AssertTrue(t, ok, "stale envelope should be extractable")
	core.AssertEqual(t, 2, vs.CurrentVersion)
	core.AssertNotEqual(t, "", vs.CurrentHash)
}

func TestAtomicWrite_MtimeRace_Ugly(t *core.T) {
	homeFixture(t)
	fp := tmpFile(t, "mtime.md")

	body1 := []byte("first")
	r := paths.AtomicWriteWithVersion(fp, paths.WriteInput{Body: body1})
	core.AssertTrue(t, r.OK)

	rd := paths.ReadVersion(fp)
	cur := rd.Value.(paths.ReadOutput)

	// Writer A passes IfMatchHash matching disk → succeeds.
	rA := paths.AtomicWriteWithVersion(fp, paths.WriteInput{
		Body:        []byte("second"),
		IfMtime:     cur.Mtime,
		IfMatchHash: cur.BodyHash,
	})
	core.AssertTrue(t, rA.OK)

	// Writer B passes the SAME old hash; even if mtime resolution is
	// only 1s and Now() == prior Mtime, the hash mismatch surfaces
	// the conflict deterministically.
	rB := paths.AtomicWriteWithVersion(fp, paths.WriteInput{
		Body:        []byte("third"),
		IfMtime:     cur.Mtime,
		IfMatchHash: cur.BodyHash,
	})
	core.AssertFalse(t, rB.OK)
	core.AssertContains(t, rB.Error(), paths.CodeVersionStale)
}

func TestAtomicWrite_CompositeAllThree_Ugly(t *core.T) {
	homeFixture(t)
	fp := tmpFile(t, "composite.md")

	body1 := []byte("---\nversion: 7\n---\nseed\n")
	r := paths.AtomicWriteWithVersion(fp, paths.WriteInput{Body: body1})
	core.AssertTrue(t, r.OK)

	rd := paths.ReadVersion(fp)
	cur := rd.Value.(paths.ReadOutput)

	// All-three match — succeed.
	rOK := paths.AtomicWriteWithVersion(fp, paths.WriteInput{
		Body:        []byte("---\nversion: 8\n---\nadvance\n"),
		IfVersion:   7,
		IfMtime:     cur.Mtime,
		IfMatchHash: cur.BodyHash,
	})
	core.AssertTrue(t, rOK.OK, "all-three-match should succeed: "+rOK.Error())

	// Read post-write snapshot for stale-attempt.
	rd2 := paths.ReadVersion(fp)
	cur2 := rd2.Value.(paths.ReadOutput)

	// Any single mismatch → stale. Mismatched IfVersion only.
	r1 := paths.AtomicWriteWithVersion(fp, paths.WriteInput{
		Body:        []byte("x"),
		IfVersion:   99,
		IfMtime:     cur2.Mtime,
		IfMatchHash: cur2.BodyHash,
	})
	core.AssertFalse(t, r1.OK)
	core.AssertContains(t, r1.Error(), paths.CodeVersionStale)

	// Mismatched IfMatchHash only.
	r2 := paths.AtomicWriteWithVersion(fp, paths.WriteInput{
		Body:        []byte("x"),
		IfVersion:   8,
		IfMtime:     cur2.Mtime,
		IfMatchHash: "wrong",
	})
	core.AssertFalse(t, r2.OK)
	core.AssertContains(t, r2.Error(), paths.CodeVersionStale)
}

func TestAtomicWrite_PickOneIfMtimeOnly_Good(t *core.T) {
	homeFixture(t)
	fp := tmpFile(t, "pick-mtime.md")
	r := paths.AtomicWriteWithVersion(fp, paths.WriteInput{Body: []byte("seed")})
	core.AssertTrue(t, r.OK)
	rd := paths.ReadVersion(fp)
	cur := rd.Value.(paths.ReadOutput)

	r2 := paths.AtomicWriteWithVersion(fp, paths.WriteInput{
		Body:    []byte("post"),
		IfMtime: cur.Mtime,
	})
	core.AssertTrue(t, r2.OK, "mtime-only pick should succeed: "+r2.Error())
}

func TestAtomicWrite_PickOneIfMatchHashOnly_Good(t *core.T) {
	homeFixture(t)
	fp := tmpFile(t, "pick-hash.md")
	r := paths.AtomicWriteWithVersion(fp, paths.WriteInput{Body: []byte("seed")})
	core.AssertTrue(t, r.OK)
	rd := paths.ReadVersion(fp)
	cur := rd.Value.(paths.ReadOutput)

	r2 := paths.AtomicWriteWithVersion(fp, paths.WriteInput{
		Body:        []byte("post"),
		IfMatchHash: cur.BodyHash,
	})
	core.AssertTrue(t, r2.OK, "hash-only pick should succeed: "+r2.Error())
}

func TestAtomicWrite_ConflictBodyOptInDefault_Bad(t *core.T) {
	homeFixture(t)
	fp := tmpFile(t, "optin.md")
	r := paths.AtomicWriteWithVersion(fp, paths.WriteInput{Body: []byte("seed")})
	core.AssertTrue(t, r.OK)

	// Stale write WITHOUT IncludeBody → response omits body.
	stale := paths.AtomicWriteWithVersion(fp, paths.WriteInput{
		Body:        []byte("x"),
		IfMatchHash: "stale-hash",
		IncludeBody: false,
	})
	core.AssertFalse(t, stale.OK)
	vs, ok := paths.VersionStaleFromError(stale.Value)
	core.AssertTrue(t, ok)
	core.AssertEqual(t, 0, len(vs.CurrentBody),
		"default omit — CurrentBody MUST be empty when IncludeBody=false")
	core.AssertNotEqual(t, "", vs.CurrentHash)
}

func TestAtomicWrite_AtRestPathNeverIncludesBody_Ugly(t *core.T) {
	homeFixture(t)
	// Plant a file under wallets/ — the prefix list catches this.
	walletDir := paths.WalletsDir()
	core.AssertTrue(t, walletDir.OK)
	fp := core.PathJoin(walletDir.Value.(string), "server.key")
	if r := core.WriteFile(fp, []byte("ciphertext"), 0o600); !r.OK {
		t.Fatalf("plant wallet file: %s", r.Error())
	}
	core.AssertTrue(t, paths.IsAtRestEncryptedPath(fp),
		"wallets/server.key MUST classify as at-rest-encrypted")

	stale := paths.AtomicWriteWithVersion(fp, paths.WriteInput{
		Body:        []byte("x"),
		IfMatchHash: "stale-hash",
		IncludeBody: true, // explicit opt-in
	})
	core.AssertFalse(t, stale.OK)
	vs, ok := paths.VersionStaleFromError(stale.Value)
	core.AssertTrue(t, ok)
	core.AssertEqual(t, 0, len(vs.CurrentBody),
		"at-rest-encrypted prefix MUST omit body regardless of IncludeBody")
	// Flag surfaces the reason for the omission.
	found := false
	for _, f := range vs.Flags {
		if f == paths.CurrentBodyAtRestOmitFlag {
			found = true
		}
	}
	core.AssertTrue(t, found,
		"flag MUST advertise the at-rest omission so the client can route to GET")
}

func TestAtomicWrite_ConflictBody1MBCap_Ugly(t *core.T) {
	homeFixture(t)
	fp := tmpFile(t, "big.md")
	big := make([]byte, paths.CurrentBodyMaxBytes+1024)
	for i := range big {
		big[i] = 'a'
	}
	if r := core.WriteFile(fp, big, 0o600); !r.OK {
		t.Fatalf("seed: %s", r.Error())
	}
	stale := paths.AtomicWriteWithVersion(fp, paths.WriteInput{
		Body:        []byte("x"),
		IfMatchHash: "stale-hash",
		IncludeBody: true,
	})
	core.AssertFalse(t, stale.OK)
	vs, ok := paths.VersionStaleFromError(stale.Value)
	core.AssertTrue(t, ok)
	core.AssertEqual(t, 0, len(vs.CurrentBody))
	found := false
	for _, f := range vs.Flags {
		if f == paths.CurrentBodyTooLargeFlag {
			found = true
		}
	}
	core.AssertTrue(t, found)
}

func TestAtomicWrite_FirstWriteFromLegacy_Ugly(t *core.T) {
	homeFixture(t)
	fp := tmpFile(t, "legacy.md")
	// MED-2: missing file + all-three-empty → unconditional write.
	r := paths.AtomicWriteWithVersion(fp, paths.WriteInput{
		Body: []byte("---\nversion: 1\n---\nseed\n"),
	})
	core.AssertTrue(t, r.OK, "first write should succeed: "+r.Error())
	rd := paths.ReadVersion(fp)
	core.AssertTrue(t, rd.OK)
	cur := rd.Value.(paths.ReadOutput)
	core.AssertEqual(t, 1, cur.Version)
}

func TestReadVersion_LegacyFileReturnsZero_Good(t *core.T) {
	homeFixture(t)
	fp := tmpFile(t, "no-fm.md")
	if r := core.WriteFile(fp, []byte("plain body, no frontmatter"), 0o600); !r.OK {
		t.Fatalf("seed: %s", r.Error())
	}
	rd := paths.ReadVersion(fp)
	core.AssertTrue(t, rd.OK)
	cur := rd.Value.(paths.ReadOutput)
	core.AssertEqual(t, 0, cur.Version)
}

func TestReadVersion_MissingFile_Good(t *core.T) {
	homeFixture(t)
	fp := tmpFile(t, "absent.md")
	rd := paths.ReadVersion(fp)
	core.AssertTrue(t, rd.OK, "missing file should produce zero ReadOutput, not Fail")
	cur := rd.Value.(paths.ReadOutput)
	core.AssertEqual(t, 0, cur.Version)
	core.AssertEqual(t, 0, len(cur.Body))
	core.AssertTrue(t, cur.Mtime.IsZero())
}

func TestVersionFrontmatter_LazyMigration_Ugly(t *core.T) {
	homeFixture(t)
	fp := tmpFile(t, "lazy.md")
	if r := core.WriteFile(fp, []byte("plain"), 0o600); !r.OK {
		t.Fatalf("seed: %s", r.Error())
	}
	// Step 1: legacy file reads as version 0.
	rd := paths.ReadVersion(fp)
	core.AssertEqual(t, 0, rd.Value.(paths.ReadOutput).Version)
	// Step 2: write a body with version: 1 frontmatter under the
	// unconditional first-write semantics.
	r := paths.AtomicWriteWithVersion(fp, paths.WriteInput{
		Body:        []byte("---\nversion: 1\n---\nbody\n"),
		IfMatchHash: rd.Value.(paths.ReadOutput).BodyHash,
	})
	core.AssertTrue(t, r.OK, "lazy migration write should succeed: "+r.Error())
	// Step 3: re-read sees version 1.
	rd2 := paths.ReadVersion(fp)
	core.AssertEqual(t, 1, rd2.Value.(paths.ReadOutput).Version)
}

func TestAtomicAppendLine_AtomicPerRecord_Good(t *core.T) {
	homeFixture(t)
	fp := tmpFile(t, "append.md")
	const N = 100

	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		i := i
		go func() {
			defer wg.Done()
			line := []byte(core.Sprintf("rec-%03d", i))
			r := paths.AtomicAppendLine(fp, line)
			if !r.OK {
				t.Errorf("append %d: %s", i, r.Error())
			}
		}()
	}
	wg.Wait()

	// Read back; every line should be intact (no interleaving).
	raw := core.ReadFile(fp)
	core.AssertTrue(t, raw.OK)
	body := raw.Value.([]byte)
	count := 0
	start := 0
	for i := 0; i < len(body); i++ {
		if body[i] == '\n' {
			line := string(body[start:i])
			if len(line) != len("rec-000") {
				t.Errorf("line %d malformed: %q", count, line)
			}
			count++
			start = i + 1
		}
	}
	core.AssertEqual(t, N, count)
}

func TestAtomicAppendLine_RecordOverLimitRejected_Bad(t *core.T) {
	homeFixture(t)
	paths.SetPipeBufLimitForTest(64)
	t.Cleanup(func() { paths.SetPipeBufLimitForTest(0) })

	fp := tmpFile(t, "over.md")
	big := make([]byte, 200)
	for i := range big {
		big[i] = 'a'
	}
	r := paths.AtomicAppendLine(fp, big)
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), paths.CodeAppendRecordTooLarge)
}

func TestAtomicAppendLine_RotationAtThreshold_Ugly(t *core.T) {
	homeFixture(t)
	paths.SetAppendRotateThresholdForTest(32)
	t.Cleanup(func() { paths.SetAppendRotateThresholdForTest(paths.AppendRotateThreshold) })

	fp := tmpFile(t, "rotate.md")
	// Two small appends — second should fit; rotation triggers when
	// the existing size already crosses the threshold.
	for i := 0; i < 6; i++ {
		r := paths.AtomicAppendLine(fp, []byte(core.Sprintf("line %d aaaaaaaaaa", i)))
		core.AssertTrue(t, r.OK, "append "+core.Itoa(i)+": "+r.Error())
	}
	// Look for any .archived file under Root().
	root := paths.Root().Value.(string)
	entries := core.ReadDir(core.DirFS(root), ".")
	core.AssertTrue(t, entries.OK)
	list, _ := entries.Value.([]core.FsDirEntry)
	rotatedFound := false
	for _, e := range list {
		if core.HasPrefix(e.Name(), "rotate.md.") && core.HasSuffix(e.Name(), ".archived") {
			rotatedFound = true
		}
	}
	core.AssertTrue(t, rotatedFound, "an archived file MUST exist after rotation")
}

// TestAtomicAppendLine_RotateRecreateFailRollsBack_Ugly — Cerberus
// DREAD-r2 F3 (Mantis #1527). When the post-rename recreate fails,
// maybeRotate MUST roll back the rename so callers don't observe
// "current missing, archived present" split state. The
// fault-injection hook forces a single recreate failure; the next
// append (with the fault cleared) MUST succeed AND find no
// .archived sibling (rename was rolled back).
func TestAtomicAppendLine_RotateRecreateFailRollsBack_Ugly(t *core.T) {
	homeFixture(t)
	// Seed at the default threshold so no rotation fires during seed.
	t.Cleanup(func() { paths.SetAppendRotateThresholdForTest(paths.AppendRotateThreshold) })

	fp := tmpFile(t, "rollback.md")
	// Seed the file with normal appends, no rotation.
	for i := 0; i < 5; i++ {
		r := paths.AtomicAppendLine(fp, []byte(core.Sprintf("seed %d xxxxxxxxxx", i)))
		core.AssertTrue(t, r.OK, "seed append "+core.Itoa(i)+": "+r.Error())
	}

	// Now drop the threshold so the NEXT append triggers rotation.
	paths.SetAppendRotateThresholdForTest(32)

	// Arm the fault: next recreate fails. Provide a clean OpenFile
	// for any subsequent calls so post-recovery rotation can succeed.
	armed := true
	paths.SetRotateRecreateFaultForTest(func(p string) core.Result {
		if !armed {
			// Normal recreate when disarmed.
			return core.OpenFile(p,
				core.O_CREATE|core.O_WRONLY|core.O_TRUNC, 0o600)
		}
		armed = false
		return core.Result{OK: false, Value: core.NewCode("test.injected", "injected recreate failure")}
	})
	t.Cleanup(func() { paths.SetRotateRecreateFaultForTest(nil) })

	// Append again — rotation triggers, recreate fails, rollback
	// runs. The error surfaces but the on-disk state must NOT be
	// split (no .archived sibling left behind).
	r := paths.AtomicAppendLine(fp, []byte("after-fault yyyyyyyyyy"))
	core.AssertFalse(t, r.OK, "rotation with recreate fault must surface failure")
	core.AssertContains(t, r.Error(), paths.CodeAppendRotateFailed,
		"rollback path surfaces CodeAppendRotateFailed (not Split — rollback succeeded)")

	// File must still exist (rollback restored it) — confirm via Lstat.
	stat := core.Lstat(fp)
	core.AssertTrue(t, stat.OK, "current file must exist after rollback")

	// No .archived sibling should have been left behind.
	root := paths.Root().Value.(string)
	entries := core.ReadDir(core.DirFS(root), ".")
	core.AssertTrue(t, entries.OK)
	list, _ := entries.Value.([]core.FsDirEntry)
	for _, e := range list {
		if core.HasPrefix(e.Name(), "rollback.md.") && core.HasSuffix(e.Name(), ".archived") {
			t.Errorf("rollback path must remove .archived sibling, found: %s", e.Name())
		}
	}

	// Clear the fault and verify the system recovers — next append
	// rotates cleanly (archived sibling appears this time).
	paths.SetRotateRecreateFaultForTest(nil)
	r2 := paths.AtomicAppendLine(fp, []byte("recovered zzzzzzzzzz"))
	core.AssertTrue(t, r2.OK, "post-fault append should rotate cleanly: "+r2.Error())
	entries2 := core.ReadDir(core.DirFS(root), ".")
	core.AssertTrue(t, entries2.OK)
	list2, _ := entries2.Value.([]core.FsDirEntry)
	rotated := false
	for _, e := range list2 {
		if core.HasPrefix(e.Name(), "rollback.md.") && core.HasSuffix(e.Name(), ".archived") {
			rotated = true
		}
	}
	core.AssertTrue(t, rotated,
		"normal rotation must succeed once the fault is cleared")
}

// TestAtomicWrite_FailureEmitsAuditEvent_Ugly — Mantis #1551.
// When the open/write/fsync/rename phase fails, AtomicWriteWithVersion
// MUST emit a paths.write.failed audit event carrying the typed
// CodeWrite* sentinel + path_hash. Without this, write-step failures
// are silently dropped from the forensic record — operators can only
// see the success cases.
//
// Forcing uses SetWriteTmpOpenFaultForTest (mirrors the F3
// RotateRecreateFault pattern in atomic_append.go) — filesystem-
// permission tricks collide with WithFileLock's sentinel needing the
// same parent dir writeable, so a typed test hook is the cleanest
// way to inject a deterministic write-step failure.
func TestAtomicWrite_FailureEmitsAuditEvent_Ugly(t *core.T) {
	homeFixture(t)
	rec := withCaptureRecorder(t)

	root := paths.Root().Value.(string)
	dir := core.PathJoin(root, "writefail")
	core.AssertTrue(t, core.MkdirAll(dir, 0o700).OK)
	fp := core.PathJoin(dir, "target.md")

	paths.SetWriteTmpOpenFaultForTest(func(tmp string) core.Result {
		return core.Result{
			OK:    false,
			Value: core.NewCode("test.injected", "injected open failure"),
		}
	})
	t.Cleanup(func() { paths.SetWriteTmpOpenFaultForTest(nil) })

	r := paths.AtomicWriteWithVersion(fp, paths.WriteInput{
		Body: []byte("doomed"),
	})
	if r.OK {
		t.Fatalf("injected open failure should propagate Fail")
	}
	core.AssertContains(t, r.Error(), paths.CodeWriteOpenFailed)

	// Recorder MUST have seen a write.failed event with the typed Code.
	found := false
	for _, ev := range rec.snapshot() {
		if ev.Kind != paths.EventWriteFailed {
			continue
		}
		// Code MUST be one of the typed CodeWrite* sentinels —
		// open_failed is the expected step for an EACCES parent.
		if ev.Code != paths.CodeWriteOpenFailed &&
			ev.Code != paths.CodeWriteFsync &&
			ev.Code != paths.CodeWriteRename {
			t.Errorf("write.failed Code MUST be a typed CodeWrite* sentinel, got %q", ev.Code)
		}
		core.AssertNotEqual(t, "", ev.PathHash,
			"write.failed event MUST carry a path_hash like every other LockEvent")
		found = true
	}
	core.AssertTrue(t, found,
		"AtomicWriteWithVersion failure MUST emit a paths.write.failed audit event")
}

// TestAtomicWrite_RandomTmpSuffix_Good — Mantis #1552. Mirrors the
// #1541 paths.json random-suffix discipline: concurrent in-process
// writers must each own a distinct staging file so two writers
// trying to overwrite the same target cannot stomp the same
// "<path>.tmp" mid-stream. Asserts the legacy fixed-suffix tmp file
// never exists post-write and no ".tmp.*" stragglers linger after
// rename consumed them.
func TestAtomicWrite_RandomTmpSuffix_Good(t *core.T) {
	homeFixture(t)
	fp := tmpFile(t, "randsuffix.md")
	r := paths.AtomicWriteWithVersion(fp, paths.WriteInput{
		Body: []byte("first"),
	})
	core.AssertTrue(t, r.OK, "write should succeed: "+r.Error())

	// Legacy fixed-suffix path must NOT exist (the #1552 fix
	// abandoned ".tmp" in favour of ".tmp.<rand>").
	legacy := fp + ".tmp"
	core.AssertFalse(t, core.Lstat(legacy).OK,
		"legacy fixed-suffix .tmp must not exist after #1552 fix")

	// No random-suffix stragglers — rename should have consumed
	// every .tmp.<rand> entry into the target path.
	stragglers := core.PathGlob(fp + ".tmp.*")
	core.AssertEqual(t, 0, len(stragglers),
		"no .tmp.* stragglers should remain after successful rename")
}

func TestIsAtRestEncryptedPath_Coverage(t *core.T) {
	homeFixture(t)
	root := paths.Root().Value.(string)
	cases := []struct {
		path    string
		want    bool
	}{
		{core.PathJoin(root, "wallets/server.key"), true},
		{core.PathJoin(root, "account/abc/private.key"), true},
		{core.PathJoin(root, "office/mail/_accounts.enc"), true},
		{core.PathJoin(root, "sales/deals/x.md"), true},
		{core.PathJoin(root, "incidents"), true},
		{core.PathJoin(root, "incidents/2026.md"), true},
		{core.PathJoin(root, "runbooks/x.md"), true},
		{core.PathJoin(root, "office/documents/notes.md"), false},
		{core.PathJoin(root, "marketing/content.md"), false},
		{core.PathJoin(root, "conf/lthn.yaml"), false},
	}
	for _, tc := range cases {
		got := paths.IsAtRestEncryptedPath(tc.path)
		if got != tc.want {
			t.Errorf("IsAtRestEncryptedPath(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

var _ = testing.AllocsPerRun
