// SPDX-Licence-Identifier: EUPL-1.2

package desktop

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/modelruntime"
)

func registerModelRuntimeEvents(c *core.Core) {
	if c == nil {
		return
	}
	modelruntime.Subscribe(c, func(c *core.Core, event modelruntime.Event) {
		emitCoreEvent(c, "lthn:model-runtime:changed", event)
	})
}
