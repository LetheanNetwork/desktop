// SPDX-Licence-Identifier: EUPL-1.2

// Package rotation owns the gzip compression + retention-candidate
// rename + cursor-friendly day-file resolution helpers per
// RFC.stage-f.md §4.2.
//
// Authorised compress/gzip import per RFC §10 ADD-LOW-1 — pkg/audit
// MUST NOT import compress/gzip directly; this internal package is
// the single load-bearing consumer. When CoreGO exports a core.Gzip
// wrapper the gzip import here retires; the package's public surface
// (Compress + Candidate + ResolveDayFile) keeps the same shapes so
// pkg/audit doesn't need a coordinated bump.
//
// Three primitives:
//
//   - Compress(path) — gzips `<name>.log` to `<name>.log.gz`, removes
//     the original on success. Returns the new path.
//   - Candidate(path) — renames `<name>.log.gz` to
//     `<name>.log.gz.candidate` (the RFC §4.2 retention-overdue
//     marker). Operator can rename back or `rm` directly.
//   - ResolveDayFile(root, date) — given the YYYY-MM-DD date stem,
//     tries `.log`, `.log.gz`, `.log.gz.candidate` in that order and
//     returns the first that exists. Used by the streaming-cursor
//     Query path so a cursor that named "2026-05-15" still resumes
//     after the file rotated through compression + retention rename.

package rotation

import (
	"compress/gzip"
	"io"

	core "dappco.re/go"
)

// LogSuffix is the active-day file suffix (uncompressed NDJSON).
const LogSuffix = ".log"

// CompressedSuffix is the post-compression suffix.
const CompressedSuffix = ".log.gz"

// CandidateSuffix marks a file that has aged past the retention
// window. The rename to this suffix IS the persistent "logged once"
// marker per RFC §4.2 — no separate state file needed.
const CandidateSuffix = ".log.gz.candidate"

// Compress gzips inPath in place. Reads inPath, writes
// `<inPath without .log>.log.gz` at fileMode, fsyncs the gz output,
// then removes the original. Returns the output path on success.
//
// Caller MUST hold an exclusive lock on inPath for the duration —
// pkg/audit's rotation goroutine handles this by only compressing
// files for past days (the current day's handle is never compressed).
//
// Usage example (internal):
//
//	gzPath, r := rotation.Compress("/Users/u/Lethean/audit/2026-05-15.log", 0o600)
//	if !r.OK { core.Print(core.Stderr(), "compress failed: %s\n", r.Error()) }
func Compress(inPath string, fileMode core.FileMode) (string, core.Result) {
	if !core.HasSuffix(inPath, LogSuffix) {
		return "", core.Fail(core.NewCode("audit.rotation.suffix",
			"compress source path must end in .log"))
	}
	outPath := inPath[:len(inPath)-len(LogSuffix)] + CompressedSuffix

	srcR := core.OpenFile(inPath, core.O_RDONLY, 0)
	if !srcR.OK {
		return "", core.Fail(core.E("rotation.Compress", "open source", srcR.Value.(error)))
	}
	src, _ := srcR.Value.(*core.OSFile)
	defer func() { _ = src.Close() }()

	dstR := core.OpenFile(outPath, core.O_CREATE|core.O_WRONLY|core.O_TRUNC, fileMode)
	if !dstR.OK {
		return "", core.Fail(core.E("rotation.Compress", "open dest", dstR.Value.(error)))
	}
	dst, _ := dstR.Value.(*core.OSFile)

	zw := gzip.NewWriter(dst)
	if _, err := io.Copy(zw, src); err != nil {
		_ = zw.Close()
		_ = dst.Close()
		_ = core.Remove(outPath)
		return "", core.Fail(core.E("rotation.Compress", "stream copy", err))
	}
	if err := zw.Close(); err != nil {
		_ = dst.Close()
		_ = core.Remove(outPath)
		return "", core.Fail(core.E("rotation.Compress", "close gzip writer", err))
	}
	if err := dst.Sync(); err != nil {
		_ = dst.Close()
		_ = core.Remove(outPath)
		return "", core.Fail(core.E("rotation.Compress", "fsync output", err))
	}
	if err := dst.Close(); err != nil {
		_ = core.Remove(outPath)
		return "", core.Fail(core.E("rotation.Compress", "close output", err))
	}
	if r := core.Remove(inPath); !r.OK {
		// Output is durable; failure to remove the source leaves
		// duplicate data on disk but doesn't lose anything. Surface
		// the error so the operator can clean up.
		return outPath, core.Fail(core.E("rotation.Compress", "remove source after compress", r.Value.(error)))
	}
	return outPath, core.Ok(outPath)
}

// Candidate renames a `.log.gz` to `.log.gz.candidate`. Returns the
// new path. Idempotent — if the source already has the candidate
// suffix it's returned unchanged.
//
// Usage example (internal):
//
//	newPath, r := rotation.Candidate("/path/2026-02-13.log.gz")
//	if r.OK { core.Print(core.Stderr(), "retention-flagged %s\n", newPath) }
func Candidate(inPath string) (string, core.Result) {
	if core.HasSuffix(inPath, CandidateSuffix) {
		return inPath, core.Ok(inPath)
	}
	if !core.HasSuffix(inPath, CompressedSuffix) {
		return "", core.Fail(core.NewCode("audit.rotation.suffix",
			"candidate rename source path must end in .log.gz"))
	}
	outPath := inPath + ".candidate"
	if r := core.Rename(inPath, outPath); !r.OK {
		return "", core.Fail(core.E("rotation.Candidate", "rename to .candidate", r.Value.(error)))
	}
	return outPath, core.Ok(outPath)
}

// ResolveDayFile returns the on-disk path for the supplied YYYY-MM-DD
// stem under root, trying suffix candidates in priority order:
//
//  1. <stem>.log (active or recent uncompressed file)
//  2. <stem>.log.gz (compressed but within retention)
//  3. <stem>.log.gz.candidate (retention-flagged but operator-readable)
//
// Returns ("", false) when none of the three exist — caller treats
// as a normal "no events for that day" outcome, NOT an error.
//
// Usage example (internal):
//
//	path, ok := rotation.ResolveDayFile("/Users/u/Lethean/audit", "2026-05-15")
//	if !ok { return nil } // no events for that day
func ResolveDayFile(root, dateStem string) (string, bool) {
	if root == "" || dateStem == "" {
		return "", false
	}
	for _, suffix := range []string{LogSuffix, CompressedSuffix, CandidateSuffix} {
		candidate := core.PathJoin(root, dateStem+suffix)
		if core.Stat(candidate).OK {
			return candidate, true
		}
	}
	return "", false
}

// OpenForRead returns a reader over the supplied path, transparently
// decompressing `.log.gz` / `.log.gz.candidate` files. Caller MUST
// invoke the returned close func before the next ResolveDayFile loop
// iteration so file handles don't leak across days.
//
// Returns (nil, noop-close, fail) on open / decompression failure.
//
// Usage example (internal):
//
//	path, ok := rotation.ResolveDayFile(root, "2026-05-15")
//	if !ok { continue }
//	rd, closer, r := rotation.OpenForRead(path)
//	if !r.OK { continue }
//	defer closer()
//	for { /* scan rd line by line */ }
func OpenForRead(path string) (io.Reader, func(), core.Result) {
	noop := func() {}
	openR := core.OpenFile(path, core.O_RDONLY, 0)
	if !openR.OK {
		return nil, noop, core.Fail(core.E("rotation.OpenForRead", "open file", openR.Value.(error)))
	}
	f, _ := openR.Value.(*core.OSFile)

	if core.HasSuffix(path, CompressedSuffix) || core.HasSuffix(path, CandidateSuffix) {
		zr, err := gzip.NewReader(f)
		if err != nil {
			_ = f.Close()
			return nil, noop, core.Fail(core.E("rotation.OpenForRead", "init gzip reader", err))
		}
		close := func() {
			_ = zr.Close()
			_ = f.Close()
		}
		return zr, close, core.Ok(nil)
	}
	close := func() {
		_ = f.Close()
	}
	return f, close, core.Ok(nil)
}
