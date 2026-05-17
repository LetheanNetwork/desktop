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

// failingRecorder is a test-only AuditRecorder that always returns
// a Fail Result. Used by Mantis #1530 propagation coverage.
type failingRecorder struct{}

func (failingRecorder) RecordPathsEvent(_ paths.LockEvent) core.Result {
	return core.Fail(core.NewCode("test.recorder.injected",
		"injected recorder failure"))
}

// TestEmitLockEvent_SyncFailurePropagates_Ugly — Mantis #1530.
// A Sync-mode audit recorder failure (auth-substrate path) MUST
// propagate to the AtomicWriteWithVersion caller carrying the typed
// CodeAuditSyncRecordFailed sentinel. Without this, an auth-
// substrate write that hits disk but loses its forensic record
// silently returns Ok — the audit guarantee that makes Sync mode
// load-bearing is broken without operator visibility.
func TestEmitLockEvent_SyncFailurePropagates_Ugly(t *core.T) {
	homeFixture(t)
	// Install a recorder that ALWAYS fails so every emission returns
	// the injected Fail to emitLockEvent.
	paths.SetAuditRecorder(failingRecorder{})
	paths.SetAuditSecretProvider(func() []byte {
		return []byte("test-secret-32-bytes-1234567890ab")
	})
	t.Cleanup(func() {
		paths.SetAuditRecorder(nil)
		paths.SetAuditSecretProvider(nil)
	})

	// Sync-mode path: wallets/server.key — auth-substrate per the
	// AuditModeForPath policy table.
	walletDir := paths.WalletsDir().Value.(string)
	fp := core.PathJoin(walletDir, "server.key")
	r := paths.AtomicWriteWithVersion(fp, paths.WriteInput{
		Body: []byte("cipher"),
	})
	core.AssertFalse(t, r.OK,
		"Sync-mode audit recorder failure MUST propagate to caller")
	core.AssertContains(t, r.Error(), paths.CodeAuditSyncRecordFailed,
		"Failure MUST carry the typed CodeAuditSyncRecordFailed sentinel")
}

// TestEmitLockEvent_BatchFailureSilentContinue_Good — Mantis #1530.
// A Batch-mode audit recorder failure (cascade path) MUST be silently
// swallowed so cascade writes maintain their throughput contract.
// The recorder's Fail must not cascade into an AtomicWriteWithVersion
// caller-visible failure.
func TestEmitLockEvent_BatchFailureSilentContinue_Good(t *core.T) {
	homeFixture(t)
	paths.SetAuditRecorder(failingRecorder{})
	paths.SetAuditSecretProvider(func() []byte {
		return []byte("test-secret-32-bytes-1234567890ab")
	})
	t.Cleanup(func() {
		paths.SetAuditRecorder(nil)
		paths.SetAuditSecretProvider(nil)
	})

	root := paths.Root().Value.(string)
	// office/documents/ is a cascade path per the policy table.
	dir := core.PathJoin(root, "office", "documents")
	core.AssertTrue(t, core.MkdirAll(dir, 0o700).OK)
	fp := core.PathJoin(dir, "notes.md")

	r := paths.AtomicWriteWithVersion(fp, paths.WriteInput{
		Body: []byte("body"),
	})
	core.AssertTrue(t, r.OK,
		"Batch-mode audit failure MUST NOT cascade to caller; write contract is best-effort there")
}

// TestEmitLockEvent_SyncPropagatesOnAppendLine_Ugly — Mantis #1530.
// AtomicAppendLine shares the Sync-mode propagation contract: an
// append into an auth-substrate path whose audit recorder fails MUST
// surface the typed sentinel rather than silent-Ok.
func TestEmitLockEvent_SyncPropagatesOnAppendLine_Ugly(t *core.T) {
	homeFixture(t)
	paths.SetAuditRecorder(failingRecorder{})
	paths.SetAuditSecretProvider(func() []byte {
		return []byte("test-secret-32-bytes-1234567890ab")
	})
	t.Cleanup(func() {
		paths.SetAuditRecorder(nil)
		paths.SetAuditSecretProvider(nil)
	})

	// account/<id>/append.log — Sync per the auth-substrate prefix.
	root := paths.Root().Value.(string)
	dir := core.PathJoin(root, "account", "abc")
	core.AssertTrue(t, core.MkdirAll(dir, 0o700).OK)
	fp := core.PathJoin(dir, "audit.log")

	r := paths.AtomicAppendLine(fp, []byte("a-line"))
	core.AssertFalse(t, r.OK,
		"AppendLine Sync-mode audit failure MUST propagate")
	core.AssertContains(t, r.Error(), paths.CodeAuditSyncRecordFailed)
}

// TestHashForAudit_MatchesInternalEmission_Good — Mantis #1564.
// The exported HashForAudit MUST produce the same digest as the
// substrate's internal emit-site path-hash so cross-package audit
// emitters (e.g. office/mail fallback events) correlate with
// LockEvent path_hash entries under the same per-account domain key.
//
// Parity is observed by emitting a LockEvent at a known cascade path,
// capturing the recorder's hash, and asserting HashForAudit returns
// the same value for the same input path. The internal hashPath
// helper is package-private so the test pivots on emission output
// rather than calling the unexported function directly from _test.
func TestHashForAudit_MatchesInternalEmission_Good(t *core.T) {
	homeFixture(t)
	rec := withCaptureRecorder(t)

	root := paths.Root().Value.(string)
	dir := core.PathJoin(root, "office", "documents")
	core.AssertTrue(t, core.MkdirAll(dir, 0o700).OK)
	fp := core.PathJoin(dir, "parity.md")

	r := paths.AtomicWriteWithVersion(fp, paths.WriteInput{Body: []byte("p")})
	core.AssertTrue(t, r.OK, r.Error())

	internalHash := ""
	for _, ev := range rec.snapshot() {
		if ev.Kind == paths.EventWriteSucceeded {
			internalHash = ev.PathHash
		}
	}
	core.AssertNotEqual(t, "", internalHash,
		"emission MUST produce a non-empty PathHash with secret installed")
	core.AssertEqual(t, 32, len(internalHash),
		"emission PathHash MUST be 32 hex chars")

	exported := paths.HashForAudit(fp)
	core.AssertEqual(t, internalHash, exported,
		"HashForAudit MUST equal the substrate emit-site digest for the same path")
	core.AssertEqual(t, 32, len(exported),
		"HashForAudit MUST return 32 hex chars when secret is available")
}

// TestHashForAudit_EmptyWhenSecretUnavailable_Bad — Mantis #1564.
// Pre-boot / session-locked state (audit secret provider returns nil)
// MUST surface as an empty string so cross-package emitters can detect
// the degraded window and either drop the event (per #1526) or emit
// with an empty path_hash flag.
func TestHashForAudit_EmptyWhenSecretUnavailable_Bad(t *core.T) {
	homeFixture(t)
	paths.SetAuditSecretProvider(func() []byte { return nil })
	t.Cleanup(func() { paths.SetAuditSecretProvider(nil) })

	got := paths.HashForAudit("/any/path/here.md")
	core.AssertEqual(t, "", got,
		"HashForAudit MUST return empty when audit secret is unavailable")
}

var _ = testing.AllocsPerRun
