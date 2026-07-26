// SPDX-License-Identifier: EUPL-1.2

package desktop

import (
	core "dappco.re/go"
	officefiles "dappco.re/lthn/desktop/pkg/office/files"
)

func registerFilesEvents(c *core.Core) {
	if c == nil {
		return
	}
	officefiles.Subscribe(c, func(
		c *core.Core,
		event officefiles.FileEvent,
	) {
		emitCoreEvent(c, "lthn:files:changed", event)
	})
}
