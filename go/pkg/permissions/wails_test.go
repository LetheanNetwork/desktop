// SPDX-License-Identifier: EUPL-1.2

package permissions

import (
	core "dappco.re/go"
	guievents "dappco.re/go/render/display/webkit/pkg/events"
)

func TestWailsService_Status_GoodEmitsTypedHostSnapshots(t *core.T) {
	host := &permissionHostProbe{states: map[ID]HostState{
		Notifications: HostGranted,
	}}
	c, service := permissionServiceFixture(t, host)
	emitted := []guievents.TaskEmit{}
	c.Action("events.emit", func(_ core.Context, options core.Options) core.Result {
		task, ok := options.Get("task").Value.(guievents.TaskEmit)
		core.RequireTrue(t, ok)
		emitted = append(emitted, task)
		return core.Ok(nil)
	})

	result := NewWailsService(service).Status()

	core.RequireTrue(t, result.OK, result.Error())
	core.AssertEqual(t, len(permissionIDs), len(emitted))
	core.AssertEqual(t, "lthn:host:intent", emitted[0].Name)
	intent, ok := emitted[0].Data.(HostIntent)
	core.RequireTrue(t, ok)
	core.AssertEqual(t, "permission", intent.Kind)
	core.AssertNotNil(t, intent.Permission)
}

func TestWailsService_Request_BadNilServiceFailsClosed(t *core.T) {
	result := NewWailsService(nil).Request(string(Notifications))

	core.AssertFalse(t, result.OK)
}

func TestWailsService_Request_UglyUnknownPermissionEmitsNothing(t *core.T) {
	host := &permissionHostProbe{states: map[ID]HostState{}}
	c, service := permissionServiceFixture(t, host)
	emitted := 0
	c.Action("events.emit", func(_ core.Context, _ core.Options) core.Result {
		emitted++
		return core.Ok(nil)
	})

	result := NewWailsService(service).Request("root-filesystem")

	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, 0, emitted)
}
