// SPDX-Licence-Identifier: EUPL-1.2

// screen.go — bridge tools for monitor/display info. Wraps the
// Wails ScreenManager API.

package bridge

import (
	"github.com/wailsapp/wails/v3/pkg/application"
)

// screenInfo serialises a *application.Screen.
func screenInfo(sc *application.Screen) map[string]any {
	if sc == nil {
		return nil
	}
	return map[string]any{
		"id":           sc.ID,
		"name":         sc.Name,
		"scale_factor": sc.ScaleFactor,
		"x":            sc.X,
		"y":            sc.Y,
		"size":         map[string]int{"width": sc.Size.Width, "height": sc.Size.Height},
		"bounds":       map[string]int{"x": sc.Bounds.X, "y": sc.Bounds.Y, "width": sc.Bounds.Width, "height": sc.Bounds.Height},
		"work_area":    map[string]int{"x": sc.WorkArea.X, "y": sc.WorkArea.Y, "width": sc.WorkArea.Width, "height": sc.WorkArea.Height},
		"is_primary":   sc.IsPrimary,
		"rotation":     sc.Rotation,
	}
}

// toolScreenList returns every connected display.
func (s *Service) toolScreenList() map[string]any {
	app := s.app()
	if app == nil || app.Screen == nil {
		return map[string]any{"ok": false, "error": screenManagerUnavailable}
	}
	screens := app.Screen.GetAll()
	out := make([]map[string]any, 0, len(screens))
	for _, sc := range screens {
		out = append(out, screenInfo(sc))
	}
	return map[string]any{"ok": true, "value": out, "count": len(out)}
}

// toolScreenPrimary returns the primary display.
func (s *Service) toolScreenPrimary() map[string]any {
	app := s.app()
	if app == nil || app.Screen == nil {
		return map[string]any{"ok": false, "error": screenManagerUnavailable}
	}
	sc := app.Screen.GetPrimary()
	if sc == nil {
		return map[string]any{"ok": false, "error": "no primary screen"}
	}
	return map[string]any{"ok": true, "value": screenInfo(sc)}
}

// toolScreenGet returns a specific screen by id. params: { id }
func (s *Service) toolScreenGet(params map[string]any) map[string]any {
	app := s.app()
	if app == nil || app.Screen == nil {
		return map[string]any{"ok": false, "error": screenManagerUnavailable}
	}
	id := paramString(params, "id", "")
	if id == "" {
		return map[string]any{"ok": false, "error": idParamRequired}
	}
	sc := app.Screen.GetByID(id)
	if sc == nil {
		return map[string]any{"ok": false, "error": "screen not found: " + id}
	}
	return map[string]any{"ok": true, "value": screenInfo(sc)}
}

// toolScreenAtPoint returns the screen containing (x, y).
// Implemented by walking all screens' bounds — Wails doesn't expose
// a single GetScreenAtPoint accessor on its manager.
// params: { x, y }
func (s *Service) toolScreenAtPoint(params map[string]any) map[string]any {
	app := s.app()
	if app == nil || app.Screen == nil {
		return map[string]any{"ok": false, "error": screenManagerUnavailable}
	}
	x := paramInt(params, "x", 0)
	y := paramInt(params, "y", 0)
	for _, sc := range app.Screen.GetAll() {
		if x >= sc.Bounds.X && x < sc.Bounds.X+sc.Bounds.Width &&
			y >= sc.Bounds.Y && y < sc.Bounds.Y+sc.Bounds.Height {
			return map[string]any{"ok": true, "value": screenInfo(sc)}
		}
	}
	return map[string]any{"ok": false, "error": "no screen contains point"}
}

// toolScreenForWindow returns the screen a named window is on.
// params: { name }
func (s *Service) toolScreenForWindow(params map[string]any) map[string]any {
	wv, errResp := s.windowOrErr(paramString(params, "name", ""))
	if errResp != nil {
		return errResp
	}
	sc, err := wv.GetScreen()
	if err != nil {
		return map[string]any{"ok": false, "error": err.Error()}
	}
	return map[string]any{"ok": true, "value": screenInfo(sc)}
}

// toolScreenWorkAreas returns the work_area Rect for every screen
// (usable space excluding dock/menubar).
func (s *Service) toolScreenWorkAreas() map[string]any {
	app := s.app()
	if app == nil || app.Screen == nil {
		return map[string]any{"ok": false, "error": screenManagerUnavailable}
	}
	screens := app.Screen.GetAll()
	out := make([]map[string]any, 0, len(screens))
	for _, sc := range screens {
		out = append(out, map[string]any{
			"id":        sc.ID,
			"name":      sc.Name,
			"work_area": map[string]int{"x": sc.WorkArea.X, "y": sc.WorkArea.Y, "width": sc.WorkArea.Width, "height": sc.WorkArea.Height},
		})
	}
	return map[string]any{"ok": true, "value": out, "count": len(out)}
}
