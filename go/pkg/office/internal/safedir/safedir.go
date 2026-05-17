// SPDX-Licence-Identifier: EUPL-1.2

// Package safedir provides a symlink-guarded MkdirAll for office sub-
// surfaces (mail / documents / files). An attacker who can pre-create a
// symlink at the office sub-tree root (e.g. ~/Lethean/office/mail →
// /tmp/evil) would otherwise redirect every subsequent write under that
// directory to the attacker location. core.MkdirAll silently accepts a
// pre-existing symlink-to-directory as "already there" and returns OK.
//
// MkdirAll wraps the discipline once: Lstat the target path BEFORE
// calling core.MkdirAll. If the path exists and is a symlink, refuse
// with the typed code "office.dir.unsafe_symlink_parent". If it exists
// and is a real directory, fall through to core.MkdirAll (which becomes
// a no-op). If it does not exist, core.MkdirAll creates it cleanly.
//
// Usage example:
//
//	r := safedir.MkdirAll(dir, 0o700)
//	if !r.OK { return r }
//
// Scope (Mantis #1499 MED, Cerberus wave-2/pass-10): the guard covers
// the office sub-tree root each callsite creates (mailDir, docsDir,
// folderDir, stateDirPath). It does NOT walk every intermediate
// ancestor — paths.Root() is presumed trusted because ~/Lethean/ itself
// is the user's home subtree owned by the same UID.
package safedir

import core "dappco.re/go"

// UnsafeSymlinkParentCode is the typed code returned when MkdirAll
// refuses a path that exists and is a symlink.
const UnsafeSymlinkParentCode = "office.dir.unsafe_symlink_parent"

// MkdirAll Lstat-guards dir, then delegates to core.MkdirAll.
//
//   - If Lstat finds a symlink at dir → refuse with UnsafeSymlinkParentCode.
//   - If Lstat finds a real directory → fall through (core.MkdirAll no-ops).
//   - If Lstat finds any non-directory non-symlink → refuse with
//     "office.dir.unsafe_non_directory" (defence-in-depth — a pre-placed
//     regular file at the sub-tree root is also an unsafe substrate).
//   - If Lstat reports "does not exist" → fall through (core.MkdirAll
//     creates cleanly).
//   - Any other Lstat error → propagate.
//
// Usage example:
//
//	r := safedir.MkdirAll(core.PathJoin(root, "office", "mail"), 0o700)
//	if !r.OK { return r }
func MkdirAll(dir string, mode core.FileMode) core.Result {
	lstatR := core.Lstat(dir)
	switch {
	case lstatR.OK:
		info, _ := lstatR.Value.(core.FsFileInfo)
		if info == nil {
			return core.Fail(core.NewCode("safedir.MkdirAll",
				"lstat returned nil info"))
		}
		if info.Mode()&core.ModeSymlink != 0 {
			return core.Fail(core.NewCode(UnsafeSymlinkParentCode,
				"refuse mkdir on existing symlink at "+dir))
		}
		if !info.IsDir() {
			return core.Fail(core.NewCode("office.dir.unsafe_non_directory",
				"refuse mkdir on existing non-directory at "+dir))
		}
		// Real directory — core.MkdirAll will no-op. Fall through.
	default:
		err, _ := lstatR.Value.(error)
		if err == nil || !core.IsNotExist(err) {
			if err == nil {
				return core.Fail(core.NewCode("safedir.MkdirAll",
					"lstat failed without error at "+dir))
			}
			return core.Fail(core.E("safedir.MkdirAll",
				"lstat failed at "+dir, err))
		}
		// Path does not exist — fall through.
	}
	return core.MkdirAll(dir, mode)
}
