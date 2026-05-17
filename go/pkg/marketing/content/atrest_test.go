// SPDX-Licence-Identifier: EUPL-1.2

// atrest_test.go — Stage E.D.B.3 marketing/content at-rest cover
// (Mantis #1487 wave 3 LAST consumer — 4 of 4). Anchors:
//
//   - RFC.stage-e-encrypt-at-rest v2 §2.4 per-field MUST table for
//     marketing/content:
//       * title  → BODY (REJECT in header)
//       * due.at → HEADER (MONTH-only, YYYY-MM) — projected from Due
//         field iff string parses strict YYYY-MM (Q5-friendly).
//     All other fields (who / when / col / body) are BODY-only by
//     default-rule. SECURITY-NOTE: brief offered `status (enum if
//     any)` escape valve; RFC §2.4 does NOT name a status header key
//     so col stays BODY-only (per [[feedback_brief_vs_rfc_deferral_
//     check]] + [[feedback_beta_ticket_dont_gate]]).
//   - §4.1 reads-while-locked: header-only List works; full-body Get
//     refused.
//   - §3.1 lazy migration: writes always emit .lthn; .md removed on
//     success; reads accept BOTH formats.
//
// Substrate-level invariants (Trix shape, header MAC, single-unlock,
// schema enforcement) are exercised by pkg/recordfile/atrest_test.go
// — this file pins consumer-side compositions only.

package content_test

import (
	"sync"
	"testing"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/account"
	subject "dappco.re/lthn/desktop/pkg/marketing/content"
	"forge.lthn.ai/Snider/Enchantrix/pkg/crypt/std/pgp"
)

// stubKeySessionGate is the at-rest-capable test double — satisfies
// both the narrow SessionGate (UnlockedAccountIDs) AND the wider
// accountKeyProvider runtime-assertion (PublicKeyFor + PrivateKeyFor)
// the at-rest writer engages. Distinct from stubSessionGate in
// content_test.go so the existing minimal-gate fail-safe + legacy
// .md tests stay untouched.
type stubKeySessionGate struct {
	ids  []string
	pub  []byte
	priv []byte
}

func (s *stubKeySessionGate) UnlockedAccountIDs() []string { return s.ids }

func (s *stubKeySessionGate) PublicKeyFor(_ string) ([]byte, bool) {
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
// expressly for sibling-package consumers like marketing/content.
func (s *stubKeySessionGate) PrivateKeyFor(_ string) (*account.PrivateKeyHandle, bool) {
	if len(s.priv) == 0 {
		return nil, false
	}
	cp := make([]byte, len(s.priv))
	copy(cp, s.priv)
	return account.NewPrivateKeyHandleForTest(cp), true
}

// atrestKey* are the package-shared keypair generated once per process
// so multiple at-rest tests reuse the same costly setup.
var (
	atrestKeyOnce sync.Once
	atrestKeyPub  []byte
	atrestKeyPriv []byte
)

// genAtRestKeyPair lazily generates a real PGP keypair for the at-
// rest tests. Idempotent — subsequent calls reuse the cached keys.
//
//	pub, priv := genAtRestKeyPair(t)
func genAtRestKeyPair(t *testing.T) (pub, priv []byte) {
	t.Helper()
	atrestKeyOnce.Do(func() {
		svc := pgp.NewService()
		p, k, err := svc.GenerateKeyPair("Test", "test@lthn.local", "test")
		if err != nil {
			t.Fatalf("generate test key pair: %v", err)
		}
		atrestKeyPub = p
		atrestKeyPriv = k
	})
	return atrestKeyPub, atrestKeyPriv
}

// newAtRestTestSvc constructs a content.Service pre-wired with the
// WIDE stubKeySessionGate (ids + pub + priv). The at-rest writer
// engages by default — Create produces `<id>.lthn` envelopes, Get
// decrypts via PrivateKeyFor.
//
// Tests that need the locked-session path call SetSessionGate
// explicitly with ids=[]. Tests that need the legacy plaintext .md
// fallback use the NARROW stubSessionGate from content_test.go.
//
// Usage example:
//
//	svc := newAtRestTestSvc(t)
//	r := svc.Create(subject.CreateInput{T: "..."})
func newAtRestTestSvc(t *testing.T) *subject.Service {
	t.Helper()
	pub, priv := genAtRestKeyPair(t)
	svc := subject.NewService(nil)
	svc.SetSessionGate(&stubKeySessionGate{
		ids:  []string{"acct-test"},
		pub:  pub,
		priv: priv,
	})
	return svc
}

// trixHeaderEnd returns the byte offset at which the Trix header ends
// (i.e. where the encrypted payload begins) per the on-disk shape
// `[Magic(4)][Version(1)][HeaderLen(4)][HeaderJSON][Payload]`. Used to
// slice header-vs-payload regions for plaintext-leak assertions.
//
//	end := trixHeaderEnd(raw)
//	header := string(raw[:end])
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

// contentLthnPath returns the absolute path to the encrypted content
// file for the named id under the test HOME.
//
//	p := contentLthnPath(t, home, "v02-release-notes-1747440000")
func contentLthnPath(t *testing.T, home, id string) string {
	t.Helper()
	return core.PathJoin(home, "Lethean", "marketing", "content", id+".lthn")
}

// --- Per-field MUST rules (RFC §2.4) --------------------------------

// TestContent_AtRestSchema_TitleInBodyNotHeader_Bad — Title (editorial
// intent, PII-adjacent) MUST NEVER appear in the plaintext header.
// Body decrypt round-trips the value back to the in-memory ContentItem.
func TestContent_AtRestSchema_TitleInBodyNotHeader_Bad(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newAtRestTestSvc(t)

	r := svc.Create(subject.CreateInput{
		T:   "Sentinel Bank · Q3 enterprise launch announcement",
		Due: "today",
	})
	if !r.OK {
		t.Fatalf("Create: %s", r.Error())
	}
	c := r.Value.(subject.ContentItem)
	lthnPath := contentLthnPath(t, home, c.ID)
	raw := core.ReadFile(lthnPath)
	if !raw.OK {
		t.Fatalf("read encrypted file: %s", raw.Error())
	}
	body, _ := raw.Value.([]byte)
	header := string(body[:trixHeaderEnd(body)])

	if core.Contains(header, "Sentinel Bank") {
		t.Fatal("Title leaked into encrypted-file header — must be body-only (RFC §2.4)")
	}
	if core.Contains(string(body[trixHeaderEnd(body):]), "Sentinel Bank") {
		t.Fatal("Title appeared in ciphertext payload — encryption skipped?")
	}

	// Round-trip via Get to prove the body retains the value.
	g := svc.Get(c.ID)
	if !g.OK {
		t.Fatalf("Get after Create: %s", g.Error())
	}
	got := g.Value.(subject.ContentItem)
	if got.T != "Sentinel Bank · Q3 enterprise launch announcement" {
		t.Fatalf("Title round-trip lost: got %q", got.T)
	}
}

// TestContent_AtRestSchema_DueMonthOnlyInHeader_Good — when Due
// parses strict YYYY-MM the header carries `due.at` per RFC §2.4.
// Pin the canonical projection.
func TestContent_AtRestSchema_DueMonthOnlyInHeader_Good(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newAtRestTestSvc(t)

	r := svc.Create(subject.CreateInput{
		T:   "header-due-month-probe",
		Due: "2026-05",
	})
	if !r.OK {
		t.Fatalf("Create: %s", r.Error())
	}
	c := r.Value.(subject.ContentItem)
	lthnPath := contentLthnPath(t, home, c.ID)
	raw := core.ReadFile(lthnPath)
	if !raw.OK {
		t.Fatalf("read encrypted file: %s", raw.Error())
	}
	body, _ := raw.Value.([]byte)
	header := string(body[:trixHeaderEnd(body)])

	if !core.Contains(header, `"due.at":"2026-05"`) {
		t.Fatalf("expected header due.at=\"2026-05\", got header: %s", header)
	}
}

// TestContent_AtRestSchema_DueFreeFormNotInHeader_Bad — when Due is
// free-form ("today", "next Friday") it MUST NOT appear in the header
// (Q5-friendly: HeaderFor only emits when string parses strict
// YYYY-MM; legacy free-form values stay BODY-only).
func TestContent_AtRestSchema_DueFreeFormNotInHeader_Bad(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newAtRestTestSvc(t)

	r := svc.Create(subject.CreateInput{
		T:   "due-freeform-probe",
		Due: "today",
	})
	if !r.OK {
		t.Fatalf("Create: %s", r.Error())
	}
	c := r.Value.(subject.ContentItem)
	lthnPath := contentLthnPath(t, home, c.ID)
	raw := core.ReadFile(lthnPath)
	if !raw.OK {
		t.Fatalf("read encrypted file: %s", raw.Error())
	}
	body, _ := raw.Value.([]byte)
	header := string(body[:trixHeaderEnd(body)])

	if core.Contains(header, "today") {
		t.Fatalf("free-form Due leaked into encrypted-file header (must be body-only when not YYYY-MM): %s", header)
	}
	if core.Contains(header, `"due.at":`) {
		t.Fatalf("due.at header key MUST be absent for free-form Due, got: %s", header)
	}

	// Round-trip body confirms persistence.
	g := svc.Get(c.ID)
	if !g.OK {
		t.Fatalf("Get: %s", g.Error())
	}
	got := g.Value.(subject.ContentItem)
	if got.Due != "today" {
		t.Fatalf("Due round-trip lost: got %q", got.Due)
	}
}

// TestContent_AtRestSchema_ColInBodyNotHeader_Bad — Col carries
// pipeline state (idea|draft|review|ready|live) but RFC §2.4 names no
// header key for it. Default-body rule: col MUST stay in the body.
// Header MUST NOT carry it (no `"col":` key, no `"status":` either).
// SECURITY-NOTE: brief offered `status (enum if any)` escape valve;
// RFC §2.4 does NOT name a status header key for marketing/content,
// so col stays BODY-only per [[feedback_brief_vs_rfc_deferral_check]].
func TestContent_AtRestSchema_ColInBodyNotHeader_Bad(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newAtRestTestSvc(t)

	r := svc.Create(subject.CreateInput{
		T:   "col-leak-probe",
		Col: "review",
	})
	if !r.OK {
		t.Fatalf("Create: %s", r.Error())
	}
	c := r.Value.(subject.ContentItem)
	lthnPath := contentLthnPath(t, home, c.ID)
	raw := core.ReadFile(lthnPath)
	if !raw.OK {
		t.Fatalf("read encrypted file: %s", raw.Error())
	}
	body, _ := raw.Value.([]byte)
	header := string(body[:trixHeaderEnd(body)])

	if core.Contains(header, `"col":`) {
		t.Fatalf("Col leaked into encrypted-file header (no RFC §2.4 ruling — default-body): %s", header)
	}
	if core.Contains(header, `"status":`) {
		t.Fatalf("brief's `status` escape valve must NOT be applied without RFC ruling: %s", header)
	}
	if core.Contains(header, `"review"`) {
		t.Fatalf("Col value leaked into encrypted-file header: %s", header)
	}

	// Round-trip body confirms persistence.
	g := svc.Get(c.ID)
	if !g.OK {
		t.Fatalf("Get: %s", g.Error())
	}
	got := g.Value.(subject.ContentItem)
	if got.Col != "review" {
		t.Fatalf("Col round-trip lost: got %q", got.Col)
	}
}

// TestContent_AtRestSchema_WhoWhenInBodyNotHeader_Bad — Who (author)
// and When (publish timestamp) are PII-adjacent / cadence-revealing.
// RFC §2.4 names no header key for either; default-body rule applies.
func TestContent_AtRestSchema_WhoWhenInBodyNotHeader_Bad(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newAtRestTestSvc(t)

	r := svc.Create(subject.CreateInput{
		T:   "who-when-leak-probe",
		Who: "claire.chambers@example.com",
		Col: "draft",
	})
	if !r.OK {
		t.Fatalf("Create: %s", r.Error())
	}
	c := r.Value.(subject.ContentItem)
	lthnPath := contentLthnPath(t, home, c.ID)
	raw := core.ReadFile(lthnPath)
	if !raw.OK {
		t.Fatalf("read encrypted file: %s", raw.Error())
	}
	body, _ := raw.Value.([]byte)
	header := string(body[:trixHeaderEnd(body)])

	for _, banned := range []string{"claire.chambers", `"who":`, `"when":`} {
		if core.Contains(header, banned) {
			t.Fatalf("PII field %q leaked into encrypted-file header (RFC §2.4 default-body): %s", banned, header)
		}
	}

	// Round-trip body confirms persistence.
	g := svc.Get(c.ID)
	if !g.OK {
		t.Fatalf("Get: %s", g.Error())
	}
	got := g.Value.(subject.ContentItem)
	if got.Who != "claire.chambers@example.com" {
		t.Fatalf("Who round-trip lost: got %q", got.Who)
	}
}

// TestContent_AtRestSchema_BodyInBodyNotHeader_Bad — the content body
// (markdown outline / notes) MUST NEVER appear in the plaintext
// header. Body decrypt round-trips the value.
func TestContent_AtRestSchema_BodyInBodyNotHeader_Bad(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newAtRestTestSvc(t)

	outline := "## Outline\n\nDeath Star reactor pitch deck — confidential."
	r := svc.Create(subject.CreateInput{
		T:    "body-leak-probe",
		Body: outline,
	})
	if !r.OK {
		t.Fatalf("Create: %s", r.Error())
	}
	c := r.Value.(subject.ContentItem)
	lthnPath := contentLthnPath(t, home, c.ID)
	raw := core.ReadFile(lthnPath)
	if !raw.OK {
		t.Fatalf("read encrypted file: %s", raw.Error())
	}
	body, _ := raw.Value.([]byte)
	header := string(body[:trixHeaderEnd(body)])

	if core.Contains(header, "Death Star") {
		t.Fatal("Body text leaked into encrypted-file header — must be body-only")
	}
	if core.Contains(string(body[trixHeaderEnd(body):]), "Death Star") {
		t.Fatal("Body text appeared in ciphertext payload — encryption skipped?")
	}

	g := svc.Get(c.ID)
	if !g.OK {
		t.Fatalf("Get: %s", g.Error())
	}
	got := g.Value.(subject.ContentItem)
	if got.Body != outline {
		t.Fatalf("Body round-trip lost: got %q", got.Body)
	}
}

// --- Reads-while-locked (RFC §4.1 + §4.2) ---------------------------

// TestContent_LockedSession_ListWorks_Good — when the SessionGate
// reports zero unlocked accounts AFTER a record was written while
// unlocked, List MUST still return the encrypted record (header-only
// projection). The PublicKeyFor call does NOT require unlock so MAC
// verification stays open.
//
// Operator sees:
//   - ID from the header
//   - Due from header `due.at` (when YYYY-MM)
//   - T / Who / When / Col / Body EMPTY (BODY-only)
func TestContent_LockedSession_ListWorks_Good(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newAtRestTestSvc(t)
	pub, priv := genAtRestKeyPair(t)

	// Seed: write while unlocked.
	cr := svc.Create(subject.CreateInput{
		T:   "header-only-survives-lock",
		Due: "2026-05",
		Col: "draft",
	})
	if !cr.OK {
		t.Fatalf("seed Create: %s", cr.Error())
	}

	// Flip to locked while keeping PublicKeyFor wired (so header MAC
	// verify can still resolve a pub key without unlock — mirrors
	// account.Service.PublicKeyFor's no-unlock contract).
	svc.SetSessionGate(&stubKeySessionGate{ids: []string{}, pub: pub, priv: priv})

	r := svc.List(subject.ListInput{})
	if !r.OK {
		t.Fatalf("List while locked MUST succeed (header-only path), got: %s", r.Error())
	}
	out := r.Value.(subject.ListOutput)
	// Iterate columns to find our single header-only entry.
	var got subject.ContentItem
	found := 0
	for _, col := range out.Columns {
		for _, item := range col.Items {
			got = item
			found++
		}
	}
	if found != 1 {
		t.Fatalf("expected 1 list entry while locked, got %d", found)
	}
	// Header-only projection: T/Who/When/Col/Body empty, ID + Due
	// (from header `due.at`) present.
	if got.T != "" {
		t.Fatalf("header-only list entry MUST NOT carry T (title), got %q", got.T)
	}
	if got.Who != "" {
		t.Fatalf("header-only list entry MUST NOT carry Who, got %q", got.Who)
	}
	if got.When != "" {
		t.Fatalf("header-only list entry MUST NOT carry When, got %q", got.When)
	}
	// Col is BODY-only per RFC §2.4; header-only reads project into
	// the first canonical column ("idea") as a placeholder so the
	// operator can see the encrypted entry exists. The frontend
	// renders an "encrypted" badge — the underlying Col value here is
	// not a leak (header-MAC verify doesn't decrypt body).
	if got.Col != "idea" {
		t.Fatalf("header-only list entry MUST project into first column as placeholder, got %q", got.Col)
	}
	if got.Body != "" {
		t.Fatalf("header-only list entry MUST NOT carry Body, got %q", got.Body)
	}
	if got.ID == "" {
		t.Fatal("header-only list entry MUST carry ID from header")
	}
	if got.Due != "2026-05" {
		t.Fatalf("header-only list entry MUST carry Due from header `due.at`, got %q", got.Due)
	}
}

// TestContent_LockedSession_GetRefused_Bad — Get on an encrypted
// record while the session is locked MUST refuse. The substrate's Read
// calls SingleUnlockedAccount() and the adapter returns
// content.no_unlocked_account; the wails layer surfaces session.
// locked (or atrest.* codes) verbatim for the frontend.
func TestContent_LockedSession_GetRefused_Bad(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newAtRestTestSvc(t)
	pub, priv := genAtRestKeyPair(t)

	cr := svc.Create(subject.CreateInput{
		T:   "locked-get-target",
		Col: "idea",
	})
	if !cr.OK {
		t.Fatalf("seed Create: %s", cr.Error())
	}
	id := cr.Value.(subject.ContentItem).ID

	// Lock the session.
	svc.SetSessionGate(&stubKeySessionGate{ids: []string{}, pub: pub, priv: priv})

	g := svc.Get(id)
	if g.OK {
		t.Fatal("Get on encrypted record while locked MUST be refused")
	}
	errStr := g.Error()
	if !core.Contains(errStr, "session.locked") &&
		!core.Contains(errStr, "multi_account_ambiguous") &&
		!core.Contains(errStr, "no_unlocked_account") {
		t.Fatalf("expected session.locked / multi_account_ambiguous / no_unlocked_account on locked Get, got %q", errStr)
	}
}

// TestContent_UnlockedSession_GetReturnsBody_Good — Get round-trips
// the full body (T, Who, When, Due, Col, Body) when the session is
// unlocked.
func TestContent_UnlockedSession_GetReturnsBody_Good(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newAtRestTestSvc(t)

	cr := svc.Create(subject.CreateInput{
		T:    "Sentinel Bank · Q3",
		Who:  "claire.chambers@example.com",
		Due:  "2026-05",
		Col:  "draft",
		Body: "## Outline\n\nFull body content.",
	})
	if !cr.OK {
		t.Fatalf("seed Create: %s", cr.Error())
	}
	id := cr.Value.(subject.ContentItem).ID

	g := svc.Get(id)
	if !g.OK {
		t.Fatalf("Get on unlocked record MUST succeed: %s", g.Error())
	}
	d := g.Value.(subject.ContentItem)
	if d.T != "Sentinel Bank · Q3" {
		t.Fatalf("T round-trip lost: got %q", d.T)
	}
	if d.Who != "claire.chambers@example.com" {
		t.Fatalf("Who round-trip lost: got %q", d.Who)
	}
	if d.Due != "2026-05" {
		t.Fatalf("Due round-trip lost: got %q", d.Due)
	}
	if d.Col != "draft" {
		t.Fatalf("Col round-trip lost: got %q", d.Col)
	}
	if d.Body != "## Outline\n\nFull body content." {
		t.Fatalf("Body round-trip lost: got %q", d.Body)
	}
}

// --- Lazy migration round-trips (RFC §3.1) --------------------------

// TestContent_AtRest_ReadAcceptsLegacyMd_Good — a pre-existing .md
// plaintext stays readable via loadOne fallthrough until the next
// write triggers re-encrypt-as-.lthn.
func TestContent_AtRest_ReadAcceptsLegacyMd_Good(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newAtRestTestSvc(t)

	dir := core.PathJoin(home, "Lethean", "marketing", "content")
	if mk := core.MkdirAll(dir, 0o700); !mk.OK {
		t.Fatalf("MkdirAll: %s", mk.Error())
	}
	legacyID := "legacy-content-2026"
	mdPath := core.PathJoin(dir, legacyID+".md")
	legacy := []byte("---\nid: " + legacyID + "\nt: LegacyPost\nwho: \"you\"\nwhen: \"\"\ndue: \"today\"\ncol: draft\n---\nLegacy body.\n")
	if w := core.WriteFile(mdPath, legacy, 0o600); !w.OK {
		t.Fatalf("WriteFile legacy: %s", w.Error())
	}

	g := svc.Get(legacyID)
	if !g.OK {
		t.Fatalf("Get on legacy .md MUST succeed via fallthrough: %s", g.Error())
	}
	got := g.Value.(subject.ContentItem)
	if got.T != "LegacyPost" {
		t.Fatalf("legacy T lost: got %q", got.T)
	}
}

// TestContent_AtRest_WriteRemovesLegacyMd_Good — Advance against a
// legacy .md record writes the .lthn envelope AND removes the legacy
// plaintext file (RFC §3.1 lazy-migration completion).
func TestContent_AtRest_WriteRemovesLegacyMd_Good(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newAtRestTestSvc(t)

	dir := core.PathJoin(home, "Lethean", "marketing", "content")
	if mk := core.MkdirAll(dir, 0o700); !mk.OK {
		t.Fatalf("MkdirAll: %s", mk.Error())
	}
	id := "legacy-cutover"
	mdPath := core.PathJoin(dir, id+".md")
	legacy := []byte("---\nid: " + id + "\nt: Cutover\nwho: \"\"\nwhen: \"\"\ndue: \"\"\ncol: idea\n---\nProcedure.\n")
	if w := core.WriteFile(mdPath, legacy, 0o600); !w.OK {
		t.Fatalf("seed legacy .md: %s", w.Error())
	}

	ur := svc.Advance(id)
	if !ur.OK {
		t.Fatalf("Advance: %s", ur.Error())
	}

	// .lthn MUST exist post-write.
	lthnPath := core.PathJoin(dir, id+".lthn")
	if st := core.Stat(lthnPath); !st.OK {
		t.Fatalf("encrypted .lthn MUST be written, missing at %s", lthnPath)
	}
	// .md MUST be removed (lazy-migration completion).
	if st := core.Stat(mdPath); st.OK {
		t.Fatal("legacy .md MUST be removed after successful encrypted write (RFC §3.1)")
	}
}

// TestContent_AtRest_ReadPrefersLthnOverMd_Good — when both
// extensions exist (mid-migration crash window between AtomicWrite and
// Remove) loadItems prefers the .lthn entry and skips the .md
// duplicate.
func TestContent_AtRest_ReadPrefersLthnOverMd_Good(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newAtRestTestSvc(t)

	cr := svc.Create(subject.CreateInput{
		T:   "encrypted-original",
		Col: "draft",
	})
	if !cr.OK {
		t.Fatalf("seed Create: %s", cr.Error())
	}
	id := cr.Value.(subject.ContentItem).ID

	// Drop a shadow .md alongside the encrypted record.
	dir := core.PathJoin(home, "Lethean", "marketing", "content")
	mdShadow := core.PathJoin(dir, id+".md")
	shadow := []byte("---\nid: " + id + "\nt: PLAINTEXT-SHADOW\nwho: \"\"\nwhen: \"\"\ndue: \"\"\ncol: draft\n---\nShadow.\n")
	if w := core.WriteFile(mdShadow, shadow, 0o600); !w.OK {
		t.Fatalf("seed shadow .md: %s", w.Error())
	}

	r := svc.List(subject.ListInput{})
	if !r.OK {
		t.Fatalf("List: %s", r.Error())
	}
	out := r.Value.(subject.ListOutput)
	// Count entries across all columns.
	total := 0
	for _, col := range out.Columns {
		total += len(col.Items)
	}
	if total != 1 {
		t.Fatalf("expected exactly one entry (.lthn preferred), got %d", total)
	}
	// The single entry must be the encrypted one (header-only
	// projection means T is empty); the shadow .md's PLAINTEXT-SHADOW
	// value must NOT appear anywhere.
	for _, col := range out.Columns {
		for _, item := range col.Items {
			if item.T == "PLAINTEXT-SHADOW" {
				t.Fatal("List returned the shadow .md entry instead of preferring .lthn")
			}
		}
	}
}

// --- File-mode + dir-mode cover for the .lthn write path ------------

// TestContent_AtRest_FileMode0600_Cerberus1487 — the substrate's
// atomic-rename path applies mode 0o600 to the encrypted file. Pin it
// alongside the legacy .md cover in security_test.go.
func TestContent_AtRest_FileMode0600_Cerberus1487(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newAtRestTestSvc(t)

	r := svc.Create(subject.CreateInput{
		T:   "mode-probe",
		Col: "idea",
	})
	if !r.OK {
		t.Fatalf("Create: %s", r.Error())
	}
	c := r.Value.(subject.ContentItem)
	lthnPath := contentLthnPath(t, home, c.ID)
	stat := core.Stat(lthnPath)
	if !stat.OK {
		t.Fatalf("stat(%s): %s", lthnPath, stat.Error())
	}
	info := stat.Value.(core.FsFileInfo)
	mode := info.Mode().Perm()
	if int(mode) != 0o600 {
		t.Fatalf(".lthn file mode = %o, want 0o600", mode)
	}
}
