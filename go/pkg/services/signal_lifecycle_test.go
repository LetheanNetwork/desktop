// SPDX-Licence-Identifier: EUPL-1.2

package services

import (
	core "dappco.re/go"
)

/*
The restart-policy interaction is the reason this file exists.

A managed service with a restart policy is watched by an exit reconciler. If a
signal ends the process, the reconciler sees an exit and restarts it — so a
kill that does not clear desired-running state first appears to work and is
undone a second later. These pin which operations mean "I want this stopped"
and which do not.
*/

func TestService_Kill_GoodClearsDesiredBeforeDelivering(t *core.T) {
	fixture := newServiceFixture(t)
	core.RequireTrue(t, fixture.service.Start(fixture.definition.ID).OK)

	result := fixture.service.Kill(fixture.definition.ID)

	core.RequireTrue(t, result.OK, result.Error())
	snapshot := result.Value.(Snapshot)

	core.AssertEqual(t, StateStopped, snapshot.State)
	// The load-bearing assertion. Desired must be false, or the reconciler
	// restarts what was just killed.
	core.AssertFalse(t, snapshot.Desired, "kill must clear desired-running state")
	core.AssertEqual(t, "", snapshot.ProcessID)
	core.AssertEqual(t, 0, snapshot.PID)
	core.AssertEqual(t, []string{"proc-1"}, fixture.runtime.recordedKills())
	// Kill does not ask politely, so the graceful path must not have run.
	core.AssertEqual(t, 0, fixture.process.shutdownCalls)
}

func TestService_Signal_GoodLeavesDesiredAlone(t *core.T) {
	fixture := newServiceFixture(t)
	core.RequireTrue(t, fixture.service.Start(fixture.definition.ID).OK)

	result := fixture.service.Signal(SignalRequest{
		ID:     fixture.definition.ID,
		Signal: SignalTerminate,
	})

	core.RequireTrue(t, result.OK, result.Error())

	// A signal is a message, not a decision. If terminate causes a
	// well-behaved service to exit and its policy says restart, restarting is
	// correct — which only happens if desired is still true.
	after := fixture.snapshot(t)
	core.AssertTrue(t, after.Desired, "a bare signal must not clear desired-running state")
	core.AssertEqual(t, StateRunning, after.State, "a signal must not invent a state change")

	delivered := fixture.runtime.recordedSignals()
	core.RequireTrue(t, len(delivered) == 1, "exactly one delivery expected")
	core.AssertEqual(t, "proc-1", delivered[0].ID)
	core.AssertEqual(t, SignalTerminate, delivered[0].Signal)
}

func TestService_Signal_GoodDeliversEachNamedSignal(t *core.T) {
	for _, name := range []Signal{SignalTerminate, SignalInterrupt, SignalHangup, SignalKill} {
		fixture := newServiceFixture(t)
		core.RequireTrue(t, fixture.service.Start(fixture.definition.ID).OK)

		result := fixture.service.Signal(SignalRequest{ID: fixture.definition.ID, Signal: name})

		core.RequireTrue(t, result.OK, result.Error())
		delivered := fixture.runtime.recordedSignals()
		core.RequireTrue(t, len(delivered) == 1, "exactly one delivery expected")
		core.AssertEqual(t, name, delivered[0].Signal)
	}
}

func TestService_Signal_BadRefusesAStoppedService(t *core.T) {
	fixture := newServiceFixture(t)

	result := fixture.service.Signal(SignalRequest{
		ID:     fixture.definition.ID,
		Signal: SignalTerminate,
	})

	// Refused, not quietly successful: a script signalling a stopped service
	// has a bug, and reporting success is how that bug survives.
	core.AssertFalse(t, result.OK, "signalling a stopped service must be refused")
	core.AssertEqual(t, ErrorServiceNotRunning, ErrorCodeOf(result))
	core.AssertEqual(t, 0, len(fixture.runtime.recordedSignals()))
}

func TestService_Signal_BadRefusesAnUnknownDefinition(t *core.T) {
	fixture := newServiceFixture(t)

	result := fixture.service.Signal(SignalRequest{ID: "no-such-service", Signal: SignalTerminate})

	core.AssertFalse(t, result.OK, "an unknown definition must be refused")
	core.AssertEqual(t, ErrorDefinitionNotFound, ErrorCodeOf(result))
	core.AssertEqual(t, 0, len(fixture.runtime.recordedSignals()))
}

func TestService_Signal_BadRefusesAnUnknownSignalBeforeTouchingTheRuntime(t *core.T) {
	fixture := newServiceFixture(t)
	core.RequireTrue(t, fixture.service.Start(fixture.definition.ID).OK)

	result := fixture.service.Signal(SignalRequest{
		ID:     fixture.definition.ID,
		Signal: Signal("9"),
	})

	core.AssertFalse(t, result.OK, "a signal number must be refused")
	core.AssertEqual(t, ErrorSignalUnknown, ErrorCodeOf(result))
	// Refused at the boundary — the runtime must never have been asked.
	core.AssertEqual(t, 0, len(fixture.runtime.recordedSignals()))
}

func TestService_Kill_BadRefusesAnUnknownDefinition(t *core.T) {
	fixture := newServiceFixture(t)

	result := fixture.service.Kill("no-such-service")

	core.AssertFalse(t, result.OK, "an unknown definition must be refused")
	core.AssertEqual(t, ErrorDefinitionNotFound, ErrorCodeOf(result))
	core.AssertEqual(t, 0, len(fixture.runtime.recordedKills()))
}

// Killing twice is what people do when the first one seemed not to work.
func TestService_Kill_UglyIsIdempotent(t *core.T) {
	fixture := newServiceFixture(t)
	core.RequireTrue(t, fixture.service.Start(fixture.definition.ID).OK)

	first := fixture.service.Kill(fixture.definition.ID)
	core.RequireTrue(t, first.OK, first.Error())

	second := fixture.service.Kill(fixture.definition.ID)

	core.RequireTrue(t, second.OK, second.Error())
	core.AssertEqual(t, StateStopped, second.Value.(Snapshot).State)
	core.AssertFalse(t, second.Value.(Snapshot).Desired)
	// The tree was killed once; the second call had nothing to reach.
	core.AssertEqual(t, 1, len(fixture.runtime.recordedKills()))
}

func TestService_Kill_UglyStoppedServiceSettlesWithoutTouchingTheRuntime(t *core.T) {
	fixture := newServiceFixture(t)

	result := fixture.service.Kill(fixture.definition.ID)

	core.RequireTrue(t, result.OK, result.Error())
	core.AssertEqual(t, StateStopped, result.Value.(Snapshot).State)
	core.AssertFalse(t, result.Value.(Snapshot).Desired)
	core.AssertEqual(t, 0, len(fixture.runtime.recordedKills()))
}

// The audit trail records the name, never the kernel constant — a reader
// should not have to know what 15 means on which platform.
func TestService_Signal_GoodAuditsTheNameNotTheNumber(t *core.T) {
	fixture := newServiceFixture(t)
	core.RequireTrue(t, fixture.service.Start(fixture.definition.ID).OK)

	core.RequireTrue(t, fixture.service.Signal(SignalRequest{
		ID:     fixture.definition.ID,
		Signal: SignalHangup,
	}).OK)

	var requested map[string]any
	for _, event := range fixture.audit.snapshot() {
		if event.Event == "service.signal.requested" {
			requested = event.Meta
		}
	}

	core.RequireTrue(t, requested != nil, "a signal request must be audited")
	core.AssertEqual(t, "hangup", requested["signal"])
	_, hasPolicy := requested["restart_policy"]
	core.AssertFalse(t, hasPolicy, "a signal is not a restart policy")
}
