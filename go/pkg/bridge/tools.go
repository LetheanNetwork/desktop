// SPDX-Licence-Identifier: EUPL-1.2

// Tool implementations + HTTP handlers for the bridge. Kept in a
// separate file so bridge.go stays focused on lifecycle.

package bridge

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	core "dappco.re/go"
	"github.com/wailsapp/wails/v3/pkg/application"
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
	if err := readJSON(r, &req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		writeJSON(w, map[string]any{"error": "invalid json: " + err.Error(), "ok": false})
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
	if err := readJSON(r, &entry); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		writeJSON(w, map[string]any{"error": err.Error()})
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
	if err := readJSON(r, &entry); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		writeJSON(w, map[string]any{"error": err.Error()})
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
	if err := readJSON(r, &reply); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		writeJSON(w, map[string]any{"error": err.Error()})
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
	}
}

func (s *Service) dispatch(ctx context.Context, tool string, params map[string]any) map[string]any {
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
	app := s.app()
	if app == nil {
		return map[string]any{"ok": false, "error": "wails application not initialised yet"}
	}
	// Wails3's Window.GetAll returns []application.Window; map each to
	// its name + class for the agent's window-pick decisions.
	all := app.Window.GetAll()
	out := make([]map[string]any, 0, len(all))
	for _, w := range all {
		out = append(out, map[string]any{"name": w.Name()})
	}
	return map[string]any{"ok": true, "value": out, "count": len(out)}
}

// ─── eval ───────────────────────────────────────────────────────────

// eval ExecJS's the wrapped body in the named window and waits for
// the fetch-back at /internal/eval-reply. 5s timeout — anything
// longer is almost certainly a hung script.
func (s *Service) eval(ctx context.Context, windowName, body string) map[string]any {
	if body == "" {
		return map[string]any{"ok": false, "error": "script param required"}
	}
	app := s.app()
	if app == nil {
		return map[string]any{"ok": false, "error": "wails application not initialised yet"}
	}
	w, ok := app.Window.GetByName(windowName)
	if !ok || w == nil {
		return map[string]any{"ok": false, "error": "window not found: " + windowName}
	}
	wv, ok := w.(*application.WebviewWindow)
	if !ok {
		return map[string]any{"ok": false, "error": "window is not a WebviewWindow: " + windowName}
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

	wrapped := core.Sprintf(`(function(){
  var __id=%s;
  var __post=function(payload){
    try{fetch('http://127.0.0.1:%d/internal/eval-reply',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(payload),keepalive:true}).catch(function(){});}catch(e){}
  };
  try{
    var __r=(function(){%s})();
    __post({reqId:__id, result:__r});
  }catch(e){
    __post({reqId:__id, error:String(e)+(e&&e.stack?'\n'+e.stack:'')});
  }
})();`, jsonLit(reqID), s.port, body)

	wv.ExecJS(wrapped)

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

// ─── HTTP helpers ───────────────────────────────────────────────────

func corsJSON(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
}

func writeJSON(w http.ResponseWriter, v any) {
	enc, err := json.Marshal(v)
	if err != nil {
		http.Error(w, `{"error":"json encode failed"}`, http.StatusInternalServerError)
		return
	}
	_, _ = w.Write(enc)
}

func readJSON(r *http.Request, dst any) error {
	body, err := io.ReadAll(io.LimitReader(r.Body, 4<<20))
	if err != nil {
		return err
	}
	return json.Unmarshal(body, dst)
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
	enc, err := json.Marshal(s)
	if err != nil {
		return `""`
	}
	return string(enc)
}
