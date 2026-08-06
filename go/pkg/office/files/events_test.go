// SPDX-License-Identifier: EUPL-1.2

package files

import (
	core "dappco.re/go"
)

func TestSubscribe_UglyToleratesNilCoreOrHandler(t *core.T) {
	c := core.New()

	// Neither call should panic; both are no-ops.
	Subscribe(nil, func(*core.Core, FileEvent) {})
	Subscribe(c, nil)
}

func TestFireEvent_UglyToleratesNilServiceOrUnregisteredCore(t *core.T) {
	var nilService *Service
	nilService.fireEvent("noop", "op-1", FileAddress{MountID: "documents"})

	unregistered := &Service{}
	unregistered.fireEvent("noop", "op-1", FileAddress{MountID: "documents"})
}

func TestFireEvent_GoodDeduplicatesMountIDs(t *core.T) {
	c := core.New()
	core.RequireTrue(t, c.ServiceStartup(core.Background(), nil).OK)
	t.Cleanup(func() {
		core.AssertTrue(t, c.ServiceShutdown(core.Background()).OK)
	})
	service := &Service{core: c}
	var received FileEvent
	Subscribe(c, func(_ *core.Core, event FileEvent) {
		received = event
	})

	service.fireEvent(
		"move",
		"op-2",
		FileAddress{MountID: "documents", Path: "a.txt"},
		FileAddress{MountID: "documents", Path: "b.txt"},
	)

	core.AssertEqual(t, "move", received.Operation)
	core.AssertEqual(t, "op-2", received.OperationID)
	core.AssertEqual(t, []string{"documents"}, received.MountIDs)
	core.AssertEqual(t, []string{"a.txt", "b.txt"}, received.Paths)
}
