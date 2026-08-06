// SPDX-Licence-Identifier: EUPL-1.2

// internal_test.go — white-box cover for service.go's unexported
// helpers (Register, socialDir, splitChannels, trimSpace, joinChannels,
// headerPubKey, loadHeaderOnly, parseAtRestRecord, parsePost, writePost
// / writePostLegacy / writePostAtRest, atrestWriterFor, containsChannel,
// fireSocialEvent). Lives in package social (mirrors pkg/sales/deals/
// service_internal_test.go's precedent) because these symbols are not
// reachable from the black-box social_test package.

package social

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

// --- socialDir ---------------------------------------------------------

func TestSocialDir_HomeUnavailable_Bad(t *testing.T) {
	t.Setenv("HOME", "")
	r := socialDir()
	if r.OK {
		t.Fatal("socialDir() must fail when $HOME is unavailable")
	}
}

func TestSocialDir_MarketingIsFile_Bad(t *testing.T) {
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
	r := socialDir()
	if r.OK {
		t.Fatal("socialDir() must fail when marketing/ is blocked by a file")
	}
}

// --- splitChannels / trimSpace / joinChannels --------------------------

func TestSplitChannels_Empty_Good(t *testing.T) {
	if got := splitChannels(""); got != nil {
		t.Fatalf("splitChannels(\"\") = %v, want nil", got)
	}
}

func TestSplitChannels_TrimsSpacesAroundEach_Good(t *testing.T) {
	got := splitChannels(" mastodon , x ,  bluesky  ")
	want := []string{"mastodon", "x", "bluesky"}
	if len(got) != len(want) {
		t.Fatalf("splitChannels = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("splitChannels[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestTrimSpace_LeadingAndTrailing_Good(t *testing.T) {
	if got := trimSpace("  hello  "); got != "hello" {
		t.Fatalf("trimSpace = %q, want hello", got)
	}
}

func TestTrimSpace_NoSpaces_Good(t *testing.T) {
	if got := trimSpace("hello"); got != "hello" {
		t.Fatalf("trimSpace(no-op) = %q, want hello", got)
	}
}

func TestTrimSpace_AllSpaces_Ugly(t *testing.T) {
	if got := trimSpace("   "); got != "" {
		t.Fatalf("trimSpace(all-spaces) = %q, want empty", got)
	}
}

func TestJoinChannels_Empty_Good(t *testing.T) {
	if got := joinChannels(nil); got != "" {
		t.Fatalf("joinChannels(nil) = %q, want empty", got)
	}
}

func TestJoinChannels_Multiple_Good(t *testing.T) {
	if got := joinChannels([]string{"mastodon", "x"}); got != "mastodon,x" {
		t.Fatalf("joinChannels = %q, want mastodon,x", got)
	}
}

// --- containsChannel ------------------------------------------------

func TestContainsChannel_Match_Good(t *testing.T) {
	if !containsChannel([]string{"mastodon", "x"}, "x") {
		t.Fatal("containsChannel must find x")
	}
}

func TestContainsChannel_NoMatch_Bad(t *testing.T) {
	if containsChannel([]string{"mastodon"}, "bluesky") {
		t.Fatal("containsChannel must not find bluesky")
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
	p, err := parseAtRestRecord(recordfile.ReadResult{
		BodyYAML: []byte("id: x\nversion: 1\n"),
		Header:   recordfile.Header{Version: 7},
	})
	if err != nil {
		t.Fatalf("parseAtRestRecord: %v", err)
	}
	if p.Version != 7 {
		t.Fatalf("expected header version (7) to win over frontmatter version (1), got %d", p.Version)
	}
}

// --- parsePost ----------------------------------------------------------

func TestParsePost_NoLeadingDelimiter_Good(t *testing.T) {
	p, err := parsePost([]byte("id: no-delim\nch: mastodon\n"))
	if err != nil {
		t.Fatalf("parsePost: %v", err)
	}
	if p.ID != "no-delim" || len(p.Ch) != 1 || p.Ch[0] != "mastodon" {
		t.Fatalf("unexpected post: %+v", p)
	}
}

func TestParsePost_BadYAML_Bad(t *testing.T) {
	_, err := parsePost([]byte("---\n[not: valid: yaml\n---\nbody"))
	if err == nil {
		t.Fatal("parsePost with malformed YAML frontmatter must error")
	}
}

// --- writePost / writePostLegacy ---------------------------------------

func TestWritePost_InvalidID_Bad(t *testing.T) {
	s := NewService(nil)
	r := s.writePost(t.TempDir(), SocialPost{ID: "../evil"}, 0)
	if r.OK {
		t.Fatal("writePost with a path-traversal ID must reject")
	}
}

func TestWritePost_NegativeIfVersion_ClampsToOne_Good(t *testing.T) {
	dir := t.TempDir()
	s := NewService(nil)
	r := s.writePost(dir, SocialPost{ID: "clamp-post", Ch: []string{"x"}, Text: "hi"}, -5)
	if !r.OK {
		t.Fatalf("writePost: %s", r.Error())
	}
	fpath := core.PathJoin(dir, "clamp-post.md")
	raw := core.ReadFile(fpath)
	if !raw.OK {
		t.Fatalf("ReadFile: %s", raw.Error())
	}
	if !core.Contains(string(raw.Value.([]byte)), "version: 1") {
		t.Fatalf("expected clamped version: 1 in frontmatter, got: %s", raw.Value.([]byte))
	}
}

func TestWritePostLegacy_InvalidID_Bad(t *testing.T) {
	r := writePostLegacy(t.TempDir(), SocialPost{ID: "../evil"}, 0)
	if r.OK {
		t.Fatal("writePostLegacy with a path-traversal ID must reject")
	}
}

func TestWritePostLegacy_NegativeIfVersion_ClampsToOne_Good(t *testing.T) {
	dir := t.TempDir()
	r := writePostLegacy(dir, SocialPost{ID: "clamp-legacy", Ch: []string{"x"}, Text: "hi"}, -9)
	if !r.OK {
		t.Fatalf("writePostLegacy: %s", r.Error())
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

func TestWritePostLegacy_WithText_Good(t *testing.T) {
	dir := t.TempDir()
	r := writePostLegacy(dir, SocialPost{ID: "with-text", Ch: []string{"x"}, Text: "Announcement body."}, 0)
	if !r.OK {
		t.Fatalf("writePostLegacy: %s", r.Error())
	}
	fpath := core.PathJoin(dir, "with-text.md")
	raw := core.ReadFile(fpath)
	if !raw.OK {
		t.Fatalf("ReadFile: %s", raw.Error())
	}
	if !core.Contains(string(raw.Value.([]byte)), "Announcement body.") {
		t.Fatalf("expected text persisted, got: %s", raw.Value.([]byte))
	}
}

// --- writePostAtRest ------------------------------------------------

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

func TestWritePostAtRest_JoinAndCheckRejects_Bad(t *testing.T) {
	pub, priv := genInternalKeyPair(t)
	s := NewService(nil)
	s.SetSessionGate(&stubFullKeyedGate{ids: []string{"acct-1"}, pub: pub, priv: priv})
	w, ok := s.atrestWriterFor()
	if !ok {
		t.Fatal("atrestWriterFor must succeed against a fully-keyed gate")
	}
	r := s.writePostAtRest(w, SocialPost{ID: "../evil", Ch: []string{"x"}}, t.TempDir())
	if r.OK {
		t.Fatal("writePostAtRest with a path-traversal ID must reject")
	}
}

func TestWritePostAtRest_PriorHashFromExistingFile_Good(t *testing.T) {
	pub, priv := genInternalKeyPair(t)
	s := NewService(nil)
	s.SetSessionGate(&stubFullKeyedGate{ids: []string{"acct-1"}, pub: pub, priv: priv})
	w, ok := s.atrestWriterFor()
	if !ok {
		t.Fatal("atrestWriterFor must succeed against a fully-keyed gate")
	}
	dir := t.TempDir()
	p := SocialPost{ID: "repeat-write", Ch: []string{"x"}, Text: "first", Version: 1}
	r1 := s.writePostAtRest(w, p, dir)
	if !r1.OK {
		t.Fatalf("first writePostAtRest: %s", r1.Error())
	}
	p.Text = "second"
	p.Version = 2
	r2 := s.writePostAtRest(w, p, dir)
	if !r2.OK {
		t.Fatalf("second writePostAtRest (prior-hash branch): %s", r2.Error())
	}
}

// --- fireSocialEvent -----------------------------------------------

func TestFireSocialEvent_NilReceiver_Good(t *testing.T) {
	var s *Service
	s.fireSocialEvent(EventSocialCreated, "x", "draft")
}

func TestFireSocialEvent_NilCore_Good(t *testing.T) {
	s := NewService(nil)
	s.fireSocialEvent(EventSocialCreated, "x", "draft")
}

func TestFireSocialEvent_PublishesOnCoreBus_Good(t *testing.T) {
	c := core.New()
	var got core.Message
	c.RegisterAction(func(_ *core.Core, msg core.Message) core.Result {
		got = msg
		return core.Result{OK: true}
	})
	s := NewService(c)
	s.fireSocialEvent(EventSocialSent, "post-1", "sent")

	ev, ok := got.(SocialEvent)
	if !ok {
		t.Fatalf("expected SocialEvent on the ACTION bus, got %T", got)
	}
	if ev.PostID != "post-1" || ev.State != "sent" || ev.EventName != EventSocialSent {
		t.Fatalf("unexpected SocialEvent payload: %+v", ev)
	}
}

// --- loadPosts / loadOne (dir-level fault injection) ---------------

func TestLoadPosts_SocialDirFails_Bad(t *testing.T) {
	t.Setenv("HOME", "")
	s := NewService(nil)
	_, err := s.loadPosts()
	if err == nil {
		t.Fatal("loadPosts must error when socialDir() fails")
	}
}

func TestLoadPosts_UnreadableDir_ReturnsNilNil_Good(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root — permission-denial fault injection does not apply")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := core.PathJoin(home, "Lethean", "marketing", "social")
	if mk := core.MkdirAll(dir, 0o700); !mk.OK {
		t.Fatalf("seed MkdirAll: %s", mk.Error())
	}
	if err := os.Chmod(dir, 0o000); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	s := NewService(nil)
	posts, err := s.loadPosts()
	if err != nil {
		t.Fatalf("loadPosts on unreadable dir must return (nil, nil), got err: %v", err)
	}
	if posts != nil {
		t.Fatalf("expected nil posts, got %+v", posts)
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
