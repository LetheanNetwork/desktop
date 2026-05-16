// SPDX-Licence-Identifier: EUPL-1.2

package contacts_test

import (
	"testing"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/sales/contacts"
)

// TestList_Empty_Good — empty contacts dir produces an empty list.
func TestList_Empty_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := contacts.NewService(nil)
	r := svc.List(contacts.ListInput{})
	if !r.OK {
		t.Fatalf("List failed: %s", r.Error())
	}
	out := r.Value.(contacts.ListOutput)
	if len(out.Contacts) != 0 {
		t.Fatalf("expected 0 contacts, got %d", len(out.Contacts))
	}
	if out.Total != 0 {
		t.Fatalf("expected total 0, got %d", out.Total)
	}
}

// TestCreate_WritesFile_Good — Create writes a Trix file with correct slug.
func TestCreate_WritesFile_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := contacts.NewService(nil)
	r := svc.Create(contacts.CreateInput{
		Name: "Ada Penley",
		Role: "CTO · Heritage Law",
		Next: "call · Fri",
	})
	if !r.OK {
		t.Fatalf("Create failed: %s", r.Error())
	}
	c := r.Value.(contacts.Contact)
	if c.Name != "Ada Penley" {
		t.Fatalf("expected name Ada Penley, got %q", c.Name)
	}
	if c.ID != "ada-penley" {
		t.Fatalf("expected id ada-penley, got %q", c.ID)
	}
}

// TestList_ReturnsCreated_Good — List returns the created entry.
func TestList_ReturnsCreated_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := contacts.NewService(nil)
	svc.Create(contacts.CreateInput{Name: "Ada Penley", Role: "CTO · Heritage Law"})
	svc.Create(contacts.CreateInput{Name: "Marcus Stannard", Role: "Partner · Stannard"})

	r := svc.List(contacts.ListInput{})
	if !r.OK {
		t.Fatalf("List failed: %s", r.Error())
	}
	out := r.Value.(contacts.ListOutput)
	if out.Total != 2 {
		t.Fatalf("expected total 2, got %d", out.Total)
	}
	if len(out.Contacts) != 2 {
		t.Fatalf("expected 2 contacts, got %d", len(out.Contacts))
	}
}

// TestList_FilterByWarmth_Good — warmth filter excludes non-matching contacts.
func TestList_FilterByWarmth_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := contacts.NewService(nil)

	// Touch one contact just now (hot) and one 30 days ago (cool).
	svc.Create(contacts.CreateInput{
		Name:      "Hot Contact",
		Role:      "CEO",
		LastTouch: core.Now(),
	})
	svc.Create(contacts.CreateInput{
		Name:      "Cool Contact",
		Role:      "CFO",
		LastTouch: core.Now().Add(-30 * 24 * core.Hour),
	})

	r := svc.List(contacts.ListInput{Warmth: "hot"})
	if !r.OK {
		t.Fatalf("List failed: %s", r.Error())
	}
	out := r.Value.(contacts.ListOutput)
	if len(out.Contacts) != 1 {
		t.Fatalf("expected 1 hot contact, got %d", len(out.Contacts))
	}
	if out.Contacts[0].Name != "Hot Contact" {
		t.Fatalf("expected Hot Contact, got %q", out.Contacts[0].Name)
	}
}

// TestUpdate_MutatesRole_Good — Update partial-updates role field.
func TestUpdate_MutatesRole_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := contacts.NewService(nil)
	svc.Create(contacts.CreateInput{Name: "Ada Penley", Role: "CTO · Heritage Law"})

	r := svc.Update(contacts.UpdateInput{
		ID:   "ada-penley",
		Role: "Partner · Heritage Law",
	})
	if !r.OK {
		t.Fatalf("Update failed: %s", r.Error())
	}
	c := r.Value.(contacts.Contact)
	if c.Role != "Partner · Heritage Law" {
		t.Fatalf("expected updated role, got %q", c.Role)
	}
}

// TestWarmth_HotWithinWeek_Good — warmthFor returns hot for ≤7 days.
func TestWarmth_HotWithinWeek_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := contacts.NewService(nil)
	svc.Create(contacts.CreateInput{
		Name:      "Recent",
		LastTouch: core.Now().Add(-3 * 24 * core.Hour),
	})
	r := svc.List(contacts.ListInput{})
	if !r.OK {
		t.Fatalf("List failed: %s", r.Error())
	}
	out := r.Value.(contacts.ListOutput)
	if len(out.Contacts) != 1 {
		t.Fatalf("expected 1 contact")
	}
	if out.Contacts[0].Warmth != "hot" {
		t.Fatalf("expected hot, got %q", out.Contacts[0].Warmth)
	}
}

// TestWarmth_CoolOverThreeWeeks_Bad — warmthFor returns cool for >21 days.
func TestWarmth_CoolOverThreeWeeks_Bad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := contacts.NewService(nil)
	svc.Create(contacts.CreateInput{
		Name:      "Old Contact",
		LastTouch: core.Now().Add(-30 * 24 * core.Hour),
	})
	r := svc.List(contacts.ListInput{})
	if !r.OK {
		t.Fatalf("List failed: %s", r.Error())
	}
	out := r.Value.(contacts.ListOutput)
	if len(out.Contacts) != 1 {
		t.Fatalf("expected 1 contact")
	}
	if out.Contacts[0].Warmth != "cool" {
		t.Fatalf("expected cool, got %q", out.Contacts[0].Warmth)
	}
}

// TestCreate_DuplicateName_Bad — second Create for same slug returns core.Fail.
func TestCreate_DuplicateName_Bad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := contacts.NewService(nil)
	svc.Create(contacts.CreateInput{Name: "Ada Penley"})
	r := svc.Create(contacts.CreateInput{Name: "Ada Penley"})
	if r.OK {
		t.Fatalf("expected failure for duplicate slug, got OK")
	}
}

// TestCreate_EmptyName_Ugly — Create with empty name returns core.Fail.
func TestCreate_EmptyName_Ugly(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := contacts.NewService(nil)
	r := svc.Create(contacts.CreateInput{Name: ""})
	if r.OK {
		t.Fatalf("expected failure for empty name, got OK")
	}
}
