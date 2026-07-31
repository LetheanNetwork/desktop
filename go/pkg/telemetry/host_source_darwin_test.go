// SPDX-Licence-Identifier: EUPL-1.2

//go:build darwin && cgo

package telemetry

import "testing"

func TestDarwinHostSource_GoodReadsNativeHostCounters(t *testing.T) {
	reading, err := (darwinHostSource{}).read()
	if err != nil {
		t.Fatalf("read Darwin host APIs: %v", err)
	}
	if reading.cpu == nil || reading.cpu.user+reading.cpu.system+reading.cpu.nice+reading.cpu.idle == 0 {
		t.Fatalf("CPU counters are unavailable: %#v", reading.cpu)
	}
	if reading.memory == nil || reading.memory.totalBytes == 0 || reading.memory.usedBytes > reading.memory.totalBytes {
		t.Fatalf("physical memory is invalid: %#v", reading.memory)
	}
	if reading.network == nil {
		t.Fatal("network counters are unavailable")
	}
	if reading.power != nil {
		switch reading.power.source {
		case PowerSourceUnknown, PowerSourceAC, PowerSourceBattery:
		default:
			t.Fatalf("power source is unbounded: %q", reading.power.source)
		}
	}
}
