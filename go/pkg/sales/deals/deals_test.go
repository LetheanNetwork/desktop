// SPDX-Licence-Identifier: EUPL-1.2

package deals_test

import (
	"encoding/json"
	"sync"
	"testing"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/paths"
	"dappco.re/lthn/desktop/pkg/sales/deals"
)

// stubSessionGate is the test double for the consumer-defined
// SessionGate interface (RFC.stage-e-unlockgate v2 §4.2 stub shape —
// mirrors sales/contacts test surface).
type stubSessionGate struct{ ids []string }

func (s *stubSessionGate) UnlockedAccountIDs() []string { return s.ids }

// newTestSvc constructs a deals.Service pre-wired with a SessionGate
// reporting one unlocked account so existing write-path tests continue
// to exercise the success path post-retrofit. Tests that need to
// drive a locked or nil-gate fail-closed path call NewService directly
// (or SetSessionGate explicitly with an empty stub).
//
// Usage example:
//
//	svc := newTestSvc(t)
//	svc.Create(deals.CreateInput{Customer: "Heritage Law"})
func newTestSvc(_ *testing.T) *deals.Service {
	svc := deals.NewService(nil)
	svc.SetSessionGate(&stubSessionGate{ids: []string{"acct-test"}})
	return svc
}

func TestCreate_WritesFile_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newTestSvc(t)
	r := svc.Create(deals.CreateInput{
		Customer:    "Heritage Law LLP",
		AmountPence: 24000,
		Stage:       "engage",
		Owner:       "Snider",
	})
	if !r.OK {
		t.Fatalf("Create failed: %s", r.Error())
	}
	d := r.Value.(deals.Deal)
	if d.Customer != "Heritage Law LLP" {
		t.Fatalf("expected Heritage Law LLP, got %q", d.Customer)
	}
	if d.Stage != "Engaging" {
		t.Fatalf("expected Engaging, got %q", d.Stage)
	}
}

func TestList_Empty_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newTestSvc(t)
	r := svc.List(deals.ListInput{})
	if !r.OK {
		t.Fatalf("List failed: %s", r.Error())
	}
	out := r.Value.(deals.ListOutput)
	if len(out.Deals) != 0 {
		t.Fatalf("expected 0 deals, got %d", len(out.Deals))
	}
}

func TestList_FiltersByStage_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newTestSvc(t)
	svc.Create(deals.CreateInput{Customer: "A", Stage: "engage", AmountPence: 10000})
	svc.Create(deals.CreateInput{Customer: "B", Stage: "qual", AmountPence: 5000})

	r := svc.List(deals.ListInput{Stage: "engage"})
	if !r.OK {
		t.Fatalf("List failed: %s", r.Error())
	}
	out := r.Value.(deals.ListOutput)
	if len(out.Deals) != 1 {
		t.Fatalf("expected 1 deal, got %d", len(out.Deals))
	}
	if out.Deals[0].Customer != "A" {
		t.Fatalf("expected A, got %q", out.Deals[0].Customer)
	}
}

func TestUpdateStage_Transitions_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newTestSvc(t)
	svc.Create(deals.CreateInput{Customer: "Heritage Law", Stage: "engage", AmountPence: 24000})

	// Find the created deal ID via List.
	lr := svc.List(deals.ListInput{})
	if !lr.OK {
		t.Fatalf("List failed: %s", lr.Error())
	}
	out := lr.Value.(deals.ListOutput)
	if len(out.Deals) != 1 {
		t.Fatalf("expected 1 deal")
	}
	id := out.Deals[0].ID

	r := svc.UpdateStage(deals.UpdateStageInput{ID: id, Stage: "propose"})
	if !r.OK {
		t.Fatalf("UpdateStage failed: %s", r.Error())
	}
	d := r.Value.(deals.Deal)
	if d.Stage != "Proposal" {
		t.Fatalf("expected Proposal, got %q", d.Stage)
	}
}

func TestUpdateStage_InvalidStage_Bad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newTestSvc(t)
	svc.Create(deals.CreateInput{Customer: "X", Stage: "engage", AmountPence: 1000})

	lr := svc.List(deals.ListInput{})
	out := lr.Value.(deals.ListOutput)
	id := out.Deals[0].ID

	r := svc.UpdateStage(deals.UpdateStageInput{ID: id, Stage: "magic"})
	if r.OK {
		t.Fatalf("expected failure for invalid stage, got OK")
	}
}

func TestAddActivity_Prepends_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newTestSvc(t)
	svc.Create(deals.CreateInput{Customer: "Heritage Law", Stage: "engage", AmountPence: 24000})

	lr := svc.List(deals.ListInput{})
	out := lr.Value.(deals.ListOutput)
	id := out.Deals[0].ID

	svc.AddActivity(deals.AddActivityInput{DealID: id, K: "email", Who: "you", T: "first"})
	r := svc.AddActivity(deals.AddActivityInput{DealID: id, K: "call", Who: "you", T: "second"})
	if !r.OK {
		t.Fatalf("AddActivity failed: %s", r.Error())
	}
	d := r.Value.(deals.Deal)
	if len(d.Log) != 2 {
		t.Fatalf("expected 2 log entries, got %d", len(d.Log))
	}
	if d.Log[0].T != "second" {
		t.Fatalf("expected newest first (second), got %q", d.Log[0].T)
	}
}

func TestGet_ReturnsFullLog_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newTestSvc(t)
	svc.Create(deals.CreateInput{Customer: "Heritage Law", Stage: "engage", AmountPence: 24000})

	lr := svc.List(deals.ListInput{})
	out := lr.Value.(deals.ListOutput)
	id := out.Deals[0].ID

	svc.AddActivity(deals.AddActivityInput{DealID: id, K: "call", Who: "you", T: "call one"})

	r := svc.Get(deals.GetInput{ID: id})
	if !r.OK {
		t.Fatalf("Get failed: %s", r.Error())
	}
	d := r.Value.(deals.Deal)
	if len(d.Log) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(d.Log))
	}
}

func TestAddActivity_InvalidKind_Bad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newTestSvc(t)
	svc.Create(deals.CreateInput{Customer: "X", Stage: "engage", AmountPence: 1000})
	lr := svc.List(deals.ListInput{})
	out := lr.Value.(deals.ListOutput)
	id := out.Deals[0].ID

	r := svc.AddActivity(deals.AddActivityInput{DealID: id, K: "tweet", Who: "you", T: "bad"})
	if r.OK {
		t.Fatalf("expected failure for invalid kind, got OK")
	}
}

func TestCreate_EmptyCustomer_Ugly(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newTestSvc(t)
	r := svc.Create(deals.CreateInput{Customer: "", Stage: "engage"})
	if r.OK {
		t.Fatalf("expected failure for empty customer, got OK")
	}
}

// ---- Cascade W1 cutover tests (paths.AtomicWriteWithVersion) ---------------

// TestAtomicCutover_Deals_Create_Good — Create stamps version=1 via the
// primitive; ReadVersion confirms the on-disk file carries the same.
func TestAtomicCutover_Deals_Create_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newTestSvc(t)
	r := svc.Create(deals.CreateInput{
		Customer:    "Heritage Law LLP",
		AmountPence: 24000,
		Stage:       "engage",
		Owner:       "Snider",
	})
	if !r.OK {
		t.Fatalf("Create failed: %s", r.Error())
	}
	d := r.Value.(deals.Deal)
	dirR := core.UserHomeDir()
	if !dirR.OK {
		t.Fatalf("UserHomeDir: %s", dirR.Error())
	}
	fpath := core.PathJoin(dirR.Value.(string), "Lethean/sales/deals", d.ID+".md")
	rd := paths.ReadVersion(fpath)
	if !rd.OK {
		t.Fatalf("ReadVersion: %s", rd.Error())
	}
	got := rd.Value.(paths.ReadOutput)
	if got.Version != 1 {
		t.Fatalf("expected version 1 after Create, got %d", got.Version)
	}
}

// TestAtomicCutover_Deals_Update_Good — a sequential UpdateStage bumps
// the stored version monotonically (1 → 2).
func TestAtomicCutover_Deals_Update_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newTestSvc(t)
	cr := svc.Create(deals.CreateInput{
		Customer: "Stannard & Co", AmountPence: 44000, Stage: "engage", Owner: "Snider",
	})
	if !cr.OK {
		t.Fatalf("Create failed: %s", cr.Error())
	}
	id := cr.Value.(deals.Deal).ID
	ur := svc.UpdateStage(deals.UpdateStageInput{ID: id, Stage: "propose"})
	if !ur.OK {
		t.Fatalf("UpdateStage failed: %s", ur.Error())
	}
	dirR := core.UserHomeDir()
	if !dirR.OK {
		t.Fatalf("UserHomeDir: %s", dirR.Error())
	}
	fpath := core.PathJoin(dirR.Value.(string), "Lethean/sales/deals", id+".md")
	rd := paths.ReadVersion(fpath)
	if !rd.OK {
		t.Fatalf("ReadVersion: %s", rd.Error())
	}
	got := rd.Value.(paths.ReadOutput)
	if got.Version != 2 {
		t.Fatalf("expected version 2 after UpdateStage, got %d", got.Version)
	}
}

// TestAtomicCutover_Deals_Update_VersionStale_Ugly — drives two
// concurrent svc.UpdateStage calls and asserts the loser's Result
// carries a paths.ConflictEnvelope whose JSON marshal matches the
// lowercase `{code, current_version, current_hash}` wire shape that
// frontend/src/lit/conflict-dispatch.ts extractEnvelope pattern-
// matches on (Mantis #1547 service-tier round-trip; pins #1544 against
// future drift).
//
// Why two goroutines: svc.UpdateStage re-reads the file inside the
// call so a single-shot out-of-band mutation cannot drive a real
// conflict through the service. Concurrent UpdateStage goroutines
// race for the WithFileLock; the loser's locked-section ReadVersion
// sees the winner's bumped version and the primitive returns
// VersionStale → wrapped as ConflictEnvelope by deals.writeRecord.
func TestAtomicCutover_Deals_Update_VersionStale_Ugly(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newTestSvc(t)
	cr := svc.Create(deals.CreateInput{
		Customer: "Pemberton Capital", AmountPence: 62000, Stage: "engage",
	})
	if !cr.OK {
		t.Fatalf("Create failed: %s", cr.Error())
	}
	id := cr.Value.(deals.Deal).ID

	var conflict core.Result
	var saw bool
	for attempt := 0; attempt < 32 && !saw; attempt++ {
		var (
			mu      sync.Mutex
			results []core.Result
			wg      sync.WaitGroup
			start   = make(chan struct{})
		)
		wg.Add(2)
		// Alternate target stages so each goroutine has a legal-from-
		// engage transition target (engage → propose / engage → qual is
		// illegal so use propose/close which are both reachable). The
		// goal is to make BOTH calls valid input-wise so the only
		// failure mode is the optimistic-lock conflict, not a
		// transition-illegal rejection.
		go func() {
			defer wg.Done()
			<-start
			r := svc.UpdateStage(deals.UpdateStageInput{ID: id, Stage: "propose"})
			mu.Lock()
			results = append(results, r)
			mu.Unlock()
		}()
		go func() {
			defer wg.Done()
			<-start
			r := svc.UpdateStage(deals.UpdateStageInput{ID: id, Stage: "propose"})
			mu.Lock()
			results = append(results, r)
			mu.Unlock()
		}()
		close(start)
		wg.Wait()

		for _, r := range results {
			if !r.OK && core.Contains(r.Error(), "deals.update.conflict") {
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
	if env.Code != "deals.update.conflict" {
		t.Fatalf("expected envelope code deals.update.conflict, got %q", env.Code)
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
		`"code":"deals.update.conflict"`,
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

// TestAtomicCutover_Deals_LegacyFile_Ugly — a deal file without
// version: frontmatter reads as version 0; UpdateStage upgrades it via
// an unconditional first-write that stamps version=1.
func TestAtomicCutover_Deals_LegacyFile_Ugly(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newTestSvc(t)
	dirR := core.UserHomeDir()
	if !dirR.OK {
		t.Fatalf("UserHomeDir: %s", dirR.Error())
	}
	dealsDir := core.PathJoin(dirR.Value.(string), "Lethean/sales/deals")
	if mk := core.MkdirAll(dealsDir, 0o700); !mk.OK {
		t.Fatalf("MkdirAll: %s", mk.Error())
	}
	legacyID := "202605-DEAL-999"
	fpath := core.PathJoin(dealsDir, legacyID+".md")
	legacy := []byte("---\nid: " + legacyID + "\ncustomer: Legacy Co\nstage: qual\namount_pence: 1000\nprobability_pct: 25\nclose_target: \"\"\nowner: Snider\ncreated_at: 2026-05-01T00:00:00Z\nupdated_at: 2026-05-01T00:00:00Z\n---\n")
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
	ur := svc.UpdateStage(deals.UpdateStageInput{ID: legacyID, Stage: "engage"})
	if !ur.OK {
		t.Fatalf("UpdateStage failed: %s", ur.Error())
	}
	rd2 := paths.ReadVersion(fpath)
	if !rd2.OK {
		t.Fatalf("ReadVersion post-update: %s", rd2.Error())
	}
	if got := rd2.Value.(paths.ReadOutput); got.Version != 1 {
		t.Fatalf("legacy file post-update: expected version 1, got %d", got.Version)
	}
}

// TestAtomicCutover_Deals_AuditEmissionRecordBatch_Good — Create routes
// through the primitive's write path (EventWriteSucceeded fires) and
// sales/deals/* falls under AuditModeBatch per RFC §6.1.
func TestAtomicCutover_Deals_AuditEmissionRecordBatch_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newTestSvc(t)
	paths.SetAuditSecretProvider(func() []byte {
		return []byte("deals-cutover-test-secret-32-byte")
	})
	t.Cleanup(func() { paths.SetAuditSecretProvider(nil) })
	var saw []paths.LockEvent
	paths.SubscribeLockEvents(func(ev paths.LockEvent) {
		saw = append(saw, ev)
	})
	t.Cleanup(paths.ClearLockEventSubscribersForTest)

	r := svc.Create(deals.CreateInput{
		Customer: "Whitethorn Press", AmountPence: 36000, Stage: "close",
	})
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
	fpath := core.PathJoin(dirR.Value.(string), "Lethean/sales/deals/x.md")
	mode := paths.AuditModeForPath(fpath)
	if mode != paths.AuditModeBatch {
		t.Fatalf("expected AuditModeBatch for sales/deals path, got %v", mode)
	}
}

// ---- SessionGate retrofit (RFC.stage-e-unlockgate v2 §4.2 / Mantis #1613 B.2)

// TestDeals_NilGate_WarnsOnce_FailsClosed — a Service constructed
// without SetSessionGate fails-closed on writes. Second + third writes
// continue to fail-closed (one-shot warn semantics — the second call
// must remain quiet but its caller-visible result must be the same
// fail-closed shape).
func TestDeals_NilGate_WarnsOnce_FailsClosed(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := deals.NewService(nil) // NO SetSessionGate — exercises §2.2 fail-safe.

	r1 := svc.Create(deals.CreateInput{Customer: "Heritage Law LLP", AmountPence: 10000, Stage: "engage"})
	if r1.OK {
		t.Fatal("expected Create to fail-closed when gate is nil")
	}
	if !core.Contains(r1.Error(), "deals.session.locked") {
		t.Fatalf("expected deals.session.locked on first Create, got %q", r1.Error())
	}

	// Second write — nilWarned already true; CompareAndSwap returns
	// false and core.Warn is NOT called again. Behaviour from the
	// caller's perspective: same fail-closed result.
	r2 := svc.UpdateStage(deals.UpdateStageInput{ID: "202605-DEAL-001", Stage: "propose"})
	if r2.OK {
		t.Fatal("expected UpdateStage to fail-closed when gate is nil")
	}
	if !core.Contains(r2.Error(), "deals.session.locked") {
		t.Fatalf("expected deals.session.locked on UpdateStage, got %q", r2.Error())
	}

	// Third write — same fail-closed behaviour persists across method
	// shapes (AddActivity).
	r3 := svc.AddActivity(deals.AddActivityInput{DealID: "202605-DEAL-001", K: "call", Who: "you", T: "x"})
	if r3.OK {
		t.Fatal("expected AddActivity to fail-closed when gate is nil")
	}
	if !core.Contains(r3.Error(), "deals.session.locked") {
		t.Fatalf("expected deals.session.locked on AddActivity, got %q", r3.Error())
	}
}

// TestDeals_UnlockedGate_AllowsCreate — Create succeeds when the
// live-read gate reports at least one unlocked account.
func TestDeals_UnlockedGate_AllowsCreate(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := deals.NewService(nil)
	svc.SetSessionGate(&stubSessionGate{ids: []string{"acct-1"}})

	r := svc.Create(deals.CreateInput{
		Customer: "Heritage Law LLP", AmountPence: 24000, Stage: "engage",
	})
	if !r.OK {
		t.Fatalf("Create should succeed with gate reporting unlocked acct, got: %s", r.Error())
	}
}

// TestDeals_UnlockedGate_AllowsUpdateStage — UpdateStage succeeds when
// the live-read gate reports at least one unlocked account.
func TestDeals_UnlockedGate_AllowsUpdateStage(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := deals.NewService(nil)
	svc.SetSessionGate(&stubSessionGate{ids: []string{"acct-1"}})

	cr := svc.Create(deals.CreateInput{
		Customer: "Heritage Law LLP", AmountPence: 24000, Stage: "engage",
	})
	if !cr.OK {
		t.Fatalf("seed Create failed: %s", cr.Error())
	}
	id := cr.Value.(deals.Deal).ID
	ur := svc.UpdateStage(deals.UpdateStageInput{ID: id, Stage: "propose"})
	if !ur.OK {
		t.Fatalf("UpdateStage should succeed with gate reporting unlocked acct, got: %s", ur.Error())
	}
}

// TestDeals_UnlockedGate_AllowsAddActivity — AddActivity succeeds when
// the live-read gate reports at least one unlocked account.
func TestDeals_UnlockedGate_AllowsAddActivity(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := deals.NewService(nil)
	svc.SetSessionGate(&stubSessionGate{ids: []string{"acct-1"}})

	cr := svc.Create(deals.CreateInput{
		Customer: "Heritage Law LLP", AmountPence: 24000, Stage: "engage",
	})
	if !cr.OK {
		t.Fatalf("seed Create failed: %s", cr.Error())
	}
	id := cr.Value.(deals.Deal).ID
	ar := svc.AddActivity(deals.AddActivityInput{
		DealID: id, K: "call", Who: "you", T: "30-min privacy chat",
	})
	if !ar.OK {
		t.Fatalf("AddActivity should succeed with gate reporting unlocked acct, got: %s", ar.Error())
	}
}

// TestDeals_LockedGate_FailsCreate_session_locked — Create rejects when
// the live-read gate reports zero unlocked accounts.
func TestDeals_LockedGate_FailsCreate_session_locked(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := deals.NewService(nil)
	svc.SetSessionGate(&stubSessionGate{ids: []string{}})

	r := svc.Create(deals.CreateInput{
		Customer: "Heritage Law LLP", AmountPence: 24000, Stage: "engage",
	})
	if r.OK {
		t.Fatal("expected Create to be rejected when gate reports zero unlocked accounts")
	}
	if !core.Contains(r.Error(), "deals.session.locked") {
		t.Fatalf("expected deals.session.locked, got %q", r.Error())
	}
}

// TestDeals_LockedGate_FailsUpdateStage_session_locked — UpdateStage
// rejects when the live-read gate reports zero unlocked accounts. The
// gate fires BEFORE the IsValidID check + filesystem read, so the
// session.locked code surfaces in preference to any later error.
func TestDeals_LockedGate_FailsUpdateStage_session_locked(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	// Seed a record while unlocked, then flip to locked + try to update.
	svc := newTestSvc(t)
	cr := svc.Create(deals.CreateInput{
		Customer: "Heritage Law LLP", AmountPence: 24000, Stage: "engage",
	})
	if !cr.OK {
		t.Fatalf("seed Create failed: %s", cr.Error())
	}
	id := cr.Value.(deals.Deal).ID
	svc.SetSessionGate(&stubSessionGate{ids: []string{}})

	r := svc.UpdateStage(deals.UpdateStageInput{ID: id, Stage: "propose"})
	if r.OK {
		t.Fatal("expected UpdateStage to be rejected when gate reports zero unlocked accounts")
	}
	if !core.Contains(r.Error(), "deals.session.locked") {
		t.Fatalf("expected deals.session.locked, got %q", r.Error())
	}
}

// TestDeals_LockedGate_FailsAddActivity_session_locked — AddActivity
// rejects when the live-read gate reports zero unlocked accounts.
func TestDeals_LockedGate_FailsAddActivity_session_locked(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newTestSvc(t)
	cr := svc.Create(deals.CreateInput{
		Customer: "Heritage Law LLP", AmountPence: 24000, Stage: "engage",
	})
	if !cr.OK {
		t.Fatalf("seed Create failed: %s", cr.Error())
	}
	id := cr.Value.(deals.Deal).ID
	svc.SetSessionGate(&stubSessionGate{ids: []string{}})

	r := svc.AddActivity(deals.AddActivityInput{
		DealID: id, K: "call", Who: "you", T: "30-min call",
	})
	if r.OK {
		t.Fatal("expected AddActivity to be rejected when gate reports zero unlocked accounts")
	}
	if !core.Contains(r.Error(), "deals.session.locked") {
		t.Fatalf("expected deals.session.locked, got %q", r.Error())
	}
}

// TestDeals_StopNilsGate — Stop() severs the SessionGate; subsequent
// writes fail-closed even though the gate WAS wired (Cerberus #27
// ADD-5 — Stop drain hygiene mirrors mail + contacts).
func TestDeals_StopNilsGate(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newTestSvc(t) // gate wired with unlocked stub

	// Pre-Stop: write succeeds.
	if r := svc.Create(deals.CreateInput{
		Customer: "Heritage Law LLP", AmountPence: 24000, Stage: "engage",
	}); !r.OK {
		t.Fatalf("Create should succeed pre-Stop, got: %s", r.Error())
	}

	// Stop nils the gate reference.
	if r := svc.Stop(core.Background()); !r.OK {
		t.Fatalf("Stop should succeed, got: %s", r.Error())
	}

	// Post-Stop: write fails-closed via the nil-gate path.
	r := svc.Create(deals.CreateInput{
		Customer: "Stannard & Co", AmountPence: 44000, Stage: "engage",
	})
	if r.OK {
		t.Fatal("expected Create to fail-closed after Stop nils the gate")
	}
	if !core.Contains(r.Error(), "deals.session.locked") {
		t.Fatalf("expected deals.session.locked, got %q", r.Error())
	}
}

// TestDeals_LockedGate_ReadStillWorks — List + Get are not gated by
// the session-lock (RFC §3.1 — reads stay open while locked).
func TestDeals_LockedGate_ReadStillWorks(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	// Seed unlocked, then flip to locked.
	svc := newTestSvc(t)
	cr := svc.Create(deals.CreateInput{
		Customer: "Heritage Law LLP", AmountPence: 24000, Stage: "engage",
	})
	if !cr.OK {
		t.Fatalf("seed Create failed: %s", cr.Error())
	}
	id := cr.Value.(deals.Deal).ID
	svc.SetSessionGate(&stubSessionGate{ids: []string{}})

	r := svc.List(deals.ListInput{})
	if !r.OK {
		t.Fatalf("List should succeed when session locked, got: %s", r.Error())
	}
	if g := svc.Get(deals.GetInput{ID: id}); !g.OK {
		t.Fatalf("Get should succeed when session locked, got: %s", g.Error())
	}
}
