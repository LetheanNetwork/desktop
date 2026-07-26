// SPDX-License-Identifier: EUPL-1.2

package files

import core "dappco.re/go"

// Subscribe registers a typed Files event listener on Core's ACTION bus.
func Subscribe(c *core.Core, fn func(*core.Core, FileEvent)) {
	if c == nil || fn == nil {
		return
	}
	c.RegisterAction(func(c *core.Core, message core.Message) core.Result {
		if event, ok := message.(FileEvent); ok {
			fn(c, event)
		}
		return core.Ok(nil)
	})
}

func (s *Service) fireEvent(
	operation string,
	operationID string,
	addresses ...FileAddress,
) {
	if s == nil || s.core == nil {
		return
	}
	mountIDs := make([]string, 0, len(addresses))
	paths := make([]string, 0, len(addresses))
	seenMount := make(map[string]bool)
	for _, address := range addresses {
		if !seenMount[address.MountID] {
			seenMount[address.MountID] = true
			mountIDs = append(mountIDs, address.MountID)
		}
		paths = append(paths, address.Path)
	}
	s.core.ACTION(FileEvent{
		Operation:   operation,
		OperationID: operationID,
		MountIDs:    mountIDs,
		Paths:       paths,
		At:          core.Now().UTC(),
	})
}
