// SPDX-Licence-Identifier: EUPL-1.2

// atrest_test.go — Stage E.D.B.3 marketing/campaigns at-rest cover
// (Mantis #1487 PR-3 wave 3). Anchors:
//
//   - RFC.stage-e-encrypt-at-rest v2 §2.4 per-field MUST table for
//     marketing/campaigns:
//       * name         → BODY (REJECT in header)
//       * platform     → HEADER (CONFIRM; projected from Channel)
//       * scheduled.at → HEADER (MONTH-only) — Campaign model has no
//         scheduled-time field today; the schema omits this header key
//         (SECURITY-NOTE in service.go atrestWriterFor doc).
//     All other fields (state / reach / convert / spend / body) are
//     BODY-only by default-rule.
//   - §4.1 reads-while-locked: header-only List works; full-body Get
//     refused.
//   - §3.1 lazy migration: writes always emit .lthn; .md removed on
//     success; reads accept BOTH formats.
//
// Substrate-level invariants (Trix shape, header MAC, single-unlock,
// schema enforcement) are exercised by pkg/recordfile/atrest_test.go
// — this file pins consumer-side compositions only.

package campaigns_test

import (
	"sync"
	"testing"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/account"
	subject "dappco.re/lthn/desktop/pkg/marketing/campaigns"
	"forge.lthn.ai/Snider/Enchantrix/pkg/crypt/std/pgp"
)

// stubKeySessionGate is the at-rest-capable test double — satisfies
// both the narrow SessionGate (UnlockedAccountIDs) AND the wider
// accountKeyProvider runtime-assertion (PublicKeyFor + PrivateKeyFor)
// the at-rest writer engages. Distinct from stubSessionGate in
// campaigns_test.go so the existing minimal-gate fail-safe + legacy
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
// expressly for sibling-package consumers like marketing/campaigns.
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

// newAtRestTestSvc constructs a campaigns.Service pre-wired with the
// WIDE stubKeySessionGate (ids + pub + priv). The at-rest writer
// engages by default — Create produces `<id>.lthn` envelopes, Get
// decrypts via PrivateKeyFor.
//
// Tests that need the locked-session path call SetSessionGate
// explicitly with ids=[]. Tests that need the legacy plaintext .md
// fallback use the NARROW stubSessionGate from campaigns_test.go.
//
// Usage example:
//
//	svc := newAtRestTestSvc(t)
//	r := svc.Create(subject.CreateInput{Name: "..."})
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

// campaignLthnPath returns the absolute path to the encrypted campaign
// file for the named id under the test HOME.
//
//	p := campaignLthnPath(t, home, "v02-launch-1747440000")
func campaignLthnPath(t *testing.T, home, id string) string {
	t.Helper()
	return core.PathJoin(home, "Lethean", "marketing", "campaigns", id+".lthn")
}

// --- Per-field MUST rules (RFC §2.4) --------------------------------

// TestCampaigns_AtRestSchema_NameInBodyNotHeader_Bad — Name (campaign
// branding, PII-adjacent) MUST NEVER appear in the plaintext header.
// Body decrypt round-trips the value back to the in-memory Campaign.
func TestCampaigns_AtRestSchema_NameInBodyNotHeader_Bad(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newAtRestTestSvc(t)

	r := svc.Create(subject.CreateInput{
		Name: "Sentinel Bank · Q3 enterprise launch",
		// State + Reach + Convert + Spend defaults engage.
		Channel: "paid",
	})
	if !r.OK {
		t.Fatalf("Create: %s", r.Error())
	}
	c := r.Value.(subject.Campaign)
	lthnPath := campaignLthnPath(t, home, c.ID)
	raw := core.ReadFile(lthnPath)
	if !raw.OK {
		t.Fatalf("read encrypted file: %s", raw.Error())
	}
	body, _ := raw.Value.([]byte)
	header := string(body[:trixHeaderEnd(body)])

	if core.Contains(header, "Sentinel Bank") {
		t.Fatal("Name leaked into encrypted-file header — must be body-only (RFC §2.4)")
	}
	if core.Contains(string(body[trixHeaderEnd(body):]), "Sentinel Bank") {
		t.Fatal("Name appeared in ciphertext payload — encryption skipped?")
	}

	// Round-trip via Get to prove the body retains the value.
	g := svc.Get(c.ID)
	if !g.OK {
		t.Fatalf("Get after Create: %s", g.Error())
	}
	got := g.Value.(subject.Campaign)
	if got.Name != "Sentinel Bank · Q3 enterprise launch" {
		t.Fatalf("Name round-trip lost: got %q", got.Name)
	}
}

// TestCampaigns_AtRestSchema_PlatformInHeader_Good — Channel projects
// into the header `platform` key per RFC §2.4 (channel ≅ platform for
// the marketing/campaigns surface). Pin the canonical projection.
func TestCampaigns_AtRestSchema_PlatformInHeader_Good(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newAtRestTestSvc(t)

	r := svc.Create(subject.CreateInput{
		Name:    "header-platform-probe",
		Channel: "paid",
	})
	if !r.OK {
		t.Fatalf("Create: %s", r.Error())
	}
	c := r.Value.(subject.Campaign)
	lthnPath := campaignLthnPath(t, home, c.ID)
	raw := core.ReadFile(lthnPath)
	if !raw.OK {
		t.Fatalf("read encrypted file: %s", raw.Error())
	}
	body, _ := raw.Value.([]byte)
	header := string(body[:trixHeaderEnd(body)])

	if !core.Contains(header, `"platform":"paid"`) {
		t.Fatalf("expected header platform=\"paid\", got header: %s", header)
	}
}

// TestCampaigns_AtRestSchema_StateInBodyNotHeader_Bad — State carries
// lifecycle info (live|scheduled|draft|complete) but RFC §2.4 names no
// header key for it. Default-body rule: state MUST stay in the body.
// Header MUST NOT carry it (no `"state":` key, no `"status":` either).
func TestCampaigns_AtRestSchema_StateInBodyNotHeader_Bad(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newAtRestTestSvc(t)

	r := svc.Create(subject.CreateInput{
		Name:    "state-leak-probe",
		State:   "live",
		Channel: "earned",
	})
	if !r.OK {
		t.Fatalf("Create: %s", r.Error())
	}
	c := r.Value.(subject.Campaign)
	lthnPath := campaignLthnPath(t, home, c.ID)
	raw := core.ReadFile(lthnPath)
	if !raw.OK {
		t.Fatalf("read encrypted file: %s", raw.Error())
	}
	body, _ := raw.Value.([]byte)
	header := string(body[:trixHeaderEnd(body)])

	if core.Contains(header, `"state":`) {
		t.Fatalf("State leaked into encrypted-file header (no RFC §2.4 ruling — default-body): %s", header)
	}
	if core.Contains(header, `"live"`) {
		t.Fatalf("State value leaked into encrypted-file header: %s", header)
	}

	// Round-trip body confirms persistence.
	g := svc.Get(c.ID)
	if !g.OK {
		t.Fatalf("Get: %s", g.Error())
	}
	got := g.Value.(subject.Campaign)
	if got.State != "live" {
		t.Fatalf("State round-trip lost: got %q", got.State)
	}
}

// TestCampaigns_AtRestSchema_MetricsInBodyNotHeader_Bad — Reach /
// Convert / Spend are sensitive metrics (financial + audience). RFC
// §2.4 names no header key for any of them; default-body rule keeps
// them encrypted. Header MUST NOT contain the metric values.
func TestCampaigns_AtRestSchema_MetricsInBodyNotHeader_Bad(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newAtRestTestSvc(t)

	r := svc.Create(subject.CreateInput{
		Name:    "metric-leak-probe",
		State:   "live",
		Reach:   "42 K",
		Convert: "3.2%",
		Spend:   "£17500",
		Channel: "earned",
	})
	if !r.OK {
		t.Fatalf("Create: %s", r.Error())
	}
	c := r.Value.(subject.Campaign)
	lthnPath := campaignLthnPath(t, home, c.ID)
	raw := core.ReadFile(lthnPath)
	if !raw.OK {
		t.Fatalf("read encrypted file: %s", raw.Error())
	}
	body, _ := raw.Value.([]byte)
	header := string(body[:trixHeaderEnd(body)])

	for _, banned := range []string{"42 K", "3.2%", "£17500", `"reach":`, `"convert":`, `"spend":`} {
		if core.Contains(header, banned) {
			t.Fatalf("metric %q leaked into encrypted-file header (RFC §2.4 default-body): %s", banned, header)
		}
	}

	// Round-trip body confirms persistence.
	g := svc.Get(c.ID)
	if !g.OK {
		t.Fatalf("Get: %s", g.Error())
	}
	got := g.Value.(subject.Campaign)
	if got.Reach != "42 K" || got.Convert != "3.2%" || got.Spend != "£17500" {
		t.Fatalf("metrics round-trip lost: reach=%q convert=%q spend=%q",
			got.Reach, got.Convert, got.Spend)
	}
}

// TestCampaigns_AtRestSchema_BodyInBodyNotHeader_Bad — the campaign
// brief body (markdown notes) MUST NEVER appear in the plaintext
// header. Body decrypt round-trips the value.
func TestCampaigns_AtRestSchema_BodyInBodyNotHeader_Bad(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newAtRestTestSvc(t)

	brief := "## Brief\n\nDeath Star reactor pitch deck — confidential."
	r := svc.Create(subject.CreateInput{
		Name:    "body-leak-probe",
		Channel: "direct",
		Body:    brief,
	})
	if !r.OK {
		t.Fatalf("Create: %s", r.Error())
	}
	c := r.Value.(subject.Campaign)
	lthnPath := campaignLthnPath(t, home, c.ID)
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
	got := g.Value.(subject.Campaign)
	if got.Body != brief {
		t.Fatalf("Body round-trip lost: got %q", got.Body)
	}
}

// --- Reads-while-locked (RFC §4.1 + §4.2) ---------------------------

// TestCampaigns_LockedSession_ListWorks_Good — when the SessionGate
// reports zero unlocked accounts AFTER a record was written while
// unlocked, List MUST still return the encrypted record (header-only
// projection). The PublicKeyFor call does NOT require unlock so MAC
// verification stays open.
//
// Operator sees:
//   - ID from the header
//   - Channel from header `platform`
//   - Name / State / Reach / Convert / Spend / Body EMPTY (BODY-only)
func TestCampaigns_LockedSession_ListWorks_Good(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newAtRestTestSvc(t)
	pub, priv := genAtRestKeyPair(t)

	// Seed: write while unlocked.
	cr := svc.Create(subject.CreateInput{
		Name:    "header-only-survives-lock",
		Channel: "paid",
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
	if len(out.Campaigns) != 1 {
		t.Fatalf("expected 1 list entry while locked, got %d", len(out.Campaigns))
	}
	got := out.Campaigns[0]
	// Header-only projection: Name/State/Reach/Convert/Spend/Body empty,
	// ID + Channel (from header `platform`) present.
	if got.Name != "" {
		t.Fatalf("header-only list entry MUST NOT carry Name, got %q", got.Name)
	}
	if got.State != "" {
		t.Fatalf("header-only list entry MUST NOT carry State, got %q", got.State)
	}
	if got.Reach != "" {
		t.Fatalf("header-only list entry MUST NOT carry Reach, got %q", got.Reach)
	}
	if got.Convert != "" {
		t.Fatalf("header-only list entry MUST NOT carry Convert, got %q", got.Convert)
	}
	if got.Spend != "" {
		t.Fatalf("header-only list entry MUST NOT carry Spend, got %q", got.Spend)
	}
	if got.Body != "" {
		t.Fatalf("header-only list entry MUST NOT carry Body, got %q", got.Body)
	}
	if got.ID == "" {
		t.Fatal("header-only list entry MUST carry ID from header")
	}
	if got.Channel != "paid" {
		t.Fatalf("header-only list entry MUST carry Channel from header `platform`, got %q", got.Channel)
	}
}

// TestCampaigns_LockedSession_GetRefused_Bad — Get on an encrypted
// record while the session is locked MUST refuse. The substrate's Read
// calls SingleUnlockedAccount() and the adapter returns
// campaigns.no_unlocked_account; the wails layer surfaces session.
// locked (or atrest.* codes) verbatim for the frontend.
func TestCampaigns_LockedSession_GetRefused_Bad(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newAtRestTestSvc(t)
	pub, priv := genAtRestKeyPair(t)

	cr := svc.Create(subject.CreateInput{
		Name:    "locked-get-target",
		Channel: "paid",
	})
	if !cr.OK {
		t.Fatalf("seed Create: %s", cr.Error())
	}
	id := cr.Value.(subject.Campaign).ID

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

// TestCampaigns_UnlockedSession_GetReturnsBody_Good — Get round-trips
// the full body (Name, State, Reach, Convert, Spend, Channel, Body)
// when the session is unlocked.
func TestCampaigns_UnlockedSession_GetReturnsBody_Good(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newAtRestTestSvc(t)

	cr := svc.Create(subject.CreateInput{
		Name:    "Sentinel Bank · Q3",
		State:   "live",
		Reach:   "42 K",
		Convert: "3.2%",
		Spend:   "£17500",
		Channel: "paid",
		Body:    "## Brief\n\nFull body content.",
	})
	if !cr.OK {
		t.Fatalf("seed Create: %s", cr.Error())
	}
	id := cr.Value.(subject.Campaign).ID

	g := svc.Get(id)
	if !g.OK {
		t.Fatalf("Get on unlocked record MUST succeed: %s", g.Error())
	}
	d := g.Value.(subject.Campaign)
	if d.Name != "Sentinel Bank · Q3" {
		t.Fatalf("Name round-trip lost: got %q", d.Name)
	}
	if d.State != "live" {
		t.Fatalf("State round-trip lost: got %q", d.State)
	}
	if d.Reach != "42 K" || d.Convert != "3.2%" || d.Spend != "£17500" {
		t.Fatalf("metrics round-trip lost: reach=%q convert=%q spend=%q",
			d.Reach, d.Convert, d.Spend)
	}
	if d.Channel != "paid" {
		t.Fatalf("Channel round-trip lost: got %q", d.Channel)
	}
	if d.Body != "## Brief\n\nFull body content." {
		t.Fatalf("Body round-trip lost: got %q", d.Body)
	}
}

// --- Lazy migration round-trips (RFC §3.1) --------------------------

// TestCampaigns_AtRest_ReadAcceptsLegacyMd_Good — a pre-existing .md
// plaintext stays readable via loadOne fallthrough until the next
// write triggers re-encrypt-as-.lthn.
func TestCampaigns_AtRest_ReadAcceptsLegacyMd_Good(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newAtRestTestSvc(t)

	dir := core.PathJoin(home, "Lethean", "marketing", "campaigns")
	if mk := core.MkdirAll(dir, 0o700); !mk.OK {
		t.Fatalf("MkdirAll: %s", mk.Error())
	}
	legacyID := "legacy-campaign-2026"
	mdPath := core.PathJoin(dir, legacyID+".md")
	legacy := []byte("---\nid: " + legacyID + "\nname: LegacyCamp\nstate: draft\nreach: \"\"\nconvert: \"\"\nspend: \"£0\"\nchannel: earned\n---\nLegacy body.\n")
	if w := core.WriteFile(mdPath, legacy, 0o600); !w.OK {
		t.Fatalf("WriteFile legacy: %s", w.Error())
	}

	g := svc.Get(legacyID)
	if !g.OK {
		t.Fatalf("Get on legacy .md MUST succeed via fallthrough: %s", g.Error())
	}
	got := g.Value.(subject.Campaign)
	if got.Name != "LegacyCamp" {
		t.Fatalf("legacy Name lost: got %q", got.Name)
	}
}

// TestCampaigns_AtRest_WriteRemovesLegacyMd_Good — Update against a
// legacy .md record writes the .lthn envelope AND removes the legacy
// plaintext file (RFC §3.1 lazy-migration completion).
func TestCampaigns_AtRest_WriteRemovesLegacyMd_Good(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newAtRestTestSvc(t)

	dir := core.PathJoin(home, "Lethean", "marketing", "campaigns")
	if mk := core.MkdirAll(dir, 0o700); !mk.OK {
		t.Fatalf("MkdirAll: %s", mk.Error())
	}
	id := "legacy-cutover"
	mdPath := core.PathJoin(dir, id+".md")
	legacy := []byte("---\nid: " + id + "\nname: Cutover\nstate: draft\nreach: \"\"\nconvert: \"\"\nspend: \"£0\"\nchannel: earned\n---\nProcedure.\n")
	if w := core.WriteFile(mdPath, legacy, 0o600); !w.OK {
		t.Fatalf("seed legacy .md: %s", w.Error())
	}

	ur := svc.Update(subject.UpdateInput{ID: id, State: "live"})
	if !ur.OK {
		t.Fatalf("Update: %s", ur.Error())
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

// TestCampaigns_AtRest_ReadPrefersLthnOverMd_Good — when both
// extensions exist (mid-migration crash window between AtomicWrite and
// Remove) loadCampaigns prefers the .lthn entry and skips the .md
// duplicate.
func TestCampaigns_AtRest_ReadPrefersLthnOverMd_Good(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newAtRestTestSvc(t)

	cr := svc.Create(subject.CreateInput{
		Name:    "encrypted-original",
		Channel: "paid",
	})
	if !cr.OK {
		t.Fatalf("seed Create: %s", cr.Error())
	}
	id := cr.Value.(subject.Campaign).ID

	// Drop a shadow .md alongside the encrypted record.
	dir := core.PathJoin(home, "Lethean", "marketing", "campaigns")
	mdShadow := core.PathJoin(dir, id+".md")
	shadow := []byte("---\nid: " + id + "\nname: PLAINTEXT-SHADOW\nstate: draft\nreach: \"\"\nconvert: \"\"\nspend: \"£0\"\nchannel: earned\n---\nShadow.\n")
	if w := core.WriteFile(mdShadow, shadow, 0o600); !w.OK {
		t.Fatalf("seed shadow .md: %s", w.Error())
	}

	r := svc.List(subject.ListInput{})
	if !r.OK {
		t.Fatalf("List: %s", r.Error())
	}
	out := r.Value.(subject.ListOutput)
	if len(out.Campaigns) != 1 {
		t.Fatalf("expected exactly one entry (.lthn preferred), got %d", len(out.Campaigns))
	}
	// The entry is the encrypted one — header-only projection means
	// Name is empty and Channel is mapped from header `platform`.
	if out.Campaigns[0].Name == "PLAINTEXT-SHADOW" {
		t.Fatal("List returned the shadow .md entry instead of preferring .lthn")
	}
}

// --- File-mode + dir-mode cover for the .lthn write path ------------

// TestCampaigns_AtRest_FileMode0600_Cerberus1487 — the substrate's
// atomic-rename path applies mode 0o600 to the encrypted file. Pin it
// alongside the legacy .md cover in security_test.go.
func TestCampaigns_AtRest_FileMode0600_Cerberus1487(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newAtRestTestSvc(t)

	r := svc.Create(subject.CreateInput{
		Name:    "mode-probe",
		Channel: "earned",
	})
	if !r.OK {
		t.Fatalf("Create: %s", r.Error())
	}
	c := r.Value.(subject.Campaign)
	lthnPath := campaignLthnPath(t, home, c.ID)
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
