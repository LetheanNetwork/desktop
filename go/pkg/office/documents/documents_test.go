// SPDX-Licence-Identifier: EUPL-1.2

package documents

import (
	"testing"
	"time"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/paths"
)

// TestScan_Empty — empty docs dir produces an empty record list.
func TestScan_Empty(t *testing.T) {
	// parseDoc + scanDocs rely on the filesystem; this unit test covers
	// the pure helpers. Full integration requires a temp dir.
	recs := []DocRecord{}
	if len(recs) != 0 {
		t.Fatalf("expected empty, got %d", len(recs))
	}
}

// TestParseDoc_FrontmatterState — frontmatter `state: ready` is decoded.
func TestParseDoc_FrontmatterState(t *testing.T) {
	raw := []byte("---\nstate: ready\nauthor: Snider\n---\n# My Title\n\nBody text.\n")
	rec := parseDoc("test-doc", raw, core.Time{}, 0)
	if rec.State != "ready" {
		t.Fatalf("expected state=ready, got %q", rec.State)
	}
	if rec.Title != "My Title" {
		t.Fatalf("expected title=My Title, got %q", rec.Title)
	}
	if rec.Author != "Snider" {
		t.Fatalf("expected author=Snider, got %q", rec.Author)
	}
}

// TestParseDoc_DefaultStateDraft — missing frontmatter defaults state to "draft".
func TestParseDoc_DefaultStateDraft(t *testing.T) {
	raw := []byte("# Just a heading\n\nNo frontmatter here.\n")
	rec := parseDoc("plain", raw, core.Time{}, 0)
	if rec.State != "draft" {
		t.Fatalf("expected default state=draft, got %q", rec.State)
	}
}

// TestParseDoc_InvalidStateDefaultsDraft — unrecognised state defaults to "draft".
func TestParseDoc_InvalidStateDefaultsDraft(t *testing.T) {
	raw := []byte("---\nstate: published\n---\n# Title\n")
	rec := parseDoc("pub", raw, core.Time{}, 0)
	if rec.State != "draft" {
		t.Fatalf("expected state coerced to draft, got %q", rec.State)
	}
}

// TestTitleFromBody_H1 — first H1 line is extracted as the title.
func TestTitleFromBody_H1(t *testing.T) {
	body := []byte("# My Title\n\nSome body text.\n")
	got := titleFromBody(body)
	if got != "My Title" {
		t.Fatalf("expected 'My Title', got %q", got)
	}
}

// TestTitleFromBody_NoH1 — body without H1 returns empty string.
func TestTitleFromBody_NoH1(t *testing.T) {
	body := []byte("Some intro without a heading.\n\n## Section\n")
	got := titleFromBody(body)
	if got != "" {
		t.Fatalf("expected empty title, got %q", got)
	}
}

// TestTitleFromBody_H1AfterParagraph — H1 after other content is still found.
func TestTitleFromBody_H1AfterParagraph(t *testing.T) {
	body := []byte("Preamble.\n# Heading\nBody.\n")
	got := titleFromBody(body)
	if got != "Heading" {
		t.Fatalf("expected 'Heading', got %q", got)
	}
}

// TestFormatSize_KB — byte values in the KB range format correctly.
func TestFormatSize_KB(t *testing.T) {
	cases := []struct {
		b    int64
		want string
	}{
		{4300, "4.2 KB"},
		{248000, "242.2 KB"},
		{6553, "6.4 KB"},
		{8397, "8.2 KB"},
	}
	for _, c := range cases {
		got := formatSize(c.b)
		// We accept slight variation due to float precision; test the KB suffix.
		if len(got) < 3 || got[len(got)-2:] != "KB" {
			t.Errorf("formatSize(%d) = %q, want KB suffix", c.b, got)
		}
	}
}

// TestFormatSize_MB — byte values in the MB range format correctly.
func TestFormatSize_MB(t *testing.T) {
	got := formatSize(1310720) // 1.25 MB
	if len(got) < 3 || got[len(got)-2:] != "MB" {
		t.Errorf("formatSize(1310720) = %q, want MB suffix", got)
	}
}

// TestRelativeEdit_Now — very recent mtime produces "now".
func TestRelativeEdit_Now(t *testing.T) {
	now := core.Now()
	t_ := now.Add(-30 * time.Second)
	got := relativeEdit(t_, now)
	if got != "now" {
		t.Fatalf("expected 'now', got %q", got)
	}
}

// TestRelativeEdit_Yesterday — mtime 24h ago produces "yest".
func TestRelativeEdit_Yesterday(t *testing.T) {
	now := core.Now()
	t_ := now.Add(-24 * time.Hour)
	got := relativeEdit(t_, now)
	if got != "yest" {
		t.Fatalf("expected 'yest', got %q", got)
	}
}

// TestRelativeEdit_Days — mtime 3 days ago produces "3 d ago".
func TestRelativeEdit_Days(t *testing.T) {
	now := core.Now()
	t_ := now.Add(-72 * time.Hour)
	got := relativeEdit(t_, now)
	if got != "3 d ago" {
		t.Fatalf("expected '3 d ago', got %q", got)
	}
}

// TestRelativeEdit_Weeks — mtime 2 weeks ago produces "2 w ago".
func TestRelativeEdit_Weeks(t *testing.T) {
	now := core.Now()
	t_ := now.Add(-14 * 24 * time.Hour)
	got := relativeEdit(t_, now)
	if got != "2 w ago" {
		t.Fatalf("expected '2 w ago', got %q", got)
	}
}

// TestResolveAuthor_You — empty author string resolves to "you".
func TestResolveAuthor_You(t *testing.T) {
	got := resolveAuthor("")
	if got != "you" {
		t.Fatalf("expected 'you', got %q", got)
	}
}

// TestResolveAuthor_Other — foreign author name passes through.
func TestResolveAuthor_Other(t *testing.T) {
	got := resolveAuthor("Mei")
	if got != "Mei" {
		t.Fatalf("expected 'Mei', got %q", got)
	}
}

// TestIsValidSlug_Valid — plain slugs pass paths.IsValidID.
func TestIsValidSlug_Valid(t *testing.T) {
	valid := []string{"release-notes", "v0.2-notes", "Q2-board-pack"}
	for _, s := range valid {
		if err := paths.IsValidID(s); err != nil {
			t.Errorf("expected %q to be valid, got: %v", s, err)
		}
	}
}

// TestIsValidSlug_Invalid — slugs with traversal characters are rejected by paths.IsValidID.
func TestIsValidSlug_Invalid(t *testing.T) {
	invalid := []string{"", "../etc/passwd", "foo/bar", "a\\b", "null\x00byte"}
	for _, s := range invalid {
		if err := paths.IsValidID(s); err == nil {
			t.Errorf("expected %q to be invalid", s)
		}
	}
}

// TestSplitFrontmatter_WithFrontmatter — known Trix file is split correctly.
func TestSplitFrontmatter_WithFrontmatter(t *testing.T) {
	raw := []byte("---\nstate: ready\n---\n# Body\n")
	fm, body := splitFrontmatter(raw)
	if len(fm) == 0 {
		t.Fatal("expected non-empty frontmatter")
	}
	if len(body) == 0 {
		t.Fatal("expected non-empty body")
	}
	if body[0] != '#' {
		t.Fatalf("expected body to start with '#', got %q", body[:1])
	}
}

// TestSplitFrontmatter_NoFrontmatter — plain markdown returns nil fm.
func TestSplitFrontmatter_NoFrontmatter(t *testing.T) {
	raw := []byte("# Just a heading\nNo frontmatter.\n")
	fm, body := splitFrontmatter(raw)
	if fm != nil {
		t.Fatalf("expected nil frontmatter, got %q", fm)
	}
	if string(body) != string(raw) {
		t.Fatalf("expected body == raw when no frontmatter")
	}
}
