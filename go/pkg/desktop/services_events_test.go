// SPDX-Licence-Identifier: EUPL-1.2

package desktop

import (
	core "dappco.re/go"
	guievents "dappco.re/go/render/display/webkit/pkg/events"
	"dappco.re/lthn/desktop/pkg/services"
)

func TestRegisterServicesEvents_GoodForwardsBoundedInvalidation(t *core.T) {
	c := core.New()
	emitted := []guievents.TaskEmit{}
	c.Action("events.emit", func(_ core.Context, options core.Options) core.Result {
		task, ok := options.Get("task").Value.(guievents.TaskEmit)
		core.RequireTrue(t, ok)
		emitted = append(emitted, task)
		return core.Ok(nil)
	})
	registerServicesEvents(c)
	event := services.Event{
		ID:        "api",
		Operation: "start",
		Previous:  services.StateStopped,
		State:     services.StateRunning,
		Desired:   true,
		ProcessID: "proc-1",
		At:        "2026-07-27T12:00:00Z",
	}

	result := c.ACTION(event)

	core.RequireTrue(t, result.OK, result.Error())
	core.RequireTrue(t, len(emitted) == 1)
	core.AssertEqual(t, "lthn:services:changed", emitted[0].Name)
	core.AssertEqual(t, event, emitted[0].Data)
	serialised := core.JSONMarshal(emitted[0].Data)
	core.RequireTrue(t, serialised.OK)
	payload := string(serialised.Value.([]byte))
	core.AssertNotContains(t, payload, "command")
	core.AssertNotContains(t, payload, "arguments")
	core.AssertNotContains(t, payload, "environment")
	core.AssertNotContains(t, payload, "output")
}

func TestRegisterServicesEvents_BadNilCore(t *core.T) {
	registerServicesEvents(nil)
}
