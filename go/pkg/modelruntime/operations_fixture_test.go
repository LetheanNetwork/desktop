// SPDX-Licence-Identifier: EUPL-1.2

// operations_fixture_test.go — fixture-driven cover for operations.go
// branches service_test.go's happy-path scenarios never provoke:
// ensureRunning's own failure modes called directly (white-box),
// withCredential's nil-dependency / re-auth-failure branches, Stop's
// early-return ladder, beginMutation's nil-service / shutting-down
// guards, and Load/Unload/Restart's lifecycle-nil and lifecycle-
// failure paths.

package modelruntime

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/services"
)

// --- ensureRunning ------------------------------------------------------

func TestEnsureRunning_LifecycleNil_Bad(t *core.T) {
	service := NewService(Options{})
	result := service.ensureRunning()
	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ErrorRuntimeUnavailable, ErrorCodeOf(result))
}

func TestEnsureRunning_GetFails_Bad(t *core.T) {
	fixture := newRuntimeFixture(t)
	fixture.lifecycle.getResult = core.Fail(core.NewError("down"))
	fixture.lifecycle.hasGetResult = true

	result := fixture.runtime.ensureRunning()

	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ErrorRuntimeUnavailable, ErrorCodeOf(result))
}

func TestEnsureRunning_GetInvalidType_Bad(t *core.T) {
	fixture := newRuntimeFixture(t)
	fixture.lifecycle.getResult = core.Ok("not-a-snapshot")
	fixture.lifecycle.hasGetResult = true

	result := fixture.runtime.ensureRunning()

	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ErrorResponseInvalid, ErrorCodeOf(result))
}

func TestEnsureRunning_StartSucceedsButNotRunning_Bad(t *core.T) {
	fixture := newRuntimeFixture(t)
	fixture.lifecycle.startResult = core.Ok(services.Snapshot{State: services.StateStarting})
	fixture.lifecycle.hasStartResult = true

	result := fixture.runtime.ensureRunning()

	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ErrorRuntimeStartFailed, ErrorCodeOf(result))
}

// --- withCredential -------------------------------------------------

func TestWithCredential_NoCredentialsProvider_Bad(t *core.T) {
	service := NewService(Options{})
	result := service.withCredential(core.Background(), func(string) core.Result {
		return core.Ok(nil)
	})
	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ErrorAdminUnauthorised, ErrorCodeOf(result))
}

func TestWithCredential_NilCall_Bad(t *core.T) {
	fixture := newRuntimeFixture(t)
	result := fixture.runtime.withCredential(core.Background(), nil)
	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ErrorAdminUnauthorised, ErrorCodeOf(result))
}

func TestWithCredential_CredentialUnavailable_Bad(t *core.T) {
	fixture := newRuntimeFixture(t)
	fixture.credentials.results = []core.Result{
		core.Fail(&CredentialFailure{Code: CredentialUnavailable, Message: "no token"}),
	}

	result := fixture.runtime.withCredential(core.Background(), func(string) core.Result {
		return core.Ok(nil)
	})

	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ErrorAdminUnauthorised, ErrorCodeOf(result))
}

// TestWithCredential_UnauthorisedThenContextCancelled_Bad — the call
// reports Unauthorised, the credential gets invalidated, but ctx is
// already cancelled by the time withCredential checks — the re-auth
// attempt must not proceed.
func TestWithCredential_UnauthorisedThenContextCancelled_Bad(t *core.T) {
	fixture := newRuntimeFixture(t)
	ctx, cancel := core.WithCancel(core.Background())
	cancel()

	result := fixture.runtime.withCredential(ctx, func(string) core.Result {
		return core.Fail(&ClientFailure{Code: ClientUnauthorised, Message: "denied"})
	})

	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ErrorAdminUnauthorised, ErrorCodeOf(result))
	core.AssertEqual(t, 1, fixture.credentials.invalidates)
}

// TestWithCredential_UnauthorisedThenRefreshFails_Bad — after
// invalidating, the second Credential() read also fails; withCredential
// must surface that failure rather than retrying the call.
func TestWithCredential_UnauthorisedThenRefreshFails_Bad(t *core.T) {
	fixture := newRuntimeFixture(t)
	fixture.credentials.results = []core.Result{
		core.Ok(fixture.credentials.token),
		core.Fail(&CredentialFailure{Code: CredentialUnavailable, Message: "gone"}),
	}
	calls := 0

	result := fixture.runtime.withCredential(core.Background(), func(string) core.Result {
		calls++
		return core.Fail(&ClientFailure{Code: ClientUnauthorised, Message: "denied"})
	})

	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ErrorAdminUnauthorised, ErrorCodeOf(result))
	core.AssertEqual(t, 1, calls, "the wrapped call must only run once when re-auth can't get a fresh credential")
	core.AssertEqual(t, 1, fixture.credentials.invalidates)
}

// --- Stop -----------------------------------------------------------

func TestService_Stop_NilReceiver_Bad(t *core.T) {
	var service *Service
	result := service.Stop()
	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ErrorRuntimeUnavailable, ErrorCodeOf(result))
}

func TestService_Stop_ShuttingDown_Bad(t *core.T) {
	fixture := newRuntimeFixture(t)
	core.RequireTrue(t, fixture.runtime.OnShutdown(core.Background()).OK)

	result := fixture.runtime.Stop()

	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ErrorRuntimeUnavailable, ErrorCodeOf(result))
}

func TestService_Stop_LifecycleNil_Bad(t *core.T) {
	service := NewService(Options{})
	result := service.Stop()
	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ErrorRuntimeUnavailable, ErrorCodeOf(result))
}

func TestService_Stop_LifecycleStopFails_Bad(t *core.T) {
	fixture := newRuntimeFixture(t)
	fixture.runtime.options.Lifecycle = &fakeStopFailsLifecycle{fakeLifecycle: fixture.lifecycle}

	result := fixture.runtime.Stop()

	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ErrorRuntimeStopFailed, ErrorCodeOf(result))
}

func TestService_Stop_CatalogueFails_Bad(t *core.T) {
	fixture := newRuntimeFixture(t)
	fixture.runtime.options.Catalogue = nil

	result := fixture.runtime.Stop()

	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ErrorCatalogueUnavailable, ErrorCodeOf(result))
}

// fakeStopFailsLifecycle wraps fakeLifecycle and forces Stop() to fail
// — a dedicated override rather than another field on fakeLifecycle
// because Stop has no natural "override result" precedent among the
// other three methods yet and this is the only test that needs it.
type fakeStopFailsLifecycle struct {
	*fakeLifecycle
}

func (l *fakeStopFailsLifecycle) Stop(id string) core.Result {
	_ = l.fakeLifecycle.Stop(id)
	return core.Fail(core.NewError("stop failed"))
}

// --- beginMutation ----------------------------------------------------

func TestBeginMutation_NilReceiver_Bad(t *core.T) {
	var service *Service
	ctx, release, result := service.beginMutation(StateStarting, "start")
	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ErrorRuntimeUnavailable, ErrorCodeOf(result))
	core.AssertTrue(t, ctx == nil)
	release() // must not panic
}

func TestBeginMutation_ShuttingDown_Bad(t *core.T) {
	fixture := newRuntimeFixture(t)
	core.RequireTrue(t, fixture.runtime.OnShutdown(core.Background()).OK)

	_, release, result := fixture.runtime.beginMutation(StateStarting, "start")

	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ErrorRuntimeUnavailable, ErrorCodeOf(result))
	release()
}

// --- Load / Unload / Restart gaps --------------------------------------

func TestService_Load_CatalogueNil_Bad(t *core.T) {
	fixture := newRuntimeFixture(t)
	fixture.runtime.options.Catalogue = nil

	result := fixture.runtime.Load(LoadRequest{ModelID: "model-1111111111111111"})

	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ErrorCatalogueUnavailable, ErrorCodeOf(result))
}

func TestService_Unload_LifecycleNil_Bad(t *core.T) {
	fixture := newRuntimeFixture(t)
	fixture.runtime.options.Lifecycle = nil

	result := fixture.runtime.Unload()

	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ErrorRuntimeUnavailable, ErrorCodeOf(result))
}

func TestService_Unload_RestartFails_Bad(t *core.T) {
	fixture := newRuntimeFixture(t)
	fixture.lifecycle.snapshot.State = services.StateRunning
	fixture.lifecycle.restartResult = core.Fail(core.NewError("restart failed"))
	fixture.lifecycle.hasRestartResult = true

	result := fixture.runtime.Unload()

	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ErrorModelUnloadFailed, ErrorCodeOf(result))
}

func TestService_Unload_RestartInvalidType_Bad(t *core.T) {
	fixture := newRuntimeFixture(t)
	fixture.lifecycle.restartResult = core.Ok("not-a-snapshot")
	fixture.lifecycle.hasRestartResult = true

	result := fixture.runtime.Unload()

	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ErrorModelUnloadFailed, ErrorCodeOf(result))
}

func TestService_Unload_CatalogueFails_Bad(t *core.T) {
	fixture := newRuntimeFixture(t)
	fixture.lifecycle.snapshot.State = services.StateRunning
	fixture.runtime.options.Catalogue = nil

	result := fixture.runtime.Unload()

	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ErrorCatalogueUnavailable, ErrorCodeOf(result))
}

func TestService_Restart_LifecycleNil_Bad(t *core.T) {
	fixture := newRuntimeFixture(t)
	fixture.runtime.options.Lifecycle = nil

	result := fixture.runtime.Restart()

	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ErrorRuntimeUnavailable, ErrorCodeOf(result))
}

func TestService_Restart_RestartFails_Bad(t *core.T) {
	fixture := newRuntimeFixture(t)
	fixture.lifecycle.restartResult = core.Fail(core.NewError("restart failed"))
	fixture.lifecycle.hasRestartResult = true

	result := fixture.runtime.Restart()

	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ErrorRuntimeStartFailed, ErrorCodeOf(result))
}

func TestService_Restart_RestartInvalidType_Bad(t *core.T) {
	fixture := newRuntimeFixture(t)
	fixture.lifecycle.restartResult = core.Ok(42)
	fixture.lifecycle.hasRestartResult = true

	result := fixture.runtime.Restart()

	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ErrorRuntimeStartFailed, ErrorCodeOf(result))
}

func TestService_Restart_CatalogueFails_Bad(t *core.T) {
	fixture := newRuntimeFixture(t)
	fixture.runtime.options.Catalogue = nil

	result := fixture.runtime.Restart()

	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ErrorCatalogueUnavailable, ErrorCodeOf(result))
}

func TestService_Start_CatalogueFails_Bad(t *core.T) {
	fixture := newRuntimeFixture(t)
	fixture.runtime.options.Catalogue = nil

	result := fixture.runtime.Start()

	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ErrorCatalogueUnavailable, ErrorCodeOf(result))
}
