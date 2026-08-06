// SPDX-Licence-Identifier: EUPL-1.2

// loadall_fault_test.go — fault injection for loadSegments' per-entry
// skip branches (RFC.stage-e-encrypt-at-rest v2 §4.1: "do NOT abort
// whole List on one bad file") plus the List/Get/Create/Update wails.go
// branches that only trigger with a populated segment set, an
// oversized name, or a locked-after-write gate swap. Mirrors
// pkg/sales/deals/loadall_fault_test.go's precedent.

package audience_test

import (
	"os"
	"testing"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/marketing/audience"
)

// TestAudience_LoadSegments_CorruptLthn_SkippedNotAborted_Ugly — a
// .lthn file too short/malformed to survive loadHeaderOnly is skipped;
// a healthy .md sibling still surfaces via List.
func TestAudience_LoadSegments_CorruptLthn_SkippedNotAborted_Ugly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newAtRestTestSvc(t)

	dir := core.PathJoin(home, "Lethean", "marketing", "audience")
	if mk := core.MkdirAll(dir, 0o700); !mk.OK {
		t.Fatalf("MkdirAll: %s", mk.Error())
	}
	if w := core.WriteFile(core.PathJoin(dir, "corrupt-entry.lthn"), []byte("not-a-real-envelope"), 0o600); !w.OK {
		t.Fatalf("seed corrupt .lthn: %s", w.Error())
	}

	cr := svc.Create(audience.CreateInput{Name: "Healthy encrypted segment", Src: "signup"})
	if !cr.OK {
		t.Fatalf("seed Create: %s", cr.Error())
	}

	r := svc.List(audience.ListInput{})
	if !r.OK {
		t.Fatalf("List must tolerate one corrupt .lthn entry: %s", r.Error())
	}
	out := r.Value.(audience.ListOutput)
	if len(out.Segments) != 1 {
		t.Fatalf("expected exactly the healthy record (corrupt one skipped), got %d", len(out.Segments))
	}
}

// TestAudience_LoadSegments_UnreadableMd_SkippedNotAborted_Bad — a .md
// file with permissions denying read hits loadSegments' `if !raw.OK {
// continue }` branch.
func TestAudience_LoadSegments_UnreadableMd_SkippedNotAborted_Bad(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root — permission-denial fault injection does not apply")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := audience.NewService(nil)
	svc.SetSessionGate(&stubSessionGate{ids: []string{"acct-legacy"}})

	dir := core.PathJoin(home, "Lethean", "marketing", "audience")
	if mk := core.MkdirAll(dir, 0o700); !mk.OK {
		t.Fatalf("MkdirAll: %s", mk.Error())
	}
	unreadable := core.PathJoin(dir, "unreadable-segment.md")
	legacy := []byte("---\nid: unreadable-segment\nname: Locked\nn: 0\ngrowth: \"\"\nsrc: signup\nspark: \"\"\n---\n")
	if w := core.WriteFile(unreadable, legacy, 0o600); !w.OK {
		t.Fatalf("seed .md: %s", w.Error())
	}
	if err := os.Chmod(unreadable, 0o000); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(unreadable, 0o600) })

	cr := svc.Create(audience.CreateInput{Name: "Healthy legacy segment", Src: "signup"})
	if !cr.OK {
		t.Fatalf("seed Create: %s", cr.Error())
	}

	r := svc.List(audience.ListInput{})
	if !r.OK {
		t.Fatalf("List must tolerate an unreadable .md entry: %s", r.Error())
	}
	out := r.Value.(audience.ListOutput)
	if len(out.Segments) != 1 {
		t.Fatalf("expected exactly the healthy record (unreadable one skipped), got %d", len(out.Segments))
	}
}

// TestAudience_LoadSegments_MalformedMdYaml_SkippedNotAborted_Bad — a
// .md file whose frontmatter fails yaml.Unmarshal hits loadSegments'
// parseSegment-error `continue` branch.
func TestAudience_LoadSegments_MalformedMdYaml_SkippedNotAborted_Bad(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := audience.NewService(nil)
	svc.SetSessionGate(&stubSessionGate{ids: []string{"acct-legacy"}})

	dir := core.PathJoin(home, "Lethean", "marketing", "audience")
	if mk := core.MkdirAll(dir, 0o700); !mk.OK {
		t.Fatalf("MkdirAll: %s", mk.Error())
	}
	bad := core.PathJoin(dir, "malformed-segment.md")
	if w := core.WriteFile(bad, []byte("---\n[not: valid: yaml\n---\n"), 0o600); !w.OK {
		t.Fatalf("seed malformed .md: %s", w.Error())
	}

	cr := svc.Create(audience.CreateInput{Name: "Healthy legacy segment two", Src: "signup"})
	if !cr.OK {
		t.Fatalf("seed Create: %s", cr.Error())
	}

	r := svc.List(audience.ListInput{})
	if !r.OK {
		t.Fatalf("List must tolerate a malformed .md entry: %s", r.Error())
	}
	out := r.Value.(audience.ListOutput)
	if len(out.Segments) != 1 {
		t.Fatalf("expected exactly the healthy record (malformed YAML skipped), got %d", len(out.Segments))
	}
}

// TestAudience_LoadSegments_SubdirEntry_Ignored_Good — a stray
// subdirectory inside the audience dir must be ignored by both
// loadSegments passes.
func TestAudience_LoadSegments_SubdirEntry_Ignored_Good(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newAtRestTestSvc(t)

	dir := core.PathJoin(home, "Lethean", "marketing", "audience")
	if mk := core.MkdirAll(core.PathJoin(dir, "stray-subdir"), 0o700); !mk.OK {
		t.Fatalf("MkdirAll: %s", mk.Error())
	}

	cr := svc.Create(audience.CreateInput{Name: "Segment beside a stray subdir", Src: "signup"})
	if !cr.OK {
		t.Fatalf("seed Create: %s", cr.Error())
	}

	r := svc.List(audience.ListInput{})
	if !r.OK {
		t.Fatalf("List must ignore a stray subdirectory: %s", r.Error())
	}
}

// TestUpdate_NotFound_Bad — Update on a syntactically valid but
// nonexistent ID forwards loadOne's not-found error.
func TestUpdate_NotFound_Bad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newTestSvc(t)
	r := svc.Update(audience.UpdateInput{ID: "does-not-exist-anywhere", N: 5})
	if r.OK {
		t.Fatal("Update on a nonexistent id must fail")
	}
	if !core.Contains(r.Error(), "not found") {
		t.Fatalf("expected not-found error, got %s", r.Error())
	}
}

// TestUpdate_PatchesGrowthSrcSpark_Good — Update's Growth/Src/Spark
// patch branches are symmetric to the N branch already covered
// elsewhere; drive all three together.
func TestUpdate_PatchesGrowthSrcSpark_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newTestSvc(t)
	cr := svc.Create(audience.CreateInput{Name: "Patchable segment", Src: "signup"})
	if !cr.OK {
		t.Fatalf("Create: %s", cr.Error())
	}
	id := cr.Value.(audience.Segment).ID

	r := svc.Update(audience.UpdateInput{ID: id, Growth: "+12 / w", Src: "referral", Spark: "▁▂▃"})
	if !r.OK {
		t.Fatalf("Update: %s", r.Error())
	}
	seg := r.Value.(audience.Segment)
	if seg.Growth != "+12 / w" || seg.Src != "referral" || seg.Spark != "▁▂▃" {
		t.Fatalf("unexpected patched segment: %+v", seg)
	}
}

// TestGet_SessionLockedViaNarrowGateAfterAtRestWrite_Bad — an .lthn
// record exists on disk; the gate is then swapped to a NARROW
// stubSessionGate (satisfies SessionGate but not accountKeyProvider).
// loadOne's atrestWriterFor check fails closed with the typed
// "audience.session.locked" code, which Get forwards verbatim.
func TestGet_SessionLockedViaNarrowGateAfterAtRestWrite_Bad(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newAtRestTestSvc(t)

	cr := svc.Create(audience.CreateInput{Name: "Narrow-gate probe", Src: "signup"})
	if !cr.OK {
		t.Fatalf("seed Create: %s", cr.Error())
	}
	id := cr.Value.(audience.Segment).ID

	svc.SetSessionGate(&stubSessionGate{ids: []string{"acct-1"}})

	g := svc.Get(id)
	if g.OK {
		t.Fatal("Get on an .lthn record with a narrow gate must fail")
	}
	if !core.Contains(g.Error(), "audience.session.locked") {
		t.Fatalf("expected audience.session.locked, got %s", g.Error())
	}
}

// TestCreate_AudienceDirFails_Bad — Create forwards audienceDir's
// failure when $HOME is unavailable.
func TestCreate_AudienceDirFails_Bad(t *testing.T) {
	t.Setenv("HOME", "")
	svc := audience.NewService(nil)
	svc.SetSessionGate(&stubSessionGate{ids: []string{"acct-1"}})
	r := svc.Create(audience.CreateInput{Name: "Homeless segment", Src: "signup"})
	if r.OK {
		t.Fatal("Create must fail when audienceDir() cannot resolve $HOME")
	}
}

// TestCreate_AllSymbolsName_DefaultsToSegmentSlug_Good — a name that
// slugifies to empty ("!!!") falls back to the literal "segment" slug.
func TestCreate_AllSymbolsName_DefaultsToSegmentSlug_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newTestSvc(t)
	r := svc.Create(audience.CreateInput{Name: "!!!", Src: "signup"})
	if !r.OK {
		t.Fatalf("Create: %s", r.Error())
	}
	seg := r.Value.(audience.Segment)
	if seg.ID != "segment" {
		t.Fatalf("expected ID to fall back to the literal segment slug, got %q", seg.ID)
	}
}

// TestCreate_OversizedName_RejectedByIsValidID_Bad — a name that
// slugifies to more than paths.MaxIDBytes (255) hits Create's own
// paths.IsValidID(id) guard (distinct from the writeSegment-internal
// guard which never sees an invalid id on this path).
func TestCreate_OversizedName_RejectedByIsValidID_Bad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := newTestSvc(t)
	longName := make([]byte, 300)
	for i := range longName {
		longName[i] = 'a'
	}
	r := svc.Create(audience.CreateInput{Name: string(longName), Src: "signup"})
	if r.OK {
		t.Fatal("Create with an oversized slugified name must reject")
	}
}
