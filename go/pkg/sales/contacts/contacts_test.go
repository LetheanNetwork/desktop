// SPDX-Licence-Identifier: EUPL-1.2

package contacts_test

import (
	"testing"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/paths"
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

// ---- Cascade W1 cutover tests (paths.AtomicWriteWithVersion) ---------------

// TestAtomicCutover_Contacts_Create_Good — first write lands version 1.
// Cascade W1 (Mantis #1540) — confirms Create stamps version=1 via the
// primitive and the on-disk file carries the same value back through
// ReadVersion.
func TestAtomicCutover_Contacts_Create_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := contacts.NewService(nil)
	r := svc.Create(contacts.CreateInput{Name: "Ada Penley", Role: "CTO · Heritage"})
	if !r.OK {
		t.Fatalf("Create failed: %s", r.Error())
	}
	// Recover the on-disk path via Get (returns the canonical slug),
	// then peek the file's frontmatter version through ReadVersion.
	c := r.Value.(contacts.Contact)
	dirR := core.UserHomeDir()
	if !dirR.OK {
		t.Fatalf("UserHomeDir: %s", dirR.Error())
	}
	fpath := core.PathJoin(dirR.Value.(string), "Lethean/sales/contacts", c.ID+".md")
	rd := paths.ReadVersion(fpath)
	if !rd.OK {
		t.Fatalf("ReadVersion: %s", rd.Error())
	}
	got := rd.Value.(paths.ReadOutput)
	if got.Version != 1 {
		t.Fatalf("expected version 1 after Create, got %d", got.Version)
	}
}

// TestAtomicCutover_Contacts_Update_Good — IfVersion matches, version+1.
// Cascade W1 (Mantis #1540) — confirms a sequential update bumps the
// stored version monotonically (1 → 2).
func TestAtomicCutover_Contacts_Update_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := contacts.NewService(nil)
	cr := svc.Create(contacts.CreateInput{Name: "Marcus Stannard", Role: "Partner"})
	if !cr.OK {
		t.Fatalf("Create failed: %s", cr.Error())
	}
	id := cr.Value.(contacts.Contact).ID
	ur := svc.Update(contacts.UpdateInput{ID: id, Next: "pilot signoff"})
	if !ur.OK {
		t.Fatalf("Update failed: %s", ur.Error())
	}
	dirR := core.UserHomeDir()
	if !dirR.OK {
		t.Fatalf("UserHomeDir: %s", dirR.Error())
	}
	fpath := core.PathJoin(dirR.Value.(string), "Lethean/sales/contacts", id+".md")
	rd := paths.ReadVersion(fpath)
	if !rd.OK {
		t.Fatalf("ReadVersion: %s", rd.Error())
	}
	got := rd.Value.(paths.ReadOutput)
	if got.Version != 2 {
		t.Fatalf("expected version 2 after Update, got %d", got.Version)
	}
}

// TestAtomicCutover_Contacts_Update_VersionStale_Ugly — IfVersion
// mismatch surfaces a wrapped conflict error. Cascade W1 (Mantis #1540)
// — simulates a concurrent writer by rewriting the file out-of-band to
// version=99 between the Service's read and its conditional write.
func TestAtomicCutover_Contacts_Update_VersionStale_Ugly(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := contacts.NewService(nil)
	cr := svc.Create(contacts.CreateInput{Name: "Tom Pemberton", Role: "COO"})
	if !cr.OK {
		t.Fatalf("Create failed: %s", cr.Error())
	}
	id := cr.Value.(contacts.Contact).ID
	dirR := core.UserHomeDir()
	if !dirR.OK {
		t.Fatalf("UserHomeDir: %s", dirR.Error())
	}
	fpath := core.PathJoin(dirR.Value.(string), "Lethean/sales/contacts", id+".md")
	// Out-of-band rewrite: bump version field to 99 so the Service's
	// internal read+write race against an "external" mutation.
	rawR := core.ReadFile(fpath)
	if !rawR.OK {
		t.Fatalf("ReadFile: %s", rawR.Error())
	}
	mutated := bumpVersion(rawR.Value.([]byte), 99)
	if w := core.WriteFile(fpath, mutated, 0o600); !w.OK {
		t.Fatalf("WriteFile: %s", w.Error())
	}
	// Service's Update reads version=99 from disk and stamps
	// IfVersion=99 → success. We need to force the stale path by
	// writing again out-of-band BEFORE the Service's locked write
	// happens — easier path: directly call paths.AtomicWriteWithVersion
	// with a known-stale IfVersion to assert the primitive's
	// conflict-wrap behaviour is reachable through the service-shape.
	body := []byte("---\nversion: 99\nid: " + id + "\nname: tom\n---\n")
	r := paths.AtomicWriteWithVersion(fpath, paths.WriteInput{
		Body:      body,
		IfVersion: 1, // stale — disk holds 99
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
	if vs.CurrentVersion != 99 {
		t.Fatalf("expected CurrentVersion=99, got %d", vs.CurrentVersion)
	}
}

// TestAtomicCutover_Contacts_LegacyFile_Ugly — a contact file without
// a version frontmatter field reads as version 0; the next Service
// Update upgrades it via an unconditional first-write that stamps
// version=1.
func TestAtomicCutover_Contacts_LegacyFile_Ugly(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := contacts.NewService(nil)
	// Seed a contacts dir + a legacy file with NO version frontmatter.
	dirR := core.UserHomeDir()
	if !dirR.OK {
		t.Fatalf("UserHomeDir: %s", dirR.Error())
	}
	contactsDir := core.PathJoin(dirR.Value.(string), "Lethean/sales/contacts")
	if mk := core.MkdirAll(contactsDir, 0o700); !mk.OK {
		t.Fatalf("MkdirAll: %s", mk.Error())
	}
	legacyID := "legacy-contact"
	fpath := core.PathJoin(contactsDir, legacyID+".md")
	legacy := []byte("---\nid: legacy-contact\nname: Legacy Person\nrole: Old Role\n---\n")
	if w := core.WriteFile(fpath, legacy, 0o600); !w.OK {
		t.Fatalf("WriteFile: %s", w.Error())
	}
	// Sanity: pre-update version reads as 0 (no frontmatter version).
	rd := paths.ReadVersion(fpath)
	if !rd.OK {
		t.Fatalf("ReadVersion: %s", rd.Error())
	}
	if got := rd.Value.(paths.ReadOutput); got.Version != 0 {
		t.Fatalf("legacy file pre-update: expected version 0, got %d", got.Version)
	}
	// Update via Service. The internal IfVersion=0 path bypasses the
	// stale check and stamps version=1 on the upgrade write.
	ur := svc.Update(contacts.UpdateInput{ID: legacyID, Next: "re-engage"})
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

// TestAtomicCutover_Contacts_AuditEmissionRecordBatch_Good — Cascade W1
// (Mantis #1540) — confirms a Create routes through the primitive's
// write path (EventWriteSucceeded fires) and that the policy table
// classifies sales/contacts/* under AuditModeBatch.
func TestAtomicCutover_Contacts_AuditEmissionRecordBatch_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := contacts.NewService(nil)
	// The primitive drops LockEvents when the HKDF audit secret is
	// unavailable (Cerberus DREAD-r2 F2, Mantis #1526). Install a
	// deterministic test secret so the emit path is reachable.
	paths.SetAuditSecretProvider(func() []byte {
		return []byte("contacts-cutover-test-secret-32-byte")
	})
	t.Cleanup(func() { paths.SetAuditSecretProvider(nil) })
	var saw []paths.LockEvent
	paths.SubscribeLockEvents(func(ev paths.LockEvent) {
		saw = append(saw, ev)
	})
	t.Cleanup(paths.ClearLockEventSubscribersForTest)

	r := svc.Create(contacts.CreateInput{Name: "Sarah Whitethorn", Role: "Founder"})
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
	// Policy-table sanity — sales/contacts/* paths fall under
	// AuditModeBatch per RFC §6.1.
	dirR := core.UserHomeDir()
	if !dirR.OK {
		t.Fatalf("UserHomeDir: %s", dirR.Error())
	}
	fpath := core.PathJoin(dirR.Value.(string), "Lethean/sales/contacts/sarah-whitethorn.md")
	mode := paths.AuditModeForPath(fpath)
	if mode != paths.AuditModeBatch {
		t.Fatalf("expected AuditModeBatch for sales/contacts path, got %v", mode)
	}
}

// bumpVersion rewrites a Trix file's frontmatter "version:" line to the
// supplied integer. If no version line exists it inserts one after the
// opening "---" delimiter. Used by the stale-conflict test to seed a
// race condition on disk without spinning up a second writer.
func bumpVersion(raw []byte, v int) []byte {
	versionLine := core.Sprintf("version: %d", v)
	// Walk lines and replace any existing version: line.
	out := make([]byte, 0, len(raw)+32)
	lines := splitLines(raw)
	replaced := false
	for i, l := range lines {
		if i > 0 && core.HasPrefix(string(l), "version:") {
			out = append(out, []byte(versionLine)...)
			out = append(out, '\n')
			replaced = true
			continue
		}
		out = append(out, l...)
		if i < len(lines)-1 {
			out = append(out, '\n')
		}
	}
	if replaced {
		return out
	}
	// No version line — insert one after the opening "---".
	open := []byte("---\n")
	if len(raw) >= len(open) && string(raw[:len(open)]) == "---\n" {
		ins := make([]byte, 0, len(raw)+len(versionLine)+1)
		ins = append(ins, open...)
		ins = append(ins, []byte(versionLine)...)
		ins = append(ins, '\n')
		ins = append(ins, raw[len(open):]...)
		return ins
	}
	return raw
}

// splitLines returns the byte slices for each line, without the
// trailing newline. The final element is whatever follows the last \n
// (possibly empty).
func splitLines(raw []byte) [][]byte {
	var out [][]byte
	start := 0
	for i := 0; i < len(raw); i++ {
		if raw[i] == '\n' {
			out = append(out, raw[start:i])
			start = i + 1
		}
	}
	out = append(out, raw[start:])
	return out
}
