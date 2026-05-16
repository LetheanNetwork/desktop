// SPDX-Licence-Identifier: EUPL-1.2

package files

import (
	"testing"
	"time"

	core "dappco.re/go"
)

// TestLocationCatalogue_Length — default catalogue has 5 entries.
func TestLocationCatalogue_Length(t *testing.T) {
	specs := locationCatalogue("/Users/test")
	if len(specs) != 5 {
		t.Fatalf("expected 5 location specs, got %d", len(specs))
	}
}

// TestLocationCatalogue_ModelsIsBrand — the models entry has Brand=true.
func TestLocationCatalogue_ModelsIsBrand(t *testing.T) {
	specs := locationCatalogue("/Users/test")
	found := false
	for _, spec := range specs {
		if spec.Brand {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected at least one Brand=true location in the catalogue")
	}
}

// TestFormatSizeGB_TB — 1 TB in bytes → "1 TB".
func TestFormatSizeGB_TB(t *testing.T) {
	got := formatSizeGB(1_000_000_000_000)
	if got != "1 TB" {
		t.Fatalf("expected '1 TB', got %q", got)
	}
}

// TestFormatSizeGB_GB — 4.2 GB in bytes → "4.2 GB".
func TestFormatSizeGB_GB(t *testing.T) {
	got := formatSizeGB(4_200_000_000)
	if got != "4.2 GB" {
		t.Fatalf("expected '4.2 GB', got %q", got)
	}
}

// TestFormatSizeGB_GB_Whole — whole GB → no decimal.
func TestFormatSizeGB_GB_Whole(t *testing.T) {
	got := formatSizeGB(2_000_000_000)
	if got != "2 GB" {
		t.Fatalf("expected '2 GB', got %q", got)
	}
}

// TestFormatSizeGB_MB — 180 MB in bytes → "180 MB".
func TestFormatSizeGB_MB(t *testing.T) {
	got := formatSizeGB(180_000_000)
	if got != "180 MB" {
		t.Fatalf("expected '180 MB', got %q", got)
	}
}

// TestFormatSize_KB — file-level KB formatting.
func TestFormatSize_KB(t *testing.T) {
	got := formatSize(38_000)
	if len(got) < 3 || got[len(got)-2:] != "KB" {
		t.Errorf("formatSize(38000) = %q, want KB suffix", got)
	}
}

// TestFormatSize_GB — large file uses GB suffix.
func TestFormatSize_GB(t *testing.T) {
	got := formatSize(2_100_000_000)
	if len(got) < 3 || got[len(got)-2:] != "GB" {
		t.Errorf("formatSize(2.1GB) = %q, want GB suffix", got)
	}
}

// TestRelativeWhen_Today — file modified 2h ago → HH:MM format.
func TestRelativeWhen_Today(t *testing.T) {
	now := core.Now()
	ts := now.Add(-2 * time.Hour)
	got := relativeWhen(ts, now)
	if len(got) != 5 || got[2] != ':' {
		t.Fatalf("expected HH:MM format, got %q", got)
	}
}

// TestRelativeWhen_Yesterday — file modified 24h ago → "yest".
func TestRelativeWhen_Yesterday(t *testing.T) {
	now := core.Now()
	ts := now.Add(-24 * time.Hour)
	got := relativeWhen(ts, now)
	if got != "yest" {
		t.Fatalf("expected 'yest', got %q", got)
	}
}

// TestRelativeWhen_Days — file modified 3 days ago → "3 d".
func TestRelativeWhen_Days(t *testing.T) {
	now := core.Now()
	ts := now.Add(-72 * time.Hour)
	got := relativeWhen(ts, now)
	if got != "3 d" {
		t.Fatalf("expected '3 d', got %q", got)
	}
}

// TestCollapseHome_Collapse — $HOME prefix is replaced with "~".
func TestCollapseHome_Collapse(t *testing.T) {
	home := "/Users/snider"
	got := collapseHome("/Users/snider/Documents/", home)
	if got != "~/Documents/" {
		t.Fatalf("expected '~/Documents/', got %q", got)
	}
}

// TestCollapseHome_NoMatch — non-home path is returned unchanged.
func TestCollapseHome_NoMatch(t *testing.T) {
	home := "/Users/snider"
	got := collapseHome("/tmp/foo", home)
	if got != "/tmp/foo" {
		t.Fatalf("expected '/tmp/foo', got %q", got)
	}
}

// TestScanLocation_MissingPath — non-existent path returns (0, 0, nil).
func TestScanLocation_MissingPath(t *testing.T) {
	spec := locationSpec{Name: "Ghost", Path: "/this/does/not/exist/ever"}
	count, sizeB, err := scanLocation(spec)
	if err != nil {
		t.Fatalf("expected nil error for missing path, got: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected count=0, got %d", count)
	}
	if sizeB != 0 {
		t.Fatalf("expected sizeB=0, got %d", sizeB)
	}
}
