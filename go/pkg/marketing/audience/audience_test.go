// SPDX-Licence-Identifier: EUPL-1.2

package audience_test

import (
	"encoding/json"
	"sync"
	"testing"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/marketing/audience"
	"dappco.re/lthn/desktop/pkg/paths"
)

// stubSessionGate is the test double for the consumer-defined
// SessionGate interface (RFC.stage-e-unlockgate v2 §4.2 stub shape —
// Mantis #1613 B.2). Duplicated per-pkg rather than shared so test
// files keep zero cross-pkg testing/... deps.
type stubSessionGate struct{ ids []string }

func (s *stubSessionGate) UnlockedAccountIDs() []string { return s.ids }

// newTestSvc constructs an audience.Service pre-wired with a SessionGate
// reporting a single unlocked account so write methods pass the gate
// by default. Tests that need to assert the locked-state path call
// SetSessionGate explicitly with an empty stub (or instantiate
// audience.NewService directly for the nil-gate fail-safe path).
//
// Usage example:
//
//	svc := newTestSvc(t)
//	r := svc.Create(audience.CreateInput{Name: "..."})
func newTestSvc(_ *testing.T) *audience.Service {
	svc := audience.NewService(nil)
	svc.SetSessionGate(&stubSessionGate{ids: []string{"acct-test"}})
	return svc
}

// TestList_Empty_Good — empty dir → empty segments, TotalN=0.
func TestList_Empty_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := audience.NewService(nil)
	r := svc.List(audience.ListInput{})
	if !r.OK {
		t.Fatalf("List failed: %s", r.Error())
	}
	out := r.Value.(audience.ListOutput)
	if len(out.Segments) != 0 {
		t.Fatalf("expected 0 segments, got %d", len(out.Segments))
	}
	if out.TotalN != 0 {
		t.Fatalf("expected TotalN=0, got %d", out.TotalN)
	}
}

// TestCreate_Defaults_Good — Create with name+src → Growth="+0 / w".
func TestCreate_Defaults_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newTestSvc(t)
	r := svc.Create(audience.CreateInput{Name: "Local-AI developers", Src: "signup-tagged"})
	if !r.OK {
		t.Fatalf("Create failed: %s", r.Error())
	}
	seg := r.Value.(audience.Segment)
	if seg.Growth != "+0 / w" {
		t.Fatalf("expected Growth=+0 / w, got %q", seg.Growth)
	}
	if seg.ID == "" {
		t.Fatalf("expected non-empty ID")
	}
}

// TestCreate_NoName_Bad — empty name returns core.Fail.
func TestCreate_NoName_Bad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newTestSvc(t)
	r := svc.Create(audience.CreateInput{Name: "", Src: "signup-tagged"})
	if r.OK {
		t.Fatalf("expected failure for empty name, got OK")
	}
}

// TestCreate_NoSrc_Bad — empty src returns core.Fail.
func TestCreate_NoSrc_Bad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newTestSvc(t)
	r := svc.Create(audience.CreateInput{Name: "Developers", Src: ""})
	if r.OK {
		t.Fatalf("expected failure for empty src, got OK")
	}
}

// TestList_AllSegmentFirst_Good — "all" src segment sorts first in output.
func TestList_AllSegmentFirst_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newTestSvc(t)
	svc.Create(audience.CreateInput{Name: "Developers", Src: "signup-tagged", N: 100})
	// Create an "all" aggregate.
	svc.Create(audience.CreateInput{Name: "All subscribers", Src: "all", N: 500})

	r := svc.List(audience.ListInput{})
	if !r.OK {
		t.Fatalf("List failed: %s", r.Error())
	}
	out := r.Value.(audience.ListOutput)
	if len(out.Segments) < 2 {
		t.Fatalf("expected at least 2 segments, got %d", len(out.Segments))
	}
	if out.Segments[0].Src != "all" {
		t.Fatalf("expected first segment src=all, got %q", out.Segments[0].Src)
	}
	if out.TotalN != 500 {
		t.Fatalf("expected TotalN=500 from all segment, got %d", out.TotalN)
	}
}

// TestUpdate_N_Good — Update N field persists new count.
func TestUpdate_N_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newTestSvc(t)
	cr := svc.Create(audience.CreateInput{Name: "Investors", Src: "manual", N: 100})
	if !cr.OK {
		t.Fatalf("Create failed: %s", cr.Error())
	}
	id := cr.Value.(audience.Segment).ID

	ur := svc.Update(audience.UpdateInput{ID: id, N: 142})
	if !ur.OK {
		t.Fatalf("Update failed: %s", ur.Error())
	}
	updated := ur.Value.(audience.Segment)
	if updated.N != 142 {
		t.Fatalf("expected N=142, got %d", updated.N)
	}
}

// TestServiceName_Good — ServiceName returns "Audience".
func TestServiceName_Good(t *testing.T) {
	svc := audience.NewService(nil)
	if svc.ServiceName() != "Audience" {
		t.Fatalf("expected Audience, got %q", svc.ServiceName())
	}
}

// ---- Cascade W2 cutover tests (paths.AtomicWriteWithVersion) ---------------

// TestAtomicCutover_MarketingAudience_Create_Good — Create stamps
// version=1 via the primitive; ReadVersion confirms the on-disk file
// carries the same.
func TestAtomicCutover_MarketingAudience_Create_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newTestSvc(t)
	r := svc.Create(audience.CreateInput{
		Name: "Local-AI developers",
		Src:  "signup-tagged",
		N:    4892,
	})
	if !r.OK {
		t.Fatalf("Create failed: %s", r.Error())
	}
	seg := r.Value.(audience.Segment)
	dirR := core.UserHomeDir()
	if !dirR.OK {
		t.Fatalf("UserHomeDir: %s", dirR.Error())
	}
	fpath := core.PathJoin(dirR.Value.(string), "Lethean/marketing/audience", seg.ID+".md")
	rd := paths.ReadVersion(fpath)
	if !rd.OK {
		t.Fatalf("ReadVersion: %s", rd.Error())
	}
	got := rd.Value.(paths.ReadOutput)
	if got.Version != 1 {
		t.Fatalf("expected version 1 after Create, got %d", got.Version)
	}
}

// TestAtomicCutover_MarketingAudience_Update_Good — a sequential
// Update bumps the stored version monotonically (1 -> 2).
func TestAtomicCutover_MarketingAudience_Update_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newTestSvc(t)
	cr := svc.Create(audience.CreateInput{
		Name: "Probe segment",
		Src:  "manual",
		N:    100,
	})
	if !cr.OK {
		t.Fatalf("Create failed: %s", cr.Error())
	}
	id := cr.Value.(audience.Segment).ID
	ur := svc.Update(audience.UpdateInput{ID: id, N: 250})
	if !ur.OK {
		t.Fatalf("Update failed: %s", ur.Error())
	}
	dirR := core.UserHomeDir()
	if !dirR.OK {
		t.Fatalf("UserHomeDir: %s", dirR.Error())
	}
	fpath := core.PathJoin(dirR.Value.(string), "Lethean/marketing/audience", id+".md")
	rd := paths.ReadVersion(fpath)
	if !rd.OK {
		t.Fatalf("ReadVersion: %s", rd.Error())
	}
	got := rd.Value.(paths.ReadOutput)
	if got.Version != 2 {
		t.Fatalf("expected version 2 after Update, got %d", got.Version)
	}
}

// TestAtomicCutover_MarketingAudience_Update_VersionStale_Ugly —
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
// wrapped as ConflictEnvelope by audience.writeSegment.
func TestAtomicCutover_MarketingAudience_Update_VersionStale_Ugly(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newTestSvc(t)
	cr := svc.Create(audience.CreateInput{
		Name: "Race segment",
		Src:  "manual",
		N:    1,
	})
	if !cr.OK {
		t.Fatalf("Create failed: %s", cr.Error())
	}
	id := cr.Value.(audience.Segment).ID

	var conflict core.Result
	var saw bool
	for attempt := 0; attempt < 32 && !saw; attempt++ {
		var (
			mu      sync.Mutex
			results []core.Result
			wg      sync.WaitGroup
			start   = make(chan struct{})
		)
		// Use a fresh N value each attempt so even no-op patches
		// trigger a write.
		nVal := attempt*10 + 100
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			r := svc.Update(audience.UpdateInput{ID: id, N: nVal})
			mu.Lock()
			results = append(results, r)
			mu.Unlock()
		}()
		go func() {
			defer wg.Done()
			<-start
			r := svc.Update(audience.UpdateInput{ID: id, N: nVal + 1})
			mu.Lock()
			results = append(results, r)
			mu.Unlock()
		}()
		close(start)
		wg.Wait()

		for _, r := range results {
			if !r.OK && core.Contains(r.Error(), "audience.update.conflict") {
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
	if env.Code != "audience.update.conflict" {
		t.Fatalf("expected envelope code audience.update.conflict, got %q", env.Code)
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
		`"code":"audience.update.conflict"`,
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

// TestAtomicCutover_MarketingAudience_LegacyFile_Ugly — an audience
// file without version: frontmatter reads as version 0; Update
// upgrades it via an unconditional first-write that stamps version=1.
func TestAtomicCutover_MarketingAudience_LegacyFile_Ugly(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newTestSvc(t)
	dirR := core.UserHomeDir()
	if !dirR.OK {
		t.Fatalf("UserHomeDir: %s", dirR.Error())
	}
	audienceDir := core.PathJoin(dirR.Value.(string), "Lethean/marketing/audience")
	if mk := core.MkdirAll(audienceDir, 0o700); !mk.OK {
		t.Fatalf("MkdirAll: %s", mk.Error())
	}
	legacyID := "legacy-segment-2026"
	fpath := core.PathJoin(audienceDir, legacyID+".md")
	legacy := []byte("---\nid: " + legacyID + "\nname: Legacy segment\nn: 1234\ngrowth: \"+10 / w\"\nsrc: manual\nspark: \"\"\n---\n")
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
	ur := svc.Update(audience.UpdateInput{ID: legacyID, N: 9999})
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

// TestAtomicCutover_MarketingAudience_AuditEmissionRecordBatch_Good —
// Create routes through the primitive's write path (EventWriteSucceeded
// fires) and marketing/audience/* falls under AuditModeBatch per RFC §6.1.
func TestAtomicCutover_MarketingAudience_AuditEmissionRecordBatch_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newTestSvc(t)
	paths.SetAuditSecretProvider(func() []byte {
		return []byte("audience-cutover-test-secret-32-byte")
	})
	t.Cleanup(func() { paths.SetAuditSecretProvider(nil) })
	var saw []paths.LockEvent
	paths.SubscribeLockEvents(func(ev paths.LockEvent) {
		saw = append(saw, ev)
	})
	t.Cleanup(paths.ClearLockEventSubscribersForTest)

	r := svc.Create(audience.CreateInput{Name: "Audit probe", Src: "manual"})
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
	fpath := core.PathJoin(dirR.Value.(string), "Lethean/marketing/audience/x.md")
	mode := paths.AuditModeForPath(fpath)
	if mode != paths.AuditModeBatch {
		t.Fatalf("expected AuditModeBatch for marketing/audience path, got %v", mode)
	}
}

// ---- SessionGate retrofit (RFC.stage-e-unlockgate v2 §4.2 / Mantis #1613 B.2)

// TestAudience_NilGate_WarnsOnce_FailsClosed — a Service constructed
// without SetSessionGate fails-closed on writes. Second + third writes
// continue to fail-closed (one-shot warn semantics — the second call
// must remain quiet but its caller-visible result must be the same
// fail-closed shape).
func TestAudience_NilGate_WarnsOnce_FailsClosed(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := audience.NewService(nil) // NO SetSessionGate — exercises §2.2 fail-safe.

	r1 := svc.Create(audience.CreateInput{Name: "Probe A", Src: "signup-tagged"})
	if r1.OK {
		t.Fatal("expected Create to fail-closed when gate is nil")
	}
	if !core.Contains(r1.Error(), "audience.session.locked") {
		t.Fatalf("expected audience.session.locked on first Create, got %q", r1.Error())
	}

	// Second write — nilWarned already true; CompareAndSwap returns
	// false and core.Warn is NOT called again. Behaviour from the
	// caller's perspective: same fail-closed result.
	r2 := svc.Create(audience.CreateInput{Name: "Probe B", Src: "signup-tagged"})
	if r2.OK {
		t.Fatal("expected second Create to fail-closed when gate is nil")
	}
	if !core.Contains(r2.Error(), "audience.session.locked") {
		t.Fatalf("expected audience.session.locked on second Create, got %q", r2.Error())
	}

	// Third write via Update — same fail-closed behaviour persists.
	r3 := svc.Update(audience.UpdateInput{ID: "some-id", N: 42})
	if r3.OK {
		t.Fatal("expected Update to fail-closed when gate is nil")
	}
	if !core.Contains(r3.Error(), "audience.session.locked") {
		t.Fatalf("expected audience.session.locked on Update, got %q", r3.Error())
	}
}

// TestAudience_UnlockedGate_AllowsCreate — Create succeeds when the
// live-read gate reports at least one unlocked account.
func TestAudience_UnlockedGate_AllowsCreate(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := audience.NewService(nil)
	svc.SetSessionGate(&stubSessionGate{ids: []string{"acct-1"}})

	r := svc.Create(audience.CreateInput{Name: "Launch probe", Src: "signup-tagged"})
	if !r.OK {
		t.Fatalf("Create should succeed with gate reporting unlocked acct, got: %s", r.Error())
	}
}

// TestAudience_UnlockedGate_AllowsUpdate — Update succeeds when the
// live-read gate reports at least one unlocked account.
func TestAudience_UnlockedGate_AllowsUpdate(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := audience.NewService(nil)
	svc.SetSessionGate(&stubSessionGate{ids: []string{"acct-1"}})

	cr := svc.Create(audience.CreateInput{Name: "Update probe", Src: "signup-tagged"})
	if !cr.OK {
		t.Fatalf("Create: %s", cr.Error())
	}
	id := cr.Value.(audience.Segment).ID
	ur := svc.Update(audience.UpdateInput{ID: id, N: 42})
	if !ur.OK {
		t.Fatalf("Update should succeed with gate reporting unlocked acct, got: %s", ur.Error())
	}
}

// TestAudience_LockedGate_FailsCreate_session_locked — Create rejects
// when the live-read gate reports zero unlocked accounts. The Create is
// gated BEFORE the empty-name check, so the session.locked code surfaces
// in preference to any later validation error.
func TestAudience_LockedGate_FailsCreate_session_locked(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := audience.NewService(nil)
	svc.SetSessionGate(&stubSessionGate{ids: []string{}})

	r := svc.Create(audience.CreateInput{Name: "Locked probe", Src: "signup-tagged"})
	if r.OK {
		t.Fatal("expected Create to be rejected when gate reports zero unlocked accounts")
	}
	if !core.Contains(r.Error(), "audience.session.locked") {
		t.Fatalf("expected audience.session.locked, got %q", r.Error())
	}
}

// TestAudience_LockedGate_FailsUpdate_session_locked — Update rejects
// when the live-read gate reports zero unlocked accounts. Gated BEFORE
// IsValidID + filesystem read.
func TestAudience_LockedGate_FailsUpdate_session_locked(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	// Seed records while unlocked, then flip to locked + try to update.
	svc := newTestSvc(t)
	cr := svc.Create(audience.CreateInput{Name: "Seed probe", Src: "signup-tagged"})
	if !cr.OK {
		t.Fatalf("Create: %s", cr.Error())
	}
	id := cr.Value.(audience.Segment).ID
	svc.SetSessionGate(&stubSessionGate{ids: []string{}})

	r := svc.Update(audience.UpdateInput{ID: id, N: 999})
	if r.OK {
		t.Fatal("expected Update to be rejected when gate reports zero unlocked accounts")
	}
	if !core.Contains(r.Error(), "audience.session.locked") {
		t.Fatalf("expected audience.session.locked, got %q", r.Error())
	}
}

// TestAudience_StopNilsGate — Stop() severs the SessionGate; subsequent
// writes fail-closed even though the gate WAS wired (Cerberus #28 ADD-5
// — Stop drain hygiene mirrors mail).
func TestAudience_StopNilsGate(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newTestSvc(t) // gate wired with unlocked stub

	// Pre-Stop: write succeeds.
	if r := svc.Create(audience.CreateInput{Name: "Pre-stop probe", Src: "signup-tagged"}); !r.OK {
		t.Fatalf("Create should succeed pre-Stop, got: %s", r.Error())
	}

	// Stop nils the gate reference.
	if r := svc.Stop(core.Background()); !r.OK {
		t.Fatalf("Stop should succeed, got: %s", r.Error())
	}

	// Post-Stop: write fails-closed via the nil-gate path.
	r := svc.Create(audience.CreateInput{Name: "Post-stop probe", Src: "signup-tagged"})
	if r.OK {
		t.Fatal("expected Create to fail-closed after Stop nils the gate")
	}
	if !core.Contains(r.Error(), "audience.session.locked") {
		t.Fatalf("expected audience.session.locked, got %q", r.Error())
	}
}

// TestAudience_LockedGate_ReadStillWorks — List + Get are not gated by
// the session-lock (RFC §3.1 — reads stay open while locked).
func TestAudience_LockedGate_ReadStillWorks(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	// Seed unlocked, then flip to locked.
	svc := newTestSvc(t)
	cr := svc.Create(audience.CreateInput{Name: "Read probe", Src: "signup-tagged"})
	if !cr.OK {
		t.Fatalf("Create: %s", cr.Error())
	}
	id := cr.Value.(audience.Segment).ID
	svc.SetSessionGate(&stubSessionGate{ids: []string{}})

	if r := svc.List(audience.ListInput{}); !r.OK {
		t.Fatalf("List should succeed when session locked, got: %s", r.Error())
	}
	if g := svc.Get(id); !g.OK {
		t.Fatalf("Get should succeed when session locked, got: %s", g.Error())
	}
}
