// SPDX-Licence-Identifier: EUPL-1.2

// atomic_append_test.go — AtomicAppendLineLarge contract tests. The
// Large variant bypasses the PipeBufLimit gate that AtomicAppendLine
// enforces, for callers whose JSONL records routinely exceed PIPE_BUF
// (e.g. pkg/r1 — LEK-1 sandwich + model response + fingerprint map).
// The kernel still serialises concurrent O_APPEND writers per the
// open file description, so concurrent goroutines writing N-KiB lines
// MUST NOT interleave mid-line under normal load. Test naming follows
// AX-10 Good / Bad / Ugly.

package paths_test

import (
	"sync"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/paths"
)

// TestAtomicAppendLineLarge_Good_SmallRecord — Large variant SHOULD
// behave identically to AtomicAppendLine for small lines. Same file
// creation, same trailing-newline, same on-disk bytes.
func TestAtomicAppendLineLarge_Good_SmallRecord(t *core.T) {
	homeFixture(t)
	fp := tmpFile(t, "large-small.md")
	r := paths.AtomicAppendLineLarge(fp, []byte("hello small"))
	core.AssertTrue(t, r.OK, "small append should succeed: "+r.Error())
	raw := core.ReadFile(fp)
	core.AssertTrue(t, raw.OK)
	core.AssertEqual(t, "hello small\n", string(raw.Value.([]byte)))
}

// TestAtomicAppendLineLarge_Good_OversizeRecord — a 64 KiB line that
// would be rejected by AtomicAppendLine with CodeAppendRecordTooLarge
// MUST succeed via the Large variant. Lifts the canonical R₁ blocker.
func TestAtomicAppendLineLarge_Good_OversizeRecord(t *core.T) {
	homeFixture(t)
	fp := tmpFile(t, "large-oversize.md")

	// Build a 64 KiB line — well above Linux PIPE_BUF (4096) and far
	// above Darwin PIPE_BUF (512), so AtomicAppendLine would reject.
	const size = 64 * 1024
	big := make([]byte, size)
	for i := range big {
		big[i] = 'A' + byte(i%26)
	}

	// Sanity check — the same payload SHOULD be rejected by the gated
	// primitive on every supported platform.
	rejected := paths.AtomicAppendLine(fp, big)
	core.AssertFalse(t, rejected.OK)
	core.AssertContains(t, rejected.Error(), paths.CodeAppendRecordTooLarge)

	// The Large variant accepts it.
	r := paths.AtomicAppendLineLarge(fp, big)
	core.AssertTrue(t, r.OK, "oversize append should succeed: "+r.Error())

	raw := core.ReadFile(fp)
	core.AssertTrue(t, raw.OK)
	body := raw.Value.([]byte)
	// File contains exactly the payload + one trailing newline.
	core.AssertEqual(t, size+1, len(body))
	core.AssertEqual(t, byte('\n'), body[len(body)-1])
}

// TestAtomicAppendLineLarge_Good_ConcurrentWriters — 8 goroutines × 32
// KiB lines each. The kernel serialises concurrent O_APPEND writers,
// so the file MUST end with exactly 8 lines, each 32 KiB + newline,
// with no mid-line interleaving. Run under `go test -race` to catch
// any goroutine-side data race in the primitive.
func TestAtomicAppendLineLarge_Good_ConcurrentWriters(t *core.T) {
	homeFixture(t)
	fp := tmpFile(t, "large-concurrent.md")

	const (
		writers = 8
		lineLen = 32 * 1024
	)

	// Each writer's line is filled with a single distinct ASCII rune
	// so we can verify no interleaving after the fact: every line
	// MUST contain exactly one distinct character across all
	// (lineLen) bytes.
	var wg sync.WaitGroup
	wg.Add(writers)
	for i := 0; i < writers; i++ {
		i := i
		go func() {
			defer wg.Done()
			line := make([]byte, lineLen)
			fill := byte('a') + byte(i)
			for j := range line {
				line[j] = fill
			}
			r := paths.AtomicAppendLineLarge(fp, line)
			if !r.OK {
				t.Errorf("writer %d: %s", i, r.Error())
			}
		}()
	}
	wg.Wait()

	raw := core.ReadFile(fp)
	core.AssertTrue(t, raw.OK)
	body := raw.Value.([]byte)
	// Expect exactly `writers` newlines.
	count := 0
	start := 0
	for i := 0; i < len(body); i++ {
		if body[i] == '\n' {
			line := body[start:i]
			if len(line) != lineLen {
				t.Errorf("line %d wrong length: got %d, want %d", count, len(line), lineLen)
			}
			// Every byte in the line MUST equal the first byte —
			// interleaved kernel writes would surface as mixed chars.
			first := line[0]
			for k := 1; k < len(line); k++ {
				if line[k] != first {
					t.Fatalf("line %d interleaved at byte %d: got %q, expected %q",
						count, k, line[k], first)
				}
			}
			count++
			start = i + 1
		}
	}
	core.AssertEqual(t, writers, count)
}

// TestAtomicAppendLineLarge_Bad_EmptyPath — empty path MUST return Fail
// with CodeAppendInvalidPath, mirroring AtomicAppendLine's contract.
func TestAtomicAppendLineLarge_Bad_EmptyPath(t *core.T) {
	homeFixture(t)
	r := paths.AtomicAppendLineLarge("", []byte("anything"))
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), paths.CodeAppendInvalidPath)
}
