// SPDX-Licence-Identifier: EUPL-1.2

package terminal

import (
	"encoding/base64"
	"sync"

	core "dappco.re/go"
	gui "dappco.re/go/render/display/webkit"
)

// Event name prefixes for the byte stream. The session ID is appended so a
// multi-tab FE can subscribe per session: "lthn:term:out:<id>" carries a
// base64-encoded output chunk; "lthn:term:exit:<id>" fires once when the shell
// exits. (Output is base64 because the Wails event bus marshals data as JSON —
// raw PTY bytes include arbitrary control sequences that aren't valid UTF-8.)
const (
	eventOutPrefix  = "lthn:term:out:"
	eventExitPrefix = "lthn:term:exit:"
)

// Service is the Wails-bound terminal control plane. A terminal runs the local
// user's own shell on their own machine; unlike the Files application, its
// process and working-directory authority is explicit at session creation.
// Bytes ride the Wails event bus (see service doc in session.go); the methods
// here are the infrequent control operations.
type Service struct {
	core *core.Core

	mu        sync.Mutex
	attached  map[string]terminalAttachment
	emitEvent func(name string, data any)
}

type terminalAttachment struct {
	session      *TerminalSession
	subscriberID uint64
}

type terminalEventGate struct {
	mu      sync.Mutex
	pending bool
	queued  []OutputChunk
	emit    func(OutputChunk)
}

func newTerminalEventGate(emit func(OutputChunk)) *terminalEventGate {
	return &terminalEventGate{pending: true, emit: emit}
}

func (g *terminalEventGate) push(chunk OutputChunk) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.pending {
		g.queued = append(g.queued, chunk)
		return
	}
	g.emit(chunk)
}

func (g *terminalEventGate) replayAndOpen(replay OutputChunk) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if replay.Reset || replay.End > replay.Start || len(replay.Data) > 0 {
		g.emit(replay)
	}
	for _, chunk := range g.queued {
		g.emit(chunk)
	}
	g.queued = nil
	g.pending = false
}

// NewService returns a terminal service bound to the core runtime.
//
//	gui.Bind(terminal.NewService(c))
func NewService(c *core.Core) *Service {
	return &Service{
		core:     c,
		attached: make(map[string]terminalAttachment),
		emitEvent: func(name string, data any) {
			gui.EmitEvent(c, name, data)
		},
	}
}

// ServiceName identifies the Wails binding namespace.
func (s *Service) ServiceName() string { return "Terminal" }

// SpawnInput configures an agent-registered session (see Spawn).
type SpawnInput struct {
	Command []string // argv to run (required)
	Env     []string // extra environment, e.g. a secret kept off the command line
	Cwd     string
	Cols    int
	Rows    int
	Label   string // human label surfaced in the tab / SessionInfo
}

// Spawn registers a process-backed PTY in the shared pool and returns its
// session ID. This is the Go-side entry an agent uses to put its own terminal
// in the pool — e.g. opencode spawning `opencode attach <url>` with the server
// password on Env so it never hits the command line. The FE then attaches a tab
// by ID (Service.Attach), exactly as for a shell, so the agent's terminal is
// watchable and drivable in-app. Kind is fixed to "agent".
//
//	id, err := terminal.Spawn(terminal.SpawnInput{
//	    Command: []string{"opencode", "attach", url},
//	    Env:     []string{"OPENCODE_SERVER_PASSWORD=" + pw},
//	    Label:   "opencode " + sandboxID,
//	})
func Spawn(in SpawnInput) (string, error) {
	if len(in.Command) == 0 {
		return "", core.E("terminal.Spawn", "command is required", nil)
	}
	sess, err := terminalPoolSingleton().Open(SessionOptions{
		Command: in.Command,
		Env:     in.Env,
		Cwd:     in.Cwd,
		Cols:    in.Cols,
		Rows:    in.Rows,
		Label:   in.Label,
		Kind:    "agent",
	})
	if err != nil {
		return "", err
	}
	return sess.ID, nil
}

// Wait blocks until a pooled session ends (its process exits), or returns
// immediately if the session is unknown (already gone). The supervision
// counterpart to Spawn — a caller that owns a Spawn'd session (e.g. the crew
// respawn loop) blocks here in place of process.Wait.
func Wait(id string) {
	if sess := terminalPoolSingleton().Get(id); sess != nil {
		<-sess.Done()
	}
}

// Kill terminates a pooled session by ID. No-op if unknown. The shutdown
// counterpart to Spawn (in place of process.Kill).
func Kill(id string) {
	terminalPoolSingleton().Close(id)
}

// OpenInput configures a new session. Cwd wins; else Repo resolves to its
// workspace path; else the session defaults to $HOME. Command, when set, runs
// that argv instead of an interactive shell ("open a tab running X").
type OpenInput struct {
	Repo    string   `json:"repo,omitempty"`
	Cwd     string   `json:"cwd,omitempty"`
	Term    string   `json:"term,omitempty"`
	Cols    int      `json:"cols,omitempty"`
	Rows    int      `json:"rows,omitempty"`
	Command []string `json:"command,omitempty"`
}

// OpenOutput carries the new session's identity back to the FE. The FE should
// register its Events.On("lthn:term:out:<id>") listener and THEN call Attach,
// so no output is produced before a listener exists.
type OpenOutput struct {
	ID    string `json:"id"`
	Host  string `json:"host"`
	Shell string `json:"shell"`
	Cwd   string `json:"cwd"`
}

// AttachInput requests output after the optional last acknowledged cursor.
type AttachInput struct {
	ID    string  `json:"id"`
	After *uint64 `json:"after,omitempty"`
}

// TerminalChunk is the renderer event payload. Data is base64-encoded PTY
// bytes; Start and End identify the exact monotonic byte range.
type TerminalChunk struct {
	Start uint64 `json:"start"`
	End   uint64 `json:"end"`
	Data  string `json:"data"`
	Reset bool   `json:"reset"`
}

// WriteInput / ResizeInput / CloseInput address a session by ID.
type WriteInput struct {
	ID   string `json:"id"`
	Data string `json:"data"`
}
type ResizeInput struct {
	ID   string `json:"id"`
	Cols int    `json:"cols"`
	Rows int    `json:"rows"`
}
type CloseInput struct {
	ID string `json:"id"`
}

// ListOutput enumerates live sessions for a tab strip.
type ListOutput struct {
	Sessions []SessionInfo `json:"sessions"`
}

// Open spawns a PTY-backed shell and returns its ID. A watcher goroutine emits
// the exit event when the shell ends. Output does NOT flow until Attach — the
// FE wires its listener in between so nothing is missed.
//
//	r := svc.Open(terminal.OpenInput{Repo: "desktop", Cols: 120, Rows: 32})
//	if r.OK { out := r.Value.(terminal.OpenOutput); _ = out.ID }
func (s *Service) Open(input OpenInput) core.Result {
	kind := "shell"
	if len(input.Command) > 0 {
		kind = "command"
	}
	sess, err := terminalPoolSingleton().Open(SessionOptions{
		Cwd:     s.resolveCwd(input),
		Term:    input.Term,
		Cols:    input.Cols,
		Rows:    input.Rows,
		Command: input.Command,
		Kind:    kind,
	})
	if err != nil {
		return core.Fail(core.E("terminal.Open", err.Error(), nil))
	}

	id := sess.ID
	go func() {
		<-sess.Done()
		s.emitEvent(eventExitPrefix+id, "")
		s.mu.Lock()
		delete(s.attached, id)
		s.mu.Unlock()
	}()

	return core.Ok(OpenOutput{ID: id, Host: sess.Host, Shell: sess.Shell, Cwd: sess.Cwd})
}

// Attach wires a session's PTY output to the event bus and resumes after the
// renderer's last acknowledged cursor. Re-attaching atomically replaces the
// previous subscriber; live output is held until replay has been emitted.
func (s *Service) Attach(input AttachInput) core.Result {
	if input.ID == "" {
		return core.Fail(core.E("terminal.Attach", "id required", nil))
	}
	sess := terminalPoolSingleton().Get(input.ID)
	if sess == nil {
		return core.Fail(core.E("terminal.Attach", "session not found: "+input.ID, nil))
	}

	id := input.ID
	after := uint64(0)
	if input.After != nil {
		after = *input.After
	}

	gate := newTerminalEventGate(func(chunk OutputChunk) {
		s.emitEvent(eventOutPrefix+id, TerminalChunk{
			Start: chunk.Start,
			End:   chunk.End,
			Data:  base64.StdEncoding.EncodeToString(chunk.Data),
			Reset: chunk.Reset,
		})
	})

	s.mu.Lock()
	var replace *uint64
	if previous, ok := s.attached[id]; ok && previous.session == sess {
		previousID := previous.subscriberID
		replace = &previousID
	}
	replay, subscriberID, err := sess.subscribeFrom(after, replace, gate.push)
	if err != nil {
		s.mu.Unlock()
		return core.Fail(core.E("terminal.Attach", err.Error(), nil))
	}
	if previous, ok := s.attached[id]; ok && previous.session != sess {
		previous.session.unsubscribe(previous.subscriberID)
	}
	s.attached[id] = terminalAttachment{session: sess, subscriberID: subscriberID}

	// Keep Attach serialised through the replay gate so a concurrent re-attach
	// cannot leak a stale snapshot after its subscriber has been replaced.
	gate.replayAndOpen(replay)
	s.mu.Unlock()
	return core.Ok(nil)
}

// Write feeds keystrokes (the raw string from xterm's onData) into the PTY.
func (s *Service) Write(input WriteInput) core.Result {
	sess := terminalPoolSingleton().Get(input.ID)
	if sess == nil {
		return core.Fail(core.E("terminal.Write", "session not found: "+input.ID, nil))
	}
	if _, err := sess.Write([]byte(input.Data)); err != nil {
		return core.Fail(core.E("terminal.Write", err.Error(), nil))
	}
	return core.Ok(nil)
}

// Resize pushes new cols/rows to the PTY (xterm fit-addon on container resize).
func (s *Service) Resize(input ResizeInput) core.Result {
	sess := terminalPoolSingleton().Get(input.ID)
	if sess == nil {
		return core.Fail(core.E("terminal.Resize", "session not found: "+input.ID, nil))
	}
	sess.Resize(input.Cols, input.Rows)
	return core.Ok(nil)
}

// Close kills a session and drops its output subscriber. No-op for unknown IDs.
func (s *Service) Close(input CloseInput) core.Result {
	id := input.ID
	s.mu.Lock()
	if attachment, ok := s.attached[id]; ok {
		attachment.session.unsubscribe(attachment.subscriberID)
		delete(s.attached, id)
	}
	s.mu.Unlock()
	terminalPoolSingleton().Close(id)
	return core.Ok(nil)
}

// List returns metadata for every live session (tab strip).
func (s *Service) List() core.Result {
	return core.Ok(ListOutput{Sessions: terminalPoolSingleton().Snapshot()})
}

// resolveCwd picks the working directory for a new session: explicit Cwd wins,
// then a Repo name resolved to its workspace path, else "" (the session falls
// back to $HOME).
func (s *Service) resolveCwd(input OpenInput) string {
	if cwd := core.Trim(input.Cwd); cwd != "" {
		return cwd
	}
	if repo := core.Trim(input.Repo); repo != "" {
		if p := resolveRepoPath(repo); p != "" {
			return p
		}
	}
	return ""
}

// resolveRepoPath finds a repo by bare name under the canonical workspace roots
// (Code/{core,lthn,host-uk,lab,snider}), mirroring pkg/repos. It returns ""
// when no matching git repo is found.
func resolveRepoPath(name string) string {
	if name == "" {
		return ""
	}
	home := core.UserHomeDir()
	if !home.OK {
		return ""
	}
	homeDir, _ := home.Value.(string)
	for _, root := range []string{"core", "lthn", "host-uk", "lab", "snider"} {
		candidate := core.PathJoin(homeDir, "Code", root, name)
		if core.Stat(core.PathJoin(candidate, ".git")).OK {
			return candidate
		}
	}
	return ""
}
