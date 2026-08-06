// SPDX-Licence-Identifier: EUPL-1.2

// service_internal_test.go — direct cover for the package-level
// Register wiring function, OnStartup's inert contract, the Snapshot
// switch arms that only fire on a Lifecycle.Get failure / unknown
// state / catalogue failure (none of runtimeFixture's happy-path
// scenarios in service_test.go provoke these), and the small pure
// mapping helpers (managedState, mapClientCode, mapCatalogueFailure's
// default arm, cloneUint's non-nil branch).

package modelruntime

import (
	core "dappco.re/go"
	coreio "dappco.re/go/io"
	"dappco.re/lthn/desktop/pkg/models"
	"dappco.re/lthn/desktop/pkg/services"
)

// --- Register (package-level wiring) ---------------------------------

func TestRegister_NilCore_Bad(t *core.T) {
	result := Register(nil)
	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ErrorRuntimeUnavailable, ErrorCodeOf(result))
}

func TestRegister_ServicesRuntimeMissing_Bad(t *core.T) {
	c := core.New()
	result := Register(c)
	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ErrorRuntimeUnavailable, ErrorCodeOf(result))
}

func TestRegister_LemIOMissing_Bad(t *core.T) {
	manager := services.NewService(services.Options{})
	c := core.New(core.WithName("services", manager.Register))
	result := Register(c)
	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ErrorCatalogueUnavailable, ErrorCodeOf(result))
}

// TestRegister_LemIORootEmpty_Bad — an "lem-io" service backed by the
// unsandboxed package-level Medium (IOConfig{} with no Root) reports
// Options().Root == "" — Register must refuse rather than wire a
// catalogue against an unbounded root.
func TestRegister_LemIORootEmpty_Bad(t *core.T) {
	manager := services.NewService(services.Options{})
	c := core.New(
		core.WithName("services", manager.Register),
		core.WithName("lem-io", coreio.NewService(coreio.IOConfig{})),
	)
	result := Register(c)
	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ErrorCatalogueUnavailable, ErrorCodeOf(result))
}

// TestRegister_Good_WiresCatalogueAgainstSandboxedRoot — full wiring
// with a real sandboxed lem-io root succeeds and produces a *Service
// registered under "modelruntime".
func TestRegister_Good_WiresCatalogueAgainstSandboxedRoot(t *core.T) {
	lemRoot := t.TempDir()
	manager := services.NewService(services.Options{})
	c := core.New(
		core.WithName("services", manager.Register),
		core.WithName("lem-io", coreio.NewService(coreio.IOConfig{Root: lemRoot})),
		core.WithName("modelruntime", Register),
	)
	registered, ok := core.ServiceFor[*Service](c, "modelruntime")
	core.RequireTrue(t, ok, "modelruntime must have registered")
	core.RequireTrue(t, registered != nil)
	core.AssertTrue(t, registered.options.Catalogue != nil)
	core.AssertTrue(t, registered.options.Client != nil)
	core.AssertTrue(t, registered.options.Credentials != nil)
}

// --- OnStartup ----------------------------------------------------------

func TestService_OnStartup_Inert_Good(t *core.T) {
	service := NewService(Options{})
	result := service.OnStartup(core.Background())
	core.AssertTrue(t, result.OK)
}

// --- Snapshot switch arms not reached by the fixture happy path -------

func TestService_Snapshot_Bad_LifecycleGetFails(t *core.T) {
	fixture := newRuntimeFixture(t)
	fixture.lifecycle.getResult = core.Fail(core.NewError("lifecycle down"))
	fixture.lifecycle.hasGetResult = true

	result := fixture.runtime.Snapshot()

	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ErrorRuntimeUnavailable, ErrorCodeOf(result))
}

func TestService_Snapshot_Bad_LifecycleGetInvalidType(t *core.T) {
	fixture := newRuntimeFixture(t)
	fixture.lifecycle.getResult = core.Ok("not-a-snapshot")
	fixture.lifecycle.hasGetResult = true

	result := fixture.runtime.Snapshot()

	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ErrorRuntimeUnavailable, ErrorCodeOf(result))
}

func TestService_Snapshot_Bad_UnknownLifecycleState(t *core.T) {
	fixture := newRuntimeFixture(t)
	fixture.lifecycle.snapshot.State = services.State("something-new")

	result := fixture.runtime.Snapshot()

	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ErrorRuntimeUnavailable, ErrorCodeOf(result))
}

func TestService_Snapshot_Bad_ManagedFailedState(t *core.T) {
	fixture := newRuntimeFixture(t)
	fixture.lifecycle.snapshot.State = services.StateFailed

	result := fixture.runtime.Snapshot()

	snapshot := result.Value.(Snapshot)
	core.AssertEqual(t, StateFailed, snapshot.State)
	core.RequireTrue(t, snapshot.LastError != nil)
	core.AssertEqual(t, ErrorRuntimeStartFailed, snapshot.LastError.Code)
}

func TestService_Snapshot_Good_StartingAndStoppingStates(t *core.T) {
	fixture := newRuntimeFixture(t)
	fixture.lifecycle.snapshot.State = services.StateStarting

	starting := fixture.runtime.Snapshot()
	core.RequireTrue(t, starting.OK, starting.Error())
	core.AssertEqual(t, StateStarting, starting.Value.(Snapshot).State)

	fixture.lifecycle.snapshot.State = services.StateStopping
	stopping := fixture.runtime.Snapshot()
	core.RequireTrue(t, stopping.OK, stopping.Error())
	core.AssertEqual(t, StateStopping, stopping.Value.(Snapshot).State)
}

// TestService_Snapshot_Bad_CatalogueFailureWhileManagedRunning — the
// catalogue failing mid-reconcile hits publishCatalogueFailure, which
// routes managed.State through managedState(); Running maps to
// StateModelLess in that projection.
func TestService_Snapshot_Bad_CatalogueFailureWhileManagedRunning(t *core.T) {
	fixture := newRuntimeFixture(t)
	fixture.lifecycle.snapshot.State = services.StateRunning
	fixture.lifecycle.snapshot.Desired = true
	fixture.runtime.options.Catalogue = nil

	result := fixture.runtime.Snapshot()

	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ErrorCatalogueUnavailable, ErrorCodeOf(result))
}

// --- managedState / mapClientCode / mapCatalogueFailure / cloneUint ---

func TestManagedState_AllBranches_Good(t *core.T) {
	cases := map[services.State]State{
		services.StateStopped:  StateStopped,
		services.StateExited:   StateStopped,
		services.StateStarting: StateStarting,
		services.StateRunning:  StateModelLess,
		services.StateStopping: StateStopping,
		services.State("odd"):  StateFailed,
	}
	for in, want := range cases {
		core.AssertEqual(t, want, managedState(in))
	}
}

func TestMapClientCode_AllBranches_Good(t *core.T) {
	cases := map[ClientErrorCode]ErrorCode{
		ClientUnauthorised:     ErrorAdminUnauthorised,
		ClientResponseInvalid:  ErrorResponseInvalid,
		ClientResponseTooLarge: ErrorResponseTooLarge,
		ClientRequestTimeout:   ErrorRequestTimeout,
		ClientUnavailable:      ErrorRuntimeNotReady,
		ClientErrorCode("odd"): "",
	}
	for in, want := range cases {
		core.AssertEqual(t, want, mapClientCode(in))
	}
}

func TestMapCatalogueFailure_DefaultBranch_Bad(t *core.T) {
	foreign := core.Fail(core.NewError("unrelated"))
	result := mapCatalogueFailure("op", foreign)
	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ErrorCatalogueUnavailable, ErrorCodeOf(result))
}

func TestMapCatalogueFailure_ModelNotFound_Bad(t *core.T) {
	failed := core.Fail(&models.CatalogueFailure{Code: models.CatalogueModelNotFound})
	result := mapCatalogueFailure("op", failed)
	core.AssertEqual(t, ErrorModelNotFound, ErrorCodeOf(result))
}

func TestCloneUint_NonNil_Good(t *core.T) {
	value := uint64(42)
	clone := cloneUint(&value)
	core.RequireTrue(t, clone != nil)
	core.AssertEqual(t, value, *clone)
	// Independent allocation — mutating the clone must not touch value.
	*clone = 99
	core.AssertEqual(t, uint64(42), value)
}

func TestCloneUint_Nil_Good(t *core.T) {
	core.AssertTrue(t, cloneUint(nil) == nil)
}
