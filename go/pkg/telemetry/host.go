// SPDX-Licence-Identifier: EUPL-1.2

package telemetry

import (
	"math"
	"sync"
	"time"

	core "dappco.re/go"
)

// PowerSource is the bounded host power-source vocabulary exposed to clients.
type PowerSource string

const (
	PowerSourceUnknown PowerSource = "unknown"
	PowerSourceAC      PowerSource = "ac"
	PowerSourceBattery PowerSource = "battery"
)

// HostCPU is the host-wide CPU portion of a system-monitor snapshot.
type HostCPU struct {
	LogicalCores int      `json:"logical_cores"`
	UsagePercent *float64 `json:"usage_percent,omitempty"`
}

// HostMemory is host physical-memory usage in bytes.
type HostMemory struct {
	TotalBytes uint64 `json:"total_bytes"`
	UsedBytes  uint64 `json:"used_bytes"`
}

// HostNetwork contains aggregate non-loopback interface counters and rates.
type HostNetwork struct {
	ReceivedBytes          uint64   `json:"received_bytes"`
	SentBytes              uint64   `json:"sent_bytes"`
	ReceivedBytesPerSecond *float64 `json:"received_bytes_per_second,omitempty"`
	SentBytesPerSecond     *float64 `json:"sent_bytes_per_second,omitempty"`
}

// HostPower contains optional battery state and the current power source.
type HostPower struct {
	Source         PowerSource `json:"source"`
	BatteryPercent *float64    `json:"battery_percent,omitempty"`
	Charging       *bool       `json:"charging,omitempty"`
}

// HostSnapshot is one immutable, renderer-safe host system reading.
// Unsupported platform metrics are absent rather than represented by zero.
type HostSnapshot struct {
	ObservedAt   string       `json:"observed_at"`
	Source       string       `json:"source"`
	Platform     string       `json:"platform"`
	Architecture string       `json:"architecture"`
	CPU          HostCPU      `json:"cpu"`
	Memory       *HostMemory  `json:"memory,omitempty"`
	Network      *HostNetwork `json:"network,omitempty"`
	Power        *HostPower   `json:"power,omitempty"`
}

type cpuCounters struct {
	user   uint64
	system uint64
	nice   uint64
	idle   uint64
}

type memoryReading struct {
	totalBytes uint64
	usedBytes  uint64
}

type networkCounters struct {
	receivedBytes uint64
	sentBytes     uint64
}

type powerReading struct {
	source         PowerSource
	batteryPercent *float64
	charging       *bool
}

type hostReading struct {
	observedAt   time.Time
	source       string
	platform     string
	architecture string
	logicalCores int
	cpu          *cpuCounters
	memory       *memoryReading
	network      *networkCounters
	power        *powerReading
}

type hostSource interface {
	read() (hostReading, error)
}

type hostSampler struct {
	mu       sync.Mutex
	source   hostSource
	previous *hostReading
}

func newHostSampler(source hostSource) *hostSampler {
	return &hostSampler{source: source}
}

func (s *hostSampler) sample() (HostSnapshot, error) {
	if s == nil || s.source == nil {
		return HostSnapshot{}, core.NewError("telemetry: host source is unavailable")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	reading, err := s.source.read()
	if err != nil {
		return HostSnapshot{}, err
	}
	if err := validateHostReading(reading); err != nil {
		return HostSnapshot{}, err
	}

	snapshot := HostSnapshot{
		ObservedAt:   reading.observedAt.UTC().Format(time.RFC3339Nano),
		Source:       reading.source,
		Platform:     reading.platform,
		Architecture: reading.architecture,
		CPU: HostCPU{
			LogicalCores: reading.logicalCores,
		},
	}
	if reading.memory != nil {
		snapshot.Memory = &HostMemory{
			TotalBytes: reading.memory.totalBytes,
			UsedBytes:  reading.memory.usedBytes,
		}
	}
	if reading.network != nil {
		snapshot.Network = &HostNetwork{
			ReceivedBytes: reading.network.receivedBytes,
			SentBytes:     reading.network.sentBytes,
		}
	}
	if reading.power != nil {
		snapshot.Power = &HostPower{
			Source:         reading.power.source,
			BatteryPercent: cloneFloat64(reading.power.batteryPercent),
			Charging:       cloneBool(reading.power.charging),
		}
	}

	if s.previous != nil {
		if usage, ok := cpuUsage(s.previous.cpu, reading.cpu); ok {
			snapshot.CPU.UsagePercent = &usage
		}
		elapsed := reading.observedAt.Sub(s.previous.observedAt).Seconds()
		if snapshot.Network != nil && elapsed > 0 {
			if received, sent, ok := networkRates(s.previous.network, reading.network, elapsed); ok {
				snapshot.Network.ReceivedBytesPerSecond = &received
				snapshot.Network.SentBytesPerSecond = &sent
			}
		}
	}

	readingCopy := reading
	s.previous = &readingCopy
	return snapshot, nil
}

func validateHostReading(reading hostReading) error {
	if reading.observedAt.IsZero() {
		return core.NewError("telemetry: host observation time is unavailable")
	}
	if reading.source == "" || len(reading.source) > 128 {
		return core.NewError("telemetry: host source is invalid")
	}
	if reading.logicalCores < 1 || reading.logicalCores > 4_096 {
		return core.NewError("telemetry: logical CPU count is invalid")
	}
	if reading.memory != nil &&
		(reading.memory.totalBytes == 0 || reading.memory.usedBytes > reading.memory.totalBytes) {
		return core.NewError("telemetry: physical memory reading is invalid")
	}
	if reading.power != nil {
		switch reading.power.source {
		case PowerSourceUnknown, PowerSourceAC, PowerSourceBattery:
		default:
			return core.NewError("telemetry: power source is invalid")
		}
		if reading.power.batteryPercent != nil &&
			(math.IsNaN(*reading.power.batteryPercent) ||
				math.IsInf(*reading.power.batteryPercent, 0) ||
				*reading.power.batteryPercent < 0 ||
				*reading.power.batteryPercent > 100) {
			return core.NewError("telemetry: battery percentage is invalid")
		}
	}
	return nil
}

func cpuUsage(previous, current *cpuCounters) (float64, bool) {
	if previous == nil || current == nil ||
		current.user < previous.user ||
		current.system < previous.system ||
		current.nice < previous.nice ||
		current.idle < previous.idle {
		return 0, false
	}
	busy := (current.user - previous.user) +
		(current.system - previous.system) +
		(current.nice - previous.nice)
	idle := current.idle - previous.idle
	total := busy + idle
	if total == 0 {
		return 0, false
	}
	usage := float64(busy) / float64(total) * 100
	return min(100, max(0, usage)), true
}

func networkRates(
	previous,
	current *networkCounters,
	elapsedSeconds float64,
) (float64, float64, bool) {
	if previous == nil || current == nil || elapsedSeconds <= 0 ||
		current.receivedBytes < previous.receivedBytes ||
		current.sentBytes < previous.sentBytes {
		return 0, 0, false
	}
	received := float64(current.receivedBytes-previous.receivedBytes) / elapsedSeconds
	sent := float64(current.sentBytes-previous.sentBytes) / elapsedSeconds
	if math.IsNaN(received) || math.IsInf(received, 0) ||
		math.IsNaN(sent) || math.IsInf(sent, 0) {
		return 0, 0, false
	}
	return received, sent, true
}

func cloneFloat64(value *float64) *float64 {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

func cloneBool(value *bool) *bool {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}
