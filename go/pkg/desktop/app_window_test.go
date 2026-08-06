// SPDX-Licence-Identifier: EUPL-1.2

// Argument validation for the tear-off binding. A native window must be
// openable at a catalogue application's solo route and at nothing else, so
// every rejection below is the boundary rather than a formatting preference.

package desktop

import core "dappco.re/go"

func TestAppWindow_TearOffRoute_Good_ApplicationOnly(t *core.T) {
	route, ok := tearOffRoute("chat", "")

	core.AssertTrue(t, ok)
	core.AssertEqual(t, "/#/w/chat", route)
}

func TestAppWindow_TearOffRoute_Good_ApplicationAndPane(t *core.T) {
	route, ok := tearOffRoute("project-manager", "backlog")

	core.AssertTrue(t, ok)
	core.AssertEqual(t, "/#/w/project-manager/backlog", route)
}

func TestAppWindow_TearOffRoute_Bad_UnknownApplication(t *core.T) {
	_, ok := tearOffRoute("terminal", "")

	core.AssertFalse(t, ok, "a retired application id opens no native window")
}

func TestAppWindow_TearOffRoute_Ugly_PaneEscapesItsSegment(t *core.T) {
	_, ok := tearOffRoute("chat", "../../etc/passwd")

	core.AssertFalse(t, ok)
}

func TestAppWindow_TearOffRoute_Ugly_ApplicationCarriesAURL(t *core.T) {
	_, ok := tearOffRoute("https://example.invalid", "")

	core.AssertFalse(t, ok)
}

func TestAppWindow_TearOffDimensions_Good_CarriesTheShellSize(t *core.T) {
	width, height, ok := tearOffDimensions(920, 640)

	core.AssertTrue(t, ok)
	core.AssertEqual(t, 920, width)
	core.AssertEqual(t, 640, height)
}

func TestAppWindow_TearOffDimensions_Good_ZeroCarriesNothing(t *core.T) {
	width, height, ok := tearOffDimensions(0, 0)

	core.AssertTrue(t, ok)
	core.AssertEqual(t, 0, width)
	core.AssertEqual(t, 0, height)
}

func TestAppWindow_TearOffDimensions_Bad_BelowTheUsableMinimum(t *core.T) {
	_, _, ok := tearOffDimensions(120, 640)

	core.AssertFalse(t, ok)
}

func TestAppWindow_TearOffDimensions_Ugly_LargerThanAnyDisplay(t *core.T) {
	_, _, ok := tearOffDimensions(920, 99_999)

	core.AssertFalse(t, ok)
}

func TestAppWindow_OpenApp_Bad_UnknownApplication(t *core.T) {
	result := NewAppWindowService(core.New()).OpenApp(AppWindowRequest{App: "../chat"})

	core.AssertFalse(t, result.OK)
	core.AssertError(t, result.Value.(error))
}

func TestAppWindow_OpenApp_Bad_NoCore(t *core.T) {
	result := NewAppWindowService(nil).OpenApp(AppWindowRequest{App: "chat"})

	core.AssertFalse(t, result.OK)
	core.AssertError(t, result.Value.(error))
}

func TestAppWindow_ServiceName_Good_RendererFacingName(t *core.T) {
	core.AssertEqual(t, "AppWindow", NewAppWindowService(core.New()).ServiceName())
}
