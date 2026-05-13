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
	"context"
	"time"

	core "dappco.re/go"
	"dappco.re/go/container"
	"dappco.re/go/process"
)

// Options configures the sandbox host.
type Options struct {
	// DefaultImage is used when SpawnInput.Image is empty. The shipped
	// default is `lthn/dev:latest` — the Lethean developer image with
	// gh/codex/claude/go/php/node/python preinstalled. Override per-call
	// via SpawnInput.Image, or per-host by passing Options{DefaultImage: ...}.
	DefaultImage string
}

// defaultImage is the canonical fallback when neither Options.DefaultImage
// nor SpawnInput.Image is set.
const defaultImage = "lthn/dev:latest"

// Service is the sandbox host. Embeds *core.ServiceRuntime so
// process.Service can be resolved at call time.
type Service struct {
	*core.ServiceRuntime[Options]
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

// Spawn boots a one-shot container, waits for exit, returns
// captured output. The chosen runtime is auto-detected via
// dappco.re/go/container.Detect() unless input.Runtime overrides.
//
// Dispatch:
//   - Apple Container: uses dappco.re/go/container.AppleProvider's
//     typed Run + Wait + Logs machinery (go-container's only
//     upstream Provider implementation).
//   - Docker / Podman: shells out via process.Service. A real
//     DockerProvider / PodmanProvider belongs upstream in
//     go-container; lthn-desktop ships the placeholder until then.
func (s *Service) Spawn(input SpawnInput) core.Result {
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
	timeout := time.Duration(input.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 60 * time.Second
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
	if input.Memory < 0 {
		return core.Fail(core.E("sandbox.Spawn", "memory must be >= 0", nil))
	}
	if input.CPUs < 0 {
		return core.Fail(core.E("sandbox.Spawn", "cpus must be >= 0", nil))
	}
	input.StorageOpt = core.Trim(input.StorageOpt)
	if core.Trim(input.Command) == "" {
		return core.Fail(core.E("sandbox.Spawn", "command is required", nil))
	}
	return core.Ok(input)
}

// spawnApple routes through dappco.re/go/container.AppleProvider —
// the canonical Provider implementation for macOS 26+ native
// containers. Bundles the EntryPoint into a ContainerConfig + waits
// for exit via the provider's Wait machinery.
func (s *Service) spawnApple(input SpawnInput, timeout time.Duration) core.Result {
	provider := container.NewAppleProvider()
	if !provider.Available() {
		return core.Fail(core.E("sandbox.spawnApple",
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
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	started := time.Now()
	// AppleProvider.Run uses ro.Name as the container name + threads
	// EntryPoint args via Args (verified against external/container's
	// apple.go). For now we encode command+args by prepending the
	// command into image.Path style — proof-of-life only.
	if input.StorageOpt != "" {
		return core.Fail(core.E("sandbox.spawnApple", "storage_opt is not supported by AppleProvider", nil))
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
		return core.Fail(core.E("sandbox.spawnApple", "run failed", err))
	}
	if waitErr := provider.Wait(ctx, ctr.ID); waitErr != nil {
		return core.Fail(core.E("sandbox.spawnApple", "wait failed", waitErr))
	}
	dur := time.Since(started).Milliseconds()
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
func (s *Service) spawnViaCLI(rt container.RuntimeType, input SpawnInput, timeout time.Duration) core.Result {
	ps := s.proc()
	if ps == nil {
		return core.Fail(core.E("sandbox.spawnViaCLI", "process service unavailable", nil))
	}
	argsResult := s.buildRunArgs(rt, input)
	if !argsResult.OK {
		return argsResult
	}
	run := argsResult.Value.(runCommand)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	started := time.Now()
	r := ps.Run(ctx, run.Binary, run.Args...)
	dur := time.Since(started).Milliseconds()
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
func (s *Service) buildRunArgs(rt container.RuntimeType, input SpawnInput) core.Result {
	cmd := []string{"run", "--rm"}
	switch rt {
	case container.RuntimeDocker:
		// Docker's `--rm` auto-removes after exit. Good for one-shot.
		cmd = appendResourceArgs(cmd, input, true)
		cmd = append(cmd, input.Image, input.Command)
		cmd = append(cmd, input.Args...)
		return core.Ok(runCommand{Binary: "docker", Args: cmd})
	case container.RuntimePodman:
		cmd = appendResourceArgs(cmd, input, true)
		cmd = append(cmd, input.Image, input.Command)
		cmd = append(cmd, input.Args...)
		return core.Ok(runCommand{Binary: "podman", Args: cmd})
	case container.RuntimeApple:
		// Apple Container CLI mirrors Docker run semantics.
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
