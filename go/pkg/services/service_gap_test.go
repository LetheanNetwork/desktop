// SPDX-Licence-Identifier: EUPL-1.2

// service_gap_test.go closes coverage gaps left by service_test.go /
// restart_test.go / registration_test.go: the small pure document
// helpers (applyPolicyOverride, upsertDefinition,
// removeDefinitionFromDocument, upsertPolicyOverride, failureFromResult,
// cloneFailure, operationInProgress) whose no-match / already-present /
// nil-cause branches are never exercised by the full-lifecycle fixture
// tests, plus (*Service).Register's own nil-guard.

package services

import (
	core "dappco.re/go"
)

// ---- operationInProgress --------------------------------------------------

func TestOperationInProgress_Good(t *core.T) {
	r := operationInProgress("services.Service.EnsureDefinition", "local-api")
	core.AssertFalse(t, r.OK)
	core.AssertEqual(t, ErrorOperationInProgress, ErrorCodeOf(r))
	core.AssertContains(t, r.Error(), "local-api")
}

// ---- applyPolicyOverride ---------------------------------------------------

func TestApplyPolicyOverride_Good_MatchingIDOverrides(t *core.T) {
	definition := validDefinition()
	overrides := []PolicyOverride{{ID: definition.ID, RestartPolicy: RestartAlways, GracePeriodMillis: 9_000}}

	applied := applyPolicyOverride(definition, overrides)

	core.AssertEqual(t, RestartAlways, applied.RestartPolicy)
	core.AssertEqual(t, int64(9_000), applied.GracePeriodMillis)
}

func TestApplyPolicyOverride_Bad_NoMatchingIDLeavesDefinitionUnchanged(t *core.T) {
	definition := validDefinition()
	overrides := []PolicyOverride{{ID: "some-other-service", RestartPolicy: RestartAlways, GracePeriodMillis: 9_000}}

	applied := applyPolicyOverride(definition, overrides)

	core.AssertEqual(t, definition.RestartPolicy, applied.RestartPolicy)
	core.AssertEqual(t, definition.GracePeriodMillis, applied.GracePeriodMillis)
}

// ---- upsertDefinition -------------------------------------------------------

func TestUpsertDefinition_Good_UpdatesExistingID(t *core.T) {
	document := &CatalogueDocument{Definitions: []Definition{validDefinition()}}
	changed := validDefinition()
	changed.DisplayName = "Renamed"

	upsertDefinition(document, changed)

	core.RequireTrue(t, len(document.Definitions) == 1)
	core.AssertEqual(t, "Renamed", document.Definitions[0].DisplayName)
}

func TestUpsertDefinition_Ugly_NewIDAppendsSorted(t *core.T) {
	document := &CatalogueDocument{Definitions: []Definition{validDefinition()}}
	second := validDefinition()
	second.ID = "zzz-later"

	upsertDefinition(document, second)

	core.RequireTrue(t, len(document.Definitions) == 2)
	core.AssertEqual(t, "local-api", document.Definitions[0].ID)
	core.AssertEqual(t, "zzz-later", document.Definitions[1].ID)
}

// ---- removeDefinitionFromDocument -------------------------------------------

func TestRemoveDefinitionFromDocument_Good_KeepsNonMatchingEntries(t *core.T) {
	other := validDefinition()
	other.ID = "keep-me"
	target := validDefinition()
	target.ID = "remove-me"
	document := &CatalogueDocument{
		Definitions: []Definition{other, target},
		PolicyOverrides: []PolicyOverride{
			{ID: "keep-me", RestartPolicy: RestartNever, GracePeriodMillis: 1_000},
			{ID: "remove-me", RestartPolicy: RestartNever, GracePeriodMillis: 1_000},
		},
	}

	removeDefinitionFromDocument(document, "remove-me")

	core.RequireTrue(t, len(document.Definitions) == 1)
	core.AssertEqual(t, "keep-me", document.Definitions[0].ID)
	core.RequireTrue(t, len(document.PolicyOverrides) == 1)
	core.AssertEqual(t, "keep-me", document.PolicyOverrides[0].ID)
}

// ---- upsertPolicyOverride ----------------------------------------------------

func TestUpsertPolicyOverride_Good_UpdatesExistingID(t *core.T) {
	document := &CatalogueDocument{
		PolicyOverrides: []PolicyOverride{{ID: "local-api", RestartPolicy: RestartNever, GracePeriodMillis: 1_000}},
	}

	upsertPolicyOverride(document, PolicyOverride{ID: "local-api", RestartPolicy: RestartAlways, GracePeriodMillis: 9_000})

	core.RequireTrue(t, len(document.PolicyOverrides) == 1)
	core.AssertEqual(t, RestartAlways, document.PolicyOverrides[0].RestartPolicy)
}

func TestUpsertPolicyOverride_Ugly_NewIDAppendsSorted(t *core.T) {
	document := &CatalogueDocument{
		PolicyOverrides: []PolicyOverride{{ID: "local-api", RestartPolicy: RestartNever, GracePeriodMillis: 1_000}},
	}

	upsertPolicyOverride(document, PolicyOverride{ID: "zzz-later", RestartPolicy: RestartAlways, GracePeriodMillis: 9_000})

	core.RequireTrue(t, len(document.PolicyOverrides) == 2)
	core.AssertEqual(t, "local-api", document.PolicyOverrides[0].ID)
	core.AssertEqual(t, "zzz-later", document.PolicyOverrides[1].ID)
}

// ---- failureFromResult / cloneFailure -----------------------------------------

func TestFailureFromResult_Bad_NonFailureErrorUsesFallback(t *core.T) {
	failure := failureFromResult(
		core.Fail(core.E("test.plain", "not a *Failure", nil)),
		ErrorServicesUnavailable,
		"services.Service",
		"fallback message",
	)
	core.RequireTrue(t, failure != nil)
	core.AssertEqual(t, ErrorServicesUnavailable, failure.Code)
	core.AssertEqual(t, "fallback message", failure.Message)
}

func TestCloneFailure_Bad_NilReturnsNil(t *core.T) {
	core.AssertTrue(t, cloneFailure(nil) == nil)
}

// ---- (*Service).Register nil-guard -------------------------------------------

func TestServiceRegister_Bad_NilService(t *core.T) {
	var service *Service
	r := service.Register(core.New())
	core.AssertFalse(t, r.OK)
	core.AssertEqual(t, ErrorServicesUnavailable, ErrorCodeOf(r))
}

func TestServiceRegister_Bad_NilCore(t *core.T) {
	service := NewService(Options{})
	r := service.Register(nil)
	core.AssertFalse(t, r.OK)
	core.AssertEqual(t, ErrorServicesUnavailable, ErrorCodeOf(r))
}
