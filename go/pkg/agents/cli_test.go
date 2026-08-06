// SPDX-Licence-Identifier: EUPL-1.2

// Internal-package tests for cli.go's remaining unexercised surface:
// flag/intFlag, run()'s two guard branches, and every Wails-facing
// method's request-validation + argument-assembly logic (Prep,
// Workspaces, Dispatch, Personas, Tasks, Resume, Scan).
//
// lthn-agent is a real external binary this package shells out to —
// never invoked here. Every method funnels through run(), and run()
// fails cleanly at its first guard when s.core is nil ("core not
// bound") — before resolveAgentBinary or process.Service is ever
// touched, let alone a real spawn. Building a request with every
// optional field populated and pointing it at a core-less Service
// exercises the full flag/intFlag argument-assembly chain (the actual
// unit of behaviour these methods own) while guaranteeing run() bails
// out before any process ever starts. The methods' success tails
// (parseXResult on real stdout) are already covered directly in
// agents_test.go via fixture JSON — no spawn needed there either.
package agents

import (
	core "dappco.re/go"
	"dappco.re/go/process"
)

// ─── flag / intFlag (pure) ────────────────────────────────────────────────

func TestCli_Flag_Good_AppendsWhenNonEmpty(t *core.T) {
	got := flag([]string{"base"}, "org", "core")
	core.AssertEqual(t, []string{"base", "--org=core"}, got)
}

func TestCli_Flag_Bad_SkipsWhenEmpty(t *core.T) {
	got := flag([]string{"base"}, "org", "")
	core.AssertEqual(t, []string{"base"}, got)
}

func TestCli_Flag_Ugly_SkipsWhenWhitespaceOnly(t *core.T) {
	got := flag([]string{"base"}, "org", "   ")
	core.AssertEqual(t, []string{"base"}, got)
}

func TestCli_IntFlag_Good_AppendsWhenNonZero(t *core.T) {
	got := intFlag([]string{"base"}, "issue", 15)
	core.AssertEqual(t, []string{"base", "--issue=15"}, got)
}

func TestCli_IntFlag_Bad_SkipsWhenZero(t *core.T) {
	got := intFlag([]string{"base"}, "issue", 0)
	core.AssertEqual(t, []string{"base"}, got)
}

func TestCli_IntFlag_Ugly_NegativeStillAppends(t *core.T) {
	got := intFlag([]string{"base"}, "limit", -1)
	core.AssertEqual(t, []string{"base", "--limit=-1"}, got)
}

// ─── resolveAgentBinary ────────────────────────────────────────────────────

// TestCli_ResolveAgentBinary_Good_ExplicitOverrideWins is the one
// deterministic branch: LTHN_AGENT_BIN short-circuits before any of the
// (environment-dependent) candidate-path checks or the final PATH
// fallback run.
func TestCli_ResolveAgentBinary_Good_ExplicitOverrideWins(t *core.T) {
	t.Setenv("LTHN_AGENT_BIN", "/custom/path/to/lthn-agent")
	core.AssertEqual(t, "/custom/path/to/lthn-agent", resolveAgentBinary())
}

// TestCli_ResolveAgentBinary_Good_FoundViaLetheanBinCandidate plants the
// binary at the first of the three fixed candidate paths under a
// controlled $HOME, deterministically hitting the "found" return
// without depending on real host installs.
func TestCli_ResolveAgentBinary_Good_FoundViaLetheanBinCandidate(t *core.T) {
	t.Setenv("LTHN_AGENT_BIN", "")
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	dir := tmp + "/Lethean/bin"
	core.RequireTrue(t, core.MkdirAll(dir, 0o755).OK)
	core.RequireTrue(t, core.WriteFile(dir+"/lthn-agent", []byte("#!/bin/sh\n"), 0o755).OK)

	core.AssertEqual(t, dir+"/lthn-agent", resolveAgentBinary())
}

// TestCli_ResolveAgentBinary_Ugly_NoCandidatesFallsBackToBareName is the
// opposite: none of the three fixed candidates exist under the
// controlled $HOME, so resolveAgentBinary falls all the way through to
// the bare "lthn-agent" name (the spawn env's PATH must then carry it).
func TestCli_ResolveAgentBinary_Ugly_NoCandidatesFallsBackToBareName(t *core.T) {
	t.Setenv("LTHN_AGENT_BIN", "")
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	core.AssertEqual(t, agentBinaryName, resolveAgentBinary())
}

// ─── run ───────────────────────────────────────────────────────────────────

func TestCli_run_Bad_CoreNotBound(t *core.T) {
	svc := &Service{}
	r := svc.run("workspace/list")
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "core not bound")
}

func TestCli_run_Bad_ProcessServiceUnavailable(t *core.T) {
	svc := &Service{core: core.New()} // no process.Register
	r := svc.run("workspace/list")
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "process service unavailable")
}

// TestCli_run_Bad_SpawnFailsMissingExecutable is real fault injection —
// the same "missing executable" pattern used against pkg/plugin's
// process.Service.StartWithOptions: LTHN_AGENT_BIN points resolveAgent
// Binary at a path that doesn't exist, so process.Service.Run's
// underlying cmd.Start() fails synchronously (ENOENT). Nothing ever
// executes; this exercises run()'s proc.Run call + its !r.OK guard
// without spawning lthn-agent for real.
func TestCli_run_Bad_SpawnFailsMissingExecutable(t *core.T) {
	tmp := t.TempDir()
	t.Setenv("LTHN_AGENT_BIN", tmp+"/does-not-exist/lthn-agent")

	svc := &Service{core: core.New(core.WithService(process.Register))}
	r := svc.run("workspace/list")
	core.AssertFalse(t, r.OK)
}

// ─── Prep ────────────────────────────────────────────────────────────────

func TestCli_Prep_Bad_RepoRequired(t *core.T) {
	svc := &Service{}
	r := svc.Prep(PrepRequest{})
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "repo is required")
}

// TestCli_Prep_Ugly_FullArgumentAssemblyThenRunFails populates every
// optional field so flag()/intFlag() build the complete argv, then lets
// run() fail cleanly (no core bound) before any process starts.
func TestCli_Prep_Ugly_FullArgumentAssemblyThenRunFails(t *core.T) {
	svc := &Service{}
	r := svc.Prep(PrepRequest{
		Repo: "go-io", Org: "core", Task: "fix tests", Agent: "codex",
		Issue: 15, PR: 3, Branch: "agent/x", Tag: "v1",
		Template: "tmpl", PlanTemplate: "plan", Persona: "senior-developer",
	})
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "core not bound")
}

// ─── Workspaces ────────────────────────────────────────────────────────────

func TestCli_Workspaces_Bad_PropagatesRunFailure(t *core.T) {
	svc := &Service{}
	r := svc.Workspaces()
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "core not bound")
}

// ─── Dispatch ──────────────────────────────────────────────────────────────

func TestCli_Dispatch_Bad_RepoAndTaskRequired(t *core.T) {
	svc := &Service{}
	core.AssertFalse(t, svc.Dispatch(DispatchRequest{}).OK)
	core.AssertFalse(t, svc.Dispatch(DispatchRequest{Repo: "go-io"}).OK)
	core.AssertFalse(t, svc.Dispatch(DispatchRequest{Task: "fix"}).OK)
}

func TestCli_Dispatch_Ugly_FullArgumentAssemblyThenRunFails(t *core.T) {
	svc := &Service{}
	r := svc.Dispatch(DispatchRequest{
		Repo: "go-io", Org: "core", Task: "fix tests", Agent: "codex",
		Issue: 15, PR: 3, Branch: "agent/x", Template: "tmpl",
		PlanTemplate: "plan", Persona: "senior-developer", Tag: "v1",
		DryRun: true,
	})
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "core not bound")
}

// ─── Personas / Tasks ────────────────────────────────────────────────────

func TestCli_Personas_Bad_PropagatesRunFailure(t *core.T) {
	svc := &Service{}
	r := svc.Personas()
	core.AssertFalse(t, r.OK)
}

func TestCli_Tasks_Bad_PropagatesRunFailure(t *core.T) {
	svc := &Service{}
	r := svc.Tasks()
	core.AssertFalse(t, r.OK)
}

// ─── Resume ────────────────────────────────────────────────────────────────

func TestCli_Resume_Bad_WorkspaceRequired(t *core.T) {
	svc := &Service{}
	r := svc.Resume(ResumeRequest{})
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "workspace is required")
}

func TestCli_Resume_Ugly_FullArgumentAssemblyThenRunFails(t *core.T) {
	svc := &Service{}
	r := svc.Resume(ResumeRequest{
		Workspace: "core/go-io/task-4", Answer: "use the shared notifier",
		Agent: "codex", DryRun: true,
	})
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "core not bound")
}

// ─── Scan ──────────────────────────────────────────────────────────────────

func TestCli_Scan_Bad_PropagatesRunFailure(t *core.T) {
	svc := &Service{}
	r := svc.Scan(ScanRequest{})
	core.AssertFalse(t, r.OK)
}

func TestCli_Scan_Ugly_FullArgumentAssemblyThenRunFails(t *core.T) {
	svc := &Service{}
	r := svc.Scan(ScanRequest{Org: "core", Labels: []string{"agentic", "bug"}, Limit: 5})
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "core not bound")
}
