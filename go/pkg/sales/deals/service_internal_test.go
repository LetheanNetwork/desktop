// SPDX-Licence-Identifier: EUPL-1.2

// service_internal_test.go — white-box cover for service.go's
// unexported helpers (stageLabel, activityTS, slugify, countDeals,
// dealStageEnumOut/In, isStrictYYYYMM, marshalRecord/parseRecord,
// stringFromRaw, Register, fireEvent). Lives in package deals (mirrors
// pkg/vi/service_internal_test.go's precedent) because these symbols
// are not reachable from the black-box deals_test package.

package deals

import (
	"testing"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/account"
)

// --- stageLabel -------------------------------------------------------

func TestService_StageLabel_Known_Good(t *testing.T) {
	if got := stageLabel("engage"); got != "Engaging" {
		t.Fatalf("stageLabel(engage) = %q, want Engaging", got)
	}
}

func TestService_StageLabel_Unknown_Bad(t *testing.T) {
	if got := stageLabel("made-up"); got != "made-up" {
		t.Fatalf("stageLabel(made-up) = %q, want passthrough made-up", got)
	}
}

// --- activityTS ---------------------------------------------------------

func TestService_ActivityTS_Zero_Good(t *testing.T) {
	now := core.Date(2026, 6, 15, 12, 0, 0, 0, core.UTC)
	if got := activityTS(core.Time{}, now); got != "" {
		t.Fatalf("activityTS(zero) = %q, want empty", got)
	}
}

func TestService_ActivityTS_SameDay_Good(t *testing.T) {
	now := core.Date(2026, 6, 15, 14, 30, 0, 0, core.UTC)
	t0 := core.Date(2026, 6, 15, 9, 5, 0, 0, core.UTC)
	got := activityTS(t0, now)
	if got != "today · 09:05" {
		t.Fatalf("activityTS(same day) = %q, want %q", got, "today · 09:05")
	}
}

func TestService_ActivityTS_Yesterday_Good(t *testing.T) {
	now := core.Date(2026, 6, 15, 14, 30, 0, 0, core.UTC)
	t0 := core.Date(2026, 6, 14, 18, 0, 0, 0, core.UTC)
	got := activityTS(t0, now)
	if got != "yest · 18:00" {
		t.Fatalf("activityTS(yesterday) = %q, want %q", got, "yest · 18:00")
	}
}

func TestService_ActivityTS_DaysAgo_Good(t *testing.T) {
	now := core.Date(2026, 6, 15, 12, 0, 0, 0, core.UTC)
	t0 := core.Date(2026, 6, 12, 12, 0, 0, 0, core.UTC)
	got := activityTS(t0, now)
	if got != "3 d ago" {
		t.Fatalf("activityTS(3 days) = %q, want %q", got, "3 d ago")
	}
}

func TestService_ActivityTS_WeeksAgo_Good(t *testing.T) {
	now := core.Date(2026, 6, 15, 12, 0, 0, 0, core.UTC)
	t0 := core.Date(2026, 5, 20, 12, 0, 0, 0, core.UTC)
	got := activityTS(t0, now)
	if got != "3 w ago" {
		t.Fatalf("activityTS(3 weeks) = %q, want %q", got, "3 w ago")
	}
}

// TestService_ActivityTS_FutureClampsAbs_Good — a future timestamp
// (t after now) exercises the diff<0 abs-clamp branch; same-day still
// wins over the abs-diff sign.
func TestService_ActivityTS_FutureClampsAbs_Good(t *testing.T) {
	now := core.Date(2026, 6, 10, 8, 0, 0, 0, core.UTC)
	t0 := core.Date(2026, 6, 20, 8, 0, 0, 0, core.UTC)
	got := activityTS(t0, now)
	if got != "1 w ago" {
		t.Fatalf("activityTS(future) = %q, want %q", got, "1 w ago")
	}
}

// --- slugify --------------------------------------------------------

func TestService_Slugify_MixedCase_Good(t *testing.T) {
	if got := slugify("Heritage Law LLP"); got != "heritage-law-llp" {
		t.Fatalf("slugify = %q, want heritage-law-llp", got)
	}
}

func TestService_Slugify_DigitsAndSymbols_Good(t *testing.T) {
	if got := slugify("Acme & Co. #2 (UK)"); got != "acme-co-2-uk" {
		t.Fatalf("slugify = %q, want acme-co-2-uk", got)
	}
}

// TestService_Slugify_TrailingHyphensTrimmed_Good — slugify only trims
// TRAILING hyphen runs (the final loop walks from the end); a leading
// hyphen run collapses to a single hyphen but is not stripped. Pins
// the real behaviour rather than the more symmetric one a reader might
// assume.
func TestService_Slugify_TrailingHyphensTrimmed_Good(t *testing.T) {
	if got := slugify("--Foo--"); got != "-foo" {
		t.Fatalf("slugify(--Foo--) = %q, want -foo", got)
	}
}

func TestService_Slugify_Empty_Ugly(t *testing.T) {
	if got := slugify(""); got != "" {
		t.Fatalf("slugify(empty) = %q, want empty", got)
	}
}

func TestService_Slugify_AllSymbols_Ugly(t *testing.T) {
	if got := slugify("***"); got != "" {
		t.Fatalf("slugify(***) = %q, want empty (all collapse+trim)", got)
	}
}

// --- countDeals -------------------------------------------------------

func TestService_CountDeals_MissingDir_Bad(t *testing.T) {
	if got := countDeals("/does/not/exist/at/all"); got != 0 {
		t.Fatalf("countDeals(missing) = %d, want 0", got)
	}
}

func TestService_CountDeals_MixedEntries_Good(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.md", "b.lthn", "c.txt"} {
		if w := core.WriteFile(core.PathJoin(dir, name), []byte("x"), 0o600); !w.OK {
			t.Fatalf("seed WriteFile(%s): %s", name, w.Error())
		}
	}
	if mk := core.MkdirAll(core.PathJoin(dir, "subdir"), 0o700); !mk.OK {
		t.Fatalf("seed MkdirAll: %s", mk.Error())
	}
	if got := countDeals(dir); got != 2 {
		t.Fatalf("countDeals = %d, want 2 (a.md + b.lthn, subdir + c.txt excluded)", got)
	}
}

// --- dealStageEnumOut / dealStageEnumIn ------------------------------

func TestService_DealStageEnumOut_Passthrough_Good(t *testing.T) {
	for _, s := range []string{"qual", "engage", "propose"} {
		if got := dealStageEnumOut(s); got != s {
			t.Fatalf("dealStageEnumOut(%s) = %q, want passthrough", s, got)
		}
	}
}

func TestService_DealStageEnumOut_MappedBranches_Good(t *testing.T) {
	cases := map[string]string{
		"close": "negotiate",
		"won":   "closed_won",
		"lost":  "closed_lost",
	}
	for in, want := range cases {
		if got := dealStageEnumOut(in); got != want {
			t.Fatalf("dealStageEnumOut(%s) = %q, want %q", in, got, want)
		}
	}
}

func TestService_DealStageEnumOut_UnknownFallsOnHold_Bad(t *testing.T) {
	if got := dealStageEnumOut("nonsense"); got != "on_hold" {
		t.Fatalf("dealStageEnumOut(nonsense) = %q, want on_hold", got)
	}
}

func TestService_DealStageEnumIn_Passthrough_Good(t *testing.T) {
	for _, s := range []string{"qual", "engage", "propose"} {
		if got := dealStageEnumIn(s); got != s {
			t.Fatalf("dealStageEnumIn(%s) = %q, want passthrough", s, got)
		}
	}
}

func TestService_DealStageEnumIn_MappedBranches_Good(t *testing.T) {
	cases := map[string]string{
		"negotiate":   "close",
		"closed_won":  "won",
		"closed_lost": "lost",
	}
	for in, want := range cases {
		if got := dealStageEnumIn(in); got != want {
			t.Fatalf("dealStageEnumIn(%s) = %q, want %q", in, got, want)
		}
	}
}

func TestService_DealStageEnumIn_OnHoldAndUnknown_Bad(t *testing.T) {
	if got := dealStageEnumIn("on_hold"); got != "" {
		t.Fatalf("dealStageEnumIn(on_hold) = %q, want empty", got)
	}
	if got := dealStageEnumIn("bogus"); got != "" {
		t.Fatalf("dealStageEnumIn(bogus) = %q, want empty", got)
	}
}

// --- isStrictYYYYMM ---------------------------------------------------

func TestService_IsStrictYYYYMM_Valid_Good(t *testing.T) {
	for _, s := range []string{"2026-01", "2026-12", "0000-06"} {
		if !isStrictYYYYMM(s) {
			t.Fatalf("isStrictYYYYMM(%s) = false, want true", s)
		}
	}
}

func TestService_IsStrictYYYYMM_WrongLength_Bad(t *testing.T) {
	for _, s := range []string{"", "2026", "2026-1", "2026-001", "2026/06"} {
		if isStrictYYYYMM(s) {
			t.Fatalf("isStrictYYYYMM(%q) = true, want false", s)
		}
	}
}

func TestService_IsStrictYYYYMM_NonDigitYear_Bad(t *testing.T) {
	if isStrictYYYYMM("20a6-06") {
		t.Fatal("isStrictYYYYMM with non-digit year must be false")
	}
}

func TestService_IsStrictYYYYMM_MonthOutOfRange_Bad(t *testing.T) {
	for _, s := range []string{"2026-00", "2026-13", "2026-99", "2026-2x"} {
		if isStrictYYYYMM(s) {
			t.Fatalf("isStrictYYYYMM(%q) = true, want false", s)
		}
	}
}

// --- marshalRecord / parseRecord round trip --------------------------

func TestService_MarshalParseRecord_RoundTrip_Good(t *testing.T) {
	rec := DealRecord{
		ID: "202606-DEAL-001", Customer: "Heritage Law", AmountPence: 24000,
		Stage: "engage", Notes: "body text here",
	}
	raw, err := marshalRecord(rec)
	if err != nil {
		t.Fatalf("marshalRecord: %v", err)
	}
	got, err := parseRecord(raw)
	if err != nil {
		t.Fatalf("parseRecord: %v", err)
	}
	if got.Customer != rec.Customer || got.Notes != rec.Notes {
		t.Fatalf("round trip mismatch: got %+v", got)
	}
}

func TestService_ParseRecord_BadYAML_Bad(t *testing.T) {
	_, err := parseRecord([]byte("---\n[not: valid: yaml\n---\nbody"))
	if err == nil {
		t.Fatal("parseRecord with malformed YAML frontmatter must error")
	}
}

// --- stringFromRaw ----------------------------------------------------

func TestService_StringFromRaw_NilMap_Bad(t *testing.T) {
	if got := stringFromRaw(nil, "stage"); got != "" {
		t.Fatalf("stringFromRaw(nil) = %q, want empty", got)
	}
}

func TestService_StringFromRaw_MissingKey_Bad(t *testing.T) {
	if got := stringFromRaw(map[string]any{"other": "x"}, "stage"); got != "" {
		t.Fatalf("stringFromRaw(missing) = %q, want empty", got)
	}
}

func TestService_StringFromRaw_WrongType_Bad(t *testing.T) {
	if got := stringFromRaw(map[string]any{"stage": 42}, "stage"); got != "" {
		t.Fatalf("stringFromRaw(non-string) = %q, want empty", got)
	}
}

func TestService_StringFromRaw_Present_Good(t *testing.T) {
	if got := stringFromRaw(map[string]any{"stage": "negotiate"}, "stage"); got != "negotiate" {
		t.Fatalf("stringFromRaw(present) = %q, want negotiate", got)
	}
}

// --- Register -----------------------------------------------------------

func TestService_Register_Good(t *testing.T) {
	r := Register(nil)
	if !r.OK {
		t.Fatalf("Register failed: %s", r.Error())
	}
	if _, ok := r.Value.(*Service); !ok {
		t.Fatalf("Register value = %T, want *Service", r.Value)
	}
}

// --- fireEvent ----------------------------------------------------------

func TestService_FireEvent_NilReceiver_Good(t *testing.T) {
	var s *Service
	// Must not panic — the nil-receiver guard is the first statement.
	s.fireEvent(EventDealCreated, DealRecord{ID: "x"})
}

func TestService_FireEvent_NilCore_Good(t *testing.T) {
	s := NewService(nil)
	// Must not panic — s.core == nil short-circuits before ACTION.
	s.fireEvent(EventDealCreated, DealRecord{ID: "x"})
}

// --- dealsDir -----------------------------------------------------------

// TestService_DealsDir_HomeUnavailable_Bad — os.UserHomeDir() (wrapped
// by core.UserHomeDir) rejects an empty $HOME on darwin/linux, so
// dealsDir's `if !root.OK { return root }` early-return is reachable
// hermetically by unsetting HOME rather than needing a real broken
// environment.
func TestService_DealsDir_HomeUnavailable_Bad(t *testing.T) {
	t.Setenv("HOME", "")
	r := dealsDir()
	if r.OK {
		t.Fatal("dealsDir() must fail when $HOME is unavailable")
	}
}

// --- toDeal -------------------------------------------------------------

// TestService_ToDeal_DocsAndLogPopulate_Good — CreateInput has no Docs
// field so DealRecord.Docs is always empty on every public-API path;
// toDeal's docs-projection loop is only reachable by constructing a
// DealRecord directly (e.g. a future docs-attach feature, or a record
// migrated from an older schema that carried them).
func TestService_ToDeal_DocsAndLogPopulate_Good(t *testing.T) {
	now := core.Date(2026, 6, 15, 12, 0, 0, 0, core.UTC)
	rec := DealRecord{
		ID: "202606-DEAL-005", Customer: "Acme", AmountPence: 1500, Stage: "engage",
		Log:  []ActivityRecord{{At: now, K: "call", Who: "you", T: "first"}},
		Docs: []DocRecord{{Title: "SOW v2 · draft", State: "draft"}},
	}
	d := toDeal(rec, now, true)
	if len(d.Docs) != 1 || d.Docs[0].T != "SOW v2 · draft" || d.Docs[0].S != "draft" {
		t.Fatalf("expected 1 doc projected, got %+v", d.Docs)
	}
	if len(d.Log) != 1 || d.Log[0].T != "first" {
		t.Fatalf("expected 1 log entry projected, got %+v", d.Log)
	}
}

func TestService_ToDeal_ExcludeLog_Good(t *testing.T) {
	now := core.Now()
	rec := DealRecord{ID: "x", Log: []ActivityRecord{{At: now, K: "call"}}}
	d := toDeal(rec, now, false)
	if d.Log != nil {
		t.Fatalf("includeLog=false must yield nil Log, got %+v", d.Log)
	}
}

// --- headerPubKey ---------------------------------------------------

func TestService_HeaderPubKey_NilGate_Bad(t *testing.T) {
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

func TestService_HeaderPubKey_GateNotKeyProvider_Bad(t *testing.T) {
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
// direct headerPubKey unit tests (avoids pulling in the real PGP
// keypair machinery from deals_test.go's genTestKeyPair — headerPubKey
// never touches PrivateKeyFor). PrivateKeyFor returns a real (nil-
// backed) *account.PrivateKeyHandle shape via NewPrivateKeyHandleFor
// Test so the accountKeyProvider interface is satisfied exactly.
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

func TestService_HeaderPubKey_MalformedBlob_Bad(t *testing.T) {
	s := NewService(nil)
	s.SetSessionGate(&stubKeyedGate{ids: []string{"x"}, pub: []byte("pub")})
	_, err := s.headerPubKey([]byte("x")) // too short for PeekAccountID
	if err == nil {
		t.Fatal("headerPubKey with a too-short blob must error")
	}
}

func TestService_HeaderPubKey_PublicKeyNotFound_Bad(t *testing.T) {
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

func TestService_HeaderPubKey_Success_Good(t *testing.T) {
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
// real PGP-sealed payload. Mirrors atrest_test.go's trixHeaderEnd
// inverse.
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

func TestService_LoadHeaderOnly_GateNotWired_Bad(t *testing.T) {
	s := NewService(nil)
	_, err := s.loadHeaderOnly("/does/not/matter")
	if err == nil {
		t.Fatal("loadHeaderOnly with unwired gate must error")
	}
	if !core.Contains(err.Error(), "session gate not wired") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestService_LoadHeaderOnly_FileMissing_Bad(t *testing.T) {
	s := NewService(nil)
	s.SetSessionGate(&stubKeyedGate{ids: []string{"x"}, pub: []byte("pub")})
	_, err := s.loadHeaderOnly(core.PathJoin(t.TempDir(), "missing.lthn"))
	if err == nil {
		t.Fatal("loadHeaderOnly on a missing file must error")
	}
}

func TestService_LoadHeaderOnly_HeaderPubKeyFails_Bad(t *testing.T) {
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

func TestService_LoadHeaderOnly_DecodeHeaderRejects_Bad(t *testing.T) {
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

func TestService_FireEvent_PublishesOnCoreBus_Good(t *testing.T) {
	c := core.New()
	var got core.Message
	c.RegisterAction(func(_ *core.Core, msg core.Message) core.Result {
		got = msg
		return core.Result{OK: true}
	})
	s := NewService(c)
	s.fireEvent(EventDealStageChanged, DealRecord{ID: "202606-DEAL-002", Stage: "propose", Customer: "Acme"})

	ev, ok := got.(DealEvent)
	if !ok {
		t.Fatalf("expected DealEvent on the ACTION bus, got %T", got)
	}
	if ev.DealID != "202606-DEAL-002" || ev.Stage != "propose" || ev.Customer != "Acme" {
		t.Fatalf("unexpected DealEvent payload: %+v", ev)
	}
	if ev.EventName != EventDealStageChanged {
		t.Fatalf("EventName = %q, want %q", ev.EventName, EventDealStageChanged)
	}
}
