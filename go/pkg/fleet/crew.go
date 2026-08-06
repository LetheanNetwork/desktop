// SPDX-Licence-Identifier: EUPL-1.2

package fleet

import (
	"context"
	"net"
	"time"

	core "dappco.re/go"
	process "dappco.re/go/process"
	terminal "dappco.re/lthn/desktop/pkg/terminal"
)

// Crew supervision — the local (is_self) machine spawns and supervises
// its crew sidecars: the lthn binary family that serves model inference
// and agentic dispatch over loopback. Supervision is a machine option,
// driven by the machine's capabilities — a member runs only when the
// machine declares the matching capability:
//
//	CapabilityInference -> lthn-ai      (the LEM host; auto-serves + supervises the
//	                                     mlx driver, OpenAI /v1 chat on :9100)
//	CapabilitySandbox   -> lthn-agent hub (HTTP control plane on :9201,
//	                                       MCP HTTP+SSE plane on :9202)
//
// Each instance is health-gated (not "up" until its port accepts a
// connection) and respawned with bounded backoff if it exits. A member
// with Count > 1 runs N instances on consecutive ports from BasePort —
// the path for lthn-ai managing a fleet of lthn-agent workers.

// crewHealthDialTimeout, crewHealthDeadline, crewHealthPollInterval, and the
// respawn backoff bounds are vars (not const) purely as a testability seam:
// crew_test.go shrinks them for fast, deterministic health-gate + respawn
// tests instead of a real 30s deadline / 1s-32s backoff ladder. Production
// never reassigns them — the values below are the real defaults.
var (
	crewHealthDialTimeout  = 2 * time.Second
	crewHealthDeadline     = 30 * time.Second
	crewHealthPollInterval = 500 * time.Millisecond
	crewBackoffMin         = 1 * time.Second
	crewBackoffMax         = 30 * time.Second
)

const (
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
	Env        []string // extra static KEY=VALUE env for the spawn (merged with the AddrEnv var)
	ModelFlag  string   // if set, the binary requires ModelFlag <path>; the
	//             member is skipped until a model path is resolvable
	BasePort int  // instance i listens on BasePort + i
	Count    int  // instances to run (clamped to >= 1)
	Watch    bool // PTY-back this member via terminal.Spawn so its output is a
	//             watchable in-app terminal session (registered in the pool).
	//             Default false: infra members keep the plain process.Start path.
}

// defaultCrew returns the darwin crew definition. lthn-ai is the inference
// host — it serves the OpenAI /v1 surface on :9100 (CORE_AI_HTTP_ADDR) and
// on boot auto-serves + supervises the mlx driver itself
// (CORE_AI_SERVE_RUNTIME=mlx), so the desktop supervises lthn-ai and
// lthn-ai supervises the driver. Its MCP socket is moved off the shared
// default so it can't collide.
//
// lthn-agent is the sandbox sidecar running `hub` (Mantis #1807 Unit D).
// The hub exposes two planes:
//   - HTTP control plane on :9201 (bearer-required, health-gate target)
//   - MCP HTTP+SSE plane on :9202 (fail-closed: requires MCP_JWT_SECRET)
//
// sandboxEnv carries MCP_JWT_SECRET and MCP_AUTH_TOKEN so both planes start;
// the caller (desktop startup) resolves these from pkg/keys tier-0 before
// invoking SuperviseLocalCrew. The health gate dials :9201 (BasePort).
//
//	crew := defaultCrew([]string{"MCP_JWT_SECRET=abc", "MCP_AUTH_TOKEN=xyz"})
func defaultCrew(sandboxEnv []string) []crewMember {
	return []crewMember{
		{
			Capability: CapabilityInference,
			Binary:     "lthn-ai",
			AddrEnv:    "CORE_AI_HTTP_ADDR",
			Env: []string{
				"CORE_AI_SERVE_RUNTIME=mlx",           // auto-serve the mlx driver on boot
				"CORE_MCP_ADDR=/tmp/lthn-ai-mcp.sock", // MCP unix socket off the shared default
			},
			BasePort: 9100,
			Count:    1,
		},
		{
			// lthn-agent hub — replaces the deleted `serve` subcommand
			// (RFC.serve.md Unit B / Mantis #1807). Static args pin both
			// listen addresses so the desktop knows where to find them
			// regardless of the hub's defaults; the health gate dials
			// the HTTP control plane (BasePort = 9201).
			//
			// The flags MUST use the --key=value form: core.Options parses
			// `--mcp-http=ADDR` but treats `--mcp-http ADDR` as a valueless
			// bool + a stray positional, so a space-separated pin silently
			// falls back to the hub's default port. (Verified 2026-06-01.)
			Capability: CapabilitySandbox,
			Binary:     "lthn-agent",
			Serve:      []string{"hub", "--http=127.0.0.1:9201", "--mcp-http=127.0.0.1:9202"},
			Env:        append([]string(nil), sandboxEnv...),
			BasePort:   9201,
			Count:      1,
			// PTY-back the hub so its live output (where dispatched agent work
			// runs) is a watchable in-app terminal tab.
			Watch: true,
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
	binary    string
	addr      string // host:port the instance listens on
	procID    string // go-process id, for Kill on shutdown (process-backed members)
	sessionID string // terminal-pool session id (Watch members); supervised via terminal.Wait/Kill
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
		// In dev (task dev sets LTHN_DEV=1) the heavy inference sidecar
		// (lthn-ai, which auto-serves lthn-mlx on the Metal device / :9100) is
		// skipped by default so it doesn't clash with tests that need the GPU
		// or that port. Production (no LTHN_DEV) runs it. Force it on in dev
		// with LTHN_CREW_INFERENCE=1.
		if m.Capability == CapabilityInference &&
			core.Trim(core.Env("LTHN_DEV")) != "" &&
			core.Trim(core.Env("LTHN_CREW_INFERENCE")) != "1" {
			core.Info("fleet.crew: dev mode — skipping inference sidecar (lthn-ai/lthn-mlx); set LTHN_CREW_INFERENCE=1 to enable")
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
// AddrEnv (an env var — lthn-ai CORE_AI_HTTP_ADDR=127.0.0.1:PORT); the two are
// exclusive. Any static m.Env (KEY=VALUE) is merged in. A spawn that carries
// env goes through go-process StartWithOptions; a bare flag spawn keeps the
// plain Start path.
func spawnMember(ctx context.Context, m crewMember, port int) (procID, sessionID, errMsg string) {
	args := append([]string(nil), m.Serve...)
	if m.AddrFlag != "" {
		args = append(args, m.AddrFlag, core.Sprintf(":%d", port))
	}
	bin := resolveCrewBinary(m.Binary)
	env := append([]string(nil), m.Env...)
	if m.AddrEnv != "" {
		env = append(env, core.Sprintf("%s=127.0.0.1:%d", m.AddrEnv, port))
	}
	// Watch members run under a PTY in the terminal pool so their output is a
	// watchable in-app session; supervised below via crewWait/crewKill.
	if m.Watch {
		sid, err := terminal.Spawn(terminal.SpawnInput{
			Command: append([]string{bin}, args...),
			Env:     env,
			Label:   m.Binary,
			Cols:    200,
			Rows:    50,
		})
		if err != nil {
			return "", "", err.Error()
		}
		return "", sid, ""
	}
	var r core.Result
	if len(env) == 0 {
		r = process.Start(ctx, bin, args...)
	} else {
		r = process.StartWithOptions(ctx, process.RunOptions{
			Command: bin,
			Args:    args,
			Env:     env,
		})
	}
	if !r.OK {
		return "", "", r.Error()
	}
	return crewProcID(r), "", ""
}

// crewWait blocks until the instance's process exits — process.Wait for a
// process-backed member, terminal.Wait for a PTY-backed (Watch) member.
func crewWait(inst *crewInstance) {
	if inst.sessionID != "" {
		terminal.Wait(inst.sessionID)
		return
	}
	process.Wait(inst.procID)
}

// crewKill terminates the instance — process.Kill or terminal.Kill.
func crewKill(inst *crewInstance) {
	if inst.sessionID != "" {
		terminal.Kill(inst.sessionID)
		return
	}
	if inst.procID != "" {
		_ = process.Kill(inst.procID)
	}
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
	procID, sessionID, errMsg := spawnMember(ctx, m, port)
	if procID == "" && sessionID == "" {
		core.Warn("fleet.crew: spawn failed", "binary", m.Binary, "addr", addr, "err", errMsg)
		return
	}
	inst := &crewInstance{binary: m.Binary, addr: addr, procID: procID, sessionID: sessionID}

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
		crewWait(inst) // blocks until the process exits (process- or PTY-backed)

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
		procID, sessionID, errMsg := spawnMember(ctx, m, port)
		if procID == "" && sessionID == "" {
			core.Warn("fleet.crew: respawn failed", "binary", m.Binary, "err", errMsg)
			continue
		}
		inst.procID = procID
		inst.sessionID = sessionID
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
		crewKill(inst)
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
		time.Sleep(crewHealthPollInterval)
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
