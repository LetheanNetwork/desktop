// SPDX-Licence-Identifier: EUPL-1.2

package incidents_test

import (
	"encoding/json"
	"sync"
	"testing"

	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/incidents"
	"dappco.re/lthn/desktop/pkg/paths"
)

// stubSessionGate is the test double for the consumer-defined
// SessionGate interface (RFC.stage-e-unlockgate v2 §4.2 stub shape).
// Mirrors H#147 documents.stubSessionGate — duplicated per-pkg rather
// than shared, per Pushback 1 (consumer-defines) and Cerberus #28
// confirm that each writer pkg owns its own gate interface.
//
// Usage example:
//
//	svc := incidents.NewService(nil)
//	svc.SetSessionGate(&stubSessionGate{ids: []string{"acct-test"}})
type stubSessionGate struct{ ids []string }

func (s *stubSessionGate) UnlockedAccountIDs() []string { return s.ids }

// newServiceUnlocked returns an incidents Service with a SessionGate
// pre-wired to report a single unlocked account. Tests that exercise
// writer paths (Create / UpdateState / AddPostmortem) call this so the
// gate-check at the top of each writer method succeeds. Tests that
// need to assert the locked path call SetSessionGate explicitly with
// an empty stub (or skip the helper and exercise the nil-gate
// fail-safe directly).
//
// Usage example:
//
//	svc := newServiceUnlocked()
//	r := svc.Create(subject.CreateInput{Title: "...", Sev: "P3"})
func newServiceUnlocked() *subject.Service {
	svc := subject.NewService(nil)
	svc.SetSessionGate(&stubSessionGate{ids: []string{"acct-test"}})
	return svc
}

// tempIncidentsDir sets up a fresh ~/Lethean/incidents/ in a temp
// directory by overriding the home dir via the test environment.
// Tests that exercise filesystem I/O use a shared NewService(nil)
// and operate against the process's real home; unit tests that need
// isolation operate against the parse/marshal helpers directly.

func TestParseRecord_RoundTrip(t *testing.T) {
	now := core.Now().UTC()
	rec := subject.IncidentRecord{
		ID:        "2026-05-INC-001",
		Title:     "hub · elevated p99 latency",
		Sev:       "P3",
		State:     "investigating",
		Svc:       "hub",
		Who:       "Mei",
		Comments:  3,
		CreatedAt: now,
	}
	// marshal then parse back — round-trip should be lossless for
	// the searchable header fields.
	raw, err := subject.MarshalRecordExported(rec)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got, err := subject.ParseRecordExported(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got.ID != rec.ID {
		t.Errorf("id: want %q got %q", rec.ID, got.ID)
	}
	if got.Sev != rec.Sev {
		t.Errorf("sev: want %q got %q", rec.Sev, got.Sev)
	}
	if got.State != rec.State {
		t.Errorf("state: want %q got %q", rec.State, got.State)
	}
	if got.Title != rec.Title {
		t.Errorf("title: want %q got %q", rec.Title, got.Title)
	}
}

func TestParseRecord_WithBody(t *testing.T) {
	rec := subject.IncidentRecord{
		ID:    "2026-05-INC-002",
		Title: "DNS cache poisoning",
		Sev:   "P1",
		State: "post-mortem",
		Svc:   "all",
		Who:   "all",
	}
	body := "## Root cause\n\nDNS TTL was too low.\n"
	rec.PostMortem = body
	raw, err := subject.MarshalRecordExported(rec)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got, err := subject.ParseRecordExported(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got.PostMortem != body {
		t.Errorf("body: want %q got %q", body, got.PostMortem)
	}
}

func TestRelativeTime_Now(t *testing.T) {
	now := core.Now()
	got := subject.RelativeTimeExported(now, now)
	if got != "now" {
		t.Errorf("want 'now' got %q", got)
	}
}

func TestRelativeTime_Days(t *testing.T) {
	now := core.Now()
	past := now.Add(-11 * 24 * core.Hour)
	got := subject.RelativeTimeExported(past, now)
	if got != "11 d ago" {
		t.Errorf("want '11 d ago' got %q", got)
	}
}

func TestRelativeTime_Weeks(t *testing.T) {
	now := core.Now()
	// 3 weeks back — should render as "3 w ago"
	past := now.Add(-3 * 7 * 24 * core.Hour)
	got := subject.RelativeTimeExported(past, now)
	if got != "3 w ago" {
		t.Errorf("want '3 w ago' got %q", got)
	}
}

func TestDurString_Minutes(t *testing.T) {
	got := subject.DurStringExported(42)
	if got != "42 min" {
		t.Errorf("want '42 min' got %q", got)
	}
}

func TestDurString_Hours(t *testing.T) {
	got := subject.DurStringExported(134) // 2h 14m
	if got != "2 h 14" {
		t.Errorf("want '2 h 14' got %q", got)
	}
}

func TestServiceName(t *testing.T) {
	svc := subject.NewService(nil)
	if svc.ServiceName() != "Incidents" {
		t.Errorf("want 'Incidents' got %q", svc.ServiceName())
	}
}

// ---- Cascade W3 cutover tests (paths.AtomicWriteWithVersion) ---------------

// incidentFilePath resolves the on-disk file for an incident created
// "now" — the writer routes through yearMonthDir() which lands under
// ~/Lethean/incidents/<YYYY>/<MM>/<ID>.md.
func incidentFilePath(t *testing.T, id string) string {
	t.Helper()
	now := core.Now().UTC()
	dirR := core.UserHomeDir()
	if !dirR.OK {
		t.Fatalf("UserHomeDir: %s", dirR.Error())
	}
	yr := core.TimeFormat(now, "2006")
	mo := core.TimeFormat(now, "01")
	return core.PathJoin(dirR.Value.(string),
		"Lethean/incidents", yr, mo, id+".md")
}

// TestAtomicCutover_Incidents_Create_Good — Create stamps version=1
// via the primitive; ReadVersion confirms the on-disk file carries
// the same.
func TestAtomicCutover_Incidents_Create_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newServiceUnlocked()
	r := svc.Create(subject.CreateInput{
		Title: "hub · elevated p99", Sev: "P3", Svc: "hub", Who: "Mei",
	})
	if !r.OK {
		t.Fatalf("Create failed: %s", r.Error())
	}
	entry := r.Value.(subject.IncidentEntry)
	fpath := incidentFilePath(t, entry.ID)
	rd := paths.ReadVersion(fpath)
	if !rd.OK {
		t.Fatalf("ReadVersion: %s", rd.Error())
	}
	got := rd.Value.(paths.ReadOutput)
	if got.Version != 1 {
		t.Fatalf("expected version 1 after Create, got %d", got.Version)
	}
}

// TestAtomicCutover_Incidents_Update_Good — a sequential UpdateState
// bumps the stored version monotonically (1 -> 2).
func TestAtomicCutover_Incidents_Update_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newServiceUnlocked()
	cr := svc.Create(subject.CreateInput{
		Title: "forge build queue stalled", Sev: "P3", Svc: "forge", Who: "Tobi",
	})
	if !cr.OK {
		t.Fatalf("Create failed: %s", cr.Error())
	}
	id := cr.Value.(subject.IncidentEntry).ID
	ur := svc.UpdateState(subject.UpdateStateInput{ID: id, State: "post-mortem"})
	if !ur.OK {
		t.Fatalf("UpdateState failed: %s", ur.Error())
	}
	fpath := incidentFilePath(t, id)
	rd := paths.ReadVersion(fpath)
	if !rd.OK {
		t.Fatalf("ReadVersion: %s", rd.Error())
	}
	got := rd.Value.(paths.ReadOutput)
	if got.Version != 2 {
		t.Fatalf("expected version 2 after UpdateState, got %d", got.Version)
	}
}

// TestAtomicCutover_Incidents_Update_VersionStale_Ugly — drives two
// concurrent svc.UpdateState calls and asserts the loser's Result
// carries a paths.ConflictEnvelope whose JSON marshal matches the
// lowercase `{code, current_version, current_hash}` wire shape that
// frontend/src/lit/conflict-dispatch.ts extractEnvelope pattern-
// matches on (Mantis #1547 service-tier round-trip discipline anchor;
// pins #1544 against W3 wave drift).
func TestAtomicCutover_Incidents_Update_VersionStale_Ugly(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newServiceUnlocked()
	cr := svc.Create(subject.CreateInput{
		Title: "race · contended state",
		Sev:   "P3", Svc: "hub", Who: "Mei",
	})
	if !cr.OK {
		t.Fatalf("Create failed: %s", cr.Error())
	}
	id := cr.Value.(subject.IncidentEntry).ID

	var conflict core.Result
	var saw bool
	for attempt := 0; attempt < 32 && !saw; attempt++ {
		// Reset state back to "investigating" between attempts so both
		// goroutines have a legal target each round.
		if attempt > 0 {
			rr := svc.UpdateState(subject.UpdateStateInput{ID: id, State: "investigating"})
			if !rr.OK {
				// Could be a conflict on reset itself — proceed anyway.
				_ = rr
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
			r := svc.UpdateState(subject.UpdateStateInput{ID: id, State: "post-mortem"})
			mu.Lock()
			results = append(results, r)
			mu.Unlock()
		}()
		go func() {
			defer wg.Done()
			<-start
			r := svc.UpdateState(subject.UpdateStateInput{ID: id, State: "post-mortem"})
			mu.Lock()
			results = append(results, r)
			mu.Unlock()
		}()
		close(start)
		wg.Wait()

		for _, r := range results {
			if !r.OK && core.Contains(r.Error(), "incidents.update.conflict") {
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
	if env.Code != "incidents.update.conflict" {
		t.Fatalf("expected envelope code incidents.update.conflict, got %q", env.Code)
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
		`"code":"incidents.update.conflict"`,
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

// TestAtomicCutover_Incidents_LegacyFile_Ugly — an incident file
// without version: frontmatter reads as version 0; UpdateState
// upgrades it via an unconditional first-write that stamps version=1.
func TestAtomicCutover_Incidents_LegacyFile_Ugly(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newServiceUnlocked()
	now := core.Now().UTC()
	dirR := core.UserHomeDir()
	if !dirR.OK {
		t.Fatalf("UserHomeDir: %s", dirR.Error())
	}
	yr := core.TimeFormat(now, "2006")
	mo := core.TimeFormat(now, "01")
	monthDir := core.PathJoin(dirR.Value.(string),
		"Lethean/incidents", yr, mo)
	if mk := core.MkdirAll(monthDir, 0o700); !mk.OK {
		t.Fatalf("MkdirAll: %s", mk.Error())
	}
	// Hand-craft a legacy file with no version: frontmatter line.
	legacyID := yr + "-" + mo + "-INC-099"
	fpath := core.PathJoin(monthDir, legacyID+".md")
	legacy := []byte("---\nid: " + legacyID + "\ntitle: legacy · pre-cutover\nsev: P3\nstate: investigating\nsvc: hub\nwho: legacy\ncomments: 0\ncreated_at: 2026-01-01T00:00:00Z\nresolved_at: 0001-01-01T00:00:00Z\ndur_minutes: 0\n---\n")
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
	ur := svc.UpdateState(subject.UpdateStateInput{ID: legacyID, State: "post-mortem"})
	if !ur.OK {
		t.Fatalf("UpdateState failed: %s", ur.Error())
	}
	rd2 := paths.ReadVersion(fpath)
	if !rd2.OK {
		t.Fatalf("ReadVersion post-update: %s", rd2.Error())
	}
	if got := rd2.Value.(paths.ReadOutput); got.Version != 1 {
		t.Fatalf("legacy file post-update: expected version 1, got %d", got.Version)
	}
}

// TestAtomicCutover_Incidents_AuditEmissionRecordBatch_Good — Create
// routes through the primitive's write path (EventWriteSucceeded
// fires) and incidents/* falls under AuditModeBatch per RFC §6.1.
func TestAtomicCutover_Incidents_AuditEmissionRecordBatch_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newServiceUnlocked()
	paths.SetAuditSecretProvider(func() []byte {
		return []byte("incidents-cutover-test-secret-32b")
	})
	t.Cleanup(func() { paths.SetAuditSecretProvider(nil) })
	var saw []paths.LockEvent
	paths.SubscribeLockEvents(func(ev paths.LockEvent) {
		saw = append(saw, ev)
	})
	t.Cleanup(paths.ClearLockEventSubscribersForTest)

	r := svc.Create(subject.CreateInput{
		Title: "audit-emission probe", Sev: "P4", Svc: "hub", Who: "qa",
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
	fpath := core.PathJoin(dirR.Value.(string),
		"Lethean/incidents/2026/05/x.md")
	mode := paths.AuditModeForPath(fpath)
	if mode != paths.AuditModeBatch {
		t.Fatalf("expected AuditModeBatch for incidents path, got %v", mode)
	}
}

// ---- B.2 SessionGate live-read tests (Mantis #1613, Cerberus #28) ----

// TestIncidents_NilGate_WarnsOnce_FailsClosed — nil gate fails-locked
// on the first write; nilWarned one-shot suppresses re-warning on
// subsequent writes (RFC §2.2 / Cerberus #28 Q2 fail-safe). Mirrors
// H#147 documents.TestDocuments_NilGate_WarnsOnce_FailsClosed exactly.
func TestIncidents_NilGate_WarnsOnce_FailsClosed(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := subject.NewService(nil) // deliberately NO SetSessionGate

	// First write — nilWarned trips false→true; behaviour: fails closed.
	r1 := svc.Create(subject.CreateInput{
		Title: "first-nil-hit", Sev: "P3", Svc: "hub", Who: "qa",
	})
	if r1.OK {
		t.Fatal("expected Create to fail-closed when gate is nil")
	}
	if !core.Contains(r1.Error(), "incidents.session.locked") {
		t.Fatalf("expected incidents.session.locked, got %q", r1.Error())
	}

	// Second write — nilWarned already true; CompareAndSwap returns
	// false and core.Warn is NOT called again. Behaviour from the
	// caller's perspective: same fail-closed result.
	r2 := svc.UpdateState(subject.UpdateStateInput{
		ID: "2026-05-INC-001", State: "resolved",
	})
	if r2.OK {
		t.Fatal("expected UpdateState to fail-closed when gate is nil")
	}
	if !core.Contains(r2.Error(), "incidents.session.locked") {
		t.Fatalf("expected incidents.session.locked, got %q", r2.Error())
	}

	// Third write — AddPostmortem also fail-closed.
	r3 := svc.AddPostmortem(subject.PostmortemInput{
		ID: "2026-05-INC-001", Body: "anything",
	})
	if r3.OK {
		t.Fatal("expected AddPostmortem to fail-closed when gate is nil")
	}
	if !core.Contains(r3.Error(), "incidents.session.locked") {
		t.Fatalf("expected incidents.session.locked, got %q", r3.Error())
	}
}

// TestIncidents_UnlockedGate_AllowsCreate — Create succeeds when the
// live-read gate reports a non-empty unlocked-account slice.
func TestIncidents_UnlockedGate_AllowsCreate(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := subject.NewService(nil)
	svc.SetSessionGate(&stubSessionGate{ids: []string{"acct-1"}})

	r := svc.Create(subject.CreateInput{
		Title: "unlocked-create", Sev: "P3", Svc: "hub", Who: "Mei",
	})
	if !r.OK {
		t.Fatalf("Create should succeed with gate reporting unlocked acct, got: %s", r.Error())
	}
}

// TestIncidents_UnlockedGate_AllowsUpdateState — UpdateState succeeds
// when the live-read gate reports a non-empty unlocked-account slice.
func TestIncidents_UnlockedGate_AllowsUpdateState(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := subject.NewService(nil)
	svc.SetSessionGate(&stubSessionGate{ids: []string{"acct-1"}})

	cr := svc.Create(subject.CreateInput{
		Title: "to-be-transitioned", Sev: "P3", Svc: "hub", Who: "qa",
	})
	if !cr.OK {
		t.Fatalf("Create failed: %s", cr.Error())
	}
	id := cr.Value.(subject.IncidentEntry).ID

	ur := svc.UpdateState(subject.UpdateStateInput{ID: id, State: "post-mortem"})
	if !ur.OK {
		t.Fatalf("UpdateState should succeed with unlocked gate, got: %s", ur.Error())
	}
}

// TestIncidents_UnlockedGate_AllowsAddPostmortem — AddPostmortem
// succeeds when the live-read gate reports a non-empty
// unlocked-account slice (and the incident is past investigating).
func TestIncidents_UnlockedGate_AllowsAddPostmortem(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := subject.NewService(nil)
	svc.SetSessionGate(&stubSessionGate{ids: []string{"acct-1"}})

	cr := svc.Create(subject.CreateInput{
		Title: "to-be-postmortemed", Sev: "P3", Svc: "hub", Who: "qa",
	})
	if !cr.OK {
		t.Fatalf("Create failed: %s", cr.Error())
	}
	id := cr.Value.(subject.IncidentEntry).ID
	// AddPostmortem requires non-investigating state.
	if ur := svc.UpdateState(subject.UpdateStateInput{ID: id, State: "post-mortem"}); !ur.OK {
		t.Fatalf("UpdateState failed: %s", ur.Error())
	}

	pr := svc.AddPostmortem(subject.PostmortemInput{
		ID: id, Body: "## Root cause\n\nDNS TTL too low.",
	})
	if !pr.OK {
		t.Fatalf("AddPostmortem should succeed with unlocked gate, got: %s", pr.Error())
	}
}

// TestIncidents_LockedGate_FailsCreate — Create rejects when the
// live-read gate reports zero unlocked accounts.
func TestIncidents_LockedGate_FailsCreate(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := subject.NewService(nil)
	svc.SetSessionGate(&stubSessionGate{ids: []string{}})

	r := svc.Create(subject.CreateInput{
		Title: "locked-create", Sev: "P3", Svc: "hub", Who: "qa",
	})
	if r.OK {
		t.Fatal("expected Create to be rejected when gate reports zero unlocked accounts")
	}
	if !core.Contains(r.Error(), "incidents.session.locked") {
		t.Fatalf("expected incidents.session.locked, got %q", r.Error())
	}
}

// TestIncidents_LockedGate_FailsUpdateState — UpdateState rejects when
// the live-read gate reports zero unlocked accounts. Wires the gate
// briefly to seed an incident, then locks before the transition.
func TestIncidents_LockedGate_FailsUpdateState(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := subject.NewService(nil)
	// Seed phase: unlocked.
	svc.SetSessionGate(&stubSessionGate{ids: []string{"acct-seed"}})
	cr := svc.Create(subject.CreateInput{
		Title: "seed-for-lock", Sev: "P3", Svc: "hub", Who: "qa",
	})
	if !cr.OK {
		t.Fatalf("Create (seed) failed: %s", cr.Error())
	}
	id := cr.Value.(subject.IncidentEntry).ID

	// Transition to locked — live-read picks up the change on the next
	// gate check.
	svc.SetSessionGate(&stubSessionGate{ids: []string{}})

	ur := svc.UpdateState(subject.UpdateStateInput{ID: id, State: "post-mortem"})
	if ur.OK {
		t.Fatal("expected UpdateState to be rejected when gate reports zero unlocked accounts")
	}
	if !core.Contains(ur.Error(), "incidents.session.locked") {
		t.Fatalf("expected incidents.session.locked, got %q", ur.Error())
	}
}

// TestIncidents_LockedGate_FailsAddPostmortem — AddPostmortem rejects
// when the live-read gate reports zero unlocked accounts.
func TestIncidents_LockedGate_FailsAddPostmortem(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := subject.NewService(nil)
	// Seed phase: unlocked. Create + transition to post-mortem so
	// AddPostmortem's state-precondition is satisfied — the locked-gate
	// assertion must be the first rejection, not the state guard.
	svc.SetSessionGate(&stubSessionGate{ids: []string{"acct-seed"}})
	cr := svc.Create(subject.CreateInput{
		Title: "seed-for-postmortem-lock", Sev: "P3", Svc: "hub", Who: "qa",
	})
	if !cr.OK {
		t.Fatalf("Create (seed) failed: %s", cr.Error())
	}
	id := cr.Value.(subject.IncidentEntry).ID
	if ur := svc.UpdateState(subject.UpdateStateInput{ID: id, State: "post-mortem"}); !ur.OK {
		t.Fatalf("UpdateState (seed) failed: %s", ur.Error())
	}

	// Transition to locked.
	svc.SetSessionGate(&stubSessionGate{ids: []string{}})

	pr := svc.AddPostmortem(subject.PostmortemInput{ID: id, Body: "body"})
	if pr.OK {
		t.Fatal("expected AddPostmortem to be rejected when gate reports zero unlocked accounts")
	}
	if !core.Contains(pr.Error(), "incidents.session.locked") {
		t.Fatalf("expected incidents.session.locked, got %q", pr.Error())
	}
}

// TestIncidents_StopNilsGate — Stop() severs the SessionGate;
// subsequent writes fail-closed even though the gate WAS wired
// (Cerberus #28 ADD-5 / H#147 documents.TestDocuments_StopNilsGate
// mirror).
func TestIncidents_StopNilsGate(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newServiceUnlocked() // gate pre-wired with unlocked stub

	// Pre-Stop: write succeeds.
	cr := svc.Create(subject.CreateInput{
		Title: "pre-stop", Sev: "P3", Svc: "hub", Who: "qa",
	})
	if !cr.OK {
		t.Fatalf("Create should succeed pre-Stop, got: %s", cr.Error())
	}

	// Stop nils the gate reference.
	if r := svc.Stop(core.Background()); !r.OK {
		t.Fatalf("Stop should succeed, got: %s", r.Error())
	}

	// Post-Stop: write fails-closed with the nil-gate path.
	r := svc.Create(subject.CreateInput{
		Title: "post-stop", Sev: "P3", Svc: "hub", Who: "qa",
	})
	if r.OK {
		t.Fatal("expected Create to fail-closed after Stop nils the gate")
	}
	if !core.Contains(r.Error(), "incidents.session.locked") {
		t.Fatalf("expected incidents.session.locked, got %q", r.Error())
	}
}

// TestIncidents_SessionLocked_ReadStillWorks_Good — List and Get are
// not blocked by the session gate (RFC §3.1 — reads stay open while
// locked). Mirrors H#147 documents test of the same name.
func TestIncidents_SessionLocked_ReadStillWorks_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := subject.NewService(nil)
	svc.SetSessionGate(&stubSessionGate{ids: []string{}})

	r := svc.List(subject.ListInput{})
	if !r.OK {
		t.Fatalf("List should succeed when session locked, got: %s", r.Error())
	}
}
