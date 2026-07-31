// SPDX-Licence-Identifier: EUPL-1.2

package telemetry

import (
	"errors"
	"testing"
	"time"
)

type sequenceHostSource struct {
	readings []hostReading
	err      error
	index    int
}

func (s *sequenceHostSource) read() (hostReading, error) {
	if s.err != nil {
		return hostReading{}, s.err
	}
	reading := s.readings[s.index]
	if s.index < len(s.readings)-1 {
		s.index++
	}
	return reading, nil
}

func TestHostSampler_GoodDerivesBoundedRatesFromMonotonicCounters(t *testing.T) {
	t0 := time.Date(2026, time.July, 31, 12, 0, 0, 0, time.UTC)
	sampler := newHostSampler(&sequenceHostSource{readings: []hostReading{
		{
			observedAt:   t0,
			source:       "test host API",
			logicalCores: 10,
			cpu:          &cpuCounters{user: 100, system: 50, idle: 850},
			memory:       &memoryReading{totalBytes: 1_000, usedBytes: 400},
			network:      &networkCounters{receivedBytes: 1_000, sentBytes: 2_000},
			power:        &powerReading{source: PowerSourceAC, batteryPercent: float64Pointer(80), charging: boolPointer(true)},
		},
		{
			observedAt:   t0.Add(5 * time.Second),
			source:       "test host API",
			logicalCores: 10,
			cpu:          &cpuCounters{user: 200, system: 100, idle: 1_200},
			memory:       &memoryReading{totalBytes: 1_000, usedBytes: 450},
			network:      &networkCounters{receivedBytes: 6_000, sentBytes: 4_500},
			power:        &powerReading{source: PowerSourceAC, batteryPercent: float64Pointer(81), charging: boolPointer(true)},
		},
	}})

	first, err := sampler.sample()
	if err != nil {
		t.Fatalf("first sample: %v", err)
	}
	if first.CPU.UsagePercent != nil {
		t.Fatalf("first CPU sample must not invent a delta: %v", *first.CPU.UsagePercent)
	}
	if first.Network == nil || first.Network.ReceivedBytesPerSecond != nil {
		t.Fatalf("first network sample must expose counters without an invented rate: %#v", first.Network)
	}

	second, err := sampler.sample()
	if err != nil {
		t.Fatalf("second sample: %v", err)
	}
	if second.CPU.UsagePercent == nil || *second.CPU.UsagePercent != 30 {
		t.Fatalf("CPU usage = %v, want 30", second.CPU.UsagePercent)
	}
	if second.Memory == nil || second.Memory.TotalBytes != 1_000 || second.Memory.UsedBytes != 450 {
		t.Fatalf("memory = %#v, want 450/1000", second.Memory)
	}
	if second.Network == nil || second.Network.ReceivedBytesPerSecond == nil || *second.Network.ReceivedBytesPerSecond != 1_000 {
		t.Fatalf("received rate = %#v, want 1000 B/s", second.Network)
	}
	if second.Network.SentBytesPerSecond == nil || *second.Network.SentBytesPerSecond != 500 {
		t.Fatalf("sent rate = %#v, want 500 B/s", second.Network.SentBytesPerSecond)
	}
	if second.Power == nil || second.Power.Source != PowerSourceAC || second.Power.BatteryPercent == nil || *second.Power.BatteryPercent != 81 {
		t.Fatalf("power = %#v, want AC at 81%%", second.Power)
	}
	if second.ObservedAt != "2026-07-31T12:00:05Z" || second.Source != "test host API" {
		t.Fatalf("provenance = %q at %q", second.Source, second.ObservedAt)
	}
}

func TestHostSampler_BadRejectsImpossibleHostReadings(t *testing.T) {
	tests := []struct {
		name    string
		reading hostReading
	}{
		{
			name: "missing observation time",
			reading: hostReading{
				source:       "test host API",
				logicalCores: 4,
			},
		},
		{
			name: "used memory exceeds total",
			reading: hostReading{
				observedAt:   time.Now(),
				source:       "test host API",
				logicalCores: 4,
				memory:       &memoryReading{totalBytes: 100, usedBytes: 101},
			},
		},
		{
			name: "battery exceeds one hundred percent",
			reading: hostReading{
				observedAt:   time.Now(),
				source:       "test host API",
				logicalCores: 4,
				power:        &powerReading{source: PowerSourceBattery, batteryPercent: float64Pointer(101)},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sampler := newHostSampler(&sequenceHostSource{readings: []hostReading{test.reading}})
			if _, err := sampler.sample(); err == nil {
				t.Fatal("sample succeeded for an impossible host reading")
			}
		})
	}
}

func TestHostSampler_UglyTreatsCounterResetAsUnavailableDelta(t *testing.T) {
	t0 := time.Date(2026, time.July, 31, 12, 0, 0, 0, time.UTC)
	sampler := newHostSampler(&sequenceHostSource{readings: []hostReading{
		{
			observedAt:   t0,
			source:       "test host API",
			logicalCores: 4,
			cpu:          &cpuCounters{user: 500, system: 200, idle: 300},
			network:      &networkCounters{receivedBytes: 5_000, sentBytes: 5_000},
		},
		{
			observedAt:   t0.Add(time.Second),
			source:       "test host API",
			logicalCores: 4,
			cpu:          &cpuCounters{user: 2, system: 1, idle: 7},
			network:      &networkCounters{receivedBytes: 20, sentBytes: 30},
		},
	}})

	if _, err := sampler.sample(); err != nil {
		t.Fatalf("first sample: %v", err)
	}
	second, err := sampler.sample()
	if err != nil {
		t.Fatalf("second sample: %v", err)
	}
	if second.CPU.UsagePercent != nil {
		t.Fatalf("CPU counter reset produced usage: %v", *second.CPU.UsagePercent)
	}
	if second.Network == nil || second.Network.ReceivedBytesPerSecond != nil || second.Network.SentBytesPerSecond != nil {
		t.Fatalf("network counter reset produced rates: %#v", second.Network)
	}
}

func TestHostSampler_BadPropagatesSourceFailure(t *testing.T) {
	sampler := newHostSampler(&sequenceHostSource{err: errors.New("host API unavailable")})
	if _, err := sampler.sample(); err == nil {
		t.Fatal("source failure was hidden")
	}
}

func float64Pointer(value float64) *float64 { return &value }

func boolPointer(value bool) *bool { return &value }
