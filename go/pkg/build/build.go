// SPDX-Licence-Identifier: EUPL-1.2

// Package build is the lthn-side build-tool surface. Detects what
// kind of project a directory contains (Go module, Wails app,
// Docker image, npm package, composer project, …) and runs the
// canonical build command for that kind via dappco.re/go/process —
// long-running, output captured into a ring buffer for live polling.
//
// Ported from core/ide's build_bridge.go + buildbridge.go. The
// MCPBridge indirection there existed for AI-tool dual-mount; lthn
// goes direct because the Wails GUI is the only consumer today.
// The DuckDB-backed detection cache from core/ide is intentionally
// dropped — marker files don't move and the probe is cheap; if it
// becomes a hot path the cache can land later.

package build

import (
	"context"

	core "dappco.re/go"
	"dappco.re/go/process"
)

const (
	goBuildNoOutputSuffix     = " && go build -o /dev/null ./..."
	startProcOp               = "build.startProc"
	processServiceUnavailable = "process service unavailable"
)

// Service owns the build surface. Holds *core.Core for late
// resolution of the process service (boot order).
type Service struct {
	core *core.Core
}

// NewService constructs the build surface against a Core container.
// Wired via application.NewService(build.NewService(c)) in
// pkg/desktop/desktop.go.
//
// Usage example:
//
//	svc := build.NewService(c)
func NewService(c *core.Core) *Service { return &Service{core: c} }

// Register constructs the build service for Core registration.
//
// Usage example:
//
//	core.New(core.WithService(build.Register))
func Register(c *core.Core) core.Result {
	return core.Ok(NewService(c))
}

// proc resolves the process service at call time. Returns nil when
// the service isn't registered — every caller checks and surfaces
// an error to the WebView rather than panicking.
func (s *Service) proc() *process.Service {
	if s == nil || s.core == nil {
		return nil
	}
	ps, _ := core.ServiceFor[*process.Service](s.core, "process")
	return ps
}

// exists checks whether `path` resolves via core.Stat.
func exists(path string) bool { return core.Stat(path).OK }

// hasCore reports whether the `core` binary is on $PATH. The IDE
// gives `core build` priority because it knows about the .core/
// build.yaml shape; otherwise we fall back to per-marker defaults.
func hasCore() bool { return core.App{}.Find("core", "core").OK }

// shellQuote single-quotes a path for safe inclusion in `sh -c`
// invocations. Mirrors core/ide's helper byte-for-byte so the
// resulting build commands look identical.
func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	out := "'"
	for _, r := range s {
		if r == '\'' {
			out += `'\''`
		} else {
			out += string(r)
		}
	}
	out += "'"
	return out
}

// Detection probes the workspace root for known build-tool marker
// files (go.mod, wails.json, Dockerfile, package.json, …) and
// returns the canonical invocation. Pure file-system check — no
// process spawn, no cache.
type Detection struct {
	Path        string   `json:"path"`
	ProjectType string   `json:"project_type"`
	Command     string   `json:"command"`
	Args        []string `json:"args"`
	HasCoreBin  bool     `json:"has_core_bin"`
}

func (d Detection) withCommand(projectType, command string, args []string) Detection {
	d.ProjectType = projectType
	d.Command = command
	d.Args = args
	return d
}

func (d Detection) withCoreFallback(projectType, command string, args []string) Detection {
	if d.HasCoreBin {
		return d.withCommand(projectType, "core", []string{"build"})
	}
	return d.withCommand(projectType, command, args)
}

func goBuildArgs(root string) []string {
	return []string{"-c", "cd " + shellQuote(root) + goBuildNoOutputSuffix}
}

// detectProject mirrors core/ide's projectdetect logic. Marker
// priority: .core/build.yaml → wails.json → go.mod (root then
// go/ subdir) → go.work → Dockerfile → CMakeLists.txt →
// Taskfile.yaml → composer.json → package.json. First match wins.
func detectProject(root string) Detection {
	d := Detection{Path: root, HasCoreBin: hasCore()}
	hasConfig := exists(core.PathJoin(root, ".core", "build.yaml"))
	switch {
	case hasConfig && d.HasCoreBin:
		d = d.withCommand("config", "core", []string{"build"})
	case exists(core.PathJoin(root, "wails.json")):
		d = d.withCoreFallback("wails", "wails", []string{"build"})
	case exists(core.PathJoin(root, "go.mod")):
		d = d.withCoreFallback("go", "sh", goBuildArgs(root))
	case exists(core.PathJoin(root, "go", "go.mod")):
		d = d.withCoreFallback("go-subdir", "sh", goBuildArgs(core.PathJoin(root, "go")))
	case exists(core.PathJoin(root, "go.work")):
		d = d.withCoreFallback("go-work", "sh", goBuildArgs(root))
	case exists(core.PathJoin(root, "Dockerfile")):
		d = d.withCoreFallback("docker", "docker", []string{"build", "."})
	case exists(core.PathJoin(root, "CMakeLists.txt")):
		d = d.withCommand("cpp", "cmake", []string{"--build", "build"})
	case exists(core.PathJoin(root, "Taskfile.yaml")), exists(core.PathJoin(root, "Taskfile.yml")):
		d = d.withCommand("taskfile", "task", []string{"build"})
	case exists(core.PathJoin(root, "composer.json")):
		d = d.withCommand("php", "composer", []string{"install"})
	case exists(core.PathJoin(root, "package.json")):
		d = d.withCommand("node", "npm", []string{"run", "build"})
	case hasConfig:
		d.ProjectType = "config" // .core/build.yaml exists but no core binary
	default:
		d.ProjectType = "unknown"
	}
	return d
}

// startProc invokes process.Service.Start at the workspace root.
// Returns the spawned *Process so callers can grab the ID.
func (s *Service) startProc(cwd, command string, args []string) core.Result {
	ps := s.proc()
	if ps == nil {
		return core.Fail(core.E(startProcOp, processServiceUnavailable, nil))
	}
	opts := process.RunOptions{
		Command: command,
		Args:    args,
		Dir:     cwd,
	}
	r := ps.StartWithOptions(context.Background(), opts)
	if !r.OK {
		return core.Fail(core.E(startProcOp, r.Error(), nil))
	}
	p, ok := r.Value.(*process.Process)
	if !ok || p == nil {
		return core.Fail(core.E(startProcOp, "process service returned non-process", nil))
	}
	return core.Ok(p)
}
