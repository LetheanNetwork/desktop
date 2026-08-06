// SPDX-Licence-Identifier: EUPL-1.2

// Benchmarks for the per-sample host telemetry collection + encode
// path (host.go). hostSampler.sample() runs on every Control /
// Telemetry poll for the app's whole life — it derives CPU/network
// rate deltas from the previous reading, then builds the renderer-safe
// HostSnapshot. The fixture source is a deterministic, monotonically
// increasing in-memory reading so every call — including the first —
// exercises the same steady-state code path a real running app sees
// after its first sample, without paying for the real cgo/syscall
// collection (that's benched separately, platform-conditionally, in
// BenchmarkDarwinHostSource_Read).
//
// Run:
//
//	go test ./pkg/telemetry/... -run '^$' -bench . -benchmem -benchtime=20x

package telemetry

import (
	"testing"
	"time"
)

// incrementingHostSource returns a fresh full hostReading (CPU, memory,
// network, power all populated) on every call, with monotonic counters
// advancing so cpuUsage/networkRates compute a real delta on every
// sample past the first — matching a live host under load rather than
// an idle one that keeps hitting the "no delta" branches.
type incrementingHostSource struct {
	n int
}

func (s *incrementingHostSource) read() (hostReading, error) {
	s.n++
	step := uint64(s.n)
	return hostReading{
		observedAt:   benchEpoch.Add(time.Duration(s.n) * time.Second),
		source:       "bench host API",
		platform:     "darwin",
		architecture: "arm64",
		logicalCores: 16,
		cpu: &cpuCounters{
			user:   100 * step,
			system: 40 * step,
			nice:   0,
			idle:   860 * step,
		},
		memory: &memoryReading{
			totalBytes: 32 * 1024 * 1024 * 1024,
			usedBytes:  (12 * 1024 * 1024 * 1024) + step,
		},
		network: &networkCounters{
			receivedBytes: 1_000_000 * step,
			sentBytes:     250_000 * step,
		},
		power: &powerReading{
			source:         PowerSourceAC,
			batteryPercent: float64Pointer(87.5),
			charging:       boolPointer(true),
		},
	}, nil
}

var benchEpoch = time.Date(2026, time.August, 6, 12, 0, 0, 0, time.UTC)

func BenchmarkHostSampler_Sample_SteadyState(b *testing.B) {
	sampler := newHostSampler(&incrementingHostSource{})
	// Warm the "previous reading" slot so every measured call takes the
	// full delta-computing path, matching steady-state polling rather
	// than the one-off cold-start sample.
	if _, err := sampler.sample(); err != nil {
		b.Fatalf("warm-up sample: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := sampler.sample(); err != nil {
			b.Fatalf("sample: %v", err)
		}
	}
}

func BenchmarkSample_ProcessReading(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r := Sample()
		if !r.OK {
			b.Fatalf("Sample: %s", r.Error())
		}
	}
}
