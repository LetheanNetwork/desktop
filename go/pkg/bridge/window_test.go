// SPDX-Licence-Identifier: EUPL-1.2

// window.go tests. Rather than hand-faking every "window.*" action
// name, these register the REAL core/gui window.Service against its
// own exported MockPlatform (window.NewMockPlatform) — genuine
// service behaviour (position, size, state flags) without a live
// Wails runtime.

package bridge

import (
	core "dappco.re/go"
	guiwindow "dappco.re/go/render/display/webkit/pkg/window"
)

// windowHarness returns a bridge *Service wired to a Core carrying a
// real, mock-backed window.Service, plus a helper to open a named
// window so QueryWindowList/QueryWindowByName resolve it.
func windowHarness(t *core.T) (*Service, *core.Core) {
	t.Helper()
	c := core.New()
	r := guiwindow.Register(guiwindow.NewMockPlatform())(c)
	core.AssertTrue(t, r.OK)
	winSvc := r.Value.(*guiwindow.Service)
	core.AssertTrue(t, winSvc.OnStartup(core.Background()).OK)
	s := &Service{ServiceRuntime: core.NewServiceRuntime[Options](c, Options{}), port: 9999}
	return s, c
}

func openWindow(t *core.T, c *core.Core, name string) {
	t.Helper()
	r := c.Action("window.open").Run(core.Background(), core.NewOptions(
		core.Option{Key: "task", Value: guiwindow.TaskOpenWindow{Window: &guiwindow.Window{Name: name, URL: "/"}}},
	))
	core.AssertTrue(t, r.OK, "test fixture: opening window %q must succeed", name)
}

// ─── coreGUIWindowList / coreGUIWindowInfo (indirect) ───────────────

func TestWindow_ToolWindowList_Good(t *core.T) {
	s, c := windowHarness(t)
	openWindow(t, c, "tray")
	openWindow(t, c, "chat")
	resp := s.toolWindowList()
	core.AssertEqual(t, true, resp["ok"])
	core.AssertEqual(t, 2, resp["count"])
}

func TestWindow_ToolWindowList_Bad_NoWindowService(t *core.T) {
	c := core.New()
	s := &Service{ServiceRuntime: core.NewServiceRuntime[Options](c, Options{}), port: 9999}
	resp := s.toolWindowList()
	core.AssertEqual(t, false, resp["ok"])
	core.AssertNotEmpty(t, resp["error"])
}

func TestWindow_ToolWindowGet_Good(t *core.T) {
	s, c := windowHarness(t)
	openWindow(t, c, "tray")
	resp := s.toolWindowGet(map[string]any{"name": "tray"})
	core.AssertEqual(t, true, resp["ok"])
	value, ok := resp["value"].(map[string]any)
	core.AssertTrue(t, ok)
	core.AssertEqual(t, "tray", value["name"])
}

func TestWindow_ToolWindowGet_Bad_MissingNameParam(t *core.T) {
	s, _ := windowHarness(t)
	resp := s.toolWindowGet(map[string]any{})
	core.AssertEqual(t, false, resp["ok"])
	core.AssertEqual(t, nameParamRequired, resp["error"])
}

func TestWindow_ToolWindowGet_Ugly_UnknownWindow(t *core.T) {
	s, _ := windowHarness(t)
	resp := s.toolWindowGet(map[string]any{"name": "ghost"})
	core.AssertEqual(t, false, resp["ok"])
	core.AssertContains(t, resp["error"], "window not found")
}

// ─── position / size / bounds ───────────────────────────────────────

func TestWindow_ToolWindowPosition_Good(t *core.T) {
	s, c := windowHarness(t)
	openWindow(t, c, "tray")
	resp := s.toolWindowPosition(map[string]any{"name": "tray", "x": 100, "y": 200})
	core.AssertEqual(t, true, resp["ok"])

	info := s.toolWindowGet(map[string]any{"name": "tray"})["value"].(map[string]any)
	core.AssertEqual(t, 100, info["x"])
	core.AssertEqual(t, 200, info["y"])
}

func TestWindow_ToolWindowPosition_Bad_UnknownWindow(t *core.T) {
	s, _ := windowHarness(t)
	resp := s.toolWindowPosition(map[string]any{"name": "ghost", "x": 1, "y": 1})
	core.AssertEqual(t, false, resp["ok"])
}

func TestWindow_ToolWindowSize_Good(t *core.T) {
	s, c := windowHarness(t)
	openWindow(t, c, "tray")
	resp := s.toolWindowSize(map[string]any{"name": "tray", "width": 640, "height": 480})
	core.AssertEqual(t, true, resp["ok"])
}

func TestWindow_ToolWindowSize_Bad_NonPositiveDims(t *core.T) {
	s, c := windowHarness(t)
	openWindow(t, c, "tray")
	resp := s.toolWindowSize(map[string]any{"name": "tray", "width": 0, "height": 480})
	core.AssertEqual(t, false, resp["ok"])
	core.AssertEqual(t, "width + height must be > 0", resp["error"])
}

func TestWindow_ToolWindowBounds_Good(t *core.T) {
	s, c := windowHarness(t)
	openWindow(t, c, "tray")
	resp := s.toolWindowBounds(map[string]any{"name": "tray", "x": 1, "y": 2, "width": 3, "height": 4})
	core.AssertEqual(t, true, resp["ok"])
}

func TestWindow_ToolWindowBounds_Bad_NonPositiveDims(t *core.T) {
	s, _ := windowHarness(t)
	resp := s.toolWindowBounds(map[string]any{"name": "tray", "x": 1, "y": 2, "width": -1, "height": 4})
	core.AssertEqual(t, false, resp["ok"])
}

// ─── state toggles ──────────────────────────────────────────────────

func TestWindow_ToolWindowMaximise_Good(t *core.T) {
	s, c := windowHarness(t)
	openWindow(t, c, "tray")
	resp := s.toolWindowMaximise(map[string]any{"name": "tray"})
	core.AssertEqual(t, true, resp["ok"])
	core.AssertEqual(t, "maximise", resp["action"])
}

func TestWindow_ToolWindowMinimise_Good(t *core.T) {
	s, c := windowHarness(t)
	openWindow(t, c, "tray")
	resp := s.toolWindowMinimise(map[string]any{"name": "tray"})
	core.AssertEqual(t, true, resp["ok"])
}

func TestWindow_ToolWindowRestore_Good(t *core.T) {
	s, c := windowHarness(t)
	openWindow(t, c, "tray")
	resp := s.toolWindowRestore(map[string]any{"name": "tray"})
	core.AssertEqual(t, true, resp["ok"])
}

func TestWindow_ToolWindowFocus_Good(t *core.T) {
	s, c := windowHarness(t)
	openWindow(t, c, "tray")
	resp := s.toolWindowFocus(map[string]any{"name": "tray"})
	core.AssertEqual(t, true, resp["ok"])
}

func TestWindow_ToolWindowFocus_Bad_Unknown(t *core.T) {
	s, _ := windowHarness(t)
	resp := s.toolWindowFocus(map[string]any{"name": "ghost"})
	core.AssertEqual(t, false, resp["ok"])
}

func TestWindow_ToolWindowFocused_Good_NoneFocused(t *core.T) {
	s, c := windowHarness(t)
	openWindow(t, c, "tray")
	resp := s.toolWindowFocused()
	core.AssertEqual(t, true, resp["ok"])
	core.AssertNil(t, resp["value"])
}

func TestWindow_ToolWindowVisibility_Good(t *core.T) {
	s, c := windowHarness(t)
	openWindow(t, c, "tray")
	resp := s.toolWindowVisibility(map[string]any{"name": "tray", "visible": false})
	core.AssertEqual(t, true, resp["ok"])
	core.AssertEqual(t, false, resp["visible"])
}

func TestWindow_ToolWindowAlwaysOnTop_Good(t *core.T) {
	s, c := windowHarness(t)
	openWindow(t, c, "tray")
	resp := s.toolWindowAlwaysOnTop(map[string]any{"name": "tray", "enabled": true})
	core.AssertEqual(t, true, resp["ok"])
	core.AssertEqual(t, true, resp["always_on_top"])
}

func TestWindow_ToolWindowSetTitle_Good(t *core.T) {
	s, c := windowHarness(t)
	openWindow(t, c, "tray")
	resp := s.toolWindowSetTitle(map[string]any{"name": "tray", "title": "Lethean"})
	core.AssertEqual(t, true, resp["ok"])

	got := s.toolWindowGetTitle(map[string]any{"name": "tray"})
	core.AssertEqual(t, true, got["ok"])
	core.AssertEqual(t, "Lethean", got["title"])
}

func TestWindow_ToolWindowGetTitle_Bad_Unknown(t *core.T) {
	s, _ := windowHarness(t)
	resp := s.toolWindowGetTitle(map[string]any{"name": "ghost"})
	core.AssertEqual(t, false, resp["ok"])
}

func TestWindow_ToolWindowFullscreen_Good(t *core.T) {
	s, c := windowHarness(t)
	openWindow(t, c, "tray")
	resp := s.toolWindowFullscreen(map[string]any{"name": "tray", "enabled": true})
	core.AssertEqual(t, true, resp["ok"])
	core.AssertEqual(t, true, resp["fullscreen"])
}

func TestWindow_ToolWindowClose_Good(t *core.T) {
	s, c := windowHarness(t)
	openWindow(t, c, "tray")
	resp := s.toolWindowClose(map[string]any{"name": "tray"})
	core.AssertEqual(t, true, resp["ok"])
	core.AssertEqual(t, "tray", resp["closed"])
}

func TestWindow_ToolWindowCenter_Good(t *core.T) {
	s, c := windowHarness(t)
	openWindow(t, c, "tray")
	resp := s.toolWindowCenter(map[string]any{"name": "tray"})
	core.AssertEqual(t, true, resp["ok"])
	core.AssertEqual(t, "tray", resp["centered"])
}

func TestWindow_ToolWindowBackgroundColour_Good(t *core.T) {
	s, c := windowHarness(t)
	openWindow(t, c, "tray")
	resp := s.toolWindowBackgroundColour(map[string]any{"name": "tray", "r": 10, "g": 20, "b": 30, "a": 40})
	core.AssertEqual(t, true, resp["ok"])
	rgba, ok := resp["rgba"].([]uint8)
	core.AssertTrue(t, ok)
	core.AssertEqual(t, []uint8{10, 20, 30, 40}, rgba)
}

func TestWindow_ToolWindowBackgroundColour_Bad_Unknown(t *core.T) {
	s, _ := windowHarness(t)
	resp := s.toolWindowBackgroundColour(map[string]any{"name": "ghost"})
	core.AssertEqual(t, false, resp["ok"])
}

// ─── windowInfo() nil guard ─────────────────────────────────────────

func TestWindow_WindowInfo_Ugly_NilInput(t *core.T) {
	core.AssertNil(t, windowInfo(nil))
}
