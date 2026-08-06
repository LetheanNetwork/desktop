// SPDX-Licence-Identifier: EUPL-1.2

// service_internal_test.go — white-box cover for service.go's
// unexported helpers (slugify, warmthFor, lastTouchLabel, parseContact,
// marshalContact, containsCI, Register, fireEvent, contactsDir). Lives
// in package contacts (mirrors pkg/sales/deals/service_internal_test.go
// and pkg/vi/service_internal_test.go's precedent) because these
// symbols are not reachable from the black-box contacts_test package.

package contacts

import (
	"testing"

	core "dappco.re/go"
)

// --- slugify --------------------------------------------------------

func TestService_Slugify_MixedCase_Good(t *testing.T) {
	if got := slugify("Ada Penley"); got != "ada-penley" {
		t.Fatalf("slugify = %q, want ada-penley", got)
	}
}

func TestService_Slugify_DigitsAndSymbols_Good(t *testing.T) {
	if got := slugify("Team #2 (EMEA)"); got != "team-2-emea" {
		t.Fatalf("slugify = %q, want team-2-emea", got)
	}
}

func TestService_Slugify_Empty_Ugly(t *testing.T) {
	if got := slugify(""); got != "" {
		t.Fatalf("slugify(empty) = %q, want empty", got)
	}
}

func TestService_Slugify_AllSymbols_Ugly(t *testing.T) {
	if got := slugify("***"); got != "" {
		t.Fatalf("slugify(***) = %q, want empty", got)
	}
}

// TestService_Slugify_TrailingHyphensTrimmed_Good — mirrors deals'
// slugify pin: only trailing hyphen runs are trimmed, a leading run
// collapses to one hyphen but survives.
func TestService_Slugify_TrailingHyphensTrimmed_Good(t *testing.T) {
	if got := slugify("--Ada--"); got != "-ada" {
		t.Fatalf("slugify(--Ada--) = %q, want -ada", got)
	}
}

// --- warmthFor --------------------------------------------------------

func TestService_WarmthFor_Zero_Good(t *testing.T) {
	now := core.Date(2026, 6, 15, 12, 0, 0, 0, core.UTC)
	if got := warmthFor(core.Time{}, now); got != "cool" {
		t.Fatalf("warmthFor(zero) = %q, want cool", got)
	}
}

func TestService_WarmthFor_HotBoundary_Good(t *testing.T) {
	now := core.Date(2026, 6, 15, 12, 0, 0, 0, core.UTC)
	touch := now.Add(-7 * 24 * core.Hour)
	if got := warmthFor(touch, now); got != "hot" {
		t.Fatalf("warmthFor(7d) = %q, want hot", got)
	}
}

func TestService_WarmthFor_WarmBoundary_Good(t *testing.T) {
	now := core.Date(2026, 6, 15, 12, 0, 0, 0, core.UTC)
	touch := now.Add(-8 * 24 * core.Hour)
	if got := warmthFor(touch, now); got != "warm" {
		t.Fatalf("warmthFor(8d) = %q, want warm", got)
	}
	touch21 := now.Add(-21 * 24 * core.Hour)
	if got := warmthFor(touch21, now); got != "warm" {
		t.Fatalf("warmthFor(21d) = %q, want warm", got)
	}
}

func TestService_WarmthFor_Cool_Good(t *testing.T) {
	now := core.Date(2026, 6, 15, 12, 0, 0, 0, core.UTC)
	touch := now.Add(-22 * 24 * core.Hour)
	if got := warmthFor(touch, now); got != "cool" {
		t.Fatalf("warmthFor(22d) = %q, want cool", got)
	}
}

// TestService_WarmthFor_FutureClampsAbs_Good — a future lastTouch
// (touch after now) exercises the diff<0 abs-clamp branch.
func TestService_WarmthFor_FutureClampsAbs_Good(t *testing.T) {
	now := core.Date(2026, 6, 10, 8, 0, 0, 0, core.UTC)
	touch := core.Date(2026, 6, 20, 8, 0, 0, 0, core.UTC)
	if got := warmthFor(touch, now); got != "warm" {
		t.Fatalf("warmthFor(future 10d) = %q, want warm", got)
	}
}

// --- lastTouchLabel -----------------------------------------------------

func TestService_LastTouchLabel_Zero_Good(t *testing.T) {
	now := core.Date(2026, 6, 15, 12, 0, 0, 0, core.UTC)
	if got := lastTouchLabel(core.Time{}, now); got != "" {
		t.Fatalf("lastTouchLabel(zero) = %q, want empty", got)
	}
}

func TestService_LastTouchLabel_JustNow_Good(t *testing.T) {
	now := core.Date(2026, 6, 15, 12, 0, 30, 0, core.UTC)
	touch := now.Add(-10 * core.Second)
	if got := lastTouchLabel(touch, now); got != "just now" {
		t.Fatalf("lastTouchLabel(10s) = %q, want just now", got)
	}
}

func TestService_LastTouchLabel_MinutesAgo_Good(t *testing.T) {
	now := core.Date(2026, 6, 15, 12, 30, 0, 0, core.UTC)
	touch := now.Add(-15 * core.Minute)
	if got := lastTouchLabel(touch, now); got != "15 min ago" {
		t.Fatalf("lastTouchLabel(15min) = %q, want 15 min ago", got)
	}
}

func TestService_LastTouchLabel_HoursAgo_Good(t *testing.T) {
	now := core.Date(2026, 6, 15, 12, 0, 0, 0, core.UTC)
	touch := now.Add(-5 * core.Hour)
	if got := lastTouchLabel(touch, now); got != "5 h ago" {
		t.Fatalf("lastTouchLabel(5h) = %q, want 5 h ago", got)
	}
}

func TestService_LastTouchLabel_DaysAgo_Good(t *testing.T) {
	now := core.Date(2026, 6, 15, 12, 0, 0, 0, core.UTC)
	touch := now.Add(-3 * 24 * core.Hour)
	if got := lastTouchLabel(touch, now); got != "3 d ago" {
		t.Fatalf("lastTouchLabel(3d) = %q, want 3 d ago", got)
	}
}

func TestService_LastTouchLabel_WeeksAgo_Good(t *testing.T) {
	now := core.Date(2026, 6, 15, 12, 0, 0, 0, core.UTC)
	touch := now.Add(-15 * 24 * core.Hour)
	if got := lastTouchLabel(touch, now); got != "2 w ago" {
		t.Fatalf("lastTouchLabel(15d) = %q, want 2 w ago", got)
	}
}

func TestService_LastTouchLabel_FutureClampsAbs_Good(t *testing.T) {
	now := core.Date(2026, 6, 10, 8, 0, 0, 0, core.UTC)
	touch := core.Date(2026, 6, 10, 9, 0, 0, 0, core.UTC)
	if got := lastTouchLabel(touch, now); got != "1 h ago" {
		t.Fatalf("lastTouchLabel(future 1h) = %q, want 1 h ago", got)
	}
}

// --- parseContact / marshalContact --------------------------------------

func TestService_MarshalParseContact_RoundTrip_Good(t *testing.T) {
	rec := ContactRecord{
		ID: "ada-penley", Name: "Ada Penley", Role: "CTO",
		Notes: "body text here",
	}
	raw, err := marshalContact(rec)
	if err != nil {
		t.Fatalf("marshalContact: %v", err)
	}
	got, err := parseContact(raw)
	if err != nil {
		t.Fatalf("parseContact: %v", err)
	}
	if got.Name != rec.Name || got.Notes != rec.Notes {
		t.Fatalf("round trip mismatch: got %+v", got)
	}
}

func TestService_MarshalContact_EmptyNotes_Good(t *testing.T) {
	rec := ContactRecord{ID: "x", Name: "X"}
	raw, err := marshalContact(rec)
	if err != nil {
		t.Fatalf("marshalContact: %v", err)
	}
	// No body appended when Notes is empty — file ends at the closing
	// delimiter.
	tail := string(raw[len(raw)-4:])
	if tail != "---\n" {
		t.Fatalf("expected file to end at the closing delimiter, got tail %q", tail)
	}
}

func TestService_ParseContact_BadYAML_Bad(t *testing.T) {
	_, err := parseContact([]byte("---\n[not: valid: yaml\n---\nbody"))
	if err == nil {
		t.Fatal("parseContact with malformed YAML frontmatter must error")
	}
}

// TestService_ParseContact_NoClosingDelimiter_Bad — content that opens
// with "---\n" but never closes falls through to the whole-content
// yaml.Unmarshal branch (closeIdx stays -1).
func TestService_ParseContact_NoClosingDelimiter_Bad(t *testing.T) {
	_, err := parseContact([]byte("---\nid: x\nname: X\n"))
	// Valid YAML with no closing delimiter parses fine via the
	// whole-content branch — this pins that the closeIdx<0 path is
	// reachable and doesn't error on well-formed YAML.
	if err != nil {
		t.Fatalf("parseContact(no closing delimiter, valid yaml): %v", err)
	}
}

func TestService_ParseContact_NoClosingDelimiter_BadYAML_Bad(t *testing.T) {
	_, err := parseContact([]byte("---\n[not valid yaml at all: :\n"))
	if err == nil {
		t.Fatal("parseContact(no closing delimiter, malformed yaml) must error")
	}
}

func TestService_ParseContact_NoOpeningDelimiter_Good(t *testing.T) {
	// Content that never opens with "---\n" — the open-delimiter skip
	// is a no-op, closeIdx scan still runs.
	rec, err := parseContact([]byte("id: x\nname: X\n---\nbody text"))
	if err != nil {
		t.Fatalf("parseContact(no opening delimiter): %v", err)
	}
	if rec.Notes != "body text" {
		t.Fatalf("expected body text, got %q", rec.Notes)
	}
}

// --- containsCI ---------------------------------------------------------

func TestService_ContainsCI_EmptyNeedle_Good(t *testing.T) {
	if !containsCI("anything", "") {
		t.Fatal("containsCI with empty needle must always be true")
	}
}

func TestService_ContainsCI_CaseInsensitiveMatch_Good(t *testing.T) {
	if !containsCI("Ada Penley", "penley") {
		t.Fatal("containsCI must match case-insensitively")
	}
}

func TestService_ContainsCI_NoMatch_Bad(t *testing.T) {
	if containsCI("Ada Penley", "marcus") {
		t.Fatal("containsCI must not match an absent substring")
	}
}

func TestService_ContainsCI_NeedleLongerThanHaystack_Bad(t *testing.T) {
	if containsCI("hi", "hello there") {
		t.Fatal("containsCI must reject a needle longer than the haystack")
	}
}

// --- contactsDir --------------------------------------------------------

func TestService_ContactsDir_HomeUnavailable_Bad(t *testing.T) {
	t.Setenv("HOME", "")
	r := contactsDir()
	if r.OK {
		t.Fatal("contactsDir() must fail when $HOME is unavailable")
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

// --- fireEvent ------------------------------------------------------

func TestService_FireEvent_NilReceiver_Good(t *testing.T) {
	var s *Service
	s.fireEvent(EventContactCreated, Contact{ID: "x"})
}

func TestService_FireEvent_NilCore_Good(t *testing.T) {
	s := NewService(nil)
	s.fireEvent(EventContactCreated, Contact{ID: "x"})
}

func TestService_FireEvent_PublishesOnCoreBus_Good(t *testing.T) {
	c := core.New()
	var got core.Message
	c.RegisterAction(func(_ *core.Core, msg core.Message) core.Result {
		got = msg
		return core.Result{OK: true}
	})
	s := NewService(c)
	s.fireEvent(EventContactUpdated, Contact{ID: "ada-penley", Name: "Ada Penley"})

	ev, ok := got.(ContactEvent)
	if !ok {
		t.Fatalf("expected ContactEvent on the ACTION bus, got %T", got)
	}
	if ev.Contact.ID != "ada-penley" || ev.EventName != EventContactUpdated {
		t.Fatalf("unexpected ContactEvent payload: %+v", ev)
	}
}

// --- loadAll ----------------------------------------------------------

func TestService_LoadAll_MissingDir_Bad(t *testing.T) {
	t.Setenv("HOME", "")
	_, err := loadAll()
	if err == nil {
		t.Fatal("loadAll() must error when contactsDir() fails")
	}
}
