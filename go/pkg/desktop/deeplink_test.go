// SPDX-Licence-Identifier: EUPL-1.2

package desktop

import (
	core "dappco.re/go"
	gui "dappco.re/go/render/display/webkit"
	guievents "dappco.re/go/render/display/webkit/pkg/events"
	guilifecycle "dappco.re/go/render/display/webkit/pkg/lifecycle"
)

func deepLinkEventFixture(t *core.T) (*core.Core, *[]guievents.TaskEmit, *[]string) {
	t.Helper()

	c := core.New()
	guiResult := gui.NewService(gui.GuiConfig{WindowRegistry: windowRegistry()})(c)
	core.AssertTrue(t, guiResult.OK)
	core.AssertTrue(t, c.RegisterService("gui", guiResult.Value).OK)

	emitted := []guievents.TaskEmit{}
	c.Action("events.emit", func(_ core.Context, opts core.Options) core.Result {
		task, ok := opts.Get("task").Value.(guievents.TaskEmit)
		core.AssertTrue(t, ok)
		emitted = append(emitted, task)
		return core.Ok(nil)
	})

	windowActions := []string{}
	for _, name := range []string{
		"dock.show_icon",
		"window.restore",
		"window.set_visibility",
		"window.focus",
	} {
		actionName := name
		c.Action(actionName, func(_ core.Context, _ core.Options) core.Result {
			windowActions = append(windowActions, actionName)
			return core.Ok(nil)
		})
	}

	registerSystemEvents(c)
	return c, &emitted, &windowActions
}

func TestDeepLink_Handle_Good_App(t *core.T) {
	c, emitted, windowActions := deepLinkEventFixture(t)

	core.AssertTrue(t, c.ACTION(guilifecycle.ActionLaunchedWithUrl{URL: "lthn://chat"}).OK)

	core.AssertEqual(t, []string{
		"dock.show_icon",
		"window.restore",
		"window.set_visibility",
		"window.focus",
	}, *windowActions)
	core.AssertEqual(t, 2, len(*emitted))
	if len(*emitted) < 2 {
		return
	}
	core.AssertEqual(t, "lthn:app:opened-url", (*emitted)[0].Name)
	core.AssertEqual(t, "lthn://chat", (*emitted)[0].Data)
	core.AssertEqual(t, "navigate", (*emitted)[1].Name)
	core.AssertEqual(t, map[string]string{
		"action":   "chat",
		"resource": "",
		"id":       "",
	}, (*emitted)[1].Data)
}

func TestDeepLink_Parse_Good_ActionResourceQueryID(t *core.T) {
	result := parseDeepLink("lthn://open/document?id=record-42")

	core.AssertTrue(t, result.OK)
	core.AssertEqual(t, map[string]string{
		"action":   "open",
		"resource": "document",
		"id":       "record-42",
	}, result.Value)
}

func TestDeepLink_Parse_Good_PathID(t *core.T) {
	result := parseDeepLink("lthn://open/document/record-42")

	core.AssertTrue(t, result.OK)
	core.AssertEqual(t, map[string]string{
		"action":   "open",
		"resource": "document",
		"id":       "record-42",
	}, result.Value)
}

func TestDeepLink_Parse_Bad_InvalidURL(t *core.T) {
	result := parseDeepLink("lthn://chat/%zz")

	core.AssertFalse(t, result.OK)
	core.AssertError(t, result.Value.(error))
}

func TestDeepLink_Parse_Ugly_WrongScheme(t *core.T) {
	result := parseDeepLink("https://chat")

	core.AssertFalse(t, result.OK)
	core.AssertError(t, result.Value.(error))
}

func TestDeepLink_Parse_Bad_EmptyActionIsRejected(t *core.T) {
	result := parseDeepLink("lthn://")

	core.AssertFalse(t, result.OK)
	core.AssertError(t, result.Value.(error))
}

func TestDeepLink_Handle_Ugly_MainWindowUnavailablePropagatesParsedTarget(t *core.T) {
	// No "gui" service registered — gui.OpenWindow(c, mainWindowName) has
	// nothing to look up and returns false, so handleDeepLink must
	// surface a clean Fail rather than emit navigate against a window
	// that was never restored.
	result := handleDeepLink(core.New(), "lthn://chat")

	core.AssertFalse(t, result.OK)
	core.AssertContains(t, result.Error(), "main window is unavailable")
}
