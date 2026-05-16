// SPDX-Licence-Identifier: EUPL-1.2

// Wails surface for the Office filesystem browser service. Methods are
// bound on the *Service receiver and exposed to the frontend via wails3
// generate bindings — the TS shape lands at
// frontend/bindings/dappco.re/lthn/desktop/pkg/office/files/.

package files

import (
	core "dappco.re/go"
)

// ListLocations returns the ordered catalogue of canonical workspace
// locations with item counts and rolled-up sizes.
// Non-existent paths produce a row with count=0 and size="0 KB".
//
// Usage example:
//
//	r := svc.ListLocations()
//	if r.OK { out := r.Value.(files.ListLocationsOutput) }
func (s *Service) ListLocations() core.Result {
	homeR := core.UserHomeDir()
	if !homeR.OK {
		return core.Fail(core.E("files.ListLocations", "home dir unresolved", nil))
	}
	home, _ := homeR.Value.(string)

	specs := locationCatalogue(home)
	rows := make([]LocationRow, 0, len(specs))

	for _, spec := range specs {
		count, sizeB, err := scanLocation(spec)
		if err != nil {
			core.Warn("files: scanLocation failed", "name", spec.Name, "err", err)
		}
		sizeStr := formatSize(sizeB)
		if sizeB == 0 {
			sizeStr = "0 KB"
		}
		rows = append(rows, LocationRow{
			Name:  spec.Name,
			Count: count,
			Size:  sizeStr,
			Brand: spec.Brand,
		})
	}

	return core.Ok(ListLocationsOutput{Locations: rows})
}

// ListRecent returns recently-modified files sorted newest-first.
// When input.LocationName is non-empty, only files under that location
// are included. Caps at input.Limit (default 20).
//
// Usage example:
//
//	r := svc.ListRecent(files.ListRecentInput{LocationName: "Code", Limit: 10})
//	if r.OK { out := r.Value.(files.ListRecentOutput) }
func (s *Service) ListRecent(input ListRecentInput) core.Result {
	homeR := core.UserHomeDir()
	if !homeR.OK {
		return core.Fail(core.E("files.ListRecent", "home dir unresolved", nil))
	}
	home, _ := homeR.Value.(string)

	limit := input.Limit
	if limit <= 0 {
		limit = 20
	}

	specs := locationCatalogue(home)
	if input.LocationName != "" {
		filtered := specs[:0]
		for _, spec := range specs {
			if spec.Name == input.LocationName {
				filtered = append(filtered, spec)
				break
			}
		}
		specs = filtered
	}

	rows, err := collectRecent(specs, home, limit)
	if err != nil {
		return core.Fail(core.E("files.ListRecent", "collect failed", err))
	}

	return core.Ok(ListRecentOutput{
		Recent: rows,
		Total:  len(rows),
	})
}

// GetDiskUsage returns the free / total / used-percent for the volume
// that contains the user's home directory.
//
// Usage example:
//
//	r := svc.GetDiskUsage()
//	if r.OK { out := r.Value.(files.DiskUsageOutput) }
func (s *Service) GetDiskUsage() core.Result {
	homeR := core.UserHomeDir()
	if !homeR.OK {
		return core.Fail(core.E("files.GetDiskUsage", "home dir unresolved", nil))
	}
	home, _ := homeR.Value.(string)

	meter, err := diskMeter(home)
	if err != nil {
		return core.Fail(core.E("files.GetDiskUsage", "disk usage failed", err))
	}

	return core.Ok(DiskUsageOutput{Disk: meter})
}
