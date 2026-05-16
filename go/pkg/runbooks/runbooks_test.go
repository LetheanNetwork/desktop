// SPDX-Licence-Identifier: EUPL-1.2

package runbooks_test

import (
	"testing"

	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/runbooks"
)

func TestParseRecord_RoundTrip(t *testing.T) {
	now := core.Now().UTC()
	rec := subject.RunbookRecord{
		ID:            "R-01",
		Title:         "Rotate runtime API keys",
		Area:          "auth",
		Tags:          []string{"auth", "api-keys"},
		Slug:          "rotate-runtime-api-keys",
		LastRehearsed: now.Add(-2 * 24 * core.Hour),
		CreatedAt:     now.Add(-90 * 24 * core.Hour),
	}
	raw, err := subject.MarshalRecordExported(rec)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got, err := subject.ParseRecordExported(rec.Slug, raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got.ID != rec.ID {
		t.Errorf("id: want %q got %q", rec.ID, got.ID)
	}
	if got.Title != rec.Title {
		t.Errorf("title: want %q got %q", rec.Title, got.Title)
	}
	if got.Area != rec.Area {
		t.Errorf("area: want %q got %q", rec.Area, got.Area)
	}
	if len(got.Tags) != len(rec.Tags) {
		t.Errorf("tags: want %v got %v", rec.Tags, got.Tags)
	}
}

func TestComputeHealth_Fresh(t *testing.T) {
	now := core.Now()
	rec := subject.RunbookRecord{LastRehearsed: now.Add(-2 * 24 * core.Hour)}
	if h := subject.ComputeHealthExported(rec, now); h != "fresh" {
		t.Errorf("want 'fresh' got %q", h)
	}
}

func TestComputeHealth_Aging(t *testing.T) {
	now := core.Now()
	rec := subject.RunbookRecord{LastRehearsed: now.Add(-45 * 24 * core.Hour)}
	if h := subject.ComputeHealthExported(rec, now); h != "aging" {
		t.Errorf("want 'aging' got %q", h)
	}
}

func TestComputeHealth_Stale(t *testing.T) {
	now := core.Now()
	rec := subject.RunbookRecord{LastRehearsed: now.Add(-120 * 24 * core.Hour)}
	if h := subject.ComputeHealthExported(rec, now); h != "stale" {
		t.Errorf("want 'stale' got %q", h)
	}
}

func TestComputeHealth_NeverRehearsed(t *testing.T) {
	now := core.Now()
	rec := subject.RunbookRecord{} // zero LastRehearsed
	if h := subject.ComputeHealthExported(rec, now); h != "stale" {
		t.Errorf("want 'stale' got %q", h)
	}
}

func TestRelativeTime_Day(t *testing.T) {
	now := core.Now()
	past := now.Add(-2 * 24 * core.Hour)
	got := subject.RelativeTimeExported(past, now)
	if got != "2 d" {
		t.Errorf("want '2 d' got %q", got)
	}
}

func TestRelativeTime_Week(t *testing.T) {
	now := core.Now()
	past := now.Add(-3 * 7 * 24 * core.Hour)
	got := subject.RelativeTimeExported(past, now)
	if got != "3 w" {
		t.Errorf("want '3 w' got %q", got)
	}
}

func TestRelativeTime_Month(t *testing.T) {
	now := core.Now()
	past := now.Add(-4 * 30 * 24 * core.Hour)
	got := subject.RelativeTimeExported(past, now)
	if got != "4 mo" {
		t.Errorf("want '4 mo' got %q", got)
	}
}

func TestRelativeTime_Never(t *testing.T) {
	now := core.Now()
	got := subject.RelativeTimeExported(core.Time{}, now)
	if got != "never" {
		t.Errorf("want 'never' got %q", got)
	}
}

func TestMatchSearch_Title(t *testing.T) {
	rec := subject.RunbookRecord{Title: "Drain a stuck Postfix queue", Area: "mail", ID: "R-05"}
	if !subject.MatchSearchExported(rec, "postfix") {
		t.Error("expected match on 'postfix'")
	}
}

func TestMatchSearch_Area(t *testing.T) {
	rec := subject.RunbookRecord{Title: "Trigger emergency model unload", Area: "runtime", ID: "R-06"}
	if !subject.MatchSearchExported(rec, "runtime") {
		t.Error("expected match on 'runtime'")
	}
}

func TestMatchSearch_Empty(t *testing.T) {
	rec := subject.RunbookRecord{Title: "anything", Area: "anything", ID: "R-01"}
	if !subject.MatchSearchExported(rec, "") {
		t.Error("empty query should match everything")
	}
}

func TestMatchSearch_NoMatch(t *testing.T) {
	rec := subject.RunbookRecord{Title: "Rotate runtime API keys", Area: "auth", ID: "R-01"}
	if subject.MatchSearchExported(rec, "postfix") {
		t.Error("expected no match on 'postfix'")
	}
}

func TestServiceName_Runbooks(t *testing.T) {
	svc := subject.NewService(nil)
	if got := svc.ServiceName(); got != "Runbooks" {
		t.Errorf("want 'Runbooks' got %q", got)
	}
}
