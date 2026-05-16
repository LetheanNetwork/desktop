// SPDX-Licence-Identifier: EUPL-1.2

package incidents_test

import (
	"testing"

	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/incidents"
)

// tempIncidentsDir sets up a fresh ~/Lethean/incidents/ in a temp
// directory by overriding the home dir via the test environment.
// Tests that exercise filesystem I/O use a shared NewService(nil)
// and operate against the process's real home; unit tests that need
// isolation operate against the parse/marshal helpers directly.

func TestParseRecord_RoundTrip(t *testing.T) {
	now := core.Now().UTC()
	rec := subject.IncidentRecord{
		ID:        "2026-05-INC-001",
		Title:     "hub · elevated p99 latency",
		Sev:       "P3",
		State:     "investigating",
		Svc:       "hub",
		Who:       "Mei",
		Comments:  3,
		CreatedAt: now,
	}
	// marshal then parse back — round-trip should be lossless for
	// the searchable header fields.
	raw, err := subject.MarshalRecordExported(rec)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got, err := subject.ParseRecordExported(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got.ID != rec.ID {
		t.Errorf("id: want %q got %q", rec.ID, got.ID)
	}
	if got.Sev != rec.Sev {
		t.Errorf("sev: want %q got %q", rec.Sev, got.Sev)
	}
	if got.State != rec.State {
		t.Errorf("state: want %q got %q", rec.State, got.State)
	}
	if got.Title != rec.Title {
		t.Errorf("title: want %q got %q", rec.Title, got.Title)
	}
}

func TestParseRecord_WithBody(t *testing.T) {
	rec := subject.IncidentRecord{
		ID:    "2026-05-INC-002",
		Title: "DNS cache poisoning",
		Sev:   "P1",
		State: "post-mortem",
		Svc:   "all",
		Who:   "all",
	}
	body := "## Root cause\n\nDNS TTL was too low.\n"
	rec.PostMortem = body
	raw, err := subject.MarshalRecordExported(rec)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got, err := subject.ParseRecordExported(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got.PostMortem != body {
		t.Errorf("body: want %q got %q", body, got.PostMortem)
	}
}

func TestRelativeTime_Now(t *testing.T) {
	now := core.Now()
	got := subject.RelativeTimeExported(now, now)
	if got != "now" {
		t.Errorf("want 'now' got %q", got)
	}
}

func TestRelativeTime_Days(t *testing.T) {
	now := core.Now()
	past := now.Add(-11 * 24 * core.Hour)
	got := subject.RelativeTimeExported(past, now)
	if got != "11 d ago" {
		t.Errorf("want '11 d ago' got %q", got)
	}
}

func TestRelativeTime_Weeks(t *testing.T) {
	now := core.Now()
	// 3 weeks back — should render as "3 w ago"
	past := now.Add(-3 * 7 * 24 * core.Hour)
	got := subject.RelativeTimeExported(past, now)
	if got != "3 w ago" {
		t.Errorf("want '3 w ago' got %q", got)
	}
}

func TestDurString_Minutes(t *testing.T) {
	got := subject.DurStringExported(42)
	if got != "42 min" {
		t.Errorf("want '42 min' got %q", got)
	}
}

func TestDurString_Hours(t *testing.T) {
	got := subject.DurStringExported(134) // 2h 14m
	if got != "2 h 14" {
		t.Errorf("want '2 h 14' got %q", got)
	}
}

func TestServiceName(t *testing.T) {
	svc := subject.NewService(nil)
	if svc.ServiceName() != "Incidents" {
		t.Errorf("want 'Incidents' got %q", svc.ServiceName())
	}
}
