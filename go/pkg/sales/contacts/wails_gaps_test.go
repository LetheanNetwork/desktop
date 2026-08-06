// SPDX-Licence-Identifier: EUPL-1.2

// wails_gaps_test.go — fault injection + branch cover for wails.go
// that contacts_test.go's happy-path CRUD scenarios don't reach: the
// free-text Query filter (and its underlying containsCI call), the
// warmth insertion-sort actually swapping out-of-order entries, Get/
// Update's not-found and malformed-file branches, Create's slugify-
// to-empty-id rejection, contactsDir() failure surfacing through each
// entry point, and atomicWriteFile's non-conflict failure wrap.

package contacts_test

import (
	"os"
	"testing"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/sales/contacts"
)

// --- List: Query filter + warmth sort -----------------------------------

func TestList_QueryFiltersByNameOrRole_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newTestSvc(t)
	svc.Create(contacts.CreateInput{Name: "Ada Penley", Role: "CTO · Heritage Law"})
	svc.Create(contacts.CreateInput{Name: "Marcus Stannard", Role: "Partner · Stannard"})

	byName := svc.List(contacts.ListInput{Query: "penley"})
	if !byName.OK {
		t.Fatalf("List failed: %s", byName.Error())
	}
	out := byName.Value.(contacts.ListOutput)
	if len(out.Contacts) != 1 || out.Contacts[0].Name != "Ada Penley" {
		t.Fatalf("expected Ada Penley matched by name substring, got %+v", out.Contacts)
	}

	byRole := svc.List(contacts.ListInput{Query: "stannard"})
	if !byRole.OK {
		t.Fatalf("List failed: %s", byRole.Error())
	}
	out2 := byRole.Value.(contacts.ListOutput)
	if len(out2.Contacts) != 1 || out2.Contacts[0].Name != "Marcus Stannard" {
		t.Fatalf("expected Marcus Stannard matched by role substring, got %+v", out2.Contacts)
	}
}

func TestList_QueryNoMatch_Bad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newTestSvc(t)
	svc.Create(contacts.CreateInput{Name: "Ada Penley", Role: "CTO"})

	r := svc.List(contacts.ListInput{Query: "nonexistent"})
	if !r.OK {
		t.Fatalf("List failed: %s", r.Error())
	}
	out := r.Value.(contacts.ListOutput)
	if len(out.Contacts) != 0 {
		t.Fatalf("expected 0 matches, got %d", len(out.Contacts))
	}
}

// TestList_WarmthSortActuallySwaps_Good — three contacts created in
// cool/warm/hot order must come back hot/warm/cool: the insertion sort
// in List must perform real swaps, not just early-break on an
// already-sorted run.
func TestList_WarmthSortActuallySwaps_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newTestSvc(t)
	svc.Create(contacts.CreateInput{Name: "Cool One", LastTouch: core.Now().Add(-30 * 24 * core.Hour)})
	svc.Create(contacts.CreateInput{Name: "Warm One", LastTouch: core.Now().Add(-10 * 24 * core.Hour)})
	svc.Create(contacts.CreateInput{Name: "Hot One", LastTouch: core.Now()})

	r := svc.List(contacts.ListInput{})
	if !r.OK {
		t.Fatalf("List failed: %s", r.Error())
	}
	out := r.Value.(contacts.ListOutput)
	if len(out.Contacts) != 3 {
		t.Fatalf("expected 3 contacts, got %d", len(out.Contacts))
	}
	got := []string{out.Contacts[0].Warmth, out.Contacts[1].Warmth, out.Contacts[2].Warmth}
	want := []string{"hot", "warm", "cool"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected sorted %v, got %v", want, got)
		}
	}
}

// TestList_LimitTruncates_Good — a Limit smaller than the filtered
// result count truncates the slice.
func TestList_LimitTruncates_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newTestSvc(t)
	svc.Create(contacts.CreateInput{Name: "A"})
	svc.Create(contacts.CreateInput{Name: "B"})
	svc.Create(contacts.CreateInput{Name: "C"})

	r := svc.List(contacts.ListInput{Limit: 2})
	if !r.OK {
		t.Fatalf("List failed: %s", r.Error())
	}
	out := r.Value.(contacts.ListOutput)
	if len(out.Contacts) != 2 {
		t.Fatalf("expected 2 (limited), got %d", len(out.Contacts))
	}
	if out.Total != 3 {
		t.Fatalf("expected Total to stay unfiltered at 3, got %d", out.Total)
	}
}

func TestList_Bad_ContactsDirUnavailable(t *testing.T) {
	t.Setenv("HOME", "")
	svc := newTestSvc(t)
	r := svc.List(contacts.ListInput{})
	if r.OK {
		t.Fatal("List must fail when contactsDir() fails")
	}
}

// --- Get: not-found, malformed file, dir failure ------------------------

func TestGet_Bad_NotFound(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newTestSvc(t)
	r := svc.Get(contacts.GetInput{ID: "ghost"})
	if r.OK {
		t.Fatal("Get must fail for a missing contact")
	}
}

func TestGet_Bad_MalformedFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newTestSvc(t)
	dir := core.PathJoin(home, "Lethean", "sales", "contacts")
	if mk := core.MkdirAll(dir, 0o700); !mk.OK {
		t.Fatalf("MkdirAll: %s", mk.Error())
	}
	bad := core.PathJoin(dir, "broken.md")
	if w := core.WriteFile(bad, []byte("---\n[not valid yaml at all: :\n"), 0o600); !w.OK {
		t.Fatalf("seed WriteFile: %s", w.Error())
	}

	r := svc.Get(contacts.GetInput{ID: "broken"})
	if r.OK {
		t.Fatal("Get must fail for a malformed record")
	}
}

func TestGet_Bad_ContactsDirUnavailable(t *testing.T) {
	t.Setenv("HOME", "")
	svc := newTestSvc(t)
	r := svc.Get(contacts.GetInput{ID: "ada-penley"})
	if r.OK {
		t.Fatal("Get must fail when contactsDir() fails")
	}
}

// --- Create: empty slugified id, dir failure ----------------------------

// TestCreate_Bad_NameSlugifiesToEmpty — a name made entirely of
// symbols passes the "Name is required" check (Name != "") but
// slugifies to the empty string, which must be rejected by
// paths.IsValidID before ever touching the filesystem.
func TestCreate_Bad_NameSlugifiesToEmpty(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newTestSvc(t)
	r := svc.Create(contacts.CreateInput{Name: "***"})
	if r.OK {
		t.Fatal("Create must reject a name that slugifies to an empty id")
	}
}

func TestCreate_Bad_ContactsDirUnavailable(t *testing.T) {
	t.Setenv("HOME", "")
	svc := newTestSvc(t)
	r := svc.Create(contacts.CreateInput{Name: "Ada Penley"})
	if r.OK {
		t.Fatal("Create must fail when contactsDir() fails")
	}
}

// --- Update: not-found, malformed file, dir failure ---------------------

func TestUpdate_Bad_NotFound(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newTestSvc(t)
	r := svc.Update(contacts.UpdateInput{ID: "ghost", Role: "x"})
	if r.OK {
		t.Fatal("Update must fail for a missing contact")
	}
}

func TestUpdate_Bad_MalformedFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newTestSvc(t)
	dir := core.PathJoin(home, "Lethean", "sales", "contacts")
	if mk := core.MkdirAll(dir, 0o700); !mk.OK {
		t.Fatalf("MkdirAll: %s", mk.Error())
	}
	bad := core.PathJoin(dir, "broken.md")
	if w := core.WriteFile(bad, []byte("---\n[not valid yaml at all: :\n"), 0o600); !w.OK {
		t.Fatalf("seed WriteFile: %s", w.Error())
	}

	r := svc.Update(contacts.UpdateInput{ID: "broken", Role: "x"})
	if r.OK {
		t.Fatal("Update must fail for a malformed record")
	}
}

func TestUpdate_Bad_ContactsDirUnavailable(t *testing.T) {
	t.Setenv("HOME", "")
	svc := newTestSvc(t)
	r := svc.Update(contacts.UpdateInput{ID: "ada-penley", Role: "x"})
	if r.OK {
		t.Fatal("Update must fail when contactsDir() fails")
	}
}

// TestUpdate_Good_AllOptionalFields — exercises every non-zero-field
// branch in one call (Role, LastTouch, Next, Notes) rather than only
// Role as the existing TestUpdate_MutatesRole_Good does.
func TestUpdate_Good_AllOptionalFields(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newTestSvc(t)
	svc.Create(contacts.CreateInput{Name: "Ada Penley", Role: "CTO"})

	touch := core.Now().Add(-2 * core.Hour)
	r := svc.Update(contacts.UpdateInput{
		ID: "ada-penley", Role: "Partner", LastTouch: touch,
		Next: "contract", Notes: "## Discussion",
	})
	if !r.OK {
		t.Fatalf("Update failed: %s", r.Error())
	}
	c := r.Value.(contacts.Contact)
	if c.Role != "Partner" || c.Next != "contract" {
		t.Fatalf("expected updated fields, got %+v", c)
	}

	detail := svc.Get(contacts.GetInput{ID: "ada-penley"})
	if !detail.OK {
		t.Fatalf("Get failed: %s", detail.Error())
	}
	if detail.Value.(contacts.ContactDetail).Notes != "## Discussion" {
		t.Fatalf("expected Notes to be applied, got %q", detail.Value.(contacts.ContactDetail).Notes)
	}
}

// --- atomicWriteFile: non-conflict failure wrap --------------------------

// TestUpdate_Bad_WriteFailsNonConflict — a permission-denied write
// (contacts dir chmod'd read-only after Create, before Update) must
// surface through atomicWriteFile's generic core.E wrap rather than
// the ConflictEnvelope branch, since paths.VersionStaleFromError
// won't match a plain permission error.
func TestUpdate_Bad_WriteFailsNonConflict(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root — permission-denial fault injection does not apply")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newTestSvc(t)
	cr := svc.Create(contacts.CreateInput{Name: "Ada Penley", Role: "CTO"})
	if !cr.OK {
		t.Fatalf("seed Create: %s", cr.Error())
	}

	dir := core.PathJoin(home, "Lethean", "sales", "contacts")
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	r := svc.Update(contacts.UpdateInput{ID: "ada-penley", Role: "Partner"})
	if r.OK {
		t.Fatal("Update must fail when the contacts dir denies write")
	}
	if core.Contains(r.Error(), "contacts.update.conflict") {
		t.Fatalf("expected a non-conflict write failure, got the conflict-shaped message: %s", r.Error())
	}
}
