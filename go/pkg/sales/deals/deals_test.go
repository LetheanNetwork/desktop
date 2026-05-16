// SPDX-Licence-Identifier: EUPL-1.2

package deals_test

import (
	"testing"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/paths"
	"dappco.re/lthn/desktop/pkg/sales/deals"
)

func TestCreate_WritesFile_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := deals.NewService(nil)
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
	svc := deals.NewService(nil)
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
	svc := deals.NewService(nil)
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
	svc := deals.NewService(nil)
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
	svc := deals.NewService(nil)
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
	svc := deals.NewService(nil)
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
	svc := deals.NewService(nil)
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
	svc := deals.NewService(nil)
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
	svc := deals.NewService(nil)
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
	svc := deals.NewService(nil)
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
	svc := deals.NewService(nil)
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

// TestAtomicCutover_Deals_Update_VersionStale_Ugly — stale IfVersion
// surfaces a wrapped conflict via the primitive. Drives the primitive
// directly with a known-stale IfVersion to assert the conflict-wrap
// shape is reachable on sales/deals paths.
func TestAtomicCutover_Deals_Update_VersionStale_Ugly(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := deals.NewService(nil)
	cr := svc.Create(deals.CreateInput{
		Customer: "Pemberton Capital", AmountPence: 62000, Stage: "engage",
	})
	if !cr.OK {
		t.Fatalf("Create failed: %s", cr.Error())
	}
	id := cr.Value.(deals.Deal).ID
	dirR := core.UserHomeDir()
	if !dirR.OK {
		t.Fatalf("UserHomeDir: %s", dirR.Error())
	}
	fpath := core.PathJoin(dirR.Value.(string), "Lethean/sales/deals", id+".md")
	body := []byte("---\nversion: 1\nid: " + id + "\ncustomer: Stale\nstage: engage\n---\n")
	r := paths.AtomicWriteWithVersion(fpath, paths.WriteInput{
		Body:      body,
		IfVersion: 99, // intentionally stale
	})
	if r.OK {
		t.Fatal("expected stale conflict, got OK")
	}
	if !core.Contains(r.Error(), paths.CodeVersionStale) {
		t.Fatalf("expected paths.CodeVersionStale in error, got %q", r.Error())
	}
	vs, ok := paths.VersionStaleFromError(r.Value)
	if !ok {
		t.Fatal("expected VersionStale envelope reachable via VersionStaleFromError")
	}
	if vs.CurrentVersion != 1 {
		t.Fatalf("expected CurrentVersion=1, got %d", vs.CurrentVersion)
	}
}

// TestAtomicCutover_Deals_LegacyFile_Ugly — a deal file without
// version: frontmatter reads as version 0; UpdateStage upgrades it via
// an unconditional first-write that stamps version=1.
func TestAtomicCutover_Deals_LegacyFile_Ugly(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := deals.NewService(nil)
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
	svc := deals.NewService(nil)
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
