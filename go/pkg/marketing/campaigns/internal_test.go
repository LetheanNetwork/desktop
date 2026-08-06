// SPDX-Licence-Identifier: EUPL-1.2

// internal_test.go — white-box cover for service.go's unexported
// helpers (Register, campaignsDir, slugify, headerPubKey,
// loadHeaderOnly, parseAtRestRecord, parseCampaign, writeCampaign /
// writeCampaignLegacy / writeCampaignAtRest, atrestWriterFor,
// fireCampaignEvent). Lives in package campaigns (mirrors pkg/sales/
// deals/service_internal_test.go's precedent) because these symbols are
// not reachable from the black-box campaigns_test package.

package campaigns

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

// --- campaignsDir ---------------------------------------------------------

func TestCampaignsDir_HomeUnavailable_Bad(t *testing.T) {
	t.Setenv("HOME", "")
	r := campaignsDir()
	if r.OK {
		t.Fatal("campaignsDir() must fail when $HOME is unavailable")
	}
}

func TestCampaignsDir_MarketingIsFile_Bad(t *testing.T) {
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
	r := campaignsDir()
	if r.OK {
		t.Fatal("campaignsDir() must fail when marketing/ is blocked by a file")
	}
}

// --- slugify -----------------------------------------------------

func TestSlugify_TrailingSpaceTrimmed_Good(t *testing.T) {
	if got := slugify("Foo bar "); got != "foo-bar" {
		t.Fatalf("slugify(trailing space) = %q, want foo-bar", got)
	}
}

func TestSlugify_AllSymbols_Ugly(t *testing.T) {
	if got := slugify("!!!"); got != "" {
		t.Fatalf("slugify(!!!) = %q, want empty", got)
	}
}

func TestSlugify_MixedCase_Good(t *testing.T) {
	if got := slugify("Product Hunt_Launch"); got != "product-hunt-launch" {
		t.Fatalf("slugify = %q, want product-hunt-launch", got)
	}
}

// --- atrestWriterFor ------------------------------------------------

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
	c, err := parseAtRestRecord(recordfile.ReadResult{
		BodyYAML: []byte("id: x\nversion: 1\n"),
		Header:   recordfile.Header{Version: 7},
	})
	if err != nil {
		t.Fatalf("parseAtRestRecord: %v", err)
	}
	if c.Version != 7 {
		t.Fatalf("expected header version (7) to win over frontmatter version (1), got %d", c.Version)
	}
}

// --- parseCampaign ----------------------------------------------------------

func TestParseCampaign_NoLeadingDelimiter_Good(t *testing.T) {
	c, err := parseCampaign([]byte("id: no-delim\nname: Title\n"))
	if err != nil {
		t.Fatalf("parseCampaign: %v", err)
	}
	if c.ID != "no-delim" || c.Name != "Title" {
		t.Fatalf("unexpected campaign: %+v", c)
	}
}

func TestParseCampaign_BadYAML_Bad(t *testing.T) {
	_, err := parseCampaign([]byte("---\n[not: valid: yaml\n---\nbody"))
	if err == nil {
		t.Fatal("parseCampaign with malformed YAML frontmatter must error")
	}
}

// --- writeCampaign / writeCampaignLegacy ---------------------------------------

func TestWriteCampaign_InvalidID_Bad(t *testing.T) {
	s := NewService(nil)
	r := s.writeCampaign(t.TempDir(), Campaign{ID: "../evil"}, 0)
	if r.OK {
		t.Fatal("writeCampaign with a path-traversal ID must reject")
	}
}

func TestWriteCampaign_NegativeIfVersion_ClampsToOne_Good(t *testing.T) {
	dir := t.TempDir()
	s := NewService(nil)
	r := s.writeCampaign(dir, Campaign{ID: "clamp-campaign", Name: "x", Channel: "earned"}, -5)
	if !r.OK {
		t.Fatalf("writeCampaign: %s", r.Error())
	}
	fpath := core.PathJoin(dir, "clamp-campaign.md")
	raw := core.ReadFile(fpath)
	if !raw.OK {
		t.Fatalf("ReadFile: %s", raw.Error())
	}
	if !core.Contains(string(raw.Value.([]byte)), "version: 1") {
		t.Fatalf("expected clamped version: 1 in frontmatter, got: %s", raw.Value.([]byte))
	}
}

func TestWriteCampaignLegacy_InvalidID_Bad(t *testing.T) {
	r := writeCampaignLegacy(t.TempDir(), Campaign{ID: "../evil"}, 0)
	if r.OK {
		t.Fatal("writeCampaignLegacy with a path-traversal ID must reject")
	}
}

func TestWriteCampaignLegacy_NegativeIfVersion_ClampsToOne_Good(t *testing.T) {
	dir := t.TempDir()
	r := writeCampaignLegacy(dir, Campaign{ID: "clamp-legacy", Name: "x", Channel: "earned"}, -9)
	if !r.OK {
		t.Fatalf("writeCampaignLegacy: %s", r.Error())
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

func TestWriteCampaignLegacy_WithBody_Good(t *testing.T) {
	dir := t.TempDir()
	r := writeCampaignLegacy(dir, Campaign{ID: "with-body", Name: "x", Channel: "earned", Body: "## Outline\nBody text."}, 0)
	if !r.OK {
		t.Fatalf("writeCampaignLegacy: %s", r.Error())
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

// --- writeCampaignAtRest ------------------------------------------------

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

func TestWriteCampaignAtRest_JoinAndCheckRejects_Bad(t *testing.T) {
	pub, priv := genInternalKeyPair(t)
	s := NewService(nil)
	s.SetSessionGate(&stubFullKeyedGate{ids: []string{"acct-1"}, pub: pub, priv: priv})
	w, ok := s.atrestWriterFor()
	if !ok {
		t.Fatal("atrestWriterFor must succeed against a fully-keyed gate")
	}
	r := s.writeCampaignAtRest(w, Campaign{ID: "../evil", Name: "x"}, t.TempDir())
	if r.OK {
		t.Fatal("writeCampaignAtRest with a path-traversal ID must reject")
	}
}

func TestWriteCampaignAtRest_PriorHashFromExistingFile_Good(t *testing.T) {
	pub, priv := genInternalKeyPair(t)
	s := NewService(nil)
	s.SetSessionGate(&stubFullKeyedGate{ids: []string{"acct-1"}, pub: pub, priv: priv})
	w, ok := s.atrestWriterFor()
	if !ok {
		t.Fatal("atrestWriterFor must succeed against a fully-keyed gate")
	}
	dir := t.TempDir()
	c := Campaign{ID: "repeat-write", Name: "first", Channel: "earned", Version: 1}
	r1 := s.writeCampaignAtRest(w, c, dir)
	if !r1.OK {
		t.Fatalf("first writeCampaignAtRest: %s", r1.Error())
	}
	c.Name = "second"
	c.Version = 2
	r2 := s.writeCampaignAtRest(w, c, dir)
	if !r2.OK {
		t.Fatalf("second writeCampaignAtRest (prior-hash branch): %s", r2.Error())
	}
}

// --- fireCampaignEvent -----------------------------------------------

func TestFireCampaignEvent_NilReceiver_Good(t *testing.T) {
	var s *Service
	s.fireCampaignEvent(EventCampaignCreated, "x")
}

func TestFireCampaignEvent_NilCore_Good(t *testing.T) {
	s := NewService(nil)
	s.fireCampaignEvent(EventCampaignCreated, "x")
}

func TestFireCampaignEvent_PublishesOnCoreBus_Good(t *testing.T) {
	c := core.New()
	var got core.Message
	c.RegisterAction(func(_ *core.Core, msg core.Message) core.Result {
		got = msg
		return core.Result{OK: true}
	})
	s := NewService(c)
	s.fireCampaignEvent(EventCampaignUpdated, "campaign-1")

	ev, ok := got.(CampaignEvent)
	if !ok {
		t.Fatalf("expected CampaignEvent on the ACTION bus, got %T", got)
	}
	if ev.CampaignID != "campaign-1" || ev.EventName != EventCampaignUpdated {
		t.Fatalf("unexpected CampaignEvent payload: %+v", ev)
	}
}

// --- loadCampaigns / loadOne (dir-level fault injection) ---------------

func TestLoadCampaigns_CampaignsDirFails_Bad(t *testing.T) {
	t.Setenv("HOME", "")
	s := NewService(nil)
	_, err := s.loadCampaigns()
	if err == nil {
		t.Fatal("loadCampaigns must error when campaignsDir() fails")
	}
}

func TestLoadCampaigns_UnreadableDir_ReturnsNilNil_Good(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root — permission-denial fault injection does not apply")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := core.PathJoin(home, "Lethean", "marketing", "campaigns")
	if mk := core.MkdirAll(dir, 0o700); !mk.OK {
		t.Fatalf("seed MkdirAll: %s", mk.Error())
	}
	if err := os.Chmod(dir, 0o000); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	s := NewService(nil)
	cs, err := s.loadCampaigns()
	if err != nil {
		t.Fatalf("loadCampaigns on unreadable dir must return (nil, nil), got err: %v", err)
	}
	if cs != nil {
		t.Fatalf("expected nil campaigns, got %+v", cs)
	}
}

func TestLoadOne_EmptyDir_NotFound_Bad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	s := NewService(nil)
	_, _, err := s.loadOne("does-not-exist")
	if err == nil {
		t.Fatal("loadOne on a missing id must error")
	}
}
