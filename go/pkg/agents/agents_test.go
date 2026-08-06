// SPDX-Licence-Identifier: EUPL-1.2

package agents

import (
	core "dappco.re/go"
)

// The CLI ops shell out to lthn-agent; the unit-testable surface is the
// --json parsers (pkg/calibrate's pattern — the spawn is verified live).

// --- Prep (parsePrepResult) ---

func TestCli_Service_Prep_Good(t *core.T) {
	r := parsePrepResult(`{"success":true,"workspace_dir":".core/workspace/core/go-io/task-15","repo_dir":".core/workspace/core/go-io/task-15/src","branch":"agent/x","memories":3,"consumers":2,"prompt_version":"v1","prompt":"do the thing"}`)
	core.AssertTrue(t, r.OK)
	out := r.Value.(PrepResult)
	core.AssertEqual(t, 3, out.Memories)
	core.AssertEqual(t, 2, out.Consumers)
	core.AssertEqual(t, "v1", out.PromptVersion)
}

func TestCli_Service_Prep_Bad(t *core.T) {
	// Empty stdout (e.g. binary printed nothing) → parse fails, no panic.
	r := parsePrepResult("")
	core.AssertFalse(t, r.OK, "empty stdout must Fail")
}

func TestCli_Service_Prep_Ugly(t *core.T) {
	// Truncated/malformed JSON → Fail cleanly.
	r := parsePrepResult(`{"success":true,"memories":`)
	core.AssertFalse(t, r.OK)
}

// --- Workspaces (parseWorkspaces) ---

func TestCli_Service_Workspaces_Good(t *core.T) {
	r := parseWorkspaces(`[{"name":"core/go-io/task-4","status":"blocked","agent":"codex","repo":"go-io","question":"which API?","runs":1}]`)
	core.AssertTrue(t, r.OK)
	ws := r.Value.([]Workspace)
	core.AssertEqual(t, 1, len(ws))
	core.AssertEqual(t, "blocked", ws[0].Status)
	core.AssertEqual(t, "core/go-io/task-4", ws[0].Name)
}

func TestCli_Service_Workspaces_Bad(t *core.T) {
	// Empty stdout → Fail (an empty list comes through as "[]", not "").
	r := parseWorkspaces("")
	core.AssertFalse(t, r.OK, "empty stdout must Fail")
}

func TestCli_Service_Workspaces_Ugly(t *core.T) {
	// An object where a list is expected → Fail cleanly.
	r := parseWorkspaces(`{"not":"a list"}`)
	core.AssertFalse(t, r.OK)
}

// --- Dispatch (parseDispatchResult) ---

func TestCli_Service_Dispatch_Good(t *core.T) {
	r := parseDispatchResult(`{"success":true,"agent":"codex","repo":"go-io","workspace_dir":".core/workspace/core/go-io/task-1","pid":4321}`)
	core.AssertTrue(t, r.OK)
	out := r.Value.(DispatchResult)
	core.AssertEqual(t, "go-io", out.Repo)
	core.AssertEqual(t, 4321, out.PID)
}

func TestCli_Service_Dispatch_Bad(t *core.T) {
	r := parseDispatchResult("")
	core.AssertFalse(t, r.OK)
}

func TestCli_Service_Dispatch_Ugly(t *core.T) {
	r := parseDispatchResult(`{"pid": "not-an-int"}`)
	core.AssertFalse(t, r.OK)
}

// --- Resume (parseResumeResult) ---

func TestCli_Service_Resume_Good(t *core.T) {
	r := parseResumeResult(`{"success":true,"workspace":"core/go-io/task-4","agent":"codex","pid":99}`)
	core.AssertTrue(t, r.OK)
	out := r.Value.(ResumeResult)
	core.AssertEqual(t, "core/go-io/task-4", out.Workspace)
	core.AssertEqual(t, 99, out.PID)
}

func TestCli_Service_Resume_Bad(t *core.T) {
	r := parseResumeResult("")
	core.AssertFalse(t, r.OK)
}

func TestCli_Service_Resume_Ugly(t *core.T) {
	r := parseResumeResult(`{"workspace":`)
	core.AssertFalse(t, r.OK)
}

// --- Scan (parseScanResult — unwraps the envelope to []ScanIssue) ---

func TestCli_Service_Scan_Good(t *core.T) {
	r := parseScanResult(`{"success":true,"count":1,"issues":[{"repo":"go-io","number":15,"title":"fix tests","labels":["agentic"],"url":"https://forge.lthn.ai/core/go-io/issues/15"}]}`)
	core.AssertTrue(t, r.OK)
	issues := r.Value.([]ScanIssue)
	core.AssertEqual(t, 1, len(issues))
	core.AssertEqual(t, 15, issues[0].Number)
}

func TestCli_Service_Scan_Bad(t *core.T) {
	r := parseScanResult("")
	core.AssertFalse(t, r.OK)
}

func TestCli_Service_Scan_Ugly(t *core.T) {
	r := parseScanResult(`{"count":1,"issues":`)
	core.AssertFalse(t, r.OK)
}

// --- jsonLine (extract the JSON payload from merged stdout+stderr) ---

func TestCli_Service_JsonLine_Good(t *core.T) {
	// lthn-agent's boot logs land before the JSON (proc.Run merges stderr).
	raw := "07:19 [INF] brain loaded\n07:19 [INF] monitor started\n[{\"name\":\"x\",\"status\":\"blocked\"}]\n"
	core.AssertEqual(t, `[{"name":"x","status":"blocked"}]`, jsonLine(raw))
}

func TestCli_Service_JsonLine_Bad(t *core.T) {
	// No JSON line → returns the trimmed raw; the parser then Fails cleanly.
	core.AssertEqual(t, "just logs", jsonLine("  just logs  "))
}

func TestCli_Service_JsonLine_Ugly(t *core.T) {
	// An object payload (prep) after logs → extract the {…} line.
	raw := "07:19 [INF] loaded\n{\"success\":true,\"memories\":3}"
	core.AssertEqual(t, `{"success":true,"memories":3}`, jsonLine(raw))
}

// --- resolveAgentBinary (always returns a candidate, never empty) ---

func TestCli_Service_ResolveBinary_Good(t *core.T) {
	core.AssertTrue(t, resolveAgentBinary() != "", "resolveAgentBinary always returns a path or the bare name")
}

// --- agents.go: New / Register / ServiceName / ServiceStartup ---

func TestAgents_New_Good_DefaultsToDefaultMCPURL(t *core.T) {
	svc := New(Config{})
	core.AssertEqual(t, DefaultMCPURL, svc.mcpURL)
	core.AssertEqual(t, "", svc.mcpToken)
}

func TestAgents_New_Good_ExplicitMCPURLPreserved(t *core.T) {
	svc := New(Config{MCPURL: "http://127.0.0.1:9999", MCPToken: "tok"})
	core.AssertEqual(t, "http://127.0.0.1:9999", svc.mcpURL)
	core.AssertEqual(t, "tok", svc.mcpToken)
}

func TestAgents_New_Ugly_WhitespaceMCPURLFallsBackToDefault(t *core.T) {
	svc := New(Config{MCPURL: "   "})
	core.AssertEqual(t, DefaultMCPURL, svc.mcpURL)
}

func TestAgents_Register_Good(t *core.T) {
	c := core.New()
	r := Register(c)
	core.RequireTrue(t, r.OK)
	svc, ok := r.Value.(*Service)
	core.RequireTrue(t, ok)
	core.AssertSame(t, c, svc.core)
}

func TestAgents_Service_ServiceName_Good(t *core.T) {
	svc := New(Config{})
	core.AssertEqual(t, "Agents", svc.ServiceName())
}

func TestAgents_Service_ServiceStartup_Good_NoOp(t *core.T) {
	svc := New(Config{})
	r := svc.ServiceStartup(core.Background(), nil)
	core.AssertTrue(t, r.OK)
}

// --- agents.go: StartChannels / ServiceShutdown ---

func TestAgents_Service_ServiceShutdown_Good_NoListenerIsNoOp(t *core.T) {
	svc := New(Config{})
	r := svc.ServiceShutdown()
	core.AssertTrue(t, r.OK)
}

func TestAgents_Service_StartChannels_Bad_NilCoreIsNoOp(t *core.T) {
	svc := New(Config{})
	svc.StartChannels(nil, "tok")
	core.AssertNil(t, svc.listener, "nil core must not wire a listener")
}

// TestAgents_Service_StartChannels_Good_IdempotentAndStoppable points the
// listener at a real (loopback) fixture server per channels_test.go's
// channelTestServer, starts it, proves sync.Once makes a second call a
// no-op (mcpToken from the second call is NOT applied), then stops it
// via ServiceShutdown so the background goroutine doesn't outlive the
// test.
func TestAgents_Service_StartChannels_Good_IdempotentAndStoppable(t *core.T) {
	srv := channelTestServer("") // empty stream body — connects, drains, idles
	defer srv.Close()

	c := core.New()
	svc := New(Config{MCPURL: srv.URL})
	svc.StartChannels(c, "first-token")
	core.RequireTrue(t, svc.listener != nil)
	core.AssertEqual(t, "first-token", svc.mcpToken)

	svc.StartChannels(c, "second-token") // sync.Once — must not replace state
	core.AssertEqual(t, "first-token", svc.mcpToken, "second StartChannels call is a no-op")

	r := svc.ServiceShutdown()
	core.AssertTrue(t, r.OK)
}

// TestAgents_Service_StartChannels_Good_RelaysChannelEventViaEmitEvent
// proves the relay closure StartChannels wires up (channel listener ->
// gui.EmitEvent(c, "lthn:agents:channel", ...)) actually runs end to
// end against a real fixture channel notification. gui.EmitEvent calls
// c.Action("events.emit") — registering a real handler under that name
// gives a synchronous, race-free signal the instant the relay fires,
// rather than a blind sleep. dappco.re/go/render/display/webkit's
// EmitEvent needs no WebView to do this (it just dispatches a Core
// action), so this is fully hermetic.
func TestAgents_Service_StartChannels_Good_RelaysChannelEventViaEmitEvent(t *core.T) {
	const evt = "event: message\n" +
		`data: {"jsonrpc":"2.0","method":"notifications/claude/channel","params":{"channel":"agent.blocked","data":{}}}` +
		"\n\n"
	srv := channelTestServer(evt)
	defer srv.Close()

	c := core.New()
	relayed := make(chan string, 1)
	c.Action("events.emit", func(_ core.Context, opts core.Options) core.Result {
		relayed <- "fired"
		return core.Ok(nil)
	})

	svc := New(Config{MCPURL: srv.URL})
	svc.StartChannels(c, "")
	t.Cleanup(func() { _ = svc.ServiceShutdown() })

	select {
	case <-relayed:
	case <-core.After(2 * core.Second):
		t.Fatal("relay closure never called gui.EmitEvent within 2s")
	}
}
