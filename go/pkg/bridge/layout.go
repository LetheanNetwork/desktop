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
	Name         string `json:"name"`
	X            int    `json:"x"`
	Y            int    `json:"y"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	Visible      bool   `json:"visible"`
	Maximised    bool   `json:"maximised,omitempty"`
	Fullscreen   bool   `json:"fullscreen,omitempty"`
	AlwaysOnTop  bool   `json:"always_on_top,omitempty"`
}

// Layout is the serialised shape of a saved layout file.
type Layout struct {
	Name    string        `json:"name"`
	SavedAt time.Time     `json:"saved_at"`
	Windows []WindowState `json:"windows"`
}

// layoutsRoot returns $HOME/Lethean/conf/layouts. Mirrors the
// pkg/plugin install-root pattern — visible under ~/Lethean/.
func layoutsRoot() (string, core.Result) {
	home := core.UserHomeDir()
	if !home.OK {
		return "", core.Fail(core.E("bridge.layoutsRoot", "home dir unavailable", nil))
	}
	homeDir, _ := home.Value.(string)
	if homeDir == "" {
		return "", core.Fail(core.E("bridge.layoutsRoot", "home dir empty", nil))
	}
	return core.PathJoin(homeDir, "Lethean", "conf", "layouts"), core.Ok(nil)
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
	root, res := layoutsRoot()
	if !res.OK {
		return map[string]any{"ok": false, "error": res.Error()}
	}
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
	root, res := layoutsRoot()
	if !res.OK {
		return map[string]any{"ok": false, "error": res.Error()}
	}
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
	root, res := layoutsRoot()
	if !res.OK {
		return map[string]any{"ok": false, "error": res.Error()}
	}
	path := core.PathJoin(root, name+".json")
	if r := core.RemoveAll(path); !r.OK {
		return map[string]any{"ok": false, "error": "remove " + path + ": " + r.Error()}
	}
	return map[string]any{"ok": true, "name": name, "removed": path}
}

// loadLayout reads + parses a saved layout file.
func (s *Service) loadLayout(name string) (*Layout, map[string]any) {
	root, res := layoutsRoot()
	if !res.OK {
		return nil, map[string]any{"ok": false, "error": res.Error()}
	}
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
