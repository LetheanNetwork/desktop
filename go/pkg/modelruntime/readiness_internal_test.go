// SPDX-Licence-Identifier: EUPL-1.2

// readiness_internal_test.go — direct cover for waitHealth/waitLoaded,
// the exponential-backoff readiness-polling loops behind Start/Load.
// Every existing scenario in service_test.go only ever exercises their
// first-attempt-succeeds fast path (or, for cancellation, the mid-
// Health-call ctx.Err() check via a full Load()+Stop() dance). The
// retry-on-transient-failure branch, the "exhausted all attempts"
// terminal branch, and the top-of-loop pre-flight cancellation check
// were all dark — real fault injection (queued failing responses, a
// cancelled context) rather than mocked-away control flow.

package modelruntime

import core "dappco.re/go"

func TestWaitHealth_CancelledBeforeFirstAttempt_Bad(t *core.T) {
	fixture := newRuntimeFixture(t)
	ctx, cancel := core.WithCancel(core.Background())
	cancel()

	result := fixture.runtime.waitHealth(ctx)

	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ErrorRuntimeNotReady, ErrorCodeOf(result))
	core.AssertContains(t, result.Error(), "cancelled")
	core.AssertLen(t, fixture.client.callSnapshot(), 0, "a pre-cancelled context must never reach the client")
}

// TestWaitHealth_RetriesOnInvalidStatusThenSucceeds_Good — the first
// poll returns a structurally-OK-but-not-ready Health (Status !=
// "ok"); waitHealth must reclassify it as a retryable failure, wait
// out the backoff, and succeed on the second poll.
func TestWaitHealth_RetriesOnInvalidStatusThenSucceeds_Good(t *core.T) {
	fixture := newRuntimeFixture(t)
	fixture.client.healthResults = []core.Result{
		core.Ok(Health{Status: "starting", Runtime: "go-inference"}),
		core.Ok(Health{Status: "ok", Runtime: "go-inference"}),
	}

	result := fixture.runtime.waitHealth(core.Background())

	core.RequireTrue(t, result.OK, result.Error())
	core.AssertEqual(t, "ok", result.Value.(Health).Status)
	core.AssertEqual(t, []string{"client.health", "client.health"}, fixture.client.callSnapshot())
}

// TestWaitHealth_ExhaustsAllRetriesThenFails_Bad — every attempt (all
// maxReadinessAttempts of them) returns a transport failure; the loop
// must exhaust cleanly and surface the terminal "did not become
// ready" message rather than looping forever or panicking on the
// attempt==last-1 boundary.
func TestWaitHealth_ExhaustsAllRetriesThenFails_Bad(t *core.T) {
	fixture := newRuntimeFixture(t)
	results := make([]core.Result, 0, maxReadinessAttempts)
	for i := 0; i < maxReadinessAttempts; i++ {
		results = append(results, core.Fail(&ClientFailure{
			Code:    ClientUnavailable,
			Message: "down",
		}))
	}
	fixture.client.healthResults = results

	result := fixture.runtime.waitHealth(core.Background())

	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ErrorRuntimeNotReady, ErrorCodeOf(result))
	core.AssertContains(t, result.Error(), "did not become ready")
	core.AssertEqual(t, maxReadinessAttempts, len(fixture.client.callSnapshot()))
}

// TestWaitLoaded_CancelledDuringHealthCall_Bad — cancelling the
// context while waitLoaded's Health() call is in flight must surface
// the "model load was cancelled" failure rather than the generic
// runtime-not-ready one waitHealth uses — different operation, distinct
// message.
func TestWaitLoaded_CancelledDuringHealthCall_Bad(t *core.T) {
	fixture := newRuntimeFixture(t)
	fixture.client.healthBlock = make(chan struct{})
	ctx, cancel := core.WithCancel(core.Background())

	done := make(chan core.Result, 1)
	go func() { done <- fixture.runtime.waitLoaded(ctx) }()
	waitForClientCalls(t, fixture.client, 1)
	cancel()

	result := <-done
	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ErrorModelLoadFailed, ErrorCodeOf(result))
	core.AssertContains(t, result.Error(), "cancelled")
}

// TestWaitLoaded_RetriesWhenStatusFailsThenSucceeds_Good — the model
// reports ready in Health on the first attempt, but the admin Status
// call fails (e.g. a transient credential race); waitLoaded must
// retry rather than surfacing the Status failure immediately, and
// succeed once Status recovers.
func TestWaitLoaded_RetriesWhenStatusFailsThenSucceeds_Good(t *core.T) {
	fixture := newRuntimeFixture(t)
	fixture.client.healthResults = []core.Result{
		core.Ok(Health{Status: "ok", Runtime: "go-inference", Models: []string{"gemma-4-e2b"}}),
		core.Ok(Health{Status: "ok", Runtime: "go-inference", Models: []string{"gemma-4-e2b"}}),
	}
	fixture.client.statusResults = []core.Result{
		core.Fail(&ClientFailure{Code: ClientUnavailable, Message: "down"}),
		core.Ok(Status{Runtime: "metal", LoadedAtUnix: 1785146400}),
	}

	result := fixture.runtime.waitLoaded(core.Background())

	core.RequireTrue(t, result.OK, result.Error())
	ready := result.Value.(loadedRuntime)
	core.AssertEqual(t, "metal", ready.status.Runtime)
}

// TestWaitLoaded_RetriesWhenModelNotYetInHealth_Good — the runtime is
// healthy but hasn't finished loading the model yet (empty Models
// list) on the first poll, hitting waitLoaded's "not ready" else
// branch; the second poll reports the model present.
func TestWaitLoaded_RetriesWhenModelNotYetInHealth_Good(t *core.T) {
	fixture := newRuntimeFixture(t)
	fixture.client.healthResults = []core.Result{
		core.Ok(Health{Status: "ok", Runtime: "go-inference", Models: []string{}}),
		core.Ok(Health{Status: "ok", Runtime: "go-inference", Models: []string{"gemma-4-e2b"}}),
	}
	fixture.client.statusResults = []core.Result{
		core.Ok(Status{Runtime: "metal", LoadedAtUnix: 1785146400}),
	}

	result := fixture.runtime.waitLoaded(core.Background())

	core.RequireTrue(t, result.OK, result.Error())
	core.AssertEqual(t, []string{
		"client.health",
		"client.health",
		"client.status",
	}, fixture.client.callSnapshot())
}

func TestWaitLoaded_ExhaustsAllRetriesThenFails_Bad(t *core.T) {
	fixture := newRuntimeFixture(t)
	results := make([]core.Result, 0, maxReadinessAttempts)
	for i := 0; i < maxReadinessAttempts; i++ {
		results = append(results, core.Ok(Health{Status: "ok", Models: []string{}}))
	}
	fixture.client.healthResults = results

	result := fixture.runtime.waitLoaded(core.Background())

	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ErrorModelLoadFailed, ErrorCodeOf(result))
	core.AssertContains(t, result.Error(), "did not become ready")
}
