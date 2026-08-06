// SPDX-Licence-Identifier: EUPL-1.2

// Internal-package tests for tools.go HTTP handlers — Cerberus
// H#9-verify F2 (Mantis #1535). Handlers and helpers are unexported,
// so tests live in package bridge rather than bridge_test.

package bridge

import (
	core "dappco.re/go"
	guiclipboard "dappco.re/go/render/display/webkit/pkg/clipboard"
	guiscreen "dappco.re/go/render/display/webkit/pkg/screen"
	guiwindow "dappco.re/go/render/display/webkit/pkg/window"

	"dappco.re/go/process"
	"dappco.re/lthn/desktop/pkg/sandbox"
)

// /mcp/* responses must NOT carry Access-Control-Allow-Origin: * —
// even with bearer auth in place, the wildcard ACAO permits browser
// JS from any origin to read the response body once it has a token,
// defeating the same-origin policy defence layer. Cerberus
// H#9-verify F2.

func TestMCPInfo_DoesNotSetCORSAllowAllOrigin_Bad(t *core.T) {
	s := &Service{port: 9999}
	req := core.NewHTTPTestRequest(core.MethodGet, "/mcp/info", nil)
	rec := core.NewHTTPTestRecorder()
	s.handleInfo(rec, req)
	got := rec.Header().Get("Access-Control-Allow-Origin")
	core.AssertEqual(t, "", got,
		"/mcp/info response must NOT carry Access-Control-Allow-Origin (Cerberus F2)")
}

func TestMCPTools_DoesNotSetCORSAllowAllOrigin_Bad(t *core.T) {
	s := &Service{port: 9999}
	req := core.NewHTTPTestRequest(core.MethodGet, "/mcp/tools", nil)
	rec := core.NewHTTPTestRecorder()
	s.handleTools(rec, req)
	got := rec.Header().Get("Access-Control-Allow-Origin")
	core.AssertEqual(t, "", got,
		"/mcp/tools response must NOT carry Access-Control-Allow-Origin (Cerberus F2)")
}

func TestMCPCall_DoesNotSetCORSAllowAllOrigin_Bad(t *core.T) {
	s := &Service{port: 9999}
	// OPTIONS path short-circuits before dispatch; still must not set ACAO.
	req := core.NewHTTPTestRequest(core.MethodOptions, "/mcp/call", nil)
	rec := core.NewHTTPTestRecorder()
	s.handleCall(rec, req)
	got := rec.Header().Get("Access-Control-Allow-Origin")
	core.AssertEqual(t, "", got,
		"/mcp/call response must NOT carry Access-Control-Allow-Origin (Cerberus F2)")
}

func TestHealth_StillSetsCORSAllowAllOrigin_Good(t *core.T) {
	// /health is the third-party probe endpoint — wildcard ACAO is
	// intentional there so browser uptime checks work cross-origin.
	s := &Service{port: 9999}
	req := core.NewHTTPTestRequest(core.MethodGet, "/health", nil)
	rec := core.NewHTTPTestRecorder()
	s.handleHealth(rec, req)
	got := rec.Header().Get("Access-Control-Allow-Origin")
	core.AssertEqual(t, "*", got,
		"/health must keep wildcard ACAO — it is the third-party probe surface")
}

func TestMCPInfo_StillSetsJSONContentType_Good(t *core.T) {
	s := &Service{port: 9999}
	req := core.NewHTTPTestRequest(core.MethodGet, "/mcp/info", nil)
	rec := core.NewHTTPTestRecorder()
	s.handleInfo(rec, req)
	core.AssertEqual(t, "application/json", rec.Header().Get("Content-Type"),
		"noCorsJSON must still emit application/json Content-Type")
}

// ─── handleCall ─────────────────────────────────────────────────────

func TestTools_HandleCall_Good_ValidPost(t *core.T) {
	s := &Service{port: 9999}
	body := core.NewReader(`{"tool":"webview_console","params":{}}`)
	req := core.NewHTTPTestRequest(core.MethodPost, "/mcp/call", body)
	rec := core.NewHTTPTestRecorder()
	s.handleCall(rec, req)
	core.AssertEqual(t, 200, rec.Code)
	core.AssertContains(t, rec.Body.String(), `"ok":true`)
}

func TestTools_HandleCall_Good_OptionsPreflightNoContent(t *core.T) {
	s := &Service{port: 9999}
	req := core.NewHTTPTestRequest(core.MethodOptions, "/mcp/call", nil)
	rec := core.NewHTTPTestRecorder()
	s.handleCall(rec, req)
	core.AssertEqual(t, core.StatusNoContent, rec.Code)
}

func TestTools_HandleCall_Bad_WrongMethod(t *core.T) {
	s := &Service{port: 9999}
	req := core.NewHTTPTestRequest(core.MethodGet, "/mcp/call", nil)
	rec := core.NewHTTPTestRecorder()
	s.handleCall(rec, req)
	core.AssertEqual(t, core.StatusMethodNotAllowed, rec.Code)
	core.AssertContains(t, rec.Body.String(), "POST required")
}

func TestTools_HandleCall_Ugly_InvalidJSONBody(t *core.T) {
	s := &Service{port: 9999}
	body := core.NewReader(`{not valid json`)
	req := core.NewHTTPTestRequest(core.MethodPost, "/mcp/call", body)
	rec := core.NewHTTPTestRecorder()
	s.handleCall(rec, req)
	core.AssertEqual(t, core.StatusBadRequest, rec.Code)
	core.AssertContains(t, rec.Body.String(), "invalid json")
}

func TestTools_HandleCall_Ugly_UnknownTool(t *core.T) {
	s := &Service{port: 9999}
	body := core.NewReader(`{"tool":"not_a_real_tool","params":{}}`)
	req := core.NewHTTPTestRequest(core.MethodPost, "/mcp/call", body)
	rec := core.NewHTTPTestRecorder()
	s.handleCall(rec, req)
	core.AssertEqual(t, 200, rec.Code)
	core.AssertContains(t, rec.Body.String(), "unknown tool")
}

// ─── readJSON ───────────────────────────────────────────────────────

func TestTools_ReadJSON_Good(t *core.T) {
	var dst struct {
		Tool string `json:"tool"`
	}
	req := core.NewHTTPTestRequest(core.MethodPost, "/mcp/call", core.NewReader(`{"tool":"x"}`))
	r := readJSON(req, &dst)
	core.AssertTrue(t, r.OK)
	core.AssertEqual(t, "x", dst.Tool)
}

func TestTools_ReadJSON_Bad_MalformedBody(t *core.T) {
	var dst map[string]any
	req := core.NewHTTPTestRequest(core.MethodPost, "/mcp/call", core.NewReader(`{bad`))
	r := readJSON(req, &dst)
	core.AssertFalse(t, r.OK)
}

// ─── writeJSON ──────────────────────────────────────────────────────

func TestTools_WriteJSON_Good(t *core.T) {
	rec := core.NewHTTPTestRecorder()
	writeJSON(rec, map[string]any{"ok": true})
	core.AssertContains(t, rec.Body.String(), `"ok":true`)
}

func TestTools_WriteJSON_Ugly_UnmarshalableValueFailsCleanly(t *core.T) {
	rec := core.NewHTTPTestRecorder()
	// channels cannot be JSON-marshalled — drives the encode-failure
	// branch (core.HTTPError) instead of a normal body write.
	writeJSON(rec, map[string]any{"bad": make(chan int)})
	core.AssertEqual(t, core.StatusInternalServerError, rec.Code)
}

// ─── handleConsoleEvent / handleErrorEvent ──────────────────────────

func TestTools_HandleConsoleEvent_Good(t *core.T) {
	s := &Service{}
	s.handleConsoleEvent(map[string]any{
		"level": "info", "message": "hello", "source": "app.js",
		"at": core.Now().UTC().Format(core.TimeRFC3339),
	})
	core.AssertEqual(t, 1, len(s.consoleBuf))
	core.AssertEqual(t, "info", s.consoleBuf[0].Level)
	core.AssertEqual(t, "hello", s.consoleBuf[0].Message)
}

func TestTools_HandleConsoleEvent_Good_MissingAtBackfillsNow(t *core.T) {
	s := &Service{}
	s.handleConsoleEvent(map[string]any{"level": "warn", "message": "x"})
	core.AssertEqual(t, 1, len(s.consoleBuf))
	core.AssertFalse(t, s.consoleBuf[0].At.IsZero())
}

func TestTools_HandleConsoleEvent_Bad_WrongDataType(t *core.T) {
	s := &Service{}
	s.handleConsoleEvent("not a map")
	core.AssertEqual(t, 0, len(s.consoleBuf))
}

func TestTools_HandleConsoleEvent_Ugly_OversizedMessageTruncates(t *core.T) {
	s := &Service{}
	huge := make([]byte, maxEntryMessageBytes+500)
	for i := range huge {
		huge[i] = 'x'
	}
	s.handleConsoleEvent(map[string]any{"level": "info", "message": string(huge)})
	core.AssertEqual(t, 1, len(s.consoleBuf))
	core.AssertLessOrEqual(t, len(s.consoleBuf[0].Message), maxEntryMessageBytes+len(" […truncated]"))
	core.AssertContains(t, s.consoleBuf[0].Message, "[…truncated]")
}

func TestTools_HandleConsoleEvent_Ugly_RingBufferCapsAtLimit(t *core.T) {
	s := &Service{}
	for i := 0; i < consoleBufLimit+10; i++ {
		s.handleConsoleEvent(map[string]any{"level": "info", "message": "x"})
	}
	core.AssertEqual(t, consoleBufLimit, len(s.consoleBuf))
}

func TestTools_HandleErrorEvent_Good(t *core.T) {
	s := &Service{}
	s.handleErrorEvent(map[string]any{
		"message": "boom", "source": "app.js", "line": float64(12), "col": float64(3),
		"stack": "Error: boom\n  at x",
	})
	core.AssertEqual(t, 1, len(s.errorBuf))
	core.AssertEqual(t, "boom", s.errorBuf[0].Message)
	core.AssertEqual(t, 12, s.errorBuf[0].Line)
	core.AssertEqual(t, 3, s.errorBuf[0].Col)
}

func TestTools_HandleErrorEvent_Bad_WrongDataType(t *core.T) {
	s := &Service{}
	s.handleErrorEvent(42)
	core.AssertEqual(t, 0, len(s.errorBuf))
}

func TestTools_HandleErrorEvent_Ugly_OversizedStackTruncates(t *core.T) {
	s := &Service{}
	huge := make([]byte, maxEntryMessageBytes+200)
	for i := range huge {
		huge[i] = 'y'
	}
	s.handleErrorEvent(map[string]any{"message": "x", "stack": string(huge)})
	core.AssertContains(t, s.errorBuf[0].Stack, "[…truncated]")
}

func TestTools_HandleErrorEvent_Ugly_RingBufferCapsAtLimit(t *core.T) {
	s := &Service{}
	for i := 0; i < consoleBufLimit+5; i++ {
		s.handleErrorEvent(map[string]any{"message": "x"})
	}
	core.AssertEqual(t, consoleBufLimit, len(s.errorBuf))
}

// ─── stringField / intField / timeField ─────────────────────────────

func TestTools_StringField_Good(t *core.T) {
	core.AssertEqual(t, "v", stringField(map[string]any{"k": "v"}, "k"))
}

func TestTools_StringField_Bad_MissingKey(t *core.T) {
	core.AssertEqual(t, "", stringField(map[string]any{}, "k"))
}

func TestTools_StringField_Ugly_WrongType(t *core.T) {
	core.AssertEqual(t, "", stringField(map[string]any{"k": 5}, "k"))
}

func TestTools_IntField_Good(t *core.T) {
	core.AssertEqual(t, 7, intField(map[string]any{"k": float64(7)}, "k"))
}

func TestTools_IntField_Bad_MissingKey(t *core.T) {
	core.AssertEqual(t, 0, intField(map[string]any{}, "k"))
}

func TestTools_IntField_Ugly_NegativeRejected(t *core.T) {
	core.AssertEqual(t, 0, intField(map[string]any{"k": float64(-3)}, "k"))
}

func TestTools_TimeField_Good(t *core.T) {
	now := core.Now().UTC().Truncate(core.Second)
	got := timeField(map[string]any{"k": now.Format(core.TimeRFC3339)}, "k")
	core.AssertEqual(t, now.Format(core.TimeRFC3339), got.Format(core.TimeRFC3339))
}

func TestTools_TimeField_Bad_MissingKey(t *core.T) {
	core.AssertTrue(t, timeField(map[string]any{}, "k").IsZero())
}

func TestTools_TimeField_Ugly_Unparseable(t *core.T) {
	core.AssertTrue(t, timeField(map[string]any{"k": "not-a-time"}, "k").IsZero())
}

// ─── Service.Console / Errors / ClearConsole / ClearErrors ─────────

func TestTools_Service_Console_Good_FiltersByLevel(t *core.T) {
	s := &Service{}
	s.handleConsoleEvent(map[string]any{"level": "info", "message": "a"})
	s.handleConsoleEvent(map[string]any{"level": "warn", "message": "b"})
	out := s.Console("warn", 0)
	core.AssertEqual(t, 1, len(out))
	core.AssertEqual(t, "b", out[0].Message)
}

func TestTools_Service_Console_Ugly_LimitTrimsFromTail(t *core.T) {
	s := &Service{}
	for i := 0; i < 5; i++ {
		s.handleConsoleEvent(map[string]any{"level": "info", "message": "x"})
	}
	out := s.Console("", 2)
	core.AssertEqual(t, 2, len(out))
}

func TestTools_Service_Errors_Good(t *core.T) {
	s := &Service{}
	s.handleErrorEvent(map[string]any{"message": "a"})
	s.handleErrorEvent(map[string]any{"message": "b"})
	out := s.Errors(1)
	core.AssertEqual(t, 1, len(out))
	core.AssertEqual(t, "b", out[0].Message)
}

func TestTools_Service_ClearConsole_Good_Direct(t *core.T) {
	s := &Service{}
	s.handleConsoleEvent(map[string]any{"level": "info", "message": "x"})
	s.ClearConsole()
	core.AssertEqual(t, 0, len(s.consoleBuf))
}

func TestTools_Service_ClearErrors_Good_Direct(t *core.T) {
	s := &Service{}
	s.handleErrorEvent(map[string]any{"message": "x"})
	s.ClearErrors()
	core.AssertEqual(t, 0, len(s.errorBuf))
}

// ─── toolConsole / toolErrors / toolWindows ─────────────────────────

func TestTools_ToolConsole_Good_FiltersAndLimits(t *core.T) {
	s := &Service{}
	s.handleConsoleEvent(map[string]any{"level": "info", "message": "a"})
	s.handleConsoleEvent(map[string]any{"level": "info", "message": "b"})
	s.handleConsoleEvent(map[string]any{"level": "error", "message": "c"})
	resp := s.toolConsole(map[string]any{"level": "info", "limit": 1})
	core.AssertEqual(t, true, resp["ok"])
	core.AssertEqual(t, 1, resp["count"])
}

func TestTools_ToolErrors_Good(t *core.T) {
	s := &Service{}
	s.handleErrorEvent(map[string]any{"message": "x"})
	resp := s.toolErrors(map[string]any{})
	core.AssertEqual(t, true, resp["ok"])
	core.AssertEqual(t, 1, resp["count"])
}

func TestTools_ToolWindows_Good(t *core.T) {
	s, c := windowHarness(t)
	openWindow(t, c, "tray")
	resp := s.toolWindows()
	core.AssertEqual(t, true, resp["ok"])
	core.AssertEqual(t, 1, resp["count"])
}

func TestTools_ToolWindows_Bad_NoWindowService(t *core.T) {
	c := core.New()
	s := &Service{ServiceRuntime: core.NewServiceRuntime[Options](c, Options{}), port: 9999}
	resp := s.toolWindows()
	core.AssertEqual(t, false, resp["ok"])
}

// ─── Service Wails-table shims ──────────────────────────────────────

func TestTools_Service_ServiceName_Good_Direct(t *core.T) {
	s := &Service{}
	core.AssertEqual(t, "Bridge", s.ServiceName())
}

func TestTools_Service_ServiceStartup_Good_Direct(t *core.T) {
	s := &Service{}
	r := s.ServiceStartup(core.Background(), nil)
	core.AssertTrue(t, r.OK)
}

func TestTools_Service_ServiceShutdown_Good_Direct(t *core.T) {
	s := &Service{}
	r := s.ServiceShutdown()
	core.AssertTrue(t, r.OK)
}

// ─── paramBool ──────────────────────────────────────────────────────

func TestTools_ParamBool_Good(t *core.T) {
	core.AssertEqual(t, true, paramBool(map[string]any{"k": true}, "k", false))
}

func TestTools_ParamBool_Bad_MissingKeyUsesDefault(t *core.T) {
	core.AssertEqual(t, true, paramBool(map[string]any{}, "k", true))
}

func TestTools_ParamBool_Ugly_WrongTypeUsesDefault(t *core.T) {
	core.AssertEqual(t, false, paramBool(map[string]any{"k": "not-a-bool"}, "k", false))
}

// ─── dispatch sweep ──────────────────────────────────────────────────
//
// dispatch() is a pure switch/delegate — every case is exercised by
// routing every name in toolCatalogue() through it against a fully
// wired mega-harness (real window/screen/clipboard/process/sandbox
// services + a scripted window.eval_js action). Each tool's own
// Good/Bad/Ugly behaviour is already proven in its dedicated test
// file (file_test.go, window_test.go, ...); this sweep's job is only
// to prove dispatch reaches every one of those tools, including the
// tool-name aliases sharing a case block ("window_maximize" shares
// its block with "window_maximise", etc — Go's cover tool marks a
// whole case block covered from any one matching literal).

// dispatchSweepHarness wires every backend dispatch() can reach:
// window + screen (mock-backed), clipboard (stub-backed), a real
// process.Service, the real sandbox.Service, and a scripted
// window.eval_js action for every webview_*/eval-shaped tool.
func dispatchSweepHarness(t *core.T) *Service {
	t.Helper()
	homeFixture(t)
	c := core.New()

	rw := guiwindow.Register(guiwindow.NewMockPlatform())(c)
	core.AssertTrue(t, rw.OK)
	core.AssertTrue(t, rw.Value.(*guiwindow.Service).OnStartup(core.Background()).OK)

	rs := guiscreen.Register(&stubScreenPlatform{screens: []guiscreen.Screen{
		{
			ID: "primary", Name: "Built-in", IsPrimary: true,
			Bounds:   guiscreen.Rect{X: 0, Y: 0, Width: 1920, Height: 1080},
			WorkArea: guiscreen.Rect{X: 0, Y: 25, Width: 1920, Height: 1055},
		},
	}})(c)
	core.AssertTrue(t, rs.OK)
	core.AssertTrue(t, rs.Value.(*guiscreen.Service).OnStartup(core.Background()).OK)

	rc := guiclipboard.Register(&stubClipboardPlatform{text: "x", hasText: true})(c)
	core.AssertTrue(t, rc.OK)
	core.AssertTrue(t, rc.Value.(*guiclipboard.Service).OnStartup(core.Background()).OK)

	rp := process.NewService(process.Options{})(c)
	core.AssertTrue(t, rp.OK)
	ps := rp.Value.(*process.Service)
	core.AssertTrue(t, ps.OnStartup(core.Background()).OK)
	core.AssertTrue(t, c.RegisterService("process", ps).OK)

	rsb := sandbox.Register(c)
	core.AssertTrue(t, rsb.OK)
	core.AssertTrue(t, c.RegisterService("sandbox", rsb.Value).OK)

	c.Action("window.eval_js", func(_ core.Context, _ core.Options) core.Result {
		return core.Ok(guiwindow.EvalJSResult{Result: "scripted-result"})
	})

	s := &Service{ServiceRuntime: core.NewServiceRuntime[Options](c, Options{}), port: 9999}
	openWindow(t, c, "tray")
	openWindow(t, c, "chat")
	return s
}

func TestTools_Dispatch_Good_EveryCatalogueToolReachable(t *core.T) {
	s := dispatchSweepHarness(t)
	// Kitchen-sink params: a single map carrying every field name any
	// catalogue tool reads, so as many tools as possible clear their
	// param-validation guard and actually reach their backend call
	// (dispatch's own delegate line is covered either way, but this
	// also reinforces the downstream tool's success branch).
	params := map[string]any{
		"window": "tray", "script": "1+1", "selector": "body", "url": "https://example.invalid",
		"name": "tray", "id": "primary", "level": "info", "limit": 5,
		"text": "hi", "image": "ghcr.io/owner/img:1.0", "command": "echo",
		"workflow": "default", "position": "left", "mode": "grid",
		"x": 10, "y": 10, "width": 100, "height": 100,
		"r": 10, "g": 10, "b": 10, "a": 255,
		"offsetX": 10, "offsetY": 10, "enabled": true, "visible": true,
		"checked": true, "value": "v", "maxDepth": 2, "duration": 100,
		"find": "a", "replace": "b", "content": "hello", "input": "hi",
		"title": "T", "windows": []any{"tray"},
	}
	ctx := core.Background()
	for _, tool := range toolCatalogue() {
		name, ok := tool["name"].(string)
		core.AssertTrue(t, ok)
		resp := s.dispatch(ctx, name, params)
		core.AssertNotNil(t, resp, "dispatch(%q) must not return a nil response", name)
		if _, hasOK := resp["ok"]; !hasOK {
			t.Fatalf("dispatch(%q) response missing ok field: %#v", name, resp)
		}
	}
}

func TestTools_Dispatch_Bad_UnknownTool(t *core.T) {
	s := &Service{}
	resp := s.dispatch(core.Background(), "no_such_tool", nil)
	core.AssertEqual(t, false, resp["ok"])
	core.AssertContains(t, resp["error"], "unknown tool")
}
