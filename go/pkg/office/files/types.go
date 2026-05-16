// SPDX-Licence-Identifier: EUPL-1.2

// Package files is the lthn-side Office filesystem browser service.
// Surfaces saved locations (workspace dirs), recent files, and disk
// usage for <lthn-view-files> in the Office role view.
//
// v1 scope: read-only catalogue from canonical workspace roots.
// No inotify / FSEvents watcher; no user-configurable custom locations.
// v2 (separate Mantis ticket) adds FSEvents push + user locations.
//
// Wire shape matches LocationRow / RecentRow / DiskMeter consumed by
// <lthn-view-files> in the Office role view.
//
// Usage example (Wails):
//
//	r := filesSvc.ListLocations()
//	if r.OK { out := r.Value.(files.ListLocationsOutput) }
package files

// LocationRow is the JSON wire type for a saved location in the left
// rail. Matches the LocationRow interface in
// frontend/src/lit/views/office/files.ts exactly.
//
// Usage example:
//
//	loc := files.LocationRow{Name: "Code", Count: 124, Size: "4.2 GB"}
type LocationRow struct {
	// Name is the display name shown to the user ("Code", "Documents").
	Name string `json:"name"`

	// Count is the number of items under this location.
	Count int `json:"count"`

	// Size is the human-readable rolled-up size ("4.2 GB", "180 MB").
	Size string `json:"size"`

	// Brand is true for Lethean-managed locations (brand-coloured in UI).
	Brand bool `json:"brand,omitempty"`
}

// RecentRow is the JSON wire type for a recently-modified file.
// Matches the RecentRow interface in
// frontend/src/lit/views/office/files.ts exactly.
//
// Usage example:
//
//	row := files.RecentRow{
//	    Name: "sow-v2.md", Path: "~/Documents/sales/",
//	    When: "14:42", Size: "38 KB",
//	}
type RecentRow struct {
	// Name is the file basename.
	Name string `json:"name"`

	// Path is the parent directory with $HOME collapsed to "~".
	Path string `json:"path"`

	// When is the human-readable last-edit time:
	// "HH:MM" (today), "yest" (yesterday), "X d" (older).
	When string `json:"when"`

	// Size is the human-readable file size.
	Size string `json:"size"`
}

// DiskMeter is the JSON wire type for the disk-free summary.
// Matches the DiskMeter interface in
// frontend/src/lit/views/office/files.ts exactly.
//
// Usage example:
//
//	meter := files.DiskMeter{Free: "312 GB", Total: "1 TB", Used: 68}
type DiskMeter struct {
	// Free is the human-readable free-space figure ("312 GB").
	Free string `json:"free"`

	// Total is the human-readable total-disk figure ("1 TB").
	Total string `json:"total"`

	// Used is the fill percentage 0–100 for the UI bar.
	Used int `json:"used"`
}

// ListLocationsOutput is the ListLocations response envelope.
//
// Usage example:
//
//	out := r.Value.(files.ListLocationsOutput)
//	for _, loc := range out.Locations { _ = loc.Name }
type ListLocationsOutput struct {
	// Locations is the ordered list of canonical workspace locations.
	Locations []LocationRow `json:"locations"`
}

// ListRecentInput drives the ListRecent method.
//
// Usage example:
//
//	r := svc.ListRecent(files.ListRecentInput{LocationName: "Code", Limit: 10})
type ListRecentInput struct {
	// LocationName filters to one named location. Empty string = all.
	LocationName string `json:"locationName,omitempty"`

	// Limit caps the result count. Zero defaults to 20.
	Limit int `json:"limit,omitempty"`
}

// ListRecentOutput is the ListRecent response envelope.
//
// Usage example:
//
//	out := r.Value.(files.ListRecentOutput)
//	for _, f := range out.Recent { _ = f.Name }
type ListRecentOutput struct {
	// Recent is the sorted (newest-first) list of recently-modified files.
	Recent []RecentRow `json:"recent"`

	// Total is the unfiltered count before the limit is applied.
	Total int `json:"total"`
}

// DiskUsageOutput is the GetDiskUsage response envelope.
//
// Usage example:
//
//	out := r.Value.(files.DiskUsageOutput)
//	_ = out.Disk.Free
type DiskUsageOutput struct {
	// Disk carries the free / total / used-percent figures.
	Disk DiskMeter `json:"disk"`
}

// locationSpec is the internal description of a canonical workspace
// location — used by scanLocation and collectRecent.
type locationSpec struct {
	// Name is the display name shown in the UI.
	Name string
	// Path is the absolute path to scan.
	Path string
	// Brand is true for Lethean-managed locations.
	Brand bool
}
