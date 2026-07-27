// SPDX-Licence-Identifier: EUPL-1.2

package modelruntime

import core "dappco.re/go"

// Subscribe registers a typed model-runtime listener on Core's ACTION bus.
func Subscribe(c *core.Core, fn func(*core.Core, Event)) {
	if c == nil || fn == nil {
		return
	}
	c.RegisterAction(func(c *core.Core, message core.Message) core.Result {
		if event, ok := message.(Event); ok {
			fn(c, event)
		}
		return core.Ok(nil)
	})
}

func (service *Service) fireEvent(reason string, state State) {
	if service == nil || service.core == nil {
		return
	}
	_ = service.core.ACTION(Event{
		Reason: reason,
		State:  state,
		At:     timestamp(service.options.Now()),
	})
}
