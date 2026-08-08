// SPDX-Licence-Identifier: EUPL-1.2

// Supervisor — Phase 2 of the plugin host. Watches each running
// plugin's ManagedProcess.Done() channel; on unexpected exit
// (i.e. plugin crashed, host did not call Stop), schedules a
// restart with backoff. After three crashes inside a 60-second
// window the plugin is marked "dead" and the host stops trying
// — the user can re-Install or fix whatever's wrong before
// manual Start.
//
// User-initiated Stop clears the restart intent so the watcher
// doesn't re-spawn. Likewise Remove drops the supervisor entry
// entirely.

package plugin

import (
	core "dappco.re/go"
	"dappco.re/go/process"
)

// crashWindow is the wall-clock window over which crashCap
// failures count. Three crashes inside 60 s → dead.
const crashWindow = 60 * core.Second

// crashCap is the maximum tolerated crashes inside crashWindow
// before the supervisor gives up and marks the plugin dead.
const crashCap = 3

// restartBaseDelay is the first backoff interval. Each
// subsequent restart doubles the delay (capped at restartMaxDelay).
const restartBaseDelay = 100 * core.Millisecond

// restartMaxDelay caps exponential growth so a long-running
// plugin that crashes once after weeks doesn't wait forever.
const restartMaxDelay = 10 * core.Second

// pluginState extension fields used by the supervisor. Lives
// here rather than plugin.go so the supervision concern is
// self-contained.
//
// (Compile-time: these fields are added directly to pluginState
// in plugin.go via a separate edit; this block documents intent.)

// watchProcess runs in a goroutine for each Start. Blocks on
// proc.Done(), then decides whether to restart, give up, or do
// nothing (user-initiated Stop).
//
// Phase 2 design: the goroutine reads the plugin's state under
// s.mu, mutates the restart history, then either calls
// startPlugin again (with the existing lock held? No — releases
// the lock around restart spawning to avoid a deadlock with
// other API surface methods) or marks the plugin dead.
func (s *Service) watchProcess(code string, proc *process.Process) {
	<-proc.Done()
	s.handleExit(code, proc)
}

// handleExit is the post-Done() decision. Re-acquires s.mu so
// the caller (background goroutine) can safely mutate state.
func (s *Service) handleExit(code string, proc *process.Process) {
	s.mu.Lock()
	ps, ok := s.state[code]
	if !ok {
		// Plugin was Removed while we were blocked on Done().
		s.mu.Unlock()
		return
	}
	// The user-initiated path (Stop or Remove) sets ps.state
	// to "stopped" before killing the process. If we observe
	// that here it means the exit was expected — no restart.
	if ps.state == "stopped" {
		s.mu.Unlock()
		return
	}
	// Unexpected exit. Log + decide restart-or-die.
	info := proc.Info()
	ps.lastError = "exited with code " + core.Itoa(info.ExitCode)
	ps.proc = nil
	// Trim the crash log to entries inside the rolling window.
	now := core.Now()
	cutoff := now.Add(-crashWindow)
	pruned := ps.crashAt[:0]
	for _, t := range ps.crashAt {
		if t.After(cutoff) {
			pruned = append(pruned, t)
		}
	}
	ps.crashAt = append(pruned, now)
	if len(ps.crashAt) >= crashCap {
		ps.state = "dead"
		s.proxy.Delete(code)
		s.mu.Unlock()
		core.Warn("plugin.supervisor.dead",
			"code", code,
			"crashes", len(ps.crashAt),
			"window", crashWindow.String())
		return
	}
	// Backoff before restart. Computed under the lock so the
	// fairness story is simple; sleep happens after release.
	delay := backoffFor(len(ps.crashAt))
	ps.state = "restarting"
	ps.lastError += " (restart scheduled in " + delay.String() + ")"
	s.proxy.Delete(code) // drop the proxy mount while we wait
	s.mu.Unlock()
	core.Warn("plugin.supervisor.restart",
		"code", code,
		"attempt", len(ps.crashAt),
		"delay", delay.String())
	core.Sleep(delay)
	// Re-spawn via the normal path. Errors land back in
	// ps.state / ps.lastError and the watcher's next iteration
	// catches the new process's Done().
	ctx, cancel := core.WithTimeout(core.Background(), 60*core.Second)
	defer cancel()
	s.mu.Lock()
	tok := s.resolveToken()
	if r := s.startPlugin(ctx, code, tok); !r.OK {
		// Spawn failed — record and let the next crash-window
		// tick handle the retry. We don't re-watch a process
		// that doesn't exist.
		ps.lastError = "restart spawn failed: " + r.Error()
		ps.state = "dead"
		s.mu.Unlock()
		return
	}
	// startPlugin set up a new ps.proc + state; grab the new
	// process so the watcher can attach to its Done() channel.
	ps = s.state[code]
	newProc := ps.proc
	s.mu.Unlock()
	if newProc != nil && newProc.proc != nil {
		s.Core().Go(func() { s.watchProcess(code, newProc.proc) })
	}
}

// backoffFor returns the delay before the Nth restart attempt
// (N counted from 1). Exponential — base × 2^(N-1) capped at
// restartMaxDelay.
func backoffFor(attempt int) core.Duration {
	if attempt <= 1 {
		return restartBaseDelay
	}
	d := restartBaseDelay
	for i := 1; i < attempt; i++ {
		d *= 2
		if d >= restartMaxDelay {
			return restartMaxDelay
		}
	}
	return d
}
