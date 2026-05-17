// SPDX-Licence-Identifier: EUPL-1.2

//go:build !darwin && !linux

package downloader

import core "dappco.re/go"

// reasonQuarantineExists — see quarantine_create_unix.go. Mirrored
// here so consumers can reference the constant without a build tag.
const reasonQuarantineExists = "download.quarantine_exists"

// reasonQuarantineSymlinkRefused — see quarantine_create_unix.go.
// Mirrored without the symlink-refusal semantic teeth on non-POSIX;
// the constant exists so code paths compile cross-platform.
const reasonQuarantineSymlinkRefused = "download.quarantine_symlink_refused"

// createQuarantineFile is the non-POSIX fallback — gets O_EXCL via
// core.OpenFile but no O_NOFOLLOW (Windows / WASM don't surface a
// portable equivalent). Lethean Desktop ships to macOS today; this
// branch keeps the package buildable on other targets.
//
// Usage example:
//
//	r := createQuarantineFile(qDest)
//	if !r.OK { return r }
//	file := r.Value.(*core.OSFile)
//	defer file.Close()
func createQuarantineFile(path string) core.Result {
	flag := core.O_CREATE | core.O_EXCL | core.O_WRONLY
	r := core.OpenFile(path, flag, core.FileMode(0o600))
	if r.OK {
		return r
	}
	err, _ := r.Value.(error)
	if err != nil && core.IsExist(err) {
		return core.Fail(core.E(fetchOp,
			reasonQuarantineExists+": "+path, err))
	}
	return core.Fail(core.E(fetchOp, "quarantine create failed", err))
}
