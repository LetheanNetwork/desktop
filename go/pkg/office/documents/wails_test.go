// SPDX-Licence-Identifier: EUPL-1.2

package documents

import (
	"testing"
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
