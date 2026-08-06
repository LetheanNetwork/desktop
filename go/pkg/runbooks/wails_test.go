// SPDX-Licence-Identifier: EUPL-1.2

package runbooks_test

import (
	"testing"

	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/runbooks"
)

func TestListInput_Fields(t *testing.T) {
	in := subject.ListInput{Health: "stale", Area: "mail", Limit: 5}
	if in.Health != "stale" || in.Area != "mail" || in.Limit != 5 {
		t.Errorf("ListInput fields mismatch")
	}
}

func TestGetInput_Fields(t *testing.T) {
	in := subject.GetInput{ID: "R-01", Slug: "rotate-runtime-api-keys"}
	if in.ID == "" || in.Slug == "" {
		t.Errorf("GetInput fields mismatch")
	}
}

func TestSearchInput_Fields(t *testing.T) {
	in := subject.SearchInput{Query: "postfix", Limit: 10}
	if in.Query != "postfix" || in.Limit != 10 {
		t.Errorf("SearchInput fields mismatch")
	}
}

func TestMarkInput_Fields(t *testing.T) {
	in := subject.MarkInput{ID: "R-05"}
	if in.ID == "" {
		t.Errorf("MarkInput fields mismatch")
	}
}

func TestList_NilService_Nonempty(t *testing.T) {
	// List on a nil-core service should not panic — seeding may fail
	// but we should return an empty-ish result, not crash.
	svc := subject.NewService(nil)
	_ = svc.List(subject.ListInput{})
}

// TestList_HealthAndAreaFilters_BothContinueBranches drives List's two
// per-record filter checks in one call: R-05 (area "mail", ~4mo since
// rehearsal -> stale) and R-07 (area "client", ~6mo -> stale) are both
// health-eligible, but only R-05 also matches Area — so the Area
// mismatch on R-07 exercises the Area-continue branch, while every
// fresh/aging seed exercises the Health-continue branch. Neither
// branch is reachable from a single-field filter alone.
func TestList_HealthAndAreaFilters_BothContinueBranches(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := subject.NewService(nil)
	if r := svc.OnStart(); !r.OK {
		t.Fatalf("OnStart: %s", r.Error())
	}
	r := svc.List(subject.ListInput{Health: "stale", Area: "mail"})
	if !r.OK {
		t.Fatalf("List: %s", r.Error())
	}
	out := r.Value.(subject.ListOutput)
	if len(out.Books) != 1 || out.Books[0].ID != "R-05" {
		t.Errorf("List(stale, mail) = %+v, want exactly R-05 (drain-a-stuck-postfix-queue)", out.Books)
	}
}

// TestList_FilteredEmptyBecomesEmptySlice covers the nil->[]RunbookEntry{}
// guard: a filter combination matching zero seed records must return an
// empty (non-nil) slice, not a nil one — JSON callers expect `[]`, not
// `null`.
func TestList_FilteredEmptyBecomesEmptySlice(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := subject.NewService(nil)
	if r := svc.OnStart(); !r.OK {
		t.Fatalf("OnStart: %s", r.Error())
	}
	r := svc.List(subject.ListInput{Area: "no-such-area"})
	if !r.OK {
		t.Fatalf("List: %s", r.Error())
	}
	out := r.Value.(subject.ListOutput)
	if out.Books == nil {
		t.Error("List with zero matches must return an empty slice, not nil")
	}
	if len(out.Books) != 0 {
		t.Errorf("List(no-such-area) = %d books, want 0", len(out.Books))
	}
}

// TestList_LimitTruncates covers the Limit>0 truncation branch — 7
// seed records, Limit 3, must return exactly 3 while Total still
// reports the full unfiltered library count.
func TestList_LimitTruncates(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := subject.NewService(nil)
	if r := svc.OnStart(); !r.OK {
		t.Fatalf("OnStart: %s", r.Error())
	}
	r := svc.List(subject.ListInput{Limit: 3})
	if !r.OK {
		t.Fatalf("List: %s", r.Error())
	}
	out := r.Value.(subject.ListOutput)
	if len(out.Books) != 3 {
		t.Errorf("List(Limit: 3) returned %d books, want 3 (truncated from 7 seeds)", len(out.Books))
	}
	if out.Total != 7 {
		t.Errorf("List(Limit: 3).Total = %d, want 7 (unfiltered library count)", out.Total)
	}
}

// TestSearch_NoMatchBecomesEmptySlice mirrors List's nil-guard for
// Search's matched slice.
func TestSearch_NoMatchBecomesEmptySlice(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := subject.NewService(nil)
	if r := svc.OnStart(); !r.OK {
		t.Fatalf("OnStart: %s", r.Error())
	}
	r := svc.Search(subject.SearchInput{Query: "zzz-does-not-exist-zzz"})
	if !r.OK {
		t.Fatalf("Search: %s", r.Error())
	}
	out := r.Value.(subject.ListOutput)
	if out.Books == nil {
		t.Error("Search with zero matches must return an empty slice, not nil")
	}
}

// TestSearch_LimitTruncates covers Search's own Limit truncation
// branch — an empty query matches every seed record.
func TestSearch_LimitTruncates(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := subject.NewService(nil)
	if r := svc.OnStart(); !r.OK {
		t.Fatalf("OnStart: %s", r.Error())
	}
	r := svc.Search(subject.SearchInput{Query: "", Limit: 3})
	if !r.OK {
		t.Fatalf("Search: %s", r.Error())
	}
	out := r.Value.(subject.ListOutput)
	if len(out.Books) != 3 {
		t.Errorf("Search(empty query, Limit: 3) returned %d books, want 3", len(out.Books))
	}
}

// TestGet_BothEmpty_Bad covers the id-or-slug-required guard.
func TestGet_BothEmpty_Bad(t *testing.T) {
	svc := subject.NewService(nil)
	r := svc.Get(subject.GetInput{})
	if r.OK {
		t.Error("Get with no ID and no Slug should fail")
	}
}

// TestGet_SlugNotFound_LabelFallbackAndNotFoundError covers two
// branches together: the error label falls back to Slug when ID is
// empty, and loadOne's generic miss (no session.locked / at-rest
// substring) reaches the final "not found" Fail — distinct from the
// gate-locked passthrough paths exercised elsewhere in this package.
func TestGet_SlugNotFound_LabelFallbackAndNotFoundError(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := subject.NewService(nil)
	if r := svc.OnStart(); !r.OK {
		t.Fatalf("OnStart: %s", r.Error())
	}
	r := svc.Get(subject.GetInput{Slug: "totally-nonexistent-runbook-slug"})
	if r.OK {
		t.Fatal("Get with a nonexistent slug must fail")
	}
	if !core.Contains(r.Error(), "not found") {
		t.Errorf("Get error = %q, want it to mention 'not found' (label falls back to Slug when ID is empty)", r.Error())
	}
}
