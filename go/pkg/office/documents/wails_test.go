// SPDX-Licence-Identifier: EUPL-1.2

package documents

import (
	"testing"

	core "dappco.re/go"
)

// TestServiceName — ServiceName returns the correct Wails binding namespace.
func TestServiceName(t *testing.T) {
	svc := &Service{}
	if svc.ServiceName() != "Documents" {
		t.Fatalf("expected ServiceName='Documents', got %q", svc.ServiceName())
	}
}

// TestDocRow_Fields — DocRow carries all fields the frontend expects.
func TestDocRow_Fields(t *testing.T) {
	row := DocRow{
		Title:  "Test",
		State:  "draft",
		Author: "you",
		Edited: "now",
		Size:   "4.2 KB",
	}
	if row.Title == "" || row.State == "" || row.Author == "" || row.Edited == "" || row.Size == "" {
		t.Fatal("DocRow has empty required fields")
	}
}

// TestDocDetail_HasBody — DocDetail embeds DocRow and adds Body.
func TestDocDetail_HasBody(t *testing.T) {
	d := DocDetail{
		DocRow: DocRow{Title: "T", State: "draft", Author: "you", Edited: "now", Size: "1 KB"},
		Body:   "# Title\n\nContent.",
	}
	if d.Body == "" {
		t.Fatal("DocDetail.Body must not be empty")
	}
	if d.Title != "T" {
		t.Fatalf("DocDetail.DocRow.Title must be accessible, got %q", d.Title)
	}
}

// TestListInput_Defaults — ListInput zero value is valid (all docs, default limit).
func TestListInput_Defaults(t *testing.T) {
	var in ListInput
	if in.State != "" {
		t.Fatal("ListInput.State default should be empty (all)")
	}
	if in.Limit != 0 {
		t.Fatal("ListInput.Limit default should be 0 (use service default 50)")
	}
}

// TestListOutput_Fields — ListOutput carries docs slice and total.
func TestListOutput_Fields(t *testing.T) {
	out := ListOutput{
		Docs:  []DocRow{{Title: "X", State: "draft", Author: "you", Edited: "now", Size: "1 KB"}},
		Total: 1,
	}
	if len(out.Docs) == 0 {
		t.Fatal("ListOutput.Docs must not be empty")
	}
	if out.Total != 1 {
		t.Fatalf("ListOutput.Total expected 1, got %d", out.Total)
	}
}

// TestGetInput_Fields — GetInput carries the Slug field.
func TestGetInput_Fields(t *testing.T) {
	in := GetInput{Slug: "release-notes"}
	if in.Slug == "" {
		t.Fatal("GetInput.Slug must not be empty")
	}
}

// TestCreateInput_Fields — CreateInput carries required Slug + Body fields.
func TestCreateInput_Fields(t *testing.T) {
	in := CreateInput{Slug: "board-pack", Body: "# Board Pack\n", State: "draft"}
	if in.Slug == "" || in.Body == "" || in.State == "" {
		t.Fatal("CreateInput has empty required fields")
	}
}

// TestSaveInput_Fields — SaveInput carries Slug, Body, and composite token.
func TestSaveInput_Fields(t *testing.T) {
	in := SaveInput{
		Slug:        "board-pack",
		Body:        "# Updated\n",
		IfMatchHash: "abc123",
	}
	if in.Slug == "" || in.Body == "" || in.IfMatchHash == "" {
		t.Fatal("SaveInput has empty required fields")
	}
}

// TestDeleteInput_Fields — DeleteInput carries Slug and composite token.
func TestDeleteInput_Fields(t *testing.T) {
	in := DeleteInput{Slug: "old-draft", IfMatchHash: "abc123"}
	if in.Slug == "" || in.IfMatchHash == "" {
		t.Fatal("DeleteInput has empty required fields")
	}
}

// TestMaxBodyBytes — constant is exactly 1 MB.
func TestMaxBodyBytes(t *testing.T) {
	if MaxBodyBytes != 1<<20 {
		t.Fatalf("MaxBodyBytes expected %d, got %d", 1<<20, MaxBodyBytes)
	}
}

// TestDocChanged_Fields — DocChanged carries all event payload fields.
func TestDocChanged_Fields(t *testing.T) {
	e := DocChanged{Kind: "created", Slug: "notes", At: core.Now()}
	if e.Kind == "" || e.Slug == "" || e.At.IsZero() {
		t.Fatal("DocChanged has empty required fields")
	}
}

// TestEventConstants — event name constants follow the documents.* namespace.
func TestEventConstants(t *testing.T) {
	for _, name := range []string{EventCreated, EventUpdated, EventDeleted} {
		if !core.HasPrefix(name, "documents.") {
			t.Errorf("event constant %q does not start with 'documents.'", name)
		}
	}
}

// TestSubscribe_Good — a subscriber registered via Subscribe receives
// a DocChanged broadcast fired through c.ACTION. Mirrors
// pkg/tasks/events_test.go's TestEvents_Subscribe_Good shape for the
// equivalent documents.Subscribe helper.
func TestSubscribe_Good(t *testing.T) {
	c := core.New()

	var got []DocChanged
	var mu core.Mutex
	Subscribe(c, func(ev DocChanged) {
		mu.Lock()
		got = append(got, ev)
		mu.Unlock()
	})

	c.ACTION(DocChanged{Kind: "created", Slug: "release-notes", At: core.Now()})

	mu.Lock()
	defer mu.Unlock()
	if len(got) != 1 {
		t.Fatalf("subscriber received %d events, want 1", len(got))
	}
	if got[0].Kind != "created" || got[0].Slug != "release-notes" {
		t.Errorf("subscriber event = %+v, want Kind=created Slug=release-notes", got[0])
	}
}

// TestSubscribe_Ugly_IgnoresOtherMessageTypes covers the type-
// assertion miss path — a broadcast of an unrelated message type must
// not reach fn.
func TestSubscribe_Ugly_IgnoresOtherMessageTypes(t *testing.T) {
	c := core.New()

	called := false
	Subscribe(c, func(DocChanged) { called = true })

	type otherMessage struct{ X int }
	c.ACTION(otherMessage{X: 1})

	if called {
		t.Error("Subscribe's fn must not fire for a non-DocChanged message")
	}
}

// --- List / Get behavioural coverage ---
//
// Every test above this point in the file only checks field shapes;
// none actually calls svc.List or svc.Get. These drive the real
// scan/filter/limit and lookup/error paths.

// TestList_Good_MultipleDocsFilteredByState covers List's per-record
// scan loop (toRow + append), the State filter's continue branch, and
// the matching-record append branch in one corpus: two "draft" docs
// and one "ready" doc, filtered down to State="draft".
func TestList_Good_MultipleDocsFilteredByState(t *testing.T) {
	svc := newTestService(t)
	base := testSlug(t)
	slugA, slugB, slugC := base+"-a", base+"-b", base+"-c"
	for _, s := range []string{slugA, slugB, slugC} {
		cleanupDoc(t, s)
	}

	for _, in := range []CreateInput{
		{Slug: slugA, Body: "# A\n", State: "draft"},
		{Slug: slugB, Body: "# B\n", State: "draft"},
		{Slug: slugC, Body: "# C\n", State: "ready"},
	} {
		if r := svc.Create(in); !r.OK {
			t.Fatalf("Create(%s) failed: %v", in.Slug, r.Error())
		}
	}

	r := svc.List(ListInput{State: "draft"})
	if !r.OK {
		t.Fatalf("List failed: %v", r.Error())
	}
	out := r.Value.(ListOutput)
	if out.Total != 3 {
		t.Errorf("List(draft).Total = %d, want 3 (unfiltered library count)", out.Total)
	}
	if len(out.Docs) != 2 {
		t.Fatalf("List(draft).Docs = %d, want 2 (slugC's ready state must be filtered out)", len(out.Docs))
	}
	for _, d := range out.Docs {
		if d.State != "draft" {
			t.Errorf("List(draft) returned a non-draft doc: %+v", d)
		}
	}
}

// TestList_Good_LimitTruncates covers the Limit>0 truncation branch.
func TestList_Good_LimitTruncates(t *testing.T) {
	svc := newTestService(t)
	base := testSlug(t)
	slugs := []string{base + "-a", base + "-b", base + "-c"}
	for _, s := range slugs {
		cleanupDoc(t, s)
		if r := svc.Create(CreateInput{Slug: s, Body: "# Doc\n"}); !r.OK {
			t.Fatalf("Create(%s) failed: %v", s, r.Error())
		}
	}

	r := svc.List(ListInput{Limit: 1})
	if !r.OK {
		t.Fatalf("List failed: %v", r.Error())
	}
	out := r.Value.(ListOutput)
	if len(out.Docs) != 1 {
		t.Errorf("List(Limit: 1).Docs = %d, want 1 (truncated from 3)", len(out.Docs))
	}
	if out.Total != 3 {
		t.Errorf("List(Limit: 1).Total = %d, want 3 (unfiltered library count)", out.Total)
	}
}

// TestList_Bad_ScanFails covers List's scanDocs error propagation —
// an unwritable HOME breaks docsDir deep inside scanDocs. A bare
// *Service{} is fine here: List is read-only and never touches
// s.core or the session gate.
func TestList_Bad_ScanFails(t *testing.T) {
	unwritableHOME(t)
	svc := &Service{}
	r := svc.List(ListInput{})
	if r.OK {
		t.Fatal("List with an unwritable HOME should fail")
	}
}

// TestGet_Bad_InvalidSlug covers Get's paths.IsValidID guard.
func TestGet_Bad_InvalidSlug(t *testing.T) {
	svc := newTestService(t)
	r := svc.Get(GetInput{Slug: "../etc/passwd"})
	if r.OK {
		t.Fatal("Get with a traversal slug should fail")
	}
}

// TestGet_Bad_NotFound covers Get's loadDoc error branch for a
// validly-shaped but nonexistent slug.
func TestGet_Bad_NotFound(t *testing.T) {
	svc := newTestService(t)
	r := svc.Get(GetInput{Slug: "totally-nonexistent-document-slug"})
	if r.OK {
		t.Fatal("Get with a nonexistent slug should fail")
	}
}

// TestGet_Good_ReturnsBody is the sibling Good case establishing that
// the two Bad tests above are testing real failure, not a universally
// broken Get.
func TestGet_Good_ReturnsBody(t *testing.T) {
	svc := newTestService(t)
	slug := testSlug(t)
	cleanupDoc(t, slug)
	if r := svc.Create(CreateInput{Slug: slug, Body: "# Hello\n\nBody text.", State: "draft"}); !r.OK {
		t.Fatalf("Create failed: %v", r.Error())
	}

	r := svc.Get(GetInput{Slug: slug})
	if !r.OK {
		t.Fatalf("Get failed: %v", r.Error())
	}
	detail := r.Value.(DocDetail)
	if detail.State != "draft" {
		t.Errorf("Get.State = %q, want draft", detail.State)
	}
	if detail.Body == "" {
		t.Error("Get.Body must not be empty")
	}
}
