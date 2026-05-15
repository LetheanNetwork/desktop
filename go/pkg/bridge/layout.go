// SPDX-Licence-Identifier: EUPL-1.2

// layout.go — bridge tools for window-arrangement persistence.
//
// Tools:
//
//	layout_save     params: { name }                    — capture every Wails window's pos/size/state → ~/Lethean/conf/layouts/<name>.json
//	layout_restore  params: { name }                    — read <name>.json → SetPosition/SetSize/Maximise/Hide each window
//	layout_list     params: {}                           — list every saved layout (name + saved_at)
//	layout_delete   params: { name }                    — rm ~/Lethean/conf/layouts/<name>.json
//	layout_get      params: { name }                    — return one layout's full JSON
//
// Persistence path: $HOME/Lethean/conf/layouts/<name>.json — keeps the layout
// alongside other conf/ artefacts per the no-hidden-bloat principle.
//
// Each window entry serialises name + x/y + w/h + visible + maximised +
// fullscreen + always-on-top. Minimised state isn't persisted (a minimised
// window restores to its pre-minimise position anyway; the user almost never
// wants "minimised" as a saved state).

package bridge

import (

	core "dappco.re/go"
	guiscreen "dappco.re/go/gui/pkg/screen"
	guiwindow "dappco.re/go/gui/pkg/window"
)

// WindowState is the serialised shape of one window in a layout.
type WindowState struct {
	Name        string `json:"name"`
	X           int    `json:"x"`
	Y           int    `json:"y"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	Visible     bool   `json:"visible"`
	Maximised   bool   `json:"maximised,omitempty"`
	Fullscreen  bool   `json:"fullscreen,omitempty"`
	AlwaysOnTop bool   `json:"always_on_top,omitempty"`
}

// Layout is the serialised shape of a saved layout file.
type Layout struct {
	Name    string        `json:"name"`
	SavedAt core.Time     `json:"saved_at"`
	Windows []WindowState `json:"windows"`
}

// layoutsRoot returns $HOME/Lethean/conf/layouts. Mirrors the
// pkg/plugin install-root pattern — visible under ~/Lethean/.
func layoutsRoot() core.Result {
	home := core.UserHomeDir()
	if !home.OK {
		return core.Fail(core.E("bridge.layoutsRoot", "home dir unavailable", nil))
	}
	homeDir, _ := home.Value.(string)
	if homeDir == "" {
		return core.Fail(core.E("bridge.layoutsRoot", "home dir empty", nil))
	}
	return core.Ok(core.PathJoin(homeDir, "Lethean", "conf", "layouts"))
}

// captureLayout snapshots every core/gui-registered window into a Layout.
func (s *Service) captureLayout(name string) (*Layout, map[string]any) {
	all, errResp := s.coreGUIWindowList()
	if errResp != nil {
		return nil, errResp
	}
	out := make([]WindowState, 0, len(all))
	for _, info := range all {
		// TODO(snider): core/gui needs visibility/fullscreen/always-on-top
		// in WindowInfo so lthn layout snapshots can preserve those fields
		// without direct Wails access.
		out = append(out, WindowState{
			Name:      info.Name,
			X:         info.X,
			Y:         info.Y,
			Width:     info.Width,
			Height:    info.Height,
			Visible:   true,
			Maximised: info.Maximized,
		})
	}
	return &Layout{
		Name:    name,
		SavedAt: core.Now(),
		Windows: out,
	}, nil
}

// applyLayout reads a Layout and pushes each WindowState through the
// core/gui window framework. Windows in the layout that no longer exist
// are skipped; windows present today but absent from the layout are
// left alone.
func (s *Service) applyLayout(layout *Layout) map[string]any {
	applied := 0
	skipped := 0
	for _, ws := range layout.Windows {
		if _, errResp := s.coreGUIWindowInfo(ws.Name); errResp != nil {
			skipped++
			continue
		}
		if errResp := s.applyWindowState(ws); errResp != nil {
			return errResp
		}
		applied++
	}
	return map[string]any{
		"ok":      true,
		"applied": applied,
		"skipped": skipped,
		"name":    layout.Name,
	}
}

func (s *Service) applyWindowState(ws WindowState) map[string]any {
	// Order matters: restore + leave fullscreen first so SetBounds lands in
	// the right frame; the saved state then re-applies chrome flags.
	if errResp := s.runCoreGUIWindowTask("window.restore", guiwindow.TaskRestore{Name: ws.Name}); errResp != nil {
		return errResp
	}
	if errResp := s.runCoreGUIWindowTask("window.fullscreen", guiwindow.TaskFullscreen{Name: ws.Name, Fullscreen: false}); errResp != nil {
		return errResp
	}
	if errResp := s.runCoreGUIWindowTask("window.set_bounds", guiwindow.TaskSetBounds{Name: ws.Name, X: ws.X, Y: ws.Y, Width: ws.Width, Height: ws.Height}); errResp != nil {
		return errResp
	}
	if errResp := s.runCoreGUIWindowTask("window.set_visibility", guiwindow.TaskSetVisibility{Name: ws.Name, Visible: ws.Visible}); errResp != nil {
		return errResp
	}
	if ws.Maximised {
		if errResp := s.runCoreGUIWindowTask("window.maximise", guiwindow.TaskMaximise{Name: ws.Name}); errResp != nil {
			return errResp
		}
	}
	if ws.Fullscreen {
		if errResp := s.runCoreGUIWindowTask("window.fullscreen", guiwindow.TaskFullscreen{Name: ws.Name, Fullscreen: true}); errResp != nil {
			return errResp
		}
	}
	if ws.AlwaysOnTop {
		if errResp := s.runCoreGUIWindowTask("window.set_always_on_top", guiwindow.TaskSetAlwaysOnTop{Name: ws.Name, AlwaysOnTop: true}); errResp != nil {
			return errResp
		}
	}
	return nil
}

// toolLayoutSave snapshots current window state to disk under the
// given name.
func (s *Service) toolLayoutSave(params map[string]any) map[string]any {
	name := core.Trim(paramString(params, "name", ""))
	if name == "" {
		return map[string]any{"ok": false, "error": nameParamRequired}
	}
	layout, errResp := s.captureLayout(name)
	if errResp != nil {
		return errResp
	}
	rootResult := layoutsRoot()
	if !rootResult.OK {
		return map[string]any{"ok": false, "error": rootResult.Error()}
	}
	root := rootResult.Value.(string)
	if r := core.MkdirAll(root, 0o755); !r.OK {
		return map[string]any{"ok": false, "error": "mkdir " + root + ": " + r.Error()}
	}
	raw := core.JSONMarshalIndent(layout, "", "  ")
	if !raw.OK {
		return map[string]any{"ok": false, "error": "marshal layout: " + raw.Error()}
	}
	bytes, _ := raw.Value.([]byte)
	path := core.PathJoin(root, name+".json")
	if r := core.WriteFile(path, bytes, 0o644); !r.OK {
		return map[string]any{"ok": false, "error": "write " + path + ": " + r.Error()}
	}
	return map[string]any{
		"ok":      true,
		"name":    name,
		"path":    path,
		"windows": len(layout.Windows),
	}
}

// toolLayoutRestore reads a saved layout and applies it.
func (s *Service) toolLayoutRestore(params map[string]any) map[string]any {
	name := core.Trim(paramString(params, "name", ""))
	if name == "" {
		return map[string]any{"ok": false, "error": nameParamRequired}
	}
	layout, errResp := s.loadLayout(name)
	if errResp != nil {
		return errResp
	}
	return s.applyLayout(layout)
}

// toolLayoutGet returns the raw layout contents without applying.
func (s *Service) toolLayoutGet(params map[string]any) map[string]any {
	name := core.Trim(paramString(params, "name", ""))
	if name == "" {
		return map[string]any{"ok": false, "error": nameParamRequired}
	}
	layout, errResp := s.loadLayout(name)
	if errResp != nil {
		return errResp
	}
	return map[string]any{"ok": true, "value": layout}
}

// toolLayoutList enumerates every saved layout.
func (s *Service) toolLayoutList() map[string]any {
	rootResult := layoutsRoot()
	if !rootResult.OK {
		return map[string]any{"ok": false, "error": rootResult.Error()}
	}
	root := rootResult.Value.(string)
	listing := core.ReadDir(core.DirFS(root), ".")
	if !listing.OK {
		// Empty dir is fine — return an empty list.
		return map[string]any{"ok": true, "value": []map[string]any{}, "count": 0}
	}
	entries, _ := listing.Value.([]core.FsDirEntry)
	out := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !core.HasSuffix(name, ".json") {
			continue
		}
		short := core.TrimSuffix(name, ".json")
		layout, errResp := s.loadLayout(short)
		if errResp != nil {
			// Skip broken files, don't fail the whole list.
			continue
		}
		out = append(out, map[string]any{
			"name":     layout.Name,
			"saved_at": layout.SavedAt,
			"windows":  len(layout.Windows),
		})
	}
	return map[string]any{"ok": true, "value": out, "count": len(out)}
}

// toolLayoutDelete removes a saved layout.
func (s *Service) toolLayoutDelete(params map[string]any) map[string]any {
	name := core.Trim(paramString(params, "name", ""))
	if name == "" {
		return map[string]any{"ok": false, "error": nameParamRequired}
	}
	rootResult := layoutsRoot()
	if !rootResult.OK {
		return map[string]any{"ok": false, "error": rootResult.Error()}
	}
	root := rootResult.Value.(string)
	path := core.PathJoin(root, name+".json")
	if r := core.RemoveAll(path); !r.OK {
		return map[string]any{"ok": false, "error": "remove " + path + ": " + r.Error()}
	}
	return map[string]any{"ok": true, "name": name, "removed": path}
}

// ─── Geometry helpers — tile / snap / stack / workflow ──────────────

// activeWorkArea returns the WorkArea of the screen containing the
// named window, or the primary screen's work area as a fallback.
// Returns x, y, width, height of the usable region (excludes dock /
// menubar).
func (s *Service) activeWorkArea(name string) (int, int, int, int, map[string]any) {
	if x, y, w, h, ok := s.windowScreenWorkArea(name); ok {
		return x, y, w, h, nil
	}
	r := s.Core().QUERY(guiscreen.QueryPrimary{})
	if !r.OK {
		return 0, 0, 0, 0, map[string]any{"ok": false, "error": r.Error()}
	}
	sc, _ := r.Value.(*guiscreen.Screen)
	if sc == nil {
		return 0, 0, 0, 0, map[string]any{"ok": false, "error": "no primary screen"}
	}
	return sc.WorkArea.X, sc.WorkArea.Y, sc.WorkArea.Width, sc.WorkArea.Height, nil
}

func (s *Service) windowScreenWorkArea(name string) (int, int, int, int, bool) {
	if name == "" {
		return 0, 0, 0, 0, false
	}
	info, errResp := s.coreGUIWindowInfo(name)
	if errResp != nil {
		return 0, 0, 0, 0, false
	}
	r := s.Core().QUERY(guiscreen.QueryAtPoint{
		X: info.X + info.Width/2,
		Y: info.Y + info.Height/2,
	})
	if !r.OK {
		return 0, 0, 0, 0, false
	}
	sc, _ := r.Value.(*guiscreen.Screen)
	if sc == nil {
		return 0, 0, 0, 0, false
	}
	return sc.WorkArea.X, sc.WorkArea.Y, sc.WorkArea.Width, sc.WorkArea.Height, true
}

// pickWindows resolves a "windows" param (string array of names)
// into a list of core/gui window names. Empty/missing resolves to every
// registered window because core/gui does not expose visibility yet.
func (s *Service) pickWindows(params map[string]any) ([]string, map[string]any) {
	names := stringSliceParam(params, "windows")
	if len(names) == 0 {
		all, errResp := s.coreGUIWindowList()
		if errResp != nil {
			return nil, errResp
		}
		out := make([]string, 0, len(all))
		for _, info := range all {
			out = append(out, info.Name)
		}
		return out, nil
	}
	out := make([]string, 0, len(names))
	for _, name := range names {
		if _, errResp := s.coreGUIWindowInfo(name); errResp == nil {
			out = append(out, name)
		}
	}
	return out, nil
}

func (s *Service) setLayoutWindowBounds(name string, x, y, w, h int) map[string]any {
	return s.runCoreGUIWindowTask("window.set_bounds", guiwindow.TaskSetBounds{Name: name, X: x, Y: y, Width: w, Height: h})
}

func (s *Service) setLayoutWindowVisibility(name string, visible bool) map[string]any {
	return s.runCoreGUIWindowTask("window.set_visibility", guiwindow.TaskSetVisibility{Name: name, Visible: visible})
}

func (s *Service) focusLayoutWindow(name string) map[string]any {
	return s.runCoreGUIWindowTask("window.focus", guiwindow.TaskFocus{Name: name})
}

// toolLayoutTile places windows into a tiled grid. modes:
//   - "left"      — first window fills left half
//   - "right"     — first window fills right half
//   - "top"       — first window fills top half
//   - "bottom"    — first window fills bottom half
//   - "grid"      — auto-pick rows/cols to fit all windows
//   - "quadrants" — 2x2 fixed (first 4 windows)
//   - "halves"    — two windows side-by-side (first 2)
//   - "thirds"    — three windows side-by-side (first 3)
//
// params: { mode, windows? }
func (s *Service) toolLayoutTile(params map[string]any) map[string]any {
	mode := core.Lower(paramString(params, "mode", "grid"))
	wins, errResp := s.pickWindows(params)
	if errResp != nil {
		return errResp
	}
	if len(wins) == 0 {
		return map[string]any{"ok": false, "error": "no windows to tile"}
	}
	x, y, w, h, errResp := s.activeWorkArea(wins[0])
	if errResp != nil {
		return errResp
	}
	applied := 0
	switch mode {
	case "left":
		if errResp := s.setLayoutWindowBounds(wins[0], x, y, w/2, h); errResp != nil {
			return errResp
		}
		applied = 1
	case "right":
		if errResp := s.setLayoutWindowBounds(wins[0], x+w/2, y, w/2, h); errResp != nil {
			return errResp
		}
		applied = 1
	case "top":
		if errResp := s.setLayoutWindowBounds(wins[0], x, y, w, h/2); errResp != nil {
			return errResp
		}
		applied = 1
	case "bottom":
		if errResp := s.setLayoutWindowBounds(wins[0], x, y+h/2, w, h/2); errResp != nil {
			return errResp
		}
		applied = 1
	case "halves":
		if len(wins) < 2 {
			return map[string]any{"ok": false, "error": "halves needs >= 2 windows"}
		}
		if errResp := s.setLayoutWindowBounds(wins[0], x, y, w/2, h); errResp != nil {
			return errResp
		}
		if errResp := s.setLayoutWindowBounds(wins[1], x+w/2, y, w/2, h); errResp != nil {
			return errResp
		}
		applied = 2
	case "thirds":
		if len(wins) < 3 {
			return map[string]any{"ok": false, "error": "thirds needs >= 3 windows"}
		}
		third := w / 3
		if errResp := s.setLayoutWindowBounds(wins[0], x, y, third, h); errResp != nil {
			return errResp
		}
		if errResp := s.setLayoutWindowBounds(wins[1], x+third, y, third, h); errResp != nil {
			return errResp
		}
		if errResp := s.setLayoutWindowBounds(wins[2], x+2*third, y, w-2*third, h); errResp != nil {
			return errResp
		}
		applied = 3
	case "quadrants":
		spots := []struct{ cx, cy int }{{0, 0}, {1, 0}, {0, 1}, {1, 1}}
		hw, hh := w/2, h/2
		for i := 0; i < len(wins) && i < 4; i++ {
			if errResp := s.setLayoutWindowBounds(wins[i], x+spots[i].cx*hw, y+spots[i].cy*hh, hw, hh); errResp != nil {
				return errResp
			}
			applied++
		}
	case "grid":
		// pick cols = ceil(sqrt(N)), rows = ceil(N / cols)
		cols := 1
		for cols*cols < len(wins) {
			cols++
		}
		rows := (len(wins) + cols - 1) / cols
		cw := w / cols
		ch := h / rows
		for i, wn := range wins {
			cx := i % cols
			cy := i / cols
			if errResp := s.setLayoutWindowBounds(wn, x+cx*cw, y+cy*ch, cw, ch); errResp != nil {
				return errResp
			}
			applied++
		}
	default:
		return map[string]any{"ok": false, "error": "unknown mode: " + mode + " (try: left, right, top, bottom, halves, thirds, quadrants, grid)"}
	}
	return map[string]any{"ok": true, "mode": mode, "applied": applied}
}

// toolLayoutSnap snaps one window to a screen edge or corner.
// positions: top, bottom, left, right, top-left, top-right,
// bottom-left, bottom-right, centre.
// params: { name, position }
func (s *Service) toolLayoutSnap(params map[string]any) map[string]any {
	name := paramString(params, "name", "")
	if name == "" {
		return map[string]any{"ok": false, "error": nameParamRequired}
	}
	position := core.Lower(paramString(params, "position", ""))
	if position == "" {
		return map[string]any{"ok": false, "error": "position param required"}
	}
	x, y, w, h, errResp := s.activeWorkArea(name)
	if errResp != nil {
		return errResp
	}
	switch position {
	case "left":
		errResp = s.setLayoutWindowBounds(name, x, y, w/2, h)
	case "right":
		errResp = s.setLayoutWindowBounds(name, x+w/2, y, w/2, h)
	case "top":
		errResp = s.setLayoutWindowBounds(name, x, y, w, h/2)
	case "bottom":
		errResp = s.setLayoutWindowBounds(name, x, y+h/2, w, h/2)
	case "top-left", "topleft":
		errResp = s.setLayoutWindowBounds(name, x, y, w/2, h/2)
	case "top-right", "topright":
		errResp = s.setLayoutWindowBounds(name, x+w/2, y, w/2, h/2)
	case "bottom-left", "bottomleft":
		errResp = s.setLayoutWindowBounds(name, x, y+h/2, w/2, h/2)
	case "bottom-right", "bottomright":
		errResp = s.setLayoutWindowBounds(name, x+w/2, y+h/2, w/2, h/2)
	case "centre", "center":
		// Roughly 60% size, centred.
		cw := w * 6 / 10
		ch := h * 6 / 10
		errResp = s.setLayoutWindowBounds(name, x+(w-cw)/2, y+(h-ch)/2, cw, ch)
	default:
		return map[string]any{"ok": false, "error": "unknown position: " + position}
	}
	if errResp != nil {
		return errResp
	}
	return map[string]any{"ok": true, "position": position, "name": name}
}

// toolLayoutStack cascades windows from the screen's top-left
// with constant pixel offsets, sized to a reasonable share of the
// work area. params: { windows?, offsetX?, offsetY?, width?, height? }
func (s *Service) toolLayoutStack(params map[string]any) map[string]any {
	wins, errResp := s.pickWindows(params)
	if errResp != nil {
		return errResp
	}
	if len(wins) == 0 {
		return map[string]any{"ok": false, "error": "no windows to stack"}
	}
	x, y, w, h, errResp := s.activeWorkArea(wins[0])
	if errResp != nil {
		return errResp
	}
	offX := paramInt(params, "offsetX", 30)
	offY := paramInt(params, "offsetY", 30)
	cw := paramInt(params, "width", w*70/100)
	ch := paramInt(params, "height", h*70/100)
	for i, name := range wins {
		if errResp := s.setLayoutWindowBounds(name, x+i*offX, y+i*offY, cw, ch); errResp != nil {
			return errResp
		}
		if errResp := s.focusLayoutWindow(name); errResp != nil {
			return errResp
		}
	}
	return map[string]any{"ok": true, "stacked": len(wins), "offsetX": offX, "offsetY": offY}
}

// toolLayoutWorkflow applies an opinionated preset arrangement
// using lthn-desktop's canonical window names. presets:
//   - "default"   — everything hidden except tray
//   - "coding"    — editor centre, git right rail
//   - "review"    — chat left half, models right half
//   - "ops"       — telemetry top, logs/processes bottom
//   - "single"    — focused single window (param: name=<window>) fullscreen-equivalent
//
// params: { workflow, name? }
func (s *Service) toolLayoutWorkflow(params map[string]any) map[string]any {
	workflow := core.Lower(paramString(params, "workflow", "default"))
	x, y, w, h, errResp := s.activeWorkArea("")
	if errResp != nil {
		return errResp
	}
	switch workflow {
	case "default":
		hidden, errResp := s.hideWorkflowWindows("tray")
		if errResp != nil {
			return errResp
		}
		return map[string]any{"ok": true, "workflow": "default", "hidden": hidden}
	case "coding":
		if _, errResp := s.hideWorkflowWindows("tray", "editor", "git"); errResp != nil {
			return errResp
		}
		s.applyWorkflowWindow("editor", x, y, w*2/3, h, true)
		s.applyWorkflowWindow("git", x+w*2/3, y, w/3, h, true)
		return map[string]any{"ok": true, "workflow": "coding"}
	case "review":
		if _, errResp := s.hideWorkflowWindows("tray", "chat", "models"); errResp != nil {
			return errResp
		}
		s.applyWorkflowWindow("chat", x, y, w/2, h, true)
		s.applyWorkflowWindow("models", x+w/2, y, w/2, h, true)
		return map[string]any{"ok": true, "workflow": "review"}
	case "ops":
		if _, errResp := s.hideWorkflowWindows("tray", "telemetry", "logs", "containers"); errResp != nil {
			return errResp
		}
		s.applyWorkflowWindow("telemetry", x, y, w, h/2, true)
		s.applyWorkflowWindow("logs", x, y+h/2, w/2, h/2, true)
		s.applyWorkflowWindow("containers", x+w/2, y+h/2, w/2, h/2, true)
		return map[string]any{"ok": true, "workflow": "ops"}
	case "single":
		name := paramString(params, "name", "")
		if name == "" {
			return map[string]any{"ok": false, "error": "single workflow needs a name param"}
		}
		if _, errResp := s.hideWorkflowWindows("tray", name); errResp != nil {
			return errResp
		}
		s.applyWorkflowWindow(name, x, y, w, h, true)
		return map[string]any{"ok": true, "workflow": "single", "focused": name}
	default:
		return map[string]any{"ok": false, "error": "unknown workflow: " + workflow + " (try: default, coding, review, ops, single)"}
	}
}

func (s *Service) applyWorkflowWindow(name string, px, py, pw, ph int, show bool) bool {
	if _, errResp := s.coreGUIWindowInfo(name); errResp != nil {
		return false
	}
	if show {
		if errResp := s.setLayoutWindowVisibility(name, true); errResp != nil {
			return false
		}
		if errResp := s.setLayoutWindowBounds(name, px, py, pw, ph); errResp != nil {
			return false
		}
		return true
	}
	return s.setLayoutWindowVisibility(name, false) == nil
}

func (s *Service) hideWorkflowWindows(keep ...string) (int, map[string]any) {
	all, errResp := s.coreGUIWindowList()
	if errResp != nil {
		return 0, errResp
	}
	keepSet := workflowKeepSet(keep...)
	hidden := 0
	for _, win := range all {
		if !keepSet[win.Name] {
			if errResp := s.setLayoutWindowVisibility(win.Name, false); errResp != nil {
				return hidden, errResp
			}
			hidden++
		}
	}
	return hidden, nil
}

func workflowKeepSet(keep ...string) map[string]bool {
	keepSet := map[string]bool{}
	for _, k := range keep {
		keepSet[k] = true
	}
	return keepSet
}

// loadLayout reads + parses a saved layout file.
func (s *Service) loadLayout(name string) (*Layout, map[string]any) {
	rootResult := layoutsRoot()
	if !rootResult.OK {
		return nil, map[string]any{"ok": false, "error": rootResult.Error()}
	}
	root := rootResult.Value.(string)
	path := core.PathJoin(root, name+".json")
	read := core.ReadFile(path)
	if !read.OK {
		return nil, map[string]any{"ok": false, "error": "read " + path + ": " + read.Error()}
	}
	bytes, _ := read.Value.([]byte)
	var layout Layout
	if r := core.JSONUnmarshal(bytes, &layout); !r.OK {
		return nil, map[string]any{"ok": false, "error": "parse " + path + ": " + r.Error()}
	}
	return &layout, nil
}
