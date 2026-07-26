// SPDX-Licence-Identifier: EUPL-1.2

package files_test

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/office/files"
)

// ExampleService_ListLocations shows how to list canonical workspace locations.
func ExampleService_ListLocations() {
	c := core.New()
	svc := files.NewService(files.Options{Core: c})

	r := svc.ListLocations()
	if !r.OK {
		return
	}
	out := r.Value.(files.ListLocationsOutput)
	for _, loc := range out.Locations {
		_ = loc.Name  // "Code"
		_ = loc.Count // 124
		_ = loc.Size  // "4.2 GB"
	}
}

// ExampleService_ListRecent shows how to get recent files for a specific location.
func ExampleService_ListRecent() {
	c := core.New()
	svc := files.NewService(files.Options{Core: c})

	r := svc.ListRecent(files.ListRecentInput{LocationName: "Code", Limit: 10})
	if !r.OK {
		return
	}
	out := r.Value.(files.ListRecentOutput)
	for _, f := range out.Recent {
		_ = f.Name // "v0.2-release-notes.md"
		_ = f.When // "14:18"
	}
}

// ExampleService_GetDiskUsage shows how to retrieve disk free/used figures.
func ExampleService_GetDiskUsage() {
	c := core.New()
	svc := files.NewService(files.Options{Core: c})

	r := svc.GetDiskUsage()
	if !r.OK {
		return
	}
	out := r.Value.(files.DiskUsageOutput)
	_ = out.Disk.Free  // "312 GB"
	_ = out.Disk.Total // "1 TB"
	_ = out.Disk.Used  // 68 (percent)
}
