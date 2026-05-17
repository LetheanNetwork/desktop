// SPDX-Licence-Identifier: EUPL-1.2

// atrest_test.go — Stage E.D.B.3 marketing/social at-rest cover
// (Mantis #1487 wave 3 consumer #3). Anchors:
//
//   - RFC.stage-e-encrypt-at-rest v2 §2.4 per-field MUST table for
//     marketing/social:
//       * text         → BODY (REJECT in header)
//       * platform     → HEADER (CONFIRM; projected from Ch joined)
//       * scheduled.at → HEADER (MONTH-only) — SocialPost model has
//         When as free-form text today; the schema omits this header
//         key (SECURITY-NOTE in service.go atrestWriterFor doc).
//     All other fields (state / attach / when) are BODY-only by
//     default-rule.
//   - §4.1 reads-while-locked: header-only List works; full-body Get
//     refused.
//   - §3.1 lazy migration: writes always emit .lthn; .md removed on
//     success; reads accept BOTH formats.
//
// Substrate-level invariants (Trix shape, header MAC, single-unlock,
// schema enforcement) are exercised by pkg/recordfile/atrest_test.go
// — this file pins consumer-side compositions only.

package social_test

import (
	"sync"
	"testing"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/account"
	subject "dappco.re/lthn/desktop/pkg/marketing/social"
	"forge.lthn.ai/Snider/Enchantrix/pkg/crypt/std/pgp"
)

// stubKeySessionGate is the at-rest-capable test double — satisfies
// both the narrow SessionGate (UnlockedAccountIDs) AND the wider
// accountKeyProvider runtime-assertion (PublicKeyFor + PrivateKeyFor)
// the at-rest writer engages. Distinct from stubSessionGate in
// social_test.go so the existing minimal-gate fail-safe + legacy
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
// expressly for sibling-package consumers like marketing/social.
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

// newAtRestTestSvc constructs a social.Service pre-wired with the
// WIDE stubKeySessionGate (ids + pub + priv). The at-rest writer
// engages by default — Create produces `<id>.lthn` envelopes, Get
// decrypts via PrivateKeyFor.
//
// Tests that need the locked-session path call SetSessionGate
// explicitly with ids=[]. Tests that need the legacy plaintext .md
// fallback use the NARROW stubSessionGate from social_test.go.
//
// Usage example:
//
//	svc := newAtRestTestSvc(t)
//	r := svc.Create(subject.CreateInput{Ch: []string{"mastodon"}, ...})
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

// postLthnPath returns the absolute path to the encrypted social post
// file for the named id under the test HOME.
//
//	p := postLthnPath(t, home, "post-1747440000")
func postLthnPath(t *testing.T, home, id string) string {
	t.Helper()
	return core.PathJoin(home, "Lethean", "marketing", "social", id+".lthn")
}

// --- Per-field MUST rules (RFC §2.4) --------------------------------

// TestSocial_AtRestSchema_TextInBodyNotHeader_Bad — Text (post content,
// brand voice + PII-adjacent) MUST NEVER appear in the plaintext header.
// Body decrypt round-trips the value back to the in-memory SocialPost.
func TestSocial_AtRestSchema_TextInBodyNotHeader_Bad(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newAtRestTestSvc(t)

	r := svc.Create(subject.CreateInput{
		Ch:   []string{"mastodon"},
		When: "today · 16:00",
		Text: "Sentinel Bank confidential pitch — Q3 enterprise launch.",
	})
	if !r.OK {
		t.Fatalf("Create: %s", r.Error())
	}
	p := r.Value.(subject.SocialPost)
	lthnPath := postLthnPath(t, home, p.ID)
	raw := core.ReadFile(lthnPath)
	if !raw.OK {
		t.Fatalf("read encrypted file: %s", raw.Error())
	}
	body, _ := raw.Value.([]byte)
	header := string(body[:trixHeaderEnd(body)])

	if core.Contains(header, "Sentinel Bank") {
		t.Fatal("Text leaked into encrypted-file header — must be body-only (RFC §2.4)")
	}
	if core.Contains(string(body[trixHeaderEnd(body):]), "Sentinel Bank") {
		t.Fatal("Text appeared in ciphertext payload — encryption skipped?")
	}

	// Round-trip via Get to prove the body retains the value.
	g := svc.Get(p.ID)
	if !g.OK {
		t.Fatalf("Get after Create: %s", g.Error())
	}
	got := g.Value.(subject.SocialPost)
	if got.Text != "Sentinel Bank confidential pitch — Q3 enterprise launch." {
		t.Fatalf("Text round-trip lost: got %q", got.Text)
	}
}

// TestSocial_AtRestSchema_PlatformInHeader_Good — Ch projects (comma-
// joined) into the header `platform` key per RFC §2.4 (channel ≅
// platform for the marketing/social surface). Pin the canonical
// projection.
func TestSocial_AtRestSchema_PlatformInHeader_Good(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newAtRestTestSvc(t)

	r := svc.Create(subject.CreateInput{
		Ch:   []string{"mastodon", "x"},
		When: "today",
		Text: "header-platform-probe",
	})
	if !r.OK {
		t.Fatalf("Create: %s", r.Error())
	}
	p := r.Value.(subject.SocialPost)
	lthnPath := postLthnPath(t, home, p.ID)
	raw := core.ReadFile(lthnPath)
	if !raw.OK {
		t.Fatalf("read encrypted file: %s", raw.Error())
	}
	body, _ := raw.Value.([]byte)
	header := string(body[:trixHeaderEnd(body)])

	if !core.Contains(header, `"platform":"mastodon,x"`) {
		t.Fatalf("expected header platform=\"mastodon,x\", got header: %s", header)
	}
}

// TestSocial_AtRestSchema_WhenInBodyNotHeader_Bad — When is free-form
// text ("today · 16:00") and not a parseable timestamp; the schema
// deliberately omits the `scheduled.at` header key per the SECURITY-
// NOTE in service.go. Header MUST NOT carry the When value or the
// `scheduled.at` key.
func TestSocial_AtRestSchema_WhenInBodyNotHeader_Bad(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newAtRestTestSvc(t)

	r := svc.Create(subject.CreateInput{
		Ch:   []string{"mastodon"},
		When: "today · 16:00",
		Text: "when-leak-probe",
	})
	if !r.OK {
		t.Fatalf("Create: %s", r.Error())
	}
	p := r.Value.(subject.SocialPost)
	lthnPath := postLthnPath(t, home, p.ID)
	raw := core.ReadFile(lthnPath)
	if !raw.OK {
		t.Fatalf("read encrypted file: %s", raw.Error())
	}
	body, _ := raw.Value.([]byte)
	header := string(body[:trixHeaderEnd(body)])

	for _, banned := range []string{`"scheduled.at"`, `"when":`, "today · 16:00"} {
		if core.Contains(header, banned) {
			t.Fatalf("When-related leak %q in encrypted-file header (RFC §2.4 free-form/no-ruling): %s", banned, header)
		}
	}

	// Round-trip body confirms persistence.
	g := svc.Get(p.ID)
	if !g.OK {
		t.Fatalf("Get: %s", g.Error())
	}
	got := g.Value.(subject.SocialPost)
	if got.When != "today · 16:00" {
		t.Fatalf("When round-trip lost: got %q", got.When)
	}
}

// TestSocial_AtRestSchema_StateInBodyNotHeader_Bad — State carries
// lifecycle info (scheduled|sent|draft) but RFC §2.4 names no header
// key for it. Default-body rule: state MUST stay in the body. Header
// MUST NOT carry it (no `"state":` key, no `"status":` either).
func TestSocial_AtRestSchema_StateInBodyNotHeader_Bad(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newAtRestTestSvc(t)

	r := svc.Create(subject.CreateInput{
		Ch:    []string{"mastodon"},
		When:  "today",
		Text:  "state-leak-probe",
		State: "scheduled",
	})
	if !r.OK {
		t.Fatalf("Create: %s", r.Error())
	}
	p := r.Value.(subject.SocialPost)
	lthnPath := postLthnPath(t, home, p.ID)
	raw := core.ReadFile(lthnPath)
	if !raw.OK {
		t.Fatalf("read encrypted file: %s", raw.Error())
	}
	body, _ := raw.Value.([]byte)
	header := string(body[:trixHeaderEnd(body)])

	if core.Contains(header, `"state":`) {
		t.Fatalf("State leaked into encrypted-file header (no RFC §2.4 ruling — default-body): %s", header)
	}
	if core.Contains(header, `"scheduled"`) {
		t.Fatalf("State value leaked into encrypted-file header: %s", header)
	}

	// Round-trip body confirms persistence.
	g := svc.Get(p.ID)
	if !g.OK {
		t.Fatalf("Get: %s", g.Error())
	}
	got := g.Value.(subject.SocialPost)
	if got.State != "scheduled" {
		t.Fatalf("State round-trip lost: got %q", got.State)
	}
}

// TestSocial_AtRestSchema_AttachInBodyNotHeader_Bad — Attach
// (attachment label, e.g. "image"|"video") is BODY-only per default-
// rule. Header MUST NOT contain it.
func TestSocial_AtRestSchema_AttachInBodyNotHeader_Bad(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newAtRestTestSvc(t)

	r := svc.Create(subject.CreateInput{
		Ch:     []string{"mastodon"},
		When:   "today",
		Text:   "attach-leak-probe",
		Attach: "image",
	})
	if !r.OK {
		t.Fatalf("Create: %s", r.Error())
	}
	p := r.Value.(subject.SocialPost)
	lthnPath := postLthnPath(t, home, p.ID)
	raw := core.ReadFile(lthnPath)
	if !raw.OK {
		t.Fatalf("read encrypted file: %s", raw.Error())
	}
	body, _ := raw.Value.([]byte)
	header := string(body[:trixHeaderEnd(body)])

	for _, banned := range []string{`"attach":`, `"image"`} {
		if core.Contains(header, banned) {
			t.Fatalf("Attach leaked into encrypted-file header (RFC §2.4 default-body): %s", header)
		}
	}

	g := svc.Get(p.ID)
	if !g.OK {
		t.Fatalf("Get: %s", g.Error())
	}
	got := g.Value.(subject.SocialPost)
	if got.Attach != "image" {
		t.Fatalf("Attach round-trip lost: got %q", got.Attach)
	}
}

// --- Reads-while-locked (RFC §4.1 + §4.2) ---------------------------

// TestSocial_LockedSession_ListWorks_Good — when the SessionGate
// reports zero unlocked accounts AFTER a record was written while
// unlocked, List MUST still return the encrypted record (header-only
// projection). The PublicKeyFor call does NOT require unlock so MAC
// verification stays open.
//
// Operator sees:
//   - ID from the header
//   - Ch (split from header `platform`)
//   - When / State / Text / Attach EMPTY (BODY-only)
func TestSocial_LockedSession_ListWorks_Good(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newAtRestTestSvc(t)
	pub, priv := genAtRestKeyPair(t)

	// Seed: write while unlocked.
	cr := svc.Create(subject.CreateInput{
		Ch:   []string{"mastodon", "x"},
		When: "today · 16:00",
		Text: "header-only-survives-lock",
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
	if len(out.Posts) != 1 {
		t.Fatalf("expected 1 list entry while locked, got %d", len(out.Posts))
	}
	got := out.Posts[0]
	// Header-only projection: When/State/Text/Attach empty, ID + Ch
	// (from header `platform`) present.
	if got.When != "" {
		t.Fatalf("header-only list entry MUST NOT carry When, got %q", got.When)
	}
	if got.State != "" {
		t.Fatalf("header-only list entry MUST NOT carry State, got %q", got.State)
	}
	if got.Text != "" {
		t.Fatalf("header-only list entry MUST NOT carry Text, got %q", got.Text)
	}
	if got.Attach != "" {
		t.Fatalf("header-only list entry MUST NOT carry Attach, got %q", got.Attach)
	}
	if got.ID == "" {
		t.Fatal("header-only list entry MUST carry ID from header")
	}
	if len(got.Ch) != 2 || got.Ch[0] != "mastodon" || got.Ch[1] != "x" {
		t.Fatalf("header-only list entry MUST carry Ch from header `platform`, got %v", got.Ch)
	}
}

// TestSocial_LockedSession_GetRefused_Bad — Get on an encrypted
// record while the session is locked MUST refuse. The substrate's Read
// calls SingleUnlockedAccount() and the adapter returns
// social.no_unlocked_account; the wails layer surfaces session.
// locked (or atrest.* codes) verbatim for the frontend.
func TestSocial_LockedSession_GetRefused_Bad(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newAtRestTestSvc(t)
	pub, priv := genAtRestKeyPair(t)

	cr := svc.Create(subject.CreateInput{
		Ch:   []string{"mastodon"},
		When: "today",
		Text: "locked-get-target",
	})
	if !cr.OK {
		t.Fatalf("seed Create: %s", cr.Error())
	}
	id := cr.Value.(subject.SocialPost).ID

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

// TestSocial_UnlockedSession_GetReturnsBody_Good — Get round-trips
// the full body (Ch, When, State, Text, Attach) when the session is
// unlocked.
func TestSocial_UnlockedSession_GetReturnsBody_Good(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newAtRestTestSvc(t)

	cr := svc.Create(subject.CreateInput{
		Ch:     []string{"mastodon", "x", "linkedin"},
		When:   "today · 16:00",
		State:  "scheduled",
		Text:   "Full body content.\nSecond line.",
		Attach: "image",
	})
	if !cr.OK {
		t.Fatalf("seed Create: %s", cr.Error())
	}
	id := cr.Value.(subject.SocialPost).ID

	g := svc.Get(id)
	if !g.OK {
		t.Fatalf("Get on unlocked record MUST succeed: %s", g.Error())
	}
	d := g.Value.(subject.SocialPost)
	if len(d.Ch) != 3 || d.Ch[0] != "mastodon" || d.Ch[1] != "x" || d.Ch[2] != "linkedin" {
		t.Fatalf("Ch round-trip lost: got %v", d.Ch)
	}
	if d.When != "today · 16:00" {
		t.Fatalf("When round-trip lost: got %q", d.When)
	}
	if d.State != "scheduled" {
		t.Fatalf("State round-trip lost: got %q", d.State)
	}
	if d.Text != "Full body content.\nSecond line." {
		t.Fatalf("Text round-trip lost: got %q", d.Text)
	}
	if d.Attach != "image" {
		t.Fatalf("Attach round-trip lost: got %q", d.Attach)
	}
}

// --- Lazy migration round-trips (RFC §3.1) --------------------------

// TestSocial_AtRest_ReadAcceptsLegacyMd_Good — a pre-existing .md
// plaintext stays readable via loadOne fallthrough until the next
// write triggers re-encrypt-as-.lthn.
func TestSocial_AtRest_ReadAcceptsLegacyMd_Good(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newAtRestTestSvc(t)

	dir := core.PathJoin(home, "Lethean", "marketing", "social")
	if mk := core.MkdirAll(dir, 0o700); !mk.OK {
		t.Fatalf("MkdirAll: %s", mk.Error())
	}
	legacyID := "legacy-post-2026"
	mdPath := core.PathJoin(dir, legacyID+".md")
	legacy := []byte("---\nid: " + legacyID + "\nch: mastodon\nwhen: yest · 11:14\nstate: scheduled\nattach: \"\"\n---\nLegacy body.\n")
	if w := core.WriteFile(mdPath, legacy, 0o600); !w.OK {
		t.Fatalf("WriteFile legacy: %s", w.Error())
	}

	g := svc.Get(legacyID)
	if !g.OK {
		t.Fatalf("Get on legacy .md MUST succeed via fallthrough: %s", g.Error())
	}
	got := g.Value.(subject.SocialPost)
	if got.Text != "Legacy body." {
		t.Fatalf("legacy Text lost: got %q", got.Text)
	}
	if got.State != "scheduled" {
		t.Fatalf("legacy State lost: got %q", got.State)
	}
}

// TestSocial_AtRest_WriteRemovesLegacyMd_Good — MarkSent against a
// legacy .md record writes the .lthn envelope AND removes the legacy
// plaintext file (RFC §3.1 lazy-migration completion).
func TestSocial_AtRest_WriteRemovesLegacyMd_Good(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newAtRestTestSvc(t)

	dir := core.PathJoin(home, "Lethean", "marketing", "social")
	if mk := core.MkdirAll(dir, 0o700); !mk.OK {
		t.Fatalf("MkdirAll: %s", mk.Error())
	}
	id := "legacy-cutover"
	mdPath := core.PathJoin(dir, id+".md")
	legacy := []byte("---\nid: " + id + "\nch: mastodon\nwhen: today\nstate: scheduled\nattach: \"\"\n---\nProcedure.\n")
	if w := core.WriteFile(mdPath, legacy, 0o600); !w.OK {
		t.Fatalf("seed legacy .md: %s", w.Error())
	}

	ur := svc.MarkSent(id)
	if !ur.OK {
		t.Fatalf("MarkSent: %s", ur.Error())
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

// TestSocial_AtRest_ReadPrefersLthnOverMd_Good — when both
// extensions exist (mid-migration crash window between AtomicWrite and
// Remove) loadPosts prefers the .lthn entry and skips the .md
// duplicate.
func TestSocial_AtRest_ReadPrefersLthnOverMd_Good(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newAtRestTestSvc(t)

	cr := svc.Create(subject.CreateInput{
		Ch:   []string{"mastodon"},
		When: "today",
		Text: "encrypted-original",
	})
	if !cr.OK {
		t.Fatalf("seed Create: %s", cr.Error())
	}
	id := cr.Value.(subject.SocialPost).ID

	// Drop a shadow .md alongside the encrypted record.
	dir := core.PathJoin(home, "Lethean", "marketing", "social")
	mdShadow := core.PathJoin(dir, id+".md")
	shadow := []byte("---\nid: " + id + "\nch: mastodon\nwhen: today\nstate: draft\nattach: \"\"\n---\nPLAINTEXT-SHADOW\n")
	if w := core.WriteFile(mdShadow, shadow, 0o600); !w.OK {
		t.Fatalf("seed shadow .md: %s", w.Error())
	}

	r := svc.List(subject.ListInput{})
	if !r.OK {
		t.Fatalf("List: %s", r.Error())
	}
	out := r.Value.(subject.ListOutput)
	if len(out.Posts) != 1 {
		t.Fatalf("expected exactly one entry (.lthn preferred), got %d", len(out.Posts))
	}
	// The entry is the encrypted one — header-only projection means
	// Text is empty (vs the shadow's "PLAINTEXT-SHADOW").
	if out.Posts[0].Text == "PLAINTEXT-SHADOW" {
		t.Fatal("List returned the shadow .md entry instead of preferring .lthn")
	}
}

// --- File-mode + dir-mode cover for the .lthn write path ------------

// TestSocial_AtRest_FileMode0600_Cerberus1487 — the substrate's
// atomic-rename path applies mode 0o600 to the encrypted file. Pin it
// alongside the legacy .md cover in security_test.go.
func TestSocial_AtRest_FileMode0600_Cerberus1487(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newAtRestTestSvc(t)

	r := svc.Create(subject.CreateInput{
		Ch:   []string{"mastodon"},
		When: "today",
		Text: "mode-probe",
	})
	if !r.OK {
		t.Fatalf("Create: %s", r.Error())
	}
	p := r.Value.(subject.SocialPost)
	lthnPath := postLthnPath(t, home, p.ID)
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
