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
	"time"

	core "dappco.re/go"
	"github.com/wailsapp/wails/v3/pkg/application"
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
	SavedAt time.Time     `json:"saved_at"`
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

// captureLayout snapshots every Wails-registered window into a Layout.
func (s *Service) captureLayout(name string) (*Layout, map[string]any) {
	app := s.app()
	if app == nil {
		return nil, map[string]any{"ok": false, "error": "wails application not initialised yet"}
	}
	all := app.Window.GetAll()
	out := make([]WindowState, 0, len(all))
	for _, w := range all {
		wv, ok := w.(*application.WebviewWindow)
		if !ok || wv == nil {
			continue
		}
		x, y := wv.Position()
		ww, hh := wv.Size()
		out = append(out, WindowState{
			Name:       wv.Name(),
			X:          x,
			Y:          y,
			Width:      ww,
			Height:     hh,
			Visible:    wv.IsVisible(),
			Maximised:  wv.IsMaximised(),
			Fullscreen: wv.IsFullscreen(),
		})
	}
	return &Layout{
		Name:    name,
		SavedAt: time.Now(),
		Windows: out,
	}, nil
}

// applyLayout reads a Layout and pushes each WindowState onto the
// matching Wails window. Windows in the layout that no longer exist
// are skipped; windows present today but absent from the layout are
// left alone.
func (s *Service) applyLayout(layout *Layout) map[string]any {
	app := s.app()
	if app == nil {
		return map[string]any{"ok": false, "error": "wails application not initialised yet"}
	}
	applied := 0
	skipped := 0
	for _, ws := range layout.Windows {
		w, ok := app.Window.GetByName(ws.Name)
		if !ok || w == nil {
			skipped++
			continue
		}
		wv, ok := w.(*application.WebviewWindow)
		if !ok {
			skipped++
			continue
		}
		// Order matters: unmaximise + unfullscreen FIRST so SetPosition + SetSize land in
		// the right frame; the saved state then re-applies the chrome flags.
		if wv.IsMaximised() {
			wv.UnMaximise()
		}
		if wv.IsFullscreen() {
			wv.UnFullscreen()
		}
		wv.SetPosition(ws.X, ws.Y)
		wv.SetSize(ws.Width, ws.Height)
		if ws.Visible {
			wv.Show()
		} else {
			wv.Hide()
		}
		if ws.Maximised {
			wv.Maximise()
		}
		if ws.Fullscreen {
			wv.Fullscreen()
		}
		if ws.AlwaysOnTop {
			wv.SetAlwaysOnTop(true)
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

// toolLayoutSave snapshots current window state to disk under the
// given name.
func (s *Service) toolLayoutSave(params map[string]any) map[string]any {
	name := core.Trim(paramString(params, "name", ""))
	if name == "" {
		return map[string]any{"ok": false, "error": "name param required"}
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
		return map[string]any{"ok": false, "error": "name param required"}
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
		return map[string]any{"ok": false, "error": "name param required"}
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
		return map[string]any{"ok": false, "error": "name param required"}
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
	app := s.app()
	if app == nil {
		return 0, 0, 0, 0, map[string]any{"ok": false, "error": "wails application not initialised yet"}
	}
	if name != "" {
		if w, ok := app.Window.GetByName(name); ok && w != nil {
			if wv, ok := w.(*application.WebviewWindow); ok {
				if sc, err := wv.GetScreen(); err == nil && sc != nil {
					return sc.WorkArea.X, sc.WorkArea.Y, sc.WorkArea.Width, sc.WorkArea.Height, nil
				}
			}
		}
	}
	if app.Screen == nil {
		return 0, 0, 0, 0, map[string]any{"ok": false, "error": "screen manager unavailable"}
	}
	sc := app.Screen.GetPrimary()
	if sc == nil {
		return 0, 0, 0, 0, map[string]any{"ok": false, "error": "no primary screen"}
	}
	return sc.WorkArea.X, sc.WorkArea.Y, sc.WorkArea.Width, sc.WorkArea.Height, nil
}

// pickWindows resolves a "windows" param (string array of names)
// into a list of WebviewWindows. Empty/missing → every visible
// window currently registered. Caller decides what to do with the
// returned slice.
func (s *Service) pickWindows(params map[string]any) ([]*application.WebviewWindow, map[string]any) {
	app := s.app()
	if app == nil {
		return nil, map[string]any{"ok": false, "error": "wails application not initialised yet"}
	}
	names := stringSliceParam(params, "windows")
	out := []*application.WebviewWindow{}
	if len(names) == 0 {
		// Default: every visible window.
		for _, w := range app.Window.GetAll() {
			wv, ok := w.(*application.WebviewWindow)
			if !ok || wv == nil {
				continue
			}
			if wv.IsVisible() {
				out = append(out, wv)
			}
		}
		return out, nil
	}
	for _, n := range names {
		w, ok := app.Window.GetByName(n)
		if !ok || w == nil {
			continue
		}
		if wv, ok := w.(*application.WebviewWindow); ok {
			out = append(out, wv)
		}
	}
	return out, nil
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
	x, y, w, h, errResp := s.activeWorkArea(wins[0].Name())
	if errResp != nil {
		return errResp
	}
	applied := 0
	switch mode {
	case "left":
		wins[0].SetPosition(x, y)
		wins[0].SetSize(w/2, h)
		applied = 1
	case "right":
		wins[0].SetPosition(x+w/2, y)
		wins[0].SetSize(w/2, h)
		applied = 1
	case "top":
		wins[0].SetPosition(x, y)
		wins[0].SetSize(w, h/2)
		applied = 1
	case "bottom":
		wins[0].SetPosition(x, y+h/2)
		wins[0].SetSize(w, h/2)
		applied = 1
	case "halves":
		if len(wins) < 2 {
			return map[string]any{"ok": false, "error": "halves needs >= 2 windows"}
		}
		wins[0].SetPosition(x, y)
		wins[0].SetSize(w/2, h)
		wins[1].SetPosition(x+w/2, y)
		wins[1].SetSize(w/2, h)
		applied = 2
	case "thirds":
		if len(wins) < 3 {
			return map[string]any{"ok": false, "error": "thirds needs >= 3 windows"}
		}
		third := w / 3
		wins[0].SetPosition(x, y)
		wins[0].SetSize(third, h)
		wins[1].SetPosition(x+third, y)
		wins[1].SetSize(third, h)
		wins[2].SetPosition(x+2*third, y)
		wins[2].SetSize(w-2*third, h)
		applied = 3
	case "quadrants":
		spots := []struct{ cx, cy int }{{0, 0}, {1, 0}, {0, 1}, {1, 1}}
		hw, hh := w/2, h/2
		for i := 0; i < len(wins) && i < 4; i++ {
			wins[i].SetPosition(x+spots[i].cx*hw, y+spots[i].cy*hh)
			wins[i].SetSize(hw, hh)
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
			wn.SetPosition(x+cx*cw, y+cy*ch)
			wn.SetSize(cw, ch)
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
	wv, errResp := s.windowOrErr(paramString(params, "name", ""))
	if errResp != nil {
		return errResp
	}
	position := core.Lower(paramString(params, "position", ""))
	if position == "" {
		return map[string]any{"ok": false, "error": "position param required"}
	}
	x, y, w, h, errResp := s.activeWorkArea(wv.Name())
	if errResp != nil {
		return errResp
	}
	switch position {
	case "left":
		wv.SetPosition(x, y)
		wv.SetSize(w/2, h)
	case "right":
		wv.SetPosition(x+w/2, y)
		wv.SetSize(w/2, h)
	case "top":
		wv.SetPosition(x, y)
		wv.SetSize(w, h/2)
	case "bottom":
		wv.SetPosition(x, y+h/2)
		wv.SetSize(w, h/2)
	case "top-left", "topleft":
		wv.SetPosition(x, y)
		wv.SetSize(w/2, h/2)
	case "top-right", "topright":
		wv.SetPosition(x+w/2, y)
		wv.SetSize(w/2, h/2)
	case "bottom-left", "bottomleft":
		wv.SetPosition(x, y+h/2)
		wv.SetSize(w/2, h/2)
	case "bottom-right", "bottomright":
		wv.SetPosition(x+w/2, y+h/2)
		wv.SetSize(w/2, h/2)
	case "centre", "center":
		// Roughly 60% size, centred.
		cw := w * 6 / 10
		ch := h * 6 / 10
		wv.SetPosition(x+(w-cw)/2, y+(h-ch)/2)
		wv.SetSize(cw, ch)
	default:
		return map[string]any{"ok": false, "error": "unknown position: " + position}
	}
	return map[string]any{"ok": true, "position": position, "name": wv.Name()}
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
	x, y, w, h, errResp := s.activeWorkArea(wins[0].Name())
	if errResp != nil {
		return errResp
	}
	offX := paramInt(params, "offsetX", 30)
	offY := paramInt(params, "offsetY", 30)
	cw := paramInt(params, "width", w*70/100)
	ch := paramInt(params, "height", h*70/100)
	for i, wv := range wins {
		wv.SetPosition(x+i*offX, y+i*offY)
		wv.SetSize(cw, ch)
		wv.Focus() // bring each to front in cascade order
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
	app := s.app()
	if app == nil {
		return map[string]any{"ok": false, "error": "wails application not initialised yet"}
	}
	x, y, w, h, errResp := s.activeWorkArea("")
	if errResp != nil {
		return errResp
	}
	apply := func(name string, px, py, pw, ph int, show bool) bool {
		win, ok := app.Window.GetByName(name)
		if !ok || win == nil {
			return false
		}
		wv, ok := win.(*application.WebviewWindow)
		if !ok {
			return false
		}
		if show {
			wv.Show()
			wv.SetPosition(px, py)
			wv.SetSize(pw, ph)
		} else {
			wv.Hide()
		}
		return true
	}
	hideAllExcept := func(keep ...string) int {
		keepSet := map[string]bool{}
		for _, k := range keep {
			keepSet[k] = true
		}
		hidden := 0
		for _, win := range app.Window.GetAll() {
			wv, ok := win.(*application.WebviewWindow)
			if !ok || wv == nil {
				continue
			}
			if keepSet[wv.Name()] {
				continue
			}
			if wv.IsVisible() {
				wv.Hide()
				hidden++
			}
		}
		return hidden
	}
	switch workflow {
	case "default":
		hidden := hideAllExcept("tray")
		return map[string]any{"ok": true, "workflow": "default", "hidden": hidden}
	case "coding":
		hideAllExcept("tray", "editor", "git")
		apply("editor", x, y, w*2/3, h, true)
		apply("git", x+w*2/3, y, w/3, h, true)
		return map[string]any{"ok": true, "workflow": "coding"}
	case "review":
		hideAllExcept("tray", "chat", "models")
		apply("chat", x, y, w/2, h, true)
		apply("models", x+w/2, y, w/2, h, true)
		return map[string]any{"ok": true, "workflow": "review"}
	case "ops":
		hideAllExcept("tray", "telemetry", "logs", "containers")
		apply("telemetry", x, y, w, h/2, true)
		apply("logs", x, y+h/2, w/2, h/2, true)
		apply("containers", x+w/2, y+h/2, w/2, h/2, true)
		return map[string]any{"ok": true, "workflow": "ops"}
	case "single":
		name := paramString(params, "name", "")
		if name == "" {
			return map[string]any{"ok": false, "error": "single workflow needs a name param"}
		}
		hideAllExcept("tray", name)
		apply(name, x, y, w, h, true)
		return map[string]any{"ok": true, "workflow": "single", "focused": name}
	default:
		return map[string]any{"ok": false, "error": "unknown workflow: " + workflow + " (try: default, coding, review, ops, single)"}
	}
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
