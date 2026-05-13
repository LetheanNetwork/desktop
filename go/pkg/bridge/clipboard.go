// SPDX-Licence-Identifier: EUPL-1.2

// clipboard.go — bridge tools for system clipboard read/write.
// Wraps the Wails ClipboardManager.

package bridge

// toolClipboardRead returns the current clipboard text.
func (s *Service) toolClipboardRead() map[string]any {
	app := s.app()
	if app == nil || app.Clipboard == nil {
		return map[string]any{"ok": false, "error": "clipboard manager unavailable"}
	}
	text, ok := app.Clipboard.Text()
	if !ok {
		return map[string]any{"ok": true, "value": "", "has_content": false}
	}
	return map[string]any{"ok": true, "value": text, "has_content": text != ""}
}

// toolClipboardWrite sets the clipboard text. params: { text }
func (s *Service) toolClipboardWrite(params map[string]any) map[string]any {
	app := s.app()
	if app == nil || app.Clipboard == nil {
		return map[string]any{"ok": false, "error": "clipboard manager unavailable"}
	}
	text := paramString(params, "text", "")
	if !app.Clipboard.SetText(text) {
		return map[string]any{"ok": false, "error": "clipboard write failed"}
	}
	return map[string]any{"ok": true, "bytes": len(text)}
}

// toolClipboardHas reports whether the clipboard has non-empty text.
func (s *Service) toolClipboardHas() map[string]any {
	app := s.app()
	if app == nil || app.Clipboard == nil {
		return map[string]any{"ok": false, "error": "clipboard manager unavailable"}
	}
	text, ok := app.Clipboard.Text()
	return map[string]any{"ok": true, "has_content": ok && text != ""}
}

// toolClipboardClear empties the clipboard by writing the empty string.
func (s *Service) toolClipboardClear() map[string]any {
	app := s.app()
	if app == nil || app.Clipboard == nil {
		return map[string]any{"ok": false, "error": "clipboard manager unavailable"}
	}
	app.Clipboard.SetText("")
	return map[string]any{"ok": true, "cleared": "clipboard"}
}
