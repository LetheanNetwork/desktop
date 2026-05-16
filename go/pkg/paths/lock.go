// SPDX-Licence-Identifier: EUPL-1.2

// lock.go — sentinel-file-based exclusive lock per
// RFC.atomic-write.md §1.2. CoreGO does not yet wrap flock(2)
// (logged as a gap; the strong case is documented in the RFC §10
// CoreGO-primitive list), so v1 ships an O_CREATE|O_EXCL sentinel
// approach. When core.Flock lands, WithFileLock's body swaps to
// flock(2) without changing the exported signature — consumers see
// no contract change.
//
// Lock semantics (v1 sentinel):
//
//   1. Sentinel suffix is ".lockfile" (LOW-2 fix — ".lock" collides
//      with legitimate workspace slugs like sales/lock.md).
//   2. OpenFile(sentinel, O_CREATE|O_EXCL|O_WRONLY, 0o600) — kernel
//      atomically tests-and-creates. No pre-Stat call (TOCTOU window).
//   3. On contention (existing sentinel): read the PID + fork-time
//      stamp inside; if PID is dead OR PID-alive-with-different-fork-
//      time → reclaim (Unlink + retry). Otherwise spin-wait with
//      exponential backoff up to Timeout.
//   4. On success: write "<pid>\n<fork-time-unix-ns>\n" to sentinel,
//      close handle (kernel does NOT need the FD held — unlink on
//      release is the discipline).
//   5. Release = Unlink. Use defer.
//
// MED-1 (fork-time NOT wall-clock): the stamp uses the PID's fork-
// time as reported by the OS (proc_pidinfo on macOS, /proc/<pid>/stat
// field 22 on Linux). Wall-clock comparisons would leak system-clock
// skew as a false-positive stale-lock trigger; fork-time-stamps are
// monotonic-relative-to-boot.
//
// HIGH-2 (NFS reject): WithFileLock consults RootFSType() at every
// call and fails-closed on network filesystems (nfs/nfs4/smbfs/cifs/
// fuse.*) with the typed code "paths.lock.network_fs". The
// per-call check (rather than boot-time-once) lets the user mount /
// unmount NFS without restarting lthn while still guaranteeing no
// silent corruption.

package paths

import (
	core "dappco.re/go"
)

// LockTimeoutDefault is the wait ceiling when a caller does not
// supply WriteInput.Timeout. 5s matches the RFC §10 default and the
// auth-substrate writers' tolerance ceiling.
const LockTimeoutDefault = 5 * core.Second

// LockfileSuffix is the sentinel-file suffix per §1.3 LOW-2.
// ".lockfile" was picked over ".lock" to avoid collision with valid
// workspace slugs (sales/lock.md, runbooks/lock.md).
const LockfileSuffix = ".lockfile"

// Error codes the lock primitive emits. Reserved schema —
// frontend / audit-tail / log-tail surfaces pattern-match on these
// literals.
const (
	CodeLockTimeout   = "paths.lock.timeout"
	CodeLockNetworkFS = "paths.lock.network_fs"
	CodeLockOpen      = "paths.lock.open_failed"
	CodeLockRelease   = "paths.lock.release_failed"
)

// backoffSteps controls the spin-wait progression on lock
// contention. Starts at 5ms, doubles each step up to 100ms cap,
// then holds at 100ms until Timeout elapses. Total polling cost on
// a contended file held for 1s ≈ 25 wakeups.
var backoffSteps = []core.Duration{
	5 * core.Millisecond,
	10 * core.Millisecond,
	20 * core.Millisecond,
	40 * core.Millisecond,
	80 * core.Millisecond,
	100 * core.Millisecond,
}

// WithFileLock acquires an exclusive sentinel-file lock against the
// supplied path, runs fn with the lock held, and releases on return.
// Locks are held on a sibling ".lockfile" rather than the target
// itself so the primitive can lock paths that do not yet exist (the
// canonical "first-write" case).
//
// timeout of 0 expands to LockTimeoutDefault. fn MUST NOT call
// WithFileLock on the same path recursively — the sentinel approach
// is NOT re-entrant. Caller responsibility.
//
// Returns the Result returned by fn on success. On failure paths:
//
//   - Network filesystem detected → Fail(CodeLockNetworkFS)
//   - Wait exceeded timeout       → Fail(CodeLockTimeout)
//   - Sentinel open failed (perm) → Fail(CodeLockOpen)
//
// Usage example:
//
//	r := paths.WithFileLock(filePath, 2*core.Second, func() core.Result {
//	    return doTheWrite(filePath)
//	})
//	if !r.OK { return r }
func WithFileLock(path string, timeout core.Duration, fn func() core.Result) core.Result {
	if path == "" {
		return core.Fail(core.NewCode("paths.lock.invalid_path",
			"WithFileLock requires a non-empty path"))
	}
	if fn == nil {
		return core.Fail(core.NewCode("paths.lock.invalid_fn",
			"WithFileLock requires a non-nil fn"))
	}
	// HIGH-2: refuse to operate on network filesystems where flock
	// semantics are not enforceable.
	if r := RootFSType(); r.OK {
		if IsNetworkFS(r.Value.(string)) {
			return core.Fail(core.NewCode(CodeLockNetworkFS,
				"refusing to lock on network filesystem: "+r.Value.(string)))
		}
	}
	if timeout <= 0 {
		timeout = LockTimeoutDefault
	}
	sentinel := path + LockfileSuffix
	deadline := core.Now().Add(timeout)
	step := 0
	for {
		acquired, fatal := tryAcquireSentinel(sentinel)
		if acquired {
			defer releaseSentinel(sentinel)
			return fn()
		}
		if fatal != nil {
			return core.Fail(core.E(CodeLockOpen,
				"sentinel open failed: "+fatal.Error(), fatal))
		}
		// Contended — check whether the holder is alive.
		if maybeReclaim(sentinel) {
			// Reclaimed; retry without waiting.
			continue
		}
		if !core.Now().Before(deadline) {
			return core.Fail(core.NewCode(CodeLockTimeout,
				"timed out waiting for lock on "+path))
		}
		// Exponential backoff bounded at 100ms.
		wait := backoffSteps[len(backoffSteps)-1]
		if step < len(backoffSteps) {
			wait = backoffSteps[step]
			step++
		}
		core.Sleep(wait)
	}
}

// tryAcquireSentinel attempts to create the sentinel file with
// O_CREATE|O_EXCL. Returns (acquired, fatalErr):
//
//   - (true, nil)   — we hold the lock.
//   - (false, nil)  — sentinel already exists; contention; retry.
//   - (false, err)  — fatal I/O error (parent missing, permission,
//                     read-only filesystem). Outer loop MUST surface
//                     this rather than spin to timeout.
//
// On success the file body is "<pid>\n<fork-time-unix-ns>\n" so the
// reclaim path can validate the holder's identity.
func tryAcquireSentinel(sentinel string) (bool, error) {
	openR := core.OpenFile(sentinel,
		core.O_CREATE|core.O_EXCL|core.O_WRONLY, 0o600)
	if !openR.OK {
		err := unwrapErr(openR)
		if core.IsExist(err) {
			// Contention — sentinel already held.
			return false, nil
		}
		// Anything else (ENOENT on parent dir, EACCES, EROFS) is a
		// fatal acquire-failure; surface it.
		if err == nil {
			err = core.NewCode(CodeLockOpen, "OpenFile failed without error")
		}
		return false, err
	}
	f, _ := openR.Value.(*core.OSFile)
	if f == nil {
		return false, core.NewCode(CodeLockOpen, "OpenFile returned nil file")
	}
	body := sentinelBody(core.Getpid(), processForkTimeUnixNanos(core.Getpid()))
	if _, err := f.Write(body); err != nil {
		_ = f.Close()
		_ = core.Remove(sentinel)
		return false, err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = core.Remove(sentinel)
		return false, err
	}
	if err := f.Close(); err != nil {
		_ = core.Remove(sentinel)
		return false, err
	}
	return true, nil
}

// releaseSentinel unlinks the sentinel file so subsequent acquisitions
// can succeed. Remove errors are not propagated — fn has already
// executed and a leaked sentinel is recoverable by the next reclaim
// pass. The most common error is the benign reclaim-already-removed
// race; logging it as a warning would just be noise.
func releaseSentinel(sentinel string) {
	_ = core.Remove(sentinel)
}

// maybeReclaim inspects an existing sentinel. Returns true when the
// sentinel was reclaimed (caller should immediately retry acquire);
// false when the sentinel is still owned by a live process with a
// matching fork-time stamp.
//
// Reclaim triggers:
//
//   - sentinel body unreadable / malformed → reclaim (corrupted)
//   - recorded PID does not currently exist → reclaim (dead holder)
//   - recorded PID exists but its current fork-time differs from
//     the recorded stamp → reclaim (PID was recycled — MED-1)
func maybeReclaim(sentinel string) bool {
	readR := core.ReadFile(sentinel)
	if !readR.OK {
		// File vanished between contention and reclaim — let the
		// outer loop retry; some other waiter likely raced us to
		// the acquire.
		return true
	}
	raw, _ := readR.Value.([]byte)
	pid, fork, ok := parseSentinelBody(raw)
	if !ok {
		// Malformed sentinel — reclaim.
		_ = core.Remove(sentinel)
		return true
	}
	if !processAlive(pid) {
		_ = core.Remove(sentinel)
		return true
	}
	current := processForkTimeUnixNanos(pid)
	if current == 0 {
		// Couldn't read fork-time but PID exists — be conservative;
		// do NOT reclaim. Outer loop will spin until timeout.
		return false
	}
	if current != fork {
		// PID recycled — the original holder died and the kernel
		// reissued the PID to a new process. Reclaim.
		_ = core.Remove(sentinel)
		return true
	}
	return false
}

// sentinelBody renders the canonical sentinel-file bytes:
//
//	"<pid>\n<fork-time-unix-ns>\n"
//
// The trailing newline keeps the file parseable by hand for
// operators inspecting a stuck lock with `cat <path>.lockfile`.
func sentinelBody(pid int, fork int64) []byte {
	s := core.Itoa(pid) + "\n" + core.Sprintf("%d", fork) + "\n"
	return []byte(s)
}

// parseSentinelBody is the inverse of sentinelBody. Returns
// (pid, fork-time, ok). ok=false on any malformation so the caller
// can treat the sentinel as stale and reclaim it.
func parseSentinelBody(raw []byte) (pid int, fork int64, ok bool) {
	// Split on newline. Expect at least two non-empty lines.
	lines := splitNewlines(raw)
	if len(lines) < 2 {
		return 0, 0, false
	}
	pR := core.Atoi(lines[0])
	if !pR.OK {
		return 0, 0, false
	}
	fR := core.Atoi(lines[1])
	if !fR.OK {
		return 0, 0, false
	}
	pid = pR.Value.(int)
	fork = int64(fR.Value.(int))
	if pid <= 0 {
		return 0, 0, false
	}
	return pid, fork, true
}

// splitNewlines splits raw on '\n' without importing strings.
// Empty trailing segments are dropped so a "x\n" input yields
// ["x"], not ["x", ""].
func splitNewlines(raw []byte) []string {
	var out []string
	start := 0
	for i := 0; i < len(raw); i++ {
		if raw[i] == '\n' {
			if i > start {
				out = append(out, string(raw[start:i]))
			} else {
				out = append(out, "")
			}
			start = i + 1
		}
	}
	if start < len(raw) {
		out = append(out, string(raw[start:]))
	}
	return out
}
