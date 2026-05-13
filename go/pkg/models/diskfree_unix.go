// SPDX-Licence-Identifier: EUPL-1.2

//go:build darwin || linux

package models

import "syscall"

// diskFreeBytes returns the free bytes available at path. macOS +
// Linux implementation — uses Statfs. Returns 0 silently on error
// so the WebView falls back to the design literal ("312 GB free")
// instead of leaking a syscall error string into a footer slot.
func diskFreeBytes(path string) int64 {
	if path == "" {
		return 0
	}
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0
	}
	// Bavail is free blocks for non-root users; Bsize is the
	// fundamental block size. Cast to int64 to match the JSON
	// boundary's signed numeric type.
	return int64(st.Bavail) * int64(st.Bsize)
}
