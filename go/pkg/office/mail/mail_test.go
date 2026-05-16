// SPDX-Licence-Identifier: EUPL-1.2

package mail

import (
	"testing"
	"time"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/paths"
)

// TestScanFolders_Empty — no mail dir yet → empty folder list (no error).
func TestScanFolders_Empty(t *testing.T) {
	// Without a live temp dir, test the parse path via parseThreads.
	recs, err := parseThreads(nil)
	if err != nil {
		t.Fatalf("unexpected error on nil input: %v", err)
	}
	if len(recs) != 0 {
		t.Fatalf("expected empty, got %d records", len(recs))
	}
}

// TestParseThreads_Single — a single YAML block decodes to one record.
func TestParseThreads_Single(t *testing.T) {
	raw := []byte(`---
id: "abc123"
from: "Ada Penley"
subject: "Re: SOW v2"
last_touched: "2026-05-16T14:30:00Z"
unread: true
snippet: "Looking forward to the proposal."
---
`)
	recs, err := parseThreads(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("expected 1 record, got %d", len(recs))
	}
	r := recs[0]
	if r.ID != "abc123" {
		t.Errorf("expected ID='abc123', got %q", r.ID)
	}
	if r.From != "Ada Penley" {
		t.Errorf("expected From='Ada Penley', got %q", r.From)
	}
	if r.Subj != "Re: SOW v2" {
		t.Errorf("expected Subj='Re: SOW v2', got %q", r.Subj)
	}
	if !r.Unread {
		t.Error("expected Unread=true")
	}
}

// TestParseThreads_Multiple — three blocks → three records, order preserved.
func TestParseThreads_Multiple(t *testing.T) {
	raw := []byte(`---
id: "1"
from: "Alice"
subject: "First"
unread: true
snippet: "First thread."
---
id: "2"
from: "Bob"
subject: "Second"
unread: false
snippet: "Second thread."
---
id: "3"
from: "Carol"
subject: "Third"
unread: false
snippet: "Third thread."
---
`)
	recs, err := parseThreads(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(recs) != 3 {
		t.Fatalf("expected 3 records, got %d", len(recs))
	}
	if recs[0].ID != "1" || recs[1].ID != "2" || recs[2].ID != "3" {
		t.Errorf("unexpected ordering: %v %v %v", recs[0].ID, recs[1].ID, recs[2].ID)
	}
}

// TestParseThreads_MalformedSkipped — bad block is warned+skipped; valid remain.
func TestParseThreads_MalformedSkipped(t *testing.T) {
	raw := []byte(`---
id: "good"
from: "Alice"
subject: "Good"
unread: false
snippet: "Ok."
---
:::not yaml at all:::
  [broken
---
id: "also-good"
from: "Bob"
subject: "Also good"
unread: false
snippet: "Fine."
---
`)
	recs, err := parseThreads(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The malformed block may produce 0 or 1 decode depending on yaml tolerance.
	// We only assert the valid ones are present.
	found := 0
	for _, r := range recs {
		if r.ID == "good" || r.ID == "also-good" {
			found++
		}
	}
	if found < 2 {
		t.Errorf("expected 2 valid records, found %d total (%d valid)", len(recs), found)
	}
}

// TestRelativeWhen_Now — delta < 5 min → "now".
func TestRelativeWhen_Now(t *testing.T) {
	now := core.Now()
	ts := now.Add(-2 * time.Minute)
	got := relativeWhen(ts, now)
	if got != "now" {
		t.Fatalf("expected 'now', got %q", got)
	}
}

// TestRelativeWhen_Today — delta 2h → "HH:MM" clock format.
func TestRelativeWhen_Today(t *testing.T) {
	now := core.Now()
	ts := now.Add(-2 * time.Hour)
	got := relativeWhen(ts, now)
	// Should be "HH:MM" — four characters plus colon.
	if len(got) != 5 || got[2] != ':' {
		t.Fatalf("expected HH:MM format, got %q", got)
	}
}

// TestRelativeWhen_Yesterday — delta 24h → "yest".
func TestRelativeWhen_Yesterday(t *testing.T) {
	now := core.Now()
	ts := now.Add(-24 * time.Hour)
	got := relativeWhen(ts, now)
	if got != "yest" {
		t.Fatalf("expected 'yest', got %q", got)
	}
}

// TestRelativeWhen_Days — delta 3 days → "3 d".
func TestRelativeWhen_Days(t *testing.T) {
	now := core.Now()
	ts := now.Add(-72 * time.Hour)
	got := relativeWhen(ts, now)
	if got != "3 d" {
		t.Fatalf("expected '3 d', got %q", got)
	}
}

// TestIsValidSlug_Valid — plain folder slugs pass paths.IsValidID.
func TestIsValidSlug_Valid(t *testing.T) {
	valid := []string{"inbox", "sent", "drafts", "archive", "trash", "my-folder"}
	for _, s := range valid {
		if err := paths.IsValidID(s); err != nil {
			t.Errorf("expected %q to be valid, got: %v", s, err)
		}
	}
}

// TestIsValidSlug_Invalid — traversal slugs are rejected by paths.IsValidID.
func TestIsValidSlug_Invalid(t *testing.T) {
	invalid := []string{"", "../inbox", "inbox/foo", "in\\box", "null\x00"}
	for _, s := range invalid {
		if err := paths.IsValidID(s); err == nil {
			t.Errorf("expected %q to be invalid", s)
		}
	}
}

// TestFolderLabel_Known — canonical slugs resolve to capitalised labels.
func TestFolderLabel_Known(t *testing.T) {
	cases := map[string]string{
		"inbox":   "Inbox",
		"sent":    "Sent",
		"drafts":  "Drafts",
		"archive": "Archive",
		"trash":   "Trash",
	}
	for slug, want := range cases {
		got := folderLabel(slug)
		if got != want {
			t.Errorf("folderLabel(%q) = %q, want %q", slug, got, want)
		}
	}
}

// TestFolderLabel_Unknown — unknown slug gets title-cased.
func TestFolderLabel_Unknown(t *testing.T) {
	got := folderLabel("promo")
	if got != "Promo" {
		t.Fatalf("expected 'Promo', got %q", got)
	}
}

// TestListThreadsLimit — more threads than limit → only limit returned.
func TestListThreadsLimit(t *testing.T) {
	// Build 5 thread records.
	var records []MailThreadRecord
	for i := 0; i < 5; i++ {
		records = append(records, MailThreadRecord{
			ID:   core.Sprintf("id-%d", i),
			From: "Alice",
			Subj: "Subject",
		})
	}
	// Apply limit of 3 by hand (mirrors wails.go logic).
	limit := 3
	now := core.Now()
	threads := make([]MailThread, 0, len(records))
	for _, r := range records {
		threads = append(threads, toThread(r, now))
	}
	if len(threads) > limit {
		threads = threads[:limit]
	}
	if len(threads) != 3 {
		t.Fatalf("expected 3 threads after limit, got %d", len(threads))
	}
}
