// SPDX-Licence-Identifier: EUPL-1.2

//go:build linux

// fstype_linux.go — Linux statfs(2) reader. Returns the f_type
// numeric magic mapped to a canonical lowercase name. Only the
// fstypes the lock primitive needs to recognise are mapped; anything
// outside the table returns the "unknown" sentinel so the primitive
// degrades to fail-soft (warn-but-permit) on platforms / filesystems
// we haven't explicitly verified.

package paths

import (
	core "dappco.re/go"
	"golang.org/x/sys/unix"
)

// Filesystem magic numbers per linux/include/uapi/linux/magic.h.
// Only the values we route policy on are listed; anything else
// returns "unknown".
const (
	magicEXT4     = 0xef53
	magicXFS      = 0x58465342
	magicBTRFS    = 0x9123683e
	magicZFS      = 0x2fc12fc1
	magicTMPFS    = 0x01021994
	magicNFS      = 0x6969
	magicSMB      = 0x517b
	magicCIFS     = 0xff534d42
	magicFUSE     = 0x65735546
	magicOVERLAY  = 0x794c7630
)

// statfsType reads the filesystem-type identifier for the directory
// at path via Linux's statfs(2). On error, returns the "unknown"
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
	switch int64(st.Type) {
	case magicEXT4:
		return core.Ok("ext4")
	case magicXFS:
		return core.Ok("xfs")
	case magicBTRFS:
		return core.Ok("btrfs")
	case magicZFS:
		return core.Ok("zfs")
	case magicTMPFS:
		return core.Ok("tmpfs")
	case magicNFS:
		return core.Ok("nfs")
	case magicSMB:
		return core.Ok("smbfs")
	case magicCIFS:
		return core.Ok("cifs")
	case magicFUSE:
		return core.Ok("fuse")
	case magicOVERLAY:
		return core.Ok("overlay")
	}
	return core.Ok("unknown")
}
