// SPDX-Licence-Identifier: EUPL-1.2

package services

import (
	"strings"
	"time"

	core "dappco.re/go"
)

func validDefinition() Definition {
	return Definition{
		ID:                "local-api",
		DisplayName:       "Lethean API",
		Description:       "OpenAI-compatible local API.",
		Kind:              KindService,
		Command:           "lthn",
		Arguments:         []string{"serve", "--port", "8080"},
		RestartPolicy:     RestartNever,
		GracePeriodMillis: 5_000,
		Owner:             "lethean",
	}
}

func TestValidateDefinition_GoodAcceptsClosedKindsAndPolicies(t *core.T) {
	for _, kind := range []Kind{KindService, KindApp, KindProcess} {
		for _, policy := range []RestartPolicy{RestartNever, RestartOnFailure, RestartAlways} {
			definition := validDefinition()
			definition.Kind = kind
			definition.RestartPolicy = policy

			result := ValidateDefinition(definition, DefaultLimits())

			core.AssertTrue(t, result.OK, core.Sprintf("kind=%s policy=%s: %s", kind, policy, result.Error()))
		}
	}
}

func TestValidateDefinition_BadRejectsInvalidRequiredFields(t *core.T) {
	tests := []struct {
		name   string
		mutate func(*Definition)
	}{
		{"empty id", func(definition *Definition) { definition.ID = "" }},
		{"upper-case id", func(definition *Definition) { definition.ID = "Local-API" }},
		{"empty display name", func(definition *Definition) { definition.DisplayName = "" }},
		{"empty description", func(definition *Definition) { definition.Description = "" }},
		{"unknown kind", func(definition *Definition) { definition.Kind = Kind("daemon") }},
		{"empty command", func(definition *Definition) { definition.Command = "" }},
		{"unknown policy", func(definition *Definition) { definition.RestartPolicy = RestartPolicy("sometimes") }},
		{"empty owner", func(definition *Definition) { definition.Owner = "" }},
		{"zero grace period", func(definition *Definition) { definition.GracePeriodMillis = 0 }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *core.T) {
			definition := validDefinition()
			test.mutate(&definition)

			result := ValidateDefinition(definition, DefaultLimits())

			core.AssertFalse(t, result.OK)
			core.AssertEqual(t, ErrorDefinitionInvalid, ErrorCodeOf(result))
		})
	}
}

func TestValidateDefinition_BadEnforcesArgumentAndGraceLimits(t *core.T) {
	limits := DefaultLimits()

	tooMany := validDefinition()
	tooMany.Arguments = make([]string, limits.MaxArguments+1)
	core.AssertEqual(t, ErrorDefinitionInvalid, ErrorCodeOf(ValidateDefinition(tooMany, limits)))

	tooLong := validDefinition()
	tooLong.Arguments = []string{strings.Repeat("x", limits.MaxArgumentBytes+1)}
	core.AssertEqual(t, ErrorDefinitionInvalid, ErrorCodeOf(ValidateDefinition(tooLong, limits)))

	tooSlow := validDefinition()
	tooSlow.GracePeriodMillis = limits.MaxGracePeriodMillis + 1
	core.AssertEqual(t, ErrorDefinitionInvalid, ErrorCodeOf(ValidateDefinition(tooSlow, limits)))
}

func TestValidateDefinition_UglyRejectsUnsafeWorkingDirectoryReferences(t *core.T) {
	tests := []WorkingDirectory{
		{MountID: "projects", Path: "/Users/sarah/Code"},
		{MountID: "projects", Path: "../outside"},
		{MountID: "projects", Path: "safe/../../outside"},
		{MountID: "projects", Path: "contains\x00nul"},
		{MountID: "projects", Path: "contains\ncontrol"},
		{MountID: "", Path: "relative/without/mount"},
		{MountID: "../projects", Path: "safe"},
	}

	for _, workingDirectory := range tests {
		definition := validDefinition()
		definition.WorkingDirectory = workingDirectory

		result := ValidateDefinition(definition, DefaultLimits())

		core.AssertFalse(t, result.OK, core.Sprintf("working directory %#v", workingDirectory))
		core.AssertEqual(t, ErrorDefinitionInvalid, ErrorCodeOf(result))
	}
}

func TestDefinitionView_GoodOmitsExecutionFields(t *core.T) {
	view := definitionView(validDefinition())
	encoded := core.JSONMarshal(view)
	core.RequireTrue(t, encoded.OK)
	payload := string(encoded.Value.([]byte))

	core.AssertNotContains(t, payload, "command")
	core.AssertNotContains(t, payload, "arguments")
	core.AssertNotContains(t, payload, "workingDirectory")
	core.AssertContains(t, payload, "local-api")
}

func TestCloneDefinition_UglyDoesNotAliasArguments(t *core.T) {
	original := validDefinition()
	clone := cloneDefinition(original)

	clone.Arguments[0] = "changed"

	core.AssertEqual(t, "serve", original.Arguments[0])
}

func TestCloneSnapshot_UglyDoesNotAliasLastError(t *core.T) {
	original := Snapshot{
		Definition: definitionView(validDefinition()),
		State:      StateFailed,
		LastError: &FailureView{
			Code:    ErrorProcessStartFailed,
			Message: "Could not start this service.",
		},
	}
	clone := cloneSnapshot(original)

	clone.LastError.Message = "changed"

	core.AssertEqual(t, "Could not start this service.", original.LastError.Message)
}

func TestDefaultLimits_GoodAreInternallyConsistent(t *core.T) {
	limits := DefaultLimits()

	core.AssertGreater(t, limits.MaxDefinitions, 0)
	core.AssertGreater(t, limits.MaxArguments, 0)
	core.AssertGreater(t, limits.MaxArgumentBytes, 0)
	core.AssertGreater(t, limits.MaxRunning, 0)
	core.AssertGreater(t, limits.MaxOutputBytes, 0)
	core.AssertGreater(t, limits.MaxGracePeriodMillis, int64(0))
	core.AssertGreater(t, limits.RestartLimit, 0)
	core.AssertGreater(t, limits.RestartWindow, time.Duration(0))
	core.AssertGreater(t, limits.RestartBaseDelay, time.Duration(0))
	core.AssertGreater(t, limits.RestartMaxDelay, limits.RestartBaseDelay-time.Nanosecond)
}

func TestErrorCodeOf_BadPreservesTypedFailureAndHidesOtherErrors(t *core.T) {
	typed := core.Fail(&Failure{
		Code:      ErrorDefinitionNotFound,
		Operation: "services.Get",
		Message:   "Service not found.",
	})

	core.AssertEqual(t, ErrorDefinitionNotFound, ErrorCodeOf(typed))
	core.AssertEqual(t, ErrorCode(""), ErrorCodeOf(core.Fail(core.E("test", "plain", nil))))
	core.AssertEqual(t, ErrorCode(""), ErrorCodeOf(core.Ok(nil)))
}
