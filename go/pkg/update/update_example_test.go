// SPDX-Licence-Identifier: EUPL-1.2

package update_test

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/update"
)

func ExampleService_OnStartup() {
	var hook func(*update.Service, core.Context) core.Result = (*update.Service).OnStartup
	core.Println(hook != nil)
	// Output: true
}
