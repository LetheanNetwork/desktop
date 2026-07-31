// SPDX-Licence-Identifier: EUPL-1.2

package desktop

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/appconfig"
)

func registerAppconfigEvents(c *core.Core) {
	if c == nil {
		return
	}
	appconfig.Subscribe(c, func(c *core.Core, event appconfig.Event) {
		_ = emitCoreEvent(c, "lthn:desktop-controls:changed", event)
	})
}
