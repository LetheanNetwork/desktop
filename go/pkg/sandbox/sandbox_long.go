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

	// InstallIDLabel is the docker label key SpawnLong stamps when
	// SpawnLongInput.InstallID is non-empty. Parallels
	// opencode.InstallIDLabel ("lthn.opencode.install_id"); a future
	// sandbox-reconcile consumer can gate adoption on this label to
	// avoid attaching to look-alike sibling-user containers
	// (Cerberus Mantis #1665 S-3, modelled on Mantis #1599).
	InstallIDLabel = "lthn.sandbox.install_id"

	// BundleIDLabel is the docker label key SpawnLong stamps when
	// SpawnLongInput.BundleID is non-empty. Lets a reconcile /
	// observability consumer map a running container back to the
	// owning marketplace bundle without re-parsing the manifest.
	BundleIDLabel = "lthn.sandbox.bundle_id"
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

	// InstallID is the per-install identifier stamped as the
	// "lthn.sandbox.install_id" docker label. Cerberus Mantis #1665
	// (S-3) — without this label, SpawnLong containers lack the
	// adoption-gate identifier that opencode containers carry via
	// "lthn.opencode.install_id" (Mantis #1599). A future
	// reconcile consumer can use the label to gate
	// adoption + filter out look-alike sibling containers.
	// Empty = label is omitted (callers that pre-date the wiring).
	InstallID string

	// BundleID is the marketplace bundle identifier stamped as the
	// "lthn.sandbox.bundle_id" docker label. Lets a future reconcile
	// / observability consumer map a running container back to the
	// installed bundle without re-parsing the manifest. Empty =
	// label is omitted (standalone SpawnLong outside marketplace).
	BundleID string
}

// LongVolumeMount maps a named host volume to a container path.
type LongVolumeMount struct {
	Name      string // named Docker volume
	Container string // mount path inside the container
}

// IsValidContainerPath reports whether p is a syntactically-valid
// container-side mount path. Cerberus Mantis #1446 — `vol.Container`
// flows into `-v <name>:<container>` and Docker parses everything
// after the colon as the container path, BUT a malicious manifest
// could declare `Container: "/data:ro,bind,private"` to inject mount
// options into the docker-run argument vector. The IsValidVolumeName
// gate (#1431) catches the host-side, this catches the container-side.
//
// Rules:
//   - Non-empty, length 1..256 (longer than a real container path).
//   - Starts with `/` (absolute container path).
//   - Contains no `:` (the docker `-v` separator and mount-option key).
//   - Contains no `,` (mount-option separator).
//   - Contains no NUL or control characters.
//   - Contains no whitespace.
//
// Backslash + path-traversal in the container path itself are NOT
// rejected — they're container-internal semantics, not a host attack.
// The host gate is IsValidVolumeName.
func IsValidContainerPath(p string) bool {
	if p == "" || len(p) > 256 {
		return false
	}
	if p[0] != '/' {
		return false
	}
	for _, ch := range p {
		if ch == 0 || ch < 32 {
			return false
		}
		if ch == ':' || ch == ',' || ch == ' ' || ch == '\t' {
			return false
		}
	}
	return true
}

// IsValidVolumeName reports whether name is a syntactically-valid
// named Docker volume. Cerberus Mantis #1431 — without this gate, a
// bundle manifest could declare VolumeMount{Persist: "/var/run/
// docker.sock"} which buildLongRunArgs would render as
// `-v /var/run/docker.sock:/sock` — a host-path bind-mount that
// gives the container access to the docker socket, which is
// root-equivalent on the host (classic Docker escape).
//
// Rules (mirror Docker's own volume-name regex):
//   - Length 1..64.
//   - First character: ASCII alphanumeric.
//   - Remaining characters: alphanumeric or `_`, `.`, `-`.
//   - NO leading `/`, `.`, or `-` (rejected by the first-char rule);
//     in particular no `/` ANYWHERE (so absolute paths can't masquerade
//     as named volumes).
//
// Usage example:
//
//	if !sandbox.IsValidVolumeName(v.Name) {
//	    return core.Fail(core.E(op, "invalid volume name: "+v.Name, nil))
//	}
func IsValidVolumeName(name string) bool {
	if name == "" || len(name) > 64 {
		return false
	}
	for i, ch := range name {
		alnum := ('a' <= ch && ch <= 'z') ||
			('A' <= ch && ch <= 'Z') ||
			('0' <= ch && ch <= '9')
		if i == 0 {
			if !alnum {
				return false
			}
			continue
		}
		if !(alnum || ch == '_' || ch == '.' || ch == '-') {
			return false
		}
	}
	return true
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

// hardenedDefaults is the conservative set of `docker run` security
// flags applied to every long-running bundle container. Cerberus
// Mantis #1434 — the previous default was wide-open (every container
// ran with default Docker caps, no PIDs cap, no setuid block). This
// list closes the obvious privilege-escalation + fork-bomb attack
// surface.
//
// Per-flag rationale:
//
//   - `--cap-drop=ALL` — drop every Linux capability. Bundles that
//     need a specific one back (e.g. CAP_CHOWN for postgres) will
//     surface a clear "operation not permitted" failure that the
//     bundle author can address by adding a `caps:` entry to the
//     manifest (future Mantis when we wire it).
//   - `--security-opt=no-new-privileges` — block setuid + capability
//     escalation INSIDE the container. Defence-in-depth on top of
//     cap-drop=ALL.
//   - `--pids-limit=512` — cap concurrent processes. Prevents fork
//     bombs from a compromised bundle from exhausting host PIDs.
//     512 is generous for an inference daemon (typical: 10-50);
//     bundles that need more (multi-worker servers) can request
//     a higher cap via manifest later.
//
// Future hardening (separate Mantis):
//   - `--user $(id -u):$(id -g)` — currently skipped because many
//     OCI images assume root inside the container for setup. Requires
//     a manifest opt-in / per-bundle override.
//   - `--read-only` — root FS read-only; bundles get a tmpfs for
//     scratch. Same reason as --user: needs per-bundle declaration.
//   - `--network=lthn-bundles` (already partly there via
//     NetworkName) — every bundle on a single private network,
//     no host network access by default.
var hardenedDefaults = []string{
	"--cap-drop=ALL",
	"--security-opt=no-new-privileges",
	"--pids-limit=512",
}

// buildLongRunArgs constructs the `docker run -d ...` arguments for
// a long-running bundle container.
func (s *Service) buildLongRunArgs(rt, containerName string, hostPort int, input SpawnLongInput) []string {
	args := []string{"run", "-d", "--name", containerName}
	args = append(args, hardenedDefaults...)

	// Cerberus Mantis #1665 (S-3) — adoption-gate labels. Empty values
	// are omitted so pre-wiring callers (and standalone SpawnLong outside
	// marketplace) keep working unchanged. A future sandbox reconcile
	// consumer filters on these labels the way opencode.Reconcile filters
	// on InstallIDLabel (Mantis #1599).
	if core.Trim(input.InstallID) != "" {
		args = append(args, "--label", InstallIDLabel+"="+input.InstallID)
	}
	if core.Trim(input.BundleID) != "" {
		args = append(args, "--label", BundleIDLabel+"="+input.BundleID)
	}

	if hostPort > 0 {
		args = append(args, "-p",
			core.Sprintf("127.0.0.1:%d:%d", hostPort, input.ExposedPort))
	}

	for k, v := range input.Env {
		args = append(args, "-e", k+"="+v)
	}

	// Cerberus #1431 — defence-in-depth gate. Marketplace's
	// resolveVolumes is the primary validator (and errors the install
	// loudly); the silent-skip here protects the sandbox from any
	// future caller that bypasses marketplace and constructs a
	// SpawnLongInput directly with an attacker-controlled volume name.
	for _, vol := range input.Volumes {
		if !IsValidVolumeName(vol.Name) {
			core.Warn("sandbox: refusing volume with invalid name",
				"name", vol.Name, "container", vol.Container)
			continue
		}
		if !IsValidContainerPath(vol.Container) {
			core.Warn("sandbox: refusing volume with invalid container path (Mantis #1446)",
				"name", vol.Name, "container", vol.Container)
			continue
		}
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
