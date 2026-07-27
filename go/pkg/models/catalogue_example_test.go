// SPDX-Licence-Identifier: EUPL-1.2

package models_test

import (
	core "dappco.re/go"
	coreio "dappco.re/go/io"
	"dappco.re/lthn/desktop/pkg/models"
)

func ExampleCatalogue_List() {
	medium := coreio.NewMemoryMedium()
	_ = medium.EnsureDir("lem/models/gemma-4-e2b")
	_ = medium.Write("lem/models/gemma-4-e2b/.sha256", "digest  weights\n")
	catalogue := models.NewCatalogue(
		medium,
		"lem/models",
		"/trusted/Lethean/lem/models",
	)

	result := catalogue.List()
	entries := result.Value.([]models.CatalogueEntry)
	core.Println(entries[0].DisplayName, entries[0].Loadable)
	// Output: gemma-4-e2b true
}
