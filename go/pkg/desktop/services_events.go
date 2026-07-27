// SPDX-License-Identifier: EUPL-1.2

package desktop

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/services"
)

func registerServicesEvents(c *core.Core) {
	if c == nil {
		return
	}
	services.Subscribe(c, func(c *core.Core, event services.Event) {
		emitCoreEvent(c, "lthn:services:changed", event)
	})
}
