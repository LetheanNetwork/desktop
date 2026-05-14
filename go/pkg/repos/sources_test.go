// SPDX-Licence-Identifier: EUPL-1.2

package repos

import (
	"context"
	"testing"
)

// TestSources_Register_Good — a registered SourceProvider's paths
// surface in collectSourcePaths, deduped against any duplicates
// the same provider returns + duplicates across providers.
func TestSources_Register_Good(t *testing.T) {
	s := NewService(nil)
	s.RegisterSource("opencode-imports", func(_ context.Context) []string {
		return []string{"/a/b", "/c/d", "/a/b"} // intra-provider dup
	})
	s.RegisterSource("codex-imports", func(_ context.Context) []string {
		return []string{"/c/d", "/e/f"} // cross-provider dup with /c/d
	})
	got := s.collectSourcePaths(context.Background())
	want := []string{"/a/b", "/c/d", "/e/f"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i, p := range want {
		if got[i] != p {
			t.Errorf("position %d: got %q want %q", i, got[i], p)
		}
	}
}

// TestSources_Register_Bad — empty paths from a provider are
// silently dropped (no panic, no inflated dedup map).
func TestSources_Register_Bad(t *testing.T) {
	s := NewService(nil)
	s.RegisterSource("empty-source", func(_ context.Context) []string {
		return []string{"", "", "/x"}
	})
	got := s.collectSourcePaths(context.Background())
	if len(got) != 1 || got[0] != "/x" {
		t.Fatalf("expected [/x], got %v", got)
	}
}

// TestSources_Register_Ugly — RegisterSource on a nil service or
// with a nil callback is a safe no-op (not a panic).
func TestSources_Register_Ugly(t *testing.T) {
	var s *Service
	s.RegisterSource("noop", func(_ context.Context) []string { return []string{"/p"} })
	// also: non-nil service, nil callback
	s2 := NewService(nil)
	s2.RegisterSource("nil-fn", nil)
	if got := s2.collectSourcePaths(context.Background()); len(got) != 0 {
		t.Fatalf("expected empty, got %v", got)
	}
}
