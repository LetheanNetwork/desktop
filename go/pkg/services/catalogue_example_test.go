// SPDX-Licence-Identifier: EUPL-1.2

package services_test

import (
	core "dappco.re/go"
	coreio "dappco.re/go/io"
	"dappco.re/lthn/desktop/pkg/services"
)

func ExampleNewMediumCatalogue() {
	medium := coreio.NewMemoryMedium()
	catalogue := services.NewMediumCatalogue(
		medium,
		"desktop/services/catalogue.json",
		services.DefaultLimits(),
	)
	saved := catalogue.Save(services.CatalogueDocument{
		Version: services.CatalogueVersion,
		Definitions: []services.Definition{{
			ID:                "api",
			DisplayName:       "Lethean API",
			Description:       "OpenAI-compatible local API.",
			Kind:              services.KindService,
			Command:           "lthn",
			Arguments:         []string{"serve"},
			RestartPolicy:     services.RestartNever,
			GracePeriodMillis: 5_000,
			Owner:             "lethean",
		}},
		PolicyOverrides: []services.PolicyOverride{},
		UpdatedAt:       "2026-07-27T12:00:00Z",
	})
	loaded := catalogue.Load()

	core.Println(saved.OK, loaded.OK)
	// Output: true true
}
