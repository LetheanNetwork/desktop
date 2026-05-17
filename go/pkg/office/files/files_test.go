// SPDX-Licence-Identifier: EUPL-1.2

package files

import (
	"testing"
	"time"

	core "dappco.re/go"
)

// TestLocationCatalogue_Length — default catalogue has 5 entries.
func TestLocationCatalogue_Length(t *testing.T) {
	specs := locationCatalogue("/Users/test")
	if len(specs) != 5 {
		t.Fatalf("expected 5 location specs, got %d", len(specs))
	}
}

// TestLocationCatalogue_ModelsIsBrand — the models entry has Brand=true.
func TestLocationCatalogue_ModelsIsBrand(t *testing.T) {
	specs := locationCatalogue("/Users/test")
	found := false
	for _, spec := range specs {
		if spec.Brand {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected at least one Brand=true location in the catalogue")
	}
}

// TestFormatSizeGB_TB — 1 TB in bytes → "1 TB".
func TestFormatSizeGB_TB(t *testing.T) {
	got := formatSizeGB(1_000_000_000_000)
	if got != "1 TB" {
		t.Fatalf("expected '1 TB', got %q", got)
	}
}

// TestFormatSizeGB_GB — 4.2 GB in bytes → "4.2 GB".
func TestFormatSizeGB_GB(t *testing.T) {
	got := formatSizeGB(4_200_000_000)
	if got != "4.2 GB" {
		t.Fatalf("expected '4.2 GB', got %q", got)
	}
}

// TestFormatSizeGB_GB_Whole — whole GB → no decimal.
func TestFormatSizeGB_GB_Whole(t *testing.T) {
	got := formatSizeGB(2_000_000_000)
	if got != "2 GB" {
		t.Fatalf("expected '2 GB', got %q", got)
	}
}

// TestFormatSizeGB_MB — 180 MB in bytes → "180 MB".
func TestFormatSizeGB_MB(t *testing.T) {
	got := formatSizeGB(180_000_000)
	if got != "180 MB" {
		t.Fatalf("expected '180 MB', got %q", got)
	}
}

// TestFormatSize_KB — file-level KB formatting.
func TestFormatSize_KB(t *testing.T) {
	got := formatSize(38_000)
	if len(got) < 3 || got[len(got)-2:] != "KB" {
		t.Errorf("formatSize(38000) = %q, want KB suffix", got)
	}
}

// TestFormatSize_GB — large file uses GB suffix.
func TestFormatSize_GB(t *testing.T) {
	got := formatSize(2_100_000_000)
	if len(got) < 3 || got[len(got)-2:] != "GB" {
		t.Errorf("formatSize(2.1GB) = %q, want GB suffix", got)
	}
}

// TestRelativeWhen_Today — file modified 2h ago → HH:MM format.
func TestRelativeWhen_Today(t *testing.T) {
	now := core.Now()
	ts := now.Add(-2 * time.Hour)
	got := relativeWhen(ts, now)
	if len(got) != 5 || got[2] != ':' {
		t.Fatalf("expected HH:MM format, got %q", got)
	}
}

// TestRelativeWhen_Yesterday — file modified 24h ago → "yest".
func TestRelativeWhen_Yesterday(t *testing.T) {
	now := core.Now()
	ts := now.Add(-24 * time.Hour)
	got := relativeWhen(ts, now)
	if got != "yest" {
		t.Fatalf("expected 'yest', got %q", got)
	}
}

// TestRelativeWhen_Days — file modified 3 days ago → "3 d".
func TestRelativeWhen_Days(t *testing.T) {
	now := core.Now()
	ts := now.Add(-72 * time.Hour)
	got := relativeWhen(ts, now)
	if got != "3 d" {
		t.Fatalf("expected '3 d', got %q", got)
	}
}

// TestCollapseHome_Collapse — $HOME prefix is replaced with "~".
func TestCollapseHome_Collapse(t *testing.T) {
	home := "/Users/snider"
	got := collapseHome("/Users/snider/Documents/", home)
	if got != "~/Documents/" {
		t.Fatalf("expected '~/Documents/', got %q", got)
	}
}

// TestCollapseHome_NoMatch — non-home path is returned unchanged.
func TestCollapseHome_NoMatch(t *testing.T) {
	home := "/Users/snider"
	got := collapseHome("/tmp/foo", home)
	if got != "/tmp/foo" {
		t.Fatalf("expected '/tmp/foo', got %q", got)
	}
}

// TestScanLocation_MissingPath — non-existent path returns (0, 0, nil).
func TestScanLocation_MissingPath(t *testing.T) {
	spec := locationSpec{Name: "Ghost", Path: "/this/does/not/exist/ever"}
	count, sizeB, err := scanLocation(spec)
	if err != nil {
		t.Fatalf("expected nil error for missing path, got: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected count=0, got %d", count)
	}
	if sizeB != 0 {
		t.Fatalf("expected sizeB=0, got %d", sizeB)
	}
}

// TestRecentRow_NameWithBidiOverride_Sanitised_Bad — a U+202E
// (right-to-left override) in a filename is replaced with U+FFFD so the
// glyphs render in source order. Mantis #1504 Trojan Source defence.
func TestRecentRow_NameWithBidiOverride_Sanitised_Bad(t *testing.T) {
	// "legitimate" + U+202E + "kcatta.exe" — renders as
	// "legitimateexe.attack" in a bidi-aware UI.
	in := "legitimate‮kcatta.exe"
	got := sanitizeFilename(in)
	want := "legitimate�kcatta.exe"
	if got != want {
		t.Fatalf("sanitizeFilename(%q) = %q, want %q", in, got, want)
	}
	for _, r := range got {
		if r == '‮' {
			t.Fatalf("sanitised output still contains U+202E: %q", got)
		}
	}
}

// TestRecentRow_NameWithIsolateControl_Sanitised_Bad — a U+2066
// (left-to-right isolate) is replaced with U+FFFD.
func TestRecentRow_NameWithIsolateControl_Sanitised_Bad(t *testing.T) {
	in := "alpha⁦bravo⁩.txt"
	got := sanitizeFilename(in)
	want := "alpha�bravo�.txt"
	if got != want {
		t.Fatalf("sanitizeFilename(%q) = %q, want %q", in, got, want)
	}
}

// TestRecentRow_NameWithBidiFamily_Sanitised_Bad — every replaced
// codepoint in the documented ranges is in fact replaced. Each case
// shows the codepoint, the input string, and the expected output.
func TestRecentRow_NameWithBidiFamily_Sanitised_Bad(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"U+202A LRE", "a‪b", "a�b"},
		{"U+202B RLE", "a‫b", "a�b"},
		{"U+202C PDF", "a‬b", "a�b"},
		{"U+202D LRO", "a‭b", "a�b"},
		{"U+202E RLO", "a‮b", "a�b"},
		{"U+2066 LRI", "a⁦b", "a�b"},
		{"U+2067 RLI", "a⁧b", "a�b"},
		{"U+2068 FSI", "a⁨b", "a�b"},
		{"U+2069 PDI", "a⁩b", "a�b"},
		{"U+200E LRM", "a‎b", "a�b"},
		{"U+200F RLM", "a‏b", "a�b"},
		{"U+0000 NUL", "a\x00b", "a�b"},
		{"U+0009 TAB", "a\tb", "a�b"},
		{"U+001B ESC", "a\x1bb", "a�b"},
	}
	for _, c := range cases {
		got := sanitizeFilename(c.in)
		if got != c.want {
			t.Errorf("%s: sanitizeFilename(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}

// TestRecentRow_PlainNameUnchanged_Good — alphanumeric + space + punct
// filenames pass through untouched (no over-strip).
func TestRecentRow_PlainNameUnchanged_Good(t *testing.T) {
	cases := []string{
		"report.pdf",
		"Q3 2026 forecast.xlsx",
		"sow-v2.md",
		"file (1).png",
		"archive.tar.gz",
		"hello_world.go",
		"data[2026-05-17].csv",
	}
	for _, in := range cases {
		got := sanitizeFilename(in)
		if got != in {
			t.Errorf("sanitizeFilename(%q) = %q, expected unchanged", in, got)
		}
	}
}

// TestRecentRow_UnicodeAccentsPreserved_Good — Latin/CJK/Cyrillic
// filenames must survive sanitisation (the sweep is bidi-control only,
// not ASCII-only).
func TestRecentRow_UnicodeAccentsPreserved_Good(t *testing.T) {
	cases := []string{
		"café.txt",
		"naïve approach.md",
		"über-plan.pdf",
		"日本語.txt",
		"Москва.doc",
		"αβγ.md",
		"résumé-2026.pdf",
		"piñata.jpg",
	}
	for _, in := range cases {
		got := sanitizeFilename(in)
		if got != in {
			t.Errorf("sanitizeFilename(%q) = %q, expected unchanged", in, got)
		}
	}
}

// TestRecentRow_EmptyName_Good — empty input returns empty output, no
// allocation surprise.
func TestRecentRow_EmptyName_Good(t *testing.T) {
	if got := sanitizeFilename(""); got != "" {
		t.Fatalf("sanitizeFilename(\"\") = %q, want \"\"", got)
	}
}

// TestRecentRow_OnlyBidi_Ugly — a filename consisting ENTIRELY of bidi
// controls collapses to a string of U+FFFD chars (still visible, still
// distinguishable from empty — operator can see something went wrong).
func TestRecentRow_OnlyBidi_Ugly(t *testing.T) {
	in := "‮‮⁦"
	got := sanitizeFilename(in)
	want := "���"
	if got != want {
		t.Fatalf("sanitizeFilename(%q) = %q, want %q", in, got, want)
	}
}
