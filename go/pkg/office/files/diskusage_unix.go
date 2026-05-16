// SPDX-Licence-Identifier: EUPL-1.2

//go:build darwin || linux

package files

import "syscall"

// diskUsageRaw returns the free and total bytes on the volume containing
// path. macOS + Linux implementation — uses Statfs. Returns (0, 0)
// silently on error so the WebView falls back to a zero DiskMeter
// rather than exposing a syscall error to the frontend.
func diskUsageRaw(path string) (free uint64, total uint64) {
	if path == "" {
		return 0, 0
	}
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, 0
	}
	// Bavail = free blocks for non-root; Blocks = total blocks.
	free = st.Bavail * uint64(st.Bsize)
	total = st.Blocks * uint64(st.Bsize)
	return free, total
}
