// SPDX-Licence-Identifier: EUPL-1.2

// service_lifecycle_gap_test.go closes coverage gaps in OnStartup,
// EnsureDefinition, RemoveDefinition, and SetPolicy left by
// service_test.go / events_test.go / restart_test.go — those files
// only ever exercise the "everything is wired and this is a brand-new
// ID" happy paths. This file drives the readyLocked/recordBusy/
// ownership-conflict/save-failure branches those tests never reach.

package services

import (
	core "dappco.re/go"
	coreio "dappco.re/go/io"
)

// fakeWrongTypeCatalogue satisfies Catalogue but returns a Value that
// is not a CatalogueDocument on Load — OnStartup must fail closed
// rather than type-assert-panic.
type fakeWrongTypeCatalogue struct{}

func (fakeWrongTypeCatalogue) Load() core.Result                  { return core.Ok("not-a-document") }
func (fakeWrongTypeCatalogue) Save(CatalogueDocument) core.Result { return core.Ok(nil) }

// conditionalFailCatalogue wraps a real Catalogue and fails every Save
// once failAfter calls to Save have already succeeded — lets a test
// let fixture setup's own Save through, then fail the Save under test.
type conditionalFailCatalogue struct {
	Catalogue
	failAfter int
	saveCalls int
}

func (c *conditionalFailCatalogue) Save(document CatalogueDocument) core.Result {
	c.saveCalls++
	if c.saveCalls > c.failAfter {
		return core.Fail(core.E("test.Save", "injected save failure", nil))
	}
	return c.Catalogue.Save(document)
}

// ---- OnStartup ------------------------------------------------------------

func TestServiceOnStartup_Bad_NilProcessRuntime(t *core.T) {
	medium := coreio.NewMemoryMedium()
	service := NewService(Options{
		Catalogue: NewMediumCatalogue(medium, testCataloguePath, DefaultLimits()),
		Limits:    DefaultLimits(),
	})

	core.RequireTrue(t, service.OnStartup(core.Background()).OK, "OnStartup itself must not fail Core startup")
	view := service.Catalogue()
	core.AssertFalse(t, view.OK)
	core.AssertEqual(t, ErrorServicesUnavailable, ErrorCodeOf(view))
}

func TestServiceOnStartup_Bad_NilCatalogue(t *core.T) {
	runtime := &fakeProcessRuntime{processes: map[string]ProcessHandle{}}
	service := NewService(Options{Process: runtime, Limits: DefaultLimits()})

	core.RequireTrue(t, service.OnStartup(core.Background()).OK)
	view := service.Catalogue()
	core.AssertFalse(t, view.OK)
	core.AssertEqual(t, ErrorServicesUnavailable, ErrorCodeOf(view))
}

func TestServiceOnStartup_Bad_CatalogueReturnsWrongType(t *core.T) {
	runtime := &fakeProcessRuntime{processes: map[string]ProcessHandle{}}
	service := NewService(Options{
		Process:   runtime,
		Catalogue: fakeWrongTypeCatalogue{},
		Limits:    DefaultLimits(),
	})

	core.RequireTrue(t, service.OnStartup(core.Background()).OK)
	view := service.Catalogue()
	core.AssertFalse(t, view.OK)
	core.AssertEqual(t, ErrorCatalogueInvalid, ErrorCodeOf(view))
}

func TestServiceOnStartup_Bad_ComposeRecordsRejectsInvalidBuiltin(t *core.T) {
	medium := coreio.NewMemoryMedium()
	runtime := &fakeProcessRuntime{processes: map[string]ProcessHandle{}}
	invalidBuiltin := validDefinition()
	invalidBuiltin.Command = "" // fails ValidateDefinition inside composeRecords
	service := NewService(Options{
		Process:   runtime,
		Catalogue: NewMediumCatalogue(medium, testCataloguePath, DefaultLimits()),
		Builtins:  []Definition{invalidBuiltin},
		Limits:    DefaultLimits(),
	})

	core.RequireTrue(t, service.OnStartup(core.Background()).OK)
	view := service.Catalogue()
	core.AssertFalse(t, view.OK)
	core.AssertEqual(t, ErrorDefinitionInvalid, ErrorCodeOf(view))
}

func TestServiceOnStartup_Bad_ComposeRecordsRejectsDuplicateBuiltinID(t *core.T) {
	medium := coreio.NewMemoryMedium()
	runtime := &fakeProcessRuntime{processes: map[string]ProcessHandle{}}
	first := validDefinition()
	second := validDefinition() // same ID as first
	service := NewService(Options{
		Process:   runtime,
		Catalogue: NewMediumCatalogue(medium, testCataloguePath, DefaultLimits()),
		Builtins:  []Definition{first, second},
		Limits:    DefaultLimits(),
	})

	core.RequireTrue(t, service.OnStartup(core.Background()).OK)
	view := service.Catalogue()
	core.AssertFalse(t, view.OK)
	core.AssertEqual(t, ErrorDefinitionConflict, ErrorCodeOf(view))
}

func TestServiceOnStartup_Bad_ComposeRecordsRejectsPersistedOwnershipConflict(t *core.T) {
	medium := coreio.NewMemoryMedium()
	builtin := validDefinition()
	persistedConflict := validDefinition()
	persistedConflict.Owner = "someone-else"
	document := CatalogueDocument{
		Version:     CatalogueVersion,
		Definitions: []Definition{persistedConflict},
		UpdatedAt:   "2026-07-27T12:00:00Z",
	}
	catalogue := NewMediumCatalogue(medium, testCataloguePath, DefaultLimits())
	core.RequireTrue(t, catalogue.Save(document).OK)

	runtime := &fakeProcessRuntime{processes: map[string]ProcessHandle{}}
	service := NewService(Options{
		Process:   runtime,
		Catalogue: catalogue,
		Builtins:  []Definition{builtin},
		Limits:    DefaultLimits(),
	})

	core.RequireTrue(t, service.OnStartup(core.Background()).OK)
	view := service.Catalogue()
	core.AssertFalse(t, view.OK)
	core.AssertEqual(t, ErrorDefinitionConflict, ErrorCodeOf(view))
}

// ---- EnsureDefinition -------------------------------------------------------

func TestServiceEnsureDefinition_Bad_InvalidDefinitionNeverReachesLock(t *core.T) {
	fixture := newServiceFixture(t)
	invalid := validDefinition()
	invalid.Command = ""

	result := fixture.service.EnsureDefinition(invalid)

	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ErrorDefinitionInvalid, ErrorCodeOf(result))
}

func TestServiceEnsureDefinition_Bad_NotReady(t *core.T) {
	// A Service that never completed a successful OnStartup is not
	// "ready" — readyLocked must reject before touching any record.
	service := NewService(Options{Limits: DefaultLimits()})

	result := service.EnsureDefinition(validDefinition())

	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ErrorServicesUnavailable, ErrorCodeOf(result))
}

func TestServiceEnsureDefinition_Bad_BusyRecordRejected(t *core.T) {
	fixture := newServiceFixture(t)
	core.RequireTrue(t, fixture.service.Start(fixture.definition.ID).OK)

	result := fixture.service.EnsureDefinition(fixture.definition)

	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ErrorOperationInProgress, ErrorCodeOf(result))
}

// TestServiceEnsureDefinition_Good_UpdatesExistingOwnedDefinition is
// the missing "same ID, same owner, not busy" case —
// TestService_EnsureDefinition_GoodPersistsBeforePublishing only ever
// exercises a brand-new ID.
func TestServiceEnsureDefinition_Good_UpdatesExistingOwnedDefinition(t *core.T) {
	fixture := newServiceFixture(t)
	changed := fixture.definition
	changed.DisplayName = "Renamed by test"

	result := fixture.service.EnsureDefinition(changed)

	core.RequireTrue(t, result.OK, result.Error())
	core.AssertEqual(t, "Renamed by test", result.Value.(Snapshot).Definition.DisplayName)
	got := fixture.service.Get(fixture.definition.ID)
	core.RequireTrue(t, got.OK, got.Error())
	core.AssertEqual(t, "Renamed by test", got.Value.(Snapshot).Definition.DisplayName)
}

func TestServiceEnsureDefinition_Bad_SaveFailureIsForwarded(t *core.T) {
	fixture := newServiceFixture(t)
	faulty := &conditionalFailCatalogue{Catalogue: fixture.service.options.Catalogue, failAfter: 0}
	fixture.service.options.Catalogue = faulty

	newDef := validDefinition()
	newDef.ID = "second-service"
	result := fixture.service.EnsureDefinition(newDef)

	core.AssertFalse(t, result.OK)
	core.AssertContains(t, result.Error(), "injected save failure")
	core.AssertEqual(t, ErrorDefinitionNotFound, ErrorCodeOf(fixture.service.Get("second-service")),
		"a failed Save must not publish the in-memory record")
}

// ---- RemoveDefinition -------------------------------------------------------

func TestServiceRemoveDefinition_Bad_UnknownID(t *core.T) {
	fixture := newServiceFixture(t)

	result := fixture.service.RemoveDefinition(fixture.definition.Owner, "no-such-id")

	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ErrorDefinitionNotFound, ErrorCodeOf(result))
}

func TestServiceRemoveDefinition_Bad_WrongOwnerRejected(t *core.T) {
	fixture := newServiceFixture(t)

	result := fixture.service.RemoveDefinition("not-the-owner", fixture.definition.ID)

	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ErrorDefinitionConflict, ErrorCodeOf(result))
}

func TestServiceRemoveDefinition_Bad_BusyRecordRejected(t *core.T) {
	fixture := newServiceFixture(t)
	core.RequireTrue(t, fixture.service.Start(fixture.definition.ID).OK)

	result := fixture.service.RemoveDefinition(fixture.definition.Owner, fixture.definition.ID)

	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ErrorOperationInProgress, ErrorCodeOf(result))
}

func TestServiceRemoveDefinition_Bad_SaveFailureIsForwarded(t *core.T) {
	fixture := newServiceFixture(t)
	faulty := &conditionalFailCatalogue{Catalogue: fixture.service.options.Catalogue, failAfter: 0}
	fixture.service.options.Catalogue = faulty

	result := fixture.service.RemoveDefinition(fixture.definition.Owner, fixture.definition.ID)

	core.AssertFalse(t, result.OK)
	core.AssertContains(t, result.Error(), "injected save failure")
	got := fixture.service.Get(fixture.definition.ID)
	core.RequireTrue(t, got.OK, "a failed Save must not remove the in-memory record")
}

// ---- SetPolicy ---------------------------------------------------------------

func TestServiceSetPolicy_Bad_InvalidOverrideNeverReachesLock(t *core.T) {
	fixture := newServiceFixture(t)

	result := fixture.service.SetPolicy(PolicyOverride{
		ID:                fixture.definition.ID,
		RestartPolicy:     RestartPolicy("sometimes"),
		GracePeriodMillis: 8_000,
	})

	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ErrorDefinitionInvalid, ErrorCodeOf(result))
}

func TestServiceSetPolicy_Bad_UnknownID(t *core.T) {
	fixture := newServiceFixture(t)

	result := fixture.service.SetPolicy(PolicyOverride{
		ID:                "no-such-id",
		RestartPolicy:     RestartAlways,
		GracePeriodMillis: 8_000,
	})

	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ErrorDefinitionNotFound, ErrorCodeOf(result))
}

func TestServiceSetPolicy_Bad_BusyRecordRejected(t *core.T) {
	fixture := newServiceFixture(t)
	core.RequireTrue(t, fixture.service.Start(fixture.definition.ID).OK)

	result := fixture.service.SetPolicy(PolicyOverride{
		ID:                fixture.definition.ID,
		RestartPolicy:     RestartAlways,
		GracePeriodMillis: 8_000,
	})

	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ErrorOperationInProgress, ErrorCodeOf(result))
}

func TestServiceSetPolicy_Bad_SaveFailureIsForwarded(t *core.T) {
	fixture := newServiceFixture(t)
	faulty := &conditionalFailCatalogue{Catalogue: fixture.service.options.Catalogue, failAfter: 0}
	fixture.service.options.Catalogue = faulty

	result := fixture.service.SetPolicy(PolicyOverride{
		ID:                fixture.definition.ID,
		RestartPolicy:     RestartAlways,
		GracePeriodMillis: 8_000,
	})

	core.AssertFalse(t, result.OK)
	core.AssertContains(t, result.Error(), "injected save failure")
	got := fixture.service.Get(fixture.definition.ID)
	core.RequireTrue(t, got.OK, got.Error())
	core.AssertEqual(t, fixture.definition.RestartPolicy, got.Value.(Snapshot).Definition.RestartPolicy,
		"a failed Save must not publish the policy change")
}
