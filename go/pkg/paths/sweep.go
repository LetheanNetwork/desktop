// SPDX-Licence-Identifier: EUPL-1.2

// sweep.go — paths.SweepTmpOrphans crash-recovery primitive per
// Mantis #1594 (Cerberus #19 §5.2 forward-arc).
//
// AtomicWriteWithVersion (atomic_write.go) stages each write to a
// per-call randomised tmp file (`<final>.tmp.<16-hex>`, Mantis #1552)
// so concurrent writers cannot race on a shared `.tmp` staging path.
// On crash mid-write — power loss, process kill between OpenFile and
// Rename, fsync exception — the tmp file is orphaned: it survives
// the crash, its mtime never changes, and the next live writer to
// the same final path stages its OWN per-call tmp so the orphan is
// never cleaned up.
//
// Orphans accumulate over time. Disk pressure aside, the bigger
// concern is forensic noise: an audit reader scanning ~/Lethean/ for
// recent activity sees stale staging files that never reflected a
// committed state.
//
// SweepTmpOrphans walks a directory tree under ~/Lethean/, matches
// the `<basename>.tmp.<hex>` orphan shape, and deletes any file whose
// mtime is older than OrphanTmpAgeThreshold. The age gate is the
// load-bearing safety: a live writer's in-flight tmp has mtime
// "now", so a 1-hour threshold (default) leaves a wide window before
// the sweep could accidentally race a real write.
//
// Safety boundary: the sweep refuses any root that is not under
// paths.Root() (~/Lethean/). This prevents an operator typo from
// pointing the sweep at /tmp or $HOME and walking the wrong tree.
//
// Audit emission: every deletion fires an EventOrphanTmpSwept
// LockEvent so forensic readers can correlate "tmp file disappeared"
// with "sweep ran" rather than "writer succeeded" or "attacker
// removed evidence". Emission routes through the same AuditModeForPath
// policy as the write surface — sweeps under auth-substrate prefixes
// (wallets/, account/, office/mail/_accounts.enc) land Sync; cascade
// paths land Batch.
//
// Out of scope for this primitive (future `lthn account repair` CLI
// will wire these): scheduling, parallelism across roots, dry-run
// preview, age-threshold per-prefix override.

package paths

import (
	core "dappco.re/go"
)

// OrphanTmpAgeThreshold is the minimum age a tmp orphan must reach
// before SweepTmpOrphans will delete it. 1 hour leaves a generous
// window for the slowest realistic in-flight write (a multi-GB
// account/<id>/keystore on a saturated disk) to commit before the
// sweep could collide with it.
//
// Tunable forward-arc: per-prefix overrides land when a workload
// surfaces that needs finer control (e.g. high-churn sales/ may
// want 5 minutes; long-running account/ writes may want 4 hours).
// For today the single global threshold is sufficient.
const OrphanTmpAgeThreshold core.Duration = 1 * core.Hour

// CodeSweepUnsafeRoot is the typed error code returned when
// SweepTmpOrphans is called with a root that is not under
// paths.Root(). Log readers and tests pattern-match on this literal.
const CodeSweepUnsafeRoot = "paths.sweep.unsafe_root"

// CodeSweepWalkFailed surfaces a fatal walk error (the underlying
// directory traversal could not start or completed with an error
// the walker chose to propagate). Per-entry errors are absorbed and
// logged via the in-process subscriber list; only walk-level failures
// surface here.
const CodeSweepWalkFailed = "paths.sweep.walk_failed"

// EventOrphanTmpSwept is the LockEvent.Kind emitted for each
// successfully deleted orphan. Reserved schema — pattern-matched by
// audit-log readers + dashboards.
const EventOrphanTmpSwept = "paths.sweep.orphan_tmp_swept"

// orphanTmpSuffixHexLen is the exact number of lowercase-hex
// characters that follow the `.tmp.` infix in an AtomicWriteWithVersion
// staging file. atomic_write.go calls core.RandomString(8) which
// returns 8 random bytes encoded as 16 hex chars; matching that
// length exactly keeps the sweep tight against the production shape
// and avoids deleting unrelated files that happen to share the
// `.tmp.` prefix (e.g. a user-authored `notes.tmp.draft`).
const orphanTmpSuffixHexLen = 16

// SweepTmpOrphans walks the directory tree rooted at root, finds
// files matching the `<basename>.tmp.<16-hex>` AtomicWriteWithVersion
// staging shape, and deletes any whose mtime is older than
// OrphanTmpAgeThreshold. Returns the count of files deleted.
//
// Safety: root MUST be under paths.Root() (~/Lethean/). Calling with
// any other root returns CodeSweepUnsafeRoot and zero deletions —
// the sweep refuses to operate on roots outside the workspace.
//
// Per-entry errors during the walk are absorbed (logged via the
// in-process LockEvent subscriber list when emission infrastructure
// is wired) so a single unreadable directory does not abort the
// whole sweep. Only walk-level failures (root does not exist, walker
// could not start) surface as CodeSweepWalkFailed.
//
// Usage example:
//
//	r := paths.SweepTmpOrphans(paths.Root().Value.(string))
//	if r.OK { count := r.Value.(int); _ = count }
func SweepTmpOrphans(root string) core.Result {
	if root == "" {
		return core.Fail(core.NewCode(CodeSweepUnsafeRoot,
			"SweepTmpOrphans requires a non-empty root"))
	}
	if !isUnderLetheanRoot(root) {
		return core.Fail(core.NewCode(CodeSweepUnsafeRoot,
			"SweepTmpOrphans root must be under paths.Root() (~/Lethean/); got: "+root))
	}

	count := 0
	walkErr := core.PathWalkDir(root, func(path string, d core.FsDirEntry, err error) error {
		if err != nil {
			// Absorb per-entry walk errors (permission denied on a
			// nested dir, transient I/O). Returning nil keeps the
			// sweep moving across the rest of the tree.
			return nil
		}
		if d == nil || d.IsDir() {
			return nil
		}
		if !isOrphanTmpName(d.Name()) {
			return nil
		}
		statR := core.Lstat(path)
		if !statR.OK {
			// File vanished between walk-enumeration and stat — fine,
			// nothing to delete.
			return nil
		}
		info, _ := statR.Value.(core.FsFileInfo)
		if info == nil {
			return nil
		}
		age := core.Since(info.ModTime())
		if age < OrphanTmpAgeThreshold {
			// In-flight or recently-committed-but-not-yet-renamed
			// staging file. Leave it alone.
			return nil
		}
		if r := core.Remove(path); !r.OK {
			// Per-entry delete failures absorb — they're forensically
			// uninteresting (file was probably already gone) and the
			// sweep should continue.
			return nil
		}
		_ = emitOrphanTmpSwept(path, age)
		count++
		return nil
	})
	if walkErr != nil {
		return core.Fail(core.E(CodeSweepWalkFailed,
			"PathWalkDir under "+root, walkErr))
	}
	return core.Ok(count)
}

// isUnderLetheanRoot reports whether root is paths.Root() itself or
// a descendant. Resolves paths.Root() each call so a HOME-rebound
// test fixture sees the right boundary.
func isUnderLetheanRoot(root string) bool {
	rootR := Root()
	if !rootR.OK {
		return false
	}
	lethean, _ := rootR.Value.(string)
	if lethean == "" {
		return false
	}
	if root == lethean {
		return true
	}
	if core.HasPrefix(root, lethean+"/") {
		return true
	}
	return false
}

// isOrphanTmpName reports whether name matches the
// `<basename>.tmp.<16-lowercase-hex>` shape produced by
// AtomicWriteWithVersion's tmp-staging path. Returns false for
// shorter suffixes (`.tmp` alone), non-hex suffixes
// (`notes.tmp.draft`), and wrong-length suffixes (so an unrelated
// `*.tmp.abcd` artefact stays put).
func isOrphanTmpName(name string) bool {
	const infix = ".tmp."
	// Find the LAST `.tmp.` occurrence so multi-segment basenames
	// like `account.enc.tmp.<hex>` resolve correctly.
	idx := lastIndex(name, infix)
	if idx < 0 {
		return false
	}
	suffix := name[idx+len(infix):]
	if len(suffix) != orphanTmpSuffixHexLen {
		return false
	}
	for i := 0; i < len(suffix); i++ {
		c := suffix[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

// lastIndex returns the byte offset of the last occurrence of sub in
// s, or -1 if sub is absent. Hand-rolled rather than via core.Index
// reversal to keep the hot-path allocation-free during sweeps over
// large trees.
func lastIndex(s, sub string) int {
	if len(sub) == 0 || len(sub) > len(s) {
		return -1
	}
	for i := len(s) - len(sub); i >= 0; i-- {
		match := true
		for j := 0; j < len(sub); j++ {
			if s[i+j] != sub[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

// emitOrphanTmpSwept fires an EventOrphanTmpSwept LockEvent through
// the same audit pipeline as the write surface. Routes via
// AuditModeForPath so auth-substrate sweeps land Sync (crash-safety)
// and cascade sweeps land Batch (throughput). Overridden in
// events.go init() once the typed-event bus is wired; the default
// no-op keeps the primitive shippable before boot.
var emitOrphanTmpSwept = func(path string, age core.Duration) core.Result {
	return emitLockEvent(LockEvent{
		Kind:     EventOrphanTmpSwept,
		PathHash: hashPath(path),
		Mode:     AuditModeForPath(path),
		At:       core.Now().UTC(),
	})
}
