// SPDX-Licence-Identifier: EUPL-1.2

// loadall_fault_test.go — fault injection for loadItems' per-entry skip
// branches (RFC.stage-e-encrypt-at-rest v2 §4.1: "do NOT abort whole
// List on one bad file") plus the List/Get/Advance/Create wails.go
// branches that only trigger with a populated, non-trivial calendar
// (Col filter, totalInFlight/dueToday tallies, not-found forwarding,
// session-locked-after-atrest forwarding). Mirrors pkg/sales/deals/
// loadall_fault_test.go's precedent.

package content_test

import (
	"os"
	"testing"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/marketing/content"
)

// TestContent_LoadItems_CorruptLthn_SkippedNotAborted_Ugly — a .lthn
// file too short/malformed to survive loadHeaderOnly is skipped; a
// healthy .md sibling still surfaces via List.
func TestContent_LoadItems_CorruptLthn_SkippedNotAborted_Ugly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newAtRestTestSvc(t)

	dir := core.PathJoin(home, "Lethean", "marketing", "content")
	if mk := core.MkdirAll(dir, 0o700); !mk.OK {
		t.Fatalf("MkdirAll: %s", mk.Error())
	}
	if w := core.WriteFile(core.PathJoin(dir, "corrupt-entry.lthn"), []byte("not-a-real-envelope"), 0o600); !w.OK {
		t.Fatalf("seed corrupt .lthn: %s", w.Error())
	}

	cr := svc.Create(content.CreateInput{T: "Healthy encrypted item", Col: "idea"})
	if !cr.OK {
		t.Fatalf("seed Create: %s", cr.Error())
	}

	r := svc.List(content.ListInput{})
	if !r.OK {
		t.Fatalf("List must tolerate one corrupt .lthn entry: %s", r.Error())
	}
	out := r.Value.(content.ListOutput)
	total := 0
	for _, col := range out.Columns {
		total += len(col.Items)
	}
	if total != 1 {
		t.Fatalf("expected exactly the healthy record (corrupt one skipped), got %d", total)
	}
}

// TestContent_LoadItems_UnreadableMd_SkippedNotAborted_Bad — a .md
// file with permissions denying read hits loadItems' `if !raw.OK {
// continue }` branch. Runs on the legacy (non-keyed) gate so the entry
// is a plaintext .md candidate, not shadowed by a preferred .lthn.
func TestContent_LoadItems_UnreadableMd_SkippedNotAborted_Bad(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root — permission-denial fault injection does not apply")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := content.NewService(nil)
	svc.SetSessionGate(&stubSessionGate{ids: []string{"acct-legacy"}})

	dir := core.PathJoin(home, "Lethean", "marketing", "content")
	if mk := core.MkdirAll(dir, 0o700); !mk.OK {
		t.Fatalf("MkdirAll: %s", mk.Error())
	}
	unreadable := core.PathJoin(dir, "unreadable-item.md")
	legacy := []byte("---\nid: unreadable-item\nt: Locked\nwho: \"\"\nwhen: \"\"\ndue: \"\"\ncol: idea\n---\n")
	if w := core.WriteFile(unreadable, legacy, 0o600); !w.OK {
		t.Fatalf("seed .md: %s", w.Error())
	}
	if err := os.Chmod(unreadable, 0o000); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(unreadable, 0o600) })

	cr := svc.Create(content.CreateInput{T: "Healthy legacy item", Col: "idea"})
	if !cr.OK {
		t.Fatalf("seed Create: %s", cr.Error())
	}

	r := svc.List(content.ListInput{})
	if !r.OK {
		t.Fatalf("List must tolerate an unreadable .md entry: %s", r.Error())
	}
	out := r.Value.(content.ListOutput)
	total := 0
	for _, col := range out.Columns {
		total += len(col.Items)
	}
	if total != 1 {
		t.Fatalf("expected exactly the healthy record (unreadable one skipped), got %d", total)
	}
}

// TestContent_LoadItems_MalformedMdYaml_SkippedNotAborted_Bad — a .md
// file whose frontmatter fails yaml.Unmarshal hits loadItems'
// parseItem-error `continue` branch.
func TestContent_LoadItems_MalformedMdYaml_SkippedNotAborted_Bad(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := content.NewService(nil)
	svc.SetSessionGate(&stubSessionGate{ids: []string{"acct-legacy"}})

	dir := core.PathJoin(home, "Lethean", "marketing", "content")
	if mk := core.MkdirAll(dir, 0o700); !mk.OK {
		t.Fatalf("MkdirAll: %s", mk.Error())
	}
	bad := core.PathJoin(dir, "malformed-item.md")
	if w := core.WriteFile(bad, []byte("---\n[not: valid: yaml\n---\nbody"), 0o600); !w.OK {
		t.Fatalf("seed malformed .md: %s", w.Error())
	}

	cr := svc.Create(content.CreateInput{T: "Healthy legacy item two", Col: "idea"})
	if !cr.OK {
		t.Fatalf("seed Create: %s", cr.Error())
	}

	r := svc.List(content.ListInput{})
	if !r.OK {
		t.Fatalf("List must tolerate a malformed .md entry: %s", r.Error())
	}
	out := r.Value.(content.ListOutput)
	total := 0
	for _, col := range out.Columns {
		total += len(col.Items)
	}
	if total != 1 {
		t.Fatalf("expected exactly the healthy record (malformed YAML skipped), got %d", total)
	}
}

// TestContent_LoadItems_SubdirEntry_Ignored_Good — a stray subdirectory
// inside the content dir must be ignored by both loadItems passes
// (entry.IsDir() continue) rather than causing an error.
func TestContent_LoadItems_SubdirEntry_Ignored_Good(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newAtRestTestSvc(t)

	dir := core.PathJoin(home, "Lethean", "marketing", "content")
	if mk := core.MkdirAll(core.PathJoin(dir, "stray-subdir"), 0o700); !mk.OK {
		t.Fatalf("MkdirAll: %s", mk.Error())
	}

	cr := svc.Create(content.CreateInput{T: "Item beside a stray subdir", Col: "idea"})
	if !cr.OK {
		t.Fatalf("seed Create: %s", cr.Error())
	}

	r := svc.List(content.ListInput{})
	if !r.OK {
		t.Fatalf("List must ignore a stray subdirectory: %s", r.Error())
	}
}

// TestList_ColFilter_Good — ListInput.Col restricts the returned
// columns to the single matching one.
func TestList_ColFilter_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newTestSvc(t)
	if r := svc.Create(content.CreateInput{T: "Idea item", Col: "idea"}); !r.OK {
		t.Fatalf("Create idea: %s", r.Error())
	}
	if r := svc.Create(content.CreateInput{T: "Draft item", Col: "draft"}); !r.OK {
		t.Fatalf("Create draft: %s", r.Error())
	}

	r := svc.List(content.ListInput{Col: "draft"})
	if !r.OK {
		t.Fatalf("List: %s", r.Error())
	}
	out := r.Value.(content.ListOutput)
	if len(out.Columns) != 1 {
		t.Fatalf("expected exactly 1 column for Col filter, got %d", len(out.Columns))
	}
	if out.Columns[0].ID != "draft" {
		t.Fatalf("expected draft column, got %q", out.Columns[0].ID)
	}
}

// TestList_TotalInFlightAndDueToday_Good — a non-"live" item counts
// toward TotalInFlight; a Due=="today" item counts toward DueToday.
func TestList_TotalInFlightAndDueToday_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newTestSvc(t)
	if r := svc.Create(content.CreateInput{T: "In-flight, due today", Col: "draft", Due: "today"}); !r.OK {
		t.Fatalf("Create: %s", r.Error())
	}
	if r := svc.Create(content.CreateInput{T: "Live item", Col: "live"}); !r.OK {
		t.Fatalf("Create: %s", r.Error())
	}

	r := svc.List(content.ListInput{})
	if !r.OK {
		t.Fatalf("List: %s", r.Error())
	}
	out := r.Value.(content.ListOutput)
	if out.TotalInFlight != 1 {
		t.Fatalf("expected TotalInFlight=1 (live item excluded), got %d", out.TotalInFlight)
	}
	if out.DueToday != 1 {
		t.Fatalf("expected DueToday=1, got %d", out.DueToday)
	}
}

// TestAdvance_NotFound_Bad — Advance on a syntactically valid but
// nonexistent ID forwards loadOne's not-found error.
func TestAdvance_NotFound_Bad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newTestSvc(t)
	r := svc.Advance("does-not-exist-anywhere")
	if r.OK {
		t.Fatal("Advance on a nonexistent id must fail")
	}
	if !core.Contains(r.Error(), "not found") {
		t.Fatalf("expected not-found error, got %s", r.Error())
	}
}

// TestGet_SessionLockedViaNarrowGateAfterAtRestWrite_Bad — an .lthn
// record exists on disk; the gate is then swapped to a NARROW
// stubSessionGate (satisfies SessionGate but not accountKeyProvider).
// loadOne's atrestWriterFor check fails closed with the typed
// "content.session.locked" code, which Get forwards verbatim (distinct
// from the substrate's own no_unlocked_account/multi_account_ambiguous
// codes exercised in atrest_test.go).
func TestGet_SessionLockedViaNarrowGateAfterAtRestWrite_Bad(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newAtRestTestSvc(t)

	cr := svc.Create(content.CreateInput{T: "Narrow-gate-after-write probe", Col: "idea"})
	if !cr.OK {
		t.Fatalf("seed Create: %s", cr.Error())
	}
	id := cr.Value.(content.ContentItem).ID

	// Swap to the narrow gate — accountKeyProvider no longer satisfied.
	svc.SetSessionGate(&stubSessionGate{ids: []string{"acct-1"}})

	g := svc.Get(id)
	if g.OK {
		t.Fatal("Get on an .lthn record with a narrow gate must fail")
	}
	if !core.Contains(g.Error(), "content.session.locked") {
		t.Fatalf("expected content.session.locked, got %s", g.Error())
	}
}

// TestCreate_ContentDirFails_Bad — Create forwards contentDir's failure
// when $HOME is unavailable (distinct from the session-locked guard,
// which runs first and must already be satisfied).
func TestCreate_ContentDirFails_Bad(t *testing.T) {
	t.Setenv("HOME", "")
	svc := content.NewService(nil)
	svc.SetSessionGate(&stubSessionGate{ids: []string{"acct-1"}})
	r := svc.Create(content.CreateInput{T: "Homeless item"})
	if r.OK {
		t.Fatal("Create must fail when contentDir() cannot resolve $HOME")
	}
}

// TestCreate_AllSymbolsTitle_DefaultsToItemSlug_Good — a title that
// slugifies to empty ("!!!") falls back to the literal "item" slug
// stem so the generated ID stays non-empty.
func TestCreate_AllSymbolsTitle_DefaultsToItemSlug_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newTestSvc(t)
	r := svc.Create(content.CreateInput{T: "!!!"})
	if !r.OK {
		t.Fatalf("Create: %s", r.Error())
	}
	item := r.Value.(content.ContentItem)
	if !core.Contains(item.ID, "item-") {
		t.Fatalf("expected ID to fall back to item-<ts> stem, got %q", item.ID)
	}
}
