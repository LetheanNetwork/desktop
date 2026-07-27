// SPDX-License-Identifier: EUPL-1.2

package main

import (
	goio "io"

	core "dappco.re/go"
	coreio "dappco.re/go/io"
)

func TestApp_ConfigMedium_GoodRegistersBoundaryBeforeConfig(t *core.T) {
	medium, err := coreio.NewSandboxed(".")
	core.RequireNoError(t, err)
	t.Cleanup(func() {
		if closer, ok := medium.(goio.Closer); ok {
			core.AssertNoError(t, closer.Close())
		}
	})
	source, err := medium.Read("app.go")
	core.RequireNoError(t, err)

	mediumRegistration := core.Index(
		source,
		`core.WithName("config-io", io.NewService`,
	)
	configRegistration := core.Index(
		source,
		`core.WithName("config", appconfig.NewConfigService`,
	)

	core.AssertTrue(t, mediumRegistration >= 0)
	core.AssertTrue(t, configRegistration > mediumRegistration)
	core.AssertNotContains(
		t,
		source,
		"config.NewConfigServiceWith(config.ServiceOptions",
	)
}
