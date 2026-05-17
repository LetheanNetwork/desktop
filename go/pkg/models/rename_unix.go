// SPDX-Licence-Identifier: EUPL-1.2

//go:build darwin || linux

package models

import (
	"os"
	"syscall"

	core "dappco.re/go"
)

// atomicRename closes the TOCTOU window in models.Rename by using
// link(2) + unlink(2) instead of Stat(dst) + rename(2). link(2) atomically
// fails with EEXIST when dst already exists — no racing-writer can
// slip in between an "is it there?" check and the rename, because
// there is no separate check. Then unlink(2) removes the source.
//
// Trade-off vs renameat2(RENAME_NOREPLACE): rename(2) is one syscall +
// truly atomic on the kernel side; link+unlink is two syscalls with a
// brief window where both names exist on disk. For an interactive
// model-rename rail that window is irrelevant — readers see the file
// under one name or the other, never zero. Crash-safety is identical
// (post-link, src is recoverable; post-unlink, dst is the durable copy).
//
// renameat2 RENAME_NOREPLACE is Linux-only (since 3.15) and not yet
// exported through CoreGO. When core.Link / core.RenameNoReplace land
// upstream this file can collapse into the platform-agnostic models.go.
//
// Mantis #1419 — Cerberus M3 TOCTOU. Filed 2026-05-16, closed via this
// commit. Symlink-source rejection shipped earlier at lthn/desktop@71d796b;
// this completes the atomic-no-clobber half.
//
// Returns a core.Result for consistency with models.go's other call
// sites — on EEXIST surfaces "destination already exists"; on other
// errors wraps with the operation scope.
//
// Usage example:
//
//	if r := atomicRename(src, dst); !r.OK { return r }
func atomicRename(src, dst string) core.Result {
	if err := syscall.Link(src, dst); err != nil {
		if os.IsExist(err) {
			return core.Fail(core.NewError(
				"models.Rename: destination already exists: " + dst))
		}
		return core.Fail(core.E("models.Rename", "link failed", err))
	}
	if err := syscall.Unlink(src); err != nil {
		// Source unlink failed — destination already landed via link(2).
		// Best-effort cleanup: try to undo the link so caller sees a
		// recognisable failure mode rather than two copies on disk. If
		// the undo also fails, surface the original unlink error.
		_ = syscall.Unlink(dst)
		return core.Fail(core.E("models.Rename", "source unlink failed", err))
	}
	return core.Ok(dst)
}
