// SPDX-Licence-Identifier: EUPL-1.2

package services_test

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/services"
)

func ExampleNewWailsService() {
	manager := services.NewService(services.Options{})
	binding := services.NewWailsService(manager)

	core.Println(binding.ServiceName())
	// Output: Lifecycle
}

func ExampleWailsService_NativeRegistry() {
	binding := services.NewWailsService(nil)

	core.Println(binding.NativeRegistry()[0].Name)
	// Output: serve
}
