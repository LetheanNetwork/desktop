// SPDX-Licence-Identifier: EUPL-1.2

// pipebuf.go — per-platform PIPE_BUF constant required by
// AtomicAppendLine for kernel-atomic write enforcement (RFC §5.1).
// The stdlib does NOT expose PIPE_BUF as a portable constant; the
// values below come from <limits.h> on each platform:
//
//   - Darwin / macOS:  512 bytes (POSIX minimum, Darwin keeps it)
//   - Linux:          4096 bytes (Linux raised the floor to PAGE_SIZE)
//   - Other:           512 bytes (POSIX guarantee — conservative)
//
// Larger records still write, but the kernel may interleave with
// concurrent writers — for the threads.md append-many-writers case
// this would corrupt records. AtomicAppendLine rejects > limit so
// the discipline is enforced at the API boundary, not deep in the
// caller's tests.

package paths

const (
	pipeBufDarwin = 512
	pipeBufLinux  = 4096
	pipeBufOther  = 512
)

// pipeBufLimit holds the current process's PIPE_BUF ceiling.
// Settable via SetPipeBufLimitForTest to exercise the over-limit
// reject path on systems whose native value would never be hit
// from a unit test record.
var pipeBufLimit = defaultPipeBufLimit()

// PipeBufLimit returns the per-platform PIPE_BUF byte ceiling for
// kernel-atomic write semantics. AtomicAppendLine consults this on
// every call to reject records that would lose the atomicity
// guarantee.
//
// Usage example:
//
//	if len(rec) > paths.PipeBufLimit() { fallback() }
func PipeBufLimit() int {
	return pipeBufLimit
}

// SetPipeBufLimitForTest substitutes the limit for the duration of
// the test. Always pair with t.Cleanup(func() {
// paths.SetPipeBufLimitForTest(0) }) — passing 0 resets to the
// platform default.
//
// Usage example:
//
//	paths.SetPipeBufLimitForTest(64)
//	t.Cleanup(func() { paths.SetPipeBufLimitForTest(0) })
func SetPipeBufLimitForTest(n int) {
	if n <= 0 {
		pipeBufLimit = defaultPipeBufLimit()
		return
	}
	pipeBufLimit = n
}
