// SPDX-Licence-Identifier: EUPL-1.2

// window.go — bridge tools for native window control. All window
// operations dispatch through core/gui actions and queries.

package bridge

import (

	core "dappco.re/go"
	guiwindow "dappco.re/go/gui/pkg/window"
)

func (s *Service) coreGUIWindowList() ([]guiwindow.WindowInfo, map[string]any) {
	r := s.Core().QUERY(guiwindow.QueryWindowList{})
	if !r.OK {
		return nil, map[string]any{"ok": false, "error": r.Error()}
	}
	windows, ok := r.Value.([]guiwindow.WindowInfo)
	if !ok {
		return nil, map[string]any{"ok": false, "error": "core/gui window list unavailable"}
	}
	return windows, nil
}

func (s *Service) coreGUIWindowInfo(name string) (*guiwindow.WindowInfo, map[string]any) {
	if name == "" {
		return nil, map[string]any{"ok": false, "error": nameParamRequired}
	}
	r := s.Core().QUERY(guiwindow.QueryWindowByName{Name: name})
	if !r.OK {
		return nil, map[string]any{"ok": false, "error": r.Error()}
	}
	info, _ := r.Value.(*guiwindow.WindowInfo)
	if info == nil {
		return nil, map[string]any{"ok": false, "error": "window not found: " + name}
	}
	return info, nil
}

func (s *Service) runCoreGUIWindowTask(action string, task any) map[string]any {
	r := s.Core().Action(action).Run(core.Background(), core.NewOptions(
		core.Option{Key: "task", Value: task},
	))
	if !r.OK {
		return map[string]any{"ok": false, "error": r.Error()}
	}
	return nil
}

// windowInfo serialises one window into a JSON-friendly shape.
// Mirrors the WindowState struct used by layout_*, plus extra
// fields (always_on_top, focused, minimised) for richer reads.
func windowInfo(info *guiwindow.WindowInfo) map[string]any {
	if info == nil {
		return nil
	}
	return map[string]any{
		"name":          info.Name,
		"title":         info.Title,
		"x":             info.X,
		"y":             info.Y,
		"width":         info.Width,
		"height":        info.Height,
		"opacity":       info.Opacity,
		"visible":       info.Visible,
		"maximised":     info.Maximized,
		"maximized":     info.Maximized,
		"minimised":     info.Minimised,
		"fullscreen":    info.Fullscreen,
		"focused":       info.Focused,
		"always_on_top": info.AlwaysOnTop,
	}
}

// toolWindowList enumerates every registered core/gui window with
// full state. Richer than webview_windows which only returns names.
func (s *Service) toolWindowList() map[string]any {
	all, errResp := s.coreGUIWindowList()
	if errResp != nil {
		return errResp
	}
	out := make([]map[string]any, 0, len(all))
	for i := range all {
		out = append(out, windowInfo(&all[i]))
	}
	return map[string]any{"ok": true, "value": out, "count": len(out)}
}

// toolWindowGet returns one window's full state. params: { name }
func (s *Service) toolWindowGet(params map[string]any) map[string]any {
	info, errResp := s.coreGUIWindowInfo(paramString(params, "name", ""))
	if errResp != nil {
		return errResp
	}
	return map[string]any{"ok": true, "value": windowInfo(info)}
}

// toolWindowPosition moves a window. params: { name, x, y }
func (s *Service) toolWindowPosition(params map[string]any) map[string]any {
	name := paramString(params, "name", "")
	x := paramInt(params, "x", 0)
	y := paramInt(params, "y", 0)
	if errResp := s.runCoreGUIWindowTask("window.set_position", guiwindow.TaskSetPosition{Name: name, X: x, Y: y}); errResp != nil {
		return errResp
	}
	return map[string]any{"ok": true, "name": name, "x": x, "y": y}
}

// toolWindowSize resizes a window. params: { name, width, height }
func (s *Service) toolWindowSize(params map[string]any) map[string]any {
	name := paramString(params, "name", "")
	w := paramInt(params, "width", 0)
	h := paramInt(params, "height", 0)
	if w <= 0 || h <= 0 {
		return map[string]any{"ok": false, "error": "width + height must be > 0"}
	}
	if errResp := s.runCoreGUIWindowTask("window.set_size", guiwindow.TaskSetSize{Name: name, Width: w, Height: h}); errResp != nil {
		return errResp
	}
	return map[string]any{"ok": true, "name": name, "width": w, "height": h}
}

// toolWindowBounds sets position + size in one call. params:
// { name, x, y, width, height }
func (s *Service) toolWindowBounds(params map[string]any) map[string]any {
	name := paramString(params, "name", "")
	x := paramInt(params, "x", 0)
	y := paramInt(params, "y", 0)
	w := paramInt(params, "width", 0)
	h := paramInt(params, "height", 0)
	if w <= 0 || h <= 0 {
		return map[string]any{"ok": false, "error": "width + height must be > 0"}
	}
	if errResp := s.runCoreGUIWindowTask("window.set_bounds", guiwindow.TaskSetBounds{Name: name, X: x, Y: y, Width: w, Height: h}); errResp != nil {
		return errResp
	}
	return map[string]any{"ok": true, "name": name, "x": x, "y": y, "width": w, "height": h}
}

// toolWindowMaximise + family each dispatches a core/gui window task.
func (s *Service) toolWindowMaximise(params map[string]any) map[string]any {
	name := paramString(params, "name", "")
	if errResp := s.runCoreGUIWindowTask("window.maximise", guiwindow.TaskMaximise{Name: name}); errResp != nil {
		return errResp
	}
	return map[string]any{"ok": true, "action": "maximise", "name": name}
}

func (s *Service) toolWindowMinimise(params map[string]any) map[string]any {
	name := paramString(params, "name", "")
	if errResp := s.runCoreGUIWindowTask("window.minimise", guiwindow.TaskMinimise{Name: name}); errResp != nil {
		return errResp
	}
	return map[string]any{"ok": true, "action": "minimise", "name": name}
}

func (s *Service) toolWindowRestore(params map[string]any) map[string]any {
	name := paramString(params, "name", "")
	if errResp := s.runCoreGUIWindowTask("window.restore", guiwindow.TaskRestore{Name: name}); errResp != nil {
		return errResp
	}
	return map[string]any{"ok": true, "action": "restore", "name": name}
}

func (s *Service) toolWindowFocus(params map[string]any) map[string]any {
	name := paramString(params, "name", "")
	if errResp := s.runCoreGUIWindowTask("window.focus", guiwindow.TaskFocus{Name: name}); errResp != nil {
		return errResp
	}
	return map[string]any{"ok": true, "action": "focus", "name": name}
}

// toolWindowFocused returns the currently focused window.
func (s *Service) toolWindowFocused() map[string]any {
	all, errResp := s.coreGUIWindowList()
	if errResp != nil {
		return errResp
	}
	for i := range all {
		if all[i].Focused {
			return map[string]any{"ok": true, "value": windowInfo(&all[i])}
		}
	}
	return map[string]any{"ok": true, "value": nil}
}

// toolWindowVisibility sets show/hide. params: { name, visible }
func (s *Service) toolWindowVisibility(params map[string]any) map[string]any {
	name := paramString(params, "name", "")
	visible := paramBool(params, "visible", true)
	if errResp := s.runCoreGUIWindowTask("window.set_visibility", guiwindow.TaskSetVisibility{Name: name, Visible: visible}); errResp != nil {
		return errResp
	}
	return map[string]any{"ok": true, "visible": visible, "name": name}
}

// toolWindowAlwaysOnTop pins/unpins. params: { name, enabled }
func (s *Service) toolWindowAlwaysOnTop(params map[string]any) map[string]any {
	name := paramString(params, "name", "")
	on := paramBool(params, "enabled", true)
	if errResp := s.runCoreGUIWindowTask("window.set_always_on_top", guiwindow.TaskSetAlwaysOnTop{Name: name, AlwaysOnTop: on}); errResp != nil {
		return errResp
	}
	return map[string]any{"ok": true, "always_on_top": on, "name": name}
}

// toolWindowSetTitle changes the title. params: { name, title }
func (s *Service) toolWindowSetTitle(params map[string]any) map[string]any {
	name := paramString(params, "name", "")
	title := paramString(params, "title", "")
	if errResp := s.runCoreGUIWindowTask("window.set_title", guiwindow.TaskSetTitle{Name: name, Title: title}); errResp != nil {
		return errResp
	}
	return map[string]any{"ok": true, "name": name}
}

// toolWindowGetTitle reads the title back. params: { name }
func (s *Service) toolWindowGetTitle(params map[string]any) map[string]any {
	info, errResp := s.coreGUIWindowInfo(paramString(params, "name", ""))
	if errResp != nil {
		return errResp
	}
	return map[string]any{"ok": true, "name": info.Name, "title": info.Title}
}

// toolWindowFullscreen toggles fullscreen. params: { name, enabled? }
func (s *Service) toolWindowFullscreen(params map[string]any) map[string]any {
	name := paramString(params, "name", "")
	enabled := paramBool(params, "enabled", true)
	if errResp := s.runCoreGUIWindowTask("window.fullscreen", guiwindow.TaskFullscreen{Name: name, Fullscreen: enabled}); errResp != nil {
		return errResp
	}
	return map[string]any{"ok": true, "fullscreen": enabled, "name": name}
}

// toolWindowClose closes a window. params: { name }
func (s *Service) toolWindowClose(params map[string]any) map[string]any {
	name := paramString(params, "name", "")
	if errResp := s.runCoreGUIWindowTask("window.close", guiwindow.TaskCloseWindow{Name: name}); errResp != nil {
		return errResp
	}
	return map[string]any{"ok": true, "closed": name}
}

// toolWindowCenter centres a window on its current screen.
// params: { name }
func (s *Service) toolWindowCenter(params map[string]any) map[string]any {
	name := paramString(params, "name", "")
	if errResp := s.runCoreGUIWindowTask("window.snap_window", guiwindow.TaskSnapWindow{Name: name, Position: "center"}); errResp != nil {
		return errResp
	}
	return map[string]any{"ok": true, "centered": name}
}

// toolWindowBackgroundColour sets the window background (with
// alpha). params: { name, r, g, b, a }
func (s *Service) toolWindowBackgroundColour(params map[string]any) map[string]any {
	name := paramString(params, "name", "")
	r := uint8(paramInt(params, "r", 0))
	g := uint8(paramInt(params, "g", 0))
	b := uint8(paramInt(params, "b", 0))
	a := uint8(paramInt(params, "a", 255))
	if errResp := s.runCoreGUIWindowTask("window.set_background_colour", guiwindow.TaskSetBackgroundColour{Name: name, Red: r, Green: g, Blue: b, Alpha: a}); errResp != nil {
		return errResp
	}
	return map[string]any{"ok": true, "rgba": []uint8{r, g, b, a}, "name": name}
}
