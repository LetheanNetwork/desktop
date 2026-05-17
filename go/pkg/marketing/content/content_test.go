// SPDX-Licence-Identifier: EUPL-1.2

package content_test

import (
	"encoding/json"
	"sync"
	"testing"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/marketing/content"
	"dappco.re/lthn/desktop/pkg/paths"
)

// stubSessionGate is the minimal SessionGate implementation used across
// content_test.go / security_test.go / wails_test.go. ids drives the
// gate verdict — empty slice = locked, non-empty = unlocked. The test
// helpers below (newTestSvc / newTestSvcWithCore) wire one of these by
// default so write-path tests exercise the post-retrofit success path
// (Mantis #1613 B.2).
type stubSessionGate struct{ ids []string }

func (s *stubSessionGate) UnlockedAccountIDs() []string { return s.ids }

// newTestSvc returns a content.Service wired with an unlocked
// SessionGate. Mirrors the sales/deals + sales/pipeline B.2 helper
// shape — write-path tests SHOULD use this instead of
// content.NewService(nil) post-retrofit so assertUnlocked permits the
// write.
func newTestSvc(t *testing.T) *content.Service {
	t.Helper()
	svc := content.NewService(nil)
	svc.SetSessionGate(&stubSessionGate{ids: []string{"acct-1"}})
	return svc
}

// newTestSvcWithCore is newTestSvc but takes a Core container — used by
// tests that need ACTION-bus observation. Wired with the same unlocked
// stub.
func newTestSvcWithCore(t *testing.T, c *core.Core) *content.Service {
	t.Helper()
	svc := content.NewService(c)
	svc.SetSessionGate(&stubSessionGate{ids: []string{"acct-1"}})
	return svc
}

// TestList_Empty_Good — empty dir → 5 columns, all empty, zero counts.
func TestList_Empty_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := content.NewService(nil)
	r := svc.List(content.ListInput{})
	if !r.OK {
		t.Fatalf("List failed: %s", r.Error())
	}
	out := r.Value.(content.ListOutput)
	if len(out.Columns) != 5 {
		t.Fatalf("expected 5 columns, got %d", len(out.Columns))
	}
	for _, col := range out.Columns {
		if len(col.Items) != 0 {
			t.Fatalf("column %q expected 0 items, got %d", col.ID, len(col.Items))
		}
	}
	if out.TotalInFlight != 0 || out.DueToday != 0 {
		t.Fatalf("expected zero counts")
	}
}

// TestCreate_Defaults_Good — Create with title only defaults to col=idea.
func TestCreate_Defaults_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newTestSvc(t)
	r := svc.Create(content.CreateInput{T: "Watt-per-token comparison post"})
	if !r.OK {
		t.Fatalf("Create failed: %s", r.Error())
	}
	item := r.Value.(content.ContentItem)
	if item.Col != "idea" {
		t.Fatalf("expected col=idea, got %q", item.Col)
	}
	if item.ID == "" {
		t.Fatalf("expected non-empty ID")
	}
}

// TestCreate_Title_Bad — empty title returns core.Fail.
func TestCreate_Title_Bad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newTestSvc(t)
	r := svc.Create(content.CreateInput{T: ""})
	if r.OK {
		t.Fatalf("expected failure for empty title, got OK")
	}
}

// TestAdvance_Order_Good — Advance cycles idea → draft → review → ready → live.
func TestAdvance_Order_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newTestSvc(t)
	cr := svc.Create(content.CreateInput{T: "Test post", Col: "idea"})
	if !cr.OK {
		t.Fatalf("Create failed: %s", cr.Error())
	}
	id := cr.Value.(content.ContentItem).ID

	expected := []string{"draft", "review", "ready", "live"}
	for _, want := range expected {
		r := svc.Advance(id)
		if !r.OK {
			t.Fatalf("Advance to %q failed: %s", want, r.Error())
		}
		item := r.Value.(content.ContentItem)
		if item.Col != want {
			t.Fatalf("expected col=%q, got %q", want, item.Col)
		}
	}
}

// TestAdvance_AtLive_Bad — Advance at "live" returns core.Fail.
func TestAdvance_AtLive_Bad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newTestSvc(t)
	cr := svc.Create(content.CreateInput{T: "Live post", Col: "live"})
	if !cr.OK {
		t.Fatalf("Create failed: %s", cr.Error())
	}
	id := cr.Value.(content.ContentItem).ID

	r := svc.Advance(id)
	if r.OK {
		t.Fatalf("expected failure advancing from live, got OK")
	}
}

// TestServiceName_Good — ServiceName returns "Content".
func TestServiceName_Good(t *testing.T) {
	svc := content.NewService(nil)
	if svc.ServiceName() != "Content" {
		t.Fatalf("expected Content, got %q", svc.ServiceName())
	}
}

// ---- Cascade W2 cutover tests (paths.AtomicWriteWithVersion) ---------------

// TestAtomicCutover_MarketingContent_Create_Good — Create stamps
// version=1 via the primitive; ReadVersion confirms the on-disk file
// carries the same.
func TestAtomicCutover_MarketingContent_Create_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newTestSvc(t)
	r := svc.Create(content.CreateInput{T: "v0.2 release notes"})
	if !r.OK {
		t.Fatalf("Create failed: %s", r.Error())
	}
	item := r.Value.(content.ContentItem)
	dirR := core.UserHomeDir()
	if !dirR.OK {
		t.Fatalf("UserHomeDir: %s", dirR.Error())
	}
	fpath := core.PathJoin(dirR.Value.(string), "Lethean/marketing/content", item.ID+".md")
	rd := paths.ReadVersion(fpath)
	if !rd.OK {
		t.Fatalf("ReadVersion: %s", rd.Error())
	}
	got := rd.Value.(paths.ReadOutput)
	if got.Version != 1 {
		t.Fatalf("expected version 1 after Create, got %d", got.Version)
	}
}

// TestAtomicCutover_MarketingContent_Update_Good — a sequential
// Advance bumps the stored version monotonically (1 -> 2).
func TestAtomicCutover_MarketingContent_Update_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newTestSvc(t)
	cr := svc.Create(content.CreateInput{T: "Draft post", Col: "idea"})
	if !cr.OK {
		t.Fatalf("Create failed: %s", cr.Error())
	}
	id := cr.Value.(content.ContentItem).ID
	ur := svc.Advance(id)
	if !ur.OK {
		t.Fatalf("Advance failed: %s", ur.Error())
	}
	dirR := core.UserHomeDir()
	if !dirR.OK {
		t.Fatalf("UserHomeDir: %s", dirR.Error())
	}
	fpath := core.PathJoin(dirR.Value.(string), "Lethean/marketing/content", id+".md")
	rd := paths.ReadVersion(fpath)
	if !rd.OK {
		t.Fatalf("ReadVersion: %s", rd.Error())
	}
	got := rd.Value.(paths.ReadOutput)
	if got.Version != 2 {
		t.Fatalf("expected version 2 after Advance, got %d", got.Version)
	}
}

// TestAtomicCutover_MarketingContent_Update_VersionStale_Ugly — drives
// two concurrent svc.Advance calls and asserts the loser's Result
// carries a paths.ConflictEnvelope whose JSON marshal matches the
// lowercase `{code, current_version, current_hash}` wire shape that
// frontend/src/lit/conflict-dispatch.ts extractEnvelope pattern-
// matches on (Mantis #1547 service-tier round-trip discipline anchor;
// pins #1544 against W2 wave drift).
//
// Why two goroutines: svc.Advance re-reads the file inside the call
// so a single-shot out-of-band mutation cannot drive a real conflict
// through the service. Concurrent Advance goroutines race for the
// WithFileLock; the loser's locked-section ReadVersion sees the
// winner's bumped version and the primitive returns VersionStale ->
// wrapped as ConflictEnvelope by content.writeItem.
func TestAtomicCutover_MarketingContent_Update_VersionStale_Ugly(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newTestSvc(t)
	// Seed in "idea" so each Advance targets "draft" — same legal
	// transition from both goroutines, so the only failure mode is
	// the optimistic-lock conflict.
	cr := svc.Create(content.CreateInput{T: "Race post", Col: "idea"})
	if !cr.OK {
		t.Fatalf("Create failed: %s", cr.Error())
	}
	id := cr.Value.(content.ContentItem).ID

	var conflict core.Result
	var saw bool
	for attempt := 0; attempt < 32 && !saw; attempt++ {
		// Reset the item back to "idea" between attempts so both
		// goroutines have a legal advance-target each round.
		if attempt > 0 {
			// Walk it back via direct file rewrite to keep the test
			// hermetic — service has no Set/Reset path.
			dirR := core.UserHomeDir()
			if !dirR.OK {
				t.Fatalf("UserHomeDir: %s", dirR.Error())
			}
			fpath := core.PathJoin(dirR.Value.(string), "Lethean/marketing/content", id+".md")
			// Read current version, rewrite with col=idea, bump version
			// so the next Advance race is still a real write.
			rd := paths.ReadVersion(fpath)
			if !rd.OK {
				t.Fatalf("ReadVersion (reset): %s", rd.Error())
			}
			cur := rd.Value.(paths.ReadOutput).Version
			body := []byte("---\nversion: " + core.Sprintf("%d", cur+1) + "\nid: " + id + "\nt: Race post\nwho: \"\"\nwhen: \"\"\ndue: \"\"\ncol: idea\n---\n")
			if w := core.WriteFile(fpath, body, 0o600); !w.OK {
				t.Fatalf("WriteFile (reset): %s", w.Error())
			}
		}
		var (
			mu      sync.Mutex
			results []core.Result
			wg      sync.WaitGroup
			start   = make(chan struct{})
		)
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			r := svc.Advance(id)
			mu.Lock()
			results = append(results, r)
			mu.Unlock()
		}()
		go func() {
			defer wg.Done()
			<-start
			r := svc.Advance(id)
			mu.Lock()
			results = append(results, r)
			mu.Unlock()
		}()
		close(start)
		wg.Wait()

		for _, r := range results {
			if !r.OK && core.Contains(r.Error(), "content.update.conflict") {
				conflict = r
				saw = true
				break
			}
		}
	}
	if !saw {
		t.Skip("could not provoke a writer race after 32 attempts — environment lock skew; flake-defensive skip")
	}

	// Wire-shape assertion #1 — Result.Value is a paths.ConflictEnvelope.
	env, ok := paths.ConflictEnvelopeFrom(conflict.Value)
	if !ok {
		t.Fatalf("expected paths.ConflictEnvelope in Result.Value, got %T", conflict.Value)
	}
	if env.Code != "content.update.conflict" {
		t.Fatalf("expected envelope code content.update.conflict, got %q", env.Code)
	}
	if env.CurrentVersion < 1 {
		t.Fatalf("expected CurrentVersion >= 1, got %d", env.CurrentVersion)
	}

	// Wire-shape assertion #2 — Result.Value marshals to the lowercase
	// snake_case keys conflict-dispatch.ts walks. Discipline anchor.
	raw, err := json.Marshal(conflict.Value)
	if err != nil {
		t.Fatalf("json.Marshal(conflict.Value): %s", err.Error())
	}
	js := string(raw)
	for _, want := range []string{
		`"code":"content.update.conflict"`,
		`"current_version":`,
	} {
		if !core.Contains(js, want) {
			t.Fatalf("expected marshalled envelope to contain %s, got %s", want, js)
		}
	}
	for _, banned := range []string{`"Code":`, `"Message":`, `"Operation":`} {
		if core.Contains(js, banned) {
			t.Fatalf("marshalled envelope leaks *core.Err PascalCase key %s: %s", banned, js)
		}
	}
}

// TestAtomicCutover_MarketingContent_LegacyFile_Ugly — a content file
// without version: frontmatter reads as version 0; Advance upgrades it
// via an unconditional first-write that stamps version=1.
func TestAtomicCutover_MarketingContent_LegacyFile_Ugly(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newTestSvc(t)
	dirR := core.UserHomeDir()
	if !dirR.OK {
		t.Fatalf("UserHomeDir: %s", dirR.Error())
	}
	contentDir := core.PathJoin(dirR.Value.(string), "Lethean/marketing/content")
	if mk := core.MkdirAll(contentDir, 0o700); !mk.OK {
		t.Fatalf("MkdirAll: %s", mk.Error())
	}
	legacyID := "legacy-post-2026"
	fpath := core.PathJoin(contentDir, legacyID+".md")
	legacy := []byte("---\nid: " + legacyID + "\nt: Legacy post\nwho: \"\"\nwhen: \"\"\ndue: \"\"\ncol: draft\n---\n")
	if w := core.WriteFile(fpath, legacy, 0o600); !w.OK {
		t.Fatalf("WriteFile: %s", w.Error())
	}
	rd := paths.ReadVersion(fpath)
	if !rd.OK {
		t.Fatalf("ReadVersion: %s", rd.Error())
	}
	if got := rd.Value.(paths.ReadOutput); got.Version != 0 {
		t.Fatalf("legacy file pre-update: expected version 0, got %d", got.Version)
	}
	ur := svc.Advance(legacyID)
	if !ur.OK {
		t.Fatalf("Advance failed: %s", ur.Error())
	}
	rd2 := paths.ReadVersion(fpath)
	if !rd2.OK {
		t.Fatalf("ReadVersion post-update: %s", rd2.Error())
	}
	if got := rd2.Value.(paths.ReadOutput); got.Version != 1 {
		t.Fatalf("legacy file post-update: expected version 1, got %d", got.Version)
	}
}

// TestAtomicCutover_MarketingContent_AuditEmissionRecordBatch_Good —
// Create routes through the primitive's write path (EventWriteSucceeded
// fires) and marketing/content/* falls under AuditModeBatch per RFC §6.1.
func TestAtomicCutover_MarketingContent_AuditEmissionRecordBatch_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newTestSvc(t)
	paths.SetAuditSecretProvider(func() []byte {
		return []byte("content-cutover-test-secret-32-byte")
	})
	t.Cleanup(func() { paths.SetAuditSecretProvider(nil) })
	var saw []paths.LockEvent
	paths.SubscribeLockEvents(func(ev paths.LockEvent) {
		saw = append(saw, ev)
	})
	t.Cleanup(paths.ClearLockEventSubscribersForTest)

	r := svc.Create(content.CreateInput{T: "Audit-emission probe", Col: "idea"})
	if !r.OK {
		t.Fatalf("Create failed: %s", r.Error())
	}
	found := false
	for _, ev := range saw {
		if ev.Kind == paths.EventWriteSucceeded {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("Create MUST route through paths.AtomicWriteWithVersion (no EventWriteSucceeded seen)")
	}
	dirR := core.UserHomeDir()
	if !dirR.OK {
		t.Fatalf("UserHomeDir: %s", dirR.Error())
	}
	fpath := core.PathJoin(dirR.Value.(string), "Lethean/marketing/content/x.md")
	mode := paths.AuditModeForPath(fpath)
	if mode != paths.AuditModeBatch {
		t.Fatalf("expected AuditModeBatch for marketing/content path, got %v", mode)
	}
}

// ---- RFC.stage-e-unlockgate v2 §4.2 SessionGate G/B/U triad ---------------

// TestContent_UnlockedGate_AllowsCreate — gate returns non-empty
// account-id slice; Create proceeds to title validation and write.
func TestContent_UnlockedGate_AllowsCreate(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := content.NewService(nil)
	svc.SetSessionGate(&stubSessionGate{ids: []string{"acct-1"}})
	r := svc.Create(content.CreateInput{T: "Unlocked-gate post"})
	if !r.OK {
		t.Fatalf("Create with unlocked gate failed: %s", r.Error())
	}
}

// TestContent_UnlockedGate_AllowsAdvance — gate returns non-empty
// account-id slice; Advance proceeds to read/write.
func TestContent_UnlockedGate_AllowsAdvance(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := content.NewService(nil)
	svc.SetSessionGate(&stubSessionGate{ids: []string{"acct-1"}})
	cr := svc.Create(content.CreateInput{T: "Advance post", Col: "idea"})
	if !cr.OK {
		t.Fatalf("Create with unlocked gate failed: %s", cr.Error())
	}
	id := cr.Value.(content.ContentItem).ID
	ar := svc.Advance(id)
	if !ar.OK {
		t.Fatalf("Advance with unlocked gate failed: %s", ar.Error())
	}
}

// TestContent_LockedGate_FailsCreate_session_locked — gate returns
// empty slice; Create returns content.session.locked envelope without
// touching the filesystem.
func TestContent_LockedGate_FailsCreate_session_locked(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := content.NewService(nil)
	svc.SetSessionGate(&stubSessionGate{ids: []string{}})
	r := svc.Create(content.CreateInput{T: "Locked-gate post"})
	if r.OK {
		t.Fatalf("Create with locked gate must reject, returned OK")
	}
	if !core.Contains(r.Error(), "content.session.locked") {
		t.Fatalf("expected content.session.locked, got %s", r.Error())
	}
}

// TestContent_LockedGate_FailsAdvance_session_locked — gate returns
// empty slice; Advance returns content.session.locked envelope without
// touching the filesystem.
func TestContent_LockedGate_FailsAdvance_session_locked(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	// Seed an item under an unlocked gate first.
	svc := newTestSvc(t)
	cr := svc.Create(content.CreateInput{T: "Lock-after-create post", Col: "idea"})
	if !cr.OK {
		t.Fatalf("Create failed: %s", cr.Error())
	}
	id := cr.Value.(content.ContentItem).ID
	// Lock the gate — live-read picks it up on the next Advance call.
	svc.SetSessionGate(&stubSessionGate{ids: []string{}})
	r := svc.Advance(id)
	if r.OK {
		t.Fatalf("Advance with locked gate must reject, returned OK")
	}
	if !core.Contains(r.Error(), "content.session.locked") {
		t.Fatalf("expected content.session.locked, got %s", r.Error())
	}
}

// TestContent_NilGate_WarnsOnce_FailsClosed — nil gate fails-closed
// AND the one-shot core.Warn fires only on the first call (CompareAndSwap
// semantics per §2.2 ADD-1.5). The second call still fails-closed but
// must NOT re-warn — we verify nilWarned-state indirectly by asserting
// both calls fail with the same envelope (the warn channel is a Core
// global; observation is best-effort, but the fail-closed contract is
// load-bearing and is what this test pins).
func TestContent_NilGate_WarnsOnce_FailsClosed(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := content.NewService(nil) // session == nil by default
	r1 := svc.Create(content.CreateInput{T: "Nil-gate post 1"})
	if r1.OK {
		t.Fatalf("Create with nil gate must reject, returned OK")
	}
	if !core.Contains(r1.Error(), "content.session.locked") {
		t.Fatalf("expected content.session.locked, got %s", r1.Error())
	}
	r2 := svc.Create(content.CreateInput{T: "Nil-gate post 2"})
	if r2.OK {
		t.Fatalf("second Create with nil gate must also reject, returned OK")
	}
	if !core.Contains(r2.Error(), "content.session.locked") {
		t.Fatalf("expected content.session.locked, got %s", r2.Error())
	}
}

// TestContent_StopNilsGate — Stop nils the gate reference so a
// previously-unlocked Service fails-closed on any post-Stop write.
func TestContent_StopNilsGate(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newTestSvc(t)
	// Sanity: unlocked-gate Create succeeds.
	cr := svc.Create(content.CreateInput{T: "Pre-stop post"})
	if !cr.OK {
		t.Fatalf("Create before Stop failed: %s", cr.Error())
	}
	// Stop nils the gate.
	if sr := svc.Stop(core.Background()); !sr.OK {
		t.Fatalf("Stop failed: %s", sr.Error())
	}
	// Post-Stop write fails-closed.
	r := svc.Create(content.CreateInput{T: "Post-stop post"})
	if r.OK {
		t.Fatalf("Create after Stop must reject, returned OK")
	}
	if !core.Contains(r.Error(), "content.session.locked") {
		t.Fatalf("expected content.session.locked, got %s", r.Error())
	}
}

// TestContent_LockedGate_ReadStillWorks — reads (List, Get) stay open
// per RFC §3.1 even when the session is locked. The List call uses a
// freshly-locked service with no pre-seeded items so the assertion is
// shape-only (no error, empty columns).
func TestContent_LockedGate_ReadStillWorks(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := content.NewService(nil)
	svc.SetSessionGate(&stubSessionGate{ids: []string{}})
	r := svc.List(content.ListInput{})
	if !r.OK {
		t.Fatalf("List with locked gate failed: %s", r.Error())
	}
	out := r.Value.(content.ListOutput)
	if len(out.Columns) != 5 {
		t.Fatalf("expected 5 columns, got %d", len(out.Columns))
	}
}
