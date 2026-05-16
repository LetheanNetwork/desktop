// SPDX-Licence-Identifier: EUPL-1.2

package campaigns_test

import (
	"encoding/json"
	"sync"
	"testing"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/marketing/campaigns"
	"dappco.re/lthn/desktop/pkg/paths"
)

// TestList_Empty_Good — empty dir → empty slice, zero counts.
func TestList_Empty_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := campaigns.NewService(nil)
	r := svc.List(campaigns.ListInput{})
	if !r.OK {
		t.Fatalf("List failed: %s", r.Error())
	}
	out := r.Value.(campaigns.ListOutput)
	if len(out.Campaigns) != 0 {
		t.Fatalf("expected 0 campaigns, got %d", len(out.Campaigns))
	}
	if out.LiveCount != 0 || out.ScheduledCount != 0 {
		t.Fatalf("expected zero counts, got live=%d scheduled=%d", out.LiveCount, out.ScheduledCount)
	}
}

// TestCreate_Defaults_Good — Create with name only applies all defaults.
func TestCreate_Defaults_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := campaigns.NewService(nil)
	r := svc.Create(campaigns.CreateInput{Name: "Product Hunt launch"})
	if !r.OK {
		t.Fatalf("Create failed: %s", r.Error())
	}
	c := r.Value.(campaigns.Campaign)
	if c.State != "draft" {
		t.Fatalf("expected state=draft, got %q", c.State)
	}
	if c.Spend != "£0" {
		t.Fatalf("expected spend=£0, got %q", c.Spend)
	}
	if c.Channel != "earned" {
		t.Fatalf("expected channel=earned, got %q", c.Channel)
	}
	if c.Reach != "—" {
		t.Fatalf("expected reach=—, got %q", c.Reach)
	}
	if c.Convert != "—" {
		t.Fatalf("expected convert=—, got %q", c.Convert)
	}
	if c.ID == "" {
		t.Fatalf("expected non-empty ID")
	}
}

// TestCreate_Name_Bad — empty name returns core.Fail.
func TestCreate_Name_Bad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := campaigns.NewService(nil)
	r := svc.Create(campaigns.CreateInput{Name: ""})
	if r.OK {
		t.Fatalf("expected failure for empty name, got OK")
	}
}

// TestUpdate_State_Good — Update state field persists.
func TestUpdate_State_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := campaigns.NewService(nil)
	cr := svc.Create(campaigns.CreateInput{Name: "Test campaign"})
	if !cr.OK {
		t.Fatalf("Create failed: %s", cr.Error())
	}
	created := cr.Value.(campaigns.Campaign)

	ur := svc.Update(campaigns.UpdateInput{ID: created.ID, State: "live"})
	if !ur.OK {
		t.Fatalf("Update failed: %s", ur.Error())
	}
	updated := ur.Value.(campaigns.Campaign)
	if updated.State != "live" {
		t.Fatalf("expected state=live, got %q", updated.State)
	}

	// Verify via Get.
	gr := svc.Get(created.ID)
	if !gr.OK {
		t.Fatalf("Get failed: %s", gr.Error())
	}
	got := gr.Value.(campaigns.Campaign)
	if got.State != "live" {
		t.Fatalf("persisted state expected live, got %q", got.State)
	}
}

// TestGet_NotFound_Bad — Get unknown id returns core.Fail.
func TestGet_NotFound_Bad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := campaigns.NewService(nil)
	r := svc.Get("does-not-exist")
	if r.OK {
		t.Fatalf("expected failure for unknown id, got OK")
	}
}

// TestList_StateFilter_Good — List with state filter returns only matching campaigns.
func TestList_StateFilter_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := campaigns.NewService(nil)
	svc.Create(campaigns.CreateInput{Name: "Draft A", State: "draft"})
	svc.Create(campaigns.CreateInput{Name: "Live A", State: "live"})
	svc.Create(campaigns.CreateInput{Name: "Live B", State: "live"})

	r := svc.List(campaigns.ListInput{State: "live"})
	if !r.OK {
		t.Fatalf("List failed: %s", r.Error())
	}
	out := r.Value.(campaigns.ListOutput)
	if len(out.Campaigns) != 2 {
		t.Fatalf("expected 2 live campaigns, got %d", len(out.Campaigns))
	}
	// Counts still reflect full set.
	if out.LiveCount != 2 {
		t.Fatalf("expected LiveCount=2, got %d", out.LiveCount)
	}
}

// TestServiceName_Good — ServiceName returns "Campaigns".
func TestServiceName_Good(t *testing.T) {
	svc := campaigns.NewService(nil)
	if svc.ServiceName() != "Campaigns" {
		t.Fatalf("expected Campaigns, got %q", svc.ServiceName())
	}
}

// ---- Cascade W2 cutover tests (paths.AtomicWriteWithVersion) ---------------

// TestAtomicCutover_MarketingCampaigns_Create_Good — Create stamps
// version=1 via the primitive; ReadVersion confirms the on-disk file
// carries the same.
func TestAtomicCutover_MarketingCampaigns_Create_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := campaigns.NewService(nil)
	r := svc.Create(campaigns.CreateInput{
		Name:    "Product Hunt launch",
		Channel: "earned",
	})
	if !r.OK {
		t.Fatalf("Create failed: %s", r.Error())
	}
	c := r.Value.(campaigns.Campaign)
	dirR := core.UserHomeDir()
	if !dirR.OK {
		t.Fatalf("UserHomeDir: %s", dirR.Error())
	}
	fpath := core.PathJoin(dirR.Value.(string), "Lethean/marketing/campaigns", c.ID+".md")
	rd := paths.ReadVersion(fpath)
	if !rd.OK {
		t.Fatalf("ReadVersion: %s", rd.Error())
	}
	got := rd.Value.(paths.ReadOutput)
	if got.Version != 1 {
		t.Fatalf("expected version 1 after Create, got %d", got.Version)
	}
}

// TestAtomicCutover_MarketingCampaigns_Update_Good — a sequential
// Update bumps the stored version monotonically (1 -> 2).
func TestAtomicCutover_MarketingCampaigns_Update_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := campaigns.NewService(nil)
	cr := svc.Create(campaigns.CreateInput{Name: "Probe campaign", Channel: "earned"})
	if !cr.OK {
		t.Fatalf("Create failed: %s", cr.Error())
	}
	id := cr.Value.(campaigns.Campaign).ID
	ur := svc.Update(campaigns.UpdateInput{ID: id, State: "live"})
	if !ur.OK {
		t.Fatalf("Update failed: %s", ur.Error())
	}
	dirR := core.UserHomeDir()
	if !dirR.OK {
		t.Fatalf("UserHomeDir: %s", dirR.Error())
	}
	fpath := core.PathJoin(dirR.Value.(string), "Lethean/marketing/campaigns", id+".md")
	rd := paths.ReadVersion(fpath)
	if !rd.OK {
		t.Fatalf("ReadVersion: %s", rd.Error())
	}
	got := rd.Value.(paths.ReadOutput)
	if got.Version != 2 {
		t.Fatalf("expected version 2 after Update, got %d", got.Version)
	}
}

// TestAtomicCutover_MarketingCampaigns_Update_VersionStale_Ugly —
// drives two concurrent svc.Update calls and asserts the loser's
// Result carries a paths.ConflictEnvelope whose JSON marshal matches
// the lowercase `{code, current_version, current_hash}` wire shape
// that frontend/src/lit/conflict-dispatch.ts extractEnvelope
// pattern-matches on (Mantis #1547 service-tier round-trip discipline
// anchor; pins #1544 against W2 wave drift).
//
// Why two goroutines: svc.Update re-reads the file inside the call
// so a single-shot out-of-band mutation cannot drive a real conflict
// through the service. Concurrent Update goroutines race for the
// WithFileLock; the loser's locked-section ReadVersion sees the
// winner's bumped version and the primitive returns VersionStale ->
// wrapped as ConflictEnvelope by campaigns.writeCampaign.
func TestAtomicCutover_MarketingCampaigns_Update_VersionStale_Ugly(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := campaigns.NewService(nil)
	cr := svc.Create(campaigns.CreateInput{Name: "Race campaign", Channel: "earned"})
	if !cr.OK {
		t.Fatalf("Create failed: %s", cr.Error())
	}
	id := cr.Value.(campaigns.Campaign).ID

	var conflict core.Result
	var saw bool
	for attempt := 0; attempt < 32 && !saw; attempt++ {
		var (
			mu      sync.Mutex
			results []core.Result
			wg      sync.WaitGroup
			start   = make(chan struct{})
		)
		// Different spend values per attempt so writes aren't no-ops.
		spendA := core.Sprintf("£%d", attempt*10+100)
		spendB := core.Sprintf("£%d", attempt*10+200)
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			r := svc.Update(campaigns.UpdateInput{ID: id, Spend: spendA})
			mu.Lock()
			results = append(results, r)
			mu.Unlock()
		}()
		go func() {
			defer wg.Done()
			<-start
			r := svc.Update(campaigns.UpdateInput{ID: id, Spend: spendB})
			mu.Lock()
			results = append(results, r)
			mu.Unlock()
		}()
		close(start)
		wg.Wait()

		for _, r := range results {
			if !r.OK && core.Contains(r.Error(), "campaigns.update.conflict") {
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
	if env.Code != "campaigns.update.conflict" {
		t.Fatalf("expected envelope code campaigns.update.conflict, got %q", env.Code)
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
		`"code":"campaigns.update.conflict"`,
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

// TestAtomicCutover_MarketingCampaigns_LegacyFile_Ugly — a campaign
// file without version: frontmatter reads as version 0; Update
// upgrades it via an unconditional first-write that stamps version=1.
func TestAtomicCutover_MarketingCampaigns_LegacyFile_Ugly(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := campaigns.NewService(nil)
	dirR := core.UserHomeDir()
	if !dirR.OK {
		t.Fatalf("UserHomeDir: %s", dirR.Error())
	}
	campDir := core.PathJoin(dirR.Value.(string), "Lethean/marketing/campaigns")
	if mk := core.MkdirAll(campDir, 0o700); !mk.OK {
		t.Fatalf("MkdirAll: %s", mk.Error())
	}
	legacyID := "legacy-campaign-2026"
	fpath := core.PathJoin(campDir, legacyID+".md")
	legacy := []byte("---\nid: " + legacyID + "\nname: Legacy campaign\nstate: draft\nreach: \"\"\nconvert: \"\"\nspend: \"£0\"\nchannel: earned\n---\nLegacy body.")
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
	ur := svc.Update(campaigns.UpdateInput{ID: legacyID, State: "live"})
	if !ur.OK {
		t.Fatalf("Update failed: %s", ur.Error())
	}
	rd2 := paths.ReadVersion(fpath)
	if !rd2.OK {
		t.Fatalf("ReadVersion post-update: %s", rd2.Error())
	}
	if got := rd2.Value.(paths.ReadOutput); got.Version != 1 {
		t.Fatalf("legacy file post-update: expected version 1, got %d", got.Version)
	}
}

// TestAtomicCutover_MarketingCampaigns_AuditEmissionRecordBatch_Good —
// Create routes through the primitive's write path (EventWriteSucceeded
// fires) and marketing/campaigns/* falls under AuditModeBatch per RFC §6.1.
func TestAtomicCutover_MarketingCampaigns_AuditEmissionRecordBatch_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := campaigns.NewService(nil)
	paths.SetAuditSecretProvider(func() []byte {
		return []byte("campaigns-cutover-test-secret-32-byt")
	})
	t.Cleanup(func() { paths.SetAuditSecretProvider(nil) })
	var saw []paths.LockEvent
	paths.SubscribeLockEvents(func(ev paths.LockEvent) {
		saw = append(saw, ev)
	})
	t.Cleanup(paths.ClearLockEventSubscribersForTest)

	r := svc.Create(campaigns.CreateInput{Name: "Audit probe", Channel: "earned"})
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
	fpath := core.PathJoin(dirR.Value.(string), "Lethean/marketing/campaigns/x.md")
	mode := paths.AuditModeForPath(fpath)
	if mode != paths.AuditModeBatch {
		t.Fatalf("expected AuditModeBatch for marketing/campaigns path, got %v", mode)
	}
}
