// SPDX-Licence-Identifier: EUPL-1.2

//go:build darwin || linux

package downloader

import (
	"syscall"

	core "dappco.re/go"
)

// reasonQuarantineExists is the typed code emitted by audit when a
// quarantine create refuses because a file (regular or symlink) is
// already at the target path. Maps to Cerberus #49 F-3 + F-4 — a
// per-name lock and O_EXCL together close the parallel-create race
// AND the pre-planted-symlink write-anywhere primitive.
const reasonQuarantineExists = "download.quarantine_exists"

// reasonQuarantineSymlinkRefused is the typed code emitted by audit
// when a quarantine create refuses because the path was a symlink
// (ELOOP from O_NOFOLLOW on the final path component). Distinct from
// _exists so a forensic walker can tell "racing user" from "attacker
// planted a symlink targeting ~/.ssh/authorized_keys".
const reasonQuarantineSymlinkRefused = "download.quarantine_symlink_refused"

// createQuarantineFile opens path for writing with both O_EXCL (refuse
// if anything is there) and O_NOFOLLOW (refuse if it's a symlink). On
// POSIX systems opening with O_NOFOLLOW on the final path component
// returns ELOOP if that component is a symlink — even before the kernel
// would follow it. Combined with O_EXCL this closes Cerberus #49 F-4:
// an attacker with filesystem write access to the quarantine dir cannot
// pre-plant `quarantine/foo -> ~/.ssh/authorized_keys` and have the
// downloader write through it.
//
// File mode 0o600 — owner-only read/write. Quarantine files are never
// shared with other users; tightening from the previous 0o644 (Create
// default) eliminates a small information-disclosure exposure on
// shared-home multi-user machines.
//
// Returns one of:
//   - core.Ok(*core.OSFile) on success
//   - core.Fail(error) with cause = ErrExist when path already exists
//   - core.Fail(error) with reason text containing "symlink_refused"
//     when path is a symlink (ELOOP)
//   - core.Fail wrapping any other OpenFile error
//
// Caller is responsible for closing the file. Use errors.Is(err,
// os.ErrExist) at the call site to branch on the "concurrent /
// stale quarantine" case vs the symlink-refused case.
//
// Usage example:
//
//	r := createQuarantineFile(qDest)
//	if !r.OK { return r }
//	file := r.Value.(*core.OSFile)
//	defer file.Close()
func createQuarantineFile(path string) core.Result {
	flag := core.O_CREATE | core.O_EXCL | core.O_WRONLY | syscall.O_NOFOLLOW
	r := core.OpenFile(path, flag, core.FileMode(0o600))
	if r.OK {
		return r
	}
	err, _ := r.Value.(error)
	if err == nil {
		return r
	}
	// ELOOP fires when O_NOFOLLOW hits a symlink. macOS + Linux both
	// surface this through syscall.ELOOP. Distinguish from EEXIST so
	// the audit row carries the attacker-signal vs racing-user signal.
	if core.Is(err, syscall.ELOOP) {
		return core.Fail(core.E(fetchOp,
			reasonQuarantineSymlinkRefused+": "+path, err))
	}
	if core.IsExist(err) {
		return core.Fail(core.E(fetchOp,
			reasonQuarantineExists+": "+path, err))
	}
	return core.Fail(core.E(fetchOp, "quarantine create failed", err))
}
