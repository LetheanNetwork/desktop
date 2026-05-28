// SPDX-Licence-Identifier: EUPL-1.2

package fleet

import (
	"context"
	"net"
	"time"

	core "dappco.re/go"
	process "dappco.re/go/process"
)

// Crew supervision — the local (is_self) machine spawns and supervises
// its crew sidecars: the lthn binary family that serves model inference
// and agentic dispatch over loopback. Supervision is a machine option,
// driven by the machine's capabilities — a member runs only when the
// machine declares the matching capability:
//
//	CapabilityInference -> lthn-mlx   serve --addr :PORT  (OpenAI /v1)
//	CapabilitySandbox   -> lthn-agent serve               (joins once its
//	                                                        listen addr is wired)
//
// Each instance is health-gated (not "up" until its port accepts a
// connection) and respawned with bounded backoff if it exits. A member
// with Count > 1 runs N instances on consecutive ports from BasePort —
// the path for lthn-ai managing a fleet of lthn-agent workers.

const (
	crewHealthDialTimeout = 2 * time.Second
	crewHealthDeadline    = 30 * time.Second
	crewBackoffMin        = 1 * time.Second
	crewBackoffMax        = 30 * time.Second
	// crewBinDirEnv overrides where crew binaries are resolved from (a
	// directory). Dev convenience; production resolves the bundled
	// sidecars next to the lthn executable (see resolveCrewBinary).
	crewBinDirEnv = "LTHN_CREW_BIN_DIR"
)

// crewMember declares one supervised sidecar and the capability that
// switches it on for a given machine.
type crewMember struct {
	Capability string   // machine capability that enables this member
	Binary     string   // sidecar binary name (resolved on disk)
	Serve      []string // serve subcommand + static args; AddrFlag :PORT is appended
	AddrFlag   string   // CLI flag for the listen address (lthn-mlx --addr :PORT)
	AddrEnv    string   // OR env var for the listen address (lthn-agent MCP_HTTP_ADDR=127.0.0.1:PORT) — exclusive with AddrFlag
	ModelFlag  string   // if set, the binary requires ModelFlag <path>; the
	//             member is skipped until a model path is resolvable
	BasePort int // instance i listens on BasePort + i
	Count    int // instances to run (clamped to >= 1)
}

// defaultCrew is the darwin crew. lthn-mlx is the inference sidecar
// (--addr :PORT, mirroring Ollama's 11434). lthn-agent is the sandbox
// sidecar — CoreAgent's orchestration engine, whose listen address goes
// via MCP_HTTP_ADDR (env, not a flag) and which serves the Streamable
// HTTP MCP at /mcp + the /v1/tools BridgeToAPI on :9101 (what the
// desktop's pkg/agents drives). The machinery handles both members.
func defaultCrew() []crewMember {
	return []crewMember{
		{
			Capability: CapabilityInference,
			Binary:     "lthn-mlx",
			Serve:      []string{"serve"},
			AddrFlag:   "--addr",
			ModelFlag:  "--model", // serve requires a model; skipped until one resolves
			BasePort:   11434,
			Count:      1,
		},
		{
			Capability: CapabilitySandbox,
			Binary:     "lthn-agent",
			Serve:      []string{"serve"},
			AddrEnv:    "MCP_HTTP_ADDR",
			BasePort:   9101,
			Count:      1,
		},
	}
}

// resolveCrewBinary finds a crew binary: an explicit dev override dir,
// the per-user ~/Lethean/bin install, then PATH (which a packaged .app
// satisfies via Contents/MacOS). Mirrors pkg/calibrate's resolver so the
// GUI, the bundle, and the supervisor all agree on which binary runs.
func resolveCrewBinary(name string) string {
	home := core.Env("HOME")
	dirs := []string{}
	if override := core.Trim(core.Env(crewBinDirEnv)); override != "" {
		dirs = append(dirs, override)
	}
	dirs = append(dirs,
		core.PathJoin(home, "Lethean", "bin"),
		core.PathJoin(home, "Code", "lthn", "desktop", "build", "darwin", "bin"),
	)
	for _, d := range dirs {
		candidate := core.PathJoin(d, name)
		if core.Stat(candidate).OK {
			return candidate
		}
	}
	return name // let the OS resolve via PATH (packaged .app: Contents/MacOS)
}

// crewInstance is one running sidecar process under supervision.
type crewInstance struct {
	binary string
	addr   string // host:port the instance listens on
	procID string // go-process id, for Kill on shutdown
}

// crewSupervisor owns the running crew for the local machine. Spawn,
// health-gate, respawn-on-crash, stop.
type crewSupervisor struct {
	mu        core.Mutex
	stopCh    chan struct{}
	stopped   bool
	instances []*crewInstance
}

// superviseCrew spawns every member whose capability the machine holds,
// Count instances each, on consecutive loopback ports from BasePort.
// Health-gates each instance, then leaves a respawn goroutine watching
// it. Returns once all enabled instances are up (or their health
// deadline lapses — a slow sidecar doesn't block boot, the respawn loop
// keeps trying).
func superviseCrew(ctx context.Context, crew []crewMember, capabilities []string) *crewSupervisor {
	sup := &crewSupervisor{stopCh: make(chan struct{})}
	caps := map[string]bool{}
	for _, c := range capabilities {
		caps[c] = true
	}
	for _, m := range crew {
		if !caps[m.Capability] {
			continue
		}
		count := m.Count
		if count < 1 {
			count = 1
		}
		for i := 0; i < count; i++ {
			sup.launch(ctx, m, i)
		}
	}
	return sup
}

// spawnMember starts one crew-member process on the given port. The listen
// address is passed via AddrFlag (a CLI flag — lthn-mlx --addr :PORT) or
// AddrEnv (an env var — lthn-agent MCP_HTTP_ADDR=127.0.0.1:PORT); the two
// are exclusive. Env spawns go through go-process StartWithOptions (which
// the boot-time process.Init wired); flag spawns keep the plain Start path.
func spawnMember(ctx context.Context, m crewMember, port int) core.Result {
	args := append([]string(nil), m.Serve...)
	if m.AddrFlag != "" {
		args = append(args, m.AddrFlag, core.Sprintf(":%d", port))
	}
	bin := resolveCrewBinary(m.Binary)
	if m.AddrEnv == "" {
		return process.Start(ctx, bin, args...)
	}
	return process.StartWithOptions(ctx, process.RunOptions{
		Command: bin,
		Args:    args,
		Env:     []string{core.Sprintf("%s=127.0.0.1:%d", m.AddrEnv, port)},
	})
}

// launch resolves the binary, spawns one instance, health-gates it, and
// starts its respawn watcher.
func (sup *crewSupervisor) launch(ctx context.Context, m crewMember, idx int) {
	port := m.BasePort + idx
	addr := core.Sprintf("127.0.0.1:%d", port)
	// Adopt, don't fight: if something is already serving this port (the
	// user started lthn-mlx by hand, or a prior session left it up), skip
	// the spawn rather than crash-loop against a bound port.
	if crewQuickUp(addr) {
		core.Info("fleet.crew: port already serving — adopting, not supervising", "binary", m.Binary, "addr", addr)
		return
	}
	startR := spawnMember(ctx, m, port)
	if !startR.OK {
		core.Warn("fleet.crew: spawn failed", "binary", m.Binary, "addr", addr, "err", startR.Error())
		return
	}
	inst := &crewInstance{binary: m.Binary, addr: addr, procID: crewProcID(startR)}

	sup.mu.Lock()
	sup.instances = append(sup.instances, inst)
	sup.mu.Unlock()

	go sup.superviseInstance(ctx, m, idx, inst)
}

// superviseInstance runs in its own goroutine (so the caller never
// blocks on boot): it health-gates the instance, then blocks until the
// process exits and respawns it with bounded backoff — unless the
// supervisor is stopping. Backoff resets after a run that stayed up past
// crewBackoffMax (a healthy run, not a crash loop).
func (sup *crewSupervisor) superviseInstance(ctx context.Context, m crewMember, idx int, inst *crewInstance) {
	if crewHealthy(inst.addr) {
		core.Info("fleet.crew: up", "binary", m.Binary, "addr", inst.addr)
	} else {
		core.Warn("fleet.crew: health deadline lapsed, respawn loop will keep trying", "binary", m.Binary, "addr", inst.addr)
	}
	backoff := crewBackoffMin
	for {
		startedAt := time.Now()
		process.Wait(inst.procID) // blocks until the process exits

		sup.mu.Lock()
		stopped := sup.stopped
		sup.mu.Unlock()
		if stopped {
			return
		}
		if time.Since(startedAt) > crewBackoffMax {
			backoff = crewBackoffMin // the crashed run had been healthy; reset
		}
		core.Warn("fleet.crew: exited, respawning", "binary", m.Binary, "addr", inst.addr, "backoff", backoff.String())
		select {
		case <-sup.stopCh:
			return
		case <-time.After(backoff):
		}
		if backoff < crewBackoffMax {
			backoff *= 2
		}

		port := m.BasePort + idx
		startR := spawnMember(ctx, m, port)
		if !startR.OK {
			core.Warn("fleet.crew: respawn failed", "binary", m.Binary, "err", startR.Error())
			continue
		}
		inst.procID = crewProcID(startR)
		crewHealthy(inst.addr)
	}
}

// stop signals the respawn loops to quit and kills every running
// instance. Idempotent.
func (sup *crewSupervisor) stop() {
	if sup == nil {
		return
	}
	sup.mu.Lock()
	if sup.stopped {
		sup.mu.Unlock()
		return
	}
	sup.stopped = true
	close(sup.stopCh)
	instances := append([]*crewInstance(nil), sup.instances...)
	sup.mu.Unlock()
	for _, inst := range instances {
		if inst.procID != "" {
			_ = process.Kill(inst.procID)
		}
	}
}

// crewHealthy polls a TCP connect to addr until it succeeds or the
// health deadline lapses. A listening socket is the readiness proxy —
// lthn-mlx serves /v1 the moment it binds. Returns true once up.
func crewHealthy(addr string) bool {
	deadline := time.Now().Add(crewHealthDeadline)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, crewHealthDialTimeout)
		if err == nil {
			_ = conn.Close()
			return true
		}
		time.Sleep(500 * time.Millisecond)
	}
	return false
}

// crewQuickUp is a single short dial — "is something already listening
// here?" — used before spawning so we adopt a sidecar that's already up
// instead of fighting for the port.
func crewQuickUp(addr string) bool {
	conn, err := net.DialTimeout("tcp", addr, crewHealthDialTimeout)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// crewProcID extracts the go-process id from a Start result, whether the
// Value is the *process.Process or its id string.
func crewProcID(r core.Result) string {
	switch v := r.Value.(type) {
	case *process.Process:
		return v.ID
	case string:
		return v
	default:
		return ""
	}
}
