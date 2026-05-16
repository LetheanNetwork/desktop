// SPDX-Licence-Identifier: EUPL-1.2

package analytics_test

import (
	"testing"

	"dappco.re/lthn/desktop/pkg/marketing/analytics"
)

// TestGet_Fixture30d_Good — unconfigured Plausible → 30d fixture data returned.
func TestGet_Fixture30d_Good(t *testing.T) {
	svc := analytics.NewService(nil)
	r := svc.Get(analytics.GetInput{Window: "30d"})
	if !r.OK {
		t.Fatalf("Get failed: %s", r.Error())
	}
	out := r.Value.(analytics.GetOutput)
	if out.Window != "30d" {
		t.Fatalf("expected window=30d, got %q", out.Window)
	}
	if out.Sessions == "—" {
		t.Fatalf("expected non-empty sessions for 30d fixture")
	}
	if len(out.Sources) == 0 {
		t.Fatalf("expected non-empty sources for 30d fixture")
	}
	if len(out.Pages) == 0 {
		t.Fatalf("expected non-empty pages for 30d fixture")
	}
}

// TestGet_Default_Good — empty Window defaults to "30d".
func TestGet_Default_Good(t *testing.T) {
	svc := analytics.NewService(nil)
	r := svc.Get(analytics.GetInput{})
	if !r.OK {
		t.Fatalf("Get failed: %s", r.Error())
	}
	out := r.Value.(analytics.GetOutput)
	if out.Window != "30d" {
		t.Fatalf("expected default window=30d, got %q", out.Window)
	}
}

// TestGet_Unknown_Bad — unknown window string → core.Fail.
func TestGet_Unknown_Bad(t *testing.T) {
	svc := analytics.NewService(nil)
	r := svc.Get(analytics.GetInput{Window: "365d"})
	if r.OK {
		t.Fatalf("expected failure for unknown window, got OK")
	}
}

// TestGet_Fixture7d_Good — 7d with no Plausible → sessions="—", empty sources.
func TestGet_Fixture7d_Good(t *testing.T) {
	svc := analytics.NewService(nil)
	r := svc.Get(analytics.GetInput{Window: "7d"})
	if !r.OK {
		t.Fatalf("Get failed: %s", r.Error())
	}
	out := r.Value.(analytics.GetOutput)
	if out.Sessions != "—" {
		t.Fatalf("expected sessions=— for 7d fixture, got %q", out.Sessions)
	}
	if len(out.Sources) != 0 {
		t.Fatalf("expected 0 sources for 7d fixture, got %d", len(out.Sources))
	}
}

// TestServiceName_Good — ServiceName returns "Analytics".
func TestServiceName_Good(t *testing.T) {
	svc := analytics.NewService(nil)
	if svc.ServiceName() != "Analytics" {
		t.Fatalf("expected Analytics, got %q", svc.ServiceName())
	}
}
