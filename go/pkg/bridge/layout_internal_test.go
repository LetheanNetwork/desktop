// SPDX-Licence-Identifier: EUPL-1.2

// layout.go tests — internal (package bridge) because every symbol
// under test (captureLayout, applyLayout, toolLayout*, pickWindows,
// activeWorkArea, loadLayout, ...) is unexported. Combines three
// hermetic seams already established elsewhere in this package:
//   - homeFixture (auth_test.go)      — redirects layoutsRoot()'s
//     $HOME/Lethean/conf/layouts to a t.TempDir().
//   - guiwindow.Register(MockPlatform) (window_test.go's pattern)
//   - guiscreen.Register(stub)         (screen_test.go's pattern)

package bridge

import (
	core "dappco.re/go"
	guiscreen "dappco.re/go/render/display/webkit/pkg/screen"
	guiwindow "dappco.re/go/render/display/webkit/pkg/window"
)

// layoutHarness wires window + screen services (both real, mock-
// backed) plus a HOME fixture so layoutsRoot()'s on-disk path is
// hermetic. Returns the bridge Service and the shared Core so tests
// can open/close windows via openWindow(t, c, name).
func layoutHarness(t *core.T) (*Service, *core.Core) {
	t.Helper()
	homeFixture(t)
	c := core.New()
	rw := guiwindow.Register(guiwindow.NewMockPlatform())(c)
	core.AssertTrue(t, rw.OK)
	core.AssertTrue(t, rw.Value.(*guiwindow.Service).OnStartup(core.Background()).OK)
	rs := guiscreen.Register(&stubScreenPlatform{screens: []guiscreen.Screen{
		{
			ID: "primary", Name: "Built-in", IsPrimary: true,
			Bounds:   guiscreen.Rect{X: 0, Y: 0, Width: 1920, Height: 1080},
			WorkArea: guiscreen.Rect{X: 0, Y: 25, Width: 1920, Height: 1055},
		},
	}})(c)
	core.AssertTrue(t, rs.OK)
	core.AssertTrue(t, rs.Value.(*guiscreen.Service).OnStartup(core.Background()).OK)
	return &Service{ServiceRuntime: core.NewServiceRuntime[Options](c, Options{}), port: 9999}, c
}

// ─── toolLayoutSave / toolLayoutGet / toolLayoutList / toolLayoutDelete ──

func TestLayout_ToolLayoutSave_Good(t *core.T) {
	s, c := layoutHarness(t)
	openWindow(t, c, "tray")
	openWindow(t, c, "chat")

	resp := s.toolLayoutSave(map[string]any{"name": "autosave"})
	core.AssertEqual(t, true, resp["ok"])
	core.AssertEqual(t, 2, resp["windows"])
}

func TestLayout_ToolLayoutSave_Bad_MissingNameParam(t *core.T) {
	s, _ := layoutHarness(t)
	resp := s.toolLayoutSave(map[string]any{})
	core.AssertEqual(t, false, resp["ok"])
	core.AssertEqual(t, nameParamRequired, resp["error"])
}

func TestLayout_ToolLayoutSave_Ugly_LayoutsRootBlockedByFile(t *core.T) {
	s, c := layoutHarness(t)
	openWindow(t, c, "tray")
	root := layoutsRoot()
	core.AssertTrue(t, root.OK)
	parent := core.PathDir(root.Value.(string))
	core.AssertTrue(t, core.MkdirAll(parent, 0o755).OK)
	// Plant a regular file where the layouts directory should be —
	// MkdirAll must fail cleanly rather than silently overwrite it.
	core.AssertTrue(t, core.WriteFile(root.Value.(string), []byte("not a dir"), 0o644).OK)

	resp := s.toolLayoutSave(map[string]any{"name": "blocked"})
	core.AssertEqual(t, false, resp["ok"])
	core.AssertContains(t, resp["error"], "mkdir")
}

func TestLayout_ToolLayoutGet_Good(t *core.T) {
	s, c := layoutHarness(t)
	openWindow(t, c, "tray")
	core.AssertEqual(t, true, s.toolLayoutSave(map[string]any{"name": "mine"})["ok"])

	resp := s.toolLayoutGet(map[string]any{"name": "mine"})
	core.AssertEqual(t, true, resp["ok"])
	layout, ok := resp["value"].(*Layout)
	core.AssertTrue(t, ok)
	core.AssertEqual(t, "mine", layout.Name)
}

func TestLayout_ToolLayoutGet_Bad_MissingNameParam(t *core.T) {
	s, _ := layoutHarness(t)
	resp := s.toolLayoutGet(map[string]any{})
	core.AssertEqual(t, false, resp["ok"])
	core.AssertEqual(t, nameParamRequired, resp["error"])
}

func TestLayout_ToolLayoutGet_Ugly_NotFound(t *core.T) {
	s, _ := layoutHarness(t)
	resp := s.toolLayoutGet(map[string]any{"name": "absent"})
	core.AssertEqual(t, false, resp["ok"])
	core.AssertContains(t, resp["error"], "read")
}

func TestLayout_ToolLayoutList_Good(t *core.T) {
	s, c := layoutHarness(t)
	openWindow(t, c, "tray")
	core.AssertEqual(t, true, s.toolLayoutSave(map[string]any{"name": "one"})["ok"])
	core.AssertEqual(t, true, s.toolLayoutSave(map[string]any{"name": "two"})["ok"])

	resp := s.toolLayoutList()
	core.AssertEqual(t, true, resp["ok"])
	core.AssertEqual(t, 2, resp["count"])
}

func TestLayout_ToolLayoutList_Ugly_EmptyDirectory(t *core.T) {
	s, _ := layoutHarness(t)
	resp := s.toolLayoutList()
	core.AssertEqual(t, true, resp["ok"])
	core.AssertEqual(t, 0, resp["count"])
}

func TestLayout_ToolLayoutList_Ugly_SkipsBrokenLayoutFile(t *core.T) {
	s, c := layoutHarness(t)
	openWindow(t, c, "tray")
	core.AssertEqual(t, true, s.toolLayoutSave(map[string]any{"name": "good"})["ok"])

	root := layoutsRoot().Value.(string)
	core.AssertTrue(t, core.WriteFile(core.PathJoin(root, "broken.json"), []byte("{not json"), 0o644).OK)

	resp := s.toolLayoutList()
	core.AssertEqual(t, true, resp["ok"])
	core.AssertEqual(t, 1, resp["count"], "broken.json must be skipped, not counted or fatal")
}

func TestLayout_ToolLayoutDelete_Good(t *core.T) {
	s, c := layoutHarness(t)
	openWindow(t, c, "tray")
	core.AssertEqual(t, true, s.toolLayoutSave(map[string]any{"name": "temp"})["ok"])

	resp := s.toolLayoutDelete(map[string]any{"name": "temp"})
	core.AssertEqual(t, true, resp["ok"])

	after := s.toolLayoutGet(map[string]any{"name": "temp"})
	core.AssertEqual(t, false, after["ok"])
}

func TestLayout_ToolLayoutDelete_Bad_MissingNameParam(t *core.T) {
	s, _ := layoutHarness(t)
	resp := s.toolLayoutDelete(map[string]any{})
	core.AssertEqual(t, false, resp["ok"])
	core.AssertEqual(t, nameParamRequired, resp["error"])
}

// ─── toolLayoutRestore / applyLayout ────────────────────────────────

func TestLayout_ToolLayoutRestore_Good(t *core.T) {
	s, c := layoutHarness(t)
	openWindow(t, c, "tray")
	openWindow(t, c, "chat")
	core.AssertEqual(t, true, s.toolLayoutSave(map[string]any{"name": "both"})["ok"])

	resp := s.toolLayoutRestore(map[string]any{"name": "both"})
	core.AssertEqual(t, true, resp["ok"])
	core.AssertEqual(t, 2, resp["applied"])
	core.AssertEqual(t, 0, resp["skipped"])
}

func TestLayout_ToolLayoutRestore_Ugly_SkipsClosedWindow(t *core.T) {
	s, c := layoutHarness(t)
	openWindow(t, c, "tray")
	openWindow(t, c, "ephemeral")
	core.AssertEqual(t, true, s.toolLayoutSave(map[string]any{"name": "partial"})["ok"])

	closeResp := s.toolWindowClose(map[string]any{"name": "ephemeral"})
	core.AssertEqual(t, true, closeResp["ok"])

	resp := s.toolLayoutRestore(map[string]any{"name": "partial"})
	core.AssertEqual(t, true, resp["ok"])
	core.AssertEqual(t, 1, resp["applied"])
	core.AssertEqual(t, 1, resp["skipped"])
}

func TestLayout_ToolLayoutRestore_Bad_MissingNameParam(t *core.T) {
	s, _ := layoutHarness(t)
	resp := s.toolLayoutRestore(map[string]any{})
	core.AssertEqual(t, false, resp["ok"])
	core.AssertEqual(t, nameParamRequired, resp["error"])
}

func TestLayout_ToolLayoutRestore_Ugly_LayoutNotFound(t *core.T) {
	s, _ := layoutHarness(t)
	resp := s.toolLayoutRestore(map[string]any{"name": "absent"})
	core.AssertEqual(t, false, resp["ok"])
	core.AssertContains(t, resp["error"], "read")
}

// ─── pickWindows ────────────────────────────────────────────────────

func TestLayout_PickWindows_Good_ExplicitList(t *core.T) {
	s, c := layoutHarness(t)
	openWindow(t, c, "tray")
	openWindow(t, c, "chat")
	out, errResp := s.pickWindows(map[string]any{"windows": []any{"tray", "ghost", "chat"}})
	core.AssertNil(t, errResp)
	core.AssertEqual(t, 2, len(out), "unknown window names must be silently dropped, not fatal")
}

func TestLayout_PickWindows_Good_FallsBackToAllWindows(t *core.T) {
	s, c := layoutHarness(t)
	openWindow(t, c, "tray")
	out, errResp := s.pickWindows(map[string]any{})
	core.AssertNil(t, errResp)
	core.AssertEqual(t, 1, len(out))
}

// ─── toolLayoutTile ─────────────────────────────────────────────────

func TestLayout_ToolLayoutTile_Good_Left(t *core.T) {
	s, c := layoutHarness(t)
	openWindow(t, c, "tray")
	resp := s.toolLayoutTile(map[string]any{"mode": "left"})
	core.AssertEqual(t, true, resp["ok"])
	core.AssertEqual(t, 1, resp["applied"])
}

func TestLayout_ToolLayoutTile_Good_Right(t *core.T) {
	s, c := layoutHarness(t)
	openWindow(t, c, "tray")
	resp := s.toolLayoutTile(map[string]any{"mode": "right"})
	core.AssertEqual(t, true, resp["ok"])
}

func TestLayout_ToolLayoutTile_Good_Top(t *core.T) {
	s, c := layoutHarness(t)
	openWindow(t, c, "tray")
	resp := s.toolLayoutTile(map[string]any{"mode": "top"})
	core.AssertEqual(t, true, resp["ok"])
}

func TestLayout_ToolLayoutTile_Good_Bottom(t *core.T) {
	s, c := layoutHarness(t)
	openWindow(t, c, "tray")
	resp := s.toolLayoutTile(map[string]any{"mode": "bottom"})
	core.AssertEqual(t, true, resp["ok"])
}

func TestLayout_ToolLayoutTile_Good_Halves(t *core.T) {
	s, c := layoutHarness(t)
	openWindow(t, c, "tray")
	openWindow(t, c, "chat")
	resp := s.toolLayoutTile(map[string]any{"mode": "halves"})
	core.AssertEqual(t, true, resp["ok"])
	core.AssertEqual(t, 2, resp["applied"])
}

func TestLayout_ToolLayoutTile_Bad_HalvesNeedsTwo(t *core.T) {
	s, c := layoutHarness(t)
	openWindow(t, c, "tray")
	resp := s.toolLayoutTile(map[string]any{"mode": "halves"})
	core.AssertEqual(t, false, resp["ok"])
	core.AssertContains(t, resp["error"], "halves needs")
}

func TestLayout_ToolLayoutTile_Good_Thirds(t *core.T) {
	s, c := layoutHarness(t)
	openWindow(t, c, "a")
	openWindow(t, c, "b")
	openWindow(t, c, "c")
	resp := s.toolLayoutTile(map[string]any{"mode": "thirds"})
	core.AssertEqual(t, true, resp["ok"])
	core.AssertEqual(t, 3, resp["applied"])
}

func TestLayout_ToolLayoutTile_Bad_ThirdsNeedsThree(t *core.T) {
	s, c := layoutHarness(t)
	openWindow(t, c, "a")
	openWindow(t, c, "b")
	resp := s.toolLayoutTile(map[string]any{"mode": "thirds"})
	core.AssertEqual(t, false, resp["ok"])
	core.AssertContains(t, resp["error"], "thirds needs")
}

func TestLayout_ToolLayoutTile_Good_Quadrants(t *core.T) {
	s, c := layoutHarness(t)
	openWindow(t, c, "a")
	openWindow(t, c, "b")
	openWindow(t, c, "c")
	openWindow(t, c, "d")
	resp := s.toolLayoutTile(map[string]any{"mode": "quadrants"})
	core.AssertEqual(t, true, resp["ok"])
	core.AssertEqual(t, 4, resp["applied"])
}

func TestLayout_ToolLayoutTile_Good_Grid(t *core.T) {
	s, c := layoutHarness(t)
	openWindow(t, c, "a")
	openWindow(t, c, "b")
	openWindow(t, c, "c")
	resp := s.toolLayoutTile(map[string]any{"mode": "grid"})
	core.AssertEqual(t, true, resp["ok"])
	core.AssertEqual(t, 3, resp["applied"])
}

func TestLayout_ToolLayoutTile_Bad_UnknownMode(t *core.T) {
	s, c := layoutHarness(t)
	openWindow(t, c, "tray")
	resp := s.toolLayoutTile(map[string]any{"mode": "spiral"})
	core.AssertEqual(t, false, resp["ok"])
	core.AssertContains(t, resp["error"], "unknown mode")
}

func TestLayout_ToolLayoutTile_Bad_NoWindowsToTile(t *core.T) {
	s, _ := layoutHarness(t)
	resp := s.toolLayoutTile(map[string]any{"mode": "grid"})
	core.AssertEqual(t, false, resp["ok"])
	core.AssertEqual(t, "no windows to tile", resp["error"])
}

// ─── toolLayoutSnap ─────────────────────────────────────────────────

func TestLayout_ToolLayoutSnap_Good_AllPositions(t *core.T) {
	positions := []string{
		"left", "right", "top", "bottom",
		"top-left", "top-right", "bottom-left", "bottom-right",
		"centre", "center",
	}
	for _, pos := range positions {
		s, c := layoutHarness(t)
		openWindow(t, c, "tray")
		resp := s.toolLayoutSnap(map[string]any{"name": "tray", "position": pos})
		core.AssertEqual(t, true, resp["ok"], "position %q must succeed", pos)
	}
}

func TestLayout_ToolLayoutSnap_Bad_MissingNameParam(t *core.T) {
	s, _ := layoutHarness(t)
	resp := s.toolLayoutSnap(map[string]any{"position": "left"})
	core.AssertEqual(t, false, resp["ok"])
	core.AssertEqual(t, nameParamRequired, resp["error"])
}

func TestLayout_ToolLayoutSnap_Bad_MissingPositionParam(t *core.T) {
	s, c := layoutHarness(t)
	openWindow(t, c, "tray")
	resp := s.toolLayoutSnap(map[string]any{"name": "tray"})
	core.AssertEqual(t, false, resp["ok"])
	core.AssertEqual(t, "position param required", resp["error"])
}

func TestLayout_ToolLayoutSnap_Ugly_UnknownPosition(t *core.T) {
	s, c := layoutHarness(t)
	openWindow(t, c, "tray")
	resp := s.toolLayoutSnap(map[string]any{"name": "tray", "position": "diagonally"})
	core.AssertEqual(t, false, resp["ok"])
	core.AssertContains(t, resp["error"], "unknown position")
}

func TestLayout_ToolLayoutSnap_Ugly_UnknownWindow(t *core.T) {
	s, _ := layoutHarness(t)
	resp := s.toolLayoutSnap(map[string]any{"name": "ghost", "position": "left"})
	core.AssertEqual(t, false, resp["ok"])
}

// ─── toolLayoutStack ────────────────────────────────────────────────

func TestLayout_ToolLayoutStack_Good_Defaults(t *core.T) {
	s, c := layoutHarness(t)
	openWindow(t, c, "a")
	openWindow(t, c, "b")
	resp := s.toolLayoutStack(map[string]any{})
	core.AssertEqual(t, true, resp["ok"])
	core.AssertEqual(t, 2, resp["stacked"])
	core.AssertEqual(t, 30, resp["offsetX"])
}

func TestLayout_ToolLayoutStack_Good_CustomOffsets(t *core.T) {
	s, c := layoutHarness(t)
	openWindow(t, c, "a")
	resp := s.toolLayoutStack(map[string]any{"offsetX": 5, "offsetY": 10, "width": 400, "height": 300})
	core.AssertEqual(t, true, resp["ok"])
	core.AssertEqual(t, 5, resp["offsetX"])
	core.AssertEqual(t, 10, resp["offsetY"])
}

func TestLayout_ToolLayoutStack_Bad_NoWindowsToStack(t *core.T) {
	s, _ := layoutHarness(t)
	resp := s.toolLayoutStack(map[string]any{})
	core.AssertEqual(t, false, resp["ok"])
	core.AssertEqual(t, "no windows to stack", resp["error"])
}

// ─── toolLayoutWorkflow ─────────────────────────────────────────────

func TestLayout_ToolLayoutWorkflow_Good_Default(t *core.T) {
	s, c := layoutHarness(t)
	openWindow(t, c, "tray")
	openWindow(t, c, "chat")
	resp := s.toolLayoutWorkflow(map[string]any{"workflow": "default"})
	core.AssertEqual(t, true, resp["ok"])
	core.AssertEqual(t, "default", resp["workflow"])
}

func TestLayout_ToolLayoutWorkflow_Good_Coding(t *core.T) {
	s, c := layoutHarness(t)
	openWindow(t, c, "tray")
	openWindow(t, c, "editor")
	openWindow(t, c, "git")
	resp := s.toolLayoutWorkflow(map[string]any{"workflow": "coding"})
	core.AssertEqual(t, true, resp["ok"])
}

func TestLayout_ToolLayoutWorkflow_Good_Review(t *core.T) {
	s, c := layoutHarness(t)
	openWindow(t, c, "tray")
	openWindow(t, c, "chat")
	openWindow(t, c, "models")
	resp := s.toolLayoutWorkflow(map[string]any{"workflow": "review"})
	core.AssertEqual(t, true, resp["ok"])
}

func TestLayout_ToolLayoutWorkflow_Good_Ops(t *core.T) {
	s, c := layoutHarness(t)
	openWindow(t, c, "tray")
	openWindow(t, c, "telemetry")
	openWindow(t, c, "logs")
	openWindow(t, c, "containers")
	resp := s.toolLayoutWorkflow(map[string]any{"workflow": "ops"})
	core.AssertEqual(t, true, resp["ok"])
}

func TestLayout_ToolLayoutWorkflow_Good_Single(t *core.T) {
	s, c := layoutHarness(t)
	openWindow(t, c, "tray")
	openWindow(t, c, "editor")
	resp := s.toolLayoutWorkflow(map[string]any{"workflow": "single", "name": "editor"})
	core.AssertEqual(t, true, resp["ok"])
	core.AssertEqual(t, "editor", resp["focused"])
}

func TestLayout_ToolLayoutWorkflow_Bad_SingleMissingName(t *core.T) {
	s, _ := layoutHarness(t)
	resp := s.toolLayoutWorkflow(map[string]any{"workflow": "single"})
	core.AssertEqual(t, false, resp["ok"])
	core.AssertContains(t, resp["error"], "single workflow needs")
}

func TestLayout_ToolLayoutWorkflow_Bad_UnknownWorkflow(t *core.T) {
	s, _ := layoutHarness(t)
	resp := s.toolLayoutWorkflow(map[string]any{"workflow": "chaos"})
	core.AssertEqual(t, false, resp["ok"])
	core.AssertContains(t, resp["error"], "unknown workflow")
}

// ─── loadLayout direct ──────────────────────────────────────────────

func TestLayout_LoadLayout_Ugly_UnparseableJSON(t *core.T) {
	s, _ := layoutHarness(t)
	root := layoutsRoot().Value.(string)
	core.AssertTrue(t, core.MkdirAll(root, 0o755).OK)
	core.AssertTrue(t, core.WriteFile(core.PathJoin(root, "bad.json"), []byte("{not json"), 0o644).OK)

	layout, errResp := s.loadLayout("bad")
	core.AssertNil(t, layout)
	core.AssertNotNil(t, errResp)
	core.AssertContains(t, errResp["error"], "parse")
}
