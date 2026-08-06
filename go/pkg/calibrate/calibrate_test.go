// SPDX-Licence-Identifier: EUPL-1.2

package calibrate

import (
	"os"
	"path/filepath"
	"testing"

	core "dappco.re/go"
	"dappco.re/go/process"
)

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
		`{"runtime": {`,   // truncated
		`not json at all`, // not JSON
		`[1,2,3]`,         // wrong top-level type
	} {
		if r := ParseDiscoverReport(junk); r.OK {
			t.Errorf("ParseDiscoverReport(%q): want fail, got OK", junk)
		}
	}
}

// --- Real tests for Service.Discover / resolveBinary / NewService /
// Register. The lthn-mlx CLI is never installed in a hermetic test
// environment, so it's exercised via a fake binary placed on PATH —
// real process.Service, real exec — with HOME repointed at a temp
// dir so resolveBinary's ~/Lethean/bin and ~/Code/core/go-mlx/bin
// candidate checks can never accidentally pick up a REAL lthn-mlx
// installed on the dev machine. Mirrors the house pattern in
// pkg/sandbox/runtime_dispatch_internal_test.go.

// writeFakeLthnMlx drops an executable shell stub named "lthn-mlx"
// into dir running `script` verbatim.
func writeFakeLthnMlx(t *testing.T, dir, script string) string {
	t.Helper()
	path := filepath.Join(dir, binaryName)
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake lthn-mlx: %v", err)
	}
	return path
}

// hermeticHome repoints HOME + clears the binary-override env var so
// resolveBinary's candidate checks land in an empty temp dir instead
// of any real ~/Lethean or ~/Code checkout on the host.
func hermeticHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv(envBinaryOverride, "")
	return home
}

// newCalibrateServiceWithProcess wires process.Service + calibrate.Service
// on a fresh Core inside a hermetic HOME sandbox. fakeBinDir (holding a
// fake "lthn-mlx") is PREPENDED to the real PATH — not a replacement —
// because the fake scripts' shebang line needs a real /bin/sh, and the
// script bodies need real `cat`/`sleep`/etc. "lthn-mlx" is not a real
// system utility name (confirmed: `which lthn-mlx` finds nothing on the
// dev machine), so prepending is safe: our fake always wins the lookup
// for that one name, while standard tools keep resolving normally. An
// empty fakeBinDir still gets a fresh EMPTY dir prepended (harmless,
// changes nothing) so resolveBinary()'s bare "lthn-mlx" fallback stays
// deterministically unresolvable for the "binary genuinely missing"
// cases.
func newCalibrateServiceWithProcess(t *testing.T, fakeBinDir string) *Service {
	t.Helper()
	hermeticHome(t)
	if fakeBinDir == "" {
		fakeBinDir = t.TempDir()
	}
	t.Setenv("PATH", fakeBinDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	c := core.New(core.WithName("process", process.NewService(process.Options{})))
	return NewService(c)
}

const discoverGoodScript = "#!/bin/sh\n" +
	"if [ \"$1\" = \"discover\" ] && [ \"$2\" = \"--json\" ]; then\n" +
	"cat <<'EOF'\n" +
	`{"runtime":{"backend":"metal","device":"Apple M3","native_runtime":true,"labels":{}},"device":{"name":"Apple M3","architecture":"Apple M3","max_buffer_length":1,"max_recommended_working_set_size":1,"memory_size":1,"labels":{}},"available":true,"capabilities":[{"id":"runtime.autotune","group":"runtime","status":"supported"}]}` + "\n" +
	"EOF\n" +
	"exit 0\n" +
	"fi\n" +
	"exit 1\n"

func TestCalibrate_Service_Discover_Good(t *testing.T) {
	dir := t.TempDir()
	writeFakeLthnMlx(t, dir, discoverGoodScript)
	svc := newCalibrateServiceWithProcess(t, dir)

	r := svc.Discover()
	if !r.OK {
		t.Fatalf("Discover: want OK, got fail: %v", r.Error())
	}
	rep, ok := r.Value.(MachineReport)
	if !ok {
		t.Fatalf("Discover value is %T, want MachineReport", r.Value)
	}
	if rep.Runtime.Backend != "metal" {
		t.Errorf("Runtime.Backend = %q, want metal", rep.Runtime.Backend)
	}
	if !rep.Supports("runtime.autotune") {
		t.Errorf("Supports(runtime.autotune) = false, want true")
	}
}

func TestCalibrate_Service_Discover_Bad_NilCore(t *testing.T) {
	svc := &Service{}
	r := svc.Discover()
	if r.OK {
		t.Fatal("Discover with nil core: want fail, got OK")
	}
	if !core.Contains(r.Error(), "core not bound") {
		t.Errorf("error = %q, want it to mention core not bound", r.Error())
	}
}

func TestCalibrate_Service_Discover_Bad_NoProcessService(t *testing.T) {
	hermeticHome(t)
	svc := NewService(core.New())
	r := svc.Discover()
	if r.OK {
		t.Fatal("Discover with no process service: want fail, got OK")
	}
	if !core.Contains(r.Error(), "process service unavailable") {
		t.Errorf("error = %q, want it to mention process service unavailable", r.Error())
	}
}

func TestCalibrate_Service_Discover_Ugly_CommandFails(t *testing.T) {
	dir := t.TempDir()
	writeFakeLthnMlx(t, dir, "#!/bin/sh\necho boom 1>&2\nexit 3\n")
	svc := newCalibrateServiceWithProcess(t, dir)

	r := svc.Discover()
	if r.OK {
		t.Fatal("Discover against a failing binary: want fail, got OK")
	}
}

func TestCalibrate_Service_Discover_Ugly_EmptyOutput(t *testing.T) {
	dir := t.TempDir()
	writeFakeLthnMlx(t, dir, "#!/bin/sh\nexit 0\n")
	svc := newCalibrateServiceWithProcess(t, dir)

	r := svc.Discover()
	if r.OK {
		t.Fatal("Discover with empty stdout: want fail, got OK")
	}
	if !core.Contains(r.Error(), "empty discover report") {
		t.Errorf("error = %q, want it to mention empty discover report", r.Error())
	}
}

func TestCalibrate_ResolveBinary_Good_EnvOverrideWins(t *testing.T) {
	hermeticHome(t)
	t.Setenv(envBinaryOverride, "/custom/path/lthn-mlx")
	if got := resolveBinary(); got != "/custom/path/lthn-mlx" {
		t.Errorf("resolveBinary() = %q, want the env override", got)
	}
}

func TestCalibrate_ResolveBinary_Good_LetheanBinCandidate(t *testing.T) {
	home := hermeticHome(t)
	dir := filepath.Join(home, "Lethean", "bin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	candidate := filepath.Join(dir, binaryName)
	if err := os.WriteFile(candidate, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write candidate: %v", err)
	}
	if got := resolveBinary(); got != candidate {
		t.Errorf("resolveBinary() = %q, want %q", got, candidate)
	}
}

func TestCalibrate_ResolveBinary_Bad_NoCandidatesFallsBackToBareName(t *testing.T) {
	hermeticHome(t)
	if got := resolveBinary(); got != binaryName {
		t.Errorf("resolveBinary() = %q, want the bare %q fallback", got, binaryName)
	}
}

func TestCalibrate_NewService_Good(t *testing.T) {
	svc := NewService(core.New())
	if svc == nil {
		t.Fatal("NewService = nil")
	}
}

func TestCalibrate_Register_Good(t *testing.T) {
	r := Register(core.New())
	if !r.OK {
		t.Fatalf("Register: want OK, got fail: %v", r.Error())
	}
	if _, ok := r.Value.(*Service); !ok {
		t.Fatalf("Register value is %T, want *Service", r.Value)
	}
}
