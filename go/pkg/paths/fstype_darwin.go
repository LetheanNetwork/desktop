// SPDX-Licence-Identifier: EUPL-1.2

//go:build darwin

// fstype_darwin.go — Darwin statfs(2) reader. Returns the
// f_fstypename field as a lowercased Go string. The Darwin enum is
// strings, not numeric IDs (unlike Linux), so no mapping table.

package paths

import (
	core "dappco.re/go"
	"golang.org/x/sys/unix"
)

// statfsType reads the filesystem-type identifier for the directory
// at path via Darwin's statfs(2). On error, returns the "unknown"
// sentinel + an OK Result — degraded-warn rather than fail-loud.
//
// CoreGO does not wrap statfs(2); the dependency on
// golang.org/x/sys/unix is logged as a CoreGO export gap (see
// project_corego_export_gaps under the paths.RootFSType consumer).
func statfsType(path string) core.Result {
	if rootFSTypeOverride != "" {
		return core.Ok(rootFSTypeOverride)
	}
	var st unix.Statfs_t
	if err := unix.Statfs(path, &st); err != nil {
		core.Warn("paths.fstype: statfs failed", "path", path, "err", err)
		return core.Ok("unknown")
	}
	// Fstypename is a [16]byte NUL-terminated array.
	end := 0
	for end < len(st.Fstypename) && st.Fstypename[end] != 0 {
		end++
	}
	return core.Ok(core.Lower(string(st.Fstypename[:end])))
}
