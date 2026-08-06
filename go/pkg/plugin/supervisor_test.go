// SPDX-Licence-Identifier: EUPL-1.2

// Internal-package tests for supervisor.go — backoffFor + handleExit.
// package plugin (white-box).
//
// handleExit only ever calls proc.Info() (never proc.Done() — that's
// watchProcess's job), and Info() is nil-safe on a zero-value
// *process.ManagedProcess (it reads only exported fields plus a
// nil-checked unexported *core.Cmd). That means handleExit's entire
// crash-accounting/backoff/restart-attempt decision tree is reachable
// from a manually constructed *process.Process — no OS process, no
// process.Service spawn, no goroutine ever created. Real fault
// injection (an unresolvable restart target) exercises the "restart
// spawn failed -> dead" branch the same way runtime_test.go's
// startPlugin fault-injection tests do.
//
// watchProcess itself (`<-proc.Done(); s.handleExit(...)`) is NOT
// exercised: Done() reads an unexported channel field that only a real
// process.Service.StartWithOptions populates, and calling it on our
// synthetic proc would block forever on a nil channel — a structural
// exec boundary, documented as a deliberate leave-out.
package plugin

import (
	core "dappco.re/go"
	"dappco.re/go/process"
)

// ─── backoffFor ──────────────────────────────────────────────────────────

func TestSupervisor_backoffFor_Good_FirstAttemptIsBaseDelay(t *core.T) {
	core.AssertEqual(t, restartBaseDelay, backoffFor(1))
}

func TestSupervisor_backoffFor_Bad_ZeroOrNegativeTreatedAsFirst(t *core.T) {
	core.AssertEqual(t, restartBaseDelay, backoffFor(0))
	core.AssertEqual(t, restartBaseDelay, backoffFor(-1))
}

func TestSupervisor_backoffFor_Good_DoublesEachAttempt(t *core.T) {
	core.AssertEqual(t, restartBaseDelay*2, backoffFor(2))
	core.AssertEqual(t, restartBaseDelay*4, backoffFor(3))
}

func TestSupervisor_backoffFor_Ugly_CapsAtRestartMaxDelay(t *core.T) {
	core.AssertEqual(t, restartMaxDelay, backoffFor(20))
}

// ─── handleExit ──────────────────────────────────────────────────────────

func TestSupervisor_handleExit_Good_RemovedPluginIsNoOp(t *core.T) {
	svc := newTestService(t, core.New())
	// Plugin was Removed while the watcher was blocked on Done() — proc
	// is never dereferenced on this path, so nil is safe.
	svc.handleExit("never-tracked", nil)
	core.AssertEqual(t, 0, len(svc.state))
}

func TestSupervisor_handleExit_Good_ExpectedStopSkipsRestart(t *core.T) {
	svc := newTestService(t, core.New())
	svc.state["x"] = &pluginState{state: "stopped"}
	svc.handleExit("x", nil) // proc unused on this path too
	core.AssertEqual(t, "stopped", svc.state["x"].state)
	core.AssertEqual(t, 0, len(svc.state["x"].crashAt), "no crash recorded for an expected stop")
}

func TestSupervisor_handleExit_Ugly_CrashCapReachedMarksDead(t *core.T) {
	svc := newTestService(t, core.New())
	now := core.Now()
	svc.state["x"] = &pluginState{
		state:   "running",
		crashAt: []core.Time{now.Add(-2 * core.Second), now.Add(-1 * core.Second)}, // 2 prior, +1 now = crashCap
	}
	svc.proxy.Set("x", "http://127.0.0.1:1")
	proc := &process.Process{ID: "p1", Status: process.StatusFailed, ExitCode: 137}

	svc.handleExit("x", proc)

	core.AssertEqual(t, "dead", svc.state["x"].state)
	core.AssertContains(t, svc.state["x"].lastError, "exited with code 137")
	core.AssertFalse(t, svc.proxy.Has("x"), "proxy mount dropped on death")
	core.AssertEqual(t, 3, len(svc.state["x"].crashAt))
}

func TestSupervisor_handleExit_Ugly_CrashWindowPrunesOldEntries(t *core.T) {
	svc := newTestService(t, core.New())
	now := core.Now()
	// Two crashes far outside the 60s window — must be pruned, so the
	// cap (3) is NOT reached by this single new crash.
	svc.state["x"] = &pluginState{
		state:   "running",
		crashAt: []core.Time{now.Add(-10 * core.Minute), now.Add(-5 * core.Minute)},
	}
	proc := &process.Process{ID: "p1", Status: process.StatusFailed, ExitCode: 1}

	svc.handleExit("x", proc)

	// Pruned to just the new entry; below crashCap so the backoff/restart
	// path runs instead of the dead path (proven separately below).
	core.AssertEqual(t, 1, len(svc.state["x"].crashAt))
}

// TestSupervisor_handleExit_Bad_RestartAttemptFailsMarksDead drives the
// backoff-then-restart branch: crashCap isn't reached, so handleExit
// sleeps restartBaseDelay (100ms — a real but tiny, deterministic sleep)
// and calls startPlugin again. No manifest exists on disk for "x", so
// the restart attempt fails exactly like runtime_test.go's startPlugin
// fault-injection cases — real fault injection, never a real spawn.
func TestSupervisor_handleExit_Bad_RestartAttemptFailsMarksDead(t *core.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	c := core.New(core.WithService(process.Register))
	svc := newTestService(t, c)
	svc.state["x"] = &pluginState{state: "running"} // crashAt empty -> len 1 after append, below cap
	svc.proxy.Set("x", "http://127.0.0.1:1")
	proc := &process.Process{ID: "p1", Status: process.StatusFailed, ExitCode: 1}

	svc.handleExit("x", proc)

	core.AssertEqual(t, "dead", svc.state["x"].state)
	core.AssertContains(t, svc.state["x"].lastError, "restart spawn failed")
}
