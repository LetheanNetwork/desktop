// SPDX-Licence-Identifier: EUPL-1.2

// coverage_gap_test.go — closes the statement-coverage gaps left by
// the existing Cascade W3 / Stage E.D.B.2 suites: List's filter/limit/
// scan-error branches, Get/UpdateState/AddPostmortem's session-locked
// forwarding, Create's validation + write-failure branches, the
// legacy plaintext write path (a genuinely gate-less-of-keys
// SessionGate, distinct from stubSessionGate which always structurally
// satisfies accountKeyProvider), and the pure header/enum helpers
// exposed via export_test.go.

package incidents_test

import (
	"testing"

	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/incidents"
)

// minimalSessionGate satisfies SessionGate (UnlockedAccountIDs only)
// and deliberately does NOT define PublicKeyFor/PrivateKeyFor — unlike
// stubSessionGate (which always structurally satisfies
// accountKeyProvider, even with empty key slices), this type lets
// tests reach the TRUE legacy-plaintext fallback in
// atrestWriterFor/writeRecord, and the "gate wired+unlocked but
// cannot decrypt" branch inside loadOne.
type minimalSessionGate struct{ ids []string }

func (m *minimalSessionGate) UnlockedAccountIDs() []string { return m.ids }

// ---- Register --------------------------------------------------------

func TestRegister_Good(t *testing.T) {
	r := subject.Register(core.New())
	if !r.OK {
		t.Fatalf("Register: %s", r.Error())
	}
	if _, ok := r.Value.(*subject.Service); !ok {
		t.Fatalf("Register value type = %T, want *subject.Service", r.Value)
	}
}

// ---- pure header/enum helpers -----------------------------------------

func TestIncidentStateEnumOut_Good(t *testing.T) {
	cases := map[string]string{
		"resolved":      "closed",
		"investigating": "open",
		"post-mortem":   "open",
	}
	for in, want := range cases {
		if got := subject.IncidentStateEnumOutExported(in); got != want {
			t.Errorf("IncidentStateEnumOutExported(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIncidentStateEnumOut_Bad_UnknownProjectsOpen(t *testing.T) {
	if got := subject.IncidentStateEnumOutExported("bogus"); got != "open" {
		t.Errorf("IncidentStateEnumOutExported(bogus) = %q, want open", got)
	}
}

func TestIncidentStateEnumIn_Good(t *testing.T) {
	cases := map[string]string{
		"closed": "resolved",
		"open":   "investigating",
	}
	for in, want := range cases {
		if got := subject.IncidentStateEnumInExported(in); got != want {
			t.Errorf("IncidentStateEnumInExported(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIncidentStateEnumIn_Bad_UnknownReturnsEmpty(t *testing.T) {
	if got := subject.IncidentStateEnumInExported("bogus"); got != "" {
		t.Errorf("IncidentStateEnumInExported(bogus) = %q, want empty", got)
	}
}

func TestStringFromRaw_Good(t *testing.T) {
	m := map[string]any{"status": "open"}
	if got := subject.StringFromRawExported(m, "status"); got != "open" {
		t.Errorf("StringFromRawExported = %q, want open", got)
	}
}

func TestStringFromRaw_Bad(t *testing.T) {
	if got := subject.StringFromRawExported(nil, "status"); got != "" {
		t.Errorf("StringFromRawExported(nil map) = %q, want empty", got)
	}
	if got := subject.StringFromRawExported(map[string]any{}, "status"); got != "" {
		t.Errorf("StringFromRawExported(missing key) = %q, want empty", got)
	}
}

func TestStringFromRaw_Ugly_NonStringValue(t *testing.T) {
	m := map[string]any{"status": 42}
	if got := subject.StringFromRawExported(m, "status"); got != "" {
		t.Errorf("StringFromRawExported(non-string) = %q, want empty", got)
	}
}

// ---- countMonthFiles ---------------------------------------------------

func TestCountMonthFiles_Good(t *testing.T) {
	dir := t.TempDir()
	if w := core.WriteFile(core.PathJoin(dir, "a.md"), []byte("x"), 0o600); !w.OK {
		t.Fatalf("seed a.md: %s", w.Error())
	}
	if w := core.WriteFile(core.PathJoin(dir, "b.lthn"), []byte("x"), 0o600); !w.OK {
		t.Fatalf("seed b.lthn: %s", w.Error())
	}
	if w := core.WriteFile(core.PathJoin(dir, "c.txt"), []byte("x"), 0o600); !w.OK {
		t.Fatalf("seed c.txt: %s", w.Error())
	}
	if r := core.MkdirAll(core.PathJoin(dir, "subdir"), 0o700); !r.OK {
		t.Fatalf("seed subdir: %s", r.Error())
	}
	if n := subject.CountMonthFilesExported(dir); n != 2 {
		t.Fatalf("CountMonthFilesExported = %d, want 2 (.md + .lthn only, subdir/.txt excluded)", n)
	}
}

func TestCountMonthFiles_Bad_MissingDir(t *testing.T) {
	if n := subject.CountMonthFilesExported(core.PathJoin(t.TempDir(), "nonexistent")); n != 0 {
		t.Fatalf("CountMonthFilesExported(missing dir) = %d, want 0", n)
	}
}

// ---- List ---------------------------------------------------------------

// TestList_Bad_ScanFailure forces incidentsDir()'s MkdirAll to fail by
// pointing HOME at a plain file (not a directory) — list90 surfaces
// the mkdir error and List wraps it as "scan failed".
func TestList_Bad_ScanFailure(t *testing.T) {
	dir := t.TempDir()
	fakeHome := core.PathJoin(dir, "not-a-dir")
	if w := core.WriteFile(fakeHome, []byte("x"), 0o600); !w.OK {
		t.Fatalf("seed fake HOME file: %s", w.Error())
	}
	t.Setenv("HOME", fakeHome)

	svc := subject.NewService(nil)
	r := svc.List(subject.ListInput{})
	if r.OK {
		t.Fatal("expected List to fail when HOME is not a directory")
	}
	if !core.Contains(r.Error(), "incidents.List") {
		t.Fatalf("expected incidents.List scoped error, got %q", r.Error())
	}
}

// TestList_FiltersAndLimit_Good seeds two incidents with distinct
// severities, then drives State-mismatch, Severity-mismatch, and
// Limit-truncation through a single List call each.
func TestList_FiltersAndLimit_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newServiceUnlocked(t)

	first := svc.Create(subject.CreateInput{Title: "first", Sev: "P1", Svc: "hub", Who: "Mei"})
	if !first.OK {
		t.Fatalf("Create(first): %s", first.Error())
	}
	second := svc.Create(subject.CreateInput{Title: "second", Sev: "P4", Svc: "forge", Who: "Tobi"})
	if !second.OK {
		t.Fatalf("Create(second): %s", second.Error())
	}

	// State filter mismatch — both incidents are "investigating"; ask
	// for "resolved" so the continue branch fires for every entry.
	byState := svc.List(subject.ListInput{State: "resolved"})
	if !byState.OK {
		t.Fatalf("List(state filter): %s", byState.Error())
	}
	if out := byState.Value.(subject.ListOutput); len(out.Incidents) != 0 {
		t.Fatalf("List(state=resolved) = %d incidents, want 0", len(out.Incidents))
	}

	// Severity filter mismatch.
	bySev := svc.List(subject.ListInput{Severity: "P2"})
	if !bySev.OK {
		t.Fatalf("List(severity filter): %s", bySev.Error())
	}
	if out := bySev.Value.(subject.ListOutput); len(out.Incidents) != 0 {
		t.Fatalf("List(sev=P2) = %d incidents, want 0", len(out.Incidents))
	}

	// Limit truncation — two incidents exist, ask for at most one.
	limited := svc.List(subject.ListInput{Limit: 1})
	if !limited.OK {
		t.Fatalf("List(limit=1): %s", limited.Error())
	}
	out := limited.Value.(subject.ListOutput)
	if len(out.Incidents) != 1 {
		t.Fatalf("List(limit=1) = %d incidents, want 1", len(out.Incidents))
	}
	if out.Total != 2 {
		t.Fatalf("List(limit=1).Total = %d, want 2 (unfiltered count)", out.Total)
	}
}

// ---- Get: session-locked forwarding -------------------------------------

// TestGet_Bad_SessionLockedForwarded creates a real encrypted incident
// via a fully keyed+unlocked gate, then reads it back through a
// second Service pointed at the same HOME with NO gate wired at all —
// loadOne finds the .lthn, atrestWriterFor fails closed, and Get must
// forward the typed incidents.session.locked error verbatim rather
// than folding it into a generic not-found.
func TestGet_Bad_SessionLockedForwarded(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	writer := newServiceUnlocked(t)
	cr := writer.Create(subject.CreateInput{Title: "locked-read probe", Sev: "P3", Svc: "hub", Who: "Mei"})
	if !cr.OK {
		t.Fatalf("Create: %s", cr.Error())
	}
	id := cr.Value.(subject.IncidentEntry).ID

	reader := subject.NewService(nil) // deliberately no SetSessionGate
	r := reader.Get(subject.GetInput{ID: id})
	if r.OK {
		t.Fatal("expected Get to fail when no gate is wired against an encrypted record")
	}
	if !core.Contains(r.Error(), "incidents.session.locked") {
		t.Fatalf("expected incidents.session.locked forwarded verbatim, got %q", r.Error())
	}
}

// ---- Create --------------------------------------------------------------

func TestCreate_Bad_TitleRequired(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newServiceUnlocked(t)
	r := svc.Create(subject.CreateInput{Sev: "P3"})
	if r.OK {
		t.Fatal("expected Create to reject an empty title")
	}
	if !core.Contains(r.Error(), "title is required") {
		t.Fatalf("expected title-required error, got %q", r.Error())
	}
}

// TestCreate_Bad_YearMonthDirFailure blocks the year-level directory
// with a plain file so yearMonthDir's own MkdirAll (not
// incidentsDir's) fails.
func TestCreate_Bad_YearMonthDirFailure(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newServiceUnlocked(t)

	now := core.Now().UTC()
	yr := core.TimeFormat(now, "2006")
	incidentsRoot := core.PathJoin(home, "Lethean", "incidents")
	if r := core.MkdirAll(incidentsRoot, 0o700); !r.OK {
		t.Fatalf("seed incidents root: %s", r.Error())
	}
	yearPath := core.PathJoin(incidentsRoot, yr)
	if w := core.WriteFile(yearPath, []byte("blocked"), 0o600); !w.OK {
		t.Fatalf("seed blocking file at %s: %s", yearPath, w.Error())
	}

	r := svc.Create(subject.CreateInput{Title: "blocked-dir", Sev: "P3"})
	if r.OK {
		t.Fatal("expected Create to fail when the year directory is blocked by a file")
	}
}

// TestCreate_Bad_WriteRecordFailure uses newServiceUnlockedLegacy — a
// gate that reports an unlocked account (assertUnlocked passes) but
// carries no PGP keys. atrestWriterFor still structurally satisfies
// accountKeyProvider (stubSessionGate defines both methods), so the
// at-rest path engages and fails downstream when PublicKeyFor cannot
// resolve a key — exercising Create's writeRecord-failure branch AND
// writeRecordAtRest's own !wr.OK forward.
func TestCreate_Bad_WriteRecordFailure(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newServiceUnlockedLegacy()
	r := svc.Create(subject.CreateInput{Title: "keyless-write", Sev: "P3"})
	if r.OK {
		t.Fatal("expected Create to fail when the gate cannot resolve a public key for encryption")
	}
}

// ---- UpdateState -----------------------------------------------------

func TestUpdateState_Bad_InvalidID(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newServiceUnlocked(t)
	r := svc.UpdateState(subject.UpdateStateInput{ID: "../evil", State: "resolved"})
	if r.OK {
		t.Fatal("expected UpdateState to reject a traversal ID with an unlocked gate")
	}
}

func TestUpdateState_Bad_InvalidState(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newServiceUnlocked(t)
	cr := svc.Create(subject.CreateInput{Title: "state-guard", Sev: "P3"})
	if !cr.OK {
		t.Fatalf("Create: %s", cr.Error())
	}
	id := cr.Value.(subject.IncidentEntry).ID
	r := svc.UpdateState(subject.UpdateStateInput{ID: id, State: "bogus-state"})
	if r.OK {
		t.Fatal("expected UpdateState to reject an unrecognised state")
	}
	if !core.Contains(r.Error(), "invalid state") {
		t.Fatalf("expected invalid-state error, got %q", r.Error())
	}
}

func TestUpdateState_Bad_NotFound(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newServiceUnlocked(t)
	r := svc.UpdateState(subject.UpdateStateInput{ID: "2026-01-INC-999", State: "resolved"})
	if r.OK {
		t.Fatal("expected UpdateState to fail for a well-formed but nonexistent ID")
	}
	if !core.Contains(r.Error(), "not found") {
		t.Fatalf("expected not-found error, got %q", r.Error())
	}
}

// TestUpdateState_Bad_SessionLockedForwarded seeds a real encrypted
// incident via a fully keyed gate, then swaps the SAME Service onto a
// minimalSessionGate that reports an unlocked account (assertUnlocked
// passes) but cannot satisfy accountKeyProvider — loadOne's own
// atrestWriterFor check fails and UpdateState must forward the typed
// session-locked error rather than a generic not-found.
func TestUpdateState_Bad_SessionLockedForwarded(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newServiceUnlocked(t)
	cr := svc.Create(subject.CreateInput{Title: "swap-to-minimal", Sev: "P3"})
	if !cr.OK {
		t.Fatalf("Create: %s", cr.Error())
	}
	id := cr.Value.(subject.IncidentEntry).ID

	svc.SetSessionGate(&minimalSessionGate{ids: []string{"acct-minimal"}})
	r := svc.UpdateState(subject.UpdateStateInput{ID: id, State: "resolved"})
	if r.OK {
		t.Fatal("expected UpdateState to fail when the gate cannot decrypt the existing record")
	}
	if !core.Contains(r.Error(), "incidents.session.locked") {
		t.Fatalf("expected incidents.session.locked forwarded verbatim, got %q", r.Error())
	}
}

// ---- AddPostmortem -----------------------------------------------------

func TestAddPostmortem_Bad_InvestigatingState(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newServiceUnlocked(t)
	cr := svc.Create(subject.CreateInput{Title: "still-investigating", Sev: "P3"})
	if !cr.OK {
		t.Fatalf("Create: %s", cr.Error())
	}
	id := cr.Value.(subject.IncidentEntry).ID
	r := svc.AddPostmortem(subject.PostmortemInput{ID: id, Body: "too early"})
	if r.OK {
		t.Fatal("expected AddPostmortem to reject a still-investigating incident")
	}
	if !core.Contains(r.Error(), "post-mortem requires state") {
		t.Fatalf("expected state-precondition error, got %q", r.Error())
	}
}

func TestAddPostmortem_Bad_NotFound(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newServiceUnlocked(t)
	r := svc.AddPostmortem(subject.PostmortemInput{ID: "2026-01-INC-998", Body: "x"})
	if r.OK {
		t.Fatal("expected AddPostmortem to fail for a well-formed but nonexistent ID")
	}
	if !core.Contains(r.Error(), "not found") {
		t.Fatalf("expected not-found error, got %q", r.Error())
	}
}

func TestAddPostmortem_Bad_SessionLockedForwarded(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newServiceUnlocked(t)
	cr := svc.Create(subject.CreateInput{Title: "swap-to-minimal-pm", Sev: "P3"})
	if !cr.OK {
		t.Fatalf("Create: %s", cr.Error())
	}
	id := cr.Value.(subject.IncidentEntry).ID
	if ur := svc.UpdateState(subject.UpdateStateInput{ID: id, State: "post-mortem"}); !ur.OK {
		t.Fatalf("UpdateState (seed to post-mortem): %s", ur.Error())
	}

	svc.SetSessionGate(&minimalSessionGate{ids: []string{"acct-minimal"}})
	r := svc.AddPostmortem(subject.PostmortemInput{ID: id, Body: "locked-out body"})
	if r.OK {
		t.Fatal("expected AddPostmortem to fail when the gate cannot decrypt the existing record")
	}
	if !core.Contains(r.Error(), "incidents.session.locked") {
		t.Fatalf("expected incidents.session.locked forwarded verbatim, got %q", r.Error())
	}
}

// ---- legacy plaintext write path (true accountKeyProvider miss) --------

// TestLegacyPlaintextPath_Good drives Create/UpdateState/AddPostmortem
// through a minimalSessionGate — the TRUE legacy fallback (distinct
// from stubSessionGate, which always structurally satisfies
// accountKeyProvider even with empty keys). Pins the .md-on-disk
// contract and exercises writeRecord's legacy branch end-to-end.
func TestLegacyPlaintextPath_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := subject.NewService(nil)
	svc.SetSessionGate(&minimalSessionGate{ids: []string{"acct-legacy"}})

	cr := svc.Create(subject.CreateInput{Title: "legacy plaintext", Sev: "P3", Svc: "hub", Who: "Mei"})
	if !cr.OK {
		t.Fatalf("Create (legacy path): %s", cr.Error())
	}
	entry := cr.Value.(subject.IncidentEntry)
	mdPath := incidentFilePathMd(t, entry.ID)
	if stat := core.Stat(mdPath); !stat.OK {
		t.Fatalf("expected legacy plaintext %s after Create, stat failed: %s", mdPath, stat.Error())
	}

	ur := svc.UpdateState(subject.UpdateStateInput{ID: entry.ID, State: "resolved"})
	if !ur.OK {
		t.Fatalf("UpdateState (legacy path): %s", ur.Error())
	}

	pr := svc.AddPostmortem(subject.PostmortemInput{ID: entry.ID, Body: "root cause: legacy path"})
	if !pr.OK {
		t.Fatalf("AddPostmortem (legacy path): %s", pr.Error())
	}
}

// TestWriteRecord_LegacyPath_VersionConflict_Ugly forces the
// version-frontmatter optimistic-lock conflict on the legacy plaintext
// write path deterministically — no goroutine race needed. A record
// is created (on-disk version=1), then WriteRecordExported is called
// with a mismatched non-zero ifVersion, which AtomicWriteWithVersion
// rejects as stale.
func TestWriteRecord_LegacyPath_VersionConflict_Ugly(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := subject.NewService(nil)
	svc.SetSessionGate(&minimalSessionGate{ids: []string{"acct-legacy"}})
	cr := svc.Create(subject.CreateInput{Title: "conflict probe", Sev: "P3"})
	if !cr.OK {
		t.Fatalf("Create: %s", cr.Error())
	}
	id := cr.Value.(subject.IncidentEntry).ID

	rec, dirPath, err := subject.LoadOneExported(id)
	if err != nil {
		t.Fatalf("LoadOneExported: %s", err.Error())
	}
	if rec.Version != 1 {
		t.Fatalf("rec.Version = %d, want 1 on first write", rec.Version)
	}

	wr := subject.WriteRecordExported(rec, dirPath, 99)
	if wr.OK {
		t.Fatal("expected WriteRecordExported to reject a stale non-zero ifVersion")
	}
	if !core.Contains(wr.Error(), "incidents.update.conflict") {
		t.Fatalf("expected incidents.update.conflict, got %q", wr.Error())
	}
}
