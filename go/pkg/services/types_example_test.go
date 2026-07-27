// SPDX-Licence-Identifier: EUPL-1.2

package services_test

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/services"
)

func ExampleValidateDefinition() {
	result := services.ValidateDefinition(services.Definition{
		ID:                "api",
		DisplayName:       "Lethean API",
		Description:       "OpenAI-compatible local API.",
		Kind:              services.KindService,
		Command:           "lthn",
		Arguments:         []string{"serve"},
		RestartPolicy:     services.RestartNever,
		GracePeriodMillis: 5_000,
		Owner:             "lethean",
	}, services.DefaultLimits())

	core.Println(result.OK)
	// Output: true
}

func ExampleDefinitionView() {
	view := services.DefinitionView{
		ID:                "api",
		DisplayName:       "Lethean API",
		Description:       "OpenAI-compatible local API.",
		Kind:              services.KindService,
		RestartPolicy:     services.RestartNever,
		GracePeriodMillis: 5_000,
		Owner:             "lethean",
	}

	core.Println(view.ID, view.RestartPolicy)
	// Output: api never
}
