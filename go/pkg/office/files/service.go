// SPDX-Licence-Identifier: EUPL-1.2

// Service — Core integration for the Office filesystem browser.
// Surfaces saved locations, recent files, and disk usage. Read-only in
// v1 — no writes, no FSEvents watcher.
//
// Lifecycle:
//   - Register(c)   wires the service into the Core container
//   - ServiceName() returns "Files" for the Wails namespace
//
// All I/O via CoreGO wrappers: core.ReadDir / core.DirFS / core.Stat /
// core.UserHomeDir / core.DiskUsage. Banned: os, path/filepath,
// strings, encoding/json, fmt, log, errors.

package files

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/paths"
)

// SECURITY-NOTE — TIER-1 TRUSTED SURFACE (Mantis #1502, Cerberus pass-10):
//
// This Wails surface returns PII (mail threads / file paths / document bodies)
// to any in-process WebView caller. Today's caller is the trusted host SPA only.
// Post-#1421 (plugin enablement) MUST NOT inherit these methods — plugin tier
// requires explicit user consent + a stronger boundary than the in-process Wails
// IPC.
//
// Bridge bearer auth (#1423) gates only /mcp/* + /internal/* — it does NOT gate
// this Wails surface. Per design_sandbox_is_the_safety_floor + Snider's A1 floor:
// adding new endpoints here MUST be deliberate; new fields on existing endpoints
// MUST be reviewed for PII propagation.
//
// When v2 features (#1495/#1496/#1497) populate threads / docs / file index,
// re-audit this surface against the plugin-capability split.

// Service owns the Office filesystem browser surface.
//
// Usage example:
//
//	svc := files.NewService(c)
type Service struct {
	core *core.Core
}

// NewService constructs the files service against a Core container.
// Wired via core.WithName("office-files", files.Register) in app.go.
//
// Usage example:
//
//	svc := files.NewService(c)
func NewService(c *core.Core) *Service {
	return &Service{core: c}
}

// Register constructs the files service for Core registration.
//
// Usage example:
//
//	core.New(core.WithName("office-files", files.Register))
func Register(c *core.Core) core.Result {
	return core.Ok(NewService(c))
}

// ServiceName labels the binding namespace exposed to JS.
// Wails binds methods as "Files.ListLocations()" etc.
func (s *Service) ServiceName() string { return "Files" }

// locationCatalogue returns the ordered list of canonical workspace
// locations relative to homeDir. The lthn / models entry uses
// paths.ModelsDir() so the user's configured override is respected.
func locationCatalogue(homeDir string) []locationSpec {
	modelsDir := ""
	if r := paths.ModelsDir(); r.OK {
		modelsDir, _ = r.Value.(string)
	}
	if modelsDir == "" {
		modelsDir = core.PathJoin(homeDir, "Lethean", "conf", "models")
	}
	return []locationSpec{
		{Name: "Code", Path: core.PathJoin(homeDir, "Code")},
		{Name: "Documents", Path: core.PathJoin(homeDir, "Documents")},
		{Name: "lthn / models", Path: modelsDir, Brand: true},
		{Name: "Recordings", Path: core.PathJoin(homeDir, "Recordings")},
		{Name: "Screenshots", Path: core.PathJoin(homeDir, "Screenshots")},
	}
}

// scanLocation reads one level of entries from spec.Path and returns
// the item count and total size in bytes. Missing paths return (0, 0, nil).
func scanLocation(spec locationSpec) (count int, sizeB int64, err error) {
	statR := core.Stat(spec.Path)
	if !statR.OK {
		// Path doesn't exist — not an error for the catalogue.
		return 0, 0, nil
	}

	entriesR := core.ReadDir(core.DirFS(spec.Path), ".")
	if !entriesR.OK {
		return 0, 0, nil
	}
	entries, _ := entriesR.Value.([]core.FsDirEntry)
	for _, entry := range entries {
		count++
		if !entry.IsDir() {
			infoR, err2 := entry.Info()
			if err2 == nil {
				sizeB += infoR.Size()
			}
		}
	}
	return count, sizeB, nil
}

// collectRecent walks one level inside each spec, collects all files with
// mtime, sorts descending by mtime, and returns the first limit rows.
// Directories are skipped. Returns rows with Path having $HOME collapsed to "~".
func collectRecent(specs []locationSpec, homeDir string, limit int) ([]RecentRow, error) {
	type candidate struct {
		row     RecentRow
		modTime core.Time
	}
	var all []candidate

	for _, spec := range specs {
		entriesR := core.ReadDir(core.DirFS(spec.Path), ".")
		if !entriesR.OK {
			continue
		}
		entries, _ := entriesR.Value.([]core.FsDirEntry)
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				continue
			}
			parentPath := spec.Path + "/"
			all = append(all, candidate{
				row: RecentRow{
					Name: sanitizeFilename(entry.Name()),
					Path: sanitizeFilename(collapseHome(parentPath, homeDir)),
					When: "", // filled after sort
					Size: formatSize(info.Size()),
				},
				modTime: info.ModTime(),
			})
		}
	}

	// Sort descending by modTime.
	for i := 1; i < len(all); i++ {
		for j := i; j > 0 && all[j].modTime.After(all[j-1].modTime); j-- {
			all[j], all[j-1] = all[j-1], all[j]
		}
	}

	now := core.Now()
	rows := make([]RecentRow, 0, len(all))
	for _, c := range all {
		c.row.When = relativeWhen(c.modTime, now)
		rows = append(rows, c.row)
	}

	if len(rows) > limit {
		rows = rows[:limit]
	}
	return rows, nil
}

// collapseHome replaces the homeDir prefix with "~" in path.
func collapseHome(path string, homeDir string) string {
	prefix := homeDir + "/"
	if len(path) >= len(prefix) {
		match := true
		for i := 0; i < len(prefix); i++ {
			if path[i] != prefix[i] {
				match = false
				break
			}
		}
		if match {
			return "~/" + path[len(prefix):]
		}
	}
	return path
}

// relativeWhen formats a file mtime as:
// "HH:MM" (today), "yest" (yesterday), "X d" (older).
func relativeWhen(t core.Time, now core.Time) string {
	if t.IsZero() {
		return ""
	}
	diff := now.Sub(t)
	if diff < 0 {
		diff = -diff
	}
	const hour = core.Hour
	const day = 24 * core.Hour

	switch {
	case diff < 20*hour:
		h, m, _ := t.Clock()
		return core.Sprintf("%02d:%02d", h, m)
	case diff < 48*hour:
		return "yest"
	default:
		return core.Sprintf("%d d", int(diff/day))
	}
}

// formatSize formats file size in bytes to human-readable string.
func formatSize(b int64) string {
	const kb = int64(1024)
	const mb = int64(1024 * 1024)
	const gb = int64(1024 * 1024 * 1024)

	switch {
	case b >= gb:
		f := float64(b) / float64(gb)
		if b%gb < gb/10 {
			return core.Sprintf("%d GB", b/gb)
		}
		return core.Sprintf("%.1f GB", f)
	case b >= mb:
		f := float64(b) / float64(mb)
		if b%mb < mb/10 {
			return core.Sprintf("%d MB", b/mb)
		}
		return core.Sprintf("%.1f MB", f)
	case b >= kb:
		f := float64(b) / float64(kb)
		if b%kb < kb/10 {
			return core.Sprintf("%d KB", b/kb)
		}
		return core.Sprintf("%.1f KB", f)
	default:
		return core.Sprintf("%d B", b)
	}
}

// formatSizeGB formats large storage sizes (disk totals / free space).
// Like formatSize but uses 1000-based units for display (matching OS
// conventions for disk size). TB threshold added for multi-TB drives.
func formatSizeGB(b uint64) string {
	const kb = uint64(1000)
	const mb = uint64(1000 * 1000)
	const gb = uint64(1000 * 1000 * 1000)
	const tb = uint64(1000 * 1000 * 1000 * 1000)

	switch {
	case b >= tb:
		f := float64(b) / float64(tb)
		if b%tb < tb/10 {
			return core.Sprintf("%d TB", b/tb)
		}
		return core.Sprintf("%.1f TB", f)
	case b >= gb:
		f := float64(b) / float64(gb)
		if b%gb < gb/10 {
			return core.Sprintf("%d GB", b/gb)
		}
		return core.Sprintf("%.1f GB", f)
	case b >= mb:
		return core.Sprintf("%d MB", b/mb)
	case b >= kb:
		return core.Sprintf("%d KB", b/kb)
	default:
		return core.Sprintf("%d B", b)
	}
}

// diskMeter calls diskUsageRaw (platform-specific Statfs) on the given
// path and formats the result. Returns a zero DiskMeter on failure so
// the WebView can fall back to a design literal rather than an error.
func diskMeter(path string) (DiskMeter, error) {
	free, total := diskUsageRaw(path)

	usedPct := 0
	if total > 0 {
		used := total - free
		if used > total {
			used = total // guard underflow on some platforms
		}
		usedPct = int(100 * used / total)
		if usedPct > 100 {
			usedPct = 100
		}
	}

	freeStr := "0 GB"
	totalStr := "0 GB"
	if free > 0 {
		freeStr = formatSizeGB(free)
	}
	if total > 0 {
		totalStr = formatSizeGB(total)
	}

	return DiskMeter{
		Free:  freeStr,
		Total: totalStr,
		Used:  usedPct,
	}, nil
}

// sanitizeFilename replaces Unicode characters that enable Trojan Source
// rendering attacks (CVE-2021-42574) with the U+FFFD replacement char.
// The filename remains identifiable but cannot reorder its own glyphs
// when rendered in a bidi-aware terminal or WebView.
//
// Replaced ranges (override + isolate + C0 controls):
//   - U+202A..U+202E — bidi format controls (LRE/RLE/PDF/LRO/RLO)
//   - U+2066..U+2069 — bidi isolate controls (LRI/RLI/FSI/PDI)
//   - U+200E, U+200F — directional marks (softer; replaced for parity
//     with override family — legitimate uses in standalone filenames
//     are vanishingly rare)
//   - U+0000..U+001F — C0 control characters, except TAB(09)/LF(0A)/CR(0D)
//     which are still stripped because they corrupt single-line rendering
//
// Per Mantis #1504: a file named "legitimate‮kcatta.exe" renders as
// "legitimateexe.attack" in a bidi-aware UI, masking the real extension.
// Replacement (vs rejection) keeps the row visible so the user still sees
// "legitimate�kcatta.exe" — an obviously-tampered name they can act on.
//
// Usage example:
//
//	clean := sanitizeFilename(entry.Name()) // "report.pdf" → "report.pdf"
func sanitizeFilename(name string) string {
	const replacement = '�'
	changed := false
	out := make([]rune, 0, len(name))
	for _, r := range name {
		switch {
		case r >= 0x202A && r <= 0x202E,
			r >= 0x2066 && r <= 0x2069,
			r == 0x200E, r == 0x200F,
			r >= 0x0000 && r <= 0x001F:
			out = append(out, replacement)
			changed = true
		default:
			out = append(out, r)
		}
	}
	if !changed {
		return name
	}
	return string(out)
}
