// SPDX-Licence-Identifier: EUPL-1.2

// Internal tests for crew.go — package fleet (not fleet_test) because the
// symbols under test (defaultCrew, resolveCrewBinary, spawnMember, launch,
// superviseCrew, superviseInstance, crewHealthy, crewQuickUp, crewProcID,
// crewWait, crewKill, crewSupervisor.stop) are all unexported.
//
// Hermetic rules observed throughout this file:
//   - Every spawn attempt targets crewFakeBinary, a name that cannot exist
//     on any real PATH, so a spawn that "succeeds" would be a test bug.
//   - This test binary never calls process.Init/SetDefault, so
//     process.Default() is nil and process.Start/StartWithOptions always
//     fail closed with ErrServiceNotInitialized — a second, independent
//     guarantee that no real child process is ever forked from this file,
//     even if a stray binary matching the fake name somehow existed.
//   - crewHealthDialTimeout / crewHealthDeadline / crewHealthPollInterval /
//     crewBackoffMin / crewBackoffMax are the seam vars declared in crew.go
//     for exactly this purpose; every test that touches them saves +
//     restores the originals so other tests keep the production defaults.
package fleet

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	core "dappco.re/go"
	process "dappco.re/go/process"
)

// crewFakeBinary can never resolve on any real PATH — every spawn attempt
// in this file uses it (or a per-test variant) so a "successful" spawn
// would itself be the test failing loudly, not a silent false-positive.
const crewFakeBinary = "lthn-desktop-test-fake-binary-does-not-exist-zzz"

// freeLoopbackPort reserves an ephemeral loopback port, then releases it so
// the caller can point a crewMember/addr at a port that is (almost
// certainly) free. Small TOCTOU race is inherent to this pattern and is the
// same one httptest.NewServer callers accept.
func freeLoopbackPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve loopback port: %v", err)
	}
	addr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		ln.Close()
		t.Fatalf("unexpected Addr type %T", ln.Addr())
	}
	ln.Close()
	return addr.Port
}

// shrinkHealthTimings shrinks the crewHealthy seam vars for the duration of
// the calling test and restores the production defaults on cleanup.
func shrinkHealthTimings(t *testing.T, deadline, dial, poll time.Duration) {
	t.Helper()
	origDeadline, origDial, origPoll := crewHealthDeadline, crewHealthDialTimeout, crewHealthPollInterval
	crewHealthDeadline, crewHealthDialTimeout, crewHealthPollInterval = deadline, dial, poll
	t.Cleanup(func() {
		crewHealthDeadline, crewHealthDialTimeout, crewHealthPollInterval = origDeadline, origDial, origPoll
	})
}

// shrinkBackoff shrinks the respawn-backoff seam vars for the duration of
// the calling test and restores the production defaults on cleanup.
func shrinkBackoff(t *testing.T, min, max time.Duration) {
	t.Helper()
	origMin, origMax := crewBackoffMin, crewBackoffMax
	crewBackoffMin, crewBackoffMax = min, max
	t.Cleanup(func() { crewBackoffMin, crewBackoffMax = origMin, origMax })
}

// --- defaultCrew ---

func TestCrew_DefaultCrew_Good(t *testing.T) {
	crew := defaultCrew([]string{"MCP_JWT_SECRET=abc", "MCP_AUTH_TOKEN=xyz"})
	if len(crew) != 2 {
		t.Fatalf("defaultCrew len = %d, want 2", len(crew))
	}
	if crew[0].Capability != CapabilityInference || crew[0].Binary != "lthn-ai" {
		t.Fatalf("crew[0] = %+v, want inference/lthn-ai", crew[0])
	}
	if crew[0].Watch {
		t.Fatalf("crew[0].Watch = true, want false (lthn-ai is process-backed)")
	}
	if crew[1].Capability != CapabilitySandbox || crew[1].Binary != "lthn-agent" {
		t.Fatalf("crew[1] = %+v, want sandbox/lthn-agent", crew[1])
	}
	if !crew[1].Watch {
		t.Fatalf("crew[1].Watch = false, want true (sandbox hub is PTY-backed)")
	}
}

func TestCrew_DefaultCrew_Bad_EmptySandboxEnvStillShapesCorrectly(t *testing.T) {
	crew := defaultCrew(nil)
	if len(crew) != 2 {
		t.Fatalf("defaultCrew(nil) len = %d, want 2", len(crew))
	}
	if crew[1].BasePort != 9201 {
		t.Fatalf("crew[1].BasePort = %d, want 9201", crew[1].BasePort)
	}
}

func TestCrew_DefaultCrew_Ugly_SandboxEnvThreadedThrough(t *testing.T) {
	crew := defaultCrew([]string{"MCP_JWT_SECRET=abc", "MCP_AUTH_TOKEN=xyz"})
	found := map[string]bool{}
	for _, e := range crew[1].Env {
		found[e] = true
	}
	if !found["MCP_JWT_SECRET=abc"] || !found["MCP_AUTH_TOKEN=xyz"] {
		t.Fatalf("sandboxEnv not threaded into crew[1].Env: %v", crew[1].Env)
	}
}

// --- resolveCrewBinary ---

func TestCrew_ResolveCrewBinary_Good(t *testing.T) {
	dir := t.TempDir()
	binPath := filepath.Join(dir, "widget")
	if err := os.WriteFile(binPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("seed fake binary: %v", err)
	}
	t.Setenv(crewBinDirEnv, dir)
	got := resolveCrewBinary("widget")
	if got != binPath {
		t.Fatalf("resolveCrewBinary = %q, want override dir path %q", got, binPath)
	}
}

func TestCrew_ResolveCrewBinary_Bad_FallsBackToBareName(t *testing.T) {
	t.Setenv(crewBinDirEnv, "")
	t.Setenv("HOME", t.TempDir())
	got := resolveCrewBinary("widget-not-anywhere")
	if got != "widget-not-anywhere" {
		t.Fatalf("resolveCrewBinary = %q, want bare-name fallback", got)
	}
}

func TestCrew_ResolveCrewBinary_Ugly_HomeBinDirSecondTier(t *testing.T) {
	t.Setenv(crewBinDirEnv, "")
	home := t.TempDir()
	binDir := filepath.Join(home, "Lethean", "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	binPath := filepath.Join(binDir, "widget")
	if err := os.WriteFile(binPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("seed fake binary: %v", err)
	}
	t.Setenv("HOME", home)
	got := resolveCrewBinary("widget")
	if got != binPath {
		t.Fatalf("resolveCrewBinary = %q, want ~/Lethean/bin path %q", got, binPath)
	}
}

// --- crewProcID ---

func TestCrew_CrewProcID_Good_ProcessPointer(t *testing.T) {
	r := core.Ok(&process.Process{ID: "proc-abc"})
	if got := crewProcID(r); got != "proc-abc" {
		t.Fatalf("crewProcID(*Process) = %q, want proc-abc", got)
	}
}

func TestCrew_CrewProcID_Bad_StringValue(t *testing.T) {
	r := core.Ok("string-id-xyz")
	if got := crewProcID(r); got != "string-id-xyz" {
		t.Fatalf("crewProcID(string) = %q, want string-id-xyz", got)
	}
}

func TestCrew_CrewProcID_Ugly_UnexpectedType(t *testing.T) {
	r := core.Ok(42)
	if got := crewProcID(r); got != "" {
		t.Fatalf("crewProcID(unexpected type) = %q, want empty string", got)
	}
}

// --- crewQuickUp ---

func TestCrew_CrewQuickUp_Good_ListenerUp(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	if !crewQuickUp(ln.Addr().String()) {
		t.Fatalf("crewQuickUp(%s) = false, want true (listener is up)", ln.Addr())
	}
}

func TestCrew_CrewQuickUp_Bad_NothingListening(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	ln.Close()
	if crewQuickUp(addr) {
		t.Fatalf("crewQuickUp(%s) = true, want false (nothing listening)", addr)
	}
}

// --- crewHealthy ---

func TestCrew_CrewHealthy_Good_AlreadyUp(t *testing.T) {
	shrinkHealthTimings(t, 500*time.Millisecond, 50*time.Millisecond, 20*time.Millisecond)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	if !crewHealthy(ln.Addr().String()) {
		t.Fatalf("crewHealthy(%s) = false, want true", ln.Addr())
	}
}

func TestCrew_CrewHealthy_Bad_DeadlineLapses(t *testing.T) {
	shrinkHealthTimings(t, 80*time.Millisecond, 15*time.Millisecond, 15*time.Millisecond)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	ln.Close()
	start := time.Now()
	if crewHealthy(addr) {
		t.Fatalf("crewHealthy(%s) = true, want false (nothing listening)", addr)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("crewHealthy took %v — the shrunk deadline seam did not apply", elapsed)
	}
}

func TestCrew_CrewHealthy_Ugly_ListenerArrivesMidPoll(t *testing.T) {
	shrinkHealthTimings(t, 2*time.Second, 100*time.Millisecond, 30*time.Millisecond)
	port := freeLoopbackPort(t)
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))

	go func() {
		time.Sleep(90 * time.Millisecond)
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			return
		}
		defer ln.Close()
		time.Sleep(1 * time.Second)
	}()

	if !crewHealthy(addr) {
		t.Fatalf("crewHealthy(%s) = false, want true (listener opened mid-poll)", addr)
	}
}

// --- spawnMember ---

func TestCrew_SpawnMember_Good_AddrFlagArgsBuilt(t *testing.T) {
	// Binary can't exist, so this exercises the AddrFlag arg-building +
	// resolveCrewBinary + process.Start error path in one pass. process.Start
	// fails closed (ErrServiceNotInitialized) because this test binary never
	// calls process.Init — belt-and-braces on top of the fake binary name.
	m := crewMember{Binary: crewFakeBinary, AddrFlag: "--addr", Serve: []string{"serve"}}
	procID, sessionID, errMsg := spawnMember(context.Background(), m, 19999)
	if procID != "" || sessionID != "" {
		t.Fatalf("spawnMember unexpectedly reported success: procID=%q sessionID=%q", procID, sessionID)
	}
	if errMsg == "" {
		t.Fatalf("expected a non-empty error message")
	}
}

func TestCrew_SpawnMember_Bad_AddrEnvWithExtraEnvUsesStartWithOptions(t *testing.T) {
	m := crewMember{Binary: crewFakeBinary, AddrEnv: "FAKE_ADDR", Env: []string{"X=1"}}
	procID, sessionID, errMsg := spawnMember(context.Background(), m, 19999)
	if procID != "" || sessionID != "" || errMsg == "" {
		t.Fatalf("expected StartWithOptions failure, got procID=%q sessionID=%q err=%q", procID, sessionID, errMsg)
	}
}

func TestCrew_SpawnMember_Ugly_WatchMemberUsesTerminalSpawn(t *testing.T) {
	m := crewMember{Binary: crewFakeBinary, Watch: true, Serve: []string{"hub"}}
	procID, sessionID, errMsg := spawnMember(context.Background(), m, 19999)
	if procID != "" {
		t.Fatalf("Watch member unexpectedly set a procID: %q", procID)
	}
	if sessionID != "" || errMsg == "" {
		t.Fatalf("expected terminal.Spawn to fail for a nonexistent binary, got sessionID=%q err=%q", sessionID, errMsg)
	}
}

// --- crewWait / crewKill ---

func TestCrew_CrewWait_Good_SessionIDPathReturnsPromptly(t *testing.T) {
	inst := &crewInstance{sessionID: "does-not-exist-in-pool"}
	done := make(chan struct{})
	go func() { crewWait(inst); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("crewWait on an unknown sessionID should return immediately")
	}
}

func TestCrew_CrewWait_Bad_ProcIDPathReturnsPromptly(t *testing.T) {
	inst := &crewInstance{procID: "does-not-exist"}
	done := make(chan struct{})
	go func() { crewWait(inst); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("crewWait on an unknown procID should return immediately (process.Default is nil here)")
	}
}

func TestCrew_CrewKill_Good_SessionIDPath(t *testing.T) {
	inst := &crewInstance{sessionID: "does-not-exist-in-pool"}
	crewKill(inst) // must not panic
}

func TestCrew_CrewKill_Bad_ProcIDPath(t *testing.T) {
	inst := &crewInstance{procID: "does-not-exist"}
	crewKill(inst) // must not panic
}

func TestCrew_CrewKill_Ugly_EmptyInstanceIsNoop(t *testing.T) {
	inst := &crewInstance{}
	crewKill(inst) // neither branch fires; must not panic
}

// --- crewSupervisor.launch ---

func TestCrew_Launch_Good_AdoptsAlreadyListeningPort(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	sup := &crewSupervisor{stopCh: make(chan struct{})}
	m := crewMember{Binary: crewFakeBinary, BasePort: port}
	sup.launch(context.Background(), m, 0)

	sup.mu.Lock()
	n := len(sup.instances)
	sup.mu.Unlock()
	if n != 0 {
		t.Fatalf("launch spawned an instance despite an already-listening port; instances=%d", n)
	}
}

func TestCrew_Launch_Bad_SpawnFailureRecordsNoInstance(t *testing.T) {
	sup := &crewSupervisor{stopCh: make(chan struct{})}
	m := crewMember{Binary: crewFakeBinary, BasePort: freeLoopbackPort(t)}
	sup.launch(context.Background(), m, 0)

	sup.mu.Lock()
	n := len(sup.instances)
	sup.mu.Unlock()
	if n != 0 {
		t.Fatalf("launch recorded an instance despite a guaranteed spawn failure; instances=%d", n)
	}
}

// --- superviseCrew ---

func TestCrew_SuperviseCrew_Good_NoMatchingCapabilitySkipsEverything(t *testing.T) {
	crew := []crewMember{{Capability: CapabilityInference, Binary: crewFakeBinary, BasePort: freeLoopbackPort(t)}}
	sup := superviseCrew(context.Background(), crew, []string{CapabilitySandbox})
	if sup == nil {
		t.Fatalf("superviseCrew returned nil")
	}
	sup.mu.Lock()
	n := len(sup.instances)
	sup.mu.Unlock()
	if n != 0 {
		t.Fatalf("no capability matched — expected zero instances, got %d", n)
	}
}

func TestCrew_SuperviseCrew_Bad_DevModeSkipsInferenceSidecar(t *testing.T) {
	t.Setenv("LTHN_DEV", "1")
	t.Setenv("LTHN_CREW_INFERENCE", "")
	crew := []crewMember{{Capability: CapabilityInference, Binary: crewFakeBinary, BasePort: freeLoopbackPort(t)}}
	sup := superviseCrew(context.Background(), crew, []string{CapabilityInference})
	sup.mu.Lock()
	n := len(sup.instances)
	sup.mu.Unlock()
	if n != 0 {
		t.Fatalf("dev-mode should skip the inference sidecar entirely, got %d instances", n)
	}
}

func TestCrew_SuperviseCrew_Ugly_ZeroCountClampsToOneAttempt(t *testing.T) {
	t.Setenv("LTHN_DEV", "")
	crew := []crewMember{{Capability: CapabilitySandbox, Binary: crewFakeBinary, BasePort: freeLoopbackPort(t), Count: 0}}
	sup := superviseCrew(context.Background(), crew, []string{CapabilitySandbox})
	sup.mu.Lock()
	n := len(sup.instances)
	sup.mu.Unlock()
	// Count<1 clamps to a single attempt; the fake binary can't spawn, so
	// launch() must record zero — asserts the clamp+attempt path ran clean.
	if n != 0 {
		t.Fatalf("spawn should have failed cleanly, got %d instances", n)
	}
}

// --- crewSupervisor.stop ---

func TestCrew_CrewSupervisorStop_Good(t *testing.T) {
	sup := &crewSupervisor{stopCh: make(chan struct{})}
	sup.instances = []*crewInstance{
		{procID: "does-not-exist"},
		{sessionID: "does-not-exist-in-pool"},
	}
	sup.stop()
	if !sup.stopped {
		t.Fatalf("stop must mark the supervisor stopped")
	}
	select {
	case <-sup.stopCh:
	default:
		t.Fatalf("stop must close stopCh")
	}
}

func TestCrew_CrewSupervisorStop_Bad_IdempotentSecondCall(t *testing.T) {
	sup := &crewSupervisor{stopCh: make(chan struct{})}
	sup.stop()
	sup.stop() // must not panic (closing an already-closed channel would)
}

func TestCrew_CrewSupervisorStop_Ugly_NilReceiver(t *testing.T) {
	var sup *crewSupervisor
	sup.stop() // nil-safe; must not panic
}

// --- superviseInstance ---

// TestCrew_SuperviseInstance_Good_StoppedBeforeFirstWaitReturnsFast asserts
// the top-of-loop stopped-check exits the goroutine without ever reaching
// the respawn/backoff machinery.
func TestCrew_SuperviseInstance_Good_StoppedBeforeFirstWaitReturnsFast(t *testing.T) {
	shrinkHealthTimings(t, 60*time.Millisecond, 15*time.Millisecond, 15*time.Millisecond)
	sup := &crewSupervisor{stopCh: make(chan struct{})}
	sup.stopped = true
	close(sup.stopCh)

	inst := &crewInstance{procID: "does-not-exist", addr: net.JoinHostPort("127.0.0.1", strconv.Itoa(freeLoopbackPort(t)))}
	m := crewMember{Binary: crewFakeBinary, BasePort: 0}

	done := make(chan struct{})
	go func() { sup.superviseInstance(context.Background(), m, 0, inst); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatalf("superviseInstance did not return promptly once stopped==true")
	}
}

// TestCrew_SuperviseInstance_Bad_StopChWinsOverBackoffWait exercises the
// health-check-false path, the "exited, respawning" branch, and the select's
// stopCh case (in place of waiting out the real backoff).
func TestCrew_SuperviseInstance_Bad_StopChWinsOverBackoffWait(t *testing.T) {
	shrinkHealthTimings(t, 60*time.Millisecond, 15*time.Millisecond, 15*time.Millisecond)
	shrinkBackoff(t, 2*time.Second, 4*time.Second) // long enough that only the stopCh case can win

	sup := &crewSupervisor{stopCh: make(chan struct{})}
	inst := &crewInstance{procID: "does-not-exist", addr: net.JoinHostPort("127.0.0.1", strconv.Itoa(freeLoopbackPort(t)))}
	m := crewMember{Binary: crewFakeBinary, BasePort: 0}

	done := make(chan struct{})
	go func() { sup.superviseInstance(context.Background(), m, 0, inst); close(done) }()

	// Let the first crewWait (instant, procID unknown) + health-check-false
	// branch run, then stop — must exit via the select's stopCh case rather
	// than waiting the full (shrunk-but-still-relatively-long) backoff.
	time.Sleep(120 * time.Millisecond)
	sup.stop()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatalf("superviseInstance did not honour stopCh over the backoff wait")
	}
}

// TestCrew_SuperviseInstance_Ugly_RespawnRetryPathOnBackoffExpiry lets the
// (shrunk) backoff actually elapse so the retry branch runs: spawnMember is
// re-invoked (fails against crewFakeBinary), "respawn failed" logs, and the
// loop continues — then stop() ends it on the next iteration.
func TestCrew_SuperviseInstance_Ugly_RespawnRetryPathOnBackoffExpiry(t *testing.T) {
	shrinkHealthTimings(t, 60*time.Millisecond, 15*time.Millisecond, 15*time.Millisecond)
	shrinkBackoff(t, 20*time.Millisecond, 100*time.Millisecond)

	sup := &crewSupervisor{stopCh: make(chan struct{})}
	inst := &crewInstance{procID: "does-not-exist", addr: net.JoinHostPort("127.0.0.1", strconv.Itoa(freeLoopbackPort(t)))}
	m := crewMember{Binary: crewFakeBinary, BasePort: 0}

	done := make(chan struct{})
	go func() { sup.superviseInstance(context.Background(), m, 0, inst); close(done) }()

	// Give the loop several backoff cycles (each ~20-100ms) to run the
	// retry-spawn branch before asking it to stop.
	time.Sleep(250 * time.Millisecond)
	sup.stop()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatalf("superviseInstance did not stop after several retry cycles")
	}
}

// TestCrew_SuperviseInstance_Good_HealthyAddrLogsUp covers the crewHealthy
// true branch (the "up" Info log) inside superviseInstance. The listener at
// inst.addr is opened directly by this test — nothing was spawned to
// produce it — so this stays within the "never invoke a real binary" rule
// while still exercising the real health-gate success path.
func TestCrew_SuperviseInstance_Good_HealthyAddrLogsUp(t *testing.T) {
	shrinkHealthTimings(t, 500*time.Millisecond, 50*time.Millisecond, 20*time.Millisecond)
	shrinkBackoff(t, 2*time.Second, 4*time.Second) // long enough that only stop() ends the loop

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	sup := &crewSupervisor{stopCh: make(chan struct{})}
	inst := &crewInstance{procID: "does-not-exist", addr: ln.Addr().String()}
	m := crewMember{Binary: crewFakeBinary, BasePort: 0}

	done := make(chan struct{})
	go func() { sup.superviseInstance(context.Background(), m, 0, inst); close(done) }()

	time.Sleep(100 * time.Millisecond)
	sup.stop()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatalf("superviseInstance did not stop after the health-check-up path")
	}
}

// TestCrew_SuperviseInstance_Ugly_BackoffResetsAfterApparentlyLongRun covers
// the "time.Since(startedAt) > crewBackoffMax { backoff = crewBackoffMin }"
// reset branch by shrinking crewBackoffMax to 1ns — any real crewWait call
// (even the near-instant bogus-procID path) takes longer than that on any
// real CPU, so the reset fires deterministically without waiting out a real
// long-lived process.
func TestCrew_SuperviseInstance_Ugly_BackoffResetsAfterApparentlyLongRun(t *testing.T) {
	shrinkHealthTimings(t, 60*time.Millisecond, 15*time.Millisecond, 15*time.Millisecond)
	shrinkBackoff(t, 10*time.Millisecond, 1*time.Nanosecond)

	sup := &crewSupervisor{stopCh: make(chan struct{})}
	inst := &crewInstance{procID: "does-not-exist", addr: net.JoinHostPort("127.0.0.1", strconv.Itoa(freeLoopbackPort(t)))}
	m := crewMember{Binary: crewFakeBinary, BasePort: 0}

	done := make(chan struct{})
	go func() { sup.superviseInstance(context.Background(), m, 0, inst); close(done) }()

	time.Sleep(80 * time.Millisecond)
	sup.stop()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatalf("superviseInstance did not stop after the backoff-reset path")
	}
}
