// SPDX-Licence-Identifier: EUPL-1.2

// internal_test.go — white-box cover for service.go's unexported
// helpers (Register, contentDir, slugifyContent, isStrictYYYYMM,
// headerPubKey, loadHeaderOnly, parseAtRestRecord, parseItem, writeItem
// / writeItemLegacy / writeItemAtRest, atrestWriterFor, fireContentEvent).
// Lives in package content (mirrors pkg/sales/deals/service_internal_
// test.go's precedent) because these symbols are not reachable from the
// black-box content_test package.

package content

import (
	"os"
	"testing"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/account"
	"dappco.re/lthn/desktop/pkg/recordfile"
	"github.com/Snider/Enchantrix/pkg/crypt/std/pgp"
)

// --- Register ---------------------------------------------------------

func TestService_Register_Good(t *testing.T) {
	r := Register(nil)
	if !r.OK {
		t.Fatalf("Register failed: %s", r.Error())
	}
	if _, ok := r.Value.(*Service); !ok {
		t.Fatalf("Register value = %T, want *Service", r.Value)
	}
}

// --- contentDir ---------------------------------------------------------

// TestContentDir_HomeUnavailable_Bad — os.UserHomeDir() (wrapped by
// core.UserHomeDir) rejects an empty $HOME on darwin/linux, so
// contentDir's `if !root.OK { return root }` early-return is reachable
// hermetically by unsetting HOME rather than needing a real broken
// environment.
func TestContentDir_HomeUnavailable_Bad(t *testing.T) {
	t.Setenv("HOME", "")
	r := contentDir()
	if r.OK {
		t.Fatal("contentDir() must fail when $HOME is unavailable")
	}
}

// TestContentDir_MarketingIsFile_Bad — paths.Root() succeeds but the
// MkdirAll(dir, 0o700) for marketing/content fails because "marketing"
// already exists as a regular file blocking the subdirectory create.
func TestContentDir_MarketingIsFile_Bad(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	letheanDir := core.PathJoin(home, "Lethean")
	if mk := core.MkdirAll(letheanDir, 0o755); !mk.OK {
		t.Fatalf("seed MkdirAll: %s", mk.Error())
	}
	blocker := core.PathJoin(letheanDir, "marketing")
	if w := core.WriteFile(blocker, []byte("blocking file"), 0o600); !w.OK {
		t.Fatalf("seed blocking file: %s", w.Error())
	}
	r := contentDir()
	if r.OK {
		t.Fatal("contentDir() must fail when marketing/ is blocked by a file")
	}
}

// --- slugifyContent -----------------------------------------------------

func TestSlugifyContent_TrailingSpaceTrimmed_Good(t *testing.T) {
	if got := slugifyContent("Foo bar "); got != "foo-bar" {
		t.Fatalf("slugifyContent(trailing space) = %q, want foo-bar", got)
	}
}

func TestSlugifyContent_AllSymbols_Ugly(t *testing.T) {
	if got := slugifyContent("!!!"); got != "" {
		t.Fatalf("slugifyContent(!!!) = %q, want empty", got)
	}
}

func TestSlugifyContent_MixedCase_Good(t *testing.T) {
	if got := slugifyContent("v0.2 Release_Notes"); got != "v02-release-notes" {
		t.Fatalf("slugifyContent = %q, want v02-release-notes", got)
	}
}

// --- isStrictYYYYMM -----------------------------------------------------

func TestIsStrictYYYYMM_Valid_Good(t *testing.T) {
	for _, s := range []string{"2026-01", "2026-12", "0000-06"} {
		if !isStrictYYYYMM(s) {
			t.Fatalf("isStrictYYYYMM(%s) = false, want true", s)
		}
	}
}

func TestIsStrictYYYYMM_WrongLength_Bad(t *testing.T) {
	for _, s := range []string{"", "2026", "2026-1", "2026-001", "2026/06"} {
		if isStrictYYYYMM(s) {
			t.Fatalf("isStrictYYYYMM(%q) = true, want false", s)
		}
	}
}

func TestIsStrictYYYYMM_NonDigitYear_Bad(t *testing.T) {
	if isStrictYYYYMM("20a6-06") {
		t.Fatal("isStrictYYYYMM with non-digit year must be false")
	}
}

func TestIsStrictYYYYMM_MonthOutOfRange_Bad(t *testing.T) {
	for _, s := range []string{"2026-00", "2026-13", "2026-99", "2026-2x"} {
		if isStrictYYYYMM(s) {
			t.Fatalf("isStrictYYYYMM(%q) = true, want false", s)
		}
	}
}

// --- atrestWriterFor ------------------------------------------------

// TestAtrestWriterFor_NilReceiver_Bad — a nil *Service must degrade
// gracefully (nil, false) rather than panicking; the nil-receiver guard
// is the function's first statement.
func TestAtrestWriterFor_NilReceiver_Bad(t *testing.T) {
	var s *Service
	w, ok := s.atrestWriterFor()
	if ok || w != nil {
		t.Fatalf("atrestWriterFor on nil receiver = (%v, %v), want (nil, false)", w, ok)
	}
}

// --- headerPubKey ---------------------------------------------------

func TestHeaderPubKey_NilGate_Bad(t *testing.T) {
	s := NewService(nil)
	_, err := s.headerPubKey([]byte("whatever"))
	if err == nil {
		t.Fatal("headerPubKey with nil gate must error")
	}
}

// minimalGateForHeaderTest satisfies SessionGate only — no
// PublicKeyFor/PrivateKeyFor — so the accountKeyProvider assertion in
// headerPubKey fails.
type minimalGateForHeaderTest struct{}

func (minimalGateForHeaderTest) UnlockedAccountIDs() []string { return []string{"x"} }

func TestHeaderPubKey_GateNotKeyProvider_Bad(t *testing.T) {
	s := NewService(nil)
	s.SetSessionGate(minimalGateForHeaderTest{})
	_, err := s.headerPubKey([]byte("whatever"))
	if err == nil {
		t.Fatal("headerPubKey with a non-keyed gate must error")
	}
	if !core.Contains(err.Error(), "does not provide account keys") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// stubKeyedGate is a minimal accountKeyProvider-satisfying gate for
// direct headerPubKey / loadHeaderOnly unit tests (avoids pulling in
// the real PGP keypair machinery — headerPubKey never touches
// PrivateKeyFor). PrivateKeyFor returns (nil, false) via
// NewPrivateKeyHandleForTest's absence so the accountKeyProvider
// interface is satisfied exactly.
type stubKeyedGate struct {
	ids []string
	pub []byte
}

func (g *stubKeyedGate) UnlockedAccountIDs() []string { return g.ids }
func (g *stubKeyedGate) PublicKeyFor(_ string) ([]byte, bool) {
	if len(g.pub) == 0 {
		return nil, false
	}
	return g.pub, true
}
func (g *stubKeyedGate) PrivateKeyFor(_ string) (*account.PrivateKeyHandle, bool) {
	return nil, false
}

func TestHeaderPubKey_MalformedBlob_Bad(t *testing.T) {
	s := NewService(nil)
	s.SetSessionGate(&stubKeyedGate{ids: []string{"x"}, pub: []byte("pub")})
	_, err := s.headerPubKey([]byte("x")) // too short for PeekAccountID
	if err == nil {
		t.Fatal("headerPubKey with a too-short blob must error")
	}
}

func TestHeaderPubKey_PublicKeyNotFound_Bad(t *testing.T) {
	s := NewService(nil)
	s.SetSessionGate(&stubKeyedGate{ids: []string{"x"}}) // no pub set
	raw := buildHeaderOnlyBlob(t, `{"account":{"id":"acct-missing"}}`)
	_, err := s.headerPubKey(raw)
	if err == nil {
		t.Fatal("headerPubKey must error when PublicKeyFor reports not-ok")
	}
	if !core.Contains(err.Error(), "returned not-ok") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHeaderPubKey_Success_Good(t *testing.T) {
	s := NewService(nil)
	s.SetSessionGate(&stubKeyedGate{ids: []string{"x"}, pub: []byte("the-pub-key")})
	raw := buildHeaderOnlyBlob(t, `{"account":{"id":"acct-1"}}`)
	pub, err := s.headerPubKey(raw)
	if err != nil {
		t.Fatalf("headerPubKey: %v", err)
	}
	if string(pub) != "the-pub-key" {
		t.Fatalf("headerPubKey pub = %q, want the-pub-key", pub)
	}
}

// buildHeaderOnlyBlob assembles a minimal Trix at-rest envelope shape
// `[Magic(4)][Version(1)][HeaderLen(4 BE)][HeaderJSON]` — enough for
// recordfile.PeekAccountID to extract account.id without needing a
// real PGP-sealed payload.
func buildHeaderOnlyBlob(t *testing.T, headerJSON string) []byte {
	t.Helper()
	hdr := []byte(headerJSON)
	n := len(hdr)
	out := make([]byte, 0, 9+n)
	out = append(out, 'L', 'T', 'H', 'N', 1)
	out = append(out, byte(n>>24), byte(n>>16), byte(n>>8), byte(n))
	out = append(out, hdr...)
	return out
}

// --- loadHeaderOnly ---------------------------------------------------

func TestLoadHeaderOnly_GateNotWired_Bad(t *testing.T) {
	s := NewService(nil)
	_, err := s.loadHeaderOnly("/does/not/matter")
	if err == nil {
		t.Fatal("loadHeaderOnly with unwired gate must error")
	}
	if !core.Contains(err.Error(), "session gate not wired") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadHeaderOnly_FileMissing_Bad(t *testing.T) {
	s := NewService(nil)
	s.SetSessionGate(&stubKeyedGate{ids: []string{"x"}, pub: []byte("pub")})
	_, err := s.loadHeaderOnly(core.PathJoin(t.TempDir(), "missing.lthn"))
	if err == nil {
		t.Fatal("loadHeaderOnly on a missing file must error")
	}
}

func TestLoadHeaderOnly_HeaderPubKeyFails_Bad(t *testing.T) {
	dir := t.TempDir()
	fp := core.PathJoin(dir, "bad.lthn")
	// Well-formed enough to pass ReadFile but too short for
	// PeekAccountID inside headerPubKey.
	if w := core.WriteFile(fp, []byte("junk"), 0o600); !w.OK {
		t.Fatalf("seed WriteFile: %s", w.Error())
	}
	s := NewService(nil)
	s.SetSessionGate(&stubKeyedGate{ids: []string{"x"}, pub: []byte("pub")})
	_, err := s.loadHeaderOnly(fp)
	if err == nil {
		t.Fatal("loadHeaderOnly must propagate headerPubKey's failure")
	}
}

func TestLoadHeaderOnly_DecodeHeaderRejects_Bad(t *testing.T) {
	dir := t.TempDir()
	fp := core.PathJoin(dir, "bad2.lthn")
	blob := buildHeaderOnlyBlob(t, `{"account":{"id":"acct-1"}}`)
	if w := core.WriteFile(fp, blob, 0o600); !w.OK {
		t.Fatalf("seed WriteFile: %s", w.Error())
	}
	s := NewService(nil)
	s.SetSessionGate(&stubKeyedGate{ids: []string{"x"}, pub: []byte("pub")})
	_, err := s.loadHeaderOnly(fp)
	if err == nil {
		t.Fatal("loadHeaderOnly must reject a header-only blob missing a MAC/payload")
	}
}

// --- parseAtRestRecord ------------------------------------------------

func TestParseAtRestRecord_BadYAML_Bad(t *testing.T) {
	_, err := parseAtRestRecord(recordfile.ReadResult{
		BodyYAML: []byte("---\n[not: valid: yaml\n"),
	})
	if err == nil {
		t.Fatal("parseAtRestRecord with malformed YAML frontmatter must error")
	}
}

func TestParseAtRestRecord_HeaderVersionWins_Good(t *testing.T) {
	item, err := parseAtRestRecord(recordfile.ReadResult{
		BodyYAML: []byte("id: x\nversion: 1\n"),
		Header:   recordfile.Header{Version: 7},
	})
	if err != nil {
		t.Fatalf("parseAtRestRecord: %v", err)
	}
	if item.Version != 7 {
		t.Fatalf("expected header version (7) to win over frontmatter version (1), got %d", item.Version)
	}
}

// --- parseItem ----------------------------------------------------------

// TestParseItem_NoLeadingDelimiter_Good — a legacy .md file that does
// not open with the "---\n" delimiter still parses as long as its
// content is valid YAML (the whole blob becomes the frontmatter, body
// stays empty).
func TestParseItem_NoLeadingDelimiter_Good(t *testing.T) {
	item, err := parseItem([]byte("id: no-delim\nt: Title\n"))
	if err != nil {
		t.Fatalf("parseItem: %v", err)
	}
	if item.ID != "no-delim" || item.T != "Title" {
		t.Fatalf("unexpected item: %+v", item)
	}
}

func TestParseItem_BadYAML_Bad(t *testing.T) {
	_, err := parseItem([]byte("---\n[not: valid: yaml\n---\nbody"))
	if err == nil {
		t.Fatal("parseItem with malformed YAML frontmatter must error")
	}
}

// --- writeItem / writeItemLegacy ---------------------------------------

func TestWriteItem_InvalidID_Bad(t *testing.T) {
	s := NewService(nil)
	r := s.writeItem(t.TempDir(), ContentItem{ID: "../evil"}, 0)
	if r.OK {
		t.Fatal("writeItem with a path-traversal ID must reject")
	}
}

// TestWriteItem_NegativeIfVersion_ClampsToOne_Good — ifVersion <= -1
// drives nextVersion below 1; writeItem clamps it back to 1 before
// stamping the frontmatter.
func TestWriteItem_NegativeIfVersion_ClampsToOne_Good(t *testing.T) {
	dir := t.TempDir()
	s := NewService(nil)
	r := s.writeItem(dir, ContentItem{ID: "clamp-item", T: "x", Col: "idea"}, -5)
	if !r.OK {
		t.Fatalf("writeItem: %s", r.Error())
	}
	fpath := core.PathJoin(dir, "clamp-item.md")
	raw := core.ReadFile(fpath)
	if !raw.OK {
		t.Fatalf("ReadFile: %s", raw.Error())
	}
	if !core.Contains(string(raw.Value.([]byte)), "version: 1") {
		t.Fatalf("expected clamped version: 1 in frontmatter, got: %s", raw.Value.([]byte))
	}
}

func TestWriteItemLegacy_InvalidID_Bad(t *testing.T) {
	r := writeItemLegacy(t.TempDir(), ContentItem{ID: "../evil"}, 0)
	if r.OK {
		t.Fatal("writeItemLegacy with a path-traversal ID must reject")
	}
}

func TestWriteItemLegacy_NegativeIfVersion_ClampsToOne_Good(t *testing.T) {
	dir := t.TempDir()
	r := writeItemLegacy(dir, ContentItem{ID: "clamp-legacy", T: "x", Col: "idea"}, -9)
	if !r.OK {
		t.Fatalf("writeItemLegacy: %s", r.Error())
	}
	fpath := core.PathJoin(dir, "clamp-legacy.md")
	raw := core.ReadFile(fpath)
	if !raw.OK {
		t.Fatalf("ReadFile: %s", raw.Error())
	}
	if !core.Contains(string(raw.Value.([]byte)), "version: 1") {
		t.Fatalf("expected clamped version: 1 in frontmatter, got: %s", raw.Value.([]byte))
	}
}

// TestWriteItemLegacy_WithBody_Good — writeItemLegacy's body-append
// branch (`if item.Body != ""`) is only reachable when the caller
// supplies non-empty Body over the legacy plaintext path.
func TestWriteItemLegacy_WithBody_Good(t *testing.T) {
	dir := t.TempDir()
	r := writeItemLegacy(dir, ContentItem{ID: "with-body", T: "x", Col: "idea", Body: "## Outline\nBody text."}, 0)
	if !r.OK {
		t.Fatalf("writeItemLegacy: %s", r.Error())
	}
	fpath := core.PathJoin(dir, "with-body.md")
	raw := core.ReadFile(fpath)
	if !raw.OK {
		t.Fatalf("ReadFile: %s", raw.Error())
	}
	if !core.Contains(string(raw.Value.([]byte)), "Body text.") {
		t.Fatalf("expected body text persisted, got: %s", raw.Value.([]byte))
	}
}

// --- writeItemAtRest ------------------------------------------------

// genInternalKeyPair builds a real PGP keypair for the internal
// writeItemAtRest tests (mirrors atrest_test.go's genAtRestKeyPair,
// duplicated here since content_test's helper lives in the external
// test package).
func genInternalKeyPair(t *testing.T) (pub, priv []byte) {
	t.Helper()
	svc := pgp.NewService()
	p, k, err := svc.GenerateKeyPair("Test", "test@lthn.local", "test")
	if err != nil {
		t.Fatalf("generate test key pair: %v", err)
	}
	return p, k
}

type stubFullKeyedGate struct {
	ids  []string
	pub  []byte
	priv []byte
}

func (g *stubFullKeyedGate) UnlockedAccountIDs() []string { return g.ids }
func (g *stubFullKeyedGate) PublicKeyFor(_ string) ([]byte, bool) {
	if len(g.pub) == 0 {
		return nil, false
	}
	return g.pub, true
}
func (g *stubFullKeyedGate) PrivateKeyFor(_ string) (*account.PrivateKeyHandle, bool) {
	if len(g.priv) == 0 {
		return nil, false
	}
	return account.NewPrivateKeyHandleForTest(g.priv), true
}

// TestWriteItemAtRest_JoinAndCheckRejects_Bad — writeItemAtRest is
// called from writeItem AFTER paths.IsValidID has already vetted
// item.ID; calling it directly bypasses that guard so its own
// paths.JoinAndCheck defence is exercised in isolation.
func TestWriteItemAtRest_JoinAndCheckRejects_Bad(t *testing.T) {
	pub, priv := genInternalKeyPair(t)
	s := NewService(nil)
	s.SetSessionGate(&stubFullKeyedGate{ids: []string{"acct-1"}, pub: pub, priv: priv})
	w, ok := s.atrestWriterFor()
	if !ok {
		t.Fatal("atrestWriterFor must succeed against a fully-keyed gate")
	}
	r := s.writeItemAtRest(w, ContentItem{ID: "../evil", T: "x"}, t.TempDir())
	if r.OK {
		t.Fatal("writeItemAtRest with a path-traversal ID must reject")
	}
}

// TestWriteItemAtRest_PriorHashFromExistingFile_Good — a second write
// to the same target path finds the prior ciphertext on disk and folds
// its SHA-256 into the IfMatch hash (the `if existing := core.ReadFile
// (target); existing.OK` branch).
func TestWriteItemAtRest_PriorHashFromExistingFile_Good(t *testing.T) {
	pub, priv := genInternalKeyPair(t)
	s := NewService(nil)
	s.SetSessionGate(&stubFullKeyedGate{ids: []string{"acct-1"}, pub: pub, priv: priv})
	w, ok := s.atrestWriterFor()
	if !ok {
		t.Fatal("atrestWriterFor must succeed against a fully-keyed gate")
	}
	dir := t.TempDir()
	item := ContentItem{ID: "repeat-write", T: "first", Col: "idea", Version: 1}
	r1 := s.writeItemAtRest(w, item, dir)
	if !r1.OK {
		t.Fatalf("first writeItemAtRest: %s", r1.Error())
	}
	item.T = "second"
	item.Version = 2
	r2 := s.writeItemAtRest(w, item, dir)
	if !r2.OK {
		t.Fatalf("second writeItemAtRest (prior-hash branch): %s", r2.Error())
	}
}

// --- fireContentEvent -----------------------------------------------

func TestFireContentEvent_NilReceiver_Good(t *testing.T) {
	var s *Service
	// Must not panic — the nil-receiver guard is the first statement.
	s.fireContentEvent(EventContentCreated, "x", "idea")
}

func TestFireContentEvent_NilCore_Good(t *testing.T) {
	s := NewService(nil)
	// Must not panic — s.core == nil short-circuits before ACTION.
	s.fireContentEvent(EventContentCreated, "x", "idea")
}

func TestFireContentEvent_PublishesOnCoreBus_Good(t *testing.T) {
	c := core.New()
	var got core.Message
	c.RegisterAction(func(_ *core.Core, msg core.Message) core.Result {
		got = msg
		return core.Result{OK: true}
	})
	s := NewService(c)
	s.fireContentEvent(EventContentAdvanced, "item-1", "draft")

	ev, ok := got.(ContentEvent)
	if !ok {
		t.Fatalf("expected ContentEvent on the ACTION bus, got %T", got)
	}
	if ev.ItemID != "item-1" || ev.Col != "draft" || ev.EventName != EventContentAdvanced {
		t.Fatalf("unexpected ContentEvent payload: %+v", ev)
	}
}

// --- loadItems / loadOne (dir-level fault injection) ---------------

// TestLoadItems_ContentDirFails_Bad — loadItems forwards contentDir's
// failure when $HOME is unavailable.
func TestLoadItems_ContentDirFails_Bad(t *testing.T) {
	t.Setenv("HOME", "")
	s := NewService(nil)
	_, err := s.loadItems()
	if err == nil {
		t.Fatal("loadItems must error when contentDir() fails")
	}
}

// TestLoadItems_UnreadableDir_ReturnsNilNil_Good — ReadDir failing on
// an existing-but-unreadable directory hits the `if !entriesR.OK {
// return nil, nil }` branch (distinct from contentDir failing outright).
func TestLoadItems_UnreadableDir_ReturnsNilNil_Good(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root — permission-denial fault injection does not apply")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := core.PathJoin(home, "Lethean", "marketing", "content")
	if mk := core.MkdirAll(dir, 0o700); !mk.OK {
		t.Fatalf("seed MkdirAll: %s", mk.Error())
	}
	if err := os.Chmod(dir, 0o000); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	s := NewService(nil)
	items, err := s.loadItems()
	if err != nil {
		t.Fatalf("loadItems on unreadable dir must return (nil, nil), got err: %v", err)
	}
	if items != nil {
		t.Fatalf("expected nil items, got %+v", items)
	}
}

// TestLoadOne_JoinAndCheckRejectsLthn_Bad — loadOne's first
// paths.JoinAndCheck call (for the .lthn candidate path) is reachable
// directly with an ID that passes paths.IsValidID but is otherwise
// awkward — pinned alongside the exported Get traversal cover in
// security_test.go which exercises the IsValidID guard itself.
func TestLoadOne_EmptyDir_NotFound_Bad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	s := NewService(nil)
	_, _, err := s.loadOne("does-not-exist")
	if err == nil {
		t.Fatal("loadOne on a missing id must error")
	}
}
