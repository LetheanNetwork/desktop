// SPDX-Licence-Identifier: EUPL-1.2

// atrest_test.go — Stage E.D.B.1 sales/deals at-rest cover (Mantis
// #1487 PR-3 wave 1). Anchors:
//
//   - RFC.stage-e-encrypt-at-rest v2 §2.4 per-field MUST table:
//     customer/amount/notes stay BODY-only; stage is bucketed enum;
//     close.target is YYYY-MM bucket only.
//   - §4.1 reads-while-locked: header-only List works; full-body Get
//     refused.
//   - §3.1 lazy migration: writes always emit .lthn; .md removed on
//     success; reads accept BOTH formats.
//
// Substrate-level invariants (Trix shape, header MAC, single-unlock,
// schema enforcement) are exercised by pkg/recordfile/atrest_test.go
// — this file pins consumer-side compositions only.

package deals_test

import (
	"testing"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/sales/deals"
)

// --- Per-field MUST rules (RFC §2.4) --------------------------------

// TestDeals_AtRestSchema_StageEnumOnly_Bad — feeding a free-form
// (non-enum) stage value MUST be rejected at encode-time with the
// substrate's schema.bucketed_field_violation code. The deals service
// maps internal stage tokens to the canonical enum via dealStageEnum
// Out; an UNMAPPED token falls back to "on_hold" (a valid enum) so the
// substrate accepts it but the visible failure mode here is the deals
// service's own isValidStage check (which rejects "fictional-stage"
// BEFORE the substrate ever sees it).
//
// We assert via Create + UpdateStage that an invalid stage token
// either fails at deals.* or at recordfile.atrest.schema.* — both are
// valid rejection paths; the contract is "free-form values do not
// survive the encode pipeline".
func TestDeals_AtRestSchema_StageEnumOnly_Bad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newTestSvc(t)

	// Seed a record on a legal stage, then UpdateStage with garbage.
	cr := svc.Create(deals.CreateInput{
		Customer: "Heritage Law", AmountPence: 24000, Stage: "engage",
	})
	if !cr.OK {
		t.Fatalf("seed Create: %s", cr.Error())
	}
	id := cr.Value.(deals.Deal).ID

	ur := svc.UpdateStage(deals.UpdateStageInput{ID: id, Stage: "fictional-stage"})
	if ur.OK {
		t.Fatal("UpdateStage with free-form stage MUST be rejected")
	}
	// Either deals.UpdateStage's own "unknown stage:" check fires first
	// (preferred) OR the substrate's schema validator rejects later —
	// both are correct.
	errStr := ur.Error()
	if !core.Contains(errStr, "unknown stage") && !core.Contains(errStr, "bucketed_field_violation") {
		t.Fatalf("expected unknown-stage or bucketed_field_violation rejection, got %q", errStr)
	}
}

// TestDeals_AtRestSchema_CloseTargetMonthOnly_Bad — when Create
// supplies a day-granularity CloseTarget, the substrate MUST NOT
// emit the value into the header (it falls into the YAML body only).
// We assert by reading the on-disk envelope and confirming the
// header JSON does NOT contain a YYYY-MM-DD-shaped value.
func TestDeals_AtRestSchema_CloseTargetMonthOnly_Bad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newTestSvc(t)

	r := svc.Create(deals.CreateInput{
		Customer: "Heritage Law", AmountPence: 24000, Stage: "engage",
		// Day-granularity: must NOT reach the header.
		CloseTarget: "2026-05-17",
	})
	if !r.OK {
		t.Fatalf("Create: %s", r.Error())
	}
	id := r.Value.(deals.Deal).ID

	lthnPath := core.PathJoin(home, "Lethean", "sales", "deals", id+".lthn")
	raw := core.ReadFile(lthnPath)
	if !raw.OK {
		t.Fatalf("read encrypted file: %s", raw.Error())
	}
	body, _ := raw.Value.([]byte)
	// The day-granularity string MUST NOT appear anywhere in the on-
	// disk envelope's header bytes. The header is plaintext JSON so a
	// substring search is sufficient — if "2026-05-17" leaks it'll be
	// here.
	if core.Contains(string(body[:trixHeaderEnd(body)]), "2026-05-17") {
		t.Fatal("day-granularity CloseTarget leaked into encrypted-file header")
	}
}

// TestDeals_AtRestSchema_CloseTargetMonth_Good — when Create supplies
// a strict YYYY-MM CloseTarget, the substrate MUST accept it AND
// emit it into the header (proving the bucket projection works on
// the happy path).
func TestDeals_AtRestSchema_CloseTargetMonth_Good(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newTestSvc(t)

	r := svc.Create(deals.CreateInput{
		Customer: "Heritage Law", AmountPence: 24000, Stage: "engage",
		CloseTarget: "2026-05",
	})
	if !r.OK {
		t.Fatalf("Create with YYYY-MM CloseTarget: %s", r.Error())
	}
	id := r.Value.(deals.Deal).ID

	lthnPath := core.PathJoin(home, "Lethean", "sales", "deals", id+".lthn")
	raw := core.ReadFile(lthnPath)
	if !raw.OK {
		t.Fatalf("read encrypted file: %s", raw.Error())
	}
	body, _ := raw.Value.([]byte)
	header := string(body[:trixHeaderEnd(body)])
	if !core.Contains(header, "2026-05") {
		t.Fatal("YYYY-MM CloseTarget MUST appear in encrypted-file header")
	}
}

// TestDeals_AtRestSchema_NameInBodyNotHeader_Good — Customer
// (PII) MUST NEVER appear in the plaintext header. Body decrypt
// round-trips the value back to the in-memory DealRecord.
func TestDeals_AtRestSchema_NameInBodyNotHeader_Good(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newTestSvc(t)

	r := svc.Create(deals.CreateInput{
		Customer: "Heritage Law LLP", AmountPence: 24000, Stage: "engage",
	})
	if !r.OK {
		t.Fatalf("Create: %s", r.Error())
	}
	id := r.Value.(deals.Deal).ID

	lthnPath := core.PathJoin(home, "Lethean", "sales", "deals", id+".lthn")
	raw := core.ReadFile(lthnPath)
	if !raw.OK {
		t.Fatalf("read encrypted file: %s", raw.Error())
	}
	body, _ := raw.Value.([]byte)

	// Header (plaintext JSON) MUST NOT contain the customer name.
	header := string(body[:trixHeaderEnd(body)])
	if core.Contains(header, "Heritage Law LLP") {
		t.Fatal("Customer PII leaked into encrypted-file header — must be body-only")
	}

	// Amount MUST NOT appear either (BODY-only per §2.4).
	if core.Contains(header, "24000") {
		t.Fatal("AmountPence leaked into encrypted-file header — must be body-only")
	}

	// And the encrypted payload bytes (after the header) MUST NOT
	// contain the customer name in plaintext either — that would
	// indicate PGP encryption failed silently.
	if core.Contains(string(body[trixHeaderEnd(body):]), "Heritage Law LLP") {
		t.Fatal("Customer name appeared in ciphertext payload — encryption skipped?")
	}

	// Round-trip via Get to prove the body retains the value.
	g := svc.Get(deals.GetInput{ID: id})
	if !g.OK {
		t.Fatalf("Get after Create: %s", g.Error())
	}
	d := g.Value.(deals.Deal)
	if d.Customer != "Heritage Law LLP" {
		t.Fatalf("Customer round-trip lost: got %q", d.Customer)
	}
}

// --- Reads-while-locked (RFC §4.1 + §4.2) ---------------------------

// TestDeals_LockedSession_ListWorks_Good — when the SessionGate
// reports zero unlocked accounts AFTER a record was written while
// unlocked, List MUST still return the encrypted record (header-only
// projection). The PublicKeyFor call does NOT require unlock so MAC
// verification stays open.
func TestDeals_LockedSession_ListWorks_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newTestSvc(t)
	pub, priv := genTestKeyPair(t)

	// Seed: write while unlocked.
	cr := svc.Create(deals.CreateInput{
		Customer: "Heritage Law", AmountPence: 24000, Stage: "engage",
	})
	if !cr.OK {
		t.Fatalf("seed Create: %s", cr.Error())
	}

	// Flip to locked while keeping PublicKeyFor wired (so header MAC
	// verify can still resolve a pub key without unlock — mirrors
	// account.Service.PublicKeyFor's no-unlock contract).
	svc.SetSessionGate(&stubSessionGate{ids: []string{}, pub: pub, priv: priv})

	r := svc.List(deals.ListInput{})
	if !r.OK {
		t.Fatalf("List while locked MUST succeed (header-only path), got: %s", r.Error())
	}
	out := r.Value.(deals.ListOutput)
	if len(out.Deals) != 1 {
		t.Fatalf("expected 1 list entry while locked, got %d", len(out.Deals))
	}
	// Header-only projection: Customer empty, Stage populated.
	if out.Deals[0].Customer != "" {
		t.Fatalf("header-only list entry MUST NOT carry Customer, got %q", out.Deals[0].Customer)
	}
	if out.Deals[0].Stage == "" {
		t.Fatal("header-only list entry MUST carry Stage label from header")
	}
}

// TestDeals_LockedSession_GetRefused_Bad — Get on an encrypted record
// while the session is locked MUST refuse. The substrate's Read calls
// SingleUnlockedAccount() and the adapter returns
// deals.no_unlocked_account; the wails layer maps that to
// deals.session.locked for the frontend.
func TestDeals_LockedSession_GetRefused_Bad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newTestSvc(t)
	pub, priv := genTestKeyPair(t)

	// Seed: write while unlocked.
	cr := svc.Create(deals.CreateInput{
		Customer: "Heritage Law", AmountPence: 24000, Stage: "engage",
	})
	if !cr.OK {
		t.Fatalf("seed Create: %s", cr.Error())
	}
	id := cr.Value.(deals.Deal).ID

	// Lock the session.
	svc.SetSessionGate(&stubSessionGate{ids: []string{}, pub: pub, priv: priv})

	g := svc.Get(deals.GetInput{ID: id})
	if g.OK {
		t.Fatal("Get on encrypted record while locked MUST be refused")
	}
	errStr := g.Error()
	if !core.Contains(errStr, "session.locked") && !core.Contains(errStr, "multi_account_ambiguous") {
		t.Fatalf("expected session.locked or multi_account_ambiguous on locked Get, got %q", errStr)
	}
}

// TestDeals_UnlockedSession_GetReturnsBody_Good — Get round-trips
// the full body (Customer, Notes, etc.) when the session is unlocked
// and the unlocked account matches the record's header account.id.
func TestDeals_UnlockedSession_GetReturnsBody_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newTestSvc(t)

	cr := svc.Create(deals.CreateInput{
		Customer: "Heritage Law LLP", AmountPence: 24000, Stage: "engage",
		Owner: "Snider", Stakeholders: []string{"Ada Penley · CTO"},
	})
	if !cr.OK {
		t.Fatalf("seed Create: %s", cr.Error())
	}
	id := cr.Value.(deals.Deal).ID

	g := svc.Get(deals.GetInput{ID: id})
	if !g.OK {
		t.Fatalf("Get on unlocked record MUST succeed: %s", g.Error())
	}
	d := g.Value.(deals.Deal)
	if d.Customer != "Heritage Law LLP" {
		t.Fatalf("Customer round-trip lost: got %q", d.Customer)
	}
	if d.Stage != "Engaging" {
		t.Fatalf("Stage round-trip lost: got %q", d.Stage)
	}
}

// --- Lazy migration round-trips (RFC §3.1) --------------------------

// TestDeals_AtRest_ReadAcceptsLegacyMd_Good — a pre-existing .md
// plaintext stays readable via Get fallthrough until the next write
// triggers re-encrypt-as-.lthn (see TestAtomicCutover_Deals_LegacyFile
// _Ugly for the write side).
func TestDeals_AtRest_ReadAcceptsLegacyMd_Good(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newTestSvc(t)

	dealsDir := core.PathJoin(home, "Lethean", "sales", "deals")
	if mk := core.MkdirAll(dealsDir, 0o700); !mk.OK {
		t.Fatalf("MkdirAll: %s", mk.Error())
	}
	legacyID := "202605-DEAL-100"
	mdPath := core.PathJoin(dealsDir, legacyID+".md")
	legacy := []byte("---\nid: " + legacyID + "\ncustomer: LegacyCo\nstage: engage\namount_pence: 1500\nprobability_pct: 25\nclose_target: \"\"\nowner: Snider\ncreated_at: 2026-05-01T00:00:00Z\nupdated_at: 2026-05-01T00:00:00Z\n---\n")
	if w := core.WriteFile(mdPath, legacy, 0o600); !w.OK {
		t.Fatalf("WriteFile legacy: %s", w.Error())
	}

	g := svc.Get(deals.GetInput{ID: legacyID})
	if !g.OK {
		t.Fatalf("Get on legacy .md MUST succeed via fallthrough: %s", g.Error())
	}
	d := g.Value.(deals.Deal)
	if d.Customer != "LegacyCo" {
		t.Fatalf("legacy Customer lost: got %q", d.Customer)
	}
}

// TestDeals_AtRest_ReadPrefersLthnOverMd_Good — when both extensions
// exist (e.g. mid-migration crash window) loadAll prefers the .lthn
// entry and skips the .md duplicate.
func TestDeals_AtRest_ReadPrefersLthnOverMd_Good(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newTestSvc(t)

	cr := svc.Create(deals.CreateInput{
		Customer: "EncryptedCo", AmountPence: 1, Stage: "engage",
	})
	if !cr.OK {
		t.Fatalf("seed Create: %s", cr.Error())
	}
	id := cr.Value.(deals.Deal).ID

	dealsDir := core.PathJoin(home, "Lethean", "sales", "deals")
	mdShadow := core.PathJoin(dealsDir, id+".md")
	shadow := []byte("---\nid: " + id + "\ncustomer: PlaintextCo\nstage: qual\namount_pence: 999\nprobability_pct: 0\nclose_target: \"\"\nowner: x\ncreated_at: 2026-05-01T00:00:00Z\nupdated_at: 2026-05-01T00:00:00Z\n---\n")
	if w := core.WriteFile(mdShadow, shadow, 0o600); !w.OK {
		t.Fatalf("seed shadow .md: %s", w.Error())
	}

	r := svc.List(deals.ListInput{})
	if !r.OK {
		t.Fatalf("List: %s", r.Error())
	}
	out := r.Value.(deals.ListOutput)
	if len(out.Deals) != 1 {
		t.Fatalf("expected exactly one entry (lthn preferred), got %d", len(out.Deals))
	}
	// The entry is the encrypted one — header-only projection means
	// Customer is empty and Stage is the header bucket label.
	if out.Deals[0].Customer == "PlaintextCo" {
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
