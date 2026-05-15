// SPDX-Licence-Identifier: EUPL-1.2

// Long-running container surface for lthn-vm bundles. Complements
// sandbox.go's one-shot Spawn with a lifecycle-managed SpawnLong +
// Kill + List + Get surface.
//
// SpawnLong starts a container detached, assigns a stable SandboxID
// (prefix "sb-"), polls for readiness, and stores the ContainerHandle
// in an in-memory registry keyed by SandboxID. Kill stops the container
// and removes the handle. List / Get expose the registry to callers.
//
// The registry is in-memory only — no orm persistence. Bundle
// lifecycle (install/launch/stop) in pkg/marketplace/install.go
// holds the durable state; the sandbox registry tracks what's
// currently running in this process lifetime.
//
// Spec: plans/project/lthn/desktop/RFC.marketplace.md §5 + RFC.opencode.md §4.
package sandbox

import (
	"net"

	core "dappco.re/go"
)

const (
	// sandboxIDPrefix is the stable identifier prefix for bundle sandboxes.
	// Distinct from opencode's "oc-" prefix so callers can distinguish.
	sandboxIDPrefix = "sb-"

	spawnLongOp = "sandbox.SpawnLong"
	killOp      = "sandbox.Kill"
	getOp       = "sandbox.Get"

	// defaultReadyTimeout is how long SpawnLong waits for the container
	// to open its exposed port before declaring failure.
	defaultReadyTimeout = 30 * core.Second
)

// SpawnLongInput drives SpawnLong. All fields are optional except
// Image and Command — the container must have something to run.
type SpawnLongInput struct {
	// Image is the OCI ref to run (e.g. "lthn/dev:latest").
	Image string

	// Command is the entrypoint binary.
	Command string

	// Args are additional arguments passed to Command.
	Args []string

	// Env is environment variables injected into the container.
	Env map[string]string

	// ExposedPort is the container port to probe for readiness.
	// 0 means no readiness check (container is assumed ready immediately).
	ExposedPort int

	// NetworkName is the Docker network the container joins.
	// Empty means the default bridge network.
	NetworkName string

	// Volumes is a list of named persistent volumes to mount.
	Volumes []LongVolumeMount

	// Runtime overrides auto-detected runtime ("docker", "podman").
	// Empty = auto-detect from host.
	Runtime string
}

// LongVolumeMount maps a named host volume to a container path.
type LongVolumeMount struct {
	Name      string // named Docker volume
	Container string // mount path inside the container
}

// ContainerHandle is the runtime record for one running bundle container.
type ContainerHandle struct {
	// SandboxID is the stable lthn-assigned identifier: "sb-<8-char-random>".
	// Used in /v1/api/sandbox/<id>/* routing.
	SandboxID string `json:"sandbox_id"`

	// Image is the OCI ref the container was started from.
	Image string `json:"image"`

	// Command is the entrypoint command.
	Command string `json:"command"`

	// Status is the last known state: starting | ready | stopped | failed.
	Status string `json:"status"`

	// HostPort is the dynamically-allocated host port mapped to ExposedPort.
	// 0 when ExposedPort was 0 (no port mapping).
	HostPort int `json:"host_port,omitempty"`

	// BundleID links this handle to the owning bundle (if spawned via install.go).
	// Empty for standalone SpawnLong calls.
	BundleID string `json:"bundle_id,omitempty"`

	// StartedAt is the spawn timestamp.
	StartedAt core.Time `json:"started_at"`
}

// Canonical status strings for ContainerHandle.
const (
	StatusStarting = "starting"
	StatusReady    = "ready"
	StatusStopped  = "stopped"
	StatusFailed   = "failed"
)

// SpawnLong starts a container detached, assigns a stable SandboxID,
// polls readiness (when ExposedPort > 0), and registers the handle.
// Returns the populated ContainerHandle on success.
//
// Usage example:
//
//	r := svc.SpawnLong(sandbox.SpawnLongInput{
//	    Image:       "lthn/dev:latest",
//	    Command:     "opencode",
//	    Args:        []string{"web", "--hostname", "0.0.0.0", "--port", "4096"},
//	    ExposedPort: 4096,
//	})
//	if r.OK { h := r.Value.(sandbox.ContainerHandle); _ = h.SandboxID }
func (s *Service) SpawnLong(input SpawnLongInput) core.Result {
	if core.Trim(input.Image) == "" {
		return core.Fail(core.E(spawnLongOp, "image is required", nil))
	}
	if core.Trim(input.Command) == "" {
		return core.Fail(core.E(spawnLongOp, "command is required", nil))
	}

	ps := s.proc()
	if ps == nil {
		return core.Fail(core.E(spawnLongOp, "process service unavailable", nil))
	}

	idR := core.RandomString(8)
	if !idR.OK {
		return core.Fail(core.E(spawnLongOp, "id generation failed", nil))
	}
	sandboxID := sandboxIDPrefix + idR.Value.(string)
	containerName := "lthn-sandbox-" + sandboxID

	hostPort := 0
	if input.ExposedPort > 0 {
		portR := allocateFreePort()
		if !portR.OK {
			return portR
		}
		hostPort = portR.Value.(int)
	}

	rt := s.resolveRuntimeName(input.Runtime)
	args := s.buildLongRunArgs(rt, containerName, hostPort, input)

	ctx, cancel := core.WithTimeout(core.Background(), 30*core.Second)
	defer cancel()

	runR := ps.Run(ctx, rt, args...)
	if !runR.OK {
		return core.Fail(core.E(spawnLongOp, "container start failed", nil))
	}

	handle := ContainerHandle{
		SandboxID: sandboxID,
		Image:     input.Image,
		Command:   input.Command,
		Status:    StatusStarting,
		HostPort:  hostPort,
		StartedAt: core.Now(),
	}

	// Readiness check — poll until the container's exposed port accepts
	// connections, or the timeout fires.
	if hostPort > 0 {
		if r := waitPortOpen(hostPort, defaultReadyTimeout); !r.OK {
			// Best-effort cleanup.
			ps.Run(core.Background(), rt, "rm", "-f", containerName)
			return core.Fail(core.E(spawnLongOp, "container did not become ready", nil))
		}
	}

	handle.Status = StatusReady

	s.mu.Lock()
	if s.handles == nil {
		s.handles = map[string]*ContainerHandle{}
	}
	s.handles[sandboxID] = &handle
	s.mu.Unlock()

	return core.Ok(handle)
}

// Kill stops the container for a given SandboxID and removes its handle.
//
// Usage example:
//
//	r := svc.Kill("sb-a1b2c3d4")
//	if r.OK { /* container stopped */ }
func (s *Service) Kill(sandboxID string) core.Result {
	if core.Trim(sandboxID) == "" {
		return core.Fail(core.E(killOp, "sandbox id is required", nil))
	}

	s.mu.Lock()
	handle, ok := s.handles[sandboxID]
	if ok {
		handle.Status = StatusStopped
		delete(s.handles, sandboxID)
	}
	s.mu.Unlock()

	containerName := "lthn-sandbox-" + sandboxID
	ps := s.proc()
	if ps != nil {
		rt := s.resolveRuntimeName("")
		// docker rm -f stops + removes in one shot; ignore failure since
		// the container may already be gone.
		ps.Run(core.Background(), rt, "rm", "-f", containerName)
	}

	if !ok {
		return core.Fail(core.E(killOp, "sandbox not found: "+sandboxID, nil))
	}
	return core.Ok(nil)
}

// ListHandles returns all currently-registered ContainerHandles.
//
// Usage example:
//
//	r := svc.ListHandles()
//	handles := r.Value.([]sandbox.ContainerHandle)
func (s *Service) ListHandles() core.Result {
	s.mu.RLock()
	out := make([]ContainerHandle, 0, len(s.handles))
	for _, h := range s.handles {
		out = append(out, *h)
	}
	s.mu.RUnlock()
	return core.Ok(out)
}

// GetHandle returns the ContainerHandle for a given SandboxID.
//
// Usage example:
//
//	r := svc.GetHandle("sb-a1b2c3d4")
//	if r.OK { h := r.Value.(sandbox.ContainerHandle); _ = h.HostPort }
func (s *Service) GetHandle(sandboxID string) core.Result {
	if core.Trim(sandboxID) == "" {
		return core.Fail(core.E(getOp, "sandbox id is required", nil))
	}
	s.mu.RLock()
	h, ok := s.handles[sandboxID]
	s.mu.RUnlock()
	if !ok {
		return core.Fail(core.E(getOp, "sandbox not found: "+sandboxID, nil))
	}
	return core.Ok(*h)
}

// buildLongRunArgs constructs the `docker run -d ...` arguments for
// a long-running bundle container.
func (s *Service) buildLongRunArgs(rt, containerName string, hostPort int, input SpawnLongInput) []string {
	args := []string{"run", "-d", "--name", containerName}

	if hostPort > 0 {
		args = append(args, "-p",
			core.Sprintf("127.0.0.1:%d:%d", hostPort, input.ExposedPort))
	}

	for k, v := range input.Env {
		args = append(args, "-e", k+"="+v)
	}

	for _, vol := range input.Volumes {
		args = append(args, "-v", vol.Name+":"+vol.Container)
	}

	if input.NetworkName != "" {
		args = append(args, "--network", input.NetworkName)
	}

	args = append(args, input.Image, input.Command)
	args = append(args, input.Args...)
	return args
}

// resolveRuntimeName returns the effective runtime name string.
// Mirrors the string-based CLI dispatch used by spawnViaCLI.
func (s *Service) resolveRuntimeName(prefer string) string {
	if core.Trim(prefer) != "" {
		return prefer
	}
	r := s.resolveRuntime("")
	if !r.OK {
		return "docker"
	}
	rt := r.Value
	return core.Sprintf("%v", rt)
}

// allocateFreePort grabs a free host port by listening on 127.0.0.1:0
// then closing — same approach as opencode.allocatePort.
func allocateFreePort() core.Result {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return core.Fail(core.E("sandbox.allocateFreePort", "listen failed", err))
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	return core.Ok(port)
}

// waitPortOpen polls until 127.0.0.1:port accepts a TCP connection or
// the timeout fires. Used to confirm a container's service is bound.
func waitPortOpen(port int, timeout core.Duration) core.Result {
	deadline := core.Now().Add(timeout)
	addr := core.Sprintf("127.0.0.1:%d", port)
	for core.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 500*core.Millisecond)
		if err == nil {
			_ = conn.Close()
			return core.Ok(nil)
		}
		core.Sleep(500 * core.Millisecond)
	}
	return core.Fail(core.E("sandbox.waitPortOpen",
		core.Sprintf("port %d did not open within %s", port, timeout), nil))
}
