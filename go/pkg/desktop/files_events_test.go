// SPDX-License-Identifier: EUPL-1.2

package desktop

import (
	core "dappco.re/go"
	guievents "dappco.re/go/render/display/webkit/pkg/events"
	officefiles "dappco.re/lthn/desktop/pkg/office/files"
)

func TestRegisterFilesEvents_Good(t *core.T) {
	c := core.New()
	emitted := []guievents.TaskEmit{}
	c.Action("events.emit", func(_ core.Context, options core.Options) core.Result {
		task, ok := options.Get("task").Value.(guievents.TaskEmit)
		core.RequireTrue(t, ok)
		emitted = append(emitted, task)
		return core.Ok(nil)
	})
	registerFilesEvents(c)
	event := officefiles.FileEvent{
		Operation:   "rename",
		OperationID: "operation-1",
		MountIDs:    []string{"documents"},
		Paths:       []string{"notes/renamed.md"},
		At:          core.Now().UTC(),
	}

	result := c.ACTION(event)

	core.AssertTrue(t, result.OK)
	core.RequireTrue(t, len(emitted) == 1)
	core.AssertEqual(t, "lthn:files:changed", emitted[0].Name)
	core.AssertEqual(t, event, emitted[0].Data)
	serialised := core.JSONMarshal(emitted[0].Data)
	core.RequireTrue(t, serialised.OK)
	payload, ok := serialised.Value.([]byte)
	core.RequireTrue(t, ok)
	core.AssertNotContains(t, string(payload), "/Users/")
	core.AssertContains(t, string(payload), `"notes/renamed.md"`)
}

func TestRegisterFilesEvents_BadNilCore(t *core.T) {
	registerFilesEvents(nil)
}
