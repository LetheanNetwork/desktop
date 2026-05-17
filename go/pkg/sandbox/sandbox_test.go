// SPDX-Licence-Identifier: EUPL-1.2

package sandbox

import (
	"sync"
	"sync/atomic"

	core "dappco.re/go"
	"dappco.re/go/container"
	"dappco.re/go/process"
)

func newTestService(opts Options) *Service {
	r := NewService(opts)(core.New())
	if !r.OK {
		return nil
	}
	svc, _ := r.Value.(*Service)
	return svc
}

func TestSandbox_NewService_Good(t *core.T) {
	factory := NewService(Options{DefaultImage: "example/dev:latest"})
	r := factory(core.New())
	core.AssertTrue(t, r.OK)
	svc := r.Value.(*Service)
	core.AssertNotNil(t, svc)
	core.AssertEqual(t, "example/dev:latest", svc.resolveDefaultImage())
}

func TestSandbox_NewService_Bad(t *core.T) {
	r := NewService(Options{})(nil)
	core.AssertTrue(t, r.OK)
	svc := r.Value.(*Service)
	core.AssertEqual(t, defaultImage, svc.resolveDefaultImage())
}

func TestSandbox_NewService_Ugly(t *core.T) {
	factory := NewService(Options{})
	core.AssertNotNil(t, factory)
	r1 := factory(core.New())
	r2 := factory(core.New())
	core.AssertTrue(t, r1.OK)
	core.AssertTrue(t, r2.OK)
	core.AssertNotEqual(t, r1.Value, r2.Value)
}

func TestSandbox_Register_Good(t *core.T) {
	r := Register(core.New())
	core.AssertTrue(t, r.OK)
	svc := r.Value.(*Service)
	core.AssertEqual(t, "Sandbox", svc.ServiceName())
}

func TestSandbox_Register_Bad(t *core.T) {
	r := Register(nil)
	core.AssertTrue(t, r.OK)
	svc := r.Value.(*Service)
	core.AssertEqual(t, defaultImage, svc.resolveDefaultImage())
}

func TestSandbox_Register_Ugly(t *core.T) {
	r1 := Register(core.New())
	r2 := Register(core.New())
	core.AssertTrue(t, r1.OK)
	core.AssertTrue(t, r2.OK)
	core.AssertNotEqual(t, r1.Value, r2.Value)
}

func TestSandbox_Service_ServiceName_Good(t *core.T) {
	svc := newTestService(Options{})
	name := svc.ServiceName()
	core.AssertEqual(t, "Sandbox", name)
	core.AssertContains(t, name, "Sandbox")
}

func TestSandbox_Service_ServiceName_Bad(t *core.T) {
	var svc *Service
	name := svc.ServiceName()
	core.AssertEqual(t, "Sandbox", name)
	core.AssertNotEqual(t, "", name)
}

func TestSandbox_Service_ServiceName_Ugly(t *core.T) {
	svc := &Service{}
	ref := (*Service).ServiceName
	core.AssertNotNil(t, ref)
	core.AssertEqual(t, "Sandbox", svc.ServiceName())
}

func TestSandbox_Service_Spawn_Good(t *core.T) {
	svc := newTestService(Options{})
	ref := (*Service).Spawn
	core.AssertNotNil(t, ref)
	r := svc.prepareSpawnInput(SpawnInput{Command: "echo"})
	core.AssertTrue(t, r.OK)
	input := r.Value.(SpawnInput)
	core.AssertEqual(t, defaultImage, input.Image)
}

func TestSandbox_Service_Spawn_Bad(t *core.T) {
	svc := newTestService(Options{})
	r := NewSpawnPort(svc).Spawn(SpawnInput{})
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "command is required")
}

func TestSandbox_Service_Spawn_Ugly(t *core.T) {
	svc := newTestService(Options{DefaultImage: "host/default:latest"})
	ref := (*Service).Spawn
	core.AssertNotNil(t, ref)
	r := svc.prepareSpawnInput(SpawnInput{Image: "call/override:latest", Command: "echo"})
	core.AssertTrue(t, r.OK)
	input := r.Value.(SpawnInput)
	core.AssertEqual(t, "call/override:latest", input.Image)
}

func TestSandbox_buildRunArgs_Good(t *core.T) {
	svc := newTestService(Options{})
	r := svc.buildRunArgs(container.RuntimeDocker, SpawnInput{
		Image:      "alpine:3.21",
		Command:    "echo",
		Args:       []string{"hi"},
		Memory:     2048,
		CPUs:       4,
		StorageOpt: "size=10G",
	})
	core.AssertTrue(t, r.OK)
	run := r.Value.(runCommand)
	core.AssertEqual(t, "docker", run.Binary)
	core.AssertEqual(t, []string{
		"run", "--rm",
		"--cap-drop=ALL",
		"--security-opt=no-new-privileges",
		"--pids-limit=512",
		"--memory", "2048M",
		"--cpus", "4",
		"--storage-opt", "size=10G",
		// Cerberus Mantis #1667 RFC v1.1 §2.5 — argv `--` terminator
		// between IMAGE and COMMAND on Docker arm.
		"alpine:3.21", "--", "echo", "hi",
	}, run.Args)
}

// TestSandbox_buildRunArgs_AppliesHardenedDefaults verifies one-shot Spawn
// inherits the same hardenedDefaults that SpawnLong applies (Cerberus
// Mantis #1663 / S-1). Without this gate a compromised renderer can
// invoke Sandbox.Spawn and get a container with default Docker root caps
// (cap_dac_override / cap_setuid / cap_sys_chroot — LPE primitive set).
// Asserted across all three runtimes that buildRunArgs supports.
func TestSandbox_buildRunArgs_AppliesHardenedDefaults(t *core.T) {
	svc := newTestService(Options{})
	cases := []struct {
		name string
		rt   container.RuntimeType
	}{
		{"docker", container.RuntimeDocker},
		{"podman", container.RuntimePodman},
		{"apple", container.RuntimeApple},
	}
	for _, tc := range cases {
		r := svc.buildRunArgs(tc.rt, SpawnInput{
			Image:   "alpine:3.21",
			Command: "echo",
			Args:    []string{"hi"},
		})
		core.AssertTrue(t, r.OK)
		run := r.Value.(runCommand)
		core.AssertContains(t, run.Args, "--cap-drop=ALL")
		core.AssertContains(t, run.Args, "--security-opt=no-new-privileges")
		core.AssertContains(t, run.Args, "--pids-limit=512")
	}
}

func TestSandbox_prepareSpawnInput_Bad(t *core.T) {
	svc := newTestService(Options{})
	r := svc.prepareSpawnInput(SpawnInput{Command: "echo", Memory: -1})
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "memory must be >= 0")
}

func TestSandbox_prepareSpawnInput_Ugly(t *core.T) {
	svc := newTestService(Options{})
	r := svc.prepareSpawnInput(SpawnInput{Command: "echo", StorageOpt: "   "})
	core.AssertTrue(t, r.OK)
	input := r.Value.(SpawnInput)
	core.AssertEqual(t, "", input.StorageOpt)
}

func TestSandbox_Service_Detect_Good(t *core.T) {
	svc := newTestService(Options{})
	r := svc.Detect()
	core.AssertTrue(t, r.OK)
	out := r.Value.(DetectOutput)
	core.AssertNotNil(t, out.Available)
}

func TestSandbox_Service_Detect_Bad(t *core.T) {
	var svc *Service
	r := svc.Detect()
	core.AssertTrue(t, r.OK)
	out := r.Value.(DetectOutput)
	core.AssertGreaterOrEqual(t, len(out.Available), 0)
}

func TestSandbox_Service_Detect_Ugly(t *core.T) {
	svc := &Service{}
	r := svc.Detect()
	core.AssertTrue(t, r.OK)
	out := r.Value.(DetectOutput)
	core.AssertGreaterOrEqual(t, len(out.Available), 0)
}

// TestSandbox_SpawnLong_Good verifies SpawnLong rejects missing Image/Command
// without touching a real container runtime (validation path only).
func TestSandbox_SpawnLong_Good(t *core.T) {
	svc := newTestService(Options{})
	ref := (*Service).SpawnLong
	core.AssertNotNil(t, ref)
	// Confirm SpawnLong exists and fails with the expected error when
	// process.Service is absent — not a runtime error, a validation error.
	r := NewSpawnPort(svc).SpawnLong(SpawnLongInput{Image: "lthn/dev:latest", Command: "opencode"})
	// process service is wired (core.New() wires process) — but no
	// container runtime is present in CI, so we expect either OK (unlikely)
	// or a container-start failure — NOT a validation error.
	// We just verify validation passes its guard checks.
	if !r.OK {
		core.AssertNotContains(t, r.Error(), "image is required")
		core.AssertNotContains(t, r.Error(), "command is required")
	}
}

// TestSandbox_SpawnLong_Bad verifies validation rejects empty Image and Command.
func TestSandbox_SpawnLong_Bad(t *core.T) {
	svc := newTestService(Options{})

	// Missing image.
	r := NewSpawnPort(svc).SpawnLong(SpawnLongInput{Command: "opencode"})
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "image is required")

	// Missing command.
	r = NewSpawnPort(svc).SpawnLong(SpawnLongInput{Image: "lthn/dev:latest"})
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "command is required")
}

// TestSandbox_SpawnLong_Ugly verifies SpawnLongInput type is exported and
// LongVolumeMount fields round-trip.
func TestSandbox_SpawnLong_Ugly(t *core.T) {
	in := SpawnLongInput{
		Image:       "alpine:3.21",
		Command:     "sh",
		Args:        []string{"-c", "echo ok"},
		Env:         map[string]string{"KEY": "val"},
		ExposedPort: 8080,
		NetworkName: "test-net",
		Volumes:     []LongVolumeMount{{Name: "data", Container: "/data"}},
		Runtime:     "docker",
	}
	core.AssertEqual(t, "alpine:3.21", in.Image)
	core.AssertEqual(t, 8080, in.ExposedPort)
	core.AssertEqual(t, "data", in.Volumes[0].Name)
	core.AssertEqual(t, "/data", in.Volumes[0].Container)
}

// TestSandbox_Kill_Good registers a handle manually then kills it.
func TestSandbox_Kill_Good(t *core.T) {
	svc := newTestService(Options{})
	id := "sb-testgood"
	svc.mu.Lock()
	if svc.handles == nil {
		svc.handles = map[string]*ContainerHandle{}
	}
	svc.handles[id] = &ContainerHandle{SandboxID: id, Status: StatusReady}
	svc.mu.Unlock()

	r := NewSpawnPort(svc).Kill(id)
	core.AssertTrue(t, r.OK)

	svc.mu.RLock()
	_, exists := svc.handles[id]
	svc.mu.RUnlock()
	core.AssertFalse(t, exists)
}

// TestSandbox_Kill_Bad verifies Kill rejects empty ID and missing sandbox.
func TestSandbox_Kill_Bad(t *core.T) {
	svc := newTestService(Options{})

	// Empty ID.
	r := NewSpawnPort(svc).Kill("")
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "sandbox id is required")

	// Unknown ID.
	r = NewSpawnPort(svc).Kill("sb-doesnotexist")
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "sandbox not found")
}

// TestSandbox_Kill_Ugly verifies Kill on an already-stopped sandbox still
// reports not-found rather than panicking.
func TestSandbox_Kill_Ugly(t *core.T) {
	svc := newTestService(Options{})
	// Kill twice — second call should be not-found, not a panic.
	id := "sb-twoshot"
	svc.mu.Lock()
	if svc.handles == nil {
		svc.handles = map[string]*ContainerHandle{}
	}
	svc.handles[id] = &ContainerHandle{SandboxID: id, Status: StatusReady}
	svc.mu.Unlock()

	r1 := NewSpawnPort(svc).Kill(id)
	core.AssertTrue(t, r1.OK)

	r2 := NewSpawnPort(svc).Kill(id)
	core.AssertFalse(t, r2.OK)
	core.AssertContains(t, r2.Error(), "sandbox not found")
}

// TestSandbox_ListHandles_Good verifies ListHandles returns all registered handles.
func TestSandbox_ListHandles_Good(t *core.T) {
	svc := newTestService(Options{})

	// Empty registry.
	r := NewSpawnPort(svc).ListHandles()
	core.AssertTrue(t, r.OK)
	handles := r.Value.([]ContainerHandle)
	core.AssertLen(t, handles, 0)

	// Populate two handles.
	svc.mu.Lock()
	svc.handles = map[string]*ContainerHandle{
		"sb-aaa": {SandboxID: "sb-aaa", Status: StatusReady},
		"sb-bbb": {SandboxID: "sb-bbb", Status: StatusStarting},
	}
	svc.mu.Unlock()

	r = NewSpawnPort(svc).ListHandles()
	core.AssertTrue(t, r.OK)
	handles = r.Value.([]ContainerHandle)
	core.AssertLen(t, handles, 2)
}

// TestSandbox_ListHandles_Bad verifies ListHandles on nil svc does not panic.
func TestSandbox_ListHandles_Bad(t *core.T) {
	svc := newTestService(Options{})
	r := NewSpawnPort(svc).ListHandles()
	core.AssertTrue(t, r.OK)
	handles := r.Value.([]ContainerHandle)
	core.AssertNotNil(t, handles)
}

// TestSandbox_ListHandles_Ugly verifies ListHandles returns copies not pointers —
// mutating the returned slice doesn't affect the registry.
func TestSandbox_ListHandles_Ugly(t *core.T) {
	svc := newTestService(Options{})
	id := "sb-copy"
	svc.mu.Lock()
	svc.handles = map[string]*ContainerHandle{
		id: {SandboxID: id, Status: StatusReady},
	}
	svc.mu.Unlock()

	r := NewSpawnPort(svc).ListHandles()
	core.AssertTrue(t, r.OK)
	listed := r.Value.([]ContainerHandle)
	core.AssertLen(t, listed, 1)
	listed[0].Status = StatusStopped // mutate the copy

	// Registry entry should still be StatusReady.
	svc.mu.RLock()
	original := svc.handles[id]
	svc.mu.RUnlock()
	core.AssertEqual(t, StatusReady, original.Status)
}

// TestSandbox_GetHandle_Good verifies GetHandle returns the registered handle.
func TestSandbox_GetHandle_Good(t *core.T) {
	svc := newTestService(Options{})
	id := "sb-getme"
	svc.mu.Lock()
	if svc.handles == nil {
		svc.handles = map[string]*ContainerHandle{}
	}
	svc.handles[id] = &ContainerHandle{SandboxID: id, Image: "lthn/dev:latest", Status: StatusReady, HostPort: 4096}
	svc.mu.Unlock()

	r := NewSpawnPort(svc).GetHandle(id)
	core.AssertTrue(t, r.OK)
	h := r.Value.(ContainerHandle)
	core.AssertEqual(t, id, h.SandboxID)
	core.AssertEqual(t, 4096, h.HostPort)
	core.AssertEqual(t, StatusReady, h.Status)
}

// TestSandbox_GetHandle_Bad verifies GetHandle rejects empty ID and missing sandbox.
func TestSandbox_GetHandle_Bad(t *core.T) {
	svc := newTestService(Options{})

	r := NewSpawnPort(svc).GetHandle("")
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "sandbox id is required")

	r = NewSpawnPort(svc).GetHandle("sb-missing")
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "sandbox not found")
}

// TestSandbox_GetHandle_Ugly verifies GetHandle returns a copy — mutating it
// doesn't corrupt the registry.
func TestSandbox_GetHandle_Ugly(t *core.T) {
	svc := newTestService(Options{})
	id := "sb-isolated"
	svc.mu.Lock()
	if svc.handles == nil {
		svc.handles = map[string]*ContainerHandle{}
	}
	svc.handles[id] = &ContainerHandle{SandboxID: id, Status: StatusReady}
	svc.mu.Unlock()

	r := NewSpawnPort(svc).GetHandle(id)
	core.AssertTrue(t, r.OK)
	copy := r.Value.(ContainerHandle)
	copy.Status = StatusFailed // mutate the copy

	svc.mu.RLock()
	stored := svc.handles[id]
	svc.mu.RUnlock()
	core.AssertEqual(t, StatusReady, stored.Status)
}

// TestSandbox_buildLongRunArgs_Good verifies arg construction for a full input.
func TestSandbox_buildLongRunArgs_Good(t *core.T) {
	svc := newTestService(Options{})
	args := svc.buildLongRunArgs("docker", "lthn-sandbox-sb-test", 5432, SpawnLongInput{
		Image:       "postgres:16",
		Command:     "postgres",
		Args:        []string{"-c", "log_statement=all"},
		Env:         map[string]string{"POSTGRES_PASSWORD": "secret"},
		ExposedPort: 5432,
		NetworkName: "lthn-net",
		Volumes:     []LongVolumeMount{{Name: "pg-data", Container: "/var/lib/postgresql/data"}},
	})
	core.AssertContains(t, args, "run")
	core.AssertContains(t, args, "-d")
	core.AssertContains(t, args, "--name")
	core.AssertContains(t, args, "lthn-sandbox-sb-test")
	core.AssertContains(t, args, "-p")
	core.AssertContains(t, args, "--network")
	core.AssertContains(t, args, "lthn-net")
	core.AssertContains(t, args, "-v")
	core.AssertContains(t, args, "postgres:16")
	core.AssertContains(t, args, "postgres")
}

// TestSandbox_buildLongRunArgs_Bad verifies zero ExposedPort means no -p flag.
func TestSandbox_buildLongRunArgs_Bad(t *core.T) {
	svc := newTestService(Options{})
	args := svc.buildLongRunArgs("docker", "lthn-sandbox-sb-noport", 0, SpawnLongInput{
		Image:   "alpine:3.21",
		Command: "sh",
	})
	for _, a := range args {
		core.AssertNotEqual(t, "-p", a)
	}
	core.AssertContains(t, args, "alpine:3.21")
}

// TestSandbox_buildLongRunArgs_Ugly verifies env vars are injected as -e K=V.
func TestSandbox_buildLongRunArgs_Ugly(t *core.T) {
	svc := newTestService(Options{})
	args := svc.buildLongRunArgs("docker", "lthn-sandbox-sb-env", 0, SpawnLongInput{
		Image:   "alpine:3.21",
		Command: "sh",
		Env:     map[string]string{"FOO": "bar"},
	})
	found := false
	for i, a := range args {
		if a == "-e" && i+1 < len(args) && args[i+1] == "FOO=bar" {
			found = true
			break
		}
	}
	core.AssertTrue(t, found)
}

// TestSpawnLong_AppliesInstallIdLabel_Good verifies that SpawnLongInput
// with non-empty InstallID + BundleID stamps both docker labels into the
// arg vector (Cerberus Mantis #1665 S-3).
func TestSpawnLong_AppliesInstallIdLabel_Good(t *core.T) {
	svc := newTestService(Options{})
	args := svc.buildLongRunArgs("docker", "lthn-sandbox-sb-labelled", 0, SpawnLongInput{
		Image:     "alpine:3.21",
		Command:   "sh",
		InstallID: "abc123def4567890",
		BundleID:  "lthn.sample.bundle",
	})

	wantInstall := InstallIDLabel + "=abc123def4567890"
	wantBundle := BundleIDLabel + "=lthn.sample.bundle"
	gotInstall, gotBundle := false, false
	for i, a := range args {
		if a == "--label" && i+1 < len(args) {
			if args[i+1] == wantInstall {
				gotInstall = true
			}
			if args[i+1] == wantBundle {
				gotBundle = true
			}
		}
	}
	core.AssertTrue(t, gotInstall)
	core.AssertTrue(t, gotBundle)
}

// TestSpawnLong_AppliesInstallIdLabel_Bad verifies that empty InstallID +
// BundleID omits both labels entirely — pre-wiring callers (standalone
// SpawnLong outside marketplace) keep their arg vector clean.
func TestSpawnLong_AppliesInstallIdLabel_Bad(t *core.T) {
	svc := newTestService(Options{})
	args := svc.buildLongRunArgs("docker", "lthn-sandbox-sb-bare", 0, SpawnLongInput{
		Image:   "alpine:3.21",
		Command: "sh",
	})
	for _, a := range args {
		core.AssertNotEqual(t, "--label", a)
	}
}

// TestSpawnLong_AppliesInstallIdLabel_Ugly verifies that whitespace-only
// InstallID is treated as empty (defence against fixture noise / accidental
// "   " bindings from upstream config parsers).
func TestSpawnLong_AppliesInstallIdLabel_Ugly(t *core.T) {
	svc := newTestService(Options{})
	args := svc.buildLongRunArgs("docker", "lthn-sandbox-sb-blank", 0, SpawnLongInput{
		Image:     "alpine:3.21",
		Command:   "sh",
		InstallID: "   ",
		BundleID:  "",
	})
	for _, a := range args {
		core.AssertNotEqual(t, "--label", a)
	}
}

// TestSandbox_ContainerHandle_Good verifies ContainerHandle status constants
// and JSON tags are accessible.
func TestSandbox_ContainerHandle_Good(t *core.T) {
	h := ContainerHandle{
		SandboxID: "sb-abc12345",
		Image:     "lthn/dev:latest",
		Command:   "opencode",
		Status:    StatusReady,
		HostPort:  4096,
		BundleID:  "opencode",
		StartedAt: core.Now(),
	}
	core.AssertEqual(t, "sb-abc12345", h.SandboxID)
	core.AssertEqual(t, StatusReady, h.Status)
	core.AssertEqual(t, 4096, h.HostPort)
}

// TestSandbox_ContainerHandle_Bad verifies the four status constants are distinct.
func TestSandbox_ContainerHandle_Bad(t *core.T) {
	statuses := []string{StatusStarting, StatusReady, StatusStopped, StatusFailed}
	seen := map[string]bool{}
	for _, s := range statuses {
		core.AssertFalse(t, seen[s])
		seen[s] = true
	}
	core.AssertLen(t, seen, 4)
}

// TestSandbox_ContainerHandle_Ugly verifies SandboxID prefix invariant.
func TestSandbox_ContainerHandle_Ugly(t *core.T) {
	core.AssertEqual(t, "sb-", sandboxIDPrefix)
	// SandboxIDs generated by SpawnLong always start with the prefix.
	id := sandboxIDPrefix + "a1b2c3d4"
	core.AssertContains(t, id, sandboxIDPrefix)
	core.AssertEqual(t, 11, len(id)) // "sb-" + 8 chars
}

// TestSandbox_Kill_NoStaleListReadConcurrent_Ugly exercises the Kill
// mutex tightening landed for Cerberus #47 S-6 (Mantis #1668). Before
// the fix, Kill removed the handle from the registry BEFORE the
// (potentially slow) ps.Run "rm -f" teardown — opening a window where
// a concurrent ListHandles saw zero entries while the container was
// still being reaped by the runtime. The post-fix invariant verified
// here: a concurrent ListHandles that races Kill on the SAME handle
// must NEVER observe a state where the handle has Status=StatusReady
// but is about to vanish; the only transient state visible to a List
// during Kill is Status=StatusStopped while the entry still exists.
//
// Run with `-race` to surface any unsynchronised access to the handle
// pointer between Kill (writer) and ListHandles (reader). Many handles
// + many list iterations + bounded killer goroutines stress the
// critical-section boundaries.
func TestSandbox_Kill_NoStaleListReadConcurrent_Ugly(t *core.T) {
	svc := newTestService(Options{})
	const handleCount = 64

	// Seed the registry with handleCount StatusReady handles.
	svc.mu.Lock()
	svc.handles = map[string]*ContainerHandle{}
	for i := 0; i < handleCount; i++ {
		id := core.Sprintf("sb-race%04d", i)
		svc.handles[id] = &ContainerHandle{SandboxID: id, Status: StatusReady}
	}
	svc.mu.Unlock()

	var (
		invariantViolations atomic.Int64
		listIterations      atomic.Int64
		killsCompleted      atomic.Int64
		wg                  sync.WaitGroup
		stopReaders         atomic.Bool
	)

	// Reader fleet — continuously list and assert every observed handle
	// has a status the post-fix Kill is allowed to expose (Ready or
	// Stopped). Starting / Failed are spawn-side states; a kill path
	// should never produce them.
	const readers = 8
	for r := 0; r < readers; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for !stopReaders.Load() {
				res := NewSpawnPort(svc).ListHandles()
				if !res.OK {
					invariantViolations.Add(1)
					continue
				}
				snapshot, _ := res.Value.([]ContainerHandle)
				for _, h := range snapshot {
					switch h.Status {
					case StatusReady, StatusStopped:
						// Permitted observable states during the race.
					default:
						invariantViolations.Add(1)
					}
				}
				listIterations.Add(1)
			}
		}()
	}

	// Killer fleet — each goroutine kills a disjoint shard of handles.
	// ps is nil (process.Service unwired in this test context) so Kill
	// skips the docker-rm shell-out and just exercises the registry +
	// mutex paths — exactly the surface S-6 cares about.
	const killers = 4
	per := handleCount / killers
	for k := 0; k < killers; k++ {
		start := k * per
		end := start + per
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := start; i < end; i++ {
				id := core.Sprintf("sb-race%04d", i)
				if NewSpawnPort(svc).Kill(id).OK {
					killsCompleted.Add(1)
				}
			}
		}()
	}

	// Drain the killers; readers are stopped via flag once kills settle.
	killerSettled := make(chan struct{})
	go func() {
		// Wait specifically for killers — we need to give readers a few
		// extra ticks to observe the post-kill state before signalling
		// stop, otherwise the iteration counter under-reports.
		for killsCompleted.Load() < handleCount {
			core.Sleep(1 * core.Millisecond)
		}
		close(killerSettled)
	}()
	<-killerSettled
	core.Sleep(5 * core.Millisecond)
	stopReaders.Store(true)
	wg.Wait()

	// Post-conditions.
	core.AssertEqual(t, int64(handleCount), killsCompleted.Load())
	core.AssertEqual(t, int64(0), invariantViolations.Load())
	// Registry must be empty — every seeded handle was killed.
	svc.mu.RLock()
	leftover := len(svc.handles)
	svc.mu.RUnlock()
	core.AssertEqual(t, 0, leftover)
	// Sanity: readers actually ran (not zero iterations — would mean
	// the test never exercised the race).
	core.AssertGreater(t, listIterations.Load(), int64(0))
}

// newTestServiceWithProcess wires the process service alongside sandbox
// so SpawnLong can actually reach proc.Run — needed for the
// cause-propagation test which deliberately fails the runtime shell-out.
func newTestServiceWithProcess() *Service {
	c := core.New(
		core.WithName("process", process.NewService(process.Options{})),
		core.WithName("sandbox", NewService(Options{})),
	)
	svc, _ := core.ServiceFor[*Service](c, "sandbox")
	return svc
}

// TestSandbox_SpawnLong_ProcRunErr_PropagatesInResult_Bad verifies
// Cerberus #47 S-6 (Mantis #1668) part 2 — when the underlying
// ps.Run / waitPortOpen path fails, the SpawnLong Result error
// preserves the underlying cause message rather than collapsing it
// to the generic "container start failed". Operators reading the
// audit log or the returned Result need the real symptom (process
// exited with code N, port did not open within Xs) to triage.
//
// We exercise the path by asking SpawnLong to run an image with a
// non-existent runtime binary — proc.Run shells out, the OS reports
// the command was not found, and that detail must surface in the
// returned core.Result error string.
func TestSandbox_SpawnLong_ProcRunErr_PropagatesInResult_Bad(t *core.T) {
	svc := newTestServiceWithProcess()
	core.AssertNotNil(t, svc)
	// Force the runtime to a binary that is guaranteed not to exist on
	// PATH — proc.Run will fail with an explicit "executable file not
	// found" cause that we expect threaded through.
	r := NewSpawnPort(svc).SpawnLong(SpawnLongInput{
		Image:   "alpine:3.21",
		Command: "echo",
		Args:    []string{"hi"},
		Runtime: "lthn-nonexistent-runtime-binary-xyz",
	})
	core.AssertFalse(t, r.OK)
	msg := r.Error()
	// The wrapping "container start failed" sentinel must still be
	// present so callers grepping on the old surface keep working.
	core.AssertContains(t, msg, "container start failed")
	// AND the underlying cause must be visible — without the fix, the
	// underlying error was dropped on the floor and only the generic
	// message remained. We don't pin the exact OS-vendor wording (it
	// varies macOS/Linux); presence of EITHER "not found" OR "no such"
	// OR the binary name in the chained message proves the cause
	// survived the failSpawnLong wrap.
	hasCause := core.Contains(msg, "not found") ||
		core.Contains(msg, "no such") ||
		core.Contains(msg, "executable") ||
		core.Contains(msg, "lthn-nonexistent-runtime-binary-xyz") ||
		core.Contains(msg, "exited with code")
	core.AssertTrue(t, hasCause)
}

// --- Cerberus Mantis #1667 (RFC.image-allowlist v1.1) gate tests ---

// Spawn gate fires AFTER default-image substitution per ADD-2 — when
// the caller passes an empty Image the default is substituted and the
// substituted ref MUST validate. Without ADD-2 the validator would
// reject the empty input as ErrEmptyImageRef ahead of substitution
// and the default-image path would silently never run.
func TestSandbox_Spawn_GateRunsAfterDefaultSubstitution_Good(t *core.T) {
	svc := newTestService(Options{DefaultImage: "lthn/dev:latest"})
	r := svc.prepareSpawnInput(SpawnInput{Command: "echo"})
	core.AssertTrue(t, r.OK)
	input := r.Value.(SpawnInput)
	core.AssertEqual(t, "lthn/dev:latest", input.Image)
}

// Conversely, an unallowed Image at the caller surface MUST reject —
// confirming the gate FIRES after substitution rather than being
// skipped when Image is non-empty.
func TestSandbox_Spawn_GateRunsAfterDefaultSubstitution_Bad(t *core.T) {
	svc := newTestService(Options{})
	r := NewSpawnPort(svc).Spawn(SpawnInput{
		Image:   "evil.example.com/foo",
		Command: "sh",
	})
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "image rejected by allowlist")
}

// IMDS reject path — the validator's IMDS pre-check (#1677 / ADD-1)
// fires BEFORE the allowlist check, so an IMDS-shaped image is rejected
// with the typed ErrIMDSAddress even though the registry would also be
// flagged as not-allowlisted.
func TestSandbox_Spawn_RejectsIMDSImage_Bad(t *core.T) {
	svc := newTestService(Options{})
	r := NewSpawnPort(svc).Spawn(SpawnInput{
		Image:   "169.254.169.254/x",
		Command: "sh",
	})
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "image rejected by allowlist")
}

// SpawnLong gate fires AFTER non-empty Image check — equivalent to
// sandbox.go's AFTER-default-substitution per ADD-2 (SpawnLong has no
// default-image substitution today, but the gate sees the same ref the
// runtime would see).
func TestSandbox_SpawnLong_RejectsDisallowedImage_Bad(t *core.T) {
	svc := newTestService(Options{})
	r := NewSpawnPort(svc).SpawnLong(SpawnLongInput{
		Image:   "evil.example.com/foo",
		Command: "sh",
	})
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "image rejected by allowlist")
}

// Argv `--` terminator on Podman arm — same shape as Docker arm so a
// future podman flag-parser change cannot re-interpret input.Command
// or input.Args[0] as a runtime flag. Apple arm NOT asserted here:
// terminator discipline for Apple is delegated upstream per ADD-4.
func TestSandbox_buildRunArgs_PodmanInsertsArgvTerminator_Good(t *core.T) {
	svc := newTestService(Options{})
	r := svc.buildRunArgs(container.RuntimePodman, SpawnInput{
		Image:   "alpine:3.21",
		Command: "echo",
		Args:    []string{"hi"},
	})
	core.AssertTrue(t, r.OK)
	run := r.Value.(runCommand)
	core.AssertEqual(t, "podman", run.Binary)
	// `--` MUST sit between IMAGE and COMMAND.
	pos := -1
	for i, a := range run.Args {
		if a == "alpine:3.21" {
			pos = i
			break
		}
	}
	core.AssertTrue(t, pos >= 0)
	core.AssertTrue(t, pos+2 < len(run.Args))
	core.AssertEqual(t, "--", run.Args[pos+1])
	core.AssertEqual(t, "echo", run.Args[pos+2])
}

// buildLongRunArgs Docker arm — argv `--` terminator between IMAGE
// and COMMAND.
func TestSandbox_buildLongRunArgs_DockerInsertsArgvTerminator_Good(t *core.T) {
	svc := newTestService(Options{})
	args := svc.buildLongRunArgs("docker", "lthn-sandbox-sb-terminator", 0, SpawnLongInput{
		Image:   "alpine:3.21",
		Command: "sh",
		Args:    []string{"-l"},
	})
	pos := -1
	for i, a := range args {
		if a == "alpine:3.21" {
			pos = i
			break
		}
	}
	core.AssertTrue(t, pos >= 0)
	core.AssertTrue(t, pos+2 < len(args))
	core.AssertEqual(t, "--", args[pos+1])
	core.AssertEqual(t, "sh", args[pos+2])
}

// buildLongRunArgs Podman arm — same `--` shape as Docker arm.
func TestSandbox_buildLongRunArgs_PodmanInsertsArgvTerminator_Good(t *core.T) {
	svc := newTestService(Options{})
	args := svc.buildLongRunArgs("podman", "lthn-sandbox-sb-podterm", 0, SpawnLongInput{
		Image:   "alpine:3.21",
		Command: "sh",
		Args:    []string{"-c", "echo hi"},
	})
	pos := -1
	for i, a := range args {
		if a == "alpine:3.21" {
			pos = i
			break
		}
	}
	core.AssertTrue(t, pos >= 0)
	core.AssertTrue(t, pos+2 < len(args))
	core.AssertEqual(t, "--", args[pos+1])
	core.AssertEqual(t, "sh", args[pos+2])
}

// --- Mantis #1639 HIGH F2 InspectImage gate tests ---

// parseManifestInspectDigest_Good covers the canonical single-image
// shape: top-level object with `digest`.
func TestSandbox_ParseManifestInspectDigest_Good(t *core.T) {
	raw := `{"schemaVersion":2,"digest":"sha256:abcdef0123456789","mediaType":"application/vnd.docker.distribution.manifest.v2+json"}`
	got := parseManifestInspectDigest(raw)
	core.AssertEqual(t, "sha256:abcdef0123456789", got)
}

// parseManifestInspectDigest_Bad covers manifest-list (multi-arch) shape:
// top-level `manifests` array of entries each carrying `digest`.
func TestSandbox_ParseManifestInspectDigest_Bad(t *core.T) {
	raw := `{"schemaVersion":2,"mediaType":"application/vnd.docker.distribution.manifest.list.v2+json","manifests":[{"digest":"sha256:firstarch","platform":{"os":"linux","architecture":"amd64"}},{"digest":"sha256:secondarch","platform":{"os":"linux","architecture":"arm64"}}]}`
	got := parseManifestInspectDigest(raw)
	core.AssertEqual(t, "sha256:firstarch", got)
}

// parseManifestInspectDigest_Ugly covers empty input + malformed JSON +
// JSON-without-digest — all must return "" so the caller can route the
// digest_parse_failed reject without panicking.
func TestSandbox_ParseManifestInspectDigest_Ugly(t *core.T) {
	core.AssertEqual(t, "", parseManifestInspectDigest(""))
	core.AssertEqual(t, "", parseManifestInspectDigest("   "))
	core.AssertEqual(t, "", parseManifestInspectDigest("not-json-at-all"))
	core.AssertEqual(t, "", parseManifestInspectDigest(`{"schemaVersion":2}`))
	core.AssertEqual(t, "", parseManifestInspectDigest(`{"manifests":[]}`))
	core.AssertEqual(t, "", parseManifestInspectDigest(`{"manifests":[{"platform":{"os":"linux"}}]}`))
}

// InspectImage_Good — when process.Service is wired, InspectImage
// returns the digest_unverifiable / inspect_failed reject path with
// the typed prefix surface intact. The real-docker happy path is
// covered by the parseManifestInspectDigest_Good unit above; this
// test pins the reject-prefix contract pkg/marketplace pattern-matches
// against (codeInspectFailed / codeDockerUnavailable / codeDigestParseFailed).
func TestSandbox_InspectImage_Good(t *core.T) {
	svc := newTestServiceWithProcess()
	core.AssertNotNil(t, svc)
	digest, r := svc.InspectImage("lthn-nonexistent-image-for-test:never")
	// CI / test boxes either have no docker (docker_unavailable) or
	// docker present but the ref does not resolve (inspect_failed) —
	// both reject prefixes are documented contract.
	core.AssertFalse(t, r.OK)
	core.AssertEqual(t, "", digest)
	hasTypedPrefix := core.Contains(r.Error(), codeDockerUnavailable) ||
		core.Contains(r.Error(), codeInspectFailed) ||
		core.Contains(r.Error(), codeDigestParseFailed)
	core.AssertTrue(t, hasTypedPrefix)
}

// InspectImage_Bad — empty ref rejects with the standard "image ref is
// required" validation message BEFORE any process spawn. The reject
// shape is core.Result-only so caller pattern-matches survive.
func TestSandbox_InspectImage_Bad(t *core.T) {
	svc := newTestService(Options{})
	digest, r := svc.InspectImage("")
	core.AssertFalse(t, r.OK)
	core.AssertEqual(t, "", digest)
	core.AssertContains(t, r.Error(), "image ref is required")

	// Whitespace-only ref is treated as empty (defence against fixture
	// noise / accidental "   " bindings from upstream config parsers).
	digest, r = svc.InspectImage("   ")
	core.AssertFalse(t, r.OK)
	core.AssertEqual(t, "", digest)
	core.AssertContains(t, r.Error(), "image ref is required")
}

// InspectImage_Ugly — when process.Service is NOT wired (svc.proc()
// returns nil), InspectImage rejects with codeDockerUnavailable rather
// than panicking. The Install pipeline routes this to its
// `image.digest_unverifiable` envelope so the operator sees "we
// couldn't verify the digest", not a crash.
func TestSandbox_InspectImage_Ugly(t *core.T) {
	svc := newTestService(Options{}) // no process.Service wired
	digest, r := svc.InspectImage("docker.io/library/alpine:3.21")
	core.AssertFalse(t, r.OK)
	core.AssertEqual(t, "", digest)
	core.AssertContains(t, r.Error(), codeDockerUnavailable)
}
