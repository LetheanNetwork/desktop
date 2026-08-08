// SPDX-Licence-Identifier: EUPL-1.2

// Wails3 Service shape for pkg/build — typed methods the WebView
// can bind directly. Lifts the project-detection logic from core/
// ide's build_bridge.go without the MCP indirection and without
// the DuckDB cache (marker probes are cheap; cache lands later if
// it ever shows up in a profile).

package build

import (
	core "dappco.re/go"
	"dappco.re/go/process"
)

const (
	detectOp        = "build.Detect"
	processOutputOp = "build.ProcessOutput"
	processKillOp   = "build.ProcessKill"
)

// ServiceName / Startup / Shutdown — Wails3 lifecycle.
func (s *Service) ServiceName() string { return "Build" }
func (s *Service) ServiceStartup(_ core.Context, _ any) core.Result {
	return core.Ok(nil)
}
func (s *Service) ServiceShutdown() core.Result { return core.Ok(nil) }

// Detect probes path for marker files and returns the canonical
// build invocation. Pure file-system probe, no spawn.
//
// Usage example (TS):
//
//	import { Detect } from "@desktop/build/service";
//	const d = await Detect("/path/to/project");
//	console.log(d.project_type, d.command, d.args);
func (s *Service) Detect(path string) core.Result {
	if core.Trim(path) == "" {
		return core.Fail(core.E(detectOp, "path required", nil))
	}
	stat := core.Stat(path)
	if !stat.OK {
		return core.Fail(core.E(detectOp, "path is not a directory: "+path, nil))
	}
	info, ok := stat.Value.(interface{ IsDir() bool })
	if !ok || !info.IsDir() {
		return core.Fail(core.E(detectOp, "path is not a directory: "+path, nil))
	}
	return core.Ok(detectProject(path))
}

// RunResult describes the spawned build process. Callers poll
// Output / Kill / list via the matching Process* methods below.
type RunResult struct {
	ProcessID    string   `json:"process_id"`
	BuildCommand string   `json:"build_command"`
	BuildArgs    []string `json:"build_args"`
	ProjectType  string   `json:"project_type"`
}

// Run starts a build process for path. When command is empty, the
// auto-detected command for the project type is used; same for
// args. Returns the process_id the WebView polls via
// ProcessOutput / ProcessKill.
//
// Usage example (TS):
//
//	const { process_id, build_command } = await Run("/path", "", []);
//	// then poll:
//	const out = await ProcessOutput(process_id);
func (s *Service) Run(path, command string, args []string) core.Result {
	if core.Trim(path) == "" {
		return core.Fail(core.E("build.Run", "path required", nil))
	}
	detected := detectProject(path)
	if command == "" {
		command = detected.Command
		if len(args) == 0 {
			args = detected.Args
		}
	}
	if command == "" {
		return core.Fail(core.E("build.Run", "could not detect a build command for: "+path, nil))
	}
	procR := s.startProc(path, command, args)
	if !procR.OK {
		return procR
	}
	proc := procR.Value.(*process.Process)
	return core.Ok(RunResult{
		ProcessID:    proc.ID,
		BuildCommand: command,
		BuildArgs:    args,
		ProjectType:  detected.ProjectType,
	})
}

// ProcessOutput returns the accumulated stdout+stderr for a
// running (or finished) process. Reads the ring buffer — bounded;
// older bytes get evicted under sustained heavy output. Callers
// can poll on a timer; a future SSE/WS hookup can stream instead.
//
// Usage example (TS):
//
//	const text = await ProcessOutput(processID);
func (s *Service) ProcessOutput(id string) core.Result {
	ps := s.proc()
	if ps == nil {
		return core.Fail(core.E(processOutputOp, processServiceUnavailable, nil))
	}
	r := ps.Get(id)
	if !r.OK {
		return core.Fail(core.E(processOutputOp, r.Error(), nil))
	}
	p, ok := r.Value.(*process.Process)
	if !ok || p == nil {
		return core.Fail(core.E(processOutputOp, "process not found: "+id, nil))
	}
	return core.Ok(p.Output())
}

// ProcessKill terminates a running process. Idempotent — killing
// an already-finished process is a no-op success.
//
// Usage example (TS):
//
//	await ProcessKill(processID);
func (s *Service) ProcessKill(id string) core.Result {
	ps := s.proc()
	if ps == nil {
		return core.Fail(core.E(processKillOp, processServiceUnavailable, nil))
	}
	r := ps.Get(id)
	if !r.OK {
		return core.Fail(core.E(processKillOp, r.Error(), nil))
	}
	p, ok := r.Value.(*process.Process)
	if !ok || p == nil {
		return core.Fail(core.E(processKillOp, "process not found: "+id, nil))
	}
	if kr := p.Kill(); !kr.OK {
		return core.Fail(core.E(processKillOp, kr.Error(), nil))
	}
	return core.Ok(nil)
}

// ProcessEntry is the lean shape ProcessList returns — enough for
// the build panel's "active processes" rail.
type ProcessEntry struct {
	ID       string `json:"id"`
	Command  string `json:"command"`
	Status   string `json:"status"`
	ExitCode int    `json:"exit_code"`
}

// ProcessList returns every process the process service is
// tracking (including non-build ones — they share one registry).
// The build panel filters client-side by ID prefix when needed.
//
// Usage example (TS):
//
//	const procs = await ProcessList();
func (s *Service) ProcessList() core.Result {
	ps := s.proc()
	if ps == nil {
		return core.Fail(core.E("build.ProcessList", processServiceUnavailable, nil))
	}
	out := []ProcessEntry{}
	for _, p := range ps.List() {
		if p == nil {
			continue
		}
		info := p.Info()
		out = append(out, ProcessEntry{
			ID:       info.ID,
			Command:  info.Command,
			Status:   string(info.Status),
			ExitCode: info.ExitCode,
		})
	}
	return core.Ok(out)
}
