// SPDX-Licence-Identifier: EUPL-1.2

// screen.go tests. Registers the REAL core/gui screen.Service against
// a small in-memory stub Platform (screen.Platform is a 3-method
// interface with no exported mock upstream, so the stub lives here).

package bridge

import (
	core "dappco.re/go"
	guiscreen "dappco.re/go/render/display/webkit/pkg/screen"
	guiwindow "dappco.re/go/render/display/webkit/pkg/window"
)

// stubScreenPlatform is a minimal in-memory guiscreen.Platform.
type stubScreenPlatform struct {
	screens []guiscreen.Screen
}

func (p *stubScreenPlatform) GetAll() []guiscreen.Screen { return p.screens }
func (p *stubScreenPlatform) GetPrimary() *guiscreen.Screen {
	for i := range p.screens {
		if p.screens[i].IsPrimary {
			return &p.screens[i]
		}
	}
	return nil
}
func (p *stubScreenPlatform) GetCurrent() *guiscreen.Screen { return p.GetPrimary() }

func twoScreenPlatform() *stubScreenPlatform {
	return &stubScreenPlatform{screens: []guiscreen.Screen{
		{
			ID: "primary", Name: "Built-in", ScaleFactor: 2, IsPrimary: true,
			Bounds:   guiscreen.Rect{X: 0, Y: 0, Width: 1920, Height: 1080},
			WorkArea: guiscreen.Rect{X: 0, Y: 25, Width: 1920, Height: 1055},
		},
		{
			ID: "second", Name: "External", ScaleFactor: 1, IsPrimary: false,
			Bounds:   guiscreen.Rect{X: 1920, Y: 0, Width: 2560, Height: 1440},
			WorkArea: guiscreen.Rect{X: 1920, Y: 0, Width: 2560, Height: 1440},
		},
	}}
}

func screenHarness(t *core.T, platform guiscreen.Platform) *Service {
	t.Helper()
	c := core.New()
	r := guiscreen.Register(platform)(c)
	core.AssertTrue(t, r.OK)
	svc := r.Value.(*guiscreen.Service)
	core.AssertTrue(t, svc.OnStartup(core.Background()).OK)
	return &Service{ServiceRuntime: core.NewServiceRuntime[Options](c, Options{}), port: 9999}
}

// ─── toolScreenList ─────────────────────────────────────────────────

func TestScreen_ToolScreenList_Good(t *core.T) {
	s := screenHarness(t, twoScreenPlatform())
	resp := s.toolScreenList()
	core.AssertEqual(t, true, resp["ok"])
	core.AssertEqual(t, 2, resp["count"])
}

func TestScreen_ToolScreenList_Ugly_NoScreens(t *core.T) {
	s := screenHarness(t, &stubScreenPlatform{})
	resp := s.toolScreenList()
	core.AssertEqual(t, true, resp["ok"])
	core.AssertEqual(t, 0, resp["count"])
}

func TestScreen_ToolScreenList_Bad_NoScreenService(t *core.T) {
	c := core.New()
	s := &Service{ServiceRuntime: core.NewServiceRuntime[Options](c, Options{}), port: 9999}
	resp := s.toolScreenList()
	core.AssertEqual(t, false, resp["ok"])
	core.AssertNotEmpty(t, resp["error"])
}

// ─── toolScreenPrimary ──────────────────────────────────────────────

func TestScreen_ToolScreenPrimary_Good(t *core.T) {
	s := screenHarness(t, twoScreenPlatform())
	resp := s.toolScreenPrimary()
	core.AssertEqual(t, true, resp["ok"])
	value := resp["value"].(map[string]any)
	core.AssertEqual(t, "primary", value["id"])
}

func TestScreen_ToolScreenPrimary_Ugly_NonePrimary(t *core.T) {
	s := screenHarness(t, &stubScreenPlatform{})
	resp := s.toolScreenPrimary()
	core.AssertEqual(t, false, resp["ok"])
	core.AssertEqual(t, "no primary screen", resp["error"])
}

// ─── toolScreenGet ──────────────────────────────────────────────────

func TestScreen_ToolScreenGet_Good(t *core.T) {
	s := screenHarness(t, twoScreenPlatform())
	resp := s.toolScreenGet(map[string]any{"id": "second"})
	core.AssertEqual(t, true, resp["ok"])
	value := resp["value"].(map[string]any)
	core.AssertEqual(t, "External", value["name"])
}

func TestScreen_ToolScreenGet_Bad_MissingIDParam(t *core.T) {
	s := screenHarness(t, twoScreenPlatform())
	resp := s.toolScreenGet(map[string]any{})
	core.AssertEqual(t, false, resp["ok"])
	core.AssertEqual(t, idParamRequired, resp["error"])
}

func TestScreen_ToolScreenGet_Ugly_UnknownID(t *core.T) {
	s := screenHarness(t, twoScreenPlatform())
	resp := s.toolScreenGet(map[string]any{"id": "ghost"})
	core.AssertEqual(t, false, resp["ok"])
	core.AssertContains(t, resp["error"], "screen not found")
}

// ─── toolScreenAtPoint ──────────────────────────────────────────────

func TestScreen_ToolScreenAtPoint_Good(t *core.T) {
	s := screenHarness(t, twoScreenPlatform())
	resp := s.toolScreenAtPoint(map[string]any{"x": 2000, "y": 100})
	core.AssertEqual(t, true, resp["ok"])
	value := resp["value"].(map[string]any)
	core.AssertEqual(t, "second", value["id"])
}

func TestScreen_ToolScreenAtPoint_Ugly_NoScreenContainsPoint(t *core.T) {
	s := screenHarness(t, twoScreenPlatform())
	resp := s.toolScreenAtPoint(map[string]any{"x": -500, "y": -500})
	core.AssertEqual(t, false, resp["ok"])
	core.AssertEqual(t, "no screen contains point", resp["error"])
}

// ─── toolScreenForWindow ────────────────────────────────────────────

func TestScreen_ToolScreenForWindow_Good(t *core.T) {
	c := core.New()
	rs := guiscreen.Register(twoScreenPlatform())(c)
	core.AssertTrue(t, rs.OK)
	core.AssertTrue(t, rs.Value.(*guiscreen.Service).OnStartup(core.Background()).OK)
	rw := guiwindow.Register(guiwindow.NewMockPlatform())(c)
	core.AssertTrue(t, rw.OK)
	core.AssertTrue(t, rw.Value.(*guiwindow.Service).OnStartup(core.Background()).OK)
	s := &Service{ServiceRuntime: core.NewServiceRuntime[Options](c, Options{}), port: 9999}

	openWindow(t, c, "tray")
	moved := s.toolWindowPosition(map[string]any{"name": "tray", "x": 2000, "y": 100})
	core.AssertEqual(t, true, moved["ok"])

	resp := s.toolScreenForWindow(map[string]any{"name": "tray"})
	core.AssertEqual(t, true, resp["ok"])
	value := resp["value"].(map[string]any)
	core.AssertEqual(t, "second", value["id"])
}

func TestScreen_ToolScreenForWindow_Bad_MissingNameParam(t *core.T) {
	s := screenHarness(t, twoScreenPlatform())
	resp := s.toolScreenForWindow(map[string]any{})
	core.AssertEqual(t, false, resp["ok"])
	core.AssertEqual(t, nameParamRequired, resp["error"])
}

func TestScreen_ToolScreenForWindow_Ugly_UnknownWindow(t *core.T) {
	// Window service registered but "ghost" was never opened — the
	// QueryWindowByName query succeeds with a nil result, which is the
	// only path that reaches the "window not found" message (a bare
	// core with no window service at all fails the QUERY call itself
	// and surfaces a different, generic error).
	c := core.New()
	rs := guiscreen.Register(twoScreenPlatform())(c)
	core.AssertTrue(t, rs.OK)
	core.AssertTrue(t, rs.Value.(*guiscreen.Service).OnStartup(core.Background()).OK)
	rw := guiwindow.Register(guiwindow.NewMockPlatform())(c)
	core.AssertTrue(t, rw.OK)
	core.AssertTrue(t, rw.Value.(*guiwindow.Service).OnStartup(core.Background()).OK)
	s := &Service{ServiceRuntime: core.NewServiceRuntime[Options](c, Options{}), port: 9999}

	resp := s.toolScreenForWindow(map[string]any{"name": "ghost"})
	core.AssertEqual(t, false, resp["ok"])
	core.AssertContains(t, resp["error"], "window not found")
}

// ─── toolScreenWorkAreas ────────────────────────────────────────────

func TestScreen_ToolScreenWorkAreas_Good(t *core.T) {
	s := screenHarness(t, twoScreenPlatform())
	resp := s.toolScreenWorkAreas()
	core.AssertEqual(t, true, resp["ok"])
	core.AssertEqual(t, 2, resp["count"])
}

func TestScreen_ToolScreenWorkAreas_Bad_NoScreenService(t *core.T) {
	c := core.New()
	s := &Service{ServiceRuntime: core.NewServiceRuntime[Options](c, Options{}), port: 9999}
	resp := s.toolScreenWorkAreas()
	core.AssertEqual(t, false, resp["ok"])
	core.AssertNotEmpty(t, resp["error"])
}

// ─── screenInfo() nil guard ─────────────────────────────────────────

func TestScreen_ScreenInfo_Ugly_NilInput(t *core.T) {
	core.AssertNil(t, screenInfo(nil))
}
