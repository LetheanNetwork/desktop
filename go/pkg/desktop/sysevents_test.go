// SPDX-Licence-Identifier: EUPL-1.2

package desktop

import (
	core "dappco.re/go"
	guienvironment "dappco.re/go/render/display/webkit/pkg/environment"
	guievents "dappco.re/go/render/display/webkit/pkg/events"
	guilifecycle "dappco.re/go/render/display/webkit/pkg/lifecycle"
	guiwindow "dappco.re/go/render/display/webkit/pkg/window"
)

// sysEventsFixture wires registerSystemEvents onto a bare Core and
// records every events.emit task the re-broadcaster fires, so each
// case below can assert on the translated lthn:* event name + payload
// without an active NSApp / Wails loop. host_intents_test.go and
// deeplink_test.go already exercise the file/drop/notification/url
// branches via this same registerSystemEvents seam; this file covers
// the remaining application + window-lifecycle + theme branches plus
// the router's default no-op.
func sysEventsFixture(t *core.T) (*core.Core, *[]guievents.TaskEmit) {
	t.Helper()
	c := core.New()
	emitted := []guievents.TaskEmit{}
	c.Action("events.emit", func(_ core.Context, options core.Options) core.Result {
		task, ok := options.Get("task").Value.(guievents.TaskEmit)
		core.RequireTrue(t, ok)
		emitted = append(emitted, task)
		return core.Ok(nil)
	})
	registerSystemEvents(c)
	return c, &emitted
}

func TestSysEvents_RegisterSystemEvents_Bad_NilCoreIsNoop(t *core.T) {
	registerSystemEvents(nil)
}

func TestSysEvents_RegisterSystemEvents_Good_ApplicationStartedEmitsAppStarted(t *core.T) {
	c, emitted := sysEventsFixture(t)

	result := c.ACTION(guilifecycle.ActionApplicationStarted{})

	core.RequireTrue(t, result.OK, result.Error())
	core.RequireTrue(t, len(*emitted) == 1)
	core.AssertEqual(t, "lthn:app:started", (*emitted)[0].Name)
}

func TestSysEvents_RegisterSystemEvents_Good_ThemeChangedDark(t *core.T) {
	c, emitted := sysEventsFixture(t)

	result := c.ACTION(guienvironment.ActionThemeChanged{IsDark: true})

	core.RequireTrue(t, result.OK, result.Error())
	core.RequireTrue(t, len(*emitted) == 1)
	core.AssertEqual(t, "lthn:theme", (*emitted)[0].Name)
	core.AssertEqual(t, "dark", (*emitted)[0].Data)
}

func TestSysEvents_RegisterSystemEvents_Good_ThemeChangedLight(t *core.T) {
	c, emitted := sysEventsFixture(t)

	result := c.ACTION(guienvironment.ActionThemeChanged{IsDark: false})

	core.RequireTrue(t, result.OK, result.Error())
	core.RequireTrue(t, len(*emitted) == 1)
	core.AssertEqual(t, "light", (*emitted)[0].Data)
}

func TestSysEvents_RegisterSystemEvents_Good_WindowLifecycleVerbs(t *core.T) {
	cases := []struct {
		name string
		verb string
		msg  core.Message
	}{
		{"focus", "focus", guiwindow.ActionWindowFocused{Name: "app"}},
		{"blur", "blur", guiwindow.ActionWindowBlurred{Name: "app"}},
		{"hide", "hide", guiwindow.ActionWindowHidden{Name: "app"}},
		{"show", "show", guiwindow.ActionWindowShown{Name: "app"}},
		{"minimise", "minimise", guiwindow.ActionWindowMinimised{Name: "app"}},
		{"unminimise", "unminimise", guiwindow.ActionWindowUnminimised{Name: "app"}},
		{"maximise", "maximise", guiwindow.ActionWindowMaximised{Name: "app"}},
		{"unmaximise", "unmaximise", guiwindow.ActionWindowUnmaximised{Name: "app"}},
		{"fullscreen", "fullscreen", guiwindow.ActionWindowFullscreened{Name: "app"}},
		{"unfullscreen", "unfullscreen", guiwindow.ActionWindowUnfullscreened{Name: "app"}},
		{"ready", "ready", guiwindow.ActionWindowRuntimeReady{Name: "app"}},
	}
	for _, tc := range cases {
		c, emitted := sysEventsFixture(t)

		result := c.ACTION(tc.msg)

		core.RequireTrue(t, result.OK, tc.name+": "+result.Error())
		core.RequireTrue(t, len(*emitted) == 1, tc.name)
		core.AssertEqual(t, "lthn:window:"+tc.verb, (*emitted)[0].Name, tc.name)
		payload, ok := (*emitted)[0].Data.(map[string]any)
		core.RequireTrue(t, ok, tc.name)
		core.AssertEqual(t, "app", payload["window"], tc.name)
	}
}

func TestSysEvents_RegisterSystemEvents_Good_WindowMovedCarriesCoordinates(t *core.T) {
	c, emitted := sysEventsFixture(t)

	result := c.ACTION(guiwindow.ActionWindowMoved{Name: "app", X: 12, Y: 34})

	core.RequireTrue(t, result.OK, result.Error())
	core.RequireTrue(t, len(*emitted) == 1)
	core.AssertEqual(t, "lthn:window:move", (*emitted)[0].Name)
	envelope, ok := (*emitted)[0].Data.(map[string]any)
	core.RequireTrue(t, ok)
	payload, ok := envelope["payload"].(map[string]any)
	core.RequireTrue(t, ok)
	core.AssertEqual(t, 12, payload["x"])
	core.AssertEqual(t, 34, payload["y"])
}

func TestSysEvents_RegisterSystemEvents_Good_WindowResizedCarriesDimensions(t *core.T) {
	c, emitted := sysEventsFixture(t)

	result := c.ACTION(guiwindow.ActionWindowResized{Name: "app", Width: 800, Height: 600})

	core.RequireTrue(t, result.OK, result.Error())
	core.RequireTrue(t, len(*emitted) == 1)
	core.AssertEqual(t, "lthn:window:resize", (*emitted)[0].Name)
	envelope, ok := (*emitted)[0].Data.(map[string]any)
	core.RequireTrue(t, ok)
	payload, ok := envelope["payload"].(map[string]any)
	core.RequireTrue(t, ok)
	core.AssertEqual(t, 800, payload["width"])
	core.AssertEqual(t, 600, payload["height"])
}

func TestSysEvents_RegisterSystemEvents_Ugly_UnknownMessageIsNoop(t *core.T) {
	c, emitted := sysEventsFixture(t)

	result := c.ACTION(struct{ unrelated bool }{true})

	core.RequireTrue(t, result.OK, result.Error())
	core.AssertEqual(t, 0, len(*emitted))
}
