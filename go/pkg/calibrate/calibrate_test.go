// SPDX-Licence-Identifier: EUPL-1.2

package calibrate

import "testing"

// discoverFixture is a trimmed-but-real `lthn-mlx discover --json`
// report captured from an Apple M3 Ultra (2026-05-28). Keeps the field
// shapes honest so the parser test breaks if the report contract drifts.
const discoverFixture = `{
  "runtime": {
    "backend": "metal",
    "device": "Apple M3 Ultra",
    "native_runtime": true,
    "labels": { "memory_bytes": "83494174720", "working_set_bytes": "83494174720" }
  },
  "device": {
    "name": "Apple M3 Ultra",
    "architecture": "Apple M3 Ultra",
    "max_buffer_length": 62620631040,
    "max_recommended_working_set_size": 83494174720,
    "memory_size": 83494174720,
    "labels": { "machine_hash": "sha256:f98df1475e6a65d7fb5821a9a424b855b4d951d462bcbc6c6ab954032b056aa7" }
  },
  "available": true,
  "capabilities": [
    { "id": "model.load", "group": "runtime", "status": "supported" },
    { "id": "runtime.autotune", "group": "runtime", "status": "supported" }
  ]
}`

// TestParseDiscoverReport_Good parses a real report and checks every
// tier (runtime / device / availability / capabilities) survives the
// round-trip.
func TestParseDiscoverReport_Good(t *testing.T) {
	r := ParseDiscoverReport(discoverFixture)
	if !r.OK {
		t.Fatalf("ParseDiscoverReport: want OK, got fail: %v", r.Error())
	}
	rep, ok := r.Value.(MachineReport)
	if !ok {
		t.Fatalf("ParseDiscoverReport: value is %T, want MachineReport", r.Value)
	}
	if rep.Runtime.Backend != "metal" {
		t.Errorf("Runtime.Backend = %q, want metal", rep.Runtime.Backend)
	}
	if !rep.Runtime.NativeRuntime {
		t.Errorf("Runtime.NativeRuntime = false, want true")
	}
	if rep.Device.MemorySize != 83494174720 {
		t.Errorf("Device.MemorySize = %d, want 83494174720", rep.Device.MemorySize)
	}
	if !rep.Available {
		t.Errorf("Available = false, want true")
	}
	if got := rep.Device.Labels["machine_hash"]; got == "" {
		t.Errorf("Device.Labels[machine_hash] missing — needed to key tuned profiles")
	}
	if !rep.Supports("runtime.autotune") {
		t.Errorf("Supports(runtime.autotune) = false, want true (gates the Calibrate offer)")
	}
	if rep.Supports("does.not.exist") {
		t.Errorf("Supports(does.not.exist) = true, want false")
	}
}

// TestParseDiscoverReport_Bad rejects an empty report (the binary
// produced no stdout — e.g. it failed before printing).
func TestParseDiscoverReport_Bad(t *testing.T) {
	if r := ParseDiscoverReport(""); r.OK {
		t.Fatalf("ParseDiscoverReport(\"\"): want fail, got OK")
	}
	if r := ParseDiscoverReport("   \n  "); r.OK {
		t.Fatalf("ParseDiscoverReport(whitespace): want fail, got OK")
	}
}

// TestParseDiscoverReport_Ugly rejects malformed JSON without panicking
// — a truncated / corrupted report must degrade to a Fail Result, never
// a crash (the whole point of the substrate hardening this session).
func TestParseDiscoverReport_Ugly(t *testing.T) {
	for _, junk := range []string{
		`{"runtime": {`,             // truncated
		`not json at all`,           // not JSON
		`[1,2,3]`,                   // wrong top-level type
	} {
		if r := ParseDiscoverReport(junk); r.OK {
			t.Errorf("ParseDiscoverReport(%q): want fail, got OK", junk)
		}
	}
}
