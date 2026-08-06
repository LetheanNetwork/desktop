// SPDX-Licence-Identifier: EUPL-1.2

// Internal (white-box) tests for service.go's unexported helpers.
// Direct unit tests of assignToQuarter / buildRows / buildKpis avoid
// coupling to the real wall clock (buildRows takes `now` as a
// parameter; assignToQuarter takes curYear/curQtr directly) — far
// more deterministic than driving these branches indirectly through
// Quarterly() and a real deal file on disk.

package forecast

import (
	"testing"

	core "dappco.re/go"
)

// --- assignToQuarter ---

func TestAssignToQuarter_Table_Good(t *testing.T) {
	cases := []struct {
		name        string
		closeTarget string
		curQtr      int
		want        int
	}{
		{"empty close target lands in current quarter", "", 3, 0},
		{"no-space month name (whole string parsed)", "Aug", 3, 0},
		{"space-prefixed month name, future quarter", "14 Dec", 1, 3},
		{"unknown month abbreviation falls back to 0", "14 Xyz", 2, 0},
		{"past quarter clips to 0 (v1 same-year simplification)", "14 Feb", 4, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := assignToQuarter(DealSummary{CloseTarget: c.closeTarget}, 2026, c.curQtr)
			if got != c.want {
				t.Fatalf("assignToQuarter(CloseTarget=%q, curQtr=%d) = %d, want %d",
					c.closeTarget, c.curQtr, got, c.want)
			}
		})
	}
}

// --- formatGBPK ---

func TestFormatGBPK_Table_Good(t *testing.T) {
	if got := formatGBPK(500); got != "£5" {
		t.Fatalf("formatGBPK(500) = %q, want £5 (sub-1000 branch, pence/100)", got)
	}
	if got := formatGBPK(150000); got != "£150 K" {
		t.Fatalf("formatGBPK(150000) = %q, want £150 K (>=1000 branch, rounded /1000)", got)
	}
}

// --- buildRows / buildKpis: controlled `now`, no wall-clock coupling ---

// TestBuildRowsAndKpis_Good_WindowSkipAndProbabilityOverride exercises
// two branches in one deterministic pass:
//   - buildRows' "offset >= n" skip (a deal assigned beyond the
//     requested forecast window must not leak into any row)
//   - the "ProbabilityPct != 0" override in both buildRows and
//     buildKpis (a deal-level probability beats the stage default)
func TestBuildRowsAndKpis_Good_WindowSkipAndProbabilityOverride(t *testing.T) {
	now := core.Date(2026, core.June, 15, 0, 0, 0, 0, core.UTC) // curQtr = 2

	summaries := []DealSummary{
		// Dec -> Q4; offset = 4-2 = 2, beyond a 1-quarter window — must
		// be excluded from buildRows entirely (offset >= n skip).
		{AmountPence: 5000, Stage: "won", CloseTarget: "14 Dec"},
		// Empty CloseTarget -> offset 0, always in-window. Explicit
		// ProbabilityPct overrides stageWeight("propose")=0.60.
		{AmountPence: 20000, Stage: "propose", ProbabilityPct: 45},
	}

	rows := buildRows(summaries, 1, now)
	if len(rows) != 1 {
		t.Fatalf("expected 1 row for a 1-quarter window, got %d", len(rows))
	}
	if rows[0].Committed != 0 {
		t.Fatalf("won deal beyond the window leaked into Committed: got %d, want 0", rows[0].Committed)
	}
	// Best = committed(0) + int(20000 * 0.45) = 9000 pence -> (0+9000+500)/1000 = 9.
	if rows[0].Best != 9 {
		t.Fatalf("Best = %d, want 9 (ProbabilityPct override not applied)", rows[0].Best)
	}

	kpis := buildKpis(rows, summaries)
	if len(kpis) != 4 {
		t.Fatalf("expected 4 KPI cards, got %d", len(kpis))
	}
	// Probability-weighted sums ALL summaries (not window-filtered):
	// won deal uses stageWeight("won")=1.00 -> 5000; propose deal uses
	// the 45% override -> 9000. (5000+9000+500)/1000 = 14.
	want := "£14 K"
	if kpis[3].V != want {
		t.Fatalf("Probability-weighted KPI = %q, want %q (deal-level override not applied)", kpis[3].V, want)
	}
}

// --- parseFM ---

// TestParseFM_Good_NoOpeningFenceStillParses covers the "content
// doesn't start with ---\n" branch (match=false, break) — parseFM
// must still attempt to parse the raw bytes as frontmatter rather
// than erroring outright, since the mismatch only means "no fence to
// strip", not "unparseable".
func TestParseFM_Good_NoOpeningFenceStillParses(t *testing.T) {
	raw := []byte("amount_pence: 4200\nstage: engage\n")
	fm, err := parseFM(raw)
	if err != nil {
		t.Fatalf("parseFM: %v", err)
	}
	if fm.AmountPence != 4200 || fm.Stage != "engage" {
		t.Fatalf("parseFM mismatch: %+v", fm)
	}
}

// TestParseFM_Bad_InvalidYAMLSyntax covers the yaml.Unmarshal failure
// branch — an unterminated quoted scalar is a genuine syntax error,
// not just a type mismatch, so it errors regardless of dealFM's field
// shapes.
func TestParseFM_Bad_InvalidYAMLSyntax(t *testing.T) {
	raw := []byte("---\namount_pence: \"unterminated\n---\n")
	_, err := parseFM(raw)
	if err == nil {
		t.Fatal("parseFM accepted an unterminated quoted YAML scalar")
	}
}

// --- dealsDir ---

// TestDealsDir_Bad_NoHome covers paths.Root()'s home.OK-false branch:
// with $HOME unset, core.UserHomeDir() fails and dealsDir must
// propagate the failure rather than falling through.
func TestDealsDir_Bad_NoHome(t *testing.T) {
	t.Setenv("HOME", "")
	r := dealsDir()
	if r.OK {
		t.Fatal("dealsDir succeeded with an empty $HOME")
	}
}

// TestDealsDir_Bad_MkdirPermissionDenied covers dealsDir's own
// core.MkdirAll failure branch: paths.Root() succeeds (the root
// already exists, so MkdirAll on it is a permission-independent
// no-op) but creating "sales/deals" underneath a read-only root
// fails.
func TestDealsDir_Bad_MkdirPermissionDenied(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	root := core.PathJoin(tmp, "Lethean")
	if r := core.MkdirAll(root, 0o755); !r.OK {
		t.Fatalf("pre-create root: %s", r.Error())
	}
	if r := core.Chmod(root, 0o500); !r.OK {
		t.Fatalf("chmod root read-only: %s", r.Error())
	}
	t.Cleanup(func() {
		_ = core.Chmod(root, 0o755)
	})

	r := dealsDir()
	if r.OK {
		t.Fatal("dealsDir succeeded creating sales/deals under a read-only Lethean root")
	}
}

// --- loadAllDeals ---

// TestLoadAllDeals_Bad_UnreadableDir covers the ReadDir-failure branch
// (distinct from "directory doesn't exist" — this is "exists but
// can't be listed"), which loadAllDeals treats as "no deals" (nil,
// nil), not an error.
func TestLoadAllDeals_Bad_UnreadableDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	dirR := dealsDir()
	if !dirR.OK {
		t.Fatalf("dealsDir setup: %s", dirR.Error())
	}
	dir := dirR.Value.(string)
	if r := core.Chmod(dir, 0o300); !r.OK {
		t.Fatalf("chmod deals dir unreadable: %s", r.Error())
	}
	t.Cleanup(func() {
		_ = core.Chmod(dir, 0o700)
	})

	summaries, err := loadAllDeals()
	if err != nil {
		t.Fatalf("loadAllDeals must degrade silently on an unreadable dir, got error: %v", err)
	}
	if summaries != nil {
		t.Fatalf("expected nil summaries for an unreadable dir, got %v", summaries)
	}
}

// TestLoadAllDeals_Good_SkipsInvalidEntries plants five entries in the
// deals dir — a subdirectory, a wrong-extension file, a valid deal, a
// deal with malformed YAML frontmatter, and a dangling symlink named
// *.md — and asserts loadAllDeals silently skips the four invalid
// shapes, returning only the one valid summary. Covers: the IsDir
// skip, the .md-suffix filter, the ReadFile failure skip, and the
// parseFM failure skip.
func TestLoadAllDeals_Good_SkipsInvalidEntries(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	dirR := dealsDir()
	if !dirR.OK {
		t.Fatalf("dealsDir setup: %s", dirR.Error())
	}
	dir := dirR.Value.(string)

	if r := core.MkdirAll(core.PathJoin(dir, "nested"), 0o700); !r.OK {
		t.Fatalf("plant subdir: %s", r.Error())
	}
	if r := core.WriteFile(core.PathJoin(dir, "notes.txt"), []byte("ignore me"), 0o600); !r.OK {
		t.Fatalf("plant wrong-extension file: %s", r.Error())
	}
	valid := "---\namount_pence: 4200\nstage: engage\n---\n"
	if r := core.WriteFile(core.PathJoin(dir, "deal1.md"), []byte(valid), 0o600); !r.OK {
		t.Fatalf("plant valid deal: %s", r.Error())
	}
	broken := "---\namount_pence: \"unterminated\n---\n"
	if r := core.WriteFile(core.PathJoin(dir, "broken.md"), []byte(broken), 0o600); !r.OK {
		t.Fatalf("plant malformed deal: %s", r.Error())
	}
	if r := core.Symlink(core.PathJoin(dir, "ghost-target"), core.PathJoin(dir, "dangling.md")); !r.OK {
		t.Fatalf("plant dangling symlink: %s", r.Error())
	}

	summaries, err := loadAllDeals()
	if err != nil {
		t.Fatalf("loadAllDeals: %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("expected exactly 1 valid summary, got %d: %+v", len(summaries), summaries)
	}
	if summaries[0].AmountPence != 4200 || summaries[0].Stage != "engage" {
		t.Fatalf("unexpected summary: %+v", summaries[0])
	}
}
