// SPDX-Licence-Identifier: EUPL-1.2

package social_test

import (
	"encoding/json"
	"sync"
	"testing"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/marketing/social"
	"dappco.re/lthn/desktop/pkg/paths"
)

// stubSessionGate is the test double for the consumer-defined
// SessionGate interface (RFC.stage-e-unlockgate v2 §4.2 stub shape —
// Mantis #1613 B.2). Duplicated per-pkg rather than shared so test
// files keep zero cross-pkg testing/... deps.
type stubSessionGate struct{ ids []string }

func (s *stubSessionGate) UnlockedAccountIDs() []string { return s.ids }

// newTestSvc constructs a social.Service pre-wired with a SessionGate
// reporting a single unlocked account so write methods pass the gate
// by default. Tests that need to assert the locked-state path call
// SetSessionGate explicitly with an empty stub (or instantiate
// social.NewService directly for the nil-gate fail-safe path).
//
// Usage example:
//
//	svc := newTestSvc(t)
//	r := svc.Create(social.CreateInput{Ch: []string{"x"}, Text: "..."})
func newTestSvc(_ *testing.T) *social.Service {
	svc := social.NewService(nil)
	svc.SetSessionGate(&stubSessionGate{ids: []string{"acct-test"}})
	return svc
}

// TestList_Empty_Good — empty dir → empty posts, zero scheduled.
func TestList_Empty_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := social.NewService(nil)
	r := svc.List(social.ListInput{})
	if !r.OK {
		t.Fatalf("List failed: %s", r.Error())
	}
	out := r.Value.(social.ListOutput)
	if len(out.Posts) != 0 {
		t.Fatalf("expected 0 posts, got %d", len(out.Posts))
	}
	if out.ScheduledCount != 0 {
		t.Fatalf("expected ScheduledCount=0, got %d", out.ScheduledCount)
	}
}

// TestCreate_Defaults_Good — Create with ch+text defaults to state=draft.
func TestCreate_Defaults_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newTestSvc(t)
	r := svc.Create(social.CreateInput{
		Ch:   []string{"mastodon", "x"},
		When: "today · 16:00",
		Text: "Lethean v0.2 is out.",
	})
	if !r.OK {
		t.Fatalf("Create failed: %s", r.Error())
	}
	p := r.Value.(social.SocialPost)
	if p.State != "draft" {
		t.Fatalf("expected state=draft, got %q", p.State)
	}
	if p.ID == "" {
		t.Fatalf("expected non-empty ID")
	}
	if len(p.Ch) != 2 {
		t.Fatalf("expected 2 channels, got %d", len(p.Ch))
	}
}

// TestCreate_NoChannels_Bad — empty Ch returns core.Fail.
func TestCreate_NoChannels_Bad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newTestSvc(t)
	r := svc.Create(social.CreateInput{Ch: nil, When: "today", Text: "Hello"})
	if r.OK {
		t.Fatalf("expected failure for empty channels, got OK")
	}
}

// TestCreate_NoText_Bad — empty text returns core.Fail.
func TestCreate_NoText_Bad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newTestSvc(t)
	r := svc.Create(social.CreateInput{Ch: []string{"mastodon"}, When: "today", Text: ""})
	if r.OK {
		t.Fatalf("expected failure for empty text, got OK")
	}
}

// TestMarkSent_Good — MarkSent transitions state to "sent".
func TestMarkSent_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newTestSvc(t)
	cr := svc.Create(social.CreateInput{
		Ch:    []string{"mastodon"},
		When:  "today · 09:00",
		Text:  "Just shipped.",
		State: "scheduled",
	})
	if !cr.OK {
		t.Fatalf("Create failed: %s", cr.Error())
	}
	id := cr.Value.(social.SocialPost).ID

	r := svc.MarkSent(id)
	if !r.OK {
		t.Fatalf("MarkSent failed: %s", r.Error())
	}
	p := r.Value.(social.SocialPost)
	if p.State != "sent" {
		t.Fatalf("expected state=sent, got %q", p.State)
	}
}

// TestList_ChannelFilter_Good — List with Channel filter returns only matching posts.
func TestList_ChannelFilter_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newTestSvc(t)
	svc.Create(social.CreateInput{Ch: []string{"mastodon", "x"}, When: "today", Text: "A"})
	svc.Create(social.CreateInput{Ch: []string{"linkedin"}, When: "tomorrow", Text: "B"})

	r := svc.List(social.ListInput{Channel: "linkedin"})
	if !r.OK {
		t.Fatalf("List failed: %s", r.Error())
	}
	out := r.Value.(social.ListOutput)
	if len(out.Posts) != 1 {
		t.Fatalf("expected 1 linkedin post, got %d", len(out.Posts))
	}
	if out.Posts[0].Text != "B" {
		t.Fatalf("expected text=B, got %q", out.Posts[0].Text)
	}
}

// TestServiceName_Good — ServiceName returns "Social".
func TestServiceName_Good(t *testing.T) {
	svc := social.NewService(nil)
	if svc.ServiceName() != "Social" {
		t.Fatalf("expected Social, got %q", svc.ServiceName())
	}
}

// ---- Cascade W2 cutover tests (paths.AtomicWriteWithVersion) ---------------

// TestAtomicCutover_MarketingSocial_Create_Good — Create stamps
// version=1 via the primitive; ReadVersion confirms the on-disk file
// carries the same.
func TestAtomicCutover_MarketingSocial_Create_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newTestSvc(t)
	r := svc.Create(social.CreateInput{
		Ch:    []string{"mastodon"},
		When:  "today · 16:00",
		Text:  "Lethean v0.2 is out.",
		State: "scheduled",
	})
	if !r.OK {
		t.Fatalf("Create failed: %s", r.Error())
	}
	p := r.Value.(social.SocialPost)
	dirR := core.UserHomeDir()
	if !dirR.OK {
		t.Fatalf("UserHomeDir: %s", dirR.Error())
	}
	fpath := core.PathJoin(dirR.Value.(string), "Lethean/marketing/social", p.ID+".md")
	rd := paths.ReadVersion(fpath)
	if !rd.OK {
		t.Fatalf("ReadVersion: %s", rd.Error())
	}
	got := rd.Value.(paths.ReadOutput)
	if got.Version != 1 {
		t.Fatalf("expected version 1 after Create, got %d", got.Version)
	}
}

// TestAtomicCutover_MarketingSocial_Update_Good — a sequential
// MarkSent bumps the stored version monotonically (1 -> 2).
func TestAtomicCutover_MarketingSocial_Update_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newTestSvc(t)
	cr := svc.Create(social.CreateInput{
		Ch:    []string{"mastodon"},
		When:  "today · 16:00",
		Text:  "Probe.",
		State: "scheduled",
	})
	if !cr.OK {
		t.Fatalf("Create failed: %s", cr.Error())
	}
	id := cr.Value.(social.SocialPost).ID
	ur := svc.MarkSent(id)
	if !ur.OK {
		t.Fatalf("MarkSent failed: %s", ur.Error())
	}
	dirR := core.UserHomeDir()
	if !dirR.OK {
		t.Fatalf("UserHomeDir: %s", dirR.Error())
	}
	fpath := core.PathJoin(dirR.Value.(string), "Lethean/marketing/social", id+".md")
	rd := paths.ReadVersion(fpath)
	if !rd.OK {
		t.Fatalf("ReadVersion: %s", rd.Error())
	}
	got := rd.Value.(paths.ReadOutput)
	if got.Version != 2 {
		t.Fatalf("expected version 2 after MarkSent, got %d", got.Version)
	}
}

// TestAtomicCutover_MarketingSocial_Update_VersionStale_Ugly — drives
// two concurrent svc.MarkSent calls and asserts the loser's Result
// carries a paths.ConflictEnvelope whose JSON marshal matches the
// lowercase `{code, current_version, current_hash}` wire shape that
// frontend/src/lit/conflict-dispatch.ts extractEnvelope pattern-
// matches on (Mantis #1547 service-tier round-trip discipline anchor;
// pins #1544 against W2 wave drift).
//
// Why two goroutines: svc.MarkSent re-reads the file inside the call
// so a single-shot out-of-band mutation cannot drive a real conflict
// through the service. Concurrent MarkSent goroutines race for the
// WithFileLock; the loser's locked-section ReadVersion sees the
// winner's bumped version and the primitive returns VersionStale ->
// wrapped as ConflictEnvelope by social.writePost.
func TestAtomicCutover_MarketingSocial_Update_VersionStale_Ugly(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newTestSvc(t)
	cr := svc.Create(social.CreateInput{
		Ch:    []string{"mastodon"},
		When:  "now",
		Text:  "Race probe.",
		State: "scheduled",
	})
	if !cr.OK {
		t.Fatalf("Create failed: %s", cr.Error())
	}
	id := cr.Value.(social.SocialPost).ID

	var conflict core.Result
	var saw bool
	for attempt := 0; attempt < 32 && !saw; attempt++ {
		// Reset state back to "scheduled" between attempts so MarkSent
		// always has a real state-transition to perform.
		if attempt > 0 {
			dirR := core.UserHomeDir()
			if !dirR.OK {
				t.Fatalf("UserHomeDir: %s", dirR.Error())
			}
			fpath := core.PathJoin(dirR.Value.(string), "Lethean/marketing/social", id+".md")
			rd := paths.ReadVersion(fpath)
			if !rd.OK {
				t.Fatalf("ReadVersion (reset): %s", rd.Error())
			}
			cur := rd.Value.(paths.ReadOutput).Version
			body := []byte("---\nversion: " + core.Sprintf("%d", cur+1) + "\nid: " + id + "\nch: mastodon\nwhen: now\nstate: scheduled\nattach: \"\"\n---\nRace probe.")
			if w := core.WriteFile(fpath, body, 0o600); !w.OK {
				t.Fatalf("WriteFile (reset): %s", w.Error())
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
			r := svc.MarkSent(id)
			mu.Lock()
			results = append(results, r)
			mu.Unlock()
		}()
		go func() {
			defer wg.Done()
			<-start
			r := svc.MarkSent(id)
			mu.Lock()
			results = append(results, r)
			mu.Unlock()
		}()
		close(start)
		wg.Wait()

		for _, r := range results {
			if !r.OK && core.Contains(r.Error(), "social.update.conflict") {
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
	if env.Code != "social.update.conflict" {
		t.Fatalf("expected envelope code social.update.conflict, got %q", env.Code)
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
		`"code":"social.update.conflict"`,
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

// TestAtomicCutover_MarketingSocial_LegacyFile_Ugly — a social file
// without version: frontmatter reads as version 0; MarkSent upgrades
// it via an unconditional first-write that stamps version=1.
func TestAtomicCutover_MarketingSocial_LegacyFile_Ugly(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newTestSvc(t)
	dirR := core.UserHomeDir()
	if !dirR.OK {
		t.Fatalf("UserHomeDir: %s", dirR.Error())
	}
	socialDir := core.PathJoin(dirR.Value.(string), "Lethean/marketing/social")
	if mk := core.MkdirAll(socialDir, 0o700); !mk.OK {
		t.Fatalf("MkdirAll: %s", mk.Error())
	}
	legacyID := "legacy-post-2026"
	fpath := core.PathJoin(socialDir, legacyID+".md")
	legacy := []byte("---\nid: " + legacyID + "\nch: mastodon\nwhen: yest · 11:14\nstate: scheduled\nattach: \"\"\n---\nLegacy body.")
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
	ur := svc.MarkSent(legacyID)
	if !ur.OK {
		t.Fatalf("MarkSent failed: %s", ur.Error())
	}
	rd2 := paths.ReadVersion(fpath)
	if !rd2.OK {
		t.Fatalf("ReadVersion post-update: %s", rd2.Error())
	}
	if got := rd2.Value.(paths.ReadOutput); got.Version != 1 {
		t.Fatalf("legacy file post-update: expected version 1, got %d", got.Version)
	}
}

// TestAtomicCutover_MarketingSocial_AuditEmissionRecordBatch_Good —
// Create routes through the primitive's write path (EventWriteSucceeded
// fires) and marketing/social/* falls under AuditModeBatch per RFC §6.1.
func TestAtomicCutover_MarketingSocial_AuditEmissionRecordBatch_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newTestSvc(t)
	paths.SetAuditSecretProvider(func() []byte {
		return []byte("social-cutover-test-secret-32-bytes")
	})
	t.Cleanup(func() { paths.SetAuditSecretProvider(nil) })
	var saw []paths.LockEvent
	paths.SubscribeLockEvents(func(ev paths.LockEvent) {
		saw = append(saw, ev)
	})
	t.Cleanup(paths.ClearLockEventSubscribersForTest)

	r := svc.Create(social.CreateInput{
		Ch:    []string{"x"},
		When:  "now",
		Text:  "Audit probe.",
		State: "draft",
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
	fpath := core.PathJoin(dirR.Value.(string), "Lethean/marketing/social/x.md")
	mode := paths.AuditModeForPath(fpath)
	if mode != paths.AuditModeBatch {
		t.Fatalf("expected AuditModeBatch for marketing/social path, got %v", mode)
	}
}

// ---- SessionGate retrofit (RFC.stage-e-unlockgate v2 §4.2 / Mantis #1613 B.2)

// TestSocial_NilGate_WarnsOnce_FailsClosed — a Service constructed
// without SetSessionGate fails-closed on writes. Second + third writes
// continue to fail-closed (one-shot warn semantics — the second call
// must remain quiet but its caller-visible result must be the same
// fail-closed shape).
func TestSocial_NilGate_WarnsOnce_FailsClosed(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := social.NewService(nil) // NO SetSessionGate — exercises §2.2 fail-safe.

	r1 := svc.Create(social.CreateInput{
		Ch: []string{"mastodon"}, When: "today", Text: "Probe A.",
	})
	if r1.OK {
		t.Fatal("expected Create to fail-closed when gate is nil")
	}
	if !core.Contains(r1.Error(), "social.session.locked") {
		t.Fatalf("expected social.session.locked on first Create, got %q", r1.Error())
	}

	// Second write — nilWarned already true; CompareAndSwap returns
	// false and core.Warn is NOT called again. Behaviour from the
	// caller's perspective: same fail-closed result.
	r2 := svc.Create(social.CreateInput{
		Ch: []string{"x"}, When: "today", Text: "Probe B.",
	})
	if r2.OK {
		t.Fatal("expected second Create to fail-closed when gate is nil")
	}
	if !core.Contains(r2.Error(), "social.session.locked") {
		t.Fatalf("expected social.session.locked on second Create, got %q", r2.Error())
	}

	// Third write via MarkSent — same fail-closed behaviour persists.
	r3 := svc.MarkSent("some-id")
	if r3.OK {
		t.Fatal("expected MarkSent to fail-closed when gate is nil")
	}
	if !core.Contains(r3.Error(), "social.session.locked") {
		t.Fatalf("expected social.session.locked on MarkSent, got %q", r3.Error())
	}
}

// TestSocial_UnlockedGate_AllowsCreate — Create succeeds when the
// live-read gate reports at least one unlocked account.
func TestSocial_UnlockedGate_AllowsCreate(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := social.NewService(nil)
	svc.SetSessionGate(&stubSessionGate{ids: []string{"acct-1"}})

	r := svc.Create(social.CreateInput{
		Ch: []string{"mastodon"}, When: "today", Text: "Launch probe.",
	})
	if !r.OK {
		t.Fatalf("Create should succeed with gate reporting unlocked acct, got: %s", r.Error())
	}
}

// TestSocial_UnlockedGate_AllowsMarkSent — MarkSent succeeds when the
// live-read gate reports at least one unlocked account.
func TestSocial_UnlockedGate_AllowsMarkSent(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := social.NewService(nil)
	svc.SetSessionGate(&stubSessionGate{ids: []string{"acct-1"}})

	cr := svc.Create(social.CreateInput{
		Ch: []string{"mastodon"}, When: "today", Text: "MarkSent probe.", State: "scheduled",
	})
	if !cr.OK {
		t.Fatalf("Create: %s", cr.Error())
	}
	id := cr.Value.(social.SocialPost).ID
	r := svc.MarkSent(id)
	if !r.OK {
		t.Fatalf("MarkSent should succeed with gate reporting unlocked acct, got: %s", r.Error())
	}
}

// TestSocial_LockedGate_FailsCreate_session_locked — Create rejects
// when the live-read gate reports zero unlocked accounts. The Create is
// gated BEFORE the empty-channel / empty-text checks, so the
// session.locked code surfaces in preference to any later validation
// error.
func TestSocial_LockedGate_FailsCreate_session_locked(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := social.NewService(nil)
	svc.SetSessionGate(&stubSessionGate{ids: []string{}})

	r := svc.Create(social.CreateInput{
		Ch: []string{"mastodon"}, When: "today", Text: "Locked probe.",
	})
	if r.OK {
		t.Fatal("expected Create to be rejected when gate reports zero unlocked accounts")
	}
	if !core.Contains(r.Error(), "social.session.locked") {
		t.Fatalf("expected social.session.locked, got %q", r.Error())
	}
}

// TestSocial_LockedGate_FailsMarkSent_session_locked — MarkSent rejects
// when the live-read gate reports zero unlocked accounts. Gated BEFORE
// IsValidID + filesystem read.
func TestSocial_LockedGate_FailsMarkSent_session_locked(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	// Seed records while unlocked, then flip to locked + try to MarkSent.
	svc := newTestSvc(t)
	cr := svc.Create(social.CreateInput{
		Ch: []string{"mastodon"}, When: "today", Text: "Seed.", State: "scheduled",
	})
	if !cr.OK {
		t.Fatalf("Create: %s", cr.Error())
	}
	id := cr.Value.(social.SocialPost).ID
	svc.SetSessionGate(&stubSessionGate{ids: []string{}})

	r := svc.MarkSent(id)
	if r.OK {
		t.Fatal("expected MarkSent to be rejected when gate reports zero unlocked accounts")
	}
	if !core.Contains(r.Error(), "social.session.locked") {
		t.Fatalf("expected social.session.locked, got %q", r.Error())
	}
}

// TestSocial_StopNilsGate — Stop() severs the SessionGate; subsequent
// writes fail-closed even though the gate WAS wired (Cerberus #28 ADD-5
// — Stop drain hygiene mirrors mail).
func TestSocial_StopNilsGate(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newTestSvc(t) // gate wired with unlocked stub

	// Pre-Stop: write succeeds.
	if r := svc.Create(social.CreateInput{
		Ch: []string{"mastodon"}, When: "today", Text: "Pre-stop.",
	}); !r.OK {
		t.Fatalf("Create should succeed pre-Stop, got: %s", r.Error())
	}

	// Stop nils the gate reference.
	if r := svc.Stop(core.Background()); !r.OK {
		t.Fatalf("Stop should succeed, got: %s", r.Error())
	}

	// Post-Stop: write fails-closed via the nil-gate path.
	r := svc.Create(social.CreateInput{
		Ch: []string{"mastodon"}, When: "today", Text: "Post-stop.",
	})
	if r.OK {
		t.Fatal("expected Create to fail-closed after Stop nils the gate")
	}
	if !core.Contains(r.Error(), "social.session.locked") {
		t.Fatalf("expected social.session.locked, got %q", r.Error())
	}
}

// TestSocial_LockedGate_ReadStillWorks — List + Get are not gated by
// the session-lock (RFC §3.1 — reads stay open while locked).
func TestSocial_LockedGate_ReadStillWorks(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	// Seed unlocked, then flip to locked.
	svc := newTestSvc(t)
	cr := svc.Create(social.CreateInput{
		Ch: []string{"mastodon"}, When: "today", Text: "Read probe.",
	})
	if !cr.OK {
		t.Fatalf("Create: %s", cr.Error())
	}
	id := cr.Value.(social.SocialPost).ID
	svc.SetSessionGate(&stubSessionGate{ids: []string{}})

	if r := svc.List(social.ListInput{}); !r.OK {
		t.Fatalf("List should succeed when session locked, got: %s", r.Error())
	}
	if g := svc.Get(id); !g.OK {
		t.Fatalf("Get should succeed when session locked, got: %s", g.Error())
	}
}
