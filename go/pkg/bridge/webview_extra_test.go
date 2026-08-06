// SPDX-Licence-Identifier: EUPL-1.2

// webview_extra.go + tools.go's eval() tests. Every tool here builds
// a JS one-liner and hands it to s.eval, which dispatches through
// s.Core().Action("window.eval_js"). Rather than driving a live
// WebView, these tests register a fake "window.eval_js" action on a
// bare core.New() that inspects the generated JS body and returns a
// scripted guiwindow.EvalJSResult — this exercises the bridge's own
// param-marshalling + response-shaping logic (the part that is
// actually bridge.go's, not core/gui's) without a Wails runtime.

package bridge

import (
	core "dappco.re/go"
	guiwindow "dappco.re/go/render/display/webkit/pkg/window"
)

// Security note: s.eval below is not Go/JS eval() of untrusted input —
// it is the bridge's own pre-existing method (tools.go) that dispatches
// a fixed, developer-authored JS template into the loopback-only,
// dev-mode WebView via Wails' TaskEvalJS. These tests call that
// existing production method directly; no new code-execution surface
// is introduced here.

// evalCall records one intercepted window.eval_js dispatch.
type evalCall struct {
	Name string
	JS   string
}

// evalHarness wires a *Service to a bare Core with a scripted
// "window.eval_js" action. JS bodies containing the sentinel
// substrings below drive the failure branches; anything else
// succeeds with evalResultValue as the JS return value.
const (
	forceEvalJSError  = "__FORCE_EVAL_JS_ERROR__"
	forceActionFailed = "__FORCE_ACTION_FAILED__"
)

func evalHarness(t *core.T) (*Service, *[]evalCall) {
	t.Helper()
	calls := &[]evalCall{}
	c := core.New()
	c.Action("window.eval_js", func(_ core.Context, opts core.Options) core.Result {
		task, _ := opts.Get("task").Value.(guiwindow.TaskEvalJS)
		*calls = append(*calls, evalCall{Name: task.Name, JS: task.JS})
		if core.Contains(task.JS, forceActionFailed) {
			return core.Fail(core.E("test.eval", "simulated action failure", nil))
		}
		if core.Contains(task.JS, forceEvalJSError) {
			return core.Ok(guiwindow.EvalJSResult{Err: "element not found"})
		}
		return core.Ok(guiwindow.EvalJSResult{Result: "scripted-result"})
	})
	s := &Service{ServiceRuntime: core.NewServiceRuntime[Options](c, Options{}), port: 9999}
	return s, calls
}

// ─── tools.go eval() itself ─────────────────────────────────────────

func TestTools_Eval_Good(t *core.T) {
	s, calls := evalHarness(t)
	resp := s.eval(core.Background(), "tray", "1+1")
	core.AssertEqual(t, true, resp["ok"])
	core.AssertEqual(t, "scripted-result", resp["value"])
	core.AssertEqual(t, 1, len(*calls))
	core.AssertEqual(t, "tray", (*calls)[0].Name)
}

func TestTools_Eval_Bad_EmptyScriptRequired(t *core.T) {
	s := &Service{}
	resp := s.eval(core.Background(), "tray", "")
	core.AssertEqual(t, false, resp["ok"])
	core.AssertEqual(t, "script param required", resp["error"])
}

func TestTools_Eval_Bad_ActionRunFails(t *core.T) {
	s, _ := evalHarness(t)
	resp := s.eval(core.Background(), "tray", forceActionFailed)
	core.AssertEqual(t, false, resp["ok"])
	core.AssertNotEmpty(t, resp["error"])
}

func TestTools_Eval_Ugly_JSSideError(t *core.T) {
	s, _ := evalHarness(t)
	resp := s.eval(core.Background(), "tray", forceEvalJSError)
	core.AssertEqual(t, false, resp["ok"])
	core.AssertEqual(t, "element not found", resp["error"])
}

func TestTools_Eval_Ugly_UnexpectedResultShape(t *core.T) {
	c := core.New()
	c.Action("window.eval_js", func(_ core.Context, _ core.Options) core.Result {
		return core.Ok("not-an-evaljsresult")
	})
	s := &Service{ServiceRuntime: core.NewServiceRuntime[Options](c, Options{}), port: 9999}
	resp := s.eval(core.Background(), "tray", "1+1")
	core.AssertEqual(t, false, resp["ok"])
	core.AssertEqual(t, "eval_js returned unexpected shape", resp["error"])
}

// ─── webview_extra.go tools ─────────────────────────────────────────

func TestWebviewExtra_ToolWebviewHover_Good(t *core.T) {
	s, calls := evalHarness(t)
	resp := s.toolWebviewHover(core.Background(), map[string]any{"selector": "#btn"})
	core.AssertEqual(t, true, resp["ok"])
	core.AssertContains(t, (*calls)[0].JS, "mouseover")
}

func TestWebviewExtra_ToolWebviewType_Good(t *core.T) {
	s, calls := evalHarness(t)
	resp := s.toolWebviewType(core.Background(), map[string]any{"selector": "#in", "value": "hi there"})
	core.AssertEqual(t, true, resp["ok"])
	core.AssertContains(t, (*calls)[0].JS, "hi there")
}

func TestWebviewExtra_ToolWebviewCheck_Good_Checked(t *core.T) {
	s, calls := evalHarness(t)
	resp := s.toolWebviewCheck(core.Background(), map[string]any{"selector": "#cb", "checked": true})
	core.AssertEqual(t, true, resp["ok"])
	core.AssertContains(t, (*calls)[0].JS, "el.checked=true")
}

func TestWebviewExtra_ToolWebviewCheck_Ugly_Unchecked(t *core.T) {
	s, calls := evalHarness(t)
	resp := s.toolWebviewCheck(core.Background(), map[string]any{"selector": "#cb", "checked": false})
	core.AssertEqual(t, true, resp["ok"])
	core.AssertContains(t, (*calls)[0].JS, "el.checked=false")
}

func TestWebviewExtra_ToolWebviewSelect_Good(t *core.T) {
	s, calls := evalHarness(t)
	resp := s.toolWebviewSelect(core.Background(), map[string]any{"selector": "#sel", "value": "opt-1"})
	core.AssertEqual(t, true, resp["ok"])
	core.AssertContains(t, (*calls)[0].JS, "opt-1")
}

func TestWebviewExtra_ToolWebviewScroll_Good_BySelector(t *core.T) {
	s, calls := evalHarness(t)
	resp := s.toolWebviewScroll(core.Background(), map[string]any{"selector": "#target"})
	core.AssertEqual(t, true, resp["ok"])
	core.AssertContains(t, (*calls)[0].JS, "scrollIntoView")
}

func TestWebviewExtra_ToolWebviewScroll_Ugly_ByCoords(t *core.T) {
	s, calls := evalHarness(t)
	resp := s.toolWebviewScroll(core.Background(), map[string]any{"x": 10, "y": 20})
	core.AssertEqual(t, true, resp["ok"])
	core.AssertContains(t, (*calls)[0].JS, "scrollTo")
}

func TestWebviewExtra_ToolWebviewDOMTree_Good(t *core.T) {
	s, calls := evalHarness(t)
	resp := s.toolWebviewDOMTree(core.Background(), map[string]any{"selector": "body", "maxDepth": 3})
	core.AssertEqual(t, true, resp["ok"])
	core.AssertContains(t, (*calls)[0].JS, "walk(root,3)")
}

func TestWebviewExtra_ToolWebviewSource_Good(t *core.T) {
	s, calls := evalHarness(t)
	resp := s.toolWebviewSource(core.Background(), map[string]any{})
	core.AssertEqual(t, true, resp["ok"])
	core.AssertContains(t, (*calls)[0].JS, "outerHTML")
}

func TestWebviewExtra_ToolWebviewComputedStyle_Good_NoProps(t *core.T) {
	s, calls := evalHarness(t)
	resp := s.toolWebviewComputedStyle(core.Background(), map[string]any{"selector": "#x"})
	core.AssertEqual(t, true, resp["ok"])
	core.AssertContains(t, (*calls)[0].JS, "picks=null")
}

func TestWebviewExtra_ToolWebviewComputedStyle_Ugly_WithProps(t *core.T) {
	s, calls := evalHarness(t)
	resp := s.toolWebviewComputedStyle(core.Background(), map[string]any{
		"selector": "#x",
		"props":    []any{"color", "display"},
	})
	core.AssertEqual(t, true, resp["ok"])
	core.AssertContains(t, (*calls)[0].JS, `["color","display"]`)
}

func TestWebviewExtra_ToolWebviewElementInfo_Good(t *core.T) {
	s, calls := evalHarness(t)
	resp := s.toolWebviewElementInfo(core.Background(), map[string]any{"selector": "#x"})
	core.AssertEqual(t, true, resp["ok"])
	core.AssertContains(t, (*calls)[0].JS, "getBoundingClientRect")
}

func TestWebviewExtra_ToolWebviewHighlight_Good(t *core.T) {
	s, calls := evalHarness(t)
	resp := s.toolWebviewHighlight(core.Background(), map[string]any{"selector": "#x", "duration": 500})
	core.AssertEqual(t, true, resp["ok"])
	core.AssertContains(t, (*calls)[0].JS, "},500)")
}

func TestWebviewExtra_ToolWebviewConsoleClear_Good(t *core.T) {
	s := &Service{}
	s.consoleBuf = []ConsoleEntry{{Level: "info", Message: "x"}}
	resp := s.toolWebviewConsoleClear()
	core.AssertEqual(t, true, resp["ok"])
	core.AssertEqual(t, "console", resp["cleared"])
	core.AssertEqual(t, 0, len(s.consoleBuf))
}

func TestWebviewExtra_ToolWebviewPerformance_Good(t *core.T) {
	s, calls := evalHarness(t)
	resp := s.toolWebviewPerformance(core.Background(), map[string]any{"window": "chat"})
	core.AssertEqual(t, true, resp["ok"])
	core.AssertEqual(t, "chat", (*calls)[0].Name)
}

func TestWebviewExtra_ToolWebviewElementInfo_Ugly_NotFound(t *core.T) {
	s, _ := evalHarness(t)
	resp := s.toolWebviewElementInfo(core.Background(), map[string]any{"selector": forceEvalJSError})
	core.AssertEqual(t, false, resp["ok"])
}

// ─── misc.go's eval-backed tool ─────────────────────────────────────

func TestMisc_ToolThemeGet_Good(t *core.T) {
	s, calls := evalHarness(t)
	resp := s.toolThemeGet(core.Background(), map[string]any{})
	core.AssertEqual(t, true, resp["ok"])
	core.AssertContains(t, (*calls)[0].JS, "prefers-color-scheme")
	core.AssertEqual(t, DefaultWindow, (*calls)[0].Name)
}
