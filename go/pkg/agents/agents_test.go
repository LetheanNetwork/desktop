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
