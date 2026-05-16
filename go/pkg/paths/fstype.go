// SPDX-Licence-Identifier: EUPL-1.2

// fstype.go — root-filesystem detection per RFC.atomic-write.md §1.4
// (HIGH-2). flock(2) over NFS on macOS is silently broken and SMB /
// CIFS / FUSE filesystems offer no advisory-lock guarantees we can
// stand behind. v1's scope is "single host, single process"; rather
// than silently corrupt a user's data when they put ~/Lethean/ on a
// network mount, the lock primitive refuses to operate on those
// fstypes and the boot path surfaces the rejection at startup.
//
// Implementation note: CoreGO does not wrap statfs(2). The two
// per-platform readers live in fstype_darwin.go + fstype_linux.go
// behind build tags; this file owns the cross-platform default
// (unknown → OK with warn) and the policy table.

package paths

import core "dappco.re/go"

// RootFSType returns the filesystem-type identifier of the volume
// hosting paths.Root(). On supported platforms the value comes from
// statfs(2); on others it returns the sentinel "unknown" so the
// primitive degrades to fail-soft (warn-but-permit) rather than
// fail-loud on platforms where we haven't verified flock behaviour.
//
// The returned strings are lowercase, no leading dot, matching the
// canonical names reported by mount(8) / df -T / proc/mounts:
//
//	"apfs", "hfs", "ext4", "xfs", "btrfs", "zfs"   (local — OK)
//	"nfs", "nfs4", "smbfs", "cifs", "fuse.<name>"  (network — reject)
//	"unknown"                                       (degraded — warn)
//
// Usage example:
//
//	r := paths.RootFSType()
//	if r.OK { fstype := r.Value.(string); _ = fstype }
func RootFSType() core.Result {
	root := Root()
	if !root.OK {
		return root
	}
	return statfsType(root.Value.(string))
}

// IsNetworkFS reports whether the supplied fstype identifier matches
// the network-or-userspace family the lock primitive refuses to
// operate on. The match is exact for "nfs" / "nfs4" / "smbfs" /
// "cifs" and prefix for "fuse." (covers fuse.sshfs, fuse.gocryptfs,
// fuse.bindfs, etc — userspace drivers without enforceable advisory-
// lock semantics).
//
// Usage example:
//
//	if paths.IsNetworkFS(fstype) { return reject }
func IsNetworkFS(fstype string) bool {
	switch fstype {
	case "nfs", "nfs4", "smbfs", "cifs":
		return true
	}
	if core.HasPrefix(fstype, "fuse.") || fstype == "fuse" {
		return true
	}
	return false
}

// rootFSTypeOverride lets tests inject a synthetic fstype WITHOUT
// touching the real filesystem. When non-empty, RootFSType returns
// this value verbatim. Production code never sets this; only test
// helpers in fstype_test.go via the SetRootFSTypeForTest helper.
var rootFSTypeOverride string

// SetRootFSTypeForTest substitutes the fstype reported by RootFSType
// for the duration of the test. The empty string resets to the
// real statfs reader. Test discipline: ALWAYS pair with a
// t.Cleanup(func() { SetRootFSTypeForTest("") }) so a panic in the
// test body doesn't leak the override into adjacent tests.
//
// Usage example:
//
//	paths.SetRootFSTypeForTest("nfs")
//	t.Cleanup(func() { paths.SetRootFSTypeForTest("") })
func SetRootFSTypeForTest(fstype string) {
	rootFSTypeOverride = fstype
}
