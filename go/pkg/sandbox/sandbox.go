// SPDX-Licence-Identifier: EUPL-1.2

// Package sandbox is the lthn-side surface for spawning agents
// inside OCI containers. Uses dappco.re/go/container for runtime
// detection + types, and shells out to whichever runtime the host
// has (Docker, Podman, Apple Container) via process.Service.
//
// Proof-of-life today: Spawn() pulls/uses an image, runs one
// command, waits for exit, returns combined stdout/stderr +
// runtime metadata. v0.8-a per the GOAL.md sandbox arc.
//
// Future: long-running agent containers with reverse-proxy mount
// at /v1/api/sandbox/<id>/* — same shape as the plugin host but
// with OS-process boundary instead of just process group.

package sandbox

import (

	core "dappco.re/go"
	"dappco.re/go/container"
	"dappco.re/go/process"

	"dappco.re/lthn/desktop/pkg/audit"
	"dappco.re/lthn/desktop/pkg/imagetrust"
)

// Options configures the sandbox host.
type Options struct {
	// DefaultImage is used when SpawnInput.Image is empty. The shipped
	// default is `lthn/dev:latest` — the Lethean developer image with
	// gh/codex/claude/go/php/node/python preinstalled. Override per-call
	// via SpawnInput.Image, or per-host by passing Options{DefaultImage: ...}.
	DefaultImage string
}

const (
	// defaultImage is the canonical fallback when neither Options.DefaultImage
	// nor SpawnInput.Image is set.
	defaultImage = "lthn/dev:latest"
	spawnOp      = "sandbox.Spawn"
	spawnAppleOp = "sandbox.spawnApple"
)

// Service is the sandbox host. Embeds *core.ServiceRuntime so
// process.Service can be resolved at call time.
type Service struct {
	*core.ServiceRuntime[Options]
	mu      core.RWMutex
	handles map[string]*ContainerHandle
}

// NewService returns the canonical Core service factory.
//
// Usage example:
//
//	core.WithName("sandbox", sandbox.NewService(sandbox.Options{}))
func NewService(opts Options) func(*core.Core) core.Result {
	return func(c *core.Core) core.Result {
		svc := &Service{
			ServiceRuntime: core.NewServiceRuntime(c, opts),
		}
		return core.Ok(svc)
	}
}

// Register constructs the sandbox service for Core registration.
//
// Usage example:
//
//	core.New(core.WithService(sandbox.Register))
func Register(c *core.Core) core.Result {
	return NewService(Options{})(c)
}

// ServiceName labels the binding namespace exposed to JS.
func (s *Service) ServiceName() string { return "Sandbox" }

// proc resolves the process service. Returns nil when missing.
func (s *Service) proc() *process.Service {
	if s == nil || s.ServiceRuntime == nil {
		return nil
	}
	c := s.Core()
	if c == nil {
		return nil
	}
	ps, _ := core.ServiceFor[*process.Service](c, "process")
	return ps
}

// SpawnInput drives Spawn. Image is an OCI tag the chosen runtime
// can resolve (`alpine`, `alpine:3.21`, `ubuntu:24.04`, etc.).
// Command + Args are the entrypoint override.
type SpawnInput struct {
	Image   string   `json:"image"`
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
	// Runtime overrides auto-detection ("docker", "podman", "apple").
	// Empty = highest-priority available.
	Runtime string `json:"runtime,omitempty"`
	// Timeout caps the container's wall-clock lifetime; 0 = 60s default.
	TimeoutSeconds int `json:"timeout_seconds,omitempty"`
	// Memory is the container memory budget in MB; 0 uses runtime defaults.
	Memory int `json:"memory,omitempty"`
	// CPUs is the container CPU budget; 0 uses runtime defaults.
	CPUs int `json:"cpus,omitempty"`
	// StorageOpt is passed to runtimes that support `--storage-opt`.
	StorageOpt string `json:"storage_opt,omitempty"`
}

// SpawnOutput is the one-shot result of a Spawn call.
type SpawnOutput struct {
	Runtime    string `json:"runtime"`
	Image      string `json:"image"`
	Command    string `json:"command"`
	Stdout     string `json:"stdout"`
	ExitCode   int    `json:"exit_code"`
	DurationMs int64  `json:"duration_ms"`
}

// Spawn is the Wails-table shim for one-shot container spawn. Returns
// ErrTierGoOnly and emits audit.EventSandboxSpawnRejected with
// `reason="tier_go_only"` (Mantis #1664 Phase B / Cerberus #55 ADD-2).
//
// Renderer-tier callers (Wails-bound frontend, MCP) reach this shim
// and get the typed reject. Go callers reach the substrate via
// SpawnPort obtained from NewSpawnPort — see spawnport.go.
//
// The method is kept exported on *Service so the Wails binding
// generator (build/Taskfile.yml `generate:bindings` walking
// `./pkg/desktop/...`) continues to produce typed TS bindings the
// frontend already imports. The runtime check intercepts before any
// substrate work; the binding is preserved-with-typed-error rather
// than stripped.
func (s *Service) Spawn(input SpawnInput) core.Result {
	emitTierReject("sandbox.Spawn")
	return core.Fail(ErrTierGoOnly)
}

// spawn is the substrate-tier implementation of one-shot container
// spawn. Reachable from Go callers only via SpawnPort.Spawn returned
// by NewSpawnPort. The chosen runtime is auto-detected via
// dappco.re/go/container.Detect() unless input.Runtime overrides.
//
// Dispatch:
//   - Apple Container: uses dappco.re/go/container.AppleProvider's
//     typed Run + Wait + Logs machinery (go-container's only
//     upstream Provider implementation).
//   - Docker / Podman: shells out via process.Service. A real
//     DockerProvider / PodmanProvider belongs upstream in
//     go-container; lthn-desktop ships the placeholder until then.
func (s *Service) spawn(input SpawnInput) core.Result {
	// Cerberus #47 S-4 (Mantis #1666) — Repudiation gap close. Emit
	// Requested BEFORE any validation / runtime work so a crash mid-call
	// still leaves the request decision in the audit substrate. The
	// command bytes are NEVER in Meta — SHA-256 hash per the brief's
	// SECURITY-NOTE escape valve (entrypoint commands occasionally
	// embed tokens / paths). Args and env are NEVER in Meta.
	commandHash := core.SHA256HexString(input.Command)
	startedAt := core.Now()
	_ = audit.Default().Record(audit.Event{
		Event:   audit.EventSandboxSpawnRequested,
		TS:      startedAt.UTC().Unix(),
		Scope:   "sandbox",
		Outcome: audit.OutcomeOK,
		Meta: map[string]any{
			"image":          input.Image,
			"command_hash":   commandHash,
			"container_name": "",
		},
	})

	res := s.spawnDispatch(input)

	if !res.OK {
		// Cerberus Mantis #1667 / #1666 — when the failure is a typed
		// imagetrust gate-reject (1:1 with Q-4 granular error_code enum),
		// surface the stable short code (`image_empty` | `image_imds` |
		// `image_registry_not_allowed` | …). Non-imagetrust failures fall
		// through to the bounded-keyspace audit.ErrorCode substrate per
		// RFC.error-code-cascade.md §4.2 (hybrid composition shape) —
		// previously assigned r.Error() raw prose, which the W1 substrate
		// (Mantis #1714) closes as a STRIDE-I leak surface.
		errCode := audit.ErrorCode(res)
		if causeErr, ok := res.Value.(error); ok {
			if tag := imagetrust.ErrorCode(causeErr); tag != "" && tag != "image_invalid" {
				errCode = tag
			}
		}
		_ = audit.Default().Record(audit.Event{
			Event:   audit.EventSandboxSpawnFailed,
			TS:      core.Now().UTC().Unix(),
			Scope:   "sandbox",
			Outcome: audit.OutcomeFailed,
			Meta: map[string]any{
				"error_code":     errCode,
				"container_name": "",
			},
		})
		return res
	}

	out, _ := res.Value.(SpawnOutput)
	_ = audit.Default().Record(audit.Event{
		Event:   audit.EventSandboxSpawnSucceeded,
		TS:      core.Now().UTC().Unix(),
		Scope:   "sandbox",
		Outcome: audit.OutcomeOK,
		Meta: map[string]any{
			"container_id": "",
			"exit_code":    out.ExitCode,
			"duration_ms":  out.DurationMs,
		},
	})
	return res
}

// spawnDispatch is the pre-audit Spawn body — kept as a separate
// helper so the public Spawn() wraps the dispatch in the Requested /
// Succeeded / Failed audit pair without losing the early-return
// shape of the original.
func (s *Service) spawnDispatch(input SpawnInput) core.Result {
	prepared := s.prepareSpawnInput(input)
	if !prepared.OK {
		return prepared
	}
	input = prepared.Value.(SpawnInput)
	rtResult := s.resolveRuntime(input.Runtime)
	if !rtResult.OK {
		return rtResult
	}
	rt := rtResult.Value.(container.RuntimeType)
	timeout := core.Duration(input.TimeoutSeconds) * core.Second
	if timeout <= 0 {
		timeout = 60 * core.Second
	}
	if rt == container.RuntimeApple {
		return s.spawnApple(input, timeout)
	}
	return s.spawnViaCLI(rt, input, timeout)
}

func (s *Service) prepareSpawnInput(input SpawnInput) core.Result {
	if core.Trim(input.Image) == "" {
		input.Image = s.resolveDefaultImage()
	}
	// Cerberus Mantis #1667 (RFC v1.1 §2.4.1, ADD-2) — image-allowlist
	// gate fires AFTER default-image substitution so the validator sees
	// the exact ref the runtime will see. Validating BEFORE substitution
	// would reject an empty input.Image as ErrEmptyImageRef when the
	// default path would have substituted a valid ref; worse, a future
	// config-mutable default could be smuggled past the gate by passing
	// empty. AFTER-substitution closes both.
	if err := imagetrust.IsAllowedImage(input.Image); err != nil {
		return core.Fail(core.E(spawnOp, "image rejected by allowlist", err))
	}
	if input.Memory < 0 {
		return core.Fail(core.E(spawnOp, "memory must be >= 0", nil))
	}
	if input.CPUs < 0 {
		return core.Fail(core.E(spawnOp, "cpus must be >= 0", nil))
	}
	input.StorageOpt = core.Trim(input.StorageOpt)
	if core.Trim(input.Command) == "" {
		return core.Fail(core.E(spawnOp, "command is required", nil))
	}
	return core.Ok(input)
}

// spawnApple routes through dappco.re/go/container.AppleProvider —
// the canonical Provider implementation for macOS 26+ native
// containers. Bundles the EntryPoint into a ContainerConfig + waits
// for exit via the provider's Wait machinery.
func (s *Service) spawnApple(input SpawnInput, timeout core.Duration) core.Result {
	provider := container.NewAppleProvider()
	if !provider.Available() {
		return core.Fail(core.E(spawnAppleOp,
			"AppleProvider not available - install Apple Container CLI on macOS 26+", nil))
	}
	// For proof-of-life we treat the image string as an already-
	// pulled OCI tag; AppleProvider's Build() expects a Containerfile
	// path or existing tag in its Source field.
	img := &container.Image{
		Name:     input.Image,
		Path:     input.Image,
		Provider: string(container.RuntimeApple),
	}
	ctx, cancel := core.WithTimeout(core.Background(), timeout)
	defer cancel()
	started := core.Now()
	// AppleProvider.Run uses ro.Name as the container name + threads
	// EntryPoint args via Args (verified against external/container's
	// apple.go). For now we encode command+args by prepending the
	// command into image.Path style — proof-of-life only.
	if input.StorageOpt != "" {
		return core.Fail(core.E(spawnAppleOp, "storage_opt is not supported by AppleProvider", nil))
	}
	runOpts := []container.RunOption{
		container.WithName(core.Sprintf("lthn-sandbox-%d", started.UnixNano())),
	}
	if input.Memory > 0 {
		runOpts = append(runOpts, container.WithMemory(input.Memory))
	}
	if input.CPUs > 0 {
		runOpts = append(runOpts, container.WithCPUs(input.CPUs))
	}
	ctr, err := provider.Run(img, runOpts...)
	if err != nil {
		return core.Fail(core.E(spawnAppleOp, "run failed", err))
	}
	if waitErr := provider.Wait(ctx, ctr.ID); waitErr != nil {
		return core.Fail(core.E(spawnAppleOp, "wait failed", waitErr))
	}
	dur := core.Since(started).Milliseconds()
	// AppleProvider's Container struct has Status but no ExitCode
	// today; map exit semantics from Status until the upstream
	// provider exposes the raw code.
	exit := 0
	if ctr.Status == container.StatusError {
		exit = -1
	}
	return core.Ok(SpawnOutput{
		Runtime:    string(container.RuntimeApple),
		Image:      input.Image,
		Command:    input.Command,
		Stdout:     "(AppleProvider doesn't surface stdout for one-shot Run today - extend its Logs() to capture for proof-of-life)",
		ExitCode:   exit,
		DurationMs: dur,
	})
}

// spawnViaCLI is the proof-of-life path for Docker / Podman until
// go-container ships real providers for them. process.Service runs
// `<runtime> run --rm <image> <command> <args...>` and captures
// combined stdout.
func (s *Service) spawnViaCLI(rt container.RuntimeType, input SpawnInput, timeout core.Duration) core.Result {
	ps := s.proc()
	if ps == nil {
		return core.Fail(core.E("sandbox.spawnViaCLI", "process service unavailable", nil))
	}
	argsResult := s.buildRunArgs(rt, input)
	if !argsResult.OK {
		return argsResult
	}
	run := argsResult.Value.(runCommand)
	ctx, cancel := core.WithTimeout(core.Background(), timeout)
	defer cancel()
	started := core.Now()
	r := ps.Run(ctx, run.Binary, run.Args...)
	dur := core.Since(started).Milliseconds()
	exit := 0
	out, _ := r.Value.(string)
	if !r.OK {
		// process.Service surfaces non-zero exit as Fail; treat as
		// partial success — the container ran, just exited non-zero.
		exit = -1
		out = r.Error()
	}
	return core.Ok(SpawnOutput{
		Runtime:    string(rt),
		Image:      input.Image,
		Command:    input.Command,
		Stdout:     out,
		ExitCode:   exit,
		DurationMs: dur,
	})
}

// resolveDefaultImage returns Options.DefaultImage when set, else
// the package-level default (lthn/dev:latest).
func (s *Service) resolveDefaultImage() string {
	if s != nil && s.ServiceRuntime != nil {
		if img := core.Trim(s.Options().DefaultImage); img != "" {
			return img
		}
	}
	return defaultImage
}

// resolveRuntime picks a container runtime. If `prefer` is set we
// require that specific runtime; otherwise we ask
// dappco.re/go/container.Detect() for the highest-priority alive.
func (s *Service) resolveRuntime(prefer string) core.Result {
	if prefer != "" {
		rt := container.RuntimeType(prefer)
		if !container.HasRuntime(rt) {
			return core.Fail(core.E("sandbox.resolveRuntime", "requested runtime not available: "+prefer, nil))
		}
		return core.Ok(rt)
	}
	detected := container.Detect()
	if detected.Type == container.RuntimeNone {
		return core.Fail(core.E("sandbox.resolveRuntime", "no container runtime detected", nil))
	}
	return core.Ok(detected.Type)
}

type runCommand struct {
	Binary string
	Args   []string
}

func appendResourceArgs(cmd []string, input SpawnInput, storageOpt bool) []string {
	if input.Memory > 0 {
		cmd = append(cmd, "--memory", core.Sprintf("%dM", input.Memory))
	}
	if input.CPUs > 0 {
		cmd = append(cmd, "--cpus", core.Sprintf("%d", input.CPUs))
	}
	if storageOpt && input.StorageOpt != "" {
		cmd = append(cmd, "--storage-opt", input.StorageOpt)
	}
	return cmd
}

// buildRunArgs constructs the `<runtime> run --rm <image> <cmd> <args...>`
// invocation. Each runtime has slightly different flag shape; we
// only need the lowest common denominator for proof-of-life.
//
// Cerberus Mantis #1663 — hardenedDefaults (cap-drop=ALL +
// no-new-privileges + pids-limit) are applied to BOTH one-shot Spawn
// (this path) AND long-running SpawnLong (buildLongRunArgs). The
// defaults predate one-shot Spawn; without them a compromised renderer
// invoking Sandbox.Spawn would get a container with default Docker
// root caps (cap_dac_override, cap_setuid, cap_sys_chroot — standard
// LPE primitive set). Apple Container CLI accepts the same flag names
// via its docker-compatible run surface.
func (s *Service) buildRunArgs(rt container.RuntimeType, input SpawnInput) core.Result {
	cmd := []string{"run", "--rm"}
	cmd = append(cmd, hardenedDefaults...)
	switch rt {
	case container.RuntimeDocker:
		// Docker's `--rm` auto-removes after exit. Good for one-shot.
		// Cerberus Mantis #1667 (RFC v1.1 §2.5) — argv `--` terminator
		// between IMAGE and COMMAND tells docker to stop parsing its
		// own flags. Neutralises a future docker CLI flag-parser change
		// re-interpreting `input.Command` starting with `-` as a docker
		// flag, AND covers `input.Args[0]` starting with `--`.
		cmd = appendResourceArgs(cmd, input, true)
		cmd = append(cmd, input.Image, "--", input.Command)
		cmd = append(cmd, input.Args...)
		return core.Ok(runCommand{Binary: "docker", Args: cmd})
	case container.RuntimePodman:
		// Same argv `--` discipline as the Docker arm.
		cmd = appendResourceArgs(cmd, input, true)
		cmd = append(cmd, input.Image, "--", input.Command)
		cmd = append(cmd, input.Args...)
		return core.Ok(runCommand{Binary: "podman", Args: cmd})
	case container.RuntimeApple:
		// Apple Container CLI mirrors Docker run semantics. Argv-
		// terminator discipline for the Apple arm is delegated to
		// dappco.re/go/container (RFC v1.1 §3.2 T-OOS-5 / ADD-4) — the
		// Apple-arm Spawn routes through container.AppleProvider.Run
		// upstream, not through this buildRunArgs path in production.
		// This branch survives only for the spawnViaCLI legacy fallback;
		// no `--` injection asserted here.
		if input.StorageOpt != "" {
			return core.Fail(core.E("sandbox.buildRunArgs", "storage_opt is not supported by Apple runtime", nil))
		}
		cmd = appendResourceArgs(cmd, input, false)
		cmd = append(cmd, input.Image, input.Command)
		cmd = append(cmd, input.Args...)
		return core.Ok(runCommand{Binary: "container", Args: cmd})
	default:
		return core.Fail(core.E("sandbox.buildRunArgs", "runtime not supported for spawn: "+string(rt), nil))
	}
}

// Detect returns the runtimes the host advertises. Wraps
// dappco.re/go/container.DetectAll() with a Wails-friendly shape.
type DetectOutput struct {
	Available []container.ContainerRuntime `json:"available"`
	Preferred string                       `json:"preferred"`
}

// Detect surfaces what runtimes the host has, with `preferred`
// pointing at the highest-priority alive one (or empty when none).
func (s *Service) Detect() core.Result {
	all := container.DetectAll()
	preferred := ""
	chosen := container.Detect()
	if chosen.Type != container.RuntimeNone {
		preferred = string(chosen.Type)
	}
	return core.Ok(DetectOutput{Available: all, Preferred: preferred})
}
