// SPDX-Licence-Identifier: EUPL-1.2

package incidents_test

import (
	"encoding/json"
	"sync"
	"testing"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/account"
	subject "dappco.re/lthn/desktop/pkg/incidents"
	"dappco.re/lthn/desktop/pkg/paths"
	"forge.lthn.ai/Snider/Enchantrix/pkg/crypt/std/pgp"
)

// stubSessionGate is the test double for the consumer-defined
// SessionGate interface (RFC.stage-e-unlockgate v2 §4.2 stub shape).
// Mirrors H#147 documents.stubSessionGate — duplicated per-pkg rather
// than shared, per Pushback 1 (consumer-defines) and Cerberus #28
// confirm that each writer pkg owns its own gate interface.
//
// Stage E.D.B.2 widens the surface (Mantis #1487 RFC v2 §5.1) to
// include PublicKeyFor + PrivateKeyFor. Default-constructed stubs (no
// pub set) still satisfy the SessionGate interface; existing tests
// that don't wire the encryption path get the legacy plaintext .md
// fallback automatically (atrestWriterFor returns false on missing
// accountKeyProvider satisfaction).
//
// Usage example:
//
//	svc := incidents.NewService(nil)
//	svc.SetSessionGate(&stubSessionGate{ids: []string{"acct-test"}})
type stubSessionGate struct {
	ids  []string
	pub  []byte
	priv []byte
}

func (s *stubSessionGate) UnlockedAccountIDs() []string { return s.ids }

func (s *stubSessionGate) PublicKeyFor(_ string) ([]byte, bool) {
	if len(s.pub) == 0 {
		return nil, false
	}
	cp := make([]byte, len(s.pub))
	copy(cp, s.pub)
	return cp, true
}

// PrivateKeyFor returns a real *account.PrivateKeyHandle so the
// substrate's read path engages the canonical zeroise-on-return
// semantics (Mantis #1589 / Cerberus #18). account.NewPrivateKey
// HandleForTest is the test-only factory exported by pkg/account
// expressly for sibling-package consumers like incidents.
func (s *stubSessionGate) PrivateKeyFor(_ string) (*account.PrivateKeyHandle, bool) {
	if len(s.priv) == 0 {
		return nil, false
	}
	cp := make([]byte, len(s.priv))
	copy(cp, s.priv)
	return account.NewPrivateKeyHandleForTest(cp), true
}

// testKeyPair is the package-shared keypair generated once per
// process so multiple tests in this file (and atrest_test.go) reuse
// the same costly Ed25519 setup without re-running GenerateKeyPair
// per test.
var (
	testKeyOnce sync.Once
	testKeyPub  []byte
	testKeyPriv []byte
)

// genTestKeyPair lazily generates a real PGP keypair for the tests.
// Idempotent — subsequent calls reuse the cached keys.
func genTestKeyPair(t *testing.T) (pub, priv []byte) {
	t.Helper()
	testKeyOnce.Do(func() {
		svc := pgp.NewService()
		p, k, err := svc.GenerateKeyPair("Test", "test@lthn.local", "test")
		if err != nil {
			t.Fatalf("generate test key pair: %v", err)
		}
		testKeyPub = p
		testKeyPriv = k
	})
	return testKeyPub, testKeyPriv
}

// newServiceUnlocked returns an incidents Service with a SessionGate
// pre-wired to report a single unlocked account AND a real PGP
// keypair. With both wired, the at-rest write path engages by
// default — Create produces `<id>.lthn` envelopes, Get decrypts via
// PrivateKeyFor.Use.
//
// Tests that need the legacy plaintext .md fallback call
// newServiceUnlockedLegacy() — a gate with ids but no keys triggers
// the accountKeyProvider type-assertion miss and drops the writer
// through to the plaintext path.
//
// Usage example:
//
//	svc := newServiceUnlocked(t)
//	r := svc.Create(subject.CreateInput{Title: "...", Sev: "P3"})
func newServiceUnlocked(t *testing.T) *subject.Service {
	t.Helper()
	pub, priv := genTestKeyPair(t)
	svc := subject.NewService(nil)
	svc.SetSessionGate(&stubSessionGate{
		ids:  []string{"acct-test"},
		pub:  pub,
		priv: priv,
	})
	return svc
}

// newServiceUnlockedLegacy returns an incidents Service with a
// SessionGate that satisfies ONLY the UnlockedAccountIDs surface —
// no keys. The at-rest path skips (accountKeyProvider type-assertion
// fails) and writes route to the legacy plaintext .md fallback.
// Reserved for tests that pin the pre-cutover behaviour for the
// legacy-file upgrade regression cover (LegacyFile_Ugly).
//
// Usage example:
//
//	svc := newServiceUnlockedLegacy()
//	r := svc.Create(subject.CreateInput{Title: "...", Sev: "P3"})
//	// → file at ~/Lethean/incidents/{YYYY}/{MM}/{id}.md
func newServiceUnlockedLegacy() *subject.Service {
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

// ---- Cascade W3 + Stage E.D.B.2 cutover tests ------------------------

// incidentFilePathLthn resolves the encrypted .lthn on-disk path for
// an incident created "now" — the at-rest writer lands under
// ~/Lethean/incidents/<YYYY>/<MM>/<ID>.lthn (Stage E.D.B.2 cutover).
func incidentFilePathLthn(t *testing.T, id string) string {
	t.Helper()
	now := core.Now().UTC()
	dirR := core.UserHomeDir()
	if !dirR.OK {
		t.Fatalf("UserHomeDir: %s", dirR.Error())
	}
	yr := core.TimeFormat(now, "2006")
	mo := core.TimeFormat(now, "01")
	return core.PathJoin(dirR.Value.(string),
		"Lethean/incidents", yr, mo, id+".lthn")
}

// incidentFilePathMd resolves the legacy plaintext .md on-disk path
// for an incident created "now" — used only by the LegacyFile_Ugly
// migration test which seeds a pre-cutover plaintext file by hand.
func incidentFilePathMd(t *testing.T, id string) string {
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

// TestAtomicCutover_Incidents_Create_Good — Stage E.D.B.2 retrofit:
// Create now writes `<id>.lthn` (PGP-encrypted envelope) instead of
// the plaintext `<id>.md`. The version=1 stamp now lives inside the
// encrypted body's YAML frontmatter — assertion shifts from
// paths.ReadVersion(<id>.md) to file-existence on .lthn plus Magic
// prefix check. Round-trip Version assertion happens via Get in
// atrest_test.go.
func TestAtomicCutover_Incidents_Create_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newServiceUnlocked(t)
	r := svc.Create(subject.CreateInput{
		Title: "hub · elevated p99", Sev: "P3", Svc: "hub", Who: "Mei",
	})
	if !r.OK {
		t.Fatalf("Create failed: %s", r.Error())
	}
	entry := r.Value.(subject.IncidentEntry)
	lthnPath := incidentFilePathLthn(t, entry.ID)
	if stat := core.Stat(lthnPath); !stat.OK {
		t.Fatalf("expected encrypted file %s after Create, stat failed: %s", lthnPath, stat.Error())
	}
	// .md MUST NOT exist (first-write produces .lthn only — legacy
	// remove is a no-op when there's nothing to remove).
	mdPath := incidentFilePathMd(t, entry.ID)
	if stat := core.Stat(mdPath); stat.OK {
		t.Fatalf("plaintext %s MUST NOT exist after Create on at-rest path", mdPath)
	}
}

// TestAtomicCutover_Incidents_Update_Good — a sequential UpdateState
// round-trips through encrypted Read + Write. Confirms the ciphertext
// is re-encrypted (LTHN magic prefix preserved) so the IfMatch
// optimistic-lock + plaintext-checksum surface stays exercised.
func TestAtomicCutover_Incidents_Update_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newServiceUnlocked(t)
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
	lthnPath := incidentFilePathLthn(t, id)
	if stat := core.Stat(lthnPath); !stat.OK {
		t.Fatalf("expected encrypted file %s after UpdateState, stat failed: %s", lthnPath, stat.Error())
	}
	raw := core.ReadFile(lthnPath)
	if !raw.OK {
		t.Fatalf("read encrypted file: %s", raw.Error())
	}
	if b, _ := raw.Value.([]byte); len(b) < 4 || string(b[:4]) != "LTHN" {
		t.Fatalf("encrypted file missing LTHN magic prefix")
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
	// Stage E.D.B.2 retrofit: write entry points now require an
	// unlocked SessionGate (assertUnlocked fires before writeRecord),
	// so the legacy plaintext .md path is unreachable through the
	// wails surface. The encrypted .lthn path uses ciphertext-hash
	// IfMatch instead of the version-frontmatter IfVersion gate; its
	// conflict surface is `recordfile.atrest.atomic_write_failed`
	// (substrate-level invariant covered by
	// TestAtRest_PriorHashGate_Ugly in pkg/recordfile). The
	// `incidents.update.conflict` ConflictEnvelope wire shape no longer
	// surfaces from the incidents service post-cutover; conflict-
	// dispatch.ts keeps the legacy mapper for other surfaces that have
	// not yet retrofitted to at-rest (runbooks/marketing — wave 3).
	t.Skip("at-rest substrate replaces version-frontmatter conflict envelope; see TestAtRest_PriorHashGate_Ugly in pkg/recordfile substrate tests")
	t.Setenv("HOME", t.TempDir())
	svc := newServiceUnlocked(t)
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

// TestAtomicCutover_Incidents_LegacyFile_Ugly — Stage E.D.B.2 lazy
// migration: a pre-cutover plaintext `<id>.md` on disk gets read on
// loadOne fallthrough, then re-written as encrypted `<id>.lthn` on
// the next UpdateState. The legacy plaintext file MUST be removed
// after the encrypted write succeeds (RFC.stage-e-encrypt-at-rest v2
// §3.1).
func TestAtomicCutover_Incidents_LegacyFile_Ugly(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newServiceUnlocked(t)
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
	// Hand-craft a legacy plaintext file (no version: frontmatter line,
	// pre-cutover shape).
	legacyID := yr + "-" + mo + "-INC-099"
	mdPath := core.PathJoin(monthDir, legacyID+".md")
	lthnPath := core.PathJoin(monthDir, legacyID+".lthn")
	legacy := []byte("---\nid: " + legacyID + "\ntitle: legacy · pre-cutover\nsev: P3\nstate: investigating\nsvc: hub\nwho: legacy\ncomments: 0\ncreated_at: 2026-01-01T00:00:00Z\nresolved_at: 0001-01-01T00:00:00Z\ndur_minutes: 0\n---\n")
	if w := core.WriteFile(mdPath, legacy, 0o600); !w.OK {
		t.Fatalf("WriteFile: %s", w.Error())
	}

	// UpdateState reads .md (fallthrough), re-writes .lthn (encrypted),
	// removes the legacy .md.
	ur := svc.UpdateState(subject.UpdateStateInput{ID: legacyID, State: "post-mortem"})
	if !ur.OK {
		t.Fatalf("UpdateState failed: %s", ur.Error())
	}
	if stat := core.Stat(lthnPath); !stat.OK {
		t.Fatalf("expected encrypted %s after lazy migration, stat failed: %s", lthnPath, stat.Error())
	}
	if stat := core.Stat(mdPath); stat.OK {
		t.Fatalf("legacy %s MUST be removed after lazy migration", mdPath)
	}
}

// TestAtomicCutover_Incidents_AuditEmissionRecordBatch_Good — Create
// routes through the primitive's write path (EventWriteSucceeded
// fires) and incidents/* falls under AuditModeBatch per RFC §6.1.
func TestAtomicCutover_Incidents_AuditEmissionRecordBatch_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newServiceUnlocked(t)
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
// live-read gate reports a non-empty unlocked-account slice. Wired
// via newServiceUnlocked which seeds the gate with real pub/priv
// keys so the at-rest path engages end-to-end (Stage E.D.B.2).
func TestIncidents_UnlockedGate_AllowsCreate(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newServiceUnlocked(t)

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
	svc := newServiceUnlocked(t)

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
	svc := newServiceUnlocked(t)

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
// briefly (with keys) to seed an incident, then locks before the
// transition.
func TestIncidents_LockedGate_FailsUpdateState(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newServiceUnlocked(t) // seed phase: keys + ids wired
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
	// Seed phase: unlocked (with keys). Create + transition to
	// post-mortem so AddPostmortem's state-precondition is satisfied —
	// the locked-gate assertion must be the first rejection, not the
	// state guard.
	svc := newServiceUnlocked(t)
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
	svc := newServiceUnlocked(t) // gate pre-wired with unlocked stub

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
