// SPDX-Licence-Identifier: EUPL-1.2

// window.go — bridge tools for native window control. All
// dispatched directly against the Wails Window API; zero core/gui
// dependency.

package bridge

import (
	"github.com/wailsapp/wails/v3/pkg/application"
)

// windowOrErr resolves a window by name + casts to WebviewWindow.
func (s *Service) windowOrErr(name string) (*application.WebviewWindow, map[string]any) {
	app := s.app()
	if app == nil {
		return nil, map[string]any{"ok": false, "error": "wails application not initialised yet"}
	}
	w, ok := app.Window.GetByName(name)
	if !ok || w == nil {
		return nil, map[string]any{"ok": false, "error": "window not found: " + name}
	}
	wv, ok := w.(*application.WebviewWindow)
	if !ok {
		return nil, map[string]any{"ok": false, "error": "window is not a WebviewWindow: " + name}
	}
	return wv, nil
}

// windowInfo serialises one window into a JSON-friendly shape.
// Mirrors the WindowState struct used by layout_*, plus extra
// fields (always_on_top, focused, minimised) for richer reads.
func windowInfo(wv *application.WebviewWindow) map[string]any {
	x, y := wv.Position()
	w, h := wv.Size()
	return map[string]any{
		"name":       wv.Name(),
		"x":          x,
		"y":          y,
		"width":      w,
		"height":     h,
		"visible":    wv.IsVisible(),
		"maximised":  wv.IsMaximised(),
		"minimised":  wv.IsMinimised(),
		"fullscreen": wv.IsFullscreen(),
		"focused":    wv.IsFocused(),
	}
}

// toolWindowList enumerates every registered Wails window with
// full state. Richer than webview_windows which only returns names.
func (s *Service) toolWindowList() map[string]any {
	app := s.app()
	if app == nil {
		return map[string]any{"ok": false, "error": "wails application not initialised yet"}
	}
	all := app.Window.GetAll()
	out := make([]map[string]any, 0, len(all))
	for _, w := range all {
		wv, ok := w.(*application.WebviewWindow)
		if !ok || wv == nil {
			continue
		}
		out = append(out, windowInfo(wv))
	}
	return map[string]any{"ok": true, "value": out, "count": len(out)}
}

// toolWindowGet returns one window's full state. params: { name }
func (s *Service) toolWindowGet(params map[string]any) map[string]any {
	wv, errResp := s.windowOrErr(paramString(params, "name", ""))
	if errResp != nil {
		return errResp
	}
	return map[string]any{"ok": true, "value": windowInfo(wv)}
}

// toolWindowPosition moves a window. params: { name, x, y }
func (s *Service) toolWindowPosition(params map[string]any) map[string]any {
	wv, errResp := s.windowOrErr(paramString(params, "name", ""))
	if errResp != nil {
		return errResp
	}
	x := paramInt(params, "x", 0)
	y := paramInt(params, "y", 0)
	wv.SetPosition(x, y)
	return map[string]any{"ok": true, "name": wv.Name(), "x": x, "y": y}
}

// toolWindowSize resizes a window. params: { name, width, height }
func (s *Service) toolWindowSize(params map[string]any) map[string]any {
	wv, errResp := s.windowOrErr(paramString(params, "name", ""))
	if errResp != nil {
		return errResp
	}
	w := paramInt(params, "width", 0)
	h := paramInt(params, "height", 0)
	if w <= 0 || h <= 0 {
		return map[string]any{"ok": false, "error": "width + height must be > 0"}
	}
	wv.SetSize(w, h)
	return map[string]any{"ok": true, "name": wv.Name(), "width": w, "height": h}
}

// toolWindowBounds sets position + size in one call. params:
// { name, x, y, width, height }
func (s *Service) toolWindowBounds(params map[string]any) map[string]any {
	wv, errResp := s.windowOrErr(paramString(params, "name", ""))
	if errResp != nil {
		return errResp
	}
	x := paramInt(params, "x", 0)
	y := paramInt(params, "y", 0)
	w := paramInt(params, "width", 0)
	h := paramInt(params, "height", 0)
	if w <= 0 || h <= 0 {
		return map[string]any{"ok": false, "error": "width + height must be > 0"}
	}
	wv.SetPosition(x, y)
	wv.SetSize(w, h)
	return map[string]any{"ok": true, "name": wv.Name(), "x": x, "y": y, "width": w, "height": h}
}

// toolWindowMaximise + family — each toggles a Wails Window state.
func (s *Service) toolWindowMaximise(params map[string]any) map[string]any {
	wv, errResp := s.windowOrErr(paramString(params, "name", ""))
	if errResp != nil {
		return errResp
	}
	wv.Maximise()
	return map[string]any{"ok": true, "action": "maximise", "name": wv.Name()}
}

func (s *Service) toolWindowMinimise(params map[string]any) map[string]any {
	wv, errResp := s.windowOrErr(paramString(params, "name", ""))
	if errResp != nil {
		return errResp
	}
	wv.Minimise()
	return map[string]any{"ok": true, "action": "minimise", "name": wv.Name()}
}

func (s *Service) toolWindowRestore(params map[string]any) map[string]any {
	wv, errResp := s.windowOrErr(paramString(params, "name", ""))
	if errResp != nil {
		return errResp
	}
	if wv.IsMinimised() {
		wv.UnMinimise()
	} else if wv.IsMaximised() {
		wv.UnMaximise()
	} else if wv.IsFullscreen() {
		wv.UnFullscreen()
	} else {
		wv.Restore()
	}
	return map[string]any{"ok": true, "action": "restore", "name": wv.Name()}
}

func (s *Service) toolWindowFocus(params map[string]any) map[string]any {
	wv, errResp := s.windowOrErr(paramString(params, "name", ""))
	if errResp != nil {
		return errResp
	}
	wv.Focus()
	return map[string]any{"ok": true, "action": "focus", "name": wv.Name()}
}

// toolWindowFocused returns the currently focused window.
func (s *Service) toolWindowFocused() map[string]any {
	app := s.app()
	if app == nil {
		return map[string]any{"ok": false, "error": "wails application not initialised yet"}
	}
	for _, w := range app.Window.GetAll() {
		wv, ok := w.(*application.WebviewWindow)
		if !ok || wv == nil {
			continue
		}
		if wv.IsFocused() {
			return map[string]any{"ok": true, "value": windowInfo(wv)}
		}
	}
	return map[string]any{"ok": true, "value": nil}
}

// toolWindowVisibility sets show/hide. params: { name, visible }
func (s *Service) toolWindowVisibility(params map[string]any) map[string]any {
	wv, errResp := s.windowOrErr(paramString(params, "name", ""))
	if errResp != nil {
		return errResp
	}
	visible := paramBool(params, "visible", true)
	if visible {
		wv.Show()
	} else {
		wv.Hide()
	}
	return map[string]any{"ok": true, "visible": visible, "name": wv.Name()}
}

// toolWindowAlwaysOnTop pins/unpins. params: { name, enabled }
func (s *Service) toolWindowAlwaysOnTop(params map[string]any) map[string]any {
	wv, errResp := s.windowOrErr(paramString(params, "name", ""))
	if errResp != nil {
		return errResp
	}
	on := paramBool(params, "enabled", true)
	wv.SetAlwaysOnTop(on)
	return map[string]any{"ok": true, "always_on_top": on, "name": wv.Name()}
}

// toolWindowSetTitle changes the title. params: { name, title }
func (s *Service) toolWindowSetTitle(params map[string]any) map[string]any {
	wv, errResp := s.windowOrErr(paramString(params, "name", ""))
	if errResp != nil {
		return errResp
	}
	wv.SetTitle(paramString(params, "title", ""))
	return map[string]any{"ok": true, "name": wv.Name()}
}

// toolWindowGetTitle reads the title back. params: { name }
// (uses ExecJS to get document.title since Wails doesn't expose
// a Window.Title() getter — the wails-managed title and the
// document title can drift.)
func (s *Service) toolWindowGetTitle(params map[string]any) map[string]any {
	wv, errResp := s.windowOrErr(paramString(params, "name", ""))
	if errResp != nil {
		return errResp
	}
	// Wails3 alpha.91 doesn't expose a Title() getter on the Go
	// side; return the window name + a hint to use webview_title
	// for the live document.title.
	return map[string]any{"ok": true, "name": wv.Name(), "hint": "use webview_title to read document.title"}
}

// toolWindowFullscreen toggles fullscreen. params: { name, enabled? }
func (s *Service) toolWindowFullscreen(params map[string]any) map[string]any {
	wv, errResp := s.windowOrErr(paramString(params, "name", ""))
	if errResp != nil {
		return errResp
	}
	enabled := paramBool(params, "enabled", true)
	if enabled {
		wv.Fullscreen()
	} else {
		wv.UnFullscreen()
	}
	return map[string]any{"ok": true, "fullscreen": enabled, "name": wv.Name()}
}

// toolWindowClose closes a window. params: { name }
func (s *Service) toolWindowClose(params map[string]any) map[string]any {
	wv, errResp := s.windowOrErr(paramString(params, "name", ""))
	if errResp != nil {
		return errResp
	}
	wv.Close()
	return map[string]any{"ok": true, "closed": wv.Name()}
}

// toolWindowCenter centres a window on its current screen.
// params: { name }
func (s *Service) toolWindowCenter(params map[string]any) map[string]any {
	wv, errResp := s.windowOrErr(paramString(params, "name", ""))
	if errResp != nil {
		return errResp
	}
	wv.Center()
	return map[string]any{"ok": true, "centered": wv.Name()}
}

// toolWindowBackgroundColour sets the window background (with
// alpha). params: { name, r, g, b, a }
func (s *Service) toolWindowBackgroundColour(params map[string]any) map[string]any {
	wv, errResp := s.windowOrErr(paramString(params, "name", ""))
	if errResp != nil {
		return errResp
	}
	r := uint8(paramInt(params, "r", 0))
	g := uint8(paramInt(params, "g", 0))
	b := uint8(paramInt(params, "b", 0))
	a := uint8(paramInt(params, "a", 255))
	wv.SetBackgroundColour(application.NewRGBA(r, g, b, a))
	return map[string]any{"ok": true, "rgba": []uint8{r, g, b, a}, "name": wv.Name()}
}
