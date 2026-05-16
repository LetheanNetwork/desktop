// SPDX-Licence-Identifier: EUPL-1.2

package files

import (
	"testing"
)

// TestServiceName — ServiceName returns the correct Wails binding namespace.
func TestServiceName(t *testing.T) {
	svc := &Service{}
	if svc.ServiceName() != "Files" {
		t.Fatalf("expected ServiceName='Files', got %q", svc.ServiceName())
	}
}

// TestLocationRow_Fields — LocationRow carries all fields the frontend expects.
func TestLocationRow_Fields(t *testing.T) {
	row := LocationRow{Name: "Code", Count: 124, Size: "4.2 GB"}
	if row.Name == "" || row.Size == "" {
		t.Fatal("LocationRow missing required fields")
	}
}

// TestRecentRow_Fields — RecentRow carries all fields the frontend expects.
func TestRecentRow_Fields(t *testing.T) {
	row := RecentRow{Name: "sow-v2.md", Path: "~/Documents/", When: "14:42", Size: "38 KB"}
	if row.Name == "" || row.Path == "" || row.When == "" || row.Size == "" {
		t.Fatal("RecentRow missing required fields")
	}
}

// TestDiskMeter_Fields — DiskMeter carries Free, Total, Used.
func TestDiskMeter_Fields(t *testing.T) {
	meter := DiskMeter{Free: "312 GB", Total: "1 TB", Used: 68}
	if meter.Free == "" || meter.Total == "" {
		t.Fatal("DiskMeter missing required fields")
	}
	if meter.Used < 0 || meter.Used > 100 {
		t.Fatalf("DiskMeter.Used out of range: %d", meter.Used)
	}
}

// TestListLocationsOutput_Fields — ListLocationsOutput carries locations slice.
func TestListLocationsOutput_Fields(t *testing.T) {
	out := ListLocationsOutput{
		Locations: []LocationRow{{Name: "Code", Count: 10, Size: "1 GB"}},
	}
	if len(out.Locations) == 0 {
		t.Fatal("ListLocationsOutput.Locations must not be empty")
	}
}

// TestListRecentInput_Defaults — ListRecentInput zero value is valid.
func TestListRecentInput_Defaults(t *testing.T) {
	var in ListRecentInput
	if in.Limit != 0 {
		t.Fatal("ListRecentInput.Limit default should be 0 (use service default 20)")
	}
}

// TestListRecentOutput_Fields — ListRecentOutput carries recent slice + total.
func TestListRecentOutput_Fields(t *testing.T) {
	out := ListRecentOutput{
		Recent: []RecentRow{{Name: "x.md", Path: "~/", When: "now", Size: "1 KB"}},
		Total:  1,
	}
	if len(out.Recent) == 0 {
		t.Fatal("ListRecentOutput.Recent must not be empty")
	}
}

// TestDiskUsageOutput_Fields — DiskUsageOutput wraps DiskMeter.
func TestDiskUsageOutput_Fields(t *testing.T) {
	out := DiskUsageOutput{Disk: DiskMeter{Free: "100 GB", Total: "500 GB", Used: 80}}
	if out.Disk.Free == "" {
		t.Fatal("DiskUsageOutput.Disk.Free must not be empty")
	}
}
