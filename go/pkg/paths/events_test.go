// SPDX-Licence-Identifier: EUPL-1.2

// events_test.go — RFC.atomic-write.md §10 audit-emission tests for
// §6.1 (RecordSync vs RecordBatch routing) and §6.2 MED-3 (HKDF
// domain separation of path-hashes).

package paths_test

import (
	"sync"
	"testing"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/paths"
)

// captureRecorder is a test-only AuditRecorder that retains every
// event it sees so assertions can inspect routing decisions.
type captureRecorder struct {
	mu     sync.Mutex
	events []paths.LockEvent
}

func (c *captureRecorder) RecordPathsEvent(ev paths.LockEvent) core.Result {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, ev)
	return core.Ok(nil)
}

func (c *captureRecorder) snapshot() []paths.LockEvent {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]paths.LockEvent, len(c.events))
	copy(out, c.events)
	return out
}

func withCaptureRecorder(t *core.T) *captureRecorder {
	t.Helper()
	rec := &captureRecorder{}
	paths.SetAuditRecorder(rec)
	// F2 (Mantis #1526): emit-site drops events when the audit
	// secret is unavailable to avoid the empty-PathHash side
	// channel. Tests that exercise the emission path MUST install
	// a non-empty secret provider.
	paths.SetAuditSecretProvider(func() []byte {
		return []byte("test-secret-32-bytes-1234567890ab")
	})
	t.Cleanup(func() {
		paths.SetAuditRecorder(nil)
		paths.SetAuditSecretProvider(nil)
	})
	return rec
}

func TestAuditEmission_RecordSyncForAuthSubstrate_Good(t *core.T) {
	homeFixture(t)
	rec := withCaptureRecorder(t)

	walletDir := paths.WalletsDir().Value.(string)
	fp := core.PathJoin(walletDir, "server.key")
	r := paths.AtomicWriteWithVersion(fp, paths.WriteInput{Body: []byte("cipher")})
	core.AssertTrue(t, r.OK, r.Error())

	got := rec.snapshot()
	core.AssertGreater(t, len(got), 0, "at least one event must fire on auth-substrate write")
	found := false
	for _, ev := range got {
		if ev.Kind == paths.EventWriteSucceeded && ev.Mode == paths.AuditModeSync {
			found = true
		}
	}
	core.AssertTrue(t, found,
		"wallets/server.key write MUST emit a Sync-mode event")
}

func TestAuditEmission_RecordBatchForCascade_Good(t *core.T) {
	homeFixture(t)
	rec := withCaptureRecorder(t)

	root := paths.Root().Value.(string)
	// Plant a deals dir under sales/. We're emitting from sales/
	// which is at-rest-encrypted (omit body) but its audit mode IS
	// cascade-batch — confirms the §6.1 table is independent of
	// the at-rest CRIT-1 list.
	dealsDir := core.PathJoin(root, "office", "documents")
	_ = core.MkdirAll(dealsDir, 0o700)
	fp := core.PathJoin(dealsDir, "notes.md")
	r := paths.AtomicWriteWithVersion(fp, paths.WriteInput{Body: []byte("body")})
	core.AssertTrue(t, r.OK, r.Error())

	got := rec.snapshot()
	found := false
	for _, ev := range got {
		if ev.Kind == paths.EventWriteSucceeded && ev.Mode == paths.AuditModeBatch {
			found = true
		}
	}
	core.AssertTrue(t, found,
		"office/documents write MUST emit a Batch-mode event")
}

func TestAuditEmission_PathHashedNotRaw_Ugly(t *core.T) {
	homeFixture(t)
	rec := withCaptureRecorder(t)
	// Install a non-empty audit secret so hashPath actually runs.
	paths.SetAuditSecretProvider(func() []byte {
		return []byte("test-secret-32-bytes-1234567890ab")
	})
	t.Cleanup(func() { paths.SetAuditSecretProvider(nil) })

	root := paths.Root().Value.(string)
	fp := core.PathJoin(root, "office", "documents", "notes.md")
	_ = core.MkdirAll(core.PathJoin(root, "office", "documents"), 0o700)
	r := paths.AtomicWriteWithVersion(fp, paths.WriteInput{Body: []byte("x")})
	core.AssertTrue(t, r.OK, r.Error())

	got := rec.snapshot()
	core.AssertGreater(t, len(got), 0)
	for _, ev := range got {
		if ev.Kind != paths.EventWriteSucceeded {
			continue
		}
		core.AssertNotEqual(t, "", ev.PathHash,
			"PathHash MUST be derived when secret is available")
		core.AssertNotContains(t, ev.PathHash, "documents",
			"PathHash MUST NOT contain raw path segments")
		core.AssertNotContains(t, ev.PathHash, fp,
			"PathHash MUST NOT carry the full path")
	}
}

func TestAuditEmission_HKDFDomainSeparation_Ugly(t *core.T) {
	homeFixture(t)
	paths.SetAuditSecretProvider(func() []byte {
		return []byte("test-secret-32-bytes-1234567890ab")
	})
	t.Cleanup(func() { paths.SetAuditSecretProvider(nil) })

	// Hash one path under two different account-id info strings;
	// outputs MUST differ.
	root := paths.Root().Value.(string)
	fp := core.PathJoin(root, "office", "documents", "samepath.md")
	_ = core.MkdirAll(core.PathJoin(root, "office", "documents"), 0o700)

	recA := &captureRecorder{}
	paths.SetAuditRecorder(recA)
	paths.SetCurrentAccountIDProvider(func() string { return "account-a" })
	r := paths.AtomicWriteWithVersion(fp, paths.WriteInput{Body: []byte("a")})
	core.AssertTrue(t, r.OK, r.Error())

	recB := &captureRecorder{}
	paths.SetAuditRecorder(recB)
	paths.SetCurrentAccountIDProvider(func() string { return "account-b" })
	r = paths.AtomicWriteWithVersion(fp, paths.WriteInput{Body: []byte("b")})
	core.AssertTrue(t, r.OK, r.Error())

	t.Cleanup(func() {
		paths.SetCurrentAccountIDProvider(nil)
		paths.SetAuditRecorder(nil)
	})

	hashA := ""
	for _, ev := range recA.snapshot() {
		if ev.Kind == paths.EventWriteSucceeded {
			hashA = ev.PathHash
		}
	}
	hashB := ""
	for _, ev := range recB.snapshot() {
		if ev.Kind == paths.EventWriteSucceeded {
			hashB = ev.PathHash
		}
	}
	core.AssertNotEqual(t, "", hashA)
	core.AssertNotEqual(t, "", hashB)
	core.AssertNotEqual(t, hashA, hashB,
		"same path under different accountIDs MUST hash differently (MED-3 domain separation)")
}

func TestAuditModeForPath_PolicyTable(t *core.T) {
	homeFixture(t)
	root := paths.Root().Value.(string)
	cases := []struct {
		rel  string
		want paths.AuditMode
	}{
		{"wallets/server.key", paths.AuditModeSync},
		{"wallets/.seed", paths.AuditModeSync},
		{"account/abc/private.key", paths.AuditModeSync},
		{"office/mail/_accounts.enc", paths.AuditModeSync},
		{"office/mail/INBOX/threads.md", paths.AuditModeBatch},
		{"office/documents/x.md", paths.AuditModeBatch},
		{"sales/deals/x.md", paths.AuditModeBatch},
		{"marketing/content.md", paths.AuditModeBatch},
		{"incidents", paths.AuditModeBatch},
		{"runbooks/x.md", paths.AuditModeBatch},
		{"conf/lthn.yaml", paths.AuditModeBatch},
	}
	for _, tc := range cases {
		got := paths.AuditModeForPath(core.PathJoin(root, tc.rel))
		if got != tc.want {
			t.Errorf("AuditModeForPath(%q) = %v, want %v", tc.rel, got, tc.want)
		}
	}
}

func TestPathHashInfo_FormatStable(t *core.T) {
	core.AssertEqual(t, "paths.lock.v1|", paths.PathHashInfo(""))
	core.AssertEqual(t, "paths.lock.v1|acct123", paths.PathHashInfo("acct123"))
}

func TestSubscribeLockEvents_Fanout(t *core.T) {
	homeFixture(t)
	withCaptureRecorder(t)
	t.Cleanup(paths.ClearLockEventSubscribersForTest)

	var got []paths.LockEvent
	var mu sync.Mutex
	paths.SubscribeLockEvents(func(ev paths.LockEvent) {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, ev)
	})

	fp := tmpFile(t, "fanout.md")
	r := paths.AtomicWriteWithVersion(fp, paths.WriteInput{Body: []byte("hi")})
	core.AssertTrue(t, r.OK, r.Error())

	mu.Lock()
	defer mu.Unlock()
	core.AssertGreater(t, len(got), 0, "subscriber MUST receive at least one event")
}

// TestAuditModeForPath_OutOfRootDefaultsToSync_Ugly — Cerberus
// DREAD-r2 F4 (Mantis #1528). A path that workspaceRel could not
// place under Root MUST default to AuditModeSync rather than
// silently downgrading auth-substrate crash-safety to Batch.
func TestAuditModeForPath_OutOfRootDefaultsToSync_Ugly(t *core.T) {
	homeFixture(t)
	// Path well outside Root, structurally absolute.
	got := paths.AuditModeForPath("/var/log/elsewhere/x.md")
	core.AssertEqual(t, paths.AuditModeSync, got,
		"out-of-Root path MUST fail-safe to Sync mode")

	// Empty path — also out-of-Root by construction.
	got = paths.AuditModeForPath("")
	core.AssertEqual(t, paths.AuditModeSync, got,
		"empty path MUST fail-safe to Sync mode")
}

// TestAuditModeForPath_TraversalNormalised_Ugly — Cerberus DREAD-r2
// F4 (Mantis #1528). Traversal segments embedded in a path under
// Root MUST resolve before the prefix table runs so a constructed
// "../wallets/..." cannot bypass the auth-substrate routing.
func TestAuditModeForPath_TraversalNormalised_Ugly(t *core.T) {
	homeFixture(t)
	root := paths.Root().Value.(string)

	// office//../wallets/server.key → wallets/server.key
	// Both representations MUST land on Sync via the auth prefix.
	dirty := core.PathJoin(root, "office", "..", "wallets", "server.key")
	core.AssertEqual(t, paths.AuditModeSync,
		paths.AuditModeForPath(dirty),
		"traversal-normalised wallets/ path MUST route to Sync")

	// office/documents/../../wallets/foo also resolves under wallets/.
	dirty2 := core.PathJoin(root, "office", "documents", "..", "..", "wallets", "foo")
	core.AssertEqual(t, paths.AuditModeSync,
		paths.AuditModeForPath(dirty2),
		"deep traversal resolving to wallets/ MUST route to Sync")

	// A path that traverses ABOVE Root (after stripping root prefix
	// the result would resolve to "../..") MUST also fail-safe to
	// Sync rather than silently match nothing.
	abovetraverse := core.PathJoin(root, "..", "..", "etc", "passwd")
	core.AssertEqual(t, paths.AuditModeSync,
		paths.AuditModeForPath(abovetraverse),
		"above-Root traversal MUST fail-safe to Sync")
}

// TestAuditModeForPath_CaseFoldOnAPFS_Ugly — Cerberus DREAD-r2 F4
// (Mantis #1528). On case-insensitive filesystems (APFS / NTFS)
// case-mangled prefixes resolve to the same on-disk inode; the
// audit-mode routing MUST treat them identically to the canonical
// lowercase form.
func TestAuditModeForPath_CaseFoldOnAPFS_Ugly(t *core.T) {
	homeFixture(t)
	// Force APFS detection regardless of actual test-host fstype.
	paths.SetRootFSTypeForTest("apfs")
	t.Cleanup(func() { paths.SetRootFSTypeForTest("") })

	root := paths.Root().Value.(string)
	// Case-mangled wallets prefix MUST still route to Sync on APFS.
	mangled := core.PathJoin(root, "Wallets", "server.key")
	core.AssertEqual(t, paths.AuditModeSync,
		paths.AuditModeForPath(mangled),
		"case-mangled wallets/ on APFS MUST route to Sync")

	mangled2 := core.PathJoin(root, "ACCOUNT", "abc", "private.key")
	core.AssertEqual(t, paths.AuditModeSync,
		paths.AuditModeForPath(mangled2),
		"case-mangled account/ on APFS MUST route to Sync")

	// Same input on a case-sensitive filesystem (ext4) MUST route
	// to the cascade mode — case-folding stays opt-in by fstype.
	paths.SetRootFSTypeForTest("ext4")
	got := paths.AuditModeForPath(mangled)
	core.AssertEqual(t, paths.AuditModeBatch, got,
		"case-mangled wallets/ on ext4 MUST stay case-sensitive (Batch)")
}

// TestEmitLockEvent_DroppedWhenSecretUnavailable_Bad — Cerberus
// DREAD-r2 F2 (Mantis #1526). When the HKDF audit secret is
// unavailable the emit site MUST drop the event (the empty
// PathHash would be a side-channel leak); the dropped count MUST
// land in auditDegradedCount instead.
func TestEmitLockEvent_DroppedWhenSecretUnavailable_Bad(t *core.T) {
	homeFixture(t)
	rec := &captureRecorder{}
	paths.SetAuditRecorder(rec)
	// Explicitly leave secret provider returning empty.
	paths.SetAuditSecretProvider(func() []byte { return nil })
	t.Cleanup(func() {
		paths.SetAuditRecorder(nil)
		paths.SetAuditSecretProvider(nil)
	})

	before := paths.AuditDegradedCount()

	root := paths.Root().Value.(string)
	fp := core.PathJoin(root, "office", "documents", "x.md")
	_ = core.MkdirAll(core.PathJoin(root, "office", "documents"), 0o700)
	r := paths.AtomicWriteWithVersion(fp, paths.WriteInput{Body: []byte("x")})
	core.AssertTrue(t, r.OK, r.Error())

	// Recorder MUST NOT have seen any events.
	core.AssertEqual(t, 0, len(rec.snapshot()),
		"emit site must drop events when secret is unavailable")
	// Counter MUST have advanced.
	after := paths.AuditDegradedCount()
	core.AssertGreater(t, after, before,
		"auditDegradedCount must record the dropped event")
}

// TestFlushDegradedCount_EmitsSummary_Good — Cerberus DREAD-r2 F2
// (Mantis #1526). Once the secret comes back online, a flush call
// surfaces the accumulated count as a single summary event so the
// audit log is honest about the gap.
func TestFlushDegradedCount_EmitsSummary_Good(t *core.T) {
	homeFixture(t)
	rec := &captureRecorder{}
	paths.SetAuditRecorder(rec)
	// Start with no secret; drive a write to bump the counter.
	paths.SetAuditSecretProvider(func() []byte { return nil })
	t.Cleanup(func() {
		paths.SetAuditRecorder(nil)
		paths.SetAuditSecretProvider(nil)
	})

	root := paths.Root().Value.(string)
	_ = core.MkdirAll(core.PathJoin(root, "office", "documents"), 0o700)
	fp := core.PathJoin(root, "office", "documents", "y.md")
	r := paths.AtomicWriteWithVersion(fp, paths.WriteInput{Body: []byte("y")})
	core.AssertTrue(t, r.OK, r.Error())

	// Flush with secret still empty — no-op, counter preserved.
	noop := paths.FlushDegradedCount()
	core.AssertEqual(t, int64(0), noop, "flush is a no-op while secret is empty")
	core.AssertGreater(t, paths.AuditDegradedCount(), int64(0),
		"counter must survive a no-op flush")

	// Secret comes online; flush MUST emit one summary event and
	// reset the counter.
	paths.SetAuditSecretProvider(func() []byte {
		return []byte("test-secret-32-bytes-1234567890ab")
	})
	count := paths.FlushDegradedCount()
	core.AssertGreater(t, count, int64(0),
		"flush must report the accumulated drop count")
	core.AssertEqual(t, int64(0), paths.AuditDegradedCount(),
		"counter must reset after a flushing call")

	// Recorder MUST have seen exactly one summary event.
	summaryCount := 0
	for _, ev := range rec.snapshot() {
		if ev.Kind == paths.EventAuditDegradedSummary {
			summaryCount++
			core.AssertEqual(t, paths.AuditModeSync, ev.Mode,
				"degraded summary MUST route via Sync recorder")
		}
	}
	core.AssertEqual(t, 1, summaryCount,
		"flush must emit exactly one summary event")
}

var _ = testing.AllocsPerRun
