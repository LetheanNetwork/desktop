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
		"--memory", "2048M",
		"--cpus", "4",
		"--storage-opt", "size=10G",
		"alpine:3.21", "echo", "hi",
	}, run.Args)
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
