// SPDX-Licence-Identifier: EUPL-1.2

//go:build windows

package files

// diskUsageRaw returns the free and total bytes on the volume containing
// path. Windows stub — returns (0, 0) until a Windows-specific
// implementation is added.
func diskUsageRaw(path string) (free uint64, total uint64) {
	return 0, 0
}
