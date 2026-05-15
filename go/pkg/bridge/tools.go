// SPDX-Licence-Identifier: EUPL-1.2

// Tool implementations + HTTP handlers for the bridge. Kept in a
// separate file so bridge.go stays focused on lifecycle.

package bridge

import (
	"io"
	"net/http"
	"time"

	core "dappco.re/go"
	guiwindow "dappco.re/go/gui/pkg/window"
)

// ─── HTTP handlers ──────────────────────────────────────────────────

func (s *Service) handleInfo(w http.ResponseWriter, _ *http.Request) {
	corsJSON(w)
	writeJSON(w, map[string]any{
		"name":    "lthn-desktop-bridge",
		"version": "0.1.0",
		"port":    s.port,
		"tools":   toolCatalogue(),
	})
}

func (s *Service) handleTools(w http.ResponseWriter, _ *http.Request) {
	corsJSON(w)
	writeJSON(w, map[string]any{"tools": toolCatalogue()})
}

func (s *Service) handleHealth(w http.ResponseWriter, _ *http.Request) {
	corsJSON(w)
	writeJSON(w, map[string]any{"ok": true, "port": s.port})
}

func (s *Service) handleCall(w http.ResponseWriter, r *http.Request) {
	corsJSON(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		writeJSON(w, map[string]any{"error": "POST required", "ok": false})
		return
	}
	var req struct {
		Tool   string         `json:"tool"`
		Params map[string]any `json:"params"`
	}
	if rr := readJSON(r, &req); !rr.OK {
		w.WriteHeader(http.StatusBadRequest)
		writeJSON(w, map[string]any{"error": "invalid json: " + rr.Error(), "ok": false})
		return
	}
	resp := s.dispatch(r.Context(), req.Tool, req.Params)
	writeJSON(w, resp)
}

func (s *Service) handleInternalConsole(w http.ResponseWriter, r *http.Request) {
	corsJSON(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var entry ConsoleEntry
	if rr := readJSON(r, &entry); !rr.OK {
		w.WriteHeader(http.StatusBadRequest)
		writeJSON(w, map[string]any{"error": rr.Error()})
		return
	}
	if entry.At.IsZero() {
		entry.At = time.Now()
	}
	s.consoleMu.Lock()
	s.consoleBuf = append(s.consoleBuf, entry)
	if len(s.consoleBuf) > consoleBufLimit {
		s.consoleBuf = s.consoleBuf[len(s.consoleBuf)-consoleBufLimit:]
	}
	s.consoleMu.Unlock()
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Service) handleInternalError(w http.ResponseWriter, r *http.Request) {
	corsJSON(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var entry ErrorEntry
	if rr := readJSON(r, &entry); !rr.OK {
		w.WriteHeader(http.StatusBadRequest)
		writeJSON(w, map[string]any{"error": rr.Error()})
		return
	}
	if entry.At.IsZero() {
		entry.At = time.Now()
	}
	s.errorMu.Lock()
	s.errorBuf = append(s.errorBuf, entry)
	if len(s.errorBuf) > consoleBufLimit {
		s.errorBuf = s.errorBuf[len(s.errorBuf)-consoleBufLimit:]
	}
	s.errorMu.Unlock()
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Service) handleInternalEvalReply(w http.ResponseWriter, r *http.Request) {
	corsJSON(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var reply evalReply
	if rr := readJSON(r, &reply); !rr.OK {
		w.WriteHeader(http.StatusBadRequest)
		writeJSON(w, map[string]any{"error": rr.Error()})
		return
	}
	s.evalMu.Lock()
	ch, ok := s.pendingEvals[reply.ReqID]
	s.evalMu.Unlock()
	if !ok {
		writeJSON(w, map[string]any{"ok": false, "error": "no pending eval for reqId"})
		return
	}
	select {
	case ch <- reply:
	default:
		// channel already received once — drop silently.
	}
	writeJSON(w, map[string]any{"ok": true})
}

// ─── Tool dispatch ──────────────────────────────────────────────────

// toolCatalogue is the descriptor list reported by /mcp/info +
// /mcp/tools. Kept tight — every tool here is implemented below.
func toolCatalogue() []map[string]any {
	return []map[string]any{
		{"name": "webview_eval", "desc": "Run JS in a webview and return the result. params: { script, window? }"},
		{"name": "webview_console", "desc": "Recent console.log/info/warn/error/debug entries. params: { level?, limit? }"},
		{"name": "webview_errors", "desc": "Recent uncaught exceptions + unhandled rejections. params: { limit? }"},
		{"name": "webview_url", "desc": "Current window.location.href. params: { window? }"},
		{"name": "webview_title", "desc": "Current document.title. params: { window? }"},
		{"name": "webview_query", "desc": "querySelectorAll → [{tag, id, classes, x, y, w, h, text}]. params: { selector, window? }"},
		{"name": "webview_click", "desc": "Click element matching selector. params: { selector, window? }"},
		{"name": "webview_navigate", "desc": "Navigate to URL. params: { url, window? }"},
		{"name": "webview_windows", "desc": "List registered Wails window names."},
		{"name": "layout_save", "desc": "Snapshot every window's pos/size/state to ~/Lethean/conf/layouts/<name>.json. params: { name }"},
		{"name": "layout_restore", "desc": "Apply a saved layout to current windows. params: { name }"},
		{"name": "layout_list", "desc": "List every saved layout (name + saved_at + window count)."},
		{"name": "layout_delete", "desc": "Remove a saved layout file. params: { name }"},
		{"name": "layout_get", "desc": "Return one saved layout's full contents without applying. params: { name }"},
		{"name": "layout_tile", "desc": "Tile windows by mode: left, right, top, bottom, halves, thirds, quadrants, grid. params: { mode, windows? }"},
		{"name": "layout_snap", "desc": "Snap window to screen edge/corner. params: { name, position: top|bottom|left|right|top-left|top-right|bottom-left|bottom-right|centre }"},
		{"name": "layout_stack", "desc": "Cascade windows from top-left with constant pixel offsets. params: { windows?, offsetX?, offsetY?, width?, height? }"},
		{"name": "layout_workflow", "desc": "Apply preset workflow arrangement: default, coding, review, ops, single. params: { workflow, name? (for single) }"},
		{"name": "webview_hover", "desc": "Dispatch mouseover/mouseenter events on selector. params: { selector, window? }"},
		{"name": "webview_type", "desc": "Set input/textarea value + dispatch input+change events. params: { selector, value, window? }"},
		{"name": "webview_check", "desc": "Set checkbox/radio checked state. params: { selector, checked, window? }"},
		{"name": "webview_select", "desc": "Pick an option in a <select>. params: { selector, value, window? }"},
		{"name": "webview_scroll", "desc": "Scroll to element (selector) or coords (x, y). params: { selector?, x?, y?, behavior?, window? }"},
		{"name": "webview_dom_tree", "desc": "Return DOM tree from selector down to maxDepth. params: { selector?, maxDepth?, window? }"},
		{"name": "webview_source", "desc": "Return document.documentElement.outerHTML. params: { window? }"},
		{"name": "webview_computed_style", "desc": "Return getComputedStyle entries (optionally narrowed by props list). params: { selector, props?, window? }"},
		{"name": "webview_element_info", "desc": "Rich descriptor: tag, attrs, bounds, computed display, text snippet. params: { selector, window? }"},
		{"name": "webview_highlight", "desc": "Temporary outline + scroll-into-view for visual locate. params: { selector, duration?, window? }"},
		{"name": "webview_console_clear", "desc": "Empty the bridge's console ring buffer."},
		{"name": "webview_performance", "desc": "Return performance.timing + memory + navigation entry. params: { window? }"},
		{"name": "window_list", "desc": "List every registered window with full state (pos, size, visible, maximised, focused…)."},
		{"name": "window_get", "desc": "One window's full state. params: { name }"},
		{"name": "window_position", "desc": "Move a window. params: { name, x, y }"},
		{"name": "window_size", "desc": "Resize a window. params: { name, width, height }"},
		{"name": "window_bounds", "desc": "Set position + size in one call. params: { name, x, y, width, height }"},
		{"name": "window_maximise", "desc": "Maximise a window. params: { name }"},
		{"name": "window_minimise", "desc": "Minimise a window. params: { name }"},
		{"name": "window_restore", "desc": "Restore from minimised/maximised/fullscreen. params: { name }"},
		{"name": "window_focus", "desc": "Bring window to front. params: { name }"},
		{"name": "window_focused", "desc": "Return the currently focused window (or null)."},
		{"name": "window_visibility", "desc": "Show or hide a window. params: { name, visible }"},
		{"name": "window_always_on_top", "desc": "Pin window above others. params: { name, enabled }"},
		{"name": "window_title", "desc": "Set the window title. params: { name, title }"},
		{"name": "window_title_get", "desc": "Read the window title (hint: prefer webview_title for live document.title). params: { name }"},
		{"name": "window_fullscreen", "desc": "Toggle fullscreen. params: { name, enabled? }"},
		{"name": "window_close", "desc": "Close a window. params: { name }"},
		{"name": "window_center", "desc": "Centre a window on its current screen. params: { name }"},
		{"name": "window_background_colour", "desc": "Set window background RGBA. params: { name, r, g, b, a }"},
		{"name": "screen_list", "desc": "List every connected display."},
		{"name": "screen_primary", "desc": "Return the primary display."},
		{"name": "screen_get", "desc": "Return one display by id. params: { id }"},
		{"name": "screen_at_point", "desc": "Return the screen containing (x, y). params: { x, y }"},
		{"name": "screen_for_window", "desc": "Return the screen a named window is on. params: { name }"},
		{"name": "screen_work_areas", "desc": "Return per-screen work_area Rect (excluding dock/menubar)."},
		{"name": "file_read", "desc": "Read a file. params: { path }"},
		{"name": "file_write", "desc": "Write content to a file (creates parent dirs). params: { path, content, mode? }"},
		{"name": "file_edit", "desc": "Replace every occurrence of find with replace. params: { path, find, replace }"},
		{"name": "file_delete", "desc": "Remove a file. params: { path }"},
		{"name": "file_exists", "desc": "Check existence + is_dir. params: { path }"},
		{"name": "file_rename", "desc": "Rename/move a file. params: { from, to }"},
		{"name": "dir_list", "desc": "Enumerate a directory's entries. params: { path }"},
		{"name": "dir_create", "desc": "Make a directory tree. params: { path, mode? }"},
		{"name": "process_start", "desc": "Spawn a process via dappco.re/go/process. params: { command, args?, dir?, env? }"},
		{"name": "process_kill", "desc": "Kill a managed process. params: { id }"},
		{"name": "process_stop", "desc": "Alias for process_kill. params: { id }"},
		{"name": "process_list", "desc": "Enumerate every managed process."},
		{"name": "process_output", "desc": "Read ring-buffered stdout/stderr. params: { id }"},
		{"name": "process_input", "desc": "Write to a managed process's stdin. params: { id, input }"},
		{"name": "clipboard_read", "desc": "Return the current clipboard text."},
		{"name": "clipboard_write", "desc": "Set the clipboard text. params: { text }"},
		{"name": "clipboard_has", "desc": "Report whether the clipboard has non-empty text."},
		{"name": "clipboard_clear", "desc": "Empty the clipboard."},
		{"name": "lang_detect", "desc": "Map a file path's extension to a language name. params: { path }"},
		{"name": "lang_list", "desc": "Return the catalogue of supported language names."},
		{"name": "theme_get", "desc": "Return the WebView's prefers-color-scheme (dark/light). params: { window? }"},
		{"name": "focus_set", "desc": "Alias for window_focus. params: { name }"},
		{"name": "sandbox_detect", "desc": "List available container runtimes (docker / podman / apple)."},
		{"name": "sandbox_spawn", "desc": "Run a one-shot container, return stdout. params: { image, command, args?, runtime?, timeout_seconds? }"},
	}
}

func (s *Service) dispatch(ctx core.Context, tool string, params map[string]any) map[string]any {
	switch tool {
	case "webview_console":
		return s.toolConsole(params)
	case "webview_errors":
		return s.toolErrors(params)
	case "webview_eval":
		return s.eval(ctx, paramString(params, "window", DefaultWindow), paramString(params, "script", ""))
	case "webview_url":
		return s.eval(ctx, paramString(params, "window", DefaultWindow), `return window.location.href;`)
	case "webview_title":
		return s.eval(ctx, paramString(params, "window", DefaultWindow), `return document.title;`)
	case "webview_query":
		sel := jsonLit(paramString(params, "selector", ""))
		return s.eval(ctx, paramString(params, "window", DefaultWindow),
			core.Sprintf(`var els=Array.from(document.querySelectorAll(%s));return els.map(function(e){var r=e.getBoundingClientRect();return {tag:e.tagName.toLowerCase(),id:e.id||null,classes:Array.from(e.classList),x:r.x,y:r.y,w:r.width,h:r.height,text:(e.textContent||'').slice(0,200)};});`, sel))
	case "webview_click":
		sel := jsonLit(paramString(params, "selector", ""))
		return s.eval(ctx, paramString(params, "window", DefaultWindow),
			core.Sprintf(`var el=document.querySelector(%s);if(!el)throw new Error("element not found: "+%s);el.click();return {clicked:true,tag:el.tagName.toLowerCase()};`, sel, sel))
	case "webview_navigate":
		url := jsonLit(paramString(params, "url", ""))
		return s.eval(ctx, paramString(params, "window", DefaultWindow),
			core.Sprintf(`window.location.href=%s;return {navigatedTo:%s};`, url, url))
	case "webview_windows":
		return s.toolWindows()
	case "layout_save":
		return s.toolLayoutSave(params)
	case "layout_restore":
		return s.toolLayoutRestore(params)
	case "layout_list":
		return s.toolLayoutList()
	case "layout_delete":
		return s.toolLayoutDelete(params)
	case "layout_get":
		return s.toolLayoutGet(params)
	case "layout_tile":
		return s.toolLayoutTile(params)
	case "layout_snap":
		return s.toolLayoutSnap(params)
	case "layout_stack":
		return s.toolLayoutStack(params)
	case "layout_workflow":
		return s.toolLayoutWorkflow(params)
	case "webview_hover":
		return s.toolWebviewHover(ctx, params)
	case "webview_type":
		return s.toolWebviewType(ctx, params)
	case "webview_check":
		return s.toolWebviewCheck(ctx, params)
	case "webview_select":
		return s.toolWebviewSelect(ctx, params)
	case "webview_scroll":
		return s.toolWebviewScroll(ctx, params)
	case "webview_dom_tree":
		return s.toolWebviewDOMTree(ctx, params)
	case "webview_source":
		return s.toolWebviewSource(ctx, params)
	case "webview_computed_style":
		return s.toolWebviewComputedStyle(ctx, params)
	case "webview_element_info":
		return s.toolWebviewElementInfo(ctx, params)
	case "webview_highlight":
		return s.toolWebviewHighlight(ctx, params)
	case "webview_console_clear":
		return s.toolWebviewConsoleClear()
	case "webview_performance":
		return s.toolWebviewPerformance(ctx, params)
	case "window_list":
		return s.toolWindowList()
	case "window_get":
		return s.toolWindowGet(params)
	case "window_position":
		return s.toolWindowPosition(params)
	case "window_size":
		return s.toolWindowSize(params)
	case "window_bounds":
		return s.toolWindowBounds(params)
	case "window_maximise", "window_maximize":
		return s.toolWindowMaximise(params)
	case "window_minimise", "window_minimize":
		return s.toolWindowMinimise(params)
	case "window_restore":
		return s.toolWindowRestore(params)
	case "window_focus":
		return s.toolWindowFocus(params)
	case "window_focused":
		return s.toolWindowFocused()
	case "window_visibility":
		return s.toolWindowVisibility(params)
	case "window_always_on_top":
		return s.toolWindowAlwaysOnTop(params)
	case "window_title":
		return s.toolWindowSetTitle(params)
	case "window_title_get":
		return s.toolWindowGetTitle(params)
	case "window_fullscreen":
		return s.toolWindowFullscreen(params)
	case "window_close":
		return s.toolWindowClose(params)
	case "window_center", "window_centre":
		return s.toolWindowCenter(params)
	case "window_background_colour", "window_background_color":
		return s.toolWindowBackgroundColour(params)
	case "screen_list":
		return s.toolScreenList()
	case "screen_primary":
		return s.toolScreenPrimary()
	case "screen_get":
		return s.toolScreenGet(params)
	case "screen_at_point":
		return s.toolScreenAtPoint(params)
	case "screen_for_window":
		return s.toolScreenForWindow(params)
	case "screen_work_areas":
		return s.toolScreenWorkAreas()
	case "file_read":
		return s.toolFileRead(params)
	case "file_write":
		return s.toolFileWrite(params)
	case "file_edit":
		return s.toolFileEdit(params)
	case "file_delete":
		return s.toolFileDelete(params)
	case "file_exists":
		return s.toolFileExists(params)
	case "file_rename":
		return s.toolFileRename(params)
	case "dir_list":
		return s.toolDirList(params)
	case "dir_create":
		return s.toolDirCreate(params)
	case "process_start":
		return s.toolProcessStart(ctx, params)
	case "process_kill":
		return s.toolProcessKill(params)
	case "process_stop":
		return s.toolProcessStop(params)
	case "process_list":
		return s.toolProcessList()
	case "process_output":
		return s.toolProcessOutput(params)
	case "process_input":
		return s.toolProcessInput(params)
	case "clipboard_read":
		return s.toolClipboardRead()
	case "clipboard_write":
		return s.toolClipboardWrite(params)
	case "clipboard_has":
		return s.toolClipboardHas()
	case "clipboard_clear":
		return s.toolClipboardClear()
	case "lang_detect":
		return s.toolLangDetect(params)
	case "lang_list":
		return s.toolLangList()
	case "theme_get", "theme_system":
		return s.toolThemeGet(ctx, params)
	case "focus_set":
		return s.toolFocusSet(params)
	case "sandbox_detect":
		return s.toolSandboxDetect()
	case "sandbox_spawn":
		return s.toolSandboxSpawn(params)
	default:
		return map[string]any{"ok": false, "error": "unknown tool: " + tool}
	}
}

// ─── Tool implementations ───────────────────────────────────────────

func (s *Service) toolConsole(params map[string]any) map[string]any {
	level := paramString(params, "level", "")
	limit := paramInt(params, "limit", 0)
	s.consoleMu.Lock()
	defer s.consoleMu.Unlock()
	out := make([]ConsoleEntry, 0, len(s.consoleBuf))
	for _, e := range s.consoleBuf {
		if level != "" && e.Level != level {
			continue
		}
		out = append(out, e)
	}
	if limit > 0 && len(out) > limit {
		out = out[len(out)-limit:]
	}
	return map[string]any{"ok": true, "value": out, "count": len(out)}
}

func (s *Service) toolErrors(params map[string]any) map[string]any {
	limit := paramInt(params, "limit", 0)
	s.errorMu.Lock()
	defer s.errorMu.Unlock()
	out := append([]ErrorEntry(nil), s.errorBuf...)
	if limit > 0 && len(out) > limit {
		out = out[len(out)-limit:]
	}
	return map[string]any{"ok": true, "value": out, "count": len(out)}
}

func (s *Service) toolWindows() map[string]any {
	all, errResp := s.coreGUIWindowList()
	if errResp != nil {
		return errResp
	}
	out := make([]map[string]any, 0, len(all))
	for i := range all {
		out = append(out, map[string]any{"name": all[i].Name})
	}
	return map[string]any{"ok": true, "value": out, "count": len(out)}
}

// ─── eval ───────────────────────────────────────────────────────────

// eval dispatches the wrapped body through core/gui and waits for
// the fetch-back at /internal/eval-reply. 5s timeout - anything
// longer is almost certainly a hung script.
func (s *Service) eval(ctx core.Context, windowName, body string) map[string]any {
	if body == "" {
		return map[string]any{"ok": false, "error": "script param required"}
	}

	s.evalMu.Lock()
	s.evalCounter++
	reqID := core.Sprintf("eval-%d", s.evalCounter)
	ch := make(chan evalReply, 1)
	s.pendingEvals[reqID] = ch
	s.evalMu.Unlock()

	defer func() {
		s.evalMu.Lock()
		delete(s.pendingEvals, reqID)
		s.evalMu.Unlock()
	}()

	// Indirect-eval ((0,eval)(body)) — returns the value of the last
	// expression statement naturally. The earlier IIFE wrap
	// ((function(){body})()) required callers to write explicit
	// `return` because expression statements inside a function body
	// don't auto-return; `1+1` and `document.title` both came back
	// as undefined. Indirect eval handles bare-expression bodies AND
	// IIFE-wrapped bodies AND multi-statement scripts (last expression
	// wins). If the body returns a Promise, await it so async fetches
	// resolve before we POST back.
	wrapped := core.Sprintf(`(function(){
  var __id=%s;
  var __post=function(payload){
    try{fetch('http://127.0.0.1:%d/internal/eval-reply',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(payload),keepalive:true}).catch(function(){});}catch(e){}
  };
  try{
    var __r=(0,eval)(%s);
    if(__r && typeof __r.then==='function'){
      __r.then(function(v){__post({reqId:__id, result:v});}, function(e){__post({reqId:__id, error:String(e)+(e&&e.stack?'\n'+e.stack:'')});});
    } else {
      __post({reqId:__id, result:__r});
    }
  }catch(e){
    __post({reqId:__id, error:String(e)+(e&&e.stack?'\n'+e.stack:'')});
  }
})();`, jsonLit(reqID), s.port, jsonLit(body))

	if errResp := s.runCoreGUIWindowTask("window.exec_js", guiwindow.TaskExecJS{Name: windowName, JS: wrapped}); errResp != nil {
		return errResp
	}

	select {
	case reply := <-ch:
		if reply.Error != "" {
			return map[string]any{"ok": false, "error": reply.Error}
		}
		return map[string]any{"ok": true, "value": reply.Result}
	case <-time.After(5 * time.Second):
		return map[string]any{"ok": false, "error": "eval timeout (5s)", "reqId": reqID}
	case <-ctx.Done():
		return map[string]any{"ok": false, "error": "context cancelled"}
	}
}

// ─── Wails3 Service shape ───────────────────────────────────────────
//
// Wails generates TS bindings for every exported method on the
// registered service instance — these mirror the MCP tool surface
// so the WebView (logs window, activity surface, future obs tools)
// can read the bridge's ring buffers directly via in-process
// bindings, no HTTP round-trip.
//
// The bridge stays registered in Core for its OnStartup/OnShutdown
// lifecycle; the wailsServices array in pkg/desktop/desktop.go also
// adds it via application.NewService so these methods are bindable.

// ServiceName labels the binding namespace exposed to JS.
func (s *Service) ServiceName() string { return "Bridge" }

// ServiceStartup is a no-op for the Wails lifecycle — the bridge's
// HTTP listener boots via the Core OnStartup hook. Wails calls this
// once per session after application.New returns.
func (s *Service) ServiceStartup(_ core.Context, _ any) core.Result {
	return core.Ok(nil)
}

// ServiceShutdown is a no-op for the Wails lifecycle. The HTTP
// listener tears down via the Core OnShutdown hook.
func (s *Service) ServiceShutdown() core.Result { return core.Ok(nil) }

// Console returns the recent console entries the JS shim has POSTed
// to /internal/console. limit caps the slice length from the tail
// (0 = no cap, returns the whole buffer). level filters by
// "log" / "info" / "warn" / "error" / "debug" — empty matches all.
//
// Usage example (TS):
//
//	import { Console } from "@desktop/bridge/service";
//	const entries = await Console("", 50);
//	for (const e of entries) console.log(e.level, e.message);
func (s *Service) Console(level string, limit int) []ConsoleEntry {
	s.consoleMu.Lock()
	defer s.consoleMu.Unlock()
	out := make([]ConsoleEntry, 0, len(s.consoleBuf))
	for _, e := range s.consoleBuf {
		if level != "" && e.Level != level {
			continue
		}
		out = append(out, e)
	}
	if limit > 0 && len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out
}

// Errors returns the recent uncaught exceptions + unhandled
// rejections the JS shim has POSTed to /internal/error. limit
// trims from the tail.
func (s *Service) Errors(limit int) []ErrorEntry {
	s.errorMu.Lock()
	defer s.errorMu.Unlock()
	out := append([]ErrorEntry(nil), s.errorBuf...)
	if limit > 0 && len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out
}

// ClearConsole empties the console ring buffer. The logs window's
// "Clear" button calls this; useful when investigating a fresh
// repro without older noise.
func (s *Service) ClearConsole() {
	s.consoleMu.Lock()
	s.consoleBuf = nil
	s.consoleMu.Unlock()
}

// ClearErrors empties the error ring buffer.
func (s *Service) ClearErrors() {
	s.errorMu.Lock()
	s.errorBuf = nil
	s.errorMu.Unlock()
}

// ─── HTTP helpers ───────────────────────────────────────────────────

func corsJSON(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
}

func writeJSON(w http.ResponseWriter, v any) {
	r := core.JSONMarshal(v)
	if !r.OK {
		http.Error(w, `{"error":"json encode failed"}`, http.StatusInternalServerError)
		return
	}
	enc, _ := r.Value.([]byte)
	_, _ = w.Write(enc)
}

func readJSON(r *http.Request, dst any) core.Result {
	body, err := io.ReadAll(io.LimitReader(r.Body, 4<<20))
	if err != nil {
		return core.Fail(core.E("bridge.readJSON", "read request JSON failed", err))
	}
	decoded := core.JSONUnmarshal(body, dst)
	if !decoded.OK {
		return core.Fail(core.E("bridge.readJSON", "decode request JSON failed", decoded.Value.(error)))
	}
	return core.Ok(nil)
}

// ─── Param helpers ──────────────────────────────────────────────────

func paramString(params map[string]any, key, dflt string) string {
	if params == nil {
		return dflt
	}
	if v, ok := params[key]; ok {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return dflt
}

func paramBool(params map[string]any, key string, dflt bool) bool {
	if v, ok := params[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return dflt
}

func paramInt(params map[string]any, key string, dflt int) int {
	if params == nil {
		return dflt
	}
	if v, ok := params[key]; ok {
		switch n := v.(type) {
		case float64:
			return int(n)
		case int:
			return n
		}
	}
	return dflt
}

// jsonLit returns a JSON-quoted string literal suitable for inline
// embedding in a JS template. Keeps quoting / escaping consistent.
func jsonLit(s string) string {
	return core.JSONMarshalString(s)
}
