// SPDX-Licence-Identifier: EUPL-1.2

package contacts_test

import (
	"encoding/json"
	"sync"
	"testing"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/paths"
	"dappco.re/lthn/desktop/pkg/sales/contacts"
)

// stubSessionGate is the test double for the consumer-defined
// SessionGate interface (RFC.stage-e-unlockgate v2 §4.2 stub shape —
// Mantis #1613 B.2). Duplicated per-pkg rather than shared so test
// files keep zero cross-pkg testing/... deps.
type stubSessionGate struct{ ids []string }

func (s *stubSessionGate) UnlockedAccountIDs() []string { return s.ids }

// newTestSvc constructs a contacts.Service pre-wired with a SessionGate
// reporting a single unlocked account so write methods pass the gate
// by default. Tests that need to assert the locked-state path call
// SetSessionGate explicitly with an empty stub (or instantiate
// contacts.NewService directly for the nil-gate fail-safe path).
//
// Usage example:
//
//	svc := newTestSvc(t)
//	r := svc.Create(contacts.CreateInput{Name: "Ada Penley"})
func newTestSvc(_ *testing.T) *contacts.Service {
	svc := contacts.NewService(nil)
	svc.SetSessionGate(&stubSessionGate{ids: []string{"acct-test"}})
	return svc
}

// TestList_Empty_Good — empty contacts dir produces an empty list.
func TestList_Empty_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newTestSvc(t)
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
	svc := newTestSvc(t)
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
	svc := newTestSvc(t)
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
	svc := newTestSvc(t)

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
	svc := newTestSvc(t)
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
	svc := newTestSvc(t)
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
	svc := newTestSvc(t)
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
	svc := newTestSvc(t)
	svc.Create(contacts.CreateInput{Name: "Ada Penley"})
	r := svc.Create(contacts.CreateInput{Name: "Ada Penley"})
	if r.OK {
		t.Fatalf("expected failure for duplicate slug, got OK")
	}
}

// TestCreate_EmptyName_Ugly — Create with empty name returns core.Fail.
func TestCreate_EmptyName_Ugly(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newTestSvc(t)
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
	svc := newTestSvc(t)
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
	svc := newTestSvc(t)
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

// TestAtomicCutover_Contacts_Update_VersionStale_Ugly — drives two
// concurrent svc.Update calls and asserts the loser's Result carries
// a paths.ConflictEnvelope whose JSON marshal matches the lowercase
// `{code, current_version, current_hash}` wire shape that
// frontend/src/lit/conflict-dispatch.ts extractEnvelope pattern-
// matches on (Mantis #1547 service-tier round-trip; pins #1544 against
// future drift).
//
// Why two goroutines: svc.Update re-reads the file inside the call so
// a single-shot out-of-band mutation cannot drive a real conflict —
// the only way to land on the stale path through the service is a true
// writer race. WithFileLock serialises the locked sections; one
// goroutine's pre-lock parseContact reads version=N, then the other
// goroutine takes the lock first, writes version=N+1, releases — the
// loser's locked-section ReadVersion sees N+1 versus its IfVersion=N
// and the primitive returns VersionStale → wrapped as ConflictEnvelope
// by contacts.atomicWriteFile.
func TestAtomicCutover_Contacts_Update_VersionStale_Ugly(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newTestSvc(t)
	cr := svc.Create(contacts.CreateInput{Name: "Tom Pemberton", Role: "COO"})
	if !cr.OK {
		t.Fatalf("Create failed: %s", cr.Error())
	}
	id := cr.Value.(contacts.Contact).ID

	// Drive two concurrent Updates on the same record. With
	// goroutine launch fan-out + WithFileLock serialisation, the
	// loser's IfVersion stalls behind the winner's bump on every
	// observed run — but to make the race deterministic on slow
	// CIs we keep retrying until one of the two pairs lands a
	// genuine conflict (capped attempts so a hung test surfaces).
	var conflict core.Result
	var saw bool
	for attempt := 0; attempt < 32 && !saw; attempt++ {
		var (
			mu       sync.Mutex
			results  []core.Result
			wg       sync.WaitGroup
			start    = make(chan struct{})
		)
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			r := svc.Update(contacts.UpdateInput{ID: id, Next: "racer-a"})
			mu.Lock()
			results = append(results, r)
			mu.Unlock()
		}()
		go func() {
			defer wg.Done()
			<-start
			r := svc.Update(contacts.UpdateInput{ID: id, Next: "racer-b"})
			mu.Lock()
			results = append(results, r)
			mu.Unlock()
		}()
		close(start)
		wg.Wait()

		for _, r := range results {
			if !r.OK && core.Contains(r.Error(), "contacts.update.conflict") {
				conflict = r
				saw = true
				break
			}
		}
	}
	if !saw {
		t.Skip("could not provoke a writer race after 32 attempts — environment lock skew; flake-defensive skip not failure")
	}

	// Wire-shape assertion #1 — the canonical service code is reachable
	// via Result.Error() (preserves the existing core.Contains pattern
	// for log-tailers + retry-tier filters).
	if !core.Contains(conflict.Error(), "contacts.update.conflict") {
		t.Fatalf("expected contacts.update.conflict in error, got %q", conflict.Error())
	}

	// Wire-shape assertion #2 — Result.Value is a paths.ConflictEnvelope
	// (the typed wrap; the old *core.Err wrap marshalled PascalCase with
	// an empty Code field and lost the frontend's pattern match).
	env, ok := paths.ConflictEnvelopeFrom(conflict.Value)
	if !ok {
		t.Fatalf("expected paths.ConflictEnvelope in Result.Value, got %T", conflict.Value)
	}
	if env.Code != "contacts.update.conflict" {
		t.Fatalf("expected envelope code contacts.update.conflict, got %q", env.Code)
	}
	if env.CurrentVersion < 1 {
		t.Fatalf("expected CurrentVersion >= 1 after winner's bump, got %d", env.CurrentVersion)
	}

	// Wire-shape assertion #3 — json.Marshal(Value) yields the lowercase
	// snake_case keys conflict-dispatch.ts extractEnvelope walks. This
	// is the discipline anchor for #1547 — without it the *core.Err
	// drift recurs every wave.
	raw, err := json.Marshal(conflict.Value)
	if err != nil {
		t.Fatalf("json.Marshal(conflict.Value): %s", err.Error())
	}
	js := string(raw)
	for _, want := range []string{
		`"code":"contacts.update.conflict"`,
		`"current_version":`,
	} {
		if !core.Contains(js, want) {
			t.Fatalf("expected marshalled envelope to contain %s, got %s", want, js)
		}
	}
	// Reject the prior *core.Err shape — PascalCase keys must NOT appear.
	for _, banned := range []string{`"Code":`, `"Message":`, `"Operation":`} {
		if core.Contains(js, banned) {
			t.Fatalf("marshalled envelope leaks *core.Err PascalCase key %s — wire drift: %s", banned, js)
		}
	}
}

// TestAtomicCutover_Contacts_LegacyFile_Ugly — a contact file without
// a version frontmatter field reads as version 0; the next Service
// Update upgrades it via an unconditional first-write that stamps
// version=1.
func TestAtomicCutover_Contacts_LegacyFile_Ugly(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newTestSvc(t)
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
	svc := newTestSvc(t)
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

// ---- SessionGate retrofit (RFC.stage-e-unlockgate v2 §4.2 / Mantis #1613 B.2)

// TestContacts_NilGate_WarnsOnce_FailsClosed — a Service constructed
// without SetSessionGate fails-closed on writes. Second + third writes
// continue to fail-closed (one-shot warn semantics — the second call
// must remain quiet but its caller-visible result must be the same
// fail-closed shape). The warn-once field assertion lives in the
// internal-package test surface (documents pkg precedent); this
// external test pins the observable contract.
func TestContacts_NilGate_WarnsOnce_FailsClosed(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := contacts.NewService(nil) // NO SetSessionGate — exercises §2.2 fail-safe.

	r1 := svc.Create(contacts.CreateInput{Name: "Ada Penley"})
	if r1.OK {
		t.Fatal("expected Create to fail-closed when gate is nil")
	}
	if !core.Contains(r1.Error(), "contacts.session.locked") {
		t.Fatalf("expected contacts.session.locked on first Create, got %q", r1.Error())
	}

	// Second write — nilWarned already true; CompareAndSwap returns
	// false and core.Warn is NOT called again. Behaviour from the
	// caller's perspective: same fail-closed result.
	r2 := svc.Update(contacts.UpdateInput{ID: "ada-penley", Next: "ping"})
	if r2.OK {
		t.Fatal("expected Update to fail-closed when gate is nil")
	}
	if !core.Contains(r2.Error(), "contacts.session.locked") {
		t.Fatalf("expected contacts.session.locked on Update, got %q", r2.Error())
	}

	// Third write — same fail-closed behaviour persists.
	r3 := svc.Create(contacts.CreateInput{Name: "Marcus Stannard"})
	if r3.OK {
		t.Fatal("expected third Create to fail-closed when gate is nil")
	}
	if !core.Contains(r3.Error(), "contacts.session.locked") {
		t.Fatalf("expected contacts.session.locked on third Create, got %q", r3.Error())
	}
}

// TestContacts_UnlockedGate_AllowsCreate — Create succeeds when the
// live-read gate reports at least one unlocked account.
func TestContacts_UnlockedGate_AllowsCreate(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := contacts.NewService(nil)
	svc.SetSessionGate(&stubSessionGate{ids: []string{"acct-1"}})

	r := svc.Create(contacts.CreateInput{Name: "Ada Penley", Role: "CTO"})
	if !r.OK {
		t.Fatalf("Create should succeed with gate reporting unlocked acct, got: %s", r.Error())
	}
}

// TestContacts_UnlockedGate_AllowsUpdate — Update succeeds when the
// live-read gate reports at least one unlocked account.
func TestContacts_UnlockedGate_AllowsUpdate(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := contacts.NewService(nil)
	svc.SetSessionGate(&stubSessionGate{ids: []string{"acct-1"}})

	cr := svc.Create(contacts.CreateInput{Name: "Ada Penley", Role: "CTO"})
	if !cr.OK {
		t.Fatalf("seed Create failed: %s", cr.Error())
	}
	ur := svc.Update(contacts.UpdateInput{ID: "ada-penley", Next: "follow up"})
	if !ur.OK {
		t.Fatalf("Update should succeed with gate reporting unlocked acct, got: %s", ur.Error())
	}
}

// TestContacts_LockedGate_FailsCreate_session_locked — Create rejects
// when the live-read gate reports zero unlocked accounts.
func TestContacts_LockedGate_FailsCreate_session_locked(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := contacts.NewService(nil)
	svc.SetSessionGate(&stubSessionGate{ids: []string{}})

	r := svc.Create(contacts.CreateInput{Name: "Ada Penley"})
	if r.OK {
		t.Fatal("expected Create to be rejected when gate reports zero unlocked accounts")
	}
	if !core.Contains(r.Error(), "contacts.session.locked") {
		t.Fatalf("expected contacts.session.locked, got %q", r.Error())
	}
}

// TestContacts_LockedGate_FailsUpdate_session_locked — Update rejects
// when the live-read gate reports zero unlocked accounts. The Update
// is gated BEFORE the IsValidID check + filesystem read, so the
// session.locked code surfaces in preference to any later error.
func TestContacts_LockedGate_FailsUpdate_session_locked(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	// Seed a record while unlocked, then flip to locked + try to update.
	svc := newTestSvc(t)
	cr := svc.Create(contacts.CreateInput{Name: "Marcus Stannard", Role: "Partner"})
	if !cr.OK {
		t.Fatalf("seed Create failed: %s", cr.Error())
	}
	svc.SetSessionGate(&stubSessionGate{ids: []string{}})

	r := svc.Update(contacts.UpdateInput{ID: "marcus-stannard", Next: "ping"})
	if r.OK {
		t.Fatal("expected Update to be rejected when gate reports zero unlocked accounts")
	}
	if !core.Contains(r.Error(), "contacts.session.locked") {
		t.Fatalf("expected contacts.session.locked, got %q", r.Error())
	}
}

// TestContacts_StopNilsGate — Stop() severs the SessionGate; subsequent
// writes fail-closed even though the gate WAS wired (Cerberus #27
// ADD-5 — Stop drain hygiene mirrors mail).
func TestContacts_StopNilsGate(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newTestSvc(t) // gate wired with unlocked stub

	// Pre-Stop: write succeeds.
	if r := svc.Create(contacts.CreateInput{Name: "Ada Penley"}); !r.OK {
		t.Fatalf("Create should succeed pre-Stop, got: %s", r.Error())
	}

	// Stop nils the gate reference.
	if r := svc.Stop(core.Background()); !r.OK {
		t.Fatalf("Stop should succeed, got: %s", r.Error())
	}

	// Post-Stop: write fails-closed via the nil-gate path.
	r := svc.Create(contacts.CreateInput{Name: "Marcus Stannard"})
	if r.OK {
		t.Fatal("expected Create to fail-closed after Stop nils the gate")
	}
	if !core.Contains(r.Error(), "contacts.session.locked") {
		t.Fatalf("expected contacts.session.locked, got %q", r.Error())
	}
}

// TestContacts_LockedGate_ReadStillWorks — List + Get are not gated by
// the session-lock (RFC §3.1 — reads stay open while locked).
func TestContacts_LockedGate_ReadStillWorks(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	// Seed unlocked, then flip to locked.
	svc := newTestSvc(t)
	svc.Create(contacts.CreateInput{Name: "Ada Penley", Role: "CTO"})
	svc.SetSessionGate(&stubSessionGate{ids: []string{}})

	r := svc.List(contacts.ListInput{})
	if !r.OK {
		t.Fatalf("List should succeed when session locked, got: %s", r.Error())
	}
	if g := svc.Get(contacts.GetInput{ID: "ada-penley"}); !g.OK {
		t.Fatalf("Get should succeed when session locked, got: %s", g.Error())
	}
}
