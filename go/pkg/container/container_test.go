// SPDX-Licence-Identifier: EUPL-1.2

// Real tests for container.go's unexported detection surface
// (proc/quickVersion/probeRuntime/detectAll/shortID) plus the
// exported constructors. The pre-existing container_example_test.go
// only takes method VALUES via reflection and never invokes them —
// that's why this package sat at 0.0% coverage. No real container
// daemon is available in any hermetic test environment, so runtimes
// are exercised via fake `docker` / `podman` binaries placed on
// PATH — real process.Service, real exec, real PATH resolution via
// core.App{}.Find, only the runtime binary itself is a stub. Mirrors
// the house pattern in pkg/sandbox/runtime_dispatch_internal_test.go.

package container

import (
	"os"
	"path/filepath"
	"testing"

	core "dappco.re/go"
	"dappco.re/go/process"
)

// writeFakeBinary drops an executable shell stub named `name` into
// dir running `script` verbatim.
func writeFakeBinary(t *testing.T, dir, name, script string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake %s: %v", name, err)
	}
}

const fakeDockerScript = "#!/bin/sh\n" +
	"case \"$1\" in\n" +
	"  --version) echo \"Docker version 24.0.7, build afdd53b\"; exit 0 ;;\n" +
	"  ps) echo '{\"ID\":\"abc123456789extra\",\"Names\":\"web1\",\"Image\":\"nginx:latest\",\"Status\":\"Up 2 minutes\",\"CreatedAt\":\"2026-01-01 00:00:00\"}'; exit 0 ;;\n" +
	"  logs) echo \"log line 1\"; echo \"log line 2\"; exit 0 ;;\n" +
	"  *) exit 1 ;;\n" +
	"esac\n"

const fakePodmanScript = "#!/bin/sh\n" +
	"case \"$1\" in\n" +
	"  --version) echo \"podman version 4.7.0\"; exit 0 ;;\n" +
	"  ps) echo '[{\"Id\":\"deadbeef1234extra\",\"Names\":[\"pod1\"],\"Image\":\"alpine:3.21\",\"State\":\"running\"}]'; exit 0 ;;\n" +
	"  logs) echo \"podman log line\"; exit 0 ;;\n" +
	"  *) exit 1 ;;\n" +
	"esac\n"

// withFakeRuntimesOnPath writes docker+podman fakes into a fresh dir
// and REPLACES PATH with just that dir for the duration of the test
// (not prepend — this dev machine genuinely has linuxkit,
// qemu-system-x86_64, and Apple's native `container` installed via
// Homebrew/macOS 15.4+, so appending to the real PATH would make the
// "these runtimes are NOT available" assertions host-dependent
// instead of hermetic).
func withFakeRuntimesOnPath(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	writeFakeBinary(t, dir, "docker", fakeDockerScript)
	writeFakeBinary(t, dir, "podman", fakePodmanScript)
	t.Setenv("PATH", dir)
}

// newServiceWithProcess wires process.Service alongside container.Service
// so detection/listing can actually shell out — needed for every test
// that reaches proc.Run.
func newServiceWithProcess(t *testing.T) *Service {
	t.Helper()
	c := core.New(core.WithName("process", process.NewService(process.Options{})))
	return NewService(c)
}

func TestContainer_ShortID_Good_TruncatesLongID(t *testing.T) {
	got := shortID("abcdefghijklmnopqrstuvwxyz", 12)
	if got != "abcdefghijkl" {
		t.Errorf("shortID = %q, want 12-char prefix", got)
	}
}

func TestContainer_ShortID_Bad_ShortStringUnchanged(t *testing.T) {
	got := shortID("short", 12)
	if got != "short" {
		t.Errorf("shortID(short) = %q, want unchanged", got)
	}
}

func TestContainer_ShortID_Ugly_ExactBoundaryUnchanged(t *testing.T) {
	id := "abcdefghijkl" // exactly 12
	if got := shortID(id, 12); got != id {
		t.Errorf("shortID(exact-len) = %q, want unchanged %q", got, id)
	}
}

func TestContainer_Proc_Good_ResolvesWiredService(t *testing.T) {
	svc := newServiceWithProcess(t)
	if svc.proc() == nil {
		t.Fatal("proc() = nil, want a resolved process.Service")
	}
}

func TestContainer_Proc_Bad_NilCore(t *testing.T) {
	svc := &Service{}
	if svc.proc() != nil {
		t.Error("proc() with nil core = non-nil, want nil")
	}
}

func TestContainer_Proc_Ugly_NilReceiver(t *testing.T) {
	var svc *Service
	if svc.proc() != nil {
		t.Error("proc() on nil *Service = non-nil, want nil")
	}
}

func TestContainer_QuickVersion_Good_ParsesFirstLine(t *testing.T) {
	withFakeRuntimesOnPath(t)
	svc := newServiceWithProcess(t)
	got := svc.quickVersion("docker", "--version")
	if got != "Docker version 24.0.7, build afdd53b" {
		t.Errorf("quickVersion = %q, want the fake docker banner", got)
	}
}

func TestContainer_QuickVersion_Bad_NoProcessService(t *testing.T) {
	svc := &Service{}
	if got := svc.quickVersion("docker", "--version"); got != "" {
		t.Errorf("quickVersion with no process service = %q, want empty", got)
	}
}

func TestContainer_QuickVersion_Ugly_CommandFails(t *testing.T) {
	svc := newServiceWithProcess(t)
	got := svc.quickVersion("lthn-nonexistent-binary-xyz", "--version")
	if got != "" {
		t.Errorf("quickVersion(missing binary) = %q, want empty", got)
	}
}

func TestContainer_ProbeRuntime_Good_FoundOnPath(t *testing.T) {
	withFakeRuntimesOnPath(t)
	svc := newServiceWithProcess(t)
	info := svc.probeRuntime("docker", "docker", "--version", Runtime{Name: "docker"})
	if !info.Available {
		t.Fatal("probeRuntime: Available = false, want true")
	}
	if info.Version != "Docker version 24.0.7, build afdd53b" {
		t.Errorf("Version = %q, want the fake banner", info.Version)
	}
	if info.Path == "" {
		t.Error("Path is empty, want the resolved fake binary path")
	}
}

func TestContainer_ProbeRuntime_Bad_NotFound(t *testing.T) {
	svc := newServiceWithProcess(t)
	info := svc.probeRuntime("lthn-fake-runtime-xyz", "fake", "--version", Runtime{Name: "fake"})
	if info.Available {
		t.Error("probeRuntime: Available = true for a binary that isn't on PATH")
	}
	if info.Version != "" || info.Path != "" {
		t.Errorf("Version/Path = %q/%q, want both empty when not found", info.Version, info.Path)
	}
}

func TestContainer_DetectAll_Good_MixOfFoundAndMissing(t *testing.T) {
	withFakeRuntimesOnPath(t)
	svc := newServiceWithProcess(t)
	runtimes := svc.detectAll()
	if len(runtimes) != 5 {
		t.Fatalf("detectAll() returned %d runtimes, want 5", len(runtimes))
	}
	byName := map[string]Runtime{}
	for _, r := range runtimes {
		byName[r.Name] = r
	}
	if !byName["docker"].Available {
		t.Error("docker should be Available (fake on PATH)")
	}
	if !byName["podman"].Available {
		t.Error("podman should be Available (fake on PATH)")
	}
	if byName["linuxkit"].Available {
		t.Error("linuxkit should NOT be Available (no fake on PATH)")
	}
	if byName["qemu"].Available {
		t.Error("qemu should NOT be Available (no fake on PATH)")
	}
	if byName["apple"].Available {
		t.Error("apple should NOT be Available (no fake on PATH)")
	}
}

func TestContainer_NewService_Good_ConstructsAgainstCore(t *testing.T) {
	c := core.New()
	svc := NewService(c)
	if svc == nil {
		t.Fatal("NewService = nil")
	}
}

func TestContainer_NewService_Bad_NilCoreStillConstructs(t *testing.T) {
	svc := NewService(nil)
	if svc == nil {
		t.Fatal("NewService(nil) = nil")
	}
	if svc.proc() != nil {
		t.Error("proc() on a nil-core Service should be nil")
	}
}

func TestContainer_Register_Good_ReturnsOKService(t *testing.T) {
	r := Register(core.New())
	if !r.OK {
		t.Fatalf("Register: want OK, got fail: %v", r.Error())
	}
	if _, ok := r.Value.(*Service); !ok {
		t.Fatalf("Register value is %T, want *Service", r.Value)
	}
}
