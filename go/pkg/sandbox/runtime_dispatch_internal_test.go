// SPDX-Licence-Identifier: EUPL-1.2

// Internal tests for the runtime-dispatch seam in sandbox.go
// (spawnDispatch / spawnViaCLI / resolveRuntime) and the port
// helpers in sandbox_long.go (allocateFreePort / waitPortOpen /
// resolveRuntimeName). No real container daemon is available in any
// hermetic test environment (Docker Desktop's socket, Apple
// Container's apiserver — both require a running system daemon this
// suite must never depend on), so the CLI runtimes are exercised via
// a fake `docker` binary placed on PATH — real process.Service, real
// exec, real PATH resolution via dappco.re/go/container's
// proc.LookPath, only the container runtime itself is a stub.
//
// spawnApple is a deliberate leave-out: AppleProvider.Available()
// requires the real macOS 26+ `container` system service to be
// registered + running (`container system start`), which is a
// host-level daemon state change entirely inappropriate to flip from
// a test. Faking the full Apple Container CLI JSON protocol
// (Run/Wait/Container-ID parsing) for one proof-of-life dispatch arm
// is out of proportion — traced leave-out, not an oversight.

package sandbox

import (
	"net"
	"os"
	"path/filepath"

	core "dappco.re/go"
	"dappco.re/go/container"
)

// writeFakeRuntimeBinary drops an executable shell stub named `name`
// into dir that answers `--version` with a fixed line (so
// dappco.re/go/container's captureVersion doesn't error) and echoes
// its argv for any other invocation (so buildRunArgs's argv shape is
// observable in the captured stdout without a real container ever
// starting).
func writeFakeRuntimeBinary(t *core.T, dir, name string) {
	t.Helper()
	script := "#!/bin/sh\n" +
		"if [ \"$1\" = \"--version\" ]; then\n" +
		"  echo \"" + name + " version 0.0.0-fake\"\n" +
		"  exit 0\n" +
		"fi\n" +
		"echo \"fake-" + name + "-ran: $*\"\n" +
		"exit 0\n"
	path := filepath.Join(dir, name)
	core.RequireNoError(t, os.WriteFile(path, []byte(script), 0o755))
}

// withFakeRuntimeOnPath prepends dir (holding one or more fake
// runtime binaries) to PATH for the duration of the test.
func withFakeRuntimeOnPath(t *core.T, dir string) {
	t.Helper()
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// TestSandbox_ResolveRuntime_Good — an explicit `prefer` naming a
// runtime that container.HasRuntime finds on PATH resolves to that
// exact RuntimeType, independent of auto-detection priority.
func TestSandbox_ResolveRuntime_Good(t *core.T) {
	dir := t.TempDir()
	writeFakeRuntimeBinary(t, dir, "docker")
	withFakeRuntimeOnPath(t, dir)

	svc := newTestService(Options{})
	r := svc.resolveRuntime("docker")
	core.RequireTrue(t, r.OK)
	core.AssertEqual(t, container.RuntimeDocker, r.Value.(container.RuntimeType))
}

// TestSandbox_ResolveRuntime_Bad — a `prefer` naming a runtime type
// no detector will ever match fails with the typed "not available"
// message, deterministically regardless of the host's real container
// tooling.
func TestSandbox_ResolveRuntime_Bad(t *core.T) {
	svc := newTestService(Options{})
	r := svc.resolveRuntime("lthn-fake-runtime-does-not-exist")
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "requested runtime not available")
}

// TestSandbox_SpawnDispatch_Good — end-to-end through spawnDispatch:
// prepareSpawnInput -> resolveRuntime -> spawnViaCLI -> buildRunArgs
// -> process.Service.Run against the fake docker binary. Proves the
// whole one-shot CLI-runtime arm without a real daemon.
func TestSandbox_SpawnDispatch_Good(t *core.T) {
	dir := t.TempDir()
	writeFakeRuntimeBinary(t, dir, "docker")
	withFakeRuntimeOnPath(t, dir)

	svc := newTestServiceWithProcess()
	r := NewSpawnPort(svc).Spawn(SpawnInput{
		Image:   "alpine:3.21",
		Command: "echo",
		Args:    []string{"hi"},
		Runtime: "docker",
	})
	core.RequireTrue(t, r.OK)
	out := r.Value.(SpawnOutput)
	core.AssertEqual(t, "docker", out.Runtime)
	core.AssertEqual(t, "alpine:3.21", out.Image)
	core.AssertEqual(t, 0, out.ExitCode)
	core.AssertContains(t, out.Stdout, "fake-docker-ran")
}

// TestSandbox_SpawnDispatch_Bad — resolveRuntime's failure threads
// straight back through spawnDispatch/spawn without ever reaching
// process.Service.
func TestSandbox_SpawnDispatch_Bad(t *core.T) {
	svc := newTestServiceWithProcess()
	r := NewSpawnPort(svc).Spawn(SpawnInput{
		Image:   "alpine:3.21",
		Command: "echo",
		Runtime: "lthn-fake-runtime-does-not-exist",
	})
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "requested runtime not available")
}

// TestSandbox_SpawnViaCLI_Ugly — process.Service unavailable (no
// "process" service registered on the Core) fails with the explicit
// "process service unavailable" message rather than panicking, even
// though resolveRuntime itself succeeds.
func TestSandbox_SpawnViaCLI_Ugly(t *core.T) {
	dir := t.TempDir()
	writeFakeRuntimeBinary(t, dir, "docker")
	withFakeRuntimeOnPath(t, dir)

	svc := newTestService(Options{}) // no process service wired
	r := NewSpawnPort(svc).Spawn(SpawnInput{
		Image:   "alpine:3.21",
		Command: "echo",
		Runtime: "docker",
	})
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "process service unavailable")
}

// TestSandbox_ResolveRuntimeName_Good — an explicit prefer string
// passes straight through untouched (SpawnLong's looser dispatch —
// no HasRuntime validation, mirrors spawnViaCLI's binary-name shape).
func TestSandbox_ResolveRuntimeName_Good(t *core.T) {
	svc := newTestService(Options{})
	core.AssertEqual(t, "podman", svc.resolveRuntimeName("podman"))
}

// TestSandbox_ResolveRuntimeName_Ugly — an empty prefer falls back to
// resolveRuntime(""); when that fails (no runtime resolvable) the
// name still degrades to "docker" rather than propagating the error,
// since SpawnLong needs SOME binary name to attempt.
func TestSandbox_ResolveRuntimeName_Ugly(t *core.T) {
	svc := newTestService(Options{})
	name := svc.resolveRuntimeName("")
	core.AssertNotEmpty(t, name)
}

// TestSandbox_AllocateFreePort_Good — returns a bindable ephemeral
// port; a fresh listener on it succeeds (proves the earlier close
// actually released it back to the OS).
func TestSandbox_AllocateFreePort_Good(t *core.T) {
	r := allocateFreePort()
	core.RequireTrue(t, r.OK)
	port := r.Value.(int)
	core.AssertGreater(t, port, 0)

	l, err := net.Listen("tcp", core.Sprintf("127.0.0.1:%d", port))
	core.RequireNoError(t, err)
	_ = l.Close()
}

// TestSandbox_WaitPortOpen_Good — a real listener on the allocated
// port is detected as open well within a generous timeout.
func TestSandbox_WaitPortOpen_Good(t *core.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	core.RequireNoError(t, err)
	defer func() { _ = l.Close() }()
	port := l.Addr().(*net.TCPAddr).Port

	r := waitPortOpen(port, 2*core.Second)
	core.RequireTrue(t, r.OK)
}

// TestSandbox_WaitPortOpen_Bad — nothing listening on the port times
// out with a typed error naming the port + timeout, rather than
// hanging indefinitely.
func TestSandbox_WaitPortOpen_Bad(t *core.T) {
	freeR := allocateFreePort()
	core.RequireTrue(t, freeR.OK)
	port := freeR.Value.(int)

	r := waitPortOpen(port, 600*core.Millisecond)
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "did not open within")
}

// TestSandbox_SpawnLong_NoExposedPort_Good — ExposedPort==0 skips
// host-port allocation AND the readiness wait entirely (both would
// otherwise need a fake container that actually binds a port), so
// this drives spawnLong's full happy path — handle construction,
// handles-map registration, Succeeded audit emit — hermetically with
// the same fake docker binary used for the one-shot dispatch tests.
func TestSandbox_SpawnLong_NoExposedPort_Good(t *core.T) {
	dir := t.TempDir()
	writeFakeRuntimeBinary(t, dir, "docker")
	withFakeRuntimeOnPath(t, dir)

	svc := newTestServiceWithProcess()
	r := NewSpawnPort(svc).SpawnLong(SpawnLongInput{
		Image:   "alpine:3.21",
		Command: "sleep",
		Args:    []string{"3600"},
		Runtime: "docker",
		// ExposedPort deliberately 0.
	})
	core.RequireTrue(t, r.OK)
	handle := r.Value.(ContainerHandle)
	core.AssertEqual(t, StatusReady, handle.Status)
	core.AssertEqual(t, 0, handle.HostPort)
	core.AssertNotEmpty(t, handle.SandboxID)

	// Registered in the live handles map — GetHandle sees it.
	getR := NewSpawnPort(svc).GetHandle(handle.SandboxID)
	core.RequireTrue(t, getR.OK)
}

// TestSandbox_InspectImage_ManifestDigest_Good — a real process.Service
// round-trip through a fake `docker manifest inspect` that returns a
// digest-bearing JSON payload.
func TestSandbox_InspectImage_ManifestDigest_Good(t *core.T) {
	dir := t.TempDir()
	script := "#!/bin/sh\n" +
		"if [ \"$1\" = \"manifest\" ] && [ \"$2\" = \"inspect\" ]; then\n" +
		"  echo '{\"digest\":\"sha256:deadbeef\"}'\n" +
		"  exit 0\n" +
		"fi\n" +
		"exit 1\n"
	core.RequireNoError(t, os.WriteFile(filepath.Join(dir, "docker"), []byte(script), 0o755))
	withFakeRuntimeOnPath(t, dir)

	svc := newTestServiceWithProcess()
	digest, r := svc.InspectImage("alpine:3.21")
	core.RequireTrue(t, r.OK)
	core.AssertEqual(t, "sha256:deadbeef", digest)
}

// TestSandbox_InspectImage_ManifestDigest_Ugly — a well-formed JSON
// reply with no `.digest` field surfaces the typed digest-parse-
// failed code rather than an empty-but-OK Result.
func TestSandbox_InspectImage_ManifestDigest_Ugly(t *core.T) {
	dir := t.TempDir()
	script := "#!/bin/sh\n" +
		"echo '{\"schemaVersion\":2}'\n" +
		"exit 0\n"
	core.RequireNoError(t, os.WriteFile(filepath.Join(dir, "docker"), []byte(script), 0o755))
	withFakeRuntimeOnPath(t, dir)

	svc := newTestServiceWithProcess()
	digest, r := svc.InspectImage("alpine:3.21")
	core.AssertFalse(t, r.OK)
	core.AssertEqual(t, "", digest)
	core.AssertContains(t, r.Error(), codeDigestParseFailed)
}

// TestSandbox_InspectImage_ManifestDigest_Bad — a non-zero-exit
// `docker manifest inspect` (auth failure, no such tag) surfaces the
// typed inspect-failed code.
func TestSandbox_InspectImage_ManifestDigest_Bad(t *core.T) {
	dir := t.TempDir()
	script := "#!/bin/sh\necho 'no such manifest' >&2\nexit 1\n"
	core.RequireNoError(t, os.WriteFile(filepath.Join(dir, "docker"), []byte(script), 0o755))
	withFakeRuntimeOnPath(t, dir)

	svc := newTestServiceWithProcess()
	digest, r := svc.InspectImage("alpine:3.21")
	core.AssertFalse(t, r.OK)
	core.AssertEqual(t, "", digest)
	core.AssertContains(t, r.Error(), codeInspectFailed)
}
