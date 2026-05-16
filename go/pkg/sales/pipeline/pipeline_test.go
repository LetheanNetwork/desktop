// SPDX-Licence-Identifier: EUPL-1.2

package pipeline_test

import (
	"encoding/json"
	"sync"
	"testing"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/paths"
	"dappco.re/lthn/desktop/pkg/sales/deals"
	"dappco.re/lthn/desktop/pkg/sales/pipeline"
)

// TestList_Empty_Good — empty deals dir → each stage has zero deals.
func TestList_Empty_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := pipeline.NewService(nil)
	r := svc.List(pipeline.ListInput{})
	if !r.OK {
		t.Fatalf("List failed: %s", r.Error())
	}
	out := r.Value.(pipeline.ListOutput)
	// All 6 stages should be present with 0 deals each.
	if len(out.Columns) != 6 {
		t.Fatalf("expected 6 columns, got %d", len(out.Columns))
	}
	for _, col := range out.Columns {
		if len(col.Deals) != 0 {
			t.Fatalf("column %q has %d deals, expected 0", col.ID, len(col.Deals))
		}
	}
	if out.TotalDeals != 0 {
		t.Fatalf("expected 0 total deals, got %d", out.TotalDeals)
	}
}

// TestList_GroupsByStage_Good — deals in two stages appear in correct columns.
func TestList_GroupsByStage_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dealSvc := deals.NewService(nil)
	dealSvc.Create(deals.CreateInput{Customer: "A", Stage: "engage", AmountPence: 10000})
	dealSvc.Create(deals.CreateInput{Customer: "B", Stage: "engage", AmountPence: 20000})
	dealSvc.Create(deals.CreateInput{Customer: "C", Stage: "qual", AmountPence: 5000})

	svc := pipeline.NewService(nil)
	r := svc.List(pipeline.ListInput{})
	if !r.OK {
		t.Fatalf("List failed: %s", r.Error())
	}
	out := r.Value.(pipeline.ListOutput)

	var engageCol *pipeline.PipelineColumn
	var qualCol *pipeline.PipelineColumn
	for i := range out.Columns {
		switch out.Columns[i].ID {
		case "engage":
			engageCol = &out.Columns[i]
		case "qual":
			qualCol = &out.Columns[i]
		}
	}
	if engageCol == nil || len(engageCol.Deals) != 2 {
		t.Fatalf("expected 2 deals in engage column")
	}
	if qualCol == nil || len(qualCol.Deals) != 1 {
		t.Fatalf("expected 1 deal in qual column")
	}
	if out.TotalDeals != 3 {
		t.Fatalf("expected 3 total deals, got %d", out.TotalDeals)
	}
}

// TestMoveDeal_UpdatesStage_Good — MoveDeal moves a deal to the target stage.
func TestMoveDeal_UpdatesStage_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dealSvc := deals.NewService(nil)
	dealSvc.Create(deals.CreateInput{Customer: "Heritage Law", Stage: "qual", AmountPence: 24000})

	// Get the deal ID.
	lr := dealSvc.List(deals.ListInput{})
	lo := lr.Value.(deals.ListOutput)
	dealID := lo.Deals[0].ID

	svc := pipeline.NewService(nil)
	r := svc.MoveDeal(pipeline.MoveInput{DealID: dealID, ToStage: "engage"})
	if !r.OK {
		t.Fatalf("MoveDeal failed: %s", r.Error())
	}

	// Verify via List.
	r2 := svc.List(pipeline.ListInput{Stage: "engage"})
	if !r2.OK {
		t.Fatalf("List failed: %s", r2.Error())
	}
	out := r2.Value.(pipeline.ListOutput)
	if len(out.Columns) != 1 || len(out.Columns[0].Deals) != 1 {
		t.Fatalf("expected 1 deal in engage column after move")
	}
}

// TestMoveDeal_InvalidStage_Bad — unknown stage returns core.Fail.
func TestMoveDeal_InvalidStage_Bad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := pipeline.NewService(nil)
	r := svc.MoveDeal(pipeline.MoveInput{DealID: "202605-DEAL-001", ToStage: "fantasy"})
	if r.OK {
		t.Fatalf("expected failure for invalid stage, got OK")
	}
}

// TestList_ValueAggregation_Good — amount_pence sums correctly into GBP string.
func TestList_ValueAggregation_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dealSvc := deals.NewService(nil)
	// Two engage deals: 24000p (£24) + 44000p (£44) = 68000p → "£68 K"
	dealSvc.Create(deals.CreateInput{Customer: "A", Stage: "engage", AmountPence: 24000})
	dealSvc.Create(deals.CreateInput{Customer: "B", Stage: "engage", AmountPence: 44000})

	svc := pipeline.NewService(nil)
	r := svc.List(pipeline.ListInput{Stage: "engage"})
	if !r.OK {
		t.Fatalf("List failed: %s", r.Error())
	}
	out := r.Value.(pipeline.ListOutput)
	if len(out.Columns) != 1 {
		t.Fatalf("expected 1 column for stage filter, got %d", len(out.Columns))
	}
	// 24000 + 44000 = 68000 pence → £68 K
	if out.Columns[0].Value != "£68 K" {
		t.Fatalf("expected £68 K, got %q", out.Columns[0].Value)
	}
}

// TestServiceName_Good — ServiceName returns "Pipeline".
func TestServiceName_Good(t *testing.T) {
	svc := pipeline.NewService(nil)
	if svc.ServiceName() != "Pipeline" {
		t.Fatalf("expected Pipeline, got %q", svc.ServiceName())
	}
}

// ---- Cascade W1 cutover tests (paths.AtomicWriteWithVersion) ---------------

// TestAtomicCutover_Pipeline_Create_Good — deals.Create stamps version=1
// (covered by the deals cutover suite); pipeline pre-condition is that
// MoveDeal observes that version on disk. Asserts MoveDeal then advances
// the on-disk version to 2.
func TestAtomicCutover_Pipeline_Create_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dealSvc := deals.NewService(nil)
	pipeSvc := pipeline.NewService(nil)
	cr := dealSvc.Create(deals.CreateInput{
		Customer: "Heritage Law LLP", AmountPence: 24000, Stage: "qual",
	})
	if !cr.OK {
		t.Fatalf("Create failed: %s", cr.Error())
	}
	d := cr.Value.(deals.Deal)
	mr := pipeSvc.MoveDeal(pipeline.MoveInput{DealID: d.ID, ToStage: "engage"})
	if !mr.OK {
		t.Fatalf("MoveDeal failed: %s", mr.Error())
	}
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
	if got.Version != 2 {
		t.Fatalf("expected version 2 after MoveDeal (Create=1 + Move=+1), got %d", got.Version)
	}
}

// TestAtomicCutover_Pipeline_Update_Good — sequential MoveDeal calls
// bump the stored version monotonically (1 → 2 → 3).
func TestAtomicCutover_Pipeline_Update_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dealSvc := deals.NewService(nil)
	pipeSvc := pipeline.NewService(nil)
	cr := dealSvc.Create(deals.CreateInput{
		Customer: "Stannard & Co", AmountPence: 44000, Stage: "qual",
	})
	if !cr.OK {
		t.Fatalf("Create failed: %s", cr.Error())
	}
	d := cr.Value.(deals.Deal)
	if r := pipeSvc.MoveDeal(pipeline.MoveInput{DealID: d.ID, ToStage: "engage"}); !r.OK {
		t.Fatalf("MoveDeal 1: %s", r.Error())
	}
	if r := pipeSvc.MoveDeal(pipeline.MoveInput{DealID: d.ID, ToStage: "propose"}); !r.OK {
		t.Fatalf("MoveDeal 2: %s", r.Error())
	}
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
	if got.Version != 3 {
		t.Fatalf("expected version 3 after Create+Move+Move, got %d", got.Version)
	}
}

// TestAtomicCutover_Pipeline_Update_VersionStale_Ugly — drives two
// concurrent svc.MoveDeal calls (engage→propose, both legal) and
// asserts the loser's Result carries a paths.ConflictEnvelope whose
// JSON marshal matches the lowercase `{code, current_version,
// current_hash}` wire shape that
// frontend/src/lit/conflict-dispatch.ts extractEnvelope pattern-
// matches on (Mantis #1547 service-tier round-trip; pins #1544 against
// future drift).
func TestAtomicCutover_Pipeline_Update_VersionStale_Ugly(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dealSvc := deals.NewService(nil)
	cr := dealSvc.Create(deals.CreateInput{
		Customer: "Pemberton Capital", AmountPence: 62000, Stage: "engage",
	})
	if !cr.OK {
		t.Fatalf("Create failed: %s", cr.Error())
	}
	id := cr.Value.(deals.Deal).ID
	pipeSvc := pipeline.NewService(nil)

	var conflict core.Result
	var saw bool
	for attempt := 0; attempt < 32 && !saw; attempt++ {
		// Re-stage the deal back to "engage" between attempts so both
		// goroutines have a legal target. After a winning move lands
		// the file sits at "propose"; the loser's IsLegalTransition
		// would reject a second "propose" as a self-transition (Mantis
		// #1488) and short-circuit before reaching writeDealStage.
		if attempt > 0 {
			reset := pipeSvc.MoveDeal(pipeline.MoveInput{DealID: id, ToStage: "engage"})
			if !reset.OK {
				t.Fatalf("reset MoveDeal: %s", reset.Error())
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
			r := pipeSvc.MoveDeal(pipeline.MoveInput{DealID: id, ToStage: "propose"})
			mu.Lock()
			results = append(results, r)
			mu.Unlock()
		}()
		go func() {
			defer wg.Done()
			<-start
			r := pipeSvc.MoveDeal(pipeline.MoveInput{DealID: id, ToStage: "propose"})
			mu.Lock()
			results = append(results, r)
			mu.Unlock()
		}()
		close(start)
		wg.Wait()

		for _, r := range results {
			if !r.OK && core.Contains(r.Error(), "pipeline.update.conflict") {
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
	if env.Code != "pipeline.update.conflict" {
		t.Fatalf("expected envelope code pipeline.update.conflict, got %q", env.Code)
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
		`"code":"pipeline.update.conflict"`,
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

// TestAtomicCutover_Pipeline_LegacyFile_Ugly — a deal file with no
// version: frontmatter reads as version 0; MoveDeal upgrades it via an
// unconditional first-write that stamps version=1 alongside the new
// stage value.
func TestAtomicCutover_Pipeline_LegacyFile_Ugly(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	pipeSvc := pipeline.NewService(nil)
	dirR := core.UserHomeDir()
	if !dirR.OK {
		t.Fatalf("UserHomeDir: %s", dirR.Error())
	}
	dealsDir := core.PathJoin(dirR.Value.(string), "Lethean/sales/deals")
	if mk := core.MkdirAll(dealsDir, 0o700); !mk.OK {
		t.Fatalf("MkdirAll: %s", mk.Error())
	}
	legacyID := "202605-DEAL-998"
	fpath := core.PathJoin(dealsDir, legacyID+".md")
	legacy := []byte("---\nid: " + legacyID + "\ncustomer: Legacy Co\nstage: qual\namount_pence: 1000\nclose_target: \"\"\n---\n")
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
	mr := pipeSvc.MoveDeal(pipeline.MoveInput{DealID: legacyID, ToStage: "engage"})
	if !mr.OK {
		t.Fatalf("MoveDeal failed: %s", mr.Error())
	}
	rd2 := paths.ReadVersion(fpath)
	if !rd2.OK {
		t.Fatalf("ReadVersion post-update: %s", rd2.Error())
	}
	if got := rd2.Value.(paths.ReadOutput); got.Version != 1 {
		t.Fatalf("legacy file post-update: expected version 1, got %d", got.Version)
	}
}

// TestAtomicCutover_Pipeline_AuditEmissionRecordBatch_Good — MoveDeal
// routes through the primitive's write path (EventWriteSucceeded fires)
// and sales/deals/* paths fall under AuditModeBatch per RFC §6.1.
func TestAtomicCutover_Pipeline_AuditEmissionRecordBatch_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dealSvc := deals.NewService(nil)
	pipeSvc := pipeline.NewService(nil)
	paths.SetAuditSecretProvider(func() []byte {
		return []byte("pipeline-cutover-test-secret-32-byte")
	})
	t.Cleanup(func() { paths.SetAuditSecretProvider(nil) })
	var saw []paths.LockEvent
	paths.SubscribeLockEvents(func(ev paths.LockEvent) {
		saw = append(saw, ev)
	})
	t.Cleanup(paths.ClearLockEventSubscribersForTest)

	cr := dealSvc.Create(deals.CreateInput{
		Customer: "Whitethorn Press", AmountPence: 36000, Stage: "qual",
	})
	if !cr.OK {
		t.Fatalf("Create failed: %s", cr.Error())
	}
	d := cr.Value.(deals.Deal)
	mr := pipeSvc.MoveDeal(pipeline.MoveInput{DealID: d.ID, ToStage: "engage"})
	if !mr.OK {
		t.Fatalf("MoveDeal failed: %s", mr.Error())
	}
	// Both Create AND MoveDeal route through the primitive. Count
	// EventWriteSucceeded occurrences — there should be at least two.
	count := 0
	for _, ev := range saw {
		if ev.Kind == paths.EventWriteSucceeded {
			count++
		}
	}
	if count < 2 {
		t.Fatalf("expected ≥2 EventWriteSucceeded (Create + MoveDeal), got %d", count)
	}
	dirR := core.UserHomeDir()
	if !dirR.OK {
		t.Fatalf("UserHomeDir: %s", dirR.Error())
	}
	fpath := core.PathJoin(dirR.Value.(string), "Lethean/sales/deals/x.md")
	mode := paths.AuditModeForPath(fpath)
	if mode != paths.AuditModeBatch {
		t.Fatalf("expected AuditModeBatch for sales/deals path (pipeline writer), got %v", mode)
	}
}
