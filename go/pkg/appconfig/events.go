// SPDX-Licence-Identifier: EUPL-1.2

package appconfig

import core "dappco.re/go"

// Event reports one committed desktop-control draft without exposing values.
type Event struct {
	Revision string   `json:"revision"`
	Keys     []string `json:"keys"`
	At       string   `json:"at"`
}

// Subscribe registers a typed desktop-control listener on Core's ACTION bus.
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

func (s *Service) fireEvent(keys []string) {
	if s == nil || s.core == nil {
		return
	}
	revision := s.revision.Add(1)
	_ = s.core.ACTION(Event{
		Revision: revisionString(revision),
		Keys:     append([]string(nil), keys...),
		At:       core.Now().UTC().Format(core.RFC3339Nano),
	})
}
