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
	t.Cleanup(func() { paths.SetAuditRecorder(nil) })
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

var _ = testing.AllocsPerRun
