// SPDX-Licence-Identifier: EUPL-1.2

// screen.go — bridge tools for monitor/display info backed by
// core/gui screen queries.

package bridge

import (
	guiscreen "dappco.re/go/gui/pkg/screen"
	guiwindow "dappco.re/go/gui/pkg/window"
)

// screenInfo serialises a core/gui screen.
func screenInfo(sc *guiscreen.Screen) map[string]any {
	if sc == nil {
		return nil
	}
	return map[string]any{
		"id":           sc.ID,
		"name":         sc.Name,
		"scale_factor": sc.ScaleFactor,
		"x":            sc.Bounds.X,
		"y":            sc.Bounds.Y,
		"size":         map[string]int{"width": sc.Size.Width, "height": sc.Size.Height},
		"bounds":       map[string]int{"x": sc.Bounds.X, "y": sc.Bounds.Y, "width": sc.Bounds.Width, "height": sc.Bounds.Height},
		"work_area":    map[string]int{"x": sc.WorkArea.X, "y": sc.WorkArea.Y, "width": sc.WorkArea.Width, "height": sc.WorkArea.Height},
		"is_primary":   sc.IsPrimary,
		"rotation":     sc.Rotation,
	}
}

// toolScreenList returns every connected display.
func (s *Service) toolScreenList() map[string]any {
	r := s.Core().QUERY(guiscreen.QueryAll{})
	if !r.OK {
		return map[string]any{"ok": false, "error": r.Error()}
	}
	screens, ok := r.Value.([]guiscreen.Screen)
	if !ok {
		return map[string]any{"ok": false, "error": screenManagerUnavailable}
	}
	out := make([]map[string]any, 0, len(screens))
	for i := range screens {
		out = append(out, screenInfo(&screens[i]))
	}
	return map[string]any{"ok": true, "value": out, "count": len(out)}
}

// toolScreenPrimary returns the primary display.
func (s *Service) toolScreenPrimary() map[string]any {
	r := s.Core().QUERY(guiscreen.QueryPrimary{})
	if !r.OK {
		return map[string]any{"ok": false, "error": r.Error()}
	}
	sc, _ := r.Value.(*guiscreen.Screen)
	if sc == nil {
		return map[string]any{"ok": false, "error": "no primary screen"}
	}
	return map[string]any{"ok": true, "value": screenInfo(sc)}
}

// toolScreenGet returns a specific screen by id. params: { id }
func (s *Service) toolScreenGet(params map[string]any) map[string]any {
	id := paramString(params, "id", "")
	if id == "" {
		return map[string]any{"ok": false, "error": idParamRequired}
	}
	r := s.Core().QUERY(guiscreen.QueryByID{ID: id})
	if !r.OK {
		return map[string]any{"ok": false, "error": r.Error()}
	}
	sc, _ := r.Value.(*guiscreen.Screen)
	if sc == nil {
		return map[string]any{"ok": false, "error": "screen not found: " + id}
	}
	return map[string]any{"ok": true, "value": screenInfo(sc)}
}

// toolScreenAtPoint returns the screen containing (x, y). params: { x, y }
func (s *Service) toolScreenAtPoint(params map[string]any) map[string]any {
	x := paramInt(params, "x", 0)
	y := paramInt(params, "y", 0)
	r := s.Core().QUERY(guiscreen.QueryAtPoint{X: x, Y: y})
	if !r.OK {
		return map[string]any{"ok": false, "error": r.Error()}
	}
	sc, _ := r.Value.(*guiscreen.Screen)
	if sc == nil {
		return map[string]any{"ok": false, "error": "no screen contains point"}
	}
	return map[string]any{"ok": true, "value": screenInfo(sc)}
}

// toolScreenForWindow returns the screen a named window is on.
// params: { name }
func (s *Service) toolScreenForWindow(params map[string]any) map[string]any {
	name := paramString(params, "name", "")
	if name == "" {
		return map[string]any{"ok": false, "error": nameParamRequired}
	}
	r := s.Core().QUERY(guiwindow.QueryWindowByName{Name: name})
	if !r.OK {
		return map[string]any{"ok": false, "error": r.Error()}
	}
	info, _ := r.Value.(*guiwindow.WindowInfo)
	if info == nil {
		return map[string]any{"ok": false, "error": "window not found: " + name}
	}
	r = s.Core().QUERY(guiscreen.QueryAtPoint{
		X: info.X + info.Width/2,
		Y: info.Y + info.Height/2,
	})
	if !r.OK {
		return map[string]any{"ok": false, "error": r.Error()}
	}
	sc, _ := r.Value.(*guiscreen.Screen)
	if sc == nil {
		return map[string]any{"ok": false, "error": "no screen contains window: " + name}
	}
	return map[string]any{"ok": true, "value": screenInfo(sc)}
}

// toolScreenWorkAreas returns the work_area Rect for every screen
// (usable space excluding dock/menubar).
func (s *Service) toolScreenWorkAreas() map[string]any {
	r := s.Core().QUERY(guiscreen.QueryAll{})
	if !r.OK {
		return map[string]any{"ok": false, "error": r.Error()}
	}
	screens, ok := r.Value.([]guiscreen.Screen)
	if !ok {
		return map[string]any{"ok": false, "error": screenManagerUnavailable}
	}
	out := make([]map[string]any, 0, len(screens))
	for i := range screens {
		sc := screens[i]
		out = append(out, map[string]any{
			"id":        sc.ID,
			"name":      sc.Name,
			"work_area": map[string]int{"x": sc.WorkArea.X, "y": sc.WorkArea.Y, "width": sc.WorkArea.Width, "height": sc.WorkArea.Height},
		})
	}
	return map[string]any{"ok": true, "value": out, "count": len(out)}
}
