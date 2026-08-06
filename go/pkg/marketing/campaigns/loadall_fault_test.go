// SPDX-Licence-Identifier: EUPL-1.2

// loadall_fault_test.go — fault injection for loadCampaigns' per-entry
// skip branches (RFC.stage-e-encrypt-at-rest v2 §4.1: "do NOT abort
// whole List on one bad file") plus the List/Get/Update wails.go
// branches that only trigger with a populated, multi-campaign set
// (State filter, LiveCount/ScheduledCount tallies, not-found
// forwarding, session-locked-after-atrest forwarding, full patch-field
// coverage). Mirrors pkg/sales/deals/loadall_fault_test.go's precedent.

package campaigns_test

import (
	"os"
	"testing"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/marketing/campaigns"
)

// TestCampaigns_LoadCampaigns_CorruptLthn_SkippedNotAborted_Ugly — a
// .lthn file too short/malformed to survive loadHeaderOnly is skipped;
// a healthy .md sibling still surfaces via List.
func TestCampaigns_LoadCampaigns_CorruptLthn_SkippedNotAborted_Ugly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newAtRestTestSvc(t)

	dir := core.PathJoin(home, "Lethean", "marketing", "campaigns")
	if mk := core.MkdirAll(dir, 0o700); !mk.OK {
		t.Fatalf("MkdirAll: %s", mk.Error())
	}
	if w := core.WriteFile(core.PathJoin(dir, "corrupt-entry.lthn"), []byte("not-a-real-envelope"), 0o600); !w.OK {
		t.Fatalf("seed corrupt .lthn: %s", w.Error())
	}

	cr := svc.Create(campaigns.CreateInput{Name: "Healthy encrypted campaign"})
	if !cr.OK {
		t.Fatalf("seed Create: %s", cr.Error())
	}

	r := svc.List(campaigns.ListInput{})
	if !r.OK {
		t.Fatalf("List must tolerate one corrupt .lthn entry: %s", r.Error())
	}
	out := r.Value.(campaigns.ListOutput)
	if len(out.Campaigns) != 1 {
		t.Fatalf("expected exactly the healthy record (corrupt one skipped), got %d", len(out.Campaigns))
	}
}

// TestCampaigns_LoadCampaigns_UnreadableMd_SkippedNotAborted_Bad — a
// .md file with permissions denying read hits loadCampaigns' `if
// !raw.OK { continue }` branch.
func TestCampaigns_LoadCampaigns_UnreadableMd_SkippedNotAborted_Bad(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root — permission-denial fault injection does not apply")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := campaigns.NewService(nil)
	svc.SetSessionGate(&stubSessionGate{ids: []string{"acct-legacy"}})

	dir := core.PathJoin(home, "Lethean", "marketing", "campaigns")
	if mk := core.MkdirAll(dir, 0o700); !mk.OK {
		t.Fatalf("MkdirAll: %s", mk.Error())
	}
	unreadable := core.PathJoin(dir, "unreadable-campaign.md")
	legacy := []byte("---\nid: unreadable-campaign\nname: Locked\nstate: draft\nreach: \"\"\nconvert: \"\"\nspend: \"\"\nchannel: earned\n---\n")
	if w := core.WriteFile(unreadable, legacy, 0o600); !w.OK {
		t.Fatalf("seed .md: %s", w.Error())
	}
	if err := os.Chmod(unreadable, 0o000); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(unreadable, 0o600) })

	cr := svc.Create(campaigns.CreateInput{Name: "Healthy legacy campaign"})
	if !cr.OK {
		t.Fatalf("seed Create: %s", cr.Error())
	}

	r := svc.List(campaigns.ListInput{})
	if !r.OK {
		t.Fatalf("List must tolerate an unreadable .md entry: %s", r.Error())
	}
	out := r.Value.(campaigns.ListOutput)
	if len(out.Campaigns) != 1 {
		t.Fatalf("expected exactly the healthy record (unreadable one skipped), got %d", len(out.Campaigns))
	}
}

// TestCampaigns_LoadCampaigns_MalformedMdYaml_SkippedNotAborted_Bad —
// a .md file whose frontmatter fails yaml.Unmarshal hits loadCampaigns'
// parseCampaign-error `continue` branch.
func TestCampaigns_LoadCampaigns_MalformedMdYaml_SkippedNotAborted_Bad(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := campaigns.NewService(nil)
	svc.SetSessionGate(&stubSessionGate{ids: []string{"acct-legacy"}})

	dir := core.PathJoin(home, "Lethean", "marketing", "campaigns")
	if mk := core.MkdirAll(dir, 0o700); !mk.OK {
		t.Fatalf("MkdirAll: %s", mk.Error())
	}
	bad := core.PathJoin(dir, "malformed-campaign.md")
	if w := core.WriteFile(bad, []byte("---\n[not: valid: yaml\n---\nbody"), 0o600); !w.OK {
		t.Fatalf("seed malformed .md: %s", w.Error())
	}

	cr := svc.Create(campaigns.CreateInput{Name: "Healthy legacy campaign two"})
	if !cr.OK {
		t.Fatalf("seed Create: %s", cr.Error())
	}

	r := svc.List(campaigns.ListInput{})
	if !r.OK {
		t.Fatalf("List must tolerate a malformed .md entry: %s", r.Error())
	}
	out := r.Value.(campaigns.ListOutput)
	if len(out.Campaigns) != 1 {
		t.Fatalf("expected exactly the healthy record (malformed YAML skipped), got %d", len(out.Campaigns))
	}
}

// TestCampaigns_LoadCampaigns_SubdirEntry_Ignored_Good — a stray
// subdirectory inside the campaigns dir must be ignored by both
// loadCampaigns passes.
func TestCampaigns_LoadCampaigns_SubdirEntry_Ignored_Good(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newAtRestTestSvc(t)

	dir := core.PathJoin(home, "Lethean", "marketing", "campaigns")
	if mk := core.MkdirAll(core.PathJoin(dir, "stray-subdir"), 0o700); !mk.OK {
		t.Fatalf("MkdirAll: %s", mk.Error())
	}

	cr := svc.Create(campaigns.CreateInput{Name: "Campaign beside a stray subdir"})
	if !cr.OK {
		t.Fatalf("seed Create: %s", cr.Error())
	}

	r := svc.List(campaigns.ListInput{})
	if !r.OK {
		t.Fatalf("List must ignore a stray subdirectory: %s", r.Error())
	}
}

// seedLegacyCampaign writes a legacy plaintext .md campaign directly to
// disk with a fixed id, sidestepping Create()'s Unix-second id
// collision risk when a test needs two or more distinct campaigns fast.
func seedLegacyCampaign(t *testing.T, dir, id, name, state, channel string) {
	t.Helper()
	body := "---\nid: " + id + "\nname: " + name + "\nstate: " + state +
		"\nreach: \"\"\nconvert: \"\"\nspend: \"\"\nchannel: " + channel + "\n---\n"
	if w := core.WriteFile(core.PathJoin(dir, id+".md"), []byte(body), 0o600); !w.OK {
		t.Fatalf("seedLegacyCampaign WriteFile: %s", w.Error())
	}
}

// TestList_StateFilter_ExcludesNonMatching_Good — with two distinct
// campaigns on disk, filtering by a state present on only one exercises
// both the keep and the implicit skip path of List's filter loop, and
// pins LiveCount/ScheduledCount computed from the unfiltered set.
func TestList_StateFilter_ExcludesNonMatching_Good(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := core.PathJoin(home, "Lethean", "marketing", "campaigns")
	if mk := core.MkdirAll(dir, 0o700); !mk.OK {
		t.Fatalf("MkdirAll: %s", mk.Error())
	}
	seedLegacyCampaign(t, dir, "campaign-live", "Live launch", "live", "earned")
	seedLegacyCampaign(t, dir, "campaign-sched", "Scheduled launch", "scheduled", "paid")

	svc := campaigns.NewService(nil)
	svc.SetSessionGate(&stubSessionGate{ids: []string{"acct-1"}})

	r := svc.List(campaigns.ListInput{State: "scheduled"})
	if !r.OK {
		t.Fatalf("List: %s", r.Error())
	}
	out := r.Value.(campaigns.ListOutput)
	if len(out.Campaigns) != 1 || out.Campaigns[0].ID != "campaign-sched" {
		t.Fatalf("expected exactly campaign-sched, got %+v", out.Campaigns)
	}
	if out.LiveCount != 1 {
		t.Fatalf("expected LiveCount=1 (computed pre-filter), got %d", out.LiveCount)
	}
	if out.ScheduledCount != 1 {
		t.Fatalf("expected ScheduledCount=1 (computed pre-filter), got %d", out.ScheduledCount)
	}
}

// TestUpdate_NotFound_Bad — Update on a syntactically valid but
// nonexistent ID forwards loadOne's not-found error.
func TestUpdate_NotFound_Bad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newTestSvc(t)
	r := svc.Update(campaigns.UpdateInput{ID: "does-not-exist-anywhere", State: "live"})
	if r.OK {
		t.Fatal("Update on a nonexistent id must fail")
	}
	if !core.Contains(r.Error(), "not found") {
		t.Fatalf("expected not-found error, got %s", r.Error())
	}
}

// TestUpdate_PatchesAllFields_Good — Update's Reach/Convert/Spend/
// Channel/Body patch branches, symmetric to the State branch already
// covered elsewhere. Drives all five together.
func TestUpdate_PatchesAllFields_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newTestSvc(t)
	cr := svc.Create(campaigns.CreateInput{Name: "Patchable campaign"})
	if !cr.OK {
		t.Fatalf("Create: %s", cr.Error())
	}
	id := cr.Value.(campaigns.Campaign).ID

	r := svc.Update(campaigns.UpdateInput{
		ID: id, Reach: "12,000", Convert: "3.2%", Spend: "£450",
		Channel: "paid", Body: "Updated narrative.",
	})
	if !r.OK {
		t.Fatalf("Update: %s", r.Error())
	}
	c := r.Value.(campaigns.Campaign)
	if c.Reach != "12,000" || c.Convert != "3.2%" || c.Spend != "£450" || c.Channel != "paid" || c.Body != "Updated narrative." {
		t.Fatalf("unexpected patched campaign: %+v", c)
	}
}

// TestGet_SessionLockedViaNarrowGateAfterAtRestWrite_Bad — an .lthn
// record exists on disk; the gate is then swapped to a NARROW
// stubSessionGate (satisfies SessionGate but not accountKeyProvider).
// loadOne's atrestWriterFor check fails closed with the typed
// "campaigns.session.locked" code, which Get forwards verbatim.
func TestGet_SessionLockedViaNarrowGateAfterAtRestWrite_Bad(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newAtRestTestSvc(t)

	cr := svc.Create(campaigns.CreateInput{Name: "Narrow-gate probe"})
	if !cr.OK {
		t.Fatalf("seed Create: %s", cr.Error())
	}
	id := cr.Value.(campaigns.Campaign).ID

	svc.SetSessionGate(&stubSessionGate{ids: []string{"acct-1"}})

	g := svc.Get(id)
	if g.OK {
		t.Fatal("Get on an .lthn record with a narrow gate must fail")
	}
	if !core.Contains(g.Error(), "campaigns.session.locked") {
		t.Fatalf("expected campaigns.session.locked, got %s", g.Error())
	}
}

// TestCreate_CampaignsDirFails_Bad — Create forwards campaignsDir's
// failure when $HOME is unavailable.
func TestCreate_CampaignsDirFails_Bad(t *testing.T) {
	t.Setenv("HOME", "")
	svc := campaigns.NewService(nil)
	svc.SetSessionGate(&stubSessionGate{ids: []string{"acct-1"}})
	r := svc.Create(campaigns.CreateInput{Name: "Homeless campaign"})
	if r.OK {
		t.Fatal("Create must fail when campaignsDir() cannot resolve $HOME")
	}
}

// TestCreate_AllSymbolsName_DefaultsToCampaignSlug_Good — a name that
// slugifies to empty ("!!!") falls back to the literal "campaign" slug
// stem so the generated ID stays non-empty.
func TestCreate_AllSymbolsName_DefaultsToCampaignSlug_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newTestSvc(t)
	r := svc.Create(campaigns.CreateInput{Name: "!!!"})
	if !r.OK {
		t.Fatalf("Create: %s", r.Error())
	}
	c := r.Value.(campaigns.Campaign)
	if !core.Contains(c.ID, "campaign-") {
		t.Fatalf("expected ID to fall back to campaign-<ts> stem, got %q", c.ID)
	}
}
