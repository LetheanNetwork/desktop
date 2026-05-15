// SPDX-Licence-Identifier: EUPL-1.2

// Runtime path — Start spawns the plugin binary via
// process.Service with the canonical CLI contract, polls health
// until ready, then registers the reverse-proxy mount. Stop
// terminates the process + unregisters. Phase 1 has no
// crash-loop supervision yet; ManagedProcess exit is observed
// but no auto-restart.

package plugin

import (
	"net"

	core "dappco.re/go"
	"dappco.re/go/process"
)

// processHandle wraps the *process.Process we got back from
// process.Service.Start + the URL the proxy forwards to. Kept
// separate from the broader pluginState so the mutation surface
// for the supervisor (future phase) is narrow.
type processHandle struct {
	proc   *process.Process
	target string // http://127.0.0.1:<port>
	port   int
}

// proc resolves the process service at call time. Returns nil
// when the service isn't registered.
func (s *Service) proc() *process.Service {
	c := s.coreApp()
	if c == nil {
		return nil
	}
	ps, _ := core.ServiceFor[*process.Service](c, "process")
	return ps
}

// pickFreePort returns a TCP port the OS just confirmed is
// free. Yes there's a TOCTOU race between Close() and the
// plugin binding — accept it for now; collisions in practice
// are rare and v2 can move to socket-activation if needed.
func pickFreePort() core.Result {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return core.Fail(core.E("plugin.pickFreePort", "listen failed", err))
	}
	defer l.Close()
	return core.Ok(l.Addr().(*net.TCPAddr).Port)
}

// waitForHealth polls http://127.0.0.1:<port><healthPath> until
// it returns 200 or `timeout` elapses. Backoff is constant
// 100 ms — plugins should start fast; if they don't, the
// manifest's startup_timeout buys more time.
func waitForHealth(ctx core.Context, port int, path string, timeout core.Duration) core.Result {
	url := "http://127.0.0.1:" + core.Itoa(port) + path
	deadline := core.Now().Add(timeout)
	for core.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return core.Fail(core.E("plugin.waitForHealth", "context cancelled", nil))
		default:
		}
		req := core.NewHTTPRequestContext(ctx, core.MethodGet, url, nil).Value.(*core.Request)
		resp, err := core.DefaultHTTPClient.Do(req)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == core.StatusOK {
				return core.Ok(nil)
			}
		}
		core.Sleep(100 * core.Millisecond)
	}
	return core.Fail(core.E("plugin.waitForHealth",
		"plugin did not become healthy within "+timeout.String(), nil))
}

// startPlugin reads the on-disk manifest for `code`, picks a
// free port, spawns the plugin with the canonical CLI flags,
// waits for health, then registers the reverse-proxy mount.
// Caller holds s.mu.
func (s *Service) startPlugin(ctx core.Context, code, token string) core.Result {
	dirR := pluginDir(code)
	if !dirR.OK {
		return dirR
	}
	dir, _ := dirR.Value.(string)
	manifestR := loadManifest(core.PathJoin(dir, "plugin.json"))
	if !manifestR.OK {
		return manifestR
	}
	m, _ := manifestR.Value.(Manifest)
	ps := s.proc()
	if ps == nil {
		return core.Fail(core.E(startPluginOp, "process service unavailable", nil))
	}
	portR := pickFreePort()
	if !portR.OK {
		return portR
	}
	port, _ := portR.Value.(int)
	binPath := core.PathJoin(dir, m.Binary)
	args := []string{
		"--namespace=" + m.Namespace,
		"--port=" + core.Itoa(port),
		"--token=" + token,
		"--data=" + core.PathJoin(dir, "data"),
	}
	opts := process.RunOptions{
		Command: binPath,
		Args:    args,
		Dir:     dir,
	}
	spawn := ps.StartWithOptions(ctx, opts)
	if !spawn.OK {
		return core.Fail(core.E(startPluginOp, "spawn: "+spawn.Error(), nil))
	}
	proc, ok := spawn.Value.(*process.Process)
	if !ok || proc == nil {
		return core.Fail(core.E(startPluginOp, "process service returned non-process", nil))
	}
	// Mark starting so List/Status reflects intent immediately.
	ps2 := &pluginState{
		manifest:  m,
		state:     "starting",
		port:      port,
		pid:       proc.Info().PID,
		startedAt: core.Now(),
		proc: &processHandle{
			proc:   proc,
			target: "http://127.0.0.1:" + core.Itoa(port),
			port:   port,
		},
	}
	s.state[code] = ps2
	// Health gate. Plugins that don't return 200 within their
	// startup window get killed; pluginState becomes "dead".
	timeout := core.Duration(m.StartupTimeout) * core.Second
	if timeout <= 0 {
		timeout = 5 * core.Second
	}
	if r := waitForHealth(ctx, port, m.Health.Path, timeout); !r.OK {
		// Kill the unhealthy process; surface the error to the caller.
		if kill := ps.Kill(proc.ID); !kill.OK {
			ps2.lastError = core.Concat(r.Error(), "; kill: ", kill.Error())
			return r
		}
		ps2.state = "dead"
		ps2.lastError = r.Error()
		return r
	}
	// Wire the proxy mount + flip state to running.
	s.proxy.Set(code, ps2.proc.target)
	ps2.state = "running"
	// Attach the supervisor — on unexpected exit it decides
	// restart-vs-die. User-initiated Stop sets state to
	// "stopped" first so the watcher knows the exit was
	// expected and skips the restart path.
	s.Core().Go(func() { s.watchProcess(code, proc) })
	return core.Ok(nil)
}

// stopPlugin signals the running plugin to terminate. Phase 1
// uses process.Service.Kill (SIGKILL) — the SIGTERM-then-SIGKILL
// dance lands when the supervisor phase adds drain windows.
// Caller holds s.mu.
func (s *Service) stopPlugin(code string) core.Result {
	ps2, ok := s.state[code]
	if !ok || ps2.proc == nil {
		return core.Ok(nil) // already stopped
	}
	ps := s.proc()
	if ps != nil && ps2.proc.proc != nil {
		if r := ps.Kill(ps2.proc.proc.ID); !r.OK {
			return r
		}
	}
	s.proxy.Delete(code)
	ps2.state = "stopped"
	ps2.stoppedAt = core.Now()
	ps2.proc = nil
	return core.Ok(nil)
}

// resolveToken pulls the current local API key from the apikey
// service. Plugin host is wired after apikey at boot so the
// lookup is reliable.
func (s *Service) resolveToken() string {
	c := s.coreApp()
	if c == nil {
		return ""
	}
	// Late-binding via a string-keyed Service lookup. The exact
	// service name + getter signature is defined in pkg/apikey;
	// referencing the package directly here would pull apikey
	// into plugin's deps. The runner already calls into apikey
	// the same way (see pkg/runner/routes.go for the pattern).
	type tokenSource interface {
		Reveal() core.Result
	}
	for _, name := range []string{"apikey", "api_key"} {
		raw, ok := core.ServiceFor[tokenSource](c, name)
		if ok && raw != nil {
			tokR := raw.Reveal()
			tok, _ := tokR.Value.(string)
			if tokR.OK && tok != "" {
				return tok
			}
		}
	}
	return ""
}
