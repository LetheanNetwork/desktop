// SPDX-Licence-Identifier: EUPL-1.2

// Tests for the Cerberus #50 (Mantis #1683) Repudiation gap close:
// Run / Start / Kill on pkg/process.Service now emit typed
// audit.EventProcess* rows via audit.Default() so the auditor sees
// lifecycle decisions instead of forensically silent spawns. Mirror of
// pkg/sandbox/audit_test.go (H#188 Cerberus #47 S-4) — the
// recordingRecorder shape is duplicated rather than shared because
// each test package's SetDefault is process-global and the fixture
// must not leak across packages running in parallel.
//
// The success paths for Run / Start are exercised via the
// validation-failure return from the upstream go-process action layer:
// a bare *core.Core has no "process.run" / "process.start" / "process.kill"
// action registered, so c.Action(...).Run(...) returns a non-OK Result
// with the "action not registered" diagnostic. That drives the
// Requested + Failed lifecycle pair end-to-end without spinning a real
// process runtime in CI.

package process

import (
	"sync"

	core "dappco.re/go"

	"dappco.re/lthn/desktop/pkg/audit"
)

// recordingRecorder captures every audit.Event the process service
// emits during a test. Mirror of the same-named fixture in
// pkg/sandbox/audit_test.go and pkg/runner/audit_test.go.
type recordingRecorder struct {
	mu     sync.Mutex
	events []audit.Event
}

func (r *recordingRecorder) Record(ev audit.Event) core.Result {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, ev)
	return core.Ok(nil)
}

func (r *recordingRecorder) snapshot() []audit.Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]audit.Event, len(r.events))
	copy(out, r.events)
	return out
}

// filterByName returns only events whose Event field matches name.
// Lets a test assert on the lifecycle pair it cares about without
// being brittle to unrelated sibling emits in the same package.
func (r *recordingRecorder) filterByName(name string) []audit.Event {
	all := r.snapshot()
	out := make([]audit.Event, 0, len(all))
	for _, ev := range all {
		if ev.Event == name {
			out = append(out, ev)
		}
	}
	return out
}

// installAuditRecorder wires a recordingRecorder as audit.Default and
// restores the prior default at test end.
func installAuditRecorder(t *core.T) *recordingRecorder {
	t.Helper()
	rec := &recordingRecorder{}
	audit.SetDefault(rec)
	t.Cleanup(func() { audit.SetDefault(nil) })
	return rec
}

// newTestService returns a Service bound to a bare *core.Core with no
// upstream go-process Service registered. Every Action dispatch
// returns "action not registered" — sufficient to drive the
// Requested + Failed audit lifecycle pair without a real runtime.
func newTestService(t *core.T) *Service {
	t.Helper()
	c := core.New()
	return NewService(c)
}

// TestProcess_Run_EmitsAuditTrail_Good drives Run() against a bare
// Core (no upstream process action registered) so the dispatch fails
// fast with "action not registered". Asserts the Requested + Failed
// lifecycle pair lands on the audit substrate with the expected Meta
// shape — command_hash is SHA-256 hex of the input; runtime is the
// reserved "core-process" literal; error_code carries the upstream
// failure diagnostic.
func TestProcess_Run_EmitsAuditTrail_Good(t *core.T) {
	rec := installAuditRecorder(t)
	svc := newTestService(t)

	r := svc.Run("echo", []string{"hello"})
	core.AssertFalse(t, r.OK,
		"Run against a Core with no process action registered must fail")

	requested := rec.filterByName(audit.EventProcessRunRequested)
	core.AssertEqual(t, 1, len(requested), "Run must emit exactly one Requested")
	core.AssertEqual(t, processScope, requested[0].Scope)
	core.AssertEqual(t, audit.OutcomeOK, requested[0].Outcome)
	commandHash, ok := requested[0].Meta["command_hash"].(string)
	core.AssertTrue(t, ok, "Requested Meta must carry command_hash as string")
	core.AssertEqual(t, 64, len(commandHash), "command_hash is SHA-256 hex (64 chars)")
	core.AssertEqual(t, core.SHA256HexString("echo"), commandHash)
	core.AssertEqual(t, processRuntime, requested[0].Meta["runtime"])

	failed := rec.filterByName(audit.EventProcessRunFailed)
	core.AssertEqual(t, 1, len(failed), "action-not-registered must emit Failed")
	core.AssertEqual(t, audit.OutcomeFailed, failed[0].Outcome)
	_, hasErrCode := failed[0].Meta["error_code"]
	core.AssertTrue(t, hasErrCode, "Failed Meta must carry error_code key")

	// Defensive: zero Succeeded rows on a failure path.
	succeeded := rec.filterByName(audit.EventProcessRunSucceeded)
	core.AssertEqual(t, 0, len(succeeded), "Failed path must not emit Succeeded")
}

// TestProcess_Start_EmitsAuditTrail_Good mirrors the Run test shape
// for the background-detach Start() entrypoint. Same dispatch-failure
// scaffolding; same lifecycle-pair assertions tuned for the Start
// event names.
func TestProcess_Start_EmitsAuditTrail_Good(t *core.T) {
	rec := installAuditRecorder(t)
	svc := newTestService(t)

	r := svc.Start("sleep", []string{"60"})
	core.AssertFalse(t, r.OK)

	requested := rec.filterByName(audit.EventProcessStartRequested)
	core.AssertEqual(t, 1, len(requested), "Start must emit Requested")
	core.AssertEqual(t, processScope, requested[0].Scope)
	core.AssertEqual(t, audit.OutcomeOK, requested[0].Outcome)
	commandHash, ok := requested[0].Meta["command_hash"].(string)
	core.AssertTrue(t, ok)
	core.AssertEqual(t, 64, len(commandHash))
	core.AssertEqual(t, core.SHA256HexString("sleep"), commandHash)
	core.AssertEqual(t, processRuntime, requested[0].Meta["runtime"])

	failed := rec.filterByName(audit.EventProcessStartFailed)
	core.AssertEqual(t, 1, len(failed), "action-not-registered must emit Failed")
	core.AssertEqual(t, audit.OutcomeFailed, failed[0].Outcome)
	_, hasErrCode := failed[0].Meta["error_code"]
	core.AssertTrue(t, hasErrCode)

	succeeded := rec.filterByName(audit.EventProcessStartSucceeded)
	core.AssertEqual(t, 0, len(succeeded))
}

// TestProcess_Kill_EmitsAuditTrail_Good drives Kill() against a bare
// Core. The Requested row commits regardless of registry state so the
// emit shape is identical to a real-runtime kill that targets a
// not-found id.
func TestProcess_Kill_EmitsAuditTrail_Good(t *core.T) {
	rec := installAuditRecorder(t)
	svc := newTestService(t)

	id := "id-1-3cfc5d"
	r := svc.Kill(id)
	core.AssertFalse(t, r.OK)

	requested := rec.filterByName(audit.EventProcessKillRequested)
	core.AssertEqual(t, 1, len(requested), "Kill must emit Requested")
	core.AssertEqual(t, processScope, requested[0].Scope)
	core.AssertEqual(t, audit.OutcomeOK, requested[0].Outcome)
	core.AssertEqual(t, id, requested[0].Meta["process_id"])

	failed := rec.filterByName(audit.EventProcessKillFailed)
	core.AssertEqual(t, 1, len(failed), "action-not-registered must emit Failed")
	core.AssertEqual(t, audit.OutcomeFailed, failed[0].Outcome)
	core.AssertEqual(t, id, failed[0].Meta["process_id"])
	_, hasErrCode := failed[0].Meta["error_code"]
	core.AssertTrue(t, hasErrCode)

	succeeded := rec.filterByName(audit.EventProcessKillSucceeded)
	core.AssertEqual(t, 0, len(succeeded), "Failed path must not emit Succeeded")
}

// TestProcess_AuditMeta_NoRawCommandOrArgs_Good asserts the Cerberus
// #1465 closure-only-scope discipline at the process lifecycle
// boundary: neither raw Command bytes nor raw Args / Env values appear
// in any Meta payload across Run / Start / Kill. The redactor in
// pkg/audit also enforces this at the sink layer; the emit-site MUST
// NOT supply these in the first place (defence in depth).
func TestProcess_AuditMeta_NoRawCommandOrArgs_Good(t *core.T) {
	rec := installAuditRecorder(t)
	svc := newTestService(t)

	const sensitiveCmd = "SUPERSECRETCMDDONOTPERSIST"
	const sensitiveArg = "SUPERSECRETARGDONOTPERSIST"

	// Drive Run — action-not-registered reject; we just want the Meta
	// shape on the Requested + Failed pair.
	_ = svc.Run(sensitiveCmd, []string{sensitiveArg})

	// Drive Start — same scaffolding.
	_ = svc.Start(sensitiveCmd, []string{sensitiveArg})

	// Drive Kill — id is the only string input; ensure it's not echoed
	// back as a raw "command" / "args" key.
	_ = svc.Kill("id-with-" + sensitiveArg)

	for _, ev := range rec.snapshot() {
		// Top-level fields must not carry the sensitive bytes.
		core.AssertFalse(t, core.Contains(ev.Event, sensitiveCmd))
		core.AssertFalse(t, core.Contains(ev.Event, sensitiveArg))
		core.AssertFalse(t, core.Contains(ev.Scope, sensitiveCmd))
		core.AssertFalse(t, core.Contains(ev.Scope, sensitiveArg))
		// Walk Meta string values.
		for k, v := range ev.Meta {
			if s, ok := v.(string); ok {
				core.AssertFalse(t, core.Contains(s, sensitiveCmd),
					"Meta["+k+"] must not echo command bytes")
				// The process_id Meta key on Kill events legitimately
				// carries the caller-supplied id which contains the
				// sensitiveArg literal in this test (it's not a secret
				// — Args are; process_id is a registry identifier).
				if k == "process_id" {
					continue
				}
				core.AssertFalse(t, core.Contains(s, sensitiveArg),
					"Meta["+k+"] must not echo arg bytes")
			}
		}
		// Reserved smuggle-surface canaries — the emit-site is the only
		// place to enforce these so an audit reader can trust the schema.
		_, hasArgs := ev.Meta["args"]
		core.AssertFalse(t, hasArgs, "Meta must never carry an `args` key")
		_, hasEnv := ev.Meta["env"]
		core.AssertFalse(t, hasEnv, "Meta must never carry an `env` key")
		_, hasCommand := ev.Meta["command"]
		core.AssertFalse(t, hasCommand,
			"Meta must never carry a raw `command` key (only command_hash)")
		_, hasCwd := ev.Meta["cwd"]
		core.AssertFalse(t, hasCwd, "Meta must never carry a raw `cwd` key")
	}
}
