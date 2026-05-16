// SPDX-Licence-Identifier: EUPL-1.2

//go:build !darwin && !linux

// process_other.go — Fallback process introspection for platforms
// without a verified /proc + sysctl implementation. Treats every
// PID > 0 as alive and returns 0 for fork-time. The sentinel-lock
// reclaim path degrades gracefully: dead-PID reclaim cannot fire, so
// only the timeout-based release recovers the lock. Acceptable for
// platforms (windows / freebsd / etc) where lthn's lock-on-shared-
// home-directory case is not the primary deployment path.

package paths

// processAlive conservatively returns true for any positive PID on
// unsupported platforms. Conservative: never claims a real holder
// is dead.
func processAlive(pid int) bool {
	return pid > 0
}

// processForkTimeUnixNanos returns 0 — the "unknown" sentinel — on
// unsupported platforms.
func processForkTimeUnixNanos(_ int) int64 {
	return 0
}
