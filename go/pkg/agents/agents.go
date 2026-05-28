// SPDX-Licence-Identifier: EUPL-1.2

// Package agents is the desktop's client to CoreAgent (lthn-agent) — the
// harness-agnostic orchestration layer that dispatches work across the
// fleet. It reaches a running `lthn-agent serve` over loopback HTTP via
// its BridgeToAPI REST surface (POST /v1/tools/<tool>) — the same
// binary-boundary discipline pkg/lemma uses for lthn-mlx: the crew
// (pkg/fleet) brings the engine up, this package drives it. CoreAgent's
// own run state lives under ~/Code/.core/ (not the master DuckDB), so the
// Agents view reads it through here, not through pkg/fleet.
//
// Wails service "Agents" — the Agents view's backend.
//
//	core.New(core.WithName("agents", agents.Register))
package agents

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"sync"
	"time"

	core "dappco.re/go"
	gui "dappco.re/go/gui"
)

// DefaultBaseURL is the loopback address the crew brings lthn-agent serve
// up on (MCP_HTTP_ADDR); mirrors the BridgeToAPI default 127.0.0.1:9101.
const DefaultBaseURL = "http://127.0.0.1:9101"

// Config configures the Service. Zero-value uses DefaultBaseURL, no auth.
type Config struct {
	// BaseURL is the lthn-agent serve loopback base; empty → DefaultBaseURL.
	BaseURL string
	// Token is an optional MCP_AUTH_TOKEN bearer; empty → unauthenticated
	// (the loopback default — serve runs open when MCP_AUTH_TOKEN is unset).
	Token string
}

// Service is the CoreAgent client. Tool calls are stateless request/response
// over the /v1/tools bridge (goroutine-safe); on top of that, ServiceStartup
// brings up a live channel listener that relays CoreAgent's push events to the
// Agents view (see channels.go).
type Service struct {
	baseURL string
	token   string
	client  *http.Client
	// core is captured at Register (and at StartChannels) so the channel
	// listener can emit desktop events; nil when constructed via New (unit
	// tests) → the listener stays off.
	core      *core.Core
	listener  *channelListener
	startOnce sync.Once
}

// New constructs a Service. Zero-value Config talks to the loopback
// lthn-agent serve at DefaultBaseURL.
//
//	svc := agents.New(agents.Config{})
func New(cfg Config) *Service {
	base := core.Trim(cfg.BaseURL)
	if base == "" {
		base = DefaultBaseURL
	}
	return &Service{
		baseURL: base,
		token:   core.Trim(cfg.Token),
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

// Register builds the Agents service for core registration. Construction
// can't fail (no DB/socket — just an HTTP client), so this always Ok's.
//
//	core.New(core.WithName("agents", agents.Register))
func Register(c *core.Core) core.Result {
	svc := New(Config{})
	svc.core = c
	return core.Ok(svc)
}

// ServiceName is the Wails binding name → frontend @desktop/agents/service.
func (s *Service) ServiceName() string { return "Agents" }

// ServiceStartup brings up the live channel listener so the Agents view
// updates on CoreAgent push events (agent.blocked, agent.complete, …) rather
// than only on its poll. With no Core (unit tests via New) there's nothing to
// emit to, so it stays off.
func (s *Service) ServiceStartup(_ core.Context, _ any) core.Result {
	// The live channel listener is started explicitly via StartChannels from
	// the desktop startup path (once the crew's serve is coming up) — the GUI
	// launch doesn't run service ServiceStartup, so we don't hook it here.
	return core.Ok(nil)
}

// StartChannels brings up the live channel listener: a consume-only MCP
// session to the crew's lthn-agent serve that relays CoreAgent push events
// (agent.blocked, agent.complete, agent.status, …) to the Agents view via
// gui.EmitEvent("lthn:agents:channel"). Idempotent — call it once the crew is
// supervising the serve. No-op with a nil Core.
//
//	agentsSvc.StartChannels(core)
func (s *Service) StartChannels(c *core.Core) {
	if c == nil {
		return
	}
	s.startOnce.Do(func() {
		s.core = c
		relay := func(channel string, data any) {
			_ = gui.EmitEvent(c, channelEventName, map[string]any{
				"channel": channel,
				"data":    data,
			})
		}
		s.listener = newChannelListener(s.baseURL+"/mcp", relay)
		core.Info("agents: starting channel listener", "mcp", s.baseURL+"/mcp")
		s.listener.start()
	})
}

// ServiceShutdown stops the channel listener.
func (s *Service) ServiceShutdown() core.Result {
	if s.listener != nil {
		s.listener.stop()
	}
	return core.Ok(nil)
}

// BlockedRun is a workspace awaiting an operator answer — the actionable
// rows agentic_status enumerates. Running/completed/failed runs are
// counted, not listed (the full run history lives in CoreAgent's events
// log — a separate bridge).
type BlockedRun struct {
	Name     string `json:"name"`
	Repo     string `json:"repo"`
	Agent    string `json:"agent"`
	Question string `json:"question"`
}

// StatusResult mirrors the agentic_status payload — the live dispatch
// counts across all CoreAgent workspaces, plus the blocked runs (each with
// the question it's waiting on).
type StatusResult struct {
	Total     int          `json:"total"`
	Running   int          `json:"running"`
	Queued    int          `json:"queued"`
	Completed int          `json:"completed"`
	Failed    int          `json:"failed"`
	Blocked   []BlockedRun `json:"blocked"`
}

// Status returns the CoreAgent dispatch-queue summary (agentic_status).
// Fails cleanly when serve is unreachable so the panel can show "engine
// down" rather than crash.
//
//	r := svc.Status()
//	if r.OK { counts := r.Value.(agents.StatusResult); _ = counts }
func (s *Service) Status() core.Result {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var resp struct {
		Success bool         `json:"success"`
		Data    StatusResult `json:"data"`
		Error   string       `json:"error"`
	}
	if err := s.doTool(ctx, "agentic_status", map[string]any{}, &resp); err != nil {
		return core.Fail(core.E("agents.Status", "agentic_status", err))
	}
	if !resp.Success {
		return core.Fail(core.NewError("agents.Status: tool reported failure: " + resp.Error))
	}
	return core.Ok(resp.Data)
}

// DispatchRequest mirrors CoreAgent's agentic_dispatch input. Repo + Task
// are the essentials; Agent picks the fleet harness, the rest refine.
type DispatchRequest struct {
	Repo     string `json:"repo"`
	Org      string `json:"org,omitempty"`
	Task     string `json:"task"`
	Agent    string `json:"agent,omitempty"`
	Template string `json:"template,omitempty"`
	Branch   string `json:"branch,omitempty"`
	Issue    int    `json:"issue,omitempty"`
	DryRun   bool   `json:"dry_run,omitempty"`
}

// DispatchResult mirrors agentic_dispatch's output — the spawned run.
type DispatchResult struct {
	Success      bool   `json:"success"`
	Agent        string `json:"agent"`
	Repo         string `json:"repo"`
	WorkspaceDir string `json:"workspace_dir"`
	PID          int    `json:"pid,omitempty"`
	OutputFile   string `json:"output_file,omitempty"`
}

// Dispatch launches a CoreAgent run: preps a sandboxed workspace for Repo
// and spawns Agent on Task (agentic_dispatch). The agent runs detached —
// this returns once it's spawned (workspace + PID), not when it finishes;
// watch progress via Status / the run feed. Repo + Task are required.
//
//	r := svc.Dispatch(agents.DispatchRequest{Repo: "go-io", Task: "fix tests", Agent: "codex"})
//	if r.OK { out := r.Value.(agents.DispatchResult); _ = out }
func (s *Service) Dispatch(req DispatchRequest) core.Result {
	if core.Trim(req.Repo) == "" || core.Trim(req.Task) == "" {
		return core.Fail(core.NewError("agents.Dispatch: repo and task are required"))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	var resp struct {
		Success bool           `json:"success"`
		Data    DispatchResult `json:"data"`
		Error   string         `json:"error"`
	}
	if err := s.doTool(ctx, "agentic_dispatch", req, &resp); err != nil {
		return core.Fail(core.E("agents.Dispatch", "agentic_dispatch", err))
	}
	if !resp.Success {
		return core.Fail(core.NewError("agents.Dispatch: tool reported failure: " + resp.Error))
	}
	return core.Ok(resp.Data)
}

// ScanIssue mirrors an agentic_scan row — an open Forge issue matching the
// label filter, a candidate for dispatch.
type ScanIssue struct {
	Repo     string   `json:"repo"`
	Number   int      `json:"number"`
	Title    string   `json:"title"`
	Labels   []string `json:"labels"`
	Assignee string   `json:"assignee,omitempty"`
	URL      string   `json:"url"`
}

// ScanRequest mirrors agentic_scan's input — a Forge org + label filter.
type ScanRequest struct {
	Org    string   `json:"org,omitempty"`
	Labels []string `json:"labels,omitempty"`
	Limit  int      `json:"limit,omitempty"`
}

// Scan lists open Forge issues across an org matching the label filter
// (agentic_scan) — the dispatch candidates. Read-only.
//
//	r := svc.Scan(agents.ScanRequest{Org: "core", Labels: []string{"agentic"}})
//	if r.OK { issues := r.Value.([]agents.ScanIssue); _ = issues }
func (s *Service) Scan(req ScanRequest) core.Result {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			Success bool        `json:"success"`
			Count   int         `json:"count"`
			Issues  []ScanIssue `json:"issues"`
		} `json:"data"`
		Error string `json:"error"`
	}
	if err := s.doTool(ctx, "agentic_scan", req, &resp); err != nil {
		return core.Fail(core.E("agents.Scan", "agentic_scan", err))
	}
	if !resp.Success {
		return core.Fail(core.NewError("agents.Scan: tool reported failure: " + resp.Error))
	}
	return core.Ok(resp.Data.Issues)
}

// ResumeRequest mirrors agentic_resume's input — re-launch a workspace
// that's blocked (or failed/completed), optionally answering the question
// it raised. Workspace is the blocked run's Name as agentic_status reports
// it (org/repo/task-N); Answer is written to the workspace's ANSWER.md
// before the agent relaunches and reads it.
type ResumeRequest struct {
	Workspace string `json:"workspace"`
	Answer    string `json:"answer,omitempty"`
	Agent     string `json:"agent,omitempty"`
	DryRun    bool   `json:"dry_run,omitempty"`
}

// ResumeResult mirrors agentic_resume's output — the relaunched run.
type ResumeResult struct {
	Success    bool   `json:"success"`
	Workspace  string `json:"workspace"`
	Agent      string `json:"agent"`
	PID        int    `json:"pid,omitempty"`
	OutputFile string `json:"output_file,omitempty"`
	Prompt     string `json:"prompt,omitempty"`
}

// Resume re-launches a blocked workspace (agentic_resume): writes the
// operator's Answer to ANSWER.md, then relaunches the agent told to read
// it and continue — the other half of Status's blocked queue. Workspace is
// required; an empty Answer just relaunches the agent against BLOCKED.md.
// Runs detached like Dispatch — returns once spawned (workspace + PID), not
// when it finishes; the run reappears in Status as "running".
//
//	r := svc.Resume(agents.ResumeRequest{Workspace: "core/go-io/task-4", Answer: "Use the shared notifier"})
//	if r.OK { out := r.Value.(agents.ResumeResult); _ = out }
func (s *Service) Resume(req ResumeRequest) core.Result {
	if core.Trim(req.Workspace) == "" {
		return core.Fail(core.NewError("agents.Resume: workspace is required"))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	var resp struct {
		Success bool         `json:"success"`
		Data    ResumeResult `json:"data"`
		Error   string       `json:"error"`
	}
	if err := s.doTool(ctx, "agentic_resume", req, &resp); err != nil {
		return core.Fail(core.E("agents.Resume", "agentic_resume", err))
	}
	if !resp.Success {
		return core.Fail(core.NewError("agents.Resume: tool reported failure: " + resp.Error))
	}
	return core.Ok(resp.Data)
}

// doTool POSTs args (JSON) to /v1/tools/<tool> on the loopback serve and
// decodes the {success,data} BridgeToAPI envelope into out. Mirrors
// pkg/lemma's doJSON idiom — core.JSON* keeps the banned-imports list
// honest; net/http + bytes carry the transport. Response body is capped
// at 1 MiB.
func (s *Service) doTool(ctx context.Context, tool string, args, out interface{}) error {
	r := core.JSONMarshal(args)
	if !r.OK {
		return core.E("agents.doTool", "marshal args", r.Value.(error))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL+"/v1/tools/"+tool, bytes.NewReader(r.Value.([]byte)))
	if err != nil {
		return core.E("agents.doTool", "build request", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if s.token != "" {
		req.Header.Set("Authorization", "Bearer "+s.token)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return core.E("agents.doTool", "transport (is lthn-agent serve up?)", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 400 {
		return core.E("agents.doTool", "status "+core.Itoa(resp.StatusCode)+": "+string(respBody), nil)
	}
	if out == nil {
		return nil
	}
	if rr := core.JSONUnmarshal(respBody, out); !rr.OK {
		return core.E("agents.doTool", "decode response", rr.Value.(error))
	}
	return nil
}
