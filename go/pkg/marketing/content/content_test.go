// SPDX-Licence-Identifier: EUPL-1.2

package content_test

import (
	"testing"

	"dappco.re/lthn/desktop/pkg/marketing/content"
)

// TestList_Empty_Good — empty dir → 5 columns, all empty, zero counts.
func TestList_Empty_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := content.NewService(nil)
	r := svc.List(content.ListInput{})
	if !r.OK {
		t.Fatalf("List failed: %s", r.Error())
	}
	out := r.Value.(content.ListOutput)
	if len(out.Columns) != 5 {
		t.Fatalf("expected 5 columns, got %d", len(out.Columns))
	}
	for _, col := range out.Columns {
		if len(col.Items) != 0 {
			t.Fatalf("column %q expected 0 items, got %d", col.ID, len(col.Items))
		}
	}
	if out.TotalInFlight != 0 || out.DueToday != 0 {
		t.Fatalf("expected zero counts")
	}
}

// TestCreate_Defaults_Good — Create with title only defaults to col=idea.
func TestCreate_Defaults_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := content.NewService(nil)
	r := svc.Create(content.CreateInput{T: "Watt-per-token comparison post"})
	if !r.OK {
		t.Fatalf("Create failed: %s", r.Error())
	}
	item := r.Value.(content.ContentItem)
	if item.Col != "idea" {
		t.Fatalf("expected col=idea, got %q", item.Col)
	}
	if item.ID == "" {
		t.Fatalf("expected non-empty ID")
	}
}

// TestCreate_Title_Bad — empty title returns core.Fail.
func TestCreate_Title_Bad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := content.NewService(nil)
	r := svc.Create(content.CreateInput{T: ""})
	if r.OK {
		t.Fatalf("expected failure for empty title, got OK")
	}
}

// TestAdvance_Order_Good — Advance cycles idea → draft → review → ready → live.
func TestAdvance_Order_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := content.NewService(nil)
	cr := svc.Create(content.CreateInput{T: "Test post", Col: "idea"})
	if !cr.OK {
		t.Fatalf("Create failed: %s", cr.Error())
	}
	id := cr.Value.(content.ContentItem).ID

	expected := []string{"draft", "review", "ready", "live"}
	for _, want := range expected {
		r := svc.Advance(id)
		if !r.OK {
			t.Fatalf("Advance to %q failed: %s", want, r.Error())
		}
		item := r.Value.(content.ContentItem)
		if item.Col != want {
			t.Fatalf("expected col=%q, got %q", want, item.Col)
		}
	}
}

// TestAdvance_AtLive_Bad — Advance at "live" returns core.Fail.
func TestAdvance_AtLive_Bad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := content.NewService(nil)
	cr := svc.Create(content.CreateInput{T: "Live post", Col: "live"})
	if !cr.OK {
		t.Fatalf("Create failed: %s", cr.Error())
	}
	id := cr.Value.(content.ContentItem).ID

	r := svc.Advance(id)
	if r.OK {
		t.Fatalf("expected failure advancing from live, got OK")
	}
}

// TestServiceName_Good — ServiceName returns "Content".
func TestServiceName_Good(t *testing.T) {
	svc := content.NewService(nil)
	if svc.ServiceName() != "Content" {
		t.Fatalf("expected Content, got %q", svc.ServiceName())
	}
}
