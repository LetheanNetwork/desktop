// SPDX-Licence-Identifier: EUPL-1.2

// atrest_test.go — Stage E.D.B.2 incidents at-rest cover (Mantis
// #1487 PR-3 wave 2). Anchors:
//
//   - RFC.stage-e-encrypt-at-rest v2 §2.4 per-field MUST table:
//     title / postmortem / svc / who / sev all stay BODY-only;
//     status is bucketed to the IncidentsStatuses enum {open, closed}
//     and is the ONLY consumer-owned header key.
//   - §4.1 reads-while-locked: header-only List works; full-body Get
//     refused.
//   - §3.1 lazy migration: writes always emit .lthn; .md removed on
//     success; reads accept BOTH formats.
//
// Substrate-level invariants (Trix shape, header MAC, single-unlock,
// schema enforcement) are exercised by pkg/recordfile/atrest_test.go
// — this file pins consumer-side compositions only.

package incidents_test

import (
	"testing"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/incidents"
)

// --- Per-field MUST rules (RFC §2.4) --------------------------------

// TestIncidents_AtRestSchema_TitleInBodyNotHeader_Good — Title is PII-
// sensitive (incident summary often names a customer/system) and MUST
// stay BODY-only. The plaintext header bytes MUST NOT contain the
// title literal. Body decrypt round-trips the value back to the
// in-memory IncidentRecord.
func TestIncidents_AtRestSchema_TitleInBodyNotHeader_Good(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newServiceUnlocked(t)

	r := svc.Create(incidents.CreateInput{
		Title: "Sentinel Bank · auth bypass triage",
		Sev:   "P1", Svc: "auth", Who: "Mei",
	})
	if !r.OK {
		t.Fatalf("Create: %s", r.Error())
	}
	entry := r.Value.(incidents.IncidentEntry)
	lthnPath := incidentFilePathLthn(t, entry.ID)
	raw := core.ReadFile(lthnPath)
	if !raw.OK {
		t.Fatalf("read encrypted file: %s", raw.Error())
	}
	body, _ := raw.Value.([]byte)

	header := string(body[:trixHeaderEnd(body)])
	if core.Contains(header, "Sentinel Bank") {
		t.Fatal("Title leaked into encrypted-file header — must be body-only")
	}
	// Svc + Who MUST NOT appear either (BODY-only per §2.4).
	if core.Contains(header, "auth") {
		t.Fatal("Svc leaked into encrypted-file header — must be body-only")
	}
	if core.Contains(header, "Mei") {
		t.Fatal("Who leaked into encrypted-file header — must be body-only")
	}
	// Severity stays body-only too (header has only status enum per
	// RFC §2.4 incidents row).
	if core.Contains(header, "P1") {
		t.Fatal("Sev leaked into encrypted-file header — must be body-only")
	}

	// The encrypted payload bytes (after the header) MUST NOT contain
	// the title in plaintext either — that would indicate PGP encrypt
	// failed silently.
	if core.Contains(string(body[trixHeaderEnd(body):]), "Sentinel Bank") {
		t.Fatal("Title appeared in ciphertext payload — encryption skipped?")
	}

	// Round-trip via Get to prove the body retains the value.
	g := svc.Get(incidents.GetInput{ID: entry.ID})
	if !g.OK {
		t.Fatalf("Get after Create: %s", g.Error())
	}
	d := g.Value.(incidents.IncidentDetail)
	if d.Title != "Sentinel Bank · auth bypass triage" {
		t.Fatalf("Title round-trip lost: got %q", d.Title)
	}
}

// TestIncidents_AtRestSchema_PostMortemInBodyNotHeader_Good — when
// AddPostmortem persists the markdown body, the plaintext header bytes
// MUST NOT contain the postmortem text. Body decrypt round-trips it
// back via Get.
func TestIncidents_AtRestSchema_PostMortemInBodyNotHeader_Good(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newServiceUnlocked(t)

	cr := svc.Create(incidents.CreateInput{
		Title: "outage probe", Sev: "P3", Svc: "hub", Who: "qa",
	})
	if !cr.OK {
		t.Fatalf("Create: %s", cr.Error())
	}
	id := cr.Value.(incidents.IncidentEntry).ID
	if ur := svc.UpdateState(incidents.UpdateStateInput{ID: id, State: "post-mortem"}); !ur.OK {
		t.Fatalf("UpdateState: %s", ur.Error())
	}

	pm := "## Root cause\n\nDeath Star reactor inhibitor regression."
	if pr := svc.AddPostmortem(incidents.PostmortemInput{ID: id, Body: pm}); !pr.OK {
		t.Fatalf("AddPostmortem: %s", pr.Error())
	}

	lthnPath := incidentFilePathLthn(t, id)
	raw := core.ReadFile(lthnPath)
	if !raw.OK {
		t.Fatalf("read encrypted file: %s", raw.Error())
	}
	body, _ := raw.Value.([]byte)

	header := string(body[:trixHeaderEnd(body)])
	if core.Contains(header, "Death Star") {
		t.Fatal("PostMortem leaked into encrypted-file header — must be body-only")
	}
	if core.Contains(string(body[trixHeaderEnd(body):]), "Death Star") {
		t.Fatal("PostMortem appeared in ciphertext payload — encryption skipped?")
	}

	g := svc.Get(incidents.GetInput{ID: id})
	if !g.OK {
		t.Fatalf("Get: %s", g.Error())
	}
	d := g.Value.(incidents.IncidentDetail)
	if d.PostMortem != pm {
		t.Fatalf("PostMortem round-trip lost: got %q", d.PostMortem)
	}
}

// TestIncidents_AtRestSchema_StatusEnumOpen_Good — a newly-created
// (investigating) incident MUST project as status="open" in the
// header per the IncidentsStatuses enum bucket.
func TestIncidents_AtRestSchema_StatusEnumOpen_Good(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newServiceUnlocked(t)

	r := svc.Create(incidents.CreateInput{
		Title: "header-status-probe", Sev: "P3", Svc: "hub", Who: "qa",
	})
	if !r.OK {
		t.Fatalf("Create: %s", r.Error())
	}
	id := r.Value.(incidents.IncidentEntry).ID

	lthnPath := incidentFilePathLthn(t, id)
	raw := core.ReadFile(lthnPath)
	if !raw.OK {
		t.Fatalf("read encrypted file: %s", raw.Error())
	}
	body, _ := raw.Value.([]byte)
	header := string(body[:trixHeaderEnd(body)])
	if !core.Contains(header, `"status":"open"`) {
		t.Fatalf("expected header status=\"open\" for investigating record, got header: %s", header)
	}
}

// TestIncidents_AtRestSchema_StatusEnumClosed_Good — transitioning to
// resolved MUST project as status="closed" in the header. Confirms
// the incidentStateEnumOut mapper feeds the canonical closed bucket.
func TestIncidents_AtRestSchema_StatusEnumClosed_Good(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newServiceUnlocked(t)

	cr := svc.Create(incidents.CreateInput{
		Title: "to-resolve", Sev: "P3", Svc: "hub", Who: "qa",
	})
	if !cr.OK {
		t.Fatalf("Create: %s", cr.Error())
	}
	id := cr.Value.(incidents.IncidentEntry).ID
	if ur := svc.UpdateState(incidents.UpdateStateInput{ID: id, State: "resolved"}); !ur.OK {
		t.Fatalf("UpdateState resolved: %s", ur.Error())
	}

	lthnPath := incidentFilePathLthn(t, id)
	raw := core.ReadFile(lthnPath)
	if !raw.OK {
		t.Fatalf("read encrypted file: %s", raw.Error())
	}
	body, _ := raw.Value.([]byte)
	header := string(body[:trixHeaderEnd(body)])
	if !core.Contains(header, `"status":"closed"`) {
		t.Fatalf("expected header status=\"closed\" for resolved record, got header: %s", header)
	}
}

// --- Reads-while-locked (RFC §4.1 + §4.2) ---------------------------

// TestIncidents_LockedSession_ListWorks_Good — when the SessionGate
// reports zero unlocked accounts AFTER a record was written while
// unlocked, List MUST still return the encrypted record (header-only
// projection). The PublicKeyFor call does NOT require unlock so MAC
// verification stays open.
func TestIncidents_LockedSession_ListWorks_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newServiceUnlocked(t)
	pub, priv := genTestKeyPair(t)

	// Seed: write while unlocked.
	cr := svc.Create(incidents.CreateInput{
		Title: "header-only-survives-lock",
		Sev:   "P3", Svc: "hub", Who: "Mei",
	})
	if !cr.OK {
		t.Fatalf("seed Create: %s", cr.Error())
	}

	// Flip to locked while keeping PublicKeyFor wired (so header MAC
	// verify can still resolve a pub key without unlock — mirrors
	// account.Service.PublicKeyFor's no-unlock contract).
	svc.SetSessionGate(&stubSessionGate{ids: []string{}, pub: pub, priv: priv})

	r := svc.List(incidents.ListInput{})
	if !r.OK {
		t.Fatalf("List while locked MUST succeed (header-only path), got: %s", r.Error())
	}
	out := r.Value.(incidents.ListOutput)
	if len(out.Incidents) != 1 {
		t.Fatalf("expected 1 list entry while locked, got %d", len(out.Incidents))
	}
	// Header-only projection: Title/Svc/Who/Sev empty, ID + State present.
	got := out.Incidents[0]
	if got.Title != "" {
		t.Fatalf("header-only list entry MUST NOT carry Title, got %q", got.Title)
	}
	if got.Svc != "" {
		t.Fatalf("header-only list entry MUST NOT carry Svc, got %q", got.Svc)
	}
	if got.Who != "" {
		t.Fatalf("header-only list entry MUST NOT carry Who, got %q", got.Who)
	}
	if got.ID == "" {
		t.Fatal("header-only list entry MUST carry ID from header")
	}
	// Status bucket projects investigating → "open" → incidentStateEnumIn
	// → "investigating".
	if got.State != "investigating" {
		t.Fatalf("header-only list entry State should map back to investigating from open enum, got %q", got.State)
	}
}

// TestIncidents_LockedSession_GetRefused_Bad — Get on an encrypted
// record while the session is locked MUST refuse. The substrate's Read
// calls SingleUnlockedAccount() and the adapter returns
// incidents.no_unlocked_account; the wails layer maps that to
// incidents.session.locked for the frontend.
func TestIncidents_LockedSession_GetRefused_Bad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newServiceUnlocked(t)
	pub, priv := genTestKeyPair(t)

	cr := svc.Create(incidents.CreateInput{
		Title: "locked-get-target", Sev: "P3", Svc: "hub", Who: "Mei",
	})
	if !cr.OK {
		t.Fatalf("seed Create: %s", cr.Error())
	}
	id := cr.Value.(incidents.IncidentEntry).ID

	// Lock the session.
	svc.SetSessionGate(&stubSessionGate{ids: []string{}, pub: pub, priv: priv})

	g := svc.Get(incidents.GetInput{ID: id})
	if g.OK {
		t.Fatal("Get on encrypted record while locked MUST be refused")
	}
	errStr := g.Error()
	if !core.Contains(errStr, "session.locked") && !core.Contains(errStr, "no_unlocked_account") {
		t.Fatalf("expected session.locked or no_unlocked_account on locked Get, got %q", errStr)
	}
}

// TestIncidents_UnlockedSession_GetReturnsBody_Good — Get round-trips
// the full body (Title, PostMortem, Svc, Who) when the session is
// unlocked and the unlocked account matches the record's header
// account.id.
func TestIncidents_UnlockedSession_GetReturnsBody_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newServiceUnlocked(t)

	cr := svc.Create(incidents.CreateInput{
		Title: "Sentinel Bank · auth bypass triage",
		Sev:   "P1", Svc: "auth", Who: "Mei",
	})
	if !cr.OK {
		t.Fatalf("seed Create: %s", cr.Error())
	}
	id := cr.Value.(incidents.IncidentEntry).ID

	g := svc.Get(incidents.GetInput{ID: id})
	if !g.OK {
		t.Fatalf("Get on unlocked record MUST succeed: %s", g.Error())
	}
	d := g.Value.(incidents.IncidentDetail)
	if d.Title != "Sentinel Bank · auth bypass triage" {
		t.Fatalf("Title round-trip lost: got %q", d.Title)
	}
	if d.Sev != "P1" {
		t.Fatalf("Sev round-trip lost: got %q", d.Sev)
	}
	if d.Svc != "auth" {
		t.Fatalf("Svc round-trip lost: got %q", d.Svc)
	}
	if d.Who != "Mei" {
		t.Fatalf("Who round-trip lost: got %q", d.Who)
	}
}

// --- Lazy migration round-trips (RFC §3.1) --------------------------

// TestIncidents_AtRest_ReadAcceptsLegacyMd_Good — a pre-existing .md
// plaintext stays readable via Get fallthrough until the next write
// triggers re-encrypt-as-.lthn (see
// TestAtomicCutover_Incidents_LegacyFile_Ugly for the write side).
func TestIncidents_AtRest_ReadAcceptsLegacyMd_Good(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newServiceUnlocked(t)

	// Seed a legacy .md under the current YYYY/MM (loadOne walks the
	// year/month tree so any existing month dir works).
	now := core.Now().UTC()
	yr := core.TimeFormat(now, "2006")
	mo := core.TimeFormat(now, "01")
	monthDir := core.PathJoin(home, "Lethean", "incidents", yr, mo)
	if mk := core.MkdirAll(monthDir, 0o700); !mk.OK {
		t.Fatalf("MkdirAll: %s", mk.Error())
	}
	legacyID := yr + "-" + mo + "-INC-500"
	mdPath := core.PathJoin(monthDir, legacyID+".md")
	legacy := []byte("---\nid: " + legacyID + "\ntitle: legacy-readable\nsev: P3\nstate: investigating\nsvc: hub\nwho: legacy\ncomments: 0\ncreated_at: 2026-05-01T00:00:00Z\nresolved_at: 0001-01-01T00:00:00Z\ndur_minutes: 0\n---\n## Body\n\nlegacy body.\n")
	if w := core.WriteFile(mdPath, legacy, 0o600); !w.OK {
		t.Fatalf("WriteFile legacy: %s", w.Error())
	}

	g := svc.Get(incidents.GetInput{ID: legacyID})
	if !g.OK {
		t.Fatalf("Get on legacy .md MUST succeed via fallthrough: %s", g.Error())
	}
	d := g.Value.(incidents.IncidentDetail)
	if d.Title != "legacy-readable" {
		t.Fatalf("legacy Title lost: got %q", d.Title)
	}
}

// TestIncidents_AtRest_ReadPrefersLthnOverMd_Good — when both
// extensions exist (e.g. mid-migration crash window) list90 prefers
// the .lthn entry and skips the .md duplicate.
func TestIncidents_AtRest_ReadPrefersLthnOverMd_Good(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newServiceUnlocked(t)

	cr := svc.Create(incidents.CreateInput{
		Title: "encrypted-original", Sev: "P3", Svc: "hub", Who: "qa",
	})
	if !cr.OK {
		t.Fatalf("seed Create: %s", cr.Error())
	}
	id := cr.Value.(incidents.IncidentEntry).ID

	// Drop a shadow .md alongside the encrypted record (simulates a
	// mid-migration crash between AtomicWrite and Remove).
	now := core.Now().UTC()
	yr := core.TimeFormat(now, "2006")
	mo := core.TimeFormat(now, "01")
	monthDir := core.PathJoin(home, "Lethean", "incidents", yr, mo)
	mdShadow := core.PathJoin(monthDir, id+".md")
	shadow := []byte("---\nid: " + id + "\ntitle: PLAINTEXT-SHADOW\nsev: P3\nstate: investigating\nsvc: hub\nwho: legacy\ncomments: 0\ncreated_at: 2026-05-01T00:00:00Z\nresolved_at: 0001-01-01T00:00:00Z\ndur_minutes: 0\n---\n")
	if w := core.WriteFile(mdShadow, shadow, 0o600); !w.OK {
		t.Fatalf("seed shadow .md: %s", w.Error())
	}

	r := svc.List(incidents.ListInput{})
	if !r.OK {
		t.Fatalf("List: %s", r.Error())
	}
	out := r.Value.(incidents.ListOutput)
	if len(out.Incidents) != 1 {
		t.Fatalf("expected exactly one entry (.lthn preferred), got %d", len(out.Incidents))
	}
	// The entry is the encrypted one — header-only projection means
	// Title is empty and the State is mapped from header status.
	if out.Incidents[0].Title == "PLAINTEXT-SHADOW" {
		t.Fatal("List returned the shadow .md entry instead of preferring .lthn")
	}
}

// --- Helpers --------------------------------------------------------

// trixHeaderEnd returns the byte offset at which the Trix header ends
// (i.e. where the encrypted payload begins) per the on-disk shape
// `[Magic(4)][Version(1)][HeaderLen(4)][HeaderJSON][Payload]`. Used
// to slice header-vs-payload regions for plaintext-leak assertions.
func trixHeaderEnd(raw []byte) int {
	if len(raw) < 9 {
		return 0
	}
	hdrLen := int(uint32(raw[5])<<24 | uint32(raw[6])<<16 | uint32(raw[7])<<8 | uint32(raw[8]))
	end := 9 + hdrLen
	if end > len(raw) {
		return len(raw)
	}
	return end
}
