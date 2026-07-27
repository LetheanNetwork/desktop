// SPDX-License-Identifier: EUPL-1.2

package permissions_test

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/permissions"
)

func ExampleNewService() {
	service := permissions.NewService(permissions.Options{Core: core.New()})
	_ = service.Status()
}

func ExampleNewWailsService() {
	service := permissions.NewService(permissions.Options{Core: core.New()})
	binding := permissions.NewWailsService(service)
	_ = binding.Status()
}
