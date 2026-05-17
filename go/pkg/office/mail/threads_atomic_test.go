// SPDX-Licence-Identifier: EUPL-1.2

// threads_atomic_test.go — cascade W4 Part 2 cutover tests for the
// threads.md adoption of paths.AtomicAppendLine + Darwin PIPE_BUF
// fallback (Cerberus #9 Concern 1.A) + per-folder Mutex
// (Concern 1.B/1.C) + per-account FetchOnce single-flight
// (Mantis #1555).

package mail

import (
	"sync"
	"testing"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/audit"
	"dappco.re/lthn/desktop/pkg/paths"
)

// makeRec is a tiny helper for synthesising thread records in tests.
//
// Usage example:
//
//	rec := makeRec("id-1", "Re: subject")
func makeRec(id, subj string) MailThreadRecord {
	return MailThreadRecord{
		ID:          id,
		From:        "Ada Penley",
		Subj:        subj,
		LastTouched: core.Now().UTC(),
		Unread:      true,
		Snippet:     "snippet body",
	}
}

// TestMail_AppendLine_SmallRecordPrimaryPath_Good — small records
// land via AtomicAppendLine (primary path) without tripping the
// fallback. Linux PipeBufLimit is 4096, so the YAML-marshalled
// record fits comfortably; on Darwin PipeBufLimit is 512 and a
// short subject still fits.
func TestMail_AppendLine_SmallRecordPrimaryPath_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	c := core.New()
	svc := NewService(c)

	// Use a generous limit so even Darwin's 512 default does not
	// trip the fallback on this short record. Pinning ensures the
	// primary path is exercised regardless of host PIPE_BUF.
	paths.SetPipeBufLimitForTest(4096)
	t.Cleanup(func() { paths.SetPipeBufLimitForTest(0) })

	if err := svc.appendThreadRecord("inbox", makeRec("id-1", "Hi")); err != nil {
		t.Fatalf("appendThreadRecord: %v", err)
	}

	// Confirm the record landed.
	tp := threadsFilePath("inbox")
	if !tp.OK {
		t.Fatalf("threadsFilePath: %s", tp.Error())
	}
	raw, err := readFileBytes(tp.Value.(string))
	if err != nil {
		t.Fatalf("read threads.md: %v", err)
	}
	if !core.Contains(string(raw), "id: id-1") {
		t.Fatalf("threads.md missing record: %s", string(raw))
	}
}

// TestMail_AppendLine_DarwinPipeBufFallback_Ugly — Cerberus #9
// Concern 1.A LOAD-BEARING test. Pin the fallback path: when a
// record exceeds PipeBufLimit() AtomicAppendLine rejects with
// CodeAppendRecordTooLarge and appendThreadRecord falls back to
// AtomicWriteWithVersion rewrite-merge. The record MUST still land
// in threads.md, AND a paths.append.fell_back_to_rewrite MailEvent
// MUST fire.
//
// Reproduction: drop PipeBufLimit to 64 bytes so even a tiny YAML
// record + framing exceeds it. Record body is irrelevant — fallback
// engages on size alone.
func TestMail_AppendLine_DarwinPipeBufFallback_Ugly(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	c := core.New()
	svc := NewService(c)

	// Force every record to exceed the limit.
	paths.SetPipeBufLimitForTest(64)
	t.Cleanup(func() { paths.SetPipeBufLimitForTest(0) })

	// Subscribe to the typed MailEvent bus so we can pin the
	// fallback-emission contract.
	var saw []MailEvent
	var mu sync.Mutex
	Subscribe(c, func(_ *core.Core, ev MailEvent) {
		mu.Lock()
		saw = append(saw, ev)
		mu.Unlock()
	})

	rec := makeRec("fallback-1",
		"Re: Re: Re: subject longer than 64 bytes to force fallback path engagement on every host")
	if err := svc.appendThreadRecord("inbox", rec); err != nil {
		t.Fatalf("appendThreadRecord MUST succeed via fallback: %v", err)
	}

	// (a) record landed durably
	tp := threadsFilePath("inbox")
	if !tp.OK {
		t.Fatalf("threadsFilePath: %s", tp.Error())
	}
	raw, rerr := readFileBytes(tp.Value.(string))
	if rerr != nil {
		t.Fatalf("read threads.md: %v", rerr)
	}
	if !core.Contains(string(raw), "id: fallback-1") {
		t.Fatalf("fallback rewrite-merge MUST land the record; threads.md=%s", string(raw))
	}

	// (b) fallback audit event fired with the typed const + Meta
	// keys per Mantis #1559 (path_hash + record_size_bytes).
	mu.Lock()
	defer mu.Unlock()
	found := false
	for _, ev := range saw {
		if ev.Kind == EventPathsAppendFellBackToRewrite {
			found = true
			if ev.FolderSlug != "inbox" {
				t.Fatalf("fallback event folder: got %q want inbox", ev.FolderSlug)
			}
			if ev.Meta == nil {
				t.Fatalf("fallback event MUST carry Meta map per Mantis #1559; ev=%+v", ev)
			}
			if ph, _ := ev.Meta["path_hash"].(string); ph == "" {
				t.Fatalf("fallback event Meta[path_hash] MUST be non-empty per Mantis #1559; meta=%+v", ev.Meta)
			}
			rsRaw, ok := ev.Meta["record_size_bytes"]
			if !ok {
				t.Fatalf("fallback event Meta[record_size_bytes] MUST be present per Mantis #1559; meta=%+v", ev.Meta)
			}
			if rs, ok := rsRaw.(int); !ok || rs <= 0 {
				t.Fatalf("fallback event Meta[record_size_bytes] MUST be positive int; got %T=%v", rsRaw, rsRaw)
			}
			break
		}
	}
	if !found {
		t.Fatalf("expected %s event; saw=%v", EventPathsAppendFellBackToRewrite, saw)
	}
}

// TestMail_AppendLine_FallbackPreservesExisting_Good — when the
// fallback rewrite-merge engages, existing records in threads.md
// MUST survive. Reproduction: seed the file via primary-path with
// limit=4096, then drop limit to 64 and append a second record via
// fallback; assert BOTH records present.
func TestMail_AppendLine_FallbackPreservesExisting_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	c := core.New()
	svc := NewService(c)

	// First record via primary path.
	paths.SetPipeBufLimitForTest(4096)
	if err := svc.appendThreadRecord("inbox", makeRec("first", "First")); err != nil {
		t.Fatalf("seed first: %v", err)
	}

	// Second record via fallback.
	paths.SetPipeBufLimitForTest(64)
	t.Cleanup(func() { paths.SetPipeBufLimitForTest(0) })
	if err := svc.appendThreadRecord("inbox",
		makeRec("second", "Second record body padded for fallback engagement")); err != nil {
		t.Fatalf("fallback append: %v", err)
	}

	tp := threadsFilePath("inbox")
	if !tp.OK {
		t.Fatalf("threadsFilePath: %s", tp.Error())
	}
	raw, _ := readFileBytes(tp.Value.(string))
	if !core.Contains(string(raw), "id: first") {
		t.Fatalf("first record lost on fallback merge: %s", string(raw))
	}
	if !core.Contains(string(raw), "id: second") {
		t.Fatalf("second record missing post-fallback: %s", string(raw))
	}
}

// TestMail_AppendLine_ConcurrentSameFolder_Good — Cerberus #9
// Concern 1.B/1.C: two goroutines call appendThreadRecord on the
// same folder concurrently; folder-mutex serialises so the
// rotation race window never opens, and BOTH records land in
// threads.md (or the rotated archive — none lost).
func TestMail_AppendLine_ConcurrentSameFolder_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	c := core.New()
	svc := NewService(c)

	// Keep limit generous so the primary path runs (faster). The
	// folder-mutex sits at appendThreadRecord regardless of primary
	// vs fallback path.
	paths.SetPipeBufLimitForTest(4096)
	t.Cleanup(func() { paths.SetPipeBufLimitForTest(0) })

	// Push rotation threshold tiny enough that the second append
	// crosses it — pins the rotation-race defence-in-depth contract.
	paths.SetAppendRotateThresholdForTest(8)
	t.Cleanup(func() { paths.SetAppendRotateThresholdForTest(paths.AppendRotateThreshold) })

	var wg sync.WaitGroup
	wg.Add(2)
	start := make(chan struct{})
	errs := make([]error, 2)
	go func() {
		defer wg.Done()
		<-start
		errs[0] = svc.appendThreadRecord("inbox", makeRec("concurrent-A", "A"))
	}()
	go func() {
		defer wg.Done()
		<-start
		errs[1] = svc.appendThreadRecord("inbox", makeRec("concurrent-B", "B"))
	}()
	close(start)
	wg.Wait()
	for i, e := range errs {
		if e != nil {
			t.Fatalf("appendThreadRecord[%d]: %v", i, e)
		}
	}

	// Records may now live in current OR a rotated archive — sum
	// the lot and assert both present.
	tp := threadsFilePath("inbox")
	if !tp.OK {
		t.Fatalf("threadsFilePath: %s", tp.Error())
	}
	threadsPath := tp.Value.(string)
	all, err := readFileBytes(threadsPath)
	if err != nil {
		// Possible the file got rotated out before we read; treat
		// missing as empty and rely on archive scan below.
		all = nil
	}
	combined := string(all)

	// Walk parent dir for *.archived siblings.
	parent := core.PathBase(threadsPath)
	dirR := core.PathDir(threadsPath)
	_ = parent
	entriesR := core.ReadDir(core.DirFS(dirR), ".")
	if entriesR.OK {
		entries := entriesR.Value.([]core.FsDirEntry)
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			if !core.Contains(name, ".archived") {
				continue
			}
			arPath := core.PathJoin(dirR, name)
			if raw, rerr := readFileBytes(arPath); rerr == nil {
				combined += "\n" + string(raw)
			}
		}
	}

	if !core.Contains(combined, "id: concurrent-A") {
		t.Fatalf("concurrent record A lost across current+archives: %s", combined)
	}
	if !core.Contains(combined, "id: concurrent-B") {
		t.Fatalf("concurrent record B lost across current+archives: %s", combined)
	}
}

// TestMail_Poll_SingleFlightPerAccount_Good — Mantis #1555. Two
// concurrent FetchOnce calls on the same account serialise on the
// per-account Mutex acquired at FetchOnce entry. Implementation
// asserts via a counter that the cheap-fail paths (which RUN AFTER
// the mutex) cannot interleave for the same account.
//
// The test exploits the cheap-fail "account not found" path — both
// goroutines pass input validation, both acquire the same mutex,
// both then fail at account-lookup (no _accounts.enc seeded). Use
// the lock-state to verify serial execution via a witness counter
// incremented under the per-account mutex.
//
// Cross-account calls MUST NOT block on each other — separate
// mutex map keys.
func TestMail_Poll_SingleFlightPerAccount_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	c := core.New()
	svc := NewService(c)
	ap := newTestAccountProvider(t)
	svc.SetAccountService(ap)

	// (1) Same-account: assert serial execution.
	// Acquire the per-account mutex from a witness goroutine to
	// PROVE that FetchOnce blocks behind it. The witness holds the
	// lock for a known minimum duration; if FetchOnce returned
	// before the witness released, single-flight is broken.
	m := svc.accountMutex("personal")
	m.Lock()

	done := make(chan struct{})
	go func() {
		// FetchOnce should block on the locked accountMutex until
		// the witness releases below. After release, it proceeds
		// to fail with mail.fetch.account_not_found (no account
		// seeded). Either way, the call returns AFTER the witness
		// releases.
		_ = svc.FetchOnce(FetchOnceInput{AccountName: "personal", FolderSlug: "inbox"})
		close(done)
	}()

	// Give the goroutine time to reach the mutex.
	core.Sleep(50 * core.Millisecond)
	select {
	case <-done:
		t.Fatal("FetchOnce returned BEFORE witness released the per-account mutex — single-flight broken")
	default:
		// expected — blocked on the mutex
	}
	m.Unlock()
	<-done // FetchOnce should now complete

	// (2) Cross-account: different account name MUST NOT block.
	mOther := svc.accountMutex("other")
	if mOther == m {
		t.Fatal("accountMutex returned the same Mutex for different account names — per-account isolation broken")
	}

	mOther.Lock()
	doneSelf := make(chan struct{})
	go func() {
		// "personal" mutex is free; this call should NOT block on
		// "other" being held.
		_ = svc.FetchOnce(FetchOnceInput{AccountName: "personal", FolderSlug: "inbox"})
		close(doneSelf)
	}()
	select {
	case <-doneSelf:
		// expected — call completed without blocking on the
		// unrelated "other" account's mutex
	case <-core.After(2 * core.Second):
		t.Fatal("FetchOnce for 'personal' blocked on 'other' account's mutex — per-account isolation broken")
	}
	mOther.Unlock()
}

// TestMail_Poll_SingleFlightInputValidation_Bad — cheap-fail input
// paths MUST NOT acquire the per-account mutex (empty account name,
// invalid folder slug). Otherwise a malformed input could hold the
// mutex briefly and reorder unrelated callers.
//
// The contract: input validation runs FIRST, mutex acquisition runs
// SECOND. Verified by holding the "personal" mutex and confirming
// that bad inputs return immediately rather than blocking.
func TestMail_Poll_SingleFlightInputValidation_Bad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	c := core.New()
	svc := NewService(c)
	ap := newTestAccountProvider(t)
	svc.SetAccountService(ap)

	// Hold the "personal" mutex; bad-input calls should still
	// return immediately because they fail BEFORE the mutex
	// acquisition.
	m := svc.accountMutex("personal")
	m.Lock()
	defer m.Unlock()

	// Empty account name — fails at input check.
	r := svc.FetchOnce(FetchOnceInput{AccountName: "", FolderSlug: "inbox"})
	if r.OK {
		t.Fatal("expected empty AccountName to fail")
	}

	// Invalid folder slug — fails at paths.IsValidID.
	r2 := svc.FetchOnce(FetchOnceInput{AccountName: "personal", FolderSlug: "../escape"})
	if r2.OK {
		t.Fatal("expected invalid FolderSlug to fail")
	}
}

// TestMail_Service_StopClearsFolderMu_Good — Mantis #1557 (Cerberus
// #11 LOW). folderMu and accountMu maps grow one entry per distinct
// folder path / account name observed for the lifetime of the
// Service. Stop() drains both maps + the pool + backoff counters in
// one pass so a long-running daemon cycled through Stop() at process
// shutdown does not leak the accumulated entries.
//
// The test populates folderMu via appendThreadRecord, accountMu via
// accountMutex(), backoff via backoffDelay(), then calls Stop() and
// asserts all four are empty.
func TestMail_Service_StopClearsFolderMu_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	c := core.New()
	svc := NewService(c)

	// Keep limit generous so the primary AppendLine path runs.
	paths.SetPipeBufLimitForTest(4096)
	t.Cleanup(func() { paths.SetPipeBufLimitForTest(0) })

	// Populate folderMu via appendThreadRecord on three folders.
	for _, slug := range []string{"inbox", "sent", "archive"} {
		if err := svc.appendThreadRecord(slug, makeRec("rec-"+slug, "subj")); err != nil {
			t.Fatalf("appendThreadRecord(%s): %v", slug, err)
		}
	}

	// Populate accountMu via accountMutex (the FetchOnce entry
	// point also populates it but requires a live AccountProvider —
	// hit the helper directly for the same effect).
	_ = svc.accountMutex("personal")
	_ = svc.accountMutex("work")

	// Populate backoff via backoffDelay (single tick advances the
	// counter and seeds the map entry).
	_ = svc.backoffDelay("personal/inbox")

	// Pre-state assertion — maps populated.
	svc.mu.Lock()
	if len(svc.folderMu) != 3 {
		svc.mu.Unlock()
		t.Fatalf("folderMu pre-state: got %d entries, want 3", len(svc.folderMu))
	}
	if len(svc.accountMu) != 2 {
		svc.mu.Unlock()
		t.Fatalf("accountMu pre-state: got %d entries, want 2", len(svc.accountMu))
	}
	if len(svc.backoff) != 1 {
		svc.mu.Unlock()
		t.Fatalf("backoff pre-state: got %d entries, want 1", len(svc.backoff))
	}
	svc.mu.Unlock()

	// Stop and assert all drained.
	if r := svc.Stop(core.Background()); !r.OK {
		t.Fatalf("Stop: %s", r.Error())
	}

	svc.mu.Lock()
	defer svc.mu.Unlock()
	if len(svc.folderMu) != 0 {
		t.Fatalf("folderMu post-Stop: got %d entries, want 0", len(svc.folderMu))
	}
	if len(svc.accountMu) != 0 {
		t.Fatalf("accountMu post-Stop: got %d entries, want 0", len(svc.accountMu))
	}
	if len(svc.pool) != 0 {
		t.Fatalf("pool post-Stop: got %d entries, want 0", len(svc.pool))
	}
	if len(svc.backoff) != 0 {
		t.Fatalf("backoff post-Stop: got %d entries, want 0", len(svc.backoff))
	}
}

// fakeAuditRecorder is a thread-safe in-memory Recorder for assertion
// in tests. Captures every Event passed to Record so the test can scan
// for cross-cascade emissions without spinning the NDJSON file sink.
//
// Usage example:
//
//	fr := &fakeAuditRecorder{}
//	audit.SetDefault(fr)
//	defer audit.SetDefault(nil)
//	// ... drive code that emits ...
//	fr.mu.Lock(); defer fr.mu.Unlock()
//	for _, ev := range fr.events { ... }
type fakeAuditRecorder struct {
	mu     sync.Mutex
	events []audit.Event
}

func (f *fakeAuditRecorder) Record(ev audit.Event) core.Result {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, ev)
	return core.Result{OK: true}
}

// TestMail_AppendLine_FallbackEmitsAuditEvent_Good — Mantis #1561.
// The W4 fallback path lands the fallback event via TWO channels:
// (a) the typed MailEvent bus (covered by FallbackUgly above), AND
// (b) the audit.Default() recorder so cross-cascade dashboards (Stage F
// log-tailer) see the event without a per-package bridge.
//
// Reproduction: install a fake audit Recorder, force the fallback via
// PipeBufLimit=64, append a record, assert the recorder received an
// audit.Event with Event=EventPathsAppendFellBackToRewrite + Meta
// carrying folder_slug + path_hash + record_size_bytes.
func TestMail_AppendLine_FallbackEmitsAuditEvent_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	c := core.New()
	svc := NewService(c)

	// Force fallback engagement.
	paths.SetPipeBufLimitForTest(64)
	t.Cleanup(func() { paths.SetPipeBufLimitForTest(0) })

	// Install fake recorder so we can assert RecordBatch landed the
	// event without spinning the NDJSON file sink.
	fr := &fakeAuditRecorder{}
	audit.SetDefault(fr)
	t.Cleanup(func() { audit.SetDefault(nil) })

	rec := makeRec("audit-cascade-1",
		"Re: Re: Re: subject longer than 64 bytes so the fallback engages on every host")
	if err := svc.appendThreadRecord("inbox", rec); err != nil {
		t.Fatalf("appendThreadRecord: %v", err)
	}

	fr.mu.Lock()
	defer fr.mu.Unlock()
	if len(fr.events) == 0 {
		t.Fatalf("expected at least one audit.Event from fallback path; got none")
	}
	var matched *audit.Event
	for i := range fr.events {
		if fr.events[i].Event == EventPathsAppendFellBackToRewrite {
			matched = &fr.events[i]
			break
		}
	}
	if matched == nil {
		t.Fatalf("expected audit.Event with Event=%s; got %d events: %+v",
			EventPathsAppendFellBackToRewrite, len(fr.events), fr.events)
	}
	if matched.Outcome != audit.OutcomeOK {
		t.Fatalf("fallback audit event Outcome: got %q want %q", matched.Outcome, audit.OutcomeOK)
	}
	if matched.Scope != "mail.append.fallback" {
		t.Fatalf("fallback audit event Scope: got %q want %q", matched.Scope, "mail.append.fallback")
	}
	if matched.Meta == nil {
		t.Fatalf("fallback audit event MUST carry Meta map; ev=%+v", matched)
	}
	if folder, _ := matched.Meta["folder_slug"].(string); folder != "inbox" {
		t.Fatalf("fallback audit event Meta[folder_slug]: got %q want inbox", folder)
	}
	if ph, _ := matched.Meta["path_hash"].(string); ph == "" {
		t.Fatalf("fallback audit event Meta[path_hash] MUST be non-empty; meta=%+v", matched.Meta)
	}
	if _, ok := matched.Meta["record_size_bytes"]; !ok {
		t.Fatalf("fallback audit event Meta[record_size_bytes] MUST be present; meta=%+v", matched.Meta)
	}
}
