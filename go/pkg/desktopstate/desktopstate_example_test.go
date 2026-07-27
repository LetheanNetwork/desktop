// SPDX-License-Identifier: EUPL-1.2

package desktopstate_test

import (
	"fmt"

	coreio "dappco.re/go/io"
	"dappco.re/lthn/desktop/pkg/desktopstate"
)

func ExampleService_SaveShellSession() {
	service := desktopstate.NewService(desktopstate.Options{
		Medium: coreio.NewMemoryMedium(),
	})
	result := service.SaveShellSession(desktopstate.SaveShellSessionInput{
		ExpectedRevision: 0,
		Session: desktopstate.ShellSession{
			View:   desktopstate.ViewDesktop,
			Device: desktopstate.DeviceFull,
		},
	})

	fmt.Println(result.OK)
	// Output: true
}
