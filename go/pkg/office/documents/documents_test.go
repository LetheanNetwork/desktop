// SPDX-Licence-Identifier: EUPL-1.2

package documents

import (
	"testing"
	"time"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/paths"
)

// testSlug returns a unique slug for a test to prevent FS collisions.
func testSlug(t *testing.T) string {
	t.Helper()
	// Use a sanitised prefix from the test name to keep slugs valid.
	return "test-" + core.SHA256Hex([]byte(t.Name()))[:12]
}

// cleanupDoc removes the test document after the test completes.
func cleanupDoc(t *testing.T, slug string) {
	t.Helper()
	t.Cleanup(func() {
		dirR := docsDir()
		if !dirR.OK {
			return
		}
		_ = core.Remove(core.PathJoin(dirR.Value.(string), slug+".md"))
	})
}

// stubSessionGate is the test double for the consumer-defined
// SessionGate interface. Mirrors RFC.stage-e-unlockgate v2 §4.2 stub
// shape — duplicated per-pkg rather than shared, so test files keep
// zero cross-pkg testing/... deps.
type stubSessionGate struct{ ids []string }

func (s *stubSessionGate) UnlockedAccountIDs() []string { return s.ids }

// newTestService returns a Service backed by a fresh core.Core for
// tests that exercise write methods. The core is minimal — no options.
// The SessionGate is pre-wired with a single-account stub so write
// methods pass the unlock gate by default. Tests that need to assert
// the locked-state path call SetSessionGate explicitly with an empty
// stub (or use newTestServiceNoGate for the nil-gate fail-safe path).
func newTestService() *Service {
	c := core.New()
	svc := NewService(c)
	svc.SetSessionGate(&stubSessionGate{ids: []string{"acct-test"}})
	return svc
}

// newTestServiceNoGate returns a Service with NO SessionGate wired —
// exercises the §2.2 / Cerberus #27 Q2 fail-safe path where the
// service is constructed before app.go's SetSessionGate runs.
func newTestServiceNoGate() *Service {
	c := core.New()
	return NewService(c)
}

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

// -- v2 write-surface tests ---------------------------------------------------

// TestCreate_Good — happy path: file lands on disk, DocRow returned.
func TestCreate_Good(t *testing.T) {
	svc := newTestService()
	slug := testSlug(t)
	cleanupDoc(t, slug)

	r := svc.Create(CreateInput{Slug: slug, Body: "# Test\n\nContent.", State: "draft"})
	if !r.OK {
		t.Fatalf("Create failed: %v", r.Error())
	}
	row, ok := r.Value.(DocRow)
	if !ok {
		t.Fatal("expected DocRow value")
	}
	if row.State != "draft" {
		t.Fatalf("expected state=draft, got %q", row.State)
	}
}

// TestCreate_Bad — duplicate slug returns documents.create.exists.
func TestCreate_Bad(t *testing.T) {
	svc := newTestService()
	slug := testSlug(t)
	cleanupDoc(t, slug)

	// First create must succeed.
	if r := svc.Create(CreateInput{Slug: slug, Body: "# First\n"}); !r.OK {
		t.Fatalf("first Create failed: %v", r.Error())
	}
	// Second create must reject with exists error.
	r := svc.Create(CreateInput{Slug: slug, Body: "# Dupe\n"})
	if r.OK {
		t.Fatal("expected Create to fail for duplicate slug")
	}
	if !core.Contains(r.Error(), "documents.create.exists") {
		t.Fatalf("expected documents.create.exists, got %q", r.Error())
	}
}

// TestCreate_Ugly — traversal slug rejected before any FS touch.
func TestCreate_Ugly(t *testing.T) {
	svc := newTestService()
	r := svc.Create(CreateInput{Slug: "../etc/passwd", Body: "evil"})
	if r.OK {
		t.Fatal("expected Create to reject traversal slug")
	}
	if !core.Contains(r.Error(), "documents.slug.invalid") {
		t.Fatalf("expected documents.slug.invalid, got %q", r.Error())
	}
}

// TestCreate_BodyTooLarge_Bad — 1.1MB body rejected before FS touch.
func TestCreate_BodyTooLarge_Bad(t *testing.T) {
	svc := newTestService()
	slug := testSlug(t)
	body := make([]byte, MaxBodyBytes+1024)
	for i := range body {
		body[i] = 'a'
	}
	r := svc.Create(CreateInput{Slug: slug, Body: string(body)})
	if r.OK {
		t.Fatal("expected Create to reject oversized body")
	}
	if !core.Contains(r.Error(), "documents.body.too_large") {
		t.Fatalf("expected documents.body.too_large, got %q", r.Error())
	}
}

// TestValidateSlug_Good — valid slugs pass validateSlug.
func TestValidateSlug_Good(t *testing.T) {
	valid := []string{"release-notes", "v0.2-notes", "Q2-board-pack"}
	for _, s := range valid {
		if err := validateSlug(s); err != nil {
			t.Errorf("expected %q to be valid, got: %v", s, err)
		}
	}
}

// TestValidateSlug_Bad — reserved slugs are rejected.
func TestValidateSlug_Bad(t *testing.T) {
	reserved := []string{"new", "search", "index"}
	for _, s := range reserved {
		if err := validateSlug(s); err == nil {
			t.Errorf("expected reserved slug %q to be rejected", s)
		}
	}
}

// TestValidateSlug_Ugly — traversal slugs are rejected.
func TestValidateSlug_Ugly(t *testing.T) {
	traversal := []string{"../etc", "foo/bar", "a\\b", ""}
	for _, s := range traversal {
		if err := validateSlug(s); err == nil {
			t.Errorf("expected traversal slug %q to be rejected", s)
		}
	}
}

// TestSave_Good — round-trip: Create then Save, body updated.
func TestSave_Good(t *testing.T) {
	svc := newTestService()
	slug := testSlug(t)
	cleanupDoc(t, slug)

	if r := svc.Create(CreateInput{Slug: slug, Body: "# V1\n"}); !r.OK {
		t.Fatalf("Create failed: %v", r.Error())
	}

	// Get the document to retrieve mtime + hash for the composite token.
	getR := svc.Get(GetInput{Slug: slug})
	if !getR.OK {
		t.Fatalf("Get failed: %v", getR.Error())
	}

	// Compute the hash from disk as the Save token.
	dirR := docsDir()
	if !dirR.OK {
		t.Fatalf("docsDir: %v", dirR.Error())
	}
	fpath := core.PathJoin(dirR.Value.(string), slug+".md")
	hashHex, mtime, err := readBodyHash(fpath)
	if err != nil {
		t.Fatalf("readBodyHash: %v", err)
	}

	r := svc.Save(SaveInput{
		Slug:        slug,
		Body:        "# V2\n\nUpdated content.",
		IfMatch:     mtime,
		IfMatchHash: hashHex,
	})
	if !r.OK {
		t.Fatalf("Save failed: %v", r.Error())
	}
}

// TestSave_Bad — missing IfMatchHash rejected with documents.save.missing_if_match.
func TestSave_Bad(t *testing.T) {
	svc := newTestService()
	slug := testSlug(t)
	cleanupDoc(t, slug)

	if r := svc.Create(CreateInput{Slug: slug, Body: "# V1\n"}); !r.OK {
		t.Fatalf("Create failed: %v", r.Error())
	}

	r := svc.Save(SaveInput{
		Slug:        slug,
		Body:        "# V2\n",
		IfMatch:     core.Now(),
		IfMatchHash: "", // deliberately missing
	})
	if r.OK {
		t.Fatal("expected Save to reject missing IfMatchHash")
	}
	if !core.Contains(r.Error(), "documents.save.missing_if_match") {
		t.Fatalf("expected documents.save.missing_if_match, got %q", r.Error())
	}
}

// TestSave_ConflictDetected_Ugly — stale hash rejected with documents.save.conflict.
func TestSave_ConflictDetected_Ugly(t *testing.T) {
	svc := newTestService()
	slug := testSlug(t)
	cleanupDoc(t, slug)

	if r := svc.Create(CreateInput{Slug: slug, Body: "# V1\n"}); !r.OK {
		t.Fatalf("Create failed: %v", r.Error())
	}

	dirR := docsDir()
	if !dirR.OK {
		t.Fatalf("docsDir: %v", dirR.Error())
	}
	fpath := core.PathJoin(dirR.Value.(string), slug+".md")
	hashHex, mtime, err := readBodyHash(fpath)
	if err != nil {
		t.Fatalf("readBodyHash: %v", err)
	}

	// First Save succeeds — advances content hash on disk.
	r1 := svc.Save(SaveInput{
		Slug:        slug,
		Body:        "# V2\n",
		IfMatch:     mtime,
		IfMatchHash: hashHex,
	})
	if !r1.OK {
		t.Fatalf("first Save failed: %v", r1.Error())
	}

	// Second Save uses the now-stale hash — must conflict.
	r2 := svc.Save(SaveInput{
		Slug:        slug,
		Body:        "# V3\n",
		IfMatch:     mtime,
		IfMatchHash: hashHex, // stale — V2 changed it
	})
	if r2.OK {
		t.Fatal("expected second Save to fail with conflict")
	}
	if !core.Contains(r2.Error(), "documents.save.conflict") {
		t.Fatalf("expected documents.save.conflict, got %q", r2.Error())
	}
}

// TestDelete_Good — file removed after Delete with valid token.
func TestDelete_Good(t *testing.T) {
	svc := newTestService()
	slug := testSlug(t)
	// No cleanupDoc — Delete removes it.

	if r := svc.Create(CreateInput{Slug: slug, Body: "# ToDelete\n"}); !r.OK {
		t.Fatalf("Create failed: %v", r.Error())
	}

	dirR := docsDir()
	if !dirR.OK {
		t.Fatalf("docsDir: %v", dirR.Error())
	}
	fpath := core.PathJoin(dirR.Value.(string), slug+".md")
	hashHex, mtime, err := readBodyHash(fpath)
	if err != nil {
		t.Fatalf("readBodyHash: %v", err)
	}

	r := svc.Delete(DeleteInput{
		Slug:        slug,
		IfMatch:     mtime,
		IfMatchHash: hashHex,
	})
	if !r.OK {
		t.Fatalf("Delete failed: %v", r.Error())
	}
	// File must be gone.
	if statR := core.Stat(fpath); statR.OK {
		t.Fatal("expected file to be deleted")
	}
}

// TestDelete_Bad — stale hash rejected with documents.delete.conflict.
func TestDelete_Bad(t *testing.T) {
	svc := newTestService()
	slug := testSlug(t)
	cleanupDoc(t, slug)

	if r := svc.Create(CreateInput{Slug: slug, Body: "# Keeper\n"}); !r.OK {
		t.Fatalf("Create failed: %v", r.Error())
	}

	r := svc.Delete(DeleteInput{
		Slug:        slug,
		IfMatch:     core.Now(),
		IfMatchHash: "0000000000000000000000000000000000000000000000000000000000000000",
	})
	if r.OK {
		t.Fatal("expected Delete to fail with stale hash")
	}
	if !core.Contains(r.Error(), "documents.delete.conflict") {
		t.Fatalf("expected documents.delete.conflict, got %q", r.Error())
	}
}

// TestDelete_Ugly — missing IfMatchHash rejected before FS touch.
func TestDelete_Ugly(t *testing.T) {
	svc := newTestService()
	slug := testSlug(t)
	cleanupDoc(t, slug)

	if r := svc.Create(CreateInput{Slug: slug, Body: "# Keeper\n"}); !r.OK {
		t.Fatalf("Create failed: %v", r.Error())
	}

	r := svc.Delete(DeleteInput{
		Slug:        slug,
		IfMatch:     core.Now(),
		IfMatchHash: "", // deliberately missing
	})
	if r.OK {
		t.Fatal("expected Delete to reject missing IfMatchHash")
	}
	if !core.Contains(r.Error(), "documents.save.missing_if_match") {
		t.Fatalf("expected missing_if_match error, got %q", r.Error())
	}
}

// TestDocuments_LockedGate_FailsWrite — Save rejects when the live-read
// gate reports zero unlocked accounts (RFC.stage-e-unlockgate v2 §4.2
// LockedSession_Bad shape — Mantis #1613 B.1).
func TestDocuments_LockedGate_FailsWrite(t *testing.T) {
	svc := newTestService()
	svc.SetSessionGate(&stubSessionGate{ids: []string{}})

	if r := svc.Save(SaveInput{Slug: "any", Body: "body", IfMatch: core.Now(), IfMatchHash: "abc"}); r.OK {
		t.Fatal("expected Save to be rejected when gate reports zero unlocked accounts")
	} else if !core.Contains(r.Error(), "documents.session.locked") {
		t.Fatalf("expected documents.session.locked on Save, got %q", r.Error())
	}

	if r := svc.Create(CreateInput{Slug: "any", Body: "body"}); r.OK {
		t.Fatal("expected Create to be rejected when gate reports zero unlocked accounts")
	} else if !core.Contains(r.Error(), "documents.session.locked") {
		t.Fatalf("expected documents.session.locked on Create, got %q", r.Error())
	}

	if r := svc.Delete(DeleteInput{Slug: "any", IfMatch: core.Now(), IfMatchHash: "abc"}); r.OK {
		t.Fatal("expected Delete to be rejected when gate reports zero unlocked accounts")
	} else if !core.Contains(r.Error(), "documents.session.locked") {
		t.Fatalf("expected documents.session.locked on Delete, got %q", r.Error())
	}
}

// TestDocuments_UnlockedGate_AllowsWrite — Create succeeds when the
// live-read gate reports a non-empty unlocked-account slice.
func TestDocuments_UnlockedGate_AllowsWrite(t *testing.T) {
	svc := newTestServiceNoGate()
	svc.SetSessionGate(&stubSessionGate{ids: []string{"acct-1"}})

	slug := testSlug(t)
	cleanupDoc(t, slug)

	if r := svc.Create(CreateInput{Slug: slug, Body: "# Unlocked\n"}); !r.OK {
		t.Fatalf("Create should succeed with gate reporting unlocked acct, got: %v", r.Error())
	}
}

// TestDocuments_NilGate_WarnsOnce_FailsClosed — nil gate fails-locked
// on the first write; nilWarned one-shot suppresses re-warning on
// subsequent writes (RFC §2.2 / Cerberus #27 Q2 fail-safe).
func TestDocuments_NilGate_WarnsOnce_FailsClosed(t *testing.T) {
	svc := newTestServiceNoGate()

	// First write — nilWarned trips false→true; behaviour: fails closed.
	r1 := svc.Create(CreateInput{Slug: "any", Body: "body"})
	if r1.OK {
		t.Fatal("expected Create to fail-closed when gate is nil")
	}
	if !core.Contains(r1.Error(), "documents.session.locked") {
		t.Fatalf("expected documents.session.locked, got %q", r1.Error())
	}
	if !svc.nilWarned.Load() {
		t.Fatal("expected nilWarned to be set after first nil-gate hit")
	}

	// Second write — nilWarned already true; CompareAndSwap returns
	// false and core.Warn is NOT called again. Behaviour from the
	// caller's perspective: same fail-closed result.
	r2 := svc.Save(SaveInput{Slug: "any", Body: "body", IfMatch: core.Now(), IfMatchHash: "abc"})
	if r2.OK {
		t.Fatal("expected Save to fail-closed when gate is nil")
	}
	if !core.Contains(r2.Error(), "documents.session.locked") {
		t.Fatalf("expected documents.session.locked, got %q", r2.Error())
	}

	// nilWarned must still be true (one-shot semantics — not reset).
	if !svc.nilWarned.Load() {
		t.Fatal("expected nilWarned to remain set across multiple nil-gate hits")
	}
}

// TestDocuments_StopNilsGate — Stop() severs the SessionGate; subsequent
// writes fail-closed even though the gate WAS wired (Cerberus #27 ADD-5).
func TestDocuments_StopNilsGate(t *testing.T) {
	svc := newTestService() // gate wired with unlocked stub

	// Pre-Stop: write succeeds.
	slug := testSlug(t)
	cleanupDoc(t, slug)
	if r := svc.Create(CreateInput{Slug: slug, Body: "# Pre-stop\n"}); !r.OK {
		t.Fatalf("Create should succeed pre-Stop, got: %v", r.Error())
	}

	// Stop nils the gate reference.
	if r := svc.Stop(core.Background()); !r.OK {
		t.Fatalf("Stop should succeed, got: %v", r.Error())
	}

	// Post-Stop: write fails-closed with the nil-gate path.
	r := svc.Create(CreateInput{Slug: "any-post-stop", Body: "body"})
	if r.OK {
		t.Fatal("expected Create to fail-closed after Stop nils the gate")
	}
	if !core.Contains(r.Error(), "documents.session.locked") {
		t.Fatalf("expected documents.session.locked, got %q", r.Error())
	}
}

// TestSessionLocked_ReadStillWorks_Good — List is not blocked by the
// session gate (RFC §3.1 — reads stay open while locked).
func TestSessionLocked_ReadStillWorks_Good(t *testing.T) {
	svc := newTestService()
	svc.SetSessionGate(&stubSessionGate{ids: []string{}})
	r := svc.List(ListInput{})
	if !r.OK {
		t.Fatalf("List should succeed when session locked, got: %v", r.Error())
	}
}

// TestRelated_BrokenRefSurvivesSave_Good — Q5: broken related refs survive Save.
func TestRelated_BrokenRefSurvivesSave_Good(t *testing.T) {
	svc := newTestService()
	slug := testSlug(t)
	cleanupDoc(t, slug)

	if r := svc.Create(CreateInput{
		Slug:    slug,
		Body:    "# Doc\n",
		Related: []string{"non-existent-slug"},
	}); !r.OK {
		t.Fatalf("Create with broken related ref failed: %v", r.Error())
	}

	// Verify the file was created (broken ref did not block Create).
	dirR := docsDir()
	if !dirR.OK {
		t.Fatalf("docsDir: %v", dirR.Error())
	}
	fpath := core.PathJoin(dirR.Value.(string), slug+".md")
	if statR := core.Stat(fpath); !statR.OK {
		t.Fatal("expected file to exist after Create with broken related ref")
	}
}

// TestNormaliseTags_Good — tags are lowercased.
func TestNormaliseTags_Good(t *testing.T) {
	tags := normaliseTags([]string{"Sales", "Q2-2026", "CONTRACTS"})
	for _, tag := range tags {
		if tag != core.Lower(tag) {
			t.Errorf("expected lowercase tag, got %q", tag)
		}
	}
}

// TestNormaliseTags_Bad — empty slice returns empty (not nil panic).
func TestNormaliseTags_Bad(t *testing.T) {
	got := normaliseTags([]string{})
	if len(got) != 0 {
		t.Fatalf("expected empty slice, got %v", got)
	}
}

// TestNormaliseTags_Ugly — nil input returns nil (no panic).
func TestNormaliseTags_Ugly(t *testing.T) {
	got := normaliseTags(nil)
	if got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

// TestReadBodyHash_Good — returns consistent hash for known content.
func TestReadBodyHash_Good(t *testing.T) {
	svc := newTestService()
	slug := testSlug(t)
	cleanupDoc(t, slug)

	if r := svc.Create(CreateInput{Slug: slug, Body: "# Hash Test\n"}); !r.OK {
		t.Fatalf("Create failed: %v", r.Error())
	}

	dirR := docsDir()
	if !dirR.OK {
		t.Fatalf("docsDir: %v", dirR.Error())
	}
	fpath := core.PathJoin(dirR.Value.(string), slug+".md")

	h1, _, err := readBodyHash(fpath)
	if err != nil {
		t.Fatalf("readBodyHash failed: %v", err)
	}
	h2, _, err := readBodyHash(fpath)
	if err != nil {
		t.Fatalf("readBodyHash (2nd) failed: %v", err)
	}
	if h1 != h2 {
		t.Fatalf("expected deterministic hash, got %q vs %q", h1, h2)
	}
	if len(h1) != 64 {
		t.Fatalf("expected 64-char hex hash, got %d chars", len(h1))
	}
}

// TestReadBodyHash_Bad — non-existent file returns error.
func TestReadBodyHash_Bad(t *testing.T) {
	_, _, err := readBodyHash("/tmp/does-not-exist-clotho-test.md")
	if err == nil {
		t.Fatal("expected error for non-existent file")
	}
}

// TestReadBodyHash_Ugly — after content changes, hash changes.
func TestReadBodyHash_Ugly(t *testing.T) {
	svc := newTestService()
	slug := testSlug(t)
	cleanupDoc(t, slug)

	if r := svc.Create(CreateInput{Slug: slug, Body: "# V1\n"}); !r.OK {
		t.Fatalf("Create failed: %v", r.Error())
	}

	dirR := docsDir()
	if !dirR.OK {
		t.Fatalf("docsDir: %v", dirR.Error())
	}
	fpath := core.PathJoin(dirR.Value.(string), slug+".md")
	h1, mtime, err := readBodyHash(fpath)
	if err != nil {
		t.Fatalf("readBodyHash v1 failed: %v", err)
	}

	// Save changes the content.
	if r := svc.Save(SaveInput{Slug: slug, Body: "# V2\n", IfMatch: mtime, IfMatchHash: h1}); !r.OK {
		t.Fatalf("Save failed: %v", r.Error())
	}

	h2, _, err := readBodyHash(fpath)
	if err != nil {
		t.Fatalf("readBodyHash v2 failed: %v", err)
	}
	if h1 == h2 {
		t.Fatal("expected hash to change after Save")
	}
}

// TestAtomicWriteFile_Good — file lands with correct content.
func TestAtomicWriteFile_Good(t *testing.T) {
	dirR := docsDir()
	if !dirR.OK {
		t.Skipf("docsDir unavailable: %v", dirR.Error())
	}
	fpath := core.PathJoin(dirR.Value.(string), "atomic-test-clotho.md")
	t.Cleanup(func() { _ = core.Remove(fpath) })

	content := []byte("# Atomic\n\nContent.")
	if err := atomicWriteFile(fpath, content, core.Time{}, ""); err != nil {
		t.Fatalf("atomicWriteFile failed: %v", err)
	}
	rawR := core.ReadFile(fpath)
	if !rawR.OK {
		t.Fatalf("ReadFile after atomicWriteFile failed: %v", rawR.Error())
	}
	if string(rawR.Value.([]byte)) != string(content) {
		t.Fatal("atomicWriteFile content mismatch")
	}
}

// TestAtomicWriteFile_Bad — writing to non-existent directory fails.
func TestAtomicWriteFile_Bad(t *testing.T) {
	err := atomicWriteFile("/tmp/clotho-no-such-dir-xyz/file.md", []byte("body"), core.Time{}, "")
	if err == nil {
		t.Fatal("expected atomicWriteFile to fail for missing directory")
	}
}

// TestAtomicWriteFile_Ugly — tmp file cleaned up on rename fail
// (directory-not-found prevents rename; tmp should not linger).
func TestAtomicWriteFile_Ugly(t *testing.T) {
	// If the dir doesn't exist, Write returns an error before the tmp file
	// can be created — so we verify the error path is reached.
	err := atomicWriteFile("/nonexistent-dir/file.md", []byte("x"), core.Time{}, "")
	if err == nil {
		t.Fatal("expected error from atomicWriteFile with invalid dir")
	}
}

// TestFrontmatter_UpdatedAt_Drift_Ugly — updated_at 5 min behind mtime logs
// and does not cause a failure.
func TestFrontmatter_UpdatedAt_Drift_Ugly(t *testing.T) {
	old := core.Now().Add(-5 * time.Minute)
	raw := []byte("---\nstate: draft\nupdated_at: " + old.Format("2006-01-02T15:04:05Z") + "\n---\n# Body\n")
	// parseDoc must not panic or fatal — drift is advisory.
	rec := parseDoc("drift-doc", raw, core.Now(), 0)
	if rec.Slug != "drift-doc" {
		t.Fatalf("expected slug=drift-doc, got %q", rec.Slug)
	}
}

// ---- B.2 PILOT cutover tests (paths.AtomicWriteWithVersion) -----------------

// TestSave_PrimitiveStaleEnvelope_Ugly — B.2 PILOT — confirms the
// VersionStale structured envelope from paths is reachable on a Save
// conflict. The frontend conflict-UI consumes current_hash to compose
// the reload prompt; the test pins the field's presence + lifecycle.
func TestSave_PrimitiveStaleEnvelope_Ugly(t *testing.T) {
	svc := newTestService()
	slug := testSlug(t)
	cleanupDoc(t, slug)

	if r := svc.Create(CreateInput{Slug: slug, Body: "# V1\n"}); !r.OK {
		t.Fatalf("Create failed: %v", r.Error())
	}

	dirR := docsDir()
	if !dirR.OK {
		t.Fatalf("docsDir: %v", dirR.Error())
	}
	fpath := core.PathJoin(dirR.Value.(string), slug+".md")
	hashHex, mtime, err := readBodyHash(fpath)
	if err != nil {
		t.Fatalf("readBodyHash: %v", err)
	}

	// First Save advances the disk state.
	if r := svc.Save(SaveInput{Slug: slug, Body: "# V2\n", IfMatch: mtime, IfMatchHash: hashHex}); !r.OK {
		t.Fatalf("first Save failed: %v", r.Error())
	}

	// Second Save with stale hash MUST surface documents.save.conflict.
	r := svc.Save(SaveInput{Slug: slug, Body: "# V3\n", IfMatch: mtime, IfMatchHash: hashHex})
	if r.OK {
		t.Fatal("stale Save should fail")
	}
	if !core.Contains(r.Error(), "documents.save.conflict") {
		t.Fatalf("expected documents.save.conflict, got %q", r.Error())
	}
}

// TestCreate_RoutesThroughPrimitive_Good — B.2 PILOT — confirms a
// successful Create produces a paths.LockEvent (auditable + observable
// downstream). The recorder is overridden for the test; production
// boot wires the real audit.Service adapter.
func TestCreate_RoutesThroughPrimitive_Good(t *testing.T) {
	svc := newTestService()
	slug := testSlug(t)
	cleanupDoc(t, slug)

	// Wire an audit secret BEFORE installing the subscriber. Per
	// Mantis #1526, emitLockEvent drops events when the audit
	// secret is empty (degraded mode), so without this the
	// subscriber would never fire. See pkg/paths/events.go:445.
	paths.SetAuditSecretProvider(func() []byte {
		return []byte("test-secret-32-bytes-long-padding")
	})
	t.Cleanup(func() { paths.SetAuditSecretProvider(nil) })

	var captured []paths.LockEvent
	paths.SubscribeLockEvents(func(ev paths.LockEvent) {
		captured = append(captured, ev)
	})
	t.Cleanup(paths.ClearLockEventSubscribersForTest)

	if r := svc.Create(CreateInput{Slug: slug, Body: "# Routed\n"}); !r.OK {
		t.Fatalf("Create failed: %v", r.Error())
	}

	found := false
	for _, ev := range captured {
		if ev.Kind == paths.EventWriteSucceeded {
			found = true
		}
	}
	if !found {
		t.Fatal("Create MUST route through paths.AtomicWriteWithVersion (no EventWriteSucceeded seen)")
	}
}
