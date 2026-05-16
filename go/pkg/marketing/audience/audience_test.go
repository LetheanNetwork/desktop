// SPDX-Licence-Identifier: EUPL-1.2

package audience_test

import (
	"testing"

	"dappco.re/lthn/desktop/pkg/marketing/audience"
)

// TestList_Empty_Good — empty dir → empty segments, TotalN=0.
func TestList_Empty_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := audience.NewService(nil)
	r := svc.List(audience.ListInput{})
	if !r.OK {
		t.Fatalf("List failed: %s", r.Error())
	}
	out := r.Value.(audience.ListOutput)
	if len(out.Segments) != 0 {
		t.Fatalf("expected 0 segments, got %d", len(out.Segments))
	}
	if out.TotalN != 0 {
		t.Fatalf("expected TotalN=0, got %d", out.TotalN)
	}
}

// TestCreate_Defaults_Good — Create with name+src → Growth="+0 / w".
func TestCreate_Defaults_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := audience.NewService(nil)
	r := svc.Create(audience.CreateInput{Name: "Local-AI developers", Src: "signup-tagged"})
	if !r.OK {
		t.Fatalf("Create failed: %s", r.Error())
	}
	seg := r.Value.(audience.Segment)
	if seg.Growth != "+0 / w" {
		t.Fatalf("expected Growth=+0 / w, got %q", seg.Growth)
	}
	if seg.ID == "" {
		t.Fatalf("expected non-empty ID")
	}
}

// TestCreate_NoName_Bad — empty name returns core.Fail.
func TestCreate_NoName_Bad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := audience.NewService(nil)
	r := svc.Create(audience.CreateInput{Name: "", Src: "signup-tagged"})
	if r.OK {
		t.Fatalf("expected failure for empty name, got OK")
	}
}

// TestCreate_NoSrc_Bad — empty src returns core.Fail.
func TestCreate_NoSrc_Bad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := audience.NewService(nil)
	r := svc.Create(audience.CreateInput{Name: "Developers", Src: ""})
	if r.OK {
		t.Fatalf("expected failure for empty src, got OK")
	}
}

// TestList_AllSegmentFirst_Good — "all" src segment sorts first in output.
func TestList_AllSegmentFirst_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := audience.NewService(nil)
	svc.Create(audience.CreateInput{Name: "Developers", Src: "signup-tagged", N: 100})
	// Create an "all" aggregate.
	svc.Create(audience.CreateInput{Name: "All subscribers", Src: "all", N: 500})

	r := svc.List(audience.ListInput{})
	if !r.OK {
		t.Fatalf("List failed: %s", r.Error())
	}
	out := r.Value.(audience.ListOutput)
	if len(out.Segments) < 2 {
		t.Fatalf("expected at least 2 segments, got %d", len(out.Segments))
	}
	if out.Segments[0].Src != "all" {
		t.Fatalf("expected first segment src=all, got %q", out.Segments[0].Src)
	}
	if out.TotalN != 500 {
		t.Fatalf("expected TotalN=500 from all segment, got %d", out.TotalN)
	}
}

// TestUpdate_N_Good — Update N field persists new count.
func TestUpdate_N_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := audience.NewService(nil)
	cr := svc.Create(audience.CreateInput{Name: "Investors", Src: "manual", N: 100})
	if !cr.OK {
		t.Fatalf("Create failed: %s", cr.Error())
	}
	id := cr.Value.(audience.Segment).ID

	ur := svc.Update(audience.UpdateInput{ID: id, N: 142})
	if !ur.OK {
		t.Fatalf("Update failed: %s", ur.Error())
	}
	updated := ur.Value.(audience.Segment)
	if updated.N != 142 {
		t.Fatalf("expected N=142, got %d", updated.N)
	}
}

// TestServiceName_Good — ServiceName returns "Audience".
func TestServiceName_Good(t *testing.T) {
	svc := audience.NewService(nil)
	if svc.ServiceName() != "Audience" {
		t.Fatalf("expected Audience, got %q", svc.ServiceName())
	}
}
