// SPDX-Licence-Identifier: EUPL-1.2

package runbooks_test

import (
	"encoding/json"
	"sync"
	"testing"

	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/runbooks"
	"dappco.re/lthn/desktop/pkg/paths"
)

func TestParseRecord_RoundTrip(t *testing.T) {
	now := core.Now().UTC()
	rec := subject.RunbookRecord{
		ID:            "R-01",
		Title:         "Rotate runtime API keys",
		Area:          "auth",
		Tags:          []string{"auth", "api-keys"},
		Slug:          "rotate-runtime-api-keys",
		LastRehearsed: now.Add(-2 * 24 * core.Hour),
		CreatedAt:     now.Add(-90 * 24 * core.Hour),
	}
	raw, err := subject.MarshalRecordExported(rec)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got, err := subject.ParseRecordExported(rec.Slug, raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got.ID != rec.ID {
		t.Errorf("id: want %q got %q", rec.ID, got.ID)
	}
	if got.Title != rec.Title {
		t.Errorf("title: want %q got %q", rec.Title, got.Title)
	}
	if got.Area != rec.Area {
		t.Errorf("area: want %q got %q", rec.Area, got.Area)
	}
	if len(got.Tags) != len(rec.Tags) {
		t.Errorf("tags: want %v got %v", rec.Tags, got.Tags)
	}
}

func TestComputeHealth_Fresh(t *testing.T) {
	now := core.Now()
	rec := subject.RunbookRecord{LastRehearsed: now.Add(-2 * 24 * core.Hour)}
	if h := subject.ComputeHealthExported(rec, now); h != "fresh" {
		t.Errorf("want 'fresh' got %q", h)
	}
}

func TestComputeHealth_Aging(t *testing.T) {
	now := core.Now()
	rec := subject.RunbookRecord{LastRehearsed: now.Add(-45 * 24 * core.Hour)}
	if h := subject.ComputeHealthExported(rec, now); h != "aging" {
		t.Errorf("want 'aging' got %q", h)
	}
}

func TestComputeHealth_Stale(t *testing.T) {
	now := core.Now()
	rec := subject.RunbookRecord{LastRehearsed: now.Add(-120 * 24 * core.Hour)}
	if h := subject.ComputeHealthExported(rec, now); h != "stale" {
		t.Errorf("want 'stale' got %q", h)
	}
}

func TestComputeHealth_NeverRehearsed(t *testing.T) {
	now := core.Now()
	rec := subject.RunbookRecord{} // zero LastRehearsed
	if h := subject.ComputeHealthExported(rec, now); h != "stale" {
		t.Errorf("want 'stale' got %q", h)
	}
}

func TestRelativeTime_Day(t *testing.T) {
	now := core.Now()
	past := now.Add(-2 * 24 * core.Hour)
	got := subject.RelativeTimeExported(past, now)
	if got != "2 d" {
		t.Errorf("want '2 d' got %q", got)
	}
}

func TestRelativeTime_Week(t *testing.T) {
	now := core.Now()
	past := now.Add(-3 * 7 * 24 * core.Hour)
	got := subject.RelativeTimeExported(past, now)
	if got != "3 w" {
		t.Errorf("want '3 w' got %q", got)
	}
}

func TestRelativeTime_Month(t *testing.T) {
	now := core.Now()
	past := now.Add(-4 * 30 * 24 * core.Hour)
	got := subject.RelativeTimeExported(past, now)
	if got != "4 mo" {
		t.Errorf("want '4 mo' got %q", got)
	}
}

func TestRelativeTime_Never(t *testing.T) {
	now := core.Now()
	got := subject.RelativeTimeExported(core.Time{}, now)
	if got != "never" {
		t.Errorf("want 'never' got %q", got)
	}
}

func TestMatchSearch_Title(t *testing.T) {
	rec := subject.RunbookRecord{Title: "Drain a stuck Postfix queue", Area: "mail", ID: "R-05"}
	if !subject.MatchSearchExported(rec, "postfix") {
		t.Error("expected match on 'postfix'")
	}
}

func TestMatchSearch_Area(t *testing.T) {
	rec := subject.RunbookRecord{Title: "Trigger emergency model unload", Area: "runtime", ID: "R-06"}
	if !subject.MatchSearchExported(rec, "runtime") {
		t.Error("expected match on 'runtime'")
	}
}

func TestMatchSearch_Empty(t *testing.T) {
	rec := subject.RunbookRecord{Title: "anything", Area: "anything", ID: "R-01"}
	if !subject.MatchSearchExported(rec, "") {
		t.Error("empty query should match everything")
	}
}

func TestMatchSearch_NoMatch(t *testing.T) {
	rec := subject.RunbookRecord{Title: "Rotate runtime API keys", Area: "auth", ID: "R-01"}
	if subject.MatchSearchExported(rec, "postfix") {
		t.Error("expected no match on 'postfix'")
	}
}

func TestServiceName_Runbooks(t *testing.T) {
	svc := subject.NewService(nil)
	if got := svc.ServiceName(); got != "Runbooks" {
		t.Errorf("want 'Runbooks' got %q", got)
	}
}

// ---- Cascade W3 cutover tests (paths.AtomicWriteWithVersion) ---------------

// runbookFilePath resolves the on-disk file for a runbook slug —
// writeRecord lands under ~/Lethean/runbooks/<slug>.md.
func runbookFilePath(t *testing.T, slug string) string {
	t.Helper()
	dirR := core.UserHomeDir()
	if !dirR.OK {
		t.Fatalf("UserHomeDir: %s", dirR.Error())
	}
	return core.PathJoin(dirR.Value.(string),
		"Lethean/runbooks", slug+".md")
}

// TestAtomicCutover_Runbooks_Create_Good — OnStart seeds the
// runbook library; each seed lands at version=1 via the primitive.
// (Runbooks has no public Create API — seedAll IS the create-path.)
func TestAtomicCutover_Runbooks_Create_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := subject.NewService(nil)
	if r := svc.OnStart(); !r.OK {
		t.Fatalf("OnStart: %s", r.Error())
	}
	// Pick a known seed slug per service.go seedAll().
	fpath := runbookFilePath(t, "rotate-runtime-api-keys")
	rd := paths.ReadVersion(fpath)
	if !rd.OK {
		t.Fatalf("ReadVersion: %s", rd.Error())
	}
	got := rd.Value.(paths.ReadOutput)
	if got.Version != 1 {
		t.Fatalf("expected version 1 after seed, got %d", got.Version)
	}
}

// TestAtomicCutover_Runbooks_Update_Good — a sequential MarkRehearsed
// bumps the stored version monotonically (1 -> 2).
func TestAtomicCutover_Runbooks_Update_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := subject.NewService(nil)
	// Seed first so MarkRehearsed has a target.
	if r := svc.OnStart(); !r.OK {
		t.Fatalf("OnStart: %s", r.Error())
	}
	ur := svc.MarkRehearsed(subject.MarkInput{ID: "R-01"})
	if !ur.OK {
		t.Fatalf("MarkRehearsed failed: %s", ur.Error())
	}
	fpath := runbookFilePath(t, "rotate-runtime-api-keys")
	rd := paths.ReadVersion(fpath)
	if !rd.OK {
		t.Fatalf("ReadVersion: %s", rd.Error())
	}
	got := rd.Value.(paths.ReadOutput)
	if got.Version != 2 {
		t.Fatalf("expected version 2 after MarkRehearsed, got %d", got.Version)
	}
}

// TestAtomicCutover_Runbooks_Update_VersionStale_Ugly — drives two
// concurrent svc.MarkRehearsed calls and asserts the loser's Result
// carries a paths.ConflictEnvelope whose JSON marshal matches the
// lowercase `{code, current_version, current_hash}` wire shape that
// frontend/src/lit/conflict-dispatch.ts extractEnvelope pattern-
// matches on (Mantis #1547 service-tier round-trip discipline anchor;
// pins #1544 against W3 wave drift).
func TestAtomicCutover_Runbooks_Update_VersionStale_Ugly(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := subject.NewService(nil)
	if r := svc.OnStart(); !r.OK {
		t.Fatalf("OnStart: %s", r.Error())
	}

	var conflict core.Result
	var saw bool
	for attempt := 0; attempt < 32 && !saw; attempt++ {
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
			r := svc.MarkRehearsed(subject.MarkInput{ID: "R-01"})
			mu.Lock()
			results = append(results, r)
			mu.Unlock()
		}()
		go func() {
			defer wg.Done()
			<-start
			r := svc.MarkRehearsed(subject.MarkInput{ID: "R-01"})
			mu.Lock()
			results = append(results, r)
			mu.Unlock()
		}()
		close(start)
		wg.Wait()

		for _, r := range results {
			if !r.OK && core.Contains(r.Error(), "runbooks.update.conflict") {
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
	if env.Code != "runbooks.update.conflict" {
		t.Fatalf("expected envelope code runbooks.update.conflict, got %q", env.Code)
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
		`"code":"runbooks.update.conflict"`,
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

// TestAtomicCutover_Runbooks_LegacyFile_Ugly — a runbook file without
// version: frontmatter reads as version 0; MarkRehearsed upgrades it
// via an unconditional first-write that stamps version=1.
func TestAtomicCutover_Runbooks_LegacyFile_Ugly(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := subject.NewService(nil)
	dirR := core.UserHomeDir()
	if !dirR.OK {
		t.Fatalf("UserHomeDir: %s", dirR.Error())
	}
	rbDir := core.PathJoin(dirR.Value.(string), "Lethean/runbooks")
	if mk := core.MkdirAll(rbDir, 0o700); !mk.OK {
		t.Fatalf("MkdirAll: %s", mk.Error())
	}
	// Hand-craft a legacy file with no version: frontmatter line.
	legacySlug := "legacy-runbook-2026"
	legacyID := "R-99"
	fpath := core.PathJoin(rbDir, legacySlug+".md")
	legacy := []byte("---\nid: " + legacyID + "\ntitle: Legacy runbook\narea: client\ntags: []\nlast_rehearsed: 2026-01-01T00:00:00Z\ncreated_at: 2026-01-01T00:00:00Z\n---\n# Legacy procedure\n")
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
	ur := svc.MarkRehearsed(subject.MarkInput{ID: legacyID})
	if !ur.OK {
		t.Fatalf("MarkRehearsed failed: %s", ur.Error())
	}
	rd2 := paths.ReadVersion(fpath)
	if !rd2.OK {
		t.Fatalf("ReadVersion post-update: %s", rd2.Error())
	}
	if got := rd2.Value.(paths.ReadOutput); got.Version != 1 {
		t.Fatalf("legacy file post-update: expected version 1, got %d", got.Version)
	}
}

// TestAtomicCutover_Runbooks_AuditEmissionRecordBatch_Good —
// MarkRehearsed routes through the primitive's write path
// (EventWriteSucceeded fires) and runbooks/* falls under
// AuditModeBatch per RFC §6.1.
func TestAtomicCutover_Runbooks_AuditEmissionRecordBatch_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := subject.NewService(nil)
	paths.SetAuditSecretProvider(func() []byte {
		return []byte("runbooks-cutover-test-secret-32by")
	})
	t.Cleanup(func() { paths.SetAuditSecretProvider(nil) })

	// Seed first (uses paths primitive too — clear subscribers below
	// AFTER seeding so we observe only the MarkRehearsed event).
	if r := svc.OnStart(); !r.OK {
		t.Fatalf("OnStart: %s", r.Error())
	}

	var saw []paths.LockEvent
	paths.SubscribeLockEvents(func(ev paths.LockEvent) {
		saw = append(saw, ev)
	})
	t.Cleanup(paths.ClearLockEventSubscribersForTest)

	r := svc.MarkRehearsed(subject.MarkInput{ID: "R-01"})
	if !r.OK {
		t.Fatalf("MarkRehearsed failed: %s", r.Error())
	}
	found := false
	for _, ev := range saw {
		if ev.Kind == paths.EventWriteSucceeded {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("MarkRehearsed MUST route through paths.AtomicWriteWithVersion (no EventWriteSucceeded seen)")
	}
	dirR := core.UserHomeDir()
	if !dirR.OK {
		t.Fatalf("UserHomeDir: %s", dirR.Error())
	}
	fpath := core.PathJoin(dirR.Value.(string),
		"Lethean/runbooks/x.md")
	mode := paths.AuditModeForPath(fpath)
	if mode != paths.AuditModeBatch {
		t.Fatalf("expected AuditModeBatch for runbooks path, got %v", mode)
	}
}

// TestAtomicCutover_Runbooks_SeedPath_Good — seedAll's per-runbook
// first-write lands version=1 via paths.AtomicWriteWithVersion (no
// more bare core.WriteFile divergence). All 7 seeds carry frontmatter
// version=1 after the first OnStart. A second OnStart is a no-op
// (countMdFiles > 0 short-circuits) — verifies seed idempotence.
func TestAtomicCutover_Runbooks_SeedPath_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := subject.NewService(nil)
	if r := svc.OnStart(); !r.OK {
		t.Fatalf("first OnStart: %s", r.Error())
	}
	// All 7 seed slugs from seedAll().
	seedSlugs := []string{
		"rotate-runtime-api-keys",
		"recover-from-corrupt-model-directory",
		"rollback-bad-deploy-production",
		"reset-dns-cache-poisoning-incident",
		"drain-a-stuck-postfix-queue",
		"trigger-emergency-model-unload",
		"restore-tray-app-from-corrupt-config",
	}
	for _, slug := range seedSlugs {
		fpath := runbookFilePath(t, slug)
		rd := paths.ReadVersion(fpath)
		if !rd.OK {
			t.Fatalf("ReadVersion(%s): %s", slug, rd.Error())
		}
		if got := rd.Value.(paths.ReadOutput); got.Version != 1 {
			t.Fatalf("seed %s: expected version 1, got %d", slug, got.Version)
		}
	}
	// Second OnStart is a no-op — countMdFiles > 0 short-circuits, so
	// the version stays at 1 (would be 2 if seedAll re-ran each call).
	if r := svc.OnStart(); !r.OK {
		t.Fatalf("second OnStart: %s", r.Error())
	}
	fpath := runbookFilePath(t, "rotate-runtime-api-keys")
	rd := paths.ReadVersion(fpath)
	if !rd.OK {
		t.Fatalf("ReadVersion post-second-OnStart: %s", rd.Error())
	}
	if got := rd.Value.(paths.ReadOutput); got.Version != 1 {
		t.Fatalf("second OnStart should be no-op: expected version 1, got %d", got.Version)
	}
}
