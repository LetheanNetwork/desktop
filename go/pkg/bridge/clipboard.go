// SPDX-Licence-Identifier: EUPL-1.2

// clipboard.go — bridge tools for system clipboard read/write.
// Wraps the core/gui clipboard service.

package bridge

import (
	"context"

	core "dappco.re/go"
	guiclipboard "dappco.re/go/gui/pkg/clipboard"
)

// toolClipboardRead returns the current clipboard text.
func (s *Service) toolClipboardRead() map[string]any {
	r := s.Core().QUERY(guiclipboard.QueryText{})
	if !r.OK {
		return map[string]any{"ok": false, "error": r.Error()}
	}
	content, ok := r.Value.(guiclipboard.ClipboardContent)
	if !ok {
		return map[string]any{"ok": false, "error": clipboardManagerUnavailable}
	}
	if !content.HasContent {
		return map[string]any{"ok": true, "value": "", "has_content": false}
	}
	return map[string]any{"ok": true, "value": content.Text, "has_content": content.Text != ""}
}

// toolClipboardWrite sets the clipboard text. params: { text }
func (s *Service) toolClipboardWrite(params map[string]any) map[string]any {
	text := paramString(params, "text", "")
	r := s.Core().Action("clipboard.set_text").Run(context.Background(), core.NewOptions(
		core.Option{Key: "text", Value: text},
	))
	if !r.OK {
		return map[string]any{"ok": false, "error": r.Error()}
	}
	ok, _ := r.Value.(bool)
	if !ok {
		return map[string]any{"ok": false, "error": "clipboard write failed"}
	}
	return map[string]any{"ok": true, "bytes": len(text)}
}

// toolClipboardHas reports whether the clipboard has non-empty text.
func (s *Service) toolClipboardHas() map[string]any {
	r := s.Core().QUERY(guiclipboard.QueryText{})
	if !r.OK {
		return map[string]any{"ok": false, "error": r.Error()}
	}
	content, ok := r.Value.(guiclipboard.ClipboardContent)
	if !ok {
		return map[string]any{"ok": false, "error": clipboardManagerUnavailable}
	}
	return map[string]any{"ok": true, "has_content": content.HasContent && content.Text != ""}
}

// toolClipboardClear empties the clipboard by writing the empty string.
func (s *Service) toolClipboardClear() map[string]any {
	r := s.Core().Action("clipboard.clear").Run(context.Background(), core.NewOptions())
	if !r.OK {
		return map[string]any{"ok": false, "error": r.Error()}
	}
	ok, _ := r.Value.(bool)
	if !ok {
		return map[string]any{"ok": false, "error": "clipboard clear failed"}
	}
	return map[string]any{"ok": true, "cleared": "clipboard"}
}
