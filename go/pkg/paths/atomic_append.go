// SPDX-Licence-Identifier: EUPL-1.2

// atomic_append.go — paths.AtomicAppendLine + paths.PipeBufLimit
// per RFC.atomic-write.md §5.1 (Call 2 OVERRIDE: Option B). The
// primitive is the IMAP-fetch-friendly append path that skips the
// read-current + version-check + whole-file-rewrite cost.
//
// Semantics:
//
//   - O_APPEND opens the target with append mode. The kernel atomic
//     guarantee (write(2) §"Atomic/non-atomic") holds for records
//     whose size is <= PIPE_BUF (512 on Darwin, 4096 on Linux).
//   - Records exceeding PipeBufLimit() are REJECTED — callers fall
//     back to AtomicWriteWithVersion + IfMatchHash.
//   - Rotation at AppendRotateThreshold (50 MiB default) — current
//     file renamed to "<path>.<unix>.archived" + a fresh empty file
//     created in its place. Rotation runs under WithFileLock so
//     concurrent appenders see a coherent transition.
//
// Tradeoff per §5.1: IMAP-fetch I/O drops 10000× (108 GB/day → ~10
// MiB/day) at the cost of no per-record version semantics. That
// match between primitive shape and consumer pattern is what makes
// this safe; conflicts on threads.md ARE possible only during the
// user-side MarkRead path, which uses the heavier
// AtomicWriteWithVersion + IfMatchHash flow.

package paths

import (
	core "dappco.re/go"
)

// Append-path error codes. Reserved schema.
const (
	CodeAppendInvalidPath     = "paths.append.invalid_path"
	CodeAppendOpenFailed      = "paths.append.open_failed"
	CodeAppendWriteFailed     = "paths.append.write_failed"
	CodeAppendFsyncFailed     = "paths.append.fsync_failed"
	CodeAppendRotateFailed    = "paths.append.rotate_failed"
	CodeAppendRotateSplit     = "paths.append.rotate_split"
	CodeAppendRecordTooLarge  = "paths.append.record_too_large"
)

// AppendRotateThreshold is the byte-size ceiling beyond which the
// next append triggers rotation. Default 50 MiB matches §5.1's
// recommendation; production callers can override per-write via the
// SetAppendRotateThresholdForTest hook (not exported beyond tests).
const AppendRotateThreshold int64 = 50 * 1024 * 1024

// AppendRotateSuffixFormat is the suffix applied during rotation:
// "<path>.<unixnano>.archived". The suffix preserves the "find . -name
// *.archived" enumeration the mail-v2 reader uses to assemble a
// composite view across the live current + rotated history.
//
// Mantis #1554 — nanosecond precision instead of seconds. Two
// rotations triggered inside the same wall-clock second (test loops,
// burst-fed IMAP fetches that cross the threshold mid-batch) used to
// collide on identical archive filenames, with the second Rename
// silently overwriting the first archive's content. UnixNano widens
// the collision window from 1s to ~1ns. NTP backward-jumps still
// produce monotonic-clock anomalies but the existing rollback path
// (#1527) keeps the on-disk state coherent.
const AppendRotateSuffixFormat = ".%d.archived"

// appendRotateThreshold is the live override; tests rewrite it via
// SetAppendRotateThresholdForTest. Production callers MUST NOT
// touch this — the constant above is the public contract.
var appendRotateThreshold = AppendRotateThreshold

// SetAppendRotateThresholdForTest substitutes the rotation threshold
// for the duration of the test. Always pair with a t.Cleanup() reset
// to AppendRotateThreshold to keep the override from leaking.
func SetAppendRotateThresholdForTest(b int64) {
	appendRotateThreshold = b
}

// rotateRecreateFaultForTest is the F3 fault-injection hook. When
// non-nil it overrides the post-rename OpenFile inside maybeRotate
// so tests can deterministically exercise the rollback path
// (Cerberus DREAD-r2 F3, Mantis #1527). Returns the Result that
// maybeRotate sees in place of the real OpenFile call. Production
// code MUST NOT touch this — pair every test setter with a
// t.Cleanup() that resets it to nil.
var rotateRecreateFaultForTest func(path string) core.Result

// SetRotateRecreateFaultForTest installs a fault-injection callback
// that maybeRotate consults in place of the recreate OpenFile.
// Pass nil to disable.
func SetRotateRecreateFaultForTest(fn func(path string) core.Result) {
	rotateRecreateFaultForTest = fn
}

// nowForRotate returns the timestamp embedded in the archive suffix.
// Indirected so Mantis #1554's NTP-backward-jump test can drive
// deterministic timestamps without burning a real second between
// rotations. Production callers always read core.Now().
var nowForRotate = func() core.Time { return core.Now() }

// SetNowForRotateForTest substitutes the rotation-timestamp source
// for the duration of a test. Always pair with t.Cleanup that resets
// it to nil (which restores the production core.Now() source).
func SetNowForRotateForTest(fn func() core.Time) {
	if fn == nil {
		nowForRotate = func() core.Time { return core.Now() }
		return
	}
	nowForRotate = fn
}

// AtomicAppendLine appends line to path with kernel-atomic write
// semantics. Records must be <= PipeBufLimit() bytes — larger
// records are rejected so the caller MUST NOT rely on
// AtomicAppendLine for content that exceeds the per-platform
// atomic-write ceiling.
//
// The primitive ensures a single trailing newline: callers may
// pass a line WITHOUT trailing '\n' (added) or WITH (preserved as
// single, not doubled).
//
// Returns Result.OK + WriteOutput on success. WriteOutput.Mtime is
// the post-append mtime; WriteOutput.Hash is the SHA-256 hex of
// the appended LINE (not the whole file).
//
// Usage example:
//
//	r := paths.AtomicAppendLine(threadsPath, []byte("a thread record"))
//	if !r.OK { return r }
//	out := r.Value.(paths.WriteOutput)
func AtomicAppendLine(path string, line []byte) core.Result {
	if path == "" {
		return core.Fail(core.NewCode(CodeAppendInvalidPath,
			"AtomicAppendLine requires a non-empty path"))
	}
	limit := PipeBufLimit()
	// Account for the trailing newline the primitive guarantees.
	effective := len(line)
	if effective == 0 || line[effective-1] != '\n' {
		effective++
	}
	if effective > limit {
		return core.Fail(core.NewCode(CodeAppendRecordTooLarge,
			core.Sprintf("record size %d > PipeBufLimit %d", effective, limit)))
	}
	return WithFileLock(path, 0, func() core.Result {
		// Rotation check — happens INSIDE the lock so concurrent
		// appenders see a coherent transition.
		if r := maybeRotate(path); !r.OK {
			return r
		}
		// Compose payload with exactly-one trailing newline.
		payload := line
		if len(payload) == 0 || payload[len(payload)-1] != '\n' {
			payload = append(append([]byte(nil), payload...), '\n')
		}
		openR := core.OpenFile(path,
			core.O_APPEND|core.O_CREATE|core.O_WRONLY, writeFileMode)
		if !openR.OK {
			return core.Fail(core.E(CodeAppendOpenFailed,
				"open append: "+openR.Error(), nil))
		}
		f, _ := openR.Value.(*core.OSFile)
		if f == nil {
			return core.Fail(core.NewCode(CodeAppendOpenFailed,
				"open append returned nil file"))
		}
		if _, err := f.Write(payload); err != nil {
			_ = f.Close()
			return core.Fail(core.E(CodeAppendWriteFailed,
				"write append", err))
		}
		if err := f.Sync(); err != nil {
			_ = f.Close()
			return core.Fail(core.E(CodeAppendFsyncFailed,
				"fsync append", err))
		}
		if err := f.Close(); err != nil {
			return core.Fail(core.E(CodeAppendOpenFailed,
				"close append", err))
		}
		out := WriteOutput{Hash: core.SHA256Hex(payload)}
		if statR := core.Lstat(path); statR.OK {
			if info, _ := statR.Value.(core.FsFileInfo); info != nil {
				out.Mtime = info.ModTime()
			}
		}
		// Mantis #1530 — Sync-mode emission Failure propagates so
		// AtomicAppendLine callers writing to auth-substrate paths
		// observe audit-recorder breakage instead of a silent green
		// return. Batch-mode (the common cascade case) returns Ok
		// regardless of recorder outcome.
		if r := emitWriteSucceeded(path, 0); !r.OK {
			return r
		}
		return core.Ok(out)
	})
}

// maybeRotate checks the current size of path against
// appendRotateThreshold. When over, the current file is renamed to
// "<path>.<unix>.archived" + a fresh empty file is created with
// the same permissions. Returns OK on no-op or successful rotation;
// Fail on rename / create errors.
//
// Rotation MUST run under WithFileLock — caller's responsibility.
// AtomicAppendLine satisfies this discipline.
func maybeRotate(path string) core.Result {
	statR := core.Lstat(path)
	if !statR.OK {
		// Not-exist is fine — first append will create the file.
		return core.Ok(nil)
	}
	info, _ := statR.Value.(core.FsFileInfo)
	if info == nil || info.Size() < appendRotateThreshold {
		return core.Ok(nil)
	}
	// Mantis #1554 — UnixNano() so two rotations in the same wall-
	// clock second produce distinct archive filenames. The previous
	// Unix() granularity could collide under burst load + the second
	// Rename would silently overwrite the first archive's content.
	archived := path + core.Sprintf(AppendRotateSuffixFormat,
		nowForRotate().UnixNano())
	if r := core.Rename(path, archived); !r.OK {
		return core.Fail(core.E(CodeAppendRotateFailed,
			"rename: "+r.Error(), nil))
	}
	// Create a fresh, empty replacement so subsequent appends in
	// the same lock window see the zero-byte file. Test fault
	// injection (F3) substitutes the result when wired.
	var openR core.Result
	if rotateRecreateFaultForTest != nil {
		openR = rotateRecreateFaultForTest(path)
	} else {
		openR = core.OpenFile(path,
			core.O_CREATE|core.O_WRONLY|core.O_TRUNC, writeFileMode)
	}
	if !openR.OK {
		// F3 (Cerberus DREAD-r2, Mantis #1527): rename succeeded but
		// the recreate failed. Without rollback the rotation left
		// state split — archived file exists, current path missing
		// — and subsequent appends would silently re-create the
		// current file with NO record of the archived history.
		// Try to roll back the rename. If rollback also fails,
		// surface CodeAppendRotateSplit with a recovery hint
		// pointing operators at the archived path.
		if rb := core.Rename(archived, path); rb.OK {
			return core.Fail(core.E(CodeAppendRotateFailed,
				"recreate empty: "+openR.Error()+
					" (rolled back rename; archived path restored)", nil))
		}
		return core.Fail(core.NewCode(CodeAppendRotateSplit,
			"split rotation state: archived="+archived+
				", current missing; rename archived back to current to recover"))
	}
	if f, _ := openR.Value.(*core.OSFile); f != nil {
		_ = f.Close()
	}
	return core.Ok(nil)
}
