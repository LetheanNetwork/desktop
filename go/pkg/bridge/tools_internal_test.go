// SPDX-Licence-Identifier: EUPL-1.2

// Internal-package tests for tools.go HTTP handlers — Cerberus
// H#9-verify F2 (Mantis #1535). Handlers and helpers are unexported,
// so tests live in package bridge rather than bridge_test.

package bridge

import (
	core "dappco.re/go"
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
