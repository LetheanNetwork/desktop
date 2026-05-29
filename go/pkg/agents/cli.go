// SPDX-Licence-Identifier: EUPL-1.2

package agents

import (
	"bytes"

	core "dappco.re/go"
	"dappco.re/go/process"
)

// CLI adapter to the lthn-agent binary — the human / GUI lane. The desktop
// drives CoreAgent's agentic pipeline by shelling out to `lthn-agent <verb>
// --json` (the same shape pkg/calibrate uses for lthn-mlx), NOT the plugin's
// /v1/tools + /mcp serve. That serve lane is for the Claude/Codex harness
// (Cladius); a human on the GUI uses the CLI. It also fits the work: prep
// clones a repo + its deps and assembles the prompt synchronously — minutes,
// the long one-shot shape an HTTP request fought, a CLI call suits.

const (
	agentBinaryName = "lthn-agent"
	// envAgentBin points the adapter at a specific lthn-agent (e.g. a packaged
	// .app's bundled binary) without relying on the search path.
	envAgentBin = "LTHN_AGENT_BIN"
)

// resolveAgentBinary resolves the lthn-agent CLI: explicit override → installed
// (~/Lethean/bin) → desktop crew build → core/agent dev build → PATH. Mirrors
// pkg/calibrate.resolveBinary for lthn-mlx.
func resolveAgentBinary() string {
	if p := core.Trim(core.Env(envAgentBin)); p != "" {
		return p
	}
	home := core.Env("HOME")
	for _, c := range []string{
		core.PathJoin(home, "Lethean", "bin", agentBinaryName),
		core.PathJoin(home, "Code", "lthn", "desktop", "build", "darwin", "bin", agentBinaryName),
		core.PathJoin(home, "Code", "core", "agent", "bin", agentBinaryName),
	} {
		if core.Stat(c).OK {
			return c
		}
	}
	return agentBinaryName
}

// run executes `lthn-agent <args...> --json` and returns stdout on Ok. --json
// is appended here so callers pass only the verb + flags.
func (s *Service) run(args ...string) core.Result {
	if s.core == nil {
		core.Warn("agents.run: core not bound")
		return core.Fail(core.NewError("agents: core not bound"))
	}
	proc, ok := core.ServiceFor[*process.Service](s.core, "process")
	if !ok || proc == nil {
		core.Warn("agents.run: process service unavailable", "ok", ok)
		return core.Fail(core.NewError("agents: process service unavailable"))
	}
	bin := resolveAgentBinary()
	r := proc.Run(core.Background(), bin, append(args, "--json")...)
	if !r.OK {
		core.Warn("agents.run: proc.Run failed", "bin", bin, "args", args, "err", r.Error())
		return core.Fail(core.E("agents.run", r.Error(), nil))
	}
	out, _ := r.Value.(string)
	return core.Ok(jsonLine(out))
}

// jsonLine pulls the JSON payload out of the captured output. proc.Run merges
// the process's stderr into the result, and lthn-agent boots noisy subsystems
// (brain, monitor) whose log lines land on the stream before the JSON. The
// JSON is emitted by core.Print as a single line, so the payload is the last
// line that starts with [ or { — everything above it is log noise.
func jsonLine(raw string) string {
	lines := bytes.Split([]byte(raw), []byte("\n"))
	for i := len(lines) - 1; i >= 0; i-- {
		t := bytes.TrimSpace(lines[i])
		if len(t) > 0 && (t[0] == '[' || t[0] == '{') {
			return string(t)
		}
	}
	return core.Trim(raw)
}

// flag appends --key=val when val is non-empty; intFlag when n is non-zero.
func flag(args []string, key, val string) []string {
	if core.Trim(val) != "" {
		return append(args, "--"+key+"="+val)
	}
	return args
}

func intFlag(args []string, key string, n int) []string {
	if n != 0 {
		return append(args, "--"+key+"="+core.Itoa(n))
	}
	return args
}

// --- prep ---

// PrepRequest mirrors lthn-agent prep's flags. Repo is the only required field
// (positional <repo>); the rest refine the assembled prompt.
type PrepRequest struct {
	Repo         string `json:"repo"`
	Org          string `json:"org,omitempty"`
	Task         string `json:"task,omitempty"`
	Agent        string `json:"agent,omitempty"`
	Issue        int    `json:"issue,omitempty"`
	PR           int    `json:"pr,omitempty"`
	Branch       string `json:"branch,omitempty"`
	Tag          string `json:"tag,omitempty"`
	Template     string `json:"template,omitempty"`
	PlanTemplate string `json:"plan_template,omitempty"`
	Persona      string `json:"persona,omitempty"`
}

// PrepResult mirrors prep --json (core/agent PrepOutput): the prepared
// workspace + the assembled, version-snapshotted prompt, with counts of the
// brain memories recalled and downstream consumers found during assembly.
type PrepResult struct {
	Success       bool   `json:"success"`
	WorkspaceDir  string `json:"workspace_dir"`
	RepoDir       string `json:"repo_dir"`
	Branch        string `json:"branch"`
	Prompt        string `json:"prompt,omitempty"`
	PromptVersion string `json:"prompt_version,omitempty"`
	Memories      int    `json:"memories"`
	Consumers     int    `json:"consumers"`
	Resumed       bool   `json:"resumed"`
}

// Prep runs `lthn-agent prep <repo> [flags] --json` — clone/resume the repo,
// branch, and assemble the agent prompt (issue body + brain recall + consumer
// impact + wiki + git log), versioned to a snapshot. Repo is required.
//
//	r := svc.Prep(agents.PrepRequest{Repo: "go-io", Issue: 15})
//	if r.OK { out := r.Value.(agents.PrepResult); _ = out }
func (s *Service) Prep(req PrepRequest) core.Result {
	if core.Trim(req.Repo) == "" {
		return core.Fail(core.NewError("agents.Prep: repo is required"))
	}
	args := []string{"prep", req.Repo}
	args = flag(args, "org", req.Org)
	args = flag(args, "task", req.Task)
	args = flag(args, "agent", req.Agent)
	args = flag(args, "branch", req.Branch)
	args = flag(args, "template", req.Template)
	args = flag(args, "plan-template", req.PlanTemplate)
	args = flag(args, "persona", req.Persona)
	args = flag(args, "tag", req.Tag)
	args = intFlag(args, "issue", req.Issue)
	args = intFlag(args, "pr", req.PR)
	r := s.run(args...)
	if !r.OK {
		return r
	}
	return parsePrepResult(r.Value.(string))
}

// parsePrepResult decodes prep --json stdout. Split out so it's unit-testable
// without spawning the binary (pkg/calibrate's pattern).
func parsePrepResult(stdout string) core.Result {
	var out PrepResult
	if pr := core.JSONUnmarshalString(stdout, &out); !pr.OK {
		return core.Fail(core.E("agents.Prep", "parse prep output: "+pr.Error(), nil))
	}
	return core.Ok(out)
}

// --- workspace list ---

// Workspace mirrors one row of workspace/list --json — a tracked agent
// workspace and its current state.
type Workspace struct {
	Name     string `json:"name"`
	Status   string `json:"status"`
	Agent    string `json:"agent"`
	Repo     string `json:"repo"`
	Org      string `json:"org,omitempty"`
	Task     string `json:"task,omitempty"`
	Branch   string `json:"branch,omitempty"`
	Issue    int    `json:"issue,omitempty"`
	Question string `json:"question,omitempty"`
	Runs     int    `json:"runs"`
	PRURL    string `json:"pr_url,omitempty"`
}

// Workspaces runs `lthn-agent workspace/list --json` — every tracked workspace
// with its status, the full feed the Activity panel renders. Fails cleanly
// when lthn-agent isn't reachable so the panel shows "engine down".
//
//	r := svc.Workspaces()
//	if r.OK { ws := r.Value.([]agents.Workspace); _ = ws }
func (s *Service) Workspaces() core.Result {
	r := s.run("workspace/list")
	if !r.OK {
		return r
	}
	return parseWorkspaces(r.Value.(string))
}

func parseWorkspaces(stdout string) core.Result {
	var out []Workspace
	if pr := core.JSONUnmarshalString(stdout, &out); !pr.OK {
		return core.Fail(core.E("agents.Workspaces", "parse workspace list: "+pr.Error(), nil))
	}
	return core.Ok(out)
}

// --- dispatch ---

// DispatchRequest mirrors workspace/dispatch's flags. Repo + Task are the
// essentials; Agent picks the harness.
type DispatchRequest struct {
	Repo     string `json:"repo"`
	Org      string `json:"org,omitempty"`
	Task     string `json:"task"`
	Agent    string `json:"agent,omitempty"`
	Issue    int    `json:"issue,omitempty"`
	PR       int    `json:"pr,omitempty"`
	Branch   string `json:"branch,omitempty"`
	Template string `json:"template,omitempty"`
	Persona  string `json:"persona,omitempty"`
	Tag      string `json:"tag,omitempty"`
	DryRun   bool   `json:"dry_run,omitempty"`
}

// DispatchResult mirrors workspace/dispatch --json (core/agent DispatchOutput).
type DispatchResult struct {
	Success      bool   `json:"success"`
	Agent        string `json:"agent"`
	Repo         string `json:"repo"`
	WorkspaceDir string `json:"workspace_dir"`
	Prompt       string `json:"prompt,omitempty"`
	PID          int    `json:"pid,omitempty"`
	OutputFile   string `json:"output_file,omitempty"`
}

// Dispatch runs `lthn-agent workspace/dispatch <repo> --task=… [flags] --json`:
// prep the workspace then spawn the agent detached. Repo + Task required.
//
//	r := svc.Dispatch(agents.DispatchRequest{Repo: "go-io", Task: "fix tests", Agent: "codex"})
//	if r.OK { out := r.Value.(agents.DispatchResult); _ = out }
func (s *Service) Dispatch(req DispatchRequest) core.Result {
	if core.Trim(req.Repo) == "" || core.Trim(req.Task) == "" {
		return core.Fail(core.NewError("agents.Dispatch: repo and task are required"))
	}
	args := []string{"workspace/dispatch", req.Repo}
	args = flag(args, "task", req.Task)
	args = flag(args, "org", req.Org)
	args = flag(args, "agent", req.Agent)
	args = flag(args, "branch", req.Branch)
	args = flag(args, "template", req.Template)
	args = flag(args, "persona", req.Persona)
	args = flag(args, "tag", req.Tag)
	args = intFlag(args, "issue", req.Issue)
	args = intFlag(args, "pr", req.PR)
	if req.DryRun {
		args = append(args, "--dry-run")
	}
	r := s.run(args...)
	if !r.OK {
		return r
	}
	return parseDispatchResult(r.Value.(string))
}

func parseDispatchResult(stdout string) core.Result {
	var out DispatchResult
	if pr := core.JSONUnmarshalString(stdout, &out); !pr.OK {
		return core.Fail(core.E("agents.Dispatch", "parse dispatch output: "+pr.Error(), nil))
	}
	return core.Ok(out)
}

// --- resume ---

// ResumeRequest mirrors resume's flags. Workspace is the blocked run's Name
// from Workspaces (positional <workspace>); Answer is written to ANSWER.md.
type ResumeRequest struct {
	Workspace string `json:"workspace"`
	Answer    string `json:"answer,omitempty"`
	Agent     string `json:"agent,omitempty"`
	DryRun    bool   `json:"dry_run,omitempty"`
}

// ResumeResult mirrors resume --json (core/agent ResumeOutput).
type ResumeResult struct {
	Success    bool   `json:"success"`
	Workspace  string `json:"workspace"`
	Agent      string `json:"agent"`
	PID        int    `json:"pid,omitempty"`
	OutputFile string `json:"output_file,omitempty"`
	Prompt     string `json:"prompt,omitempty"`
}

// Resume runs `lthn-agent resume <workspace> [--answer=…] [--agent=…] --json`:
// answer a blocked run and relaunch it. Workspace is required.
//
//	r := svc.Resume(agents.ResumeRequest{Workspace: "core/go-io/task-4", Answer: "use the shared notifier"})
//	if r.OK { out := r.Value.(agents.ResumeResult); _ = out }
func (s *Service) Resume(req ResumeRequest) core.Result {
	if core.Trim(req.Workspace) == "" {
		return core.Fail(core.NewError("agents.Resume: workspace is required"))
	}
	args := []string{"resume", req.Workspace}
	args = flag(args, "answer", req.Answer)
	args = flag(args, "agent", req.Agent)
	if req.DryRun {
		args = append(args, "--dry-run")
	}
	r := s.run(args...)
	if !r.OK {
		return r
	}
	return parseResumeResult(r.Value.(string))
}

func parseResumeResult(stdout string) core.Result {
	var out ResumeResult
	if pr := core.JSONUnmarshalString(stdout, &out); !pr.OK {
		return core.Fail(core.E("agents.Resume", "parse resume output: "+pr.Error(), nil))
	}
	return core.Ok(out)
}

// --- scan ---

// ScanRequest mirrors scan's flags — a Forge org + label filter.
type ScanRequest struct {
	Org    string   `json:"org,omitempty"`
	Labels []string `json:"labels,omitempty"`
	Limit  int      `json:"limit,omitempty"`
}

// ScanIssue mirrors one scan --json issue — a dispatch candidate.
type ScanIssue struct {
	Repo     string   `json:"repo"`
	Number   int      `json:"number"`
	Title    string   `json:"title"`
	Labels   []string `json:"labels"`
	Assignee string   `json:"assignee,omitempty"`
	URL      string   `json:"url"`
}

// scanResult mirrors scan --json (core/agent ScanOutput envelope).
type scanResult struct {
	Success bool        `json:"success"`
	Count   int         `json:"count"`
	Issues  []ScanIssue `json:"issues"`
}

// Scan runs `lthn-agent scan [--org=…] [--labels=a,b] --json` — open Forge
// issues matching the label filter, the dispatch candidates.
//
//	r := svc.Scan(agents.ScanRequest{Org: "core", Labels: []string{"agentic"}})
//	if r.OK { issues := r.Value.([]agents.ScanIssue); _ = issues }
func (s *Service) Scan(req ScanRequest) core.Result {
	args := []string{"scan"}
	args = flag(args, "org", req.Org)
	if len(req.Labels) > 0 {
		args = flag(args, "labels", core.Join(",", req.Labels...))
	}
	args = intFlag(args, "limit", req.Limit)
	r := s.run(args...)
	if !r.OK {
		return r
	}
	return parseScanResult(r.Value.(string))
}

func parseScanResult(stdout string) core.Result {
	var out scanResult
	if pr := core.JSONUnmarshalString(stdout, &out); !pr.OK {
		return core.Fail(core.E("agents.Scan", "parse scan output: "+pr.Error(), nil))
	}
	return core.Ok(out.Issues)
}
