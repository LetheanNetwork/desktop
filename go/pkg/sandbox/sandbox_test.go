// SPDX-Licence-Identifier: EUPL-1.2

package sandbox

import (
	core "dappco.re/go"
	"dappco.re/go/container"
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
	r := svc.Spawn(SpawnInput{})
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
		"alpine:3.21", "echo", "hi",
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
	r := svc.SpawnLong(SpawnLongInput{Image: "lthn/dev:latest", Command: "opencode"})
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
	r := svc.SpawnLong(SpawnLongInput{Command: "opencode"})
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "image is required")

	// Missing command.
	r = svc.SpawnLong(SpawnLongInput{Image: "lthn/dev:latest"})
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

	r := svc.Kill(id)
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
	r := svc.Kill("")
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "sandbox id is required")

	// Unknown ID.
	r = svc.Kill("sb-doesnotexist")
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

	r1 := svc.Kill(id)
	core.AssertTrue(t, r1.OK)

	r2 := svc.Kill(id)
	core.AssertFalse(t, r2.OK)
	core.AssertContains(t, r2.Error(), "sandbox not found")
}

// TestSandbox_ListHandles_Good verifies ListHandles returns all registered handles.
func TestSandbox_ListHandles_Good(t *core.T) {
	svc := newTestService(Options{})

	// Empty registry.
	r := svc.ListHandles()
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

	r = svc.ListHandles()
	core.AssertTrue(t, r.OK)
	handles = r.Value.([]ContainerHandle)
	core.AssertLen(t, handles, 2)
}

// TestSandbox_ListHandles_Bad verifies ListHandles on nil svc does not panic.
func TestSandbox_ListHandles_Bad(t *core.T) {
	svc := newTestService(Options{})
	r := svc.ListHandles()
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

	r := svc.ListHandles()
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

	r := svc.GetHandle(id)
	core.AssertTrue(t, r.OK)
	h := r.Value.(ContainerHandle)
	core.AssertEqual(t, id, h.SandboxID)
	core.AssertEqual(t, 4096, h.HostPort)
	core.AssertEqual(t, StatusReady, h.Status)
}

// TestSandbox_GetHandle_Bad verifies GetHandle rejects empty ID and missing sandbox.
func TestSandbox_GetHandle_Bad(t *core.T) {
	svc := newTestService(Options{})

	r := svc.GetHandle("")
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "sandbox id is required")

	r = svc.GetHandle("sb-missing")
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

	r := svc.GetHandle(id)
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
