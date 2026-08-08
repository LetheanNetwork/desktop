// SPDX-Licence-Identifier: EUPL-1.2

// process.go — bridge tools for spawning + supervising processes.
// Wraps dappco.re/go/process.Service, which is already registered
// on Core (see cmd/lthn/app.go) and used by pkg/build, pkg/lint,
// pkg/git, pkg/php for their long-running command surface.
//
// Each tool resolves the process service at call time so the
// bridge stays decoupled from process.Service's lifecycle.

package bridge

import (
	core "dappco.re/go"
	"dappco.re/go/process"
)

// procSvc resolves the process service from Core. Bridge embeds
// *core.ServiceRuntime[Options], same pattern as plugin host, so
// s.Core() returns the attached *core.Core.
func (s *Service) procSvc() *process.Service {
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

// toolProcessStart spawns a process. params: { command, args?, dir?, env? }
func (s *Service) toolProcessStart(ctx core.Context, params map[string]any) map[string]any {
	ps := s.procSvc()
	if ps == nil {
		return map[string]any{"ok": false, "error": processServiceUnavailable}
	}
	command := paramString(params, "command", "")
	if command == "" {
		return map[string]any{"ok": false, "error": "command param required"}
	}
	args := stringSliceParam(params, "args")
	dir := paramString(params, "dir", "")
	env := stringSliceParam(params, "env")
	opts := process.RunOptions{
		Command: command,
		Args:    args,
		Dir:     dir,
		Env:     env,
	}
	r := ps.StartWithOptions(ctx, opts)
	if !r.OK {
		return map[string]any{"ok": false, "error": r.Error()}
	}
	p, ok := r.Value.(*process.Process)
	if !ok || p == nil {
		return map[string]any{"ok": false, "error": "process service returned non-process"}
	}
	return map[string]any{"ok": true, "id": p.ID, "pid": p.Info().PID, "command": command}
}

// toolProcessKill terminates a process by ID. params: { id }
func (s *Service) toolProcessKill(params map[string]any) map[string]any {
	ps := s.procSvc()
	if ps == nil {
		return map[string]any{"ok": false, "error": processServiceUnavailable}
	}
	id := paramString(params, "id", "")
	if id == "" {
		return map[string]any{"ok": false, "error": idParamRequired}
	}
	r := ps.Kill(id)
	if !r.OK {
		return map[string]any{"ok": false, "error": r.Error()}
	}
	return map[string]any{"ok": true, "killed": id}
}

// toolProcessStop is an alias for kill — the reference bridge had
// separate concepts but Wails/process.Service collapse them.
func (s *Service) toolProcessStop(params map[string]any) map[string]any {
	return s.toolProcessKill(params)
}

// toolProcessList enumerates every managed process.
func (s *Service) toolProcessList() map[string]any {
	ps := s.procSvc()
	if ps == nil {
		return map[string]any{"ok": false, "error": processServiceUnavailable}
	}
	procs := ps.List()
	out := make([]map[string]any, 0, len(procs))
	for _, p := range procs {
		info := p.Info()
		out = append(out, map[string]any{
			"id":       p.ID,
			"command":  info.Command,
			"args":     info.Args,
			"dir":      info.Dir,
			"pid":      info.PID,
			"running":  info.Running,
			"status":   string(info.Status),
			"exitCode": info.ExitCode,
		})
	}
	return map[string]any{"ok": true, "value": out, "count": len(out)}
}

// toolProcessOutput returns the ring-buffered stdout/stderr for a
// managed process. params: { id }
func (s *Service) toolProcessOutput(params map[string]any) map[string]any {
	ps := s.procSvc()
	if ps == nil {
		return map[string]any{"ok": false, "error": processServiceUnavailable}
	}
	id := paramString(params, "id", "")
	if id == "" {
		return map[string]any{"ok": false, "error": idParamRequired}
	}
	r := ps.Output(id)
	if !r.OK {
		return map[string]any{"ok": false, "error": r.Error()}
	}
	out, _ := r.Value.(string)
	return map[string]any{"ok": true, "value": out, "size": len(out)}
}

// toolProcessInput writes to the managed process's stdin.
// params: { id, input }
func (s *Service) toolProcessInput(params map[string]any) map[string]any {
	ps := s.procSvc()
	if ps == nil {
		return map[string]any{"ok": false, "error": processServiceUnavailable}
	}
	id := paramString(params, "id", "")
	input := paramString(params, "input", "")
	if id == "" {
		return map[string]any{"ok": false, "error": idParamRequired}
	}
	r := ps.Input(id, input)
	if !r.OK {
		return map[string]any{"ok": false, "error": r.Error()}
	}
	return map[string]any{"ok": true, "id": id, "bytes": len(input)}
}

// stringSliceParam — params often arrive as []any from JSON; normalise to []string.
func stringSliceParam(params map[string]any, key string) []string {
	raw, ok := params[key]
	if !ok || raw == nil {
		return nil
	}
	if s, ok := raw.([]string); ok {
		return s
	}
	arr, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, v := range arr {
		if s, ok := v.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}
