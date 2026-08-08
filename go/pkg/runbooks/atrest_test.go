// SPDX-Licence-Identifier: EUPL-1.2

// atrest_test.go — Stage E.D.B.2 runbooks at-rest cover (Mantis #1487
// PR-3 wave 2). Anchors:
//
//   - RFC.stage-e-encrypt-at-rest v2 §2.4 per-field MUST table:
//     title stays BODY-only; tags MUST be SHA-256 hash-projected in
//     the header ONLY (free-form rejected); area / body / last_
//     rehearsed / created_at stay BODY-only.
//   - §4.1 reads-while-locked: header-only List works; full-body Get
//     refused; MarkRehearsed refused.
//   - §3.1 lazy migration: writes always emit .lthn; .md removed on
//     success; reads accept BOTH formats.
//
// Substrate-level invariants (Trix shape, header MAC, single-unlock,
// schema enforcement) are exercised by pkg/recordfile/atrest_test.go
// — this file pins consumer-side compositions only.

package runbooks_test

import (
	"sync"
	"testing"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/account"
	"dappco.re/lthn/desktop/pkg/recordfile"
	subject "dappco.re/lthn/desktop/pkg/runbooks"
	"github.com/Snider/Enchantrix/pkg/crypt/std/pgp"
)

// stubKeySessionGate is the at-rest-capable test double — satisfies
// both the narrow SessionGate (UnlockedAccountIDs) AND the wider
// accountKeyProvider runtime-assertion (PublicKeyFor + PrivateKeyFor)
// the at-rest writer engages. Distinct from stubSessionGate in
// runbooks_test.go so the existing minimal-gate fail-safe tests stay
// untouched.
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
// expressly for sibling-package consumers like runbooks.
func (s *stubKeySessionGate) PrivateKeyFor(_ string) (*account.PrivateKeyHandle, bool) {
	if len(s.priv) == 0 {
		return nil, false
	}
	cp := make([]byte, len(s.priv))
	copy(cp, s.priv)
	return account.NewPrivateKeyHandleForTest(cp), true
}

// testKeyPair is the package-shared keypair generated once per
// process so multiple at-rest tests reuse the same costly setup.
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

// newAtRestTestSvc constructs a runbooks.Service pre-wired with a
// SessionGate reporting one unlocked account AND a real PGP keypair.
// With both wired, the at-rest write path engages by default —
// seedAll still writes plaintext .md (it runs from OnStart pre-Gate
// in production) but MarkRehearsed routes through the .lthn substrate.
//
// Tests that need to exercise the locked-session path call
// SetSessionGate explicitly with an empty-ids stub. Tests that need
// the legacy plaintext fallback use the narrow stubSessionGate from
// runbooks_test.go (no PublicKeyFor).
//
// Usage example:
//
//	svc := newAtRestTestSvc(t)
//	_ = svc.OnStart()
//	r := svc.MarkRehearsed(subject.MarkInput{ID: "R-01"})
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
// `[Magic(4)][Version(1)][HeaderLen(4)][HeaderJSON][Payload]`. Used
// to slice header-vs-payload regions for plaintext-leak assertions.
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

// runbookLthnPath returns the absolute path to the encrypted runbook
// file for the named slug under the test HOME.
//
//	p := runbookLthnPath(t, "rotate-runtime-api-keys")
func runbookLthnPath(t *testing.T, home, slug string) string {
	t.Helper()
	return core.PathJoin(home, "Lethean", "runbooks", slug+".lthn")
}

// --- Per-field MUST rules (RFC §2.4) --------------------------------

// TestRunbooks_AtRestSchema_TitleInBodyOnly_Bad — Title (operational
// secret context) MUST NEVER appear in the plaintext header. Body
// decrypt round-trips the value back to the in-memory RunbookRecord.
func TestRunbooks_AtRestSchema_TitleInBodyOnly_Bad(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newAtRestTestSvc(t)
	if r := svc.OnStart(); !r.OK {
		t.Fatalf("OnStart: %s", r.Error())
	}

	// Trigger an at-rest write via MarkRehearsed.
	if mr := svc.MarkRehearsed(subject.MarkInput{ID: "R-01"}); !mr.OK {
		t.Fatalf("MarkRehearsed: %s", mr.Error())
	}

	// The encrypted envelope should now exist under the seed slug.
	lthnPath := runbookLthnPath(t, home, "rotate-runtime-api-keys")
	raw := core.ReadFile(lthnPath)
	if !raw.OK {
		t.Fatalf("read encrypted file: %s", raw.Error())
	}
	body, _ := raw.Value.([]byte)

	// Header MUST NOT contain the runbook title — per RFC §2.4 it's
	// BODY-only.
	header := string(body[:trixHeaderEnd(body)])
	if core.Contains(header, "Rotate runtime API keys") {
		t.Fatal("Title leaked into encrypted-file header — must be body-only")
	}

	// Ciphertext payload MUST NOT contain the title in plaintext —
	// that would indicate PGP encryption silently failed.
	if core.Contains(string(body[trixHeaderEnd(body):]), "Rotate runtime API keys") {
		t.Fatal("Title appeared in ciphertext payload — encryption skipped?")
	}

	// Round-trip via Get to prove the body retains the value.
	g := svc.Get(subject.GetInput{ID: "R-01"})
	if !g.OK {
		t.Fatalf("Get after MarkRehearsed: %s", g.Error())
	}
	d := g.Value.(subject.RunbookDetail)
	if d.Title != "Rotate runtime API keys" {
		t.Fatalf("Title round-trip lost: got %q", d.Title)
	}
}

// TestRunbooks_AtRestSchema_TagsHashedInHeader_Good — per RFC §2.4
// runbook tags MUST be SHA-256 hash-projected in the header (each
// header tag = `sha256(tag)[:16]` hex). The plaintext tag list stays
// in the encrypted body. Operator-while-locked sees hashes, not
// names — that IS the intent of the Cerberus C#7 Q8 ruling.
func TestRunbooks_AtRestSchema_TagsHashedInHeader_Good(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newAtRestTestSvc(t)
	if r := svc.OnStart(); !r.OK {
		t.Fatalf("OnStart: %s", r.Error())
	}

	// Seed runbook R-01 has tags ["auth","api-keys","security"].
	if mr := svc.MarkRehearsed(subject.MarkInput{ID: "R-01"}); !mr.OK {
		t.Fatalf("MarkRehearsed: %s", mr.Error())
	}

	lthnPath := runbookLthnPath(t, home, "rotate-runtime-api-keys")
	raw := core.ReadFile(lthnPath)
	if !raw.OK {
		t.Fatalf("read encrypted file: %s", raw.Error())
	}
	body, _ := raw.Value.([]byte)
	header := string(body[:trixHeaderEnd(body)])

	// Plaintext tag names MUST NOT appear in the header.
	for _, plaintext := range []string{"auth", "api-keys", "security"} {
		if core.Contains(header, "\""+plaintext+"\"") {
			t.Fatalf("plaintext tag %q leaked into header — MUST be hashed (RFC §2.4)", plaintext)
		}
	}

	// Each tag's HashTag projection MUST appear in the header — the
	// substrate's ValidateHashTagList enforces shape; we assert
	// presence here as the round-trip pin.
	for _, plaintext := range []string{"auth", "api-keys", "security"} {
		want := recordfile.HashTag(plaintext)
		if !core.Contains(header, want) {
			t.Fatalf("expected HashTag(%q)=%s in header, header=%s", plaintext, want, header)
		}
	}

	// And the encrypted payload MUST NOT contain plaintext tag names
	// either — encryption sanity.
	payload := string(body[trixHeaderEnd(body):])
	for _, plaintext := range []string{"\"auth\"", "\"api-keys\"", "\"security\""} {
		if core.Contains(payload, plaintext) {
			t.Fatalf("plaintext tag literal %q appeared in ciphertext payload — encryption skipped?", plaintext)
		}
	}
}

// TestRunbooks_AtRestSchema_BodyInBodyOnly_Bad — the runbook body
// (procedure markdown) MUST NEVER appear in the plaintext header.
// Body decrypt round-trips the value back.
func TestRunbooks_AtRestSchema_BodyInBodyOnly_Bad(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newAtRestTestSvc(t)
	if r := svc.OnStart(); !r.OK {
		t.Fatalf("OnStart: %s", r.Error())
	}

	// Trigger an at-rest write.
	if mr := svc.MarkRehearsed(subject.MarkInput{ID: "R-01"}); !mr.OK {
		t.Fatalf("MarkRehearsed: %s", mr.Error())
	}

	lthnPath := runbookLthnPath(t, home, "rotate-runtime-api-keys")
	raw := core.ReadFile(lthnPath)
	if !raw.OK {
		t.Fatalf("read encrypted file: %s", raw.Error())
	}
	body, _ := raw.Value.([]byte)
	header := string(body[:trixHeaderEnd(body)])

	// The seed body contains "This is a seed runbook." — MUST stay
	// inside the encrypted payload.
	if core.Contains(header, "seed runbook") {
		t.Fatal("runbook body text leaked into encrypted-file header — must be body-only")
	}
	if core.Contains(string(body[trixHeaderEnd(body):]), "seed runbook") {
		t.Fatal("runbook body text appeared in ciphertext payload — encryption skipped?")
	}
}

// --- Reads-while-locked (RFC §4.1 + §4.2) ---------------------------

// TestRunbooks_LockedSession_ListWorks_Good — when the SessionGate
// reports zero unlocked accounts AFTER a record was written while
// unlocked, List MUST still return the encrypted record (header-only
// projection). The PublicKeyFor call does NOT require unlock so MAC
// verification stays open.
//
// Operator sees:
//   - ID from the header (e.g. "R-01")
//   - Tags as hashes (16-hex-char SHA-256 truncations)
//   - Title EMPTY (BODY-only)
//   - Health "stale" (computed from LastRehearsed=zero on header-only)
func TestRunbooks_LockedSession_ListWorks_Good(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newAtRestTestSvc(t)
	pub, priv := genAtRestKeyPair(t)

	// Seed + force one at-rest write while unlocked.
	if r := svc.OnStart(); !r.OK {
		t.Fatalf("OnStart: %s", r.Error())
	}
	if mr := svc.MarkRehearsed(subject.MarkInput{ID: "R-01"}); !mr.OK {
		t.Fatalf("MarkRehearsed (unlocked seed): %s", mr.Error())
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
	// Find the R-01 entry — it's the one that landed as .lthn.
	var found *subject.RunbookEntry
	for i := range out.Books {
		if out.Books[i].ID == "R-01" {
			found = &out.Books[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("R-01 missing from locked-session List output; got %+v", out.Books)
	}
	// Header-only projection: Title empty, ID populated.
	if found.Title != "" {
		t.Fatalf("header-only list entry MUST NOT carry Title, got %q", found.Title)
	}
	if found.ID != "R-01" {
		t.Fatalf("header-only list entry MUST carry ID from header, got %q", found.ID)
	}
}

// TestRunbooks_LockedSession_GetRefused_Bad — Get on an encrypted
// record while the session is locked MUST refuse. The substrate's
// Read calls SingleUnlockedAccount() and the adapter returns
// runbooks.no_unlocked_account; the wails layer surfaces session.
// locked (or atrest.* codes) verbatim for the frontend.
func TestRunbooks_LockedSession_GetRefused_Bad(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newAtRestTestSvc(t)
	pub, priv := genAtRestKeyPair(t)

	if r := svc.OnStart(); !r.OK {
		t.Fatalf("OnStart: %s", r.Error())
	}
	if mr := svc.MarkRehearsed(subject.MarkInput{ID: "R-01"}); !mr.OK {
		t.Fatalf("MarkRehearsed (seed): %s", mr.Error())
	}

	// Lock the session.
	svc.SetSessionGate(&stubKeySessionGate{ids: []string{}, pub: pub, priv: priv})

	g := svc.Get(subject.GetInput{ID: "R-01"})
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

// TestRunbooks_UnlockedSession_GetReturnsBody_Good — Get round-trips
// the full body (Title, Area, Tags, Body) when the session is
// unlocked and the unlocked account matches the record's header
// account.id.
func TestRunbooks_UnlockedSession_GetReturnsBody_Good(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newAtRestTestSvc(t)

	if r := svc.OnStart(); !r.OK {
		t.Fatalf("OnStart: %s", r.Error())
	}
	if mr := svc.MarkRehearsed(subject.MarkInput{ID: "R-01"}); !mr.OK {
		t.Fatalf("MarkRehearsed: %s", mr.Error())
	}

	g := svc.Get(subject.GetInput{ID: "R-01"})
	if !g.OK {
		t.Fatalf("Get on unlocked record MUST succeed: %s", g.Error())
	}
	d := g.Value.(subject.RunbookDetail)
	if d.Title != "Rotate runtime API keys" {
		t.Fatalf("Title round-trip lost: got %q", d.Title)
	}
	if d.Area != "auth" {
		t.Fatalf("Area round-trip lost: got %q", d.Area)
	}
	if d.Body == "" {
		t.Fatal("Body MUST round-trip from encrypted payload")
	}
}

// --- Lazy migration round-trips (RFC §3.1) --------------------------

// TestRunbooks_AtRest_ReadAcceptsLegacyMd_Good — a pre-existing .md
// plaintext stays readable via the scanAll/loadOne fallthrough until
// the next write triggers re-encrypt-as-.lthn.
func TestRunbooks_AtRest_ReadAcceptsLegacyMd_Good(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newAtRestTestSvc(t)

	dir := core.PathJoin(home, "Lethean", "runbooks")
	if mk := core.MkdirAll(dir, 0o700); !mk.OK {
		t.Fatalf("MkdirAll: %s", mk.Error())
	}
	legacySlug := "legacy-runbook-2026"
	legacyID := "R-LEGACY"
	mdPath := core.PathJoin(dir, legacySlug+".md")
	legacy := []byte("---\nid: " + legacyID + "\ntitle: LegacyRb\narea: client\ntags: [legacy]\nlast_rehearsed: 2026-01-01T00:00:00Z\ncreated_at: 2026-01-01T00:00:00Z\n---\nLegacy body.\n")
	if w := core.WriteFile(mdPath, legacy, 0o600); !w.OK {
		t.Fatalf("WriteFile legacy: %s", w.Error())
	}

	g := svc.Get(subject.GetInput{ID: legacyID})
	if !g.OK {
		t.Fatalf("Get on legacy .md MUST succeed via fallthrough: %s", g.Error())
	}
	d := g.Value.(subject.RunbookDetail)
	if d.Title != "LegacyRb" {
		t.Fatalf("legacy Title lost: got %q", d.Title)
	}
}

// TestRunbooks_AtRest_WriteRemovesLegacyMd_Good — MarkRehearsed against
// a legacy .md record writes the .lthn envelope AND removes the legacy
// plaintext file (RFC §3.1 lazy-migration completion). Tolerates one-
// shot remove-failure but exercises the happy path here.
func TestRunbooks_AtRest_WriteRemovesLegacyMd_Good(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newAtRestTestSvc(t)

	dir := core.PathJoin(home, "Lethean", "runbooks")
	if mk := core.MkdirAll(dir, 0o700); !mk.OK {
		t.Fatalf("MkdirAll: %s", mk.Error())
	}
	slug := "legacy-runbook-cutover"
	id := "R-CUTOVER"
	mdPath := core.PathJoin(dir, slug+".md")
	legacy := []byte("---\nid: " + id + "\ntitle: Cutover\narea: ops\ntags: [cutover]\nlast_rehearsed: 2026-01-01T00:00:00Z\ncreated_at: 2026-01-01T00:00:00Z\n---\nProcedure.\n")
	if w := core.WriteFile(mdPath, legacy, 0o600); !w.OK {
		t.Fatalf("seed legacy .md: %s", w.Error())
	}

	if mr := svc.MarkRehearsed(subject.MarkInput{ID: id}); !mr.OK {
		t.Fatalf("MarkRehearsed: %s", mr.Error())
	}

	// .lthn MUST exist post-write.
	lthnPath := core.PathJoin(dir, slug+".lthn")
	if st := core.Stat(lthnPath); !st.OK {
		t.Fatalf("encrypted .lthn MUST be written, missing at %s", lthnPath)
	}
	// .md MUST be removed (lazy-migration completion).
	if st := core.Stat(mdPath); st.OK {
		t.Fatal("legacy .md MUST be removed after successful encrypted write (RFC §3.1)")
	}
}

// TestRunbooks_AtRest_ReadPrefersLthnOverMd_Good — when both
// extensions exist (mid-migration crash window between AtomicWrite and
// Remove) scanAll prefers the .lthn entry and skips the .md duplicate.
func TestRunbooks_AtRest_ReadPrefersLthnOverMd_Good(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newAtRestTestSvc(t)

	if r := svc.OnStart(); !r.OK {
		t.Fatalf("OnStart: %s", r.Error())
	}
	if mr := svc.MarkRehearsed(subject.MarkInput{ID: "R-01"}); !mr.OK {
		t.Fatalf("MarkRehearsed: %s", mr.Error())
	}
	// MarkRehearsed should have removed the seed .md and written the
	// .lthn. Hand-craft a SHADOW .md alongside to simulate a crash
	// between AtomicWrite and Remove.
	dir := core.PathJoin(home, "Lethean", "runbooks")
	slug := "rotate-runtime-api-keys"
	mdShadow := core.PathJoin(dir, slug+".md")
	shadow := []byte("---\nid: R-01\ntitle: PlaintextShadow\narea: shadow\ntags: [shadow]\nlast_rehearsed: 2026-01-01T00:00:00Z\ncreated_at: 2026-01-01T00:00:00Z\n---\nShadow body.\n")
	if w := core.WriteFile(mdShadow, shadow, 0o600); !w.OK {
		t.Fatalf("seed shadow .md: %s", w.Error())
	}

	r := svc.List(subject.ListInput{})
	if !r.OK {
		t.Fatalf("List: %s", r.Error())
	}
	out := r.Value.(subject.ListOutput)
	// Find R-01 — there must be exactly one entry for this slug,
	// from the .lthn (header-only Title="").
	var matches []subject.RunbookEntry
	for _, b := range out.Books {
		if b.ID == "R-01" {
			matches = append(matches, b)
		}
	}
	if len(matches) != 1 {
		t.Fatalf("expected exactly one R-01 entry (lthn preferred), got %d: %+v", len(matches), matches)
	}
	if matches[0].Title == "PlaintextShadow" {
		t.Fatal("List returned the shadow .md entry instead of preferring .lthn")
	}
}

// --- Sole-writer discipline (brief done-criterion 6) ----------------

// TestRunbooks_MarkRehearsed_IsSoleWriter — confirms MarkRehearsed
// continues to be the only mutation entry-point on the runbooks
// surface. The existing SessionGate retrofit (Mantis #1613 / H#151)
// continues to gate it via assertUnlocked; this test pins the
// reads-stay-open/writes-stay-gated contract surviving the at-rest
// cutover.
func TestRunbooks_MarkRehearsed_IsSoleWriter(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newAtRestTestSvc(t)
	if r := svc.OnStart(); !r.OK {
		t.Fatalf("OnStart: %s", r.Error())
	}

	// Lock the session.
	pub, priv := genAtRestKeyPair(t)
	svc.SetSessionGate(&stubKeySessionGate{ids: []string{}, pub: pub, priv: priv})

	// Reads still work.
	if r := svc.List(subject.ListInput{}); !r.OK {
		t.Fatalf("List while locked MUST succeed: %s", r.Error())
	}
	if r := svc.Search(subject.SearchInput{Query: "auth"}); !r.OK {
		t.Fatalf("Search while locked MUST succeed: %s", r.Error())
	}

	// MarkRehearsed (sole writer) rejects when locked.
	r := svc.MarkRehearsed(subject.MarkInput{ID: "R-01"})
	if r.OK {
		t.Fatal("MarkRehearsed MUST reject when session is locked")
	}
	if !core.Contains(r.Error(), "session.locked") {
		t.Fatalf("expected session.locked, got %q", r.Error())
	}
}
