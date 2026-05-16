// SPDX-Licence-Identifier: EUPL-1.2

package campaigns_test

import (
	"testing"

	"dappco.re/lthn/desktop/pkg/marketing/campaigns"
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
