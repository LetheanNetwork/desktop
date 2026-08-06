// SPDX-Licence-Identifier: EUPL-1.2

//go:build darwin && cgo

// Benchmark for the real macOS host-API collection darwinHostSource.read()
// wraps (host_source_darwin.go) — the cgo/syscall layer hostSampler.sample()
// sits on top of. Kept separate from BenchmarkHostSampler_Sample_SteadyState
// (host_bench_test.go), which uses a fixture source to isolate the pure-Go
// snapshot-building cost from this platform-call cost.

package telemetry

import "testing"

func BenchmarkDarwinHostSource_Read(b *testing.B) {
	source := darwinHostSource{}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := source.read(); err != nil {
			b.Fatalf("read: %v", err)
		}
	}
}
