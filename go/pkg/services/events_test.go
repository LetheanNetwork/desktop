// SPDX-Licence-Identifier: EUPL-1.2

package services

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/audit"
)

func TestServiceEvent_GoodCarriesOnlyBoundedInvalidationMetadata(t *core.T) {
	fixture := newServiceFixture(t)
	events := []Event{}
	Subscribe(fixture.core, func(_ *core.Core, event Event) {
		events = append(events, event)
	})

	result := fixture.service.Start("local-api")

	core.RequireTrue(t, result.OK, result.Error())
	core.RequireTrue(t, len(events) >= 2)
	last := events[len(events)-1]
	core.AssertEqual(t, "local-api", last.ID)
	core.AssertEqual(t, "start", last.Operation)
	core.AssertEqual(t, StateRunning, last.State)
	core.AssertTrue(t, last.Desired)
	serialised := core.JSONMarshal(events)
	core.RequireTrue(t, serialised.OK)
	payload := string(serialised.Value.([]byte))
	for _, forbidden := range []string{
		`"command"`, `"arguments"`, `"workingDirectory"`, `"environment"`,
		`"output"`, "lthn", "serve",
	} {
		core.AssertNotContains(t, payload, forbidden)
	}
}

func TestServiceAudit_GoodUsesTypedLifecycleAndBoundedMeta(t *core.T) {
	fixture := newServiceFixture(t)

	core.RequireTrue(t, fixture.service.Start("local-api").OK)
	core.RequireTrue(t, fixture.service.Stop("local-api").OK)

	events := fixture.audit.snapshot()
	names := make([]string, 0, len(events))
	for _, event := range events {
		names = append(names, event.Event)
		for key := range event.Meta {
			core.AssertContains(t, []string{
				"service_id",
				"process_id",
				"restart_policy",
				audit.MetaKeyErrorCode,
				audit.MetaKeyErrorScope,
			}, key)
		}
	}
	core.AssertContains(t, names, audit.EventServiceStartRequested)
	core.AssertContains(t, names, audit.EventServiceStartSucceeded)
	core.AssertContains(t, names, audit.EventServiceStopRequested)
	core.AssertContains(t, names, audit.EventServiceStopSucceeded)
	serialised := core.JSONMarshal(events)
	core.RequireTrue(t, serialised.OK)
	payload := string(serialised.Value.([]byte))
	core.AssertNotContains(t, payload, `"command"`)
	core.AssertNotContains(t, payload, `"arguments"`)
	core.AssertNotContains(t, payload, `"environment"`)
	core.AssertNotContains(t, payload, `"output"`)
	core.AssertNotContains(t, payload, `"lthn"`)
	core.AssertNotContains(t, payload, `"serve"`)
}

func TestServiceAudit_BadFailureUsesStableCodeNotRawProse(t *core.T) {
	fixture := newServiceFixture(t)
	fixture.runtime.startResult = core.Fail(core.E(
		"fake.Start",
		"SECRET raw failure prose",
		nil,
	))

	result := fixture.service.Start("local-api")

	core.AssertFalse(t, result.OK)
	events := fixture.audit.snapshot()
	core.RequireTrue(t, len(events) == 2)
	failed := events[1]
	core.AssertEqual(t, audit.EventServiceStartFailed, failed.Event)
	core.AssertEqual(t, string(ErrorProcessStartFailed), failed.Meta[audit.MetaKeyErrorCode])
	serialised := core.JSONMarshal(events)
	core.RequireTrue(t, serialised.OK)
	core.AssertNotContains(t, string(serialised.Value.([]byte)), "SECRET")
}

func TestServiceDefinitionEvent_GoodPublishesOnlySafeMetadataAfterPersistence(t *core.T) {
	fixture := newServiceFixture(t)
	events := []Event{}
	Subscribe(fixture.core, func(_ *core.Core, event Event) {
		events = append(events, event)
	})
	definition := validDefinition()
	definition.ID = "indexer"
	definition.DisplayName = "Workspace indexer"
	definition.Command = "SECRET-COMMAND"
	definition.Arguments = []string{"SECRET-ARG"}

	result := fixture.service.EnsureDefinition(definition)

	core.RequireTrue(t, result.OK, result.Error())
	core.RequireTrue(t, len(events) == 1)
	core.AssertEqual(t, "definition", events[0].Operation)
	core.AssertEqual(t, "indexer", events[0].ID)
	names := []string{}
	for _, event := range fixture.audit.snapshot() {
		names = append(names, event.Event)
	}
	core.AssertContains(t, names, audit.EventServiceDefinitionChanged)
	serialised := core.JSONMarshal(events)
	core.RequireTrue(t, serialised.OK)
	payload := string(serialised.Value.([]byte))
	core.AssertNotContains(t, payload, "SECRET-COMMAND")
	core.AssertNotContains(t, payload, "SECRET-ARG")
}
