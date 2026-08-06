// SPDX-License-Identifier: EUPL-1.2

// bench_test.go — read-lane instrument for pkg/office/files (perf
// campaign, lane/perf-c). Targets ListDirectory — the walk every
// Files pane re-issues on open/navigate — at a realistic 100-entry
// directory, backed by a REAL disk-backed coreio.Sandboxed medium (not
// the in-memory test double) so entry.Info() pays the genuine syscall
// cost production hits. Hermetic via b.TempDir, -benchmem,
// steady-state confirmed over -benchtime=20x.
//
// Two page sizes are benched against the SAME 100-entry directory:
// a full page (limit >= total, everything returned) and a small page
// (limit << total). listDirectory calls entry.Info() for EVERY entry
// before pagination slices the result — if the small-page benchmark
// doesn't cost meaningfully less than the full-page one, that's FLAT
// evidence the Info() walk is unconditional (a stat-then-discard
// pattern) rather than page-bounded.
package files

import (
	"testing"

	core "dappco.re/go"
	coreio "dappco.re/go/io"
)

// seedSandboxedFiles creates n small files (+ a handful of
// subdirectories, mirroring a realistic mixed listing) under root via
// a real disk-backed medium, then returns a registered Service whose
// sole mount points at that directory.
func seedSandboxedFiles(b *testing.B, n int) *Service {
	b.Helper()
	root := b.TempDir()
	medium, err := coreio.NewSandboxed(root)
	if err != nil {
		b.Fatalf("NewSandboxed: %v", err)
	}
	for i := 0; i < n; i++ {
		name := core.Sprintf("file-%04d.txt", i)
		if err := medium.Write(name, "bench fixture content"); err != nil {
			b.Fatalf("Write[%d]: %v", i, err)
		}
	}
	// A few directories mixed in — realistic listings aren't flat.
	for i := 0; i < 5; i++ {
		if err := medium.EnsureDir(core.Sprintf("folder-%02d", i)); err != nil {
			b.Fatalf("EnsureDir[%d]: %v", i, err)
		}
	}

	service := NewService(Options{
		Mounts:  []Mount{memoryMount("bench", medium, ReadWriteCapabilities())},
		Runtime: &stubRuntimeMetadata{},
	})
	if r := service.Register(core.New()); !r.OK {
		b.Fatalf("Register: %s", r.Error())
	}
	return service
}

// BenchmarkListDirectory_100_FullPage — limit covers the whole
// directory (105 entries: 100 files + 5 dirs), the common case for a
// small/medium folder.
func BenchmarkListDirectory_100_FullPage(b *testing.B) {
	service := seedSandboxedFiles(b, 100)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result := service.ListDirectory(ListDirectoryInput{MountID: "bench", Limit: 200})
		if !result.OK {
			b.Fatalf("ListDirectory: %s", result.Error())
		}
		snapshot := result.Value.(DirectorySnapshot)
		if snapshot.TotalKnown != 105 {
			b.Fatalf("TotalKnown: got %d want 105", snapshot.TotalKnown)
		}
	}
}

// BenchmarkListDirectory_100_SmallPage — same 105-entry directory, but
// the caller only wants the first 10 rows (the realistic GUI-pane
// case: a viewport shows a handful of rows, not the whole folder).
// listDirectory calls entry.Info() for every one of the 105 entries
// regardless of Limit — if this benchmark doesn't come in meaningfully
// cheaper than the full-page one above, that's the stat-then-discard
// waste confirmed with FLAT-comparable evidence.
func BenchmarkListDirectory_100_SmallPage(b *testing.B) {
	service := seedSandboxedFiles(b, 100)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result := service.ListDirectory(ListDirectoryInput{MountID: "bench", Limit: 10})
		if !result.OK {
			b.Fatalf("ListDirectory: %s", result.Error())
		}
		snapshot := result.Value.(DirectorySnapshot)
		if len(snapshot.Entries) != 10 {
			b.Fatalf("Entries: got %d want 10", len(snapshot.Entries))
		}
	}
}
