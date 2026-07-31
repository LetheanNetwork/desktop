// SPDX-Licence-Identifier: EUPL-1.2

package desktop

import (
	core "dappco.re/go"
	guievents "dappco.re/go/render/display/webkit/pkg/events"
	"dappco.re/lthn/desktop/pkg/appconfig"
)

func TestRegisterAppconfigEvents_GoodForwardsBoundedChange(t *core.T) {
	c := core.New()
	emitted := []guievents.TaskEmit{}
	c.Action("events.emit", func(_ core.Context, options core.Options) core.Result {
		task, ok := options.Get("task").Value.(guievents.TaskEmit)
		core.RequireTrue(t, ok)
		emitted = append(emitted, task)
		return core.Ok(nil)
	})
	registerAppconfigEvents(c)
	event := appconfig.Event{
		Revision: "7",
		Keys:     []string{"desktop.theme.interface"},
		At:       "2026-07-31T12:00:00Z",
	}

	result := c.ACTION(event)

	core.RequireTrue(t, result.OK, result.Error())
	core.RequireTrue(t, len(emitted) == 1)
	core.AssertEqual(t, "lthn:desktop-controls:changed", emitted[0].Name)
	core.AssertEqual(t, event, emitted[0].Data)
	serialised := core.JSONMarshal(emitted[0].Data)
	core.RequireTrue(t, serialised.OK, serialised.Error())
	payload := string(serialised.Bytes())
	for _, forbidden := range []string{
		"path", "token", "credential", "command", "environment", "value",
	} {
		core.AssertNotContains(t, payload, forbidden)
	}
}

func TestRegisterAppconfigEvents_BadNilCore(t *core.T) {
	registerAppconfigEvents(nil)
}
