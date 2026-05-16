// SPDX-Licence-Identifier: EUPL-1.2

// paths_audit_adapter_test.go — Mantis #1521 boot wire verification.
//
// Exercises pathsAuditAdapter end-to-end: build a real audit.Service
// rooted under a temp HOME, install the adapter via paths.SetAuditRecorder,
// trigger an AtomicWriteWithVersion, and assert the day-file gained an
// NDJSON record carrying the expected event-name + outcome + mode-
// derived Meta. Bypasses newAppCore so the test isn't gated on
// unrelated cmd/lthn package wiring (firstlaunch / sessions / bridge
// imports that are independently in-flight on dev).
//
// AX-1: predictable name TestPathsAuditAdapter_<Function>_<Good|Bad|Ugly>.
// AX-6: every I/O via core.* — no os / encoding/json / strings.

package main

import (
	"testing"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/audit"
	"dappco.re/lthn/desktop/pkg/paths"
)

// homeForAdapter rebinds $HOME to a t-scoped temp dir so the audit
// Service + paths.AtomicWriteWithVersion land under an isolated root.
// Matches the pkg/paths homeFixture pattern (internal package, no
// import path to share).
func homeForAdapter(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	return tmp
}

// TestPathsAuditAdapter_RecordPathsEvent_Good_BatchPathLandsInDayFile
// confirms the cascade-path emission flows through RecordBatch and
// lands as an NDJSON record in the audit day-file.
func TestPathsAuditAdapter_RecordPathsEvent_Good_BatchPathLandsInDayFile(t *testing.T) {
	home := homeForAdapter(t)
	_ = home

	// Build the audit Service with a deterministic secret so the run
	// is reproducible. Process-local random fallback also works for
	// the assertion path but a fixed secret keeps the test stable
	// across reruns.
	auditSvc := audit.New(nil, audit.Options{AuditSecret: []byte("test-secret-32-bytes-padding-xx!")})
	t.Cleanup(func() { _ = auditSvc.Close() })
	audit.SetDefault(auditSvc)
	t.Cleanup(func() { audit.SetDefault(nil) })

	// Install the adapter under test + the secret provider so the
	// HKDF path-hash derivation lands a non-empty path_hash.
	paths.SetAuditRecorder(&pathsAuditAdapter{svc: auditSvc})
	t.Cleanup(func() { paths.SetAuditRecorder(nil) })
	paths.SetAuditSecretProvider(func() []byte { return []byte("test-secret-32-bytes-padding-xx!") })
	t.Cleanup(func() { paths.SetAuditSecretProvider(nil) })

	// Trigger a cascade-mode write (office/documents falls under the
	// AuditModeBatch branch per AuditModeForPath's §6.1 policy).
	root := paths.Root().Value.(string)
	dir := core.PathJoin(root, "office", "documents")
	_ = core.MkdirAll(dir, 0o700)
	fp := core.PathJoin(dir, "smoke.md")
	wr := paths.AtomicWriteWithVersion(fp, paths.WriteInput{Body: []byte("hello")})
	core.AssertTrue(t, wr.OK, "AtomicWriteWithVersion must succeed")

	// Force the batched write to flush so the day-file contains the
	// line before our read-back. audit.Service.Close fsyncs.
	core.AssertTrue(t, auditSvc.Close().OK, "audit close must succeed")

	// Day-file path follows audit.Service's dateStem helper (private);
	// glob the audit dir for any *.log file produced this run instead
	// of duplicating the timestamp math.
	auditDir := core.PathJoin(root, "audit")
	entriesR := core.ReadDir(core.DirFS(auditDir), ".")
	core.AssertTrue(t, entriesR.OK, "audit dir must be readable")
	entries := entriesR.Value.([]core.FsDirEntry)
	core.AssertGreater(t, len(entries), 0, "audit day-file must exist after a batch write")

	var dayFile string
	for _, e := range entries {
		if !e.IsDir() {
			dayFile = core.PathJoin(auditDir, e.Name())
			break
		}
	}
	core.AssertNotEqual(t, "", dayFile, "audit day-file must be discoverable")

	body := core.ReadFile(dayFile)
	core.AssertTrue(t, body.OK, "day-file must be readable")
	contents := string(body.Value.([]byte))
	core.AssertTrue(t, core.Contains(contents, paths.EventWriteSucceeded),
		"day-file must carry the paths.write.succeeded event-name; got: "+contents)
	core.AssertTrue(t, core.Contains(contents, `"outcome":"ok"`),
		"day-file event must carry outcome=ok; got: "+contents)
	core.AssertTrue(t, core.Contains(contents, `"path_hash":"`),
		"day-file event must carry a non-empty path_hash field; got: "+contents)
}

// TestPathsAuditAdapter_RecordPathsEvent_Good_SyncPathLandsInDayFile
// confirms the auth-substrate emission flows through RecordSync. Same
// day-file shape as the batch case but the fsync happens per-event so
// no Close-flush is needed.
func TestPathsAuditAdapter_RecordPathsEvent_Good_SyncPathLandsInDayFile(t *testing.T) {
	homeForAdapter(t)

	auditSvc := audit.New(nil, audit.Options{AuditSecret: []byte("test-secret-32-bytes-padding-xx!")})
	t.Cleanup(func() { _ = auditSvc.Close() })
	audit.SetDefault(auditSvc)
	t.Cleanup(func() { audit.SetDefault(nil) })

	paths.SetAuditRecorder(&pathsAuditAdapter{svc: auditSvc})
	t.Cleanup(func() { paths.SetAuditRecorder(nil) })
	paths.SetAuditSecretProvider(func() []byte { return []byte("test-secret-32-bytes-padding-xx!") })
	t.Cleanup(func() { paths.SetAuditSecretProvider(nil) })

	// wallets/ falls under the AuditModeSync branch.
	walletDir := paths.WalletsDir().Value.(string)
	fp := core.PathJoin(walletDir, "auth-substrate-smoke.key")
	wr := paths.AtomicWriteWithVersion(fp, paths.WriteInput{Body: []byte("cipher")})
	core.AssertTrue(t, wr.OK, "AtomicWriteWithVersion must succeed")

	auditDir := core.PathJoin(paths.Root().Value.(string), "audit")
	entriesR := core.ReadDir(core.DirFS(auditDir), ".")
	core.AssertTrue(t, entriesR.OK, "audit dir must be readable")
	entries := entriesR.Value.([]core.FsDirEntry)
	core.AssertGreater(t, len(entries), 0, "audit day-file must exist after a sync write")

	var dayFile string
	for _, e := range entries {
		if !e.IsDir() {
			dayFile = core.PathJoin(auditDir, e.Name())
			break
		}
	}
	body := core.ReadFile(dayFile)
	core.AssertTrue(t, body.OK, "day-file must be readable")
	contents := string(body.Value.([]byte))
	core.AssertTrue(t, core.Contains(contents, paths.EventWriteSucceeded),
		"day-file must carry the paths.write.succeeded event-name; got: "+contents)
	core.AssertTrue(t, core.Contains(contents, `"outcome":"ok"`),
		"day-file event must carry outcome=ok; got: "+contents)
}

// TestFlushDegradedCount_CalledAtBootWire_Good — Mantis #1533.
// The boot wire in app.go invokes paths.FlushDegradedCount() right
// after SetAuditSecretProvider so any LockEvents dropped during the
// pre-secret window are summarised into a single audit event rather
// than left stranded in the auditDegradedCount counter. This test
// reproduces that wiring sequence in miniature: pre-secret emission
// drops + increments the counter; flushing with the secret + adapter
// installed drains the counter to zero and lands the summary event
// in the audit day-file.
func TestFlushDegradedCount_CalledAtBootWire_Good(t *testing.T) {
	homeForAdapter(t)

	// 1. Pre-secret window — no SetAuditSecretProvider yet. An
	// AtomicWriteWithVersion under wallets/ (AuditModeSync) emits a
	// LockEvent that the emit-site drops because currentAuditSecret()
	// is empty; auditDegradedCount increments. Capture before/after
	// so the test stays robust to other tests in the same process
	// having left a non-zero baseline on the package-global counter.
	// (Provider cleanup in sibling tests means we should see zero
	// at entry, but the delta assertion doesn't depend on that.)
	before := paths.AuditDegradedCount()
	walletDir := paths.WalletsDir().Value.(string)
	fp := core.PathJoin(walletDir, "pre-secret.key")
	wr := paths.AtomicWriteWithVersion(fp, paths.WriteInput{Body: []byte("v1")})
	core.AssertTrue(t, wr.OK, "AtomicWriteWithVersion must succeed even when audit is degraded")
	pending := paths.AuditDegradedCount()
	core.AssertGreater(t, pending, before,
		"pre-secret write must increment auditDegradedCount")

	// 2. Boot wire — install audit Service, recorder adapter, secret
	// provider. Mirrors the app.go sequence at lines 366-392.
	auditSvc := audit.New(nil, audit.Options{AuditSecret: []byte("test-secret-32-bytes-padding-xx!")})
	t.Cleanup(func() { _ = auditSvc.Close() })
	audit.SetDefault(auditSvc)
	t.Cleanup(func() { audit.SetDefault(nil) })

	paths.SetAuditRecorder(&pathsAuditAdapter{svc: auditSvc})
	t.Cleanup(func() { paths.SetAuditRecorder(nil) })
	paths.SetAuditSecretProvider(func() []byte { return []byte("test-secret-32-bytes-padding-xx!") })
	t.Cleanup(func() { paths.SetAuditSecretProvider(nil) })

	// 3. Flush — the boot-wire call. Returns the count it drained.
	flushed := paths.FlushDegradedCount()
	core.AssertEqual(t, pending, flushed,
		"FlushDegradedCount must drain the pre-secret pending count")
	core.AssertEqual(t, int64(0), paths.AuditDegradedCount(),
		"auditDegradedCount must be zero after a successful flush")

	// 4. Verify the summary event landed in the audit day-file.
	// FlushDegradedCount routes through RecordPathsEvent → RecordSync
	// (AuditModeSync) so no auditSvc.Close flush is needed.
	auditDir := core.PathJoin(paths.Root().Value.(string), "audit")
	entriesR := core.ReadDir(core.DirFS(auditDir), ".")
	core.AssertTrue(t, entriesR.OK, "audit dir must be readable post-flush")
	entries := entriesR.Value.([]core.FsDirEntry)
	core.AssertGreater(t, len(entries), 0, "audit day-file must exist post-flush")

	var dayFile string
	for _, e := range entries {
		if !e.IsDir() {
			dayFile = core.PathJoin(auditDir, e.Name())
			break
		}
	}
	body := core.ReadFile(dayFile)
	core.AssertTrue(t, body.OK, "day-file must be readable")
	contents := string(body.Value.([]byte))
	core.AssertTrue(t, core.Contains(contents, paths.EventAuditDegradedSummary),
		"day-file must carry the paths.audit.degraded_summary event-name; got: "+contents)
}

// TestFlushDegradedCount_NoOpWhenCounterZero_Good — calling the
// boot wire on a clean process (no dropped events) is a no-op. The
// app.go wire's `if n > 0 { core.Print(...) }` guard relies on the
// returned count being zero in this case so cold boots stay quiet.
func TestFlushDegradedCount_NoOpWhenCounterZero_Good(t *testing.T) {
	homeForAdapter(t)

	auditSvc := audit.New(nil, audit.Options{AuditSecret: []byte("test-secret-32-bytes-padding-xx!")})
	t.Cleanup(func() { _ = auditSvc.Close() })
	audit.SetDefault(auditSvc)
	t.Cleanup(func() { audit.SetDefault(nil) })

	paths.SetAuditRecorder(&pathsAuditAdapter{svc: auditSvc})
	t.Cleanup(func() { paths.SetAuditRecorder(nil) })
	paths.SetAuditSecretProvider(func() []byte { return []byte("test-secret-32-bytes-padding-xx!") })
	t.Cleanup(func() { paths.SetAuditSecretProvider(nil) })

	// Counter is zero — no pre-secret drops happened in this test.
	core.AssertEqual(t, int64(0), paths.AuditDegradedCount(),
		"counter must start at zero for the no-op assertion")
	flushed := paths.FlushDegradedCount()
	core.AssertEqual(t, int64(0), flushed,
		"FlushDegradedCount must return 0 when there's nothing to flush")
}

// TestPathsAuditAdapter_PathsEventOutcome_Ugly_KindToOutcomeMapping
// pins the closed-set Kind→Outcome map so a future event-schema
// addition fails this test loud rather than silently rejecting at
// audit.Service.recordCommon (empty Outcome → codeAuditEventInvalid).
func TestPathsAuditAdapter_PathsEventOutcome_Ugly_KindToOutcomeMapping(t *testing.T) {
	cases := []struct {
		kind string
		want string
	}{
		{paths.EventWriteSucceeded, audit.OutcomeOK},
		{paths.EventLockAcquired, audit.OutcomeOK},
		{paths.EventLockReleased, audit.OutcomeOK},
		{paths.EventVersionStale, audit.OutcomeFailed},
	}
	for _, tc := range cases {
		got := pathsEventOutcome(tc.kind)
		core.AssertEqual(t, tc.want, got)
	}
}
