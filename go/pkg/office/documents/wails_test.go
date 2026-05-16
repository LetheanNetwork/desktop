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
