// SPDX-Licence-Identifier: EUPL-1.2

// Package terminal is the desktop's PTY-backed shell surface — the in-app
// terminal that lets the IDE replace an external editor's console. A PTY
// fundamentally needs os/exec + creack/pty (pty.Start wants an *exec.Cmd and
// returns an *os.File); core/go has no PTY wrapper, so raw os/exec here is
// correct, not a convention break.
//
// The engine in this file (TerminalSession + TerminalPool + scrollback ring)
// is transport-agnostic: it exposes Subscribe / Write / Resize and knows
// nothing about how bytes reach the screen. service.go bridges it to the Wails
// event bus — every PTY chunk is emitted as a "lthn:term:out:<id>" event,
// keystrokes arrive via the Write binding. (core/ide rides the same engine over
// a WebSocket; the desktop has no always-on localhost listener in GUI mode and
// retired its /internal HTTP surface for the event bus, so events are the
// desktop-native transport.)
package terminal

import (
	"crypto/rand"
	"encoding/hex"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	core "dappco.re/go"
	"github.com/creack/pty"
)

// terminalRingCap is the per-session scrollback ring — ~10MB so a fresh
// subscriber (view re-mount, future reconnect) re-streams recent history before
// the live feed. Big enough for a `go test ./...` firehose, small enough that a
// few idle background sessions don't balloon RSS.
const terminalRingCap = 10 * 1024 * 1024

// terminalDefaultShell falls back when neither SessionOptions.Shell nor $SHELL
// is set. zsh because macOS; /bin/sh would work but loses dotfile sourcing.
const terminalDefaultShell = "/bin/zsh"

// TerminalSubscriber is fired for every byte chunk a session's PTY produces.
// Implementations must NOT block — the pump runs subscribers synchronously, so
// a slow subscriber slows every other subscriber on the same session.
type TerminalSubscriber func(chunk []byte)

// TerminalSession is one PTY-backed shell. Lifecycle: NewTerminalSession →
// Run (blocks until the shell exits) → Close (idempotent). The pool's Close
// also kills the process so a hung shell exits its Run.
//
// Concurrency: Subscribe / Write / Resize are safe from any goroutine; Run is
// called once by the pool and hosts the pty→subscribers pump.
type TerminalSession struct {
	ID    string
	Host  string // "local" today; room for remote sessions later
	Shell string
	Cwd   string
	Env   []string

	pty    *os.File
	cmd    *exec.Cmd
	closed bool

	mu          sync.RWMutex
	ring        []byte // last terminalRingCap bytes of PTY output
	ringStart   int    // index of oldest byte (0 until wrapped)
	ringLen     int    // valid bytes in ring (≤ terminalRingCap)
	wrapped     bool   // true once more than terminalRingCap bytes written
	subscribers map[uint64]TerminalSubscriber
	nextSubID   uint64
	doneOnce    sync.Once
	done        chan struct{}
}

// SessionOptions configures a new session. Empty fields fall back to sensible
// defaults ($SHELL or /bin/zsh, $HOME, xterm-256color).
type SessionOptions struct {
	ID    string
	Host  string
	Shell string
	Cwd   string
	Term  string
	Env   []string
	Cols  int
	Rows  int
}

// NewTerminalSession allocates a PTY and spawns the configured shell. Returns
// the session ready to Run; call Close if you abandon it before Run.
func NewTerminalSession(opts SessionOptions) (*TerminalSession, error) {
	shell := opts.Shell
	if shell == "" {
		shell = core.Env("SHELL")
	}
	if shell == "" {
		shell = terminalDefaultShell
	}
	cwd := opts.Cwd
	if cwd == "" {
		cwd = core.Env("HOME")
	}
	term := opts.Term
	if term == "" {
		term = "xterm-256color"
	}

	cmd := exec.Command(shell)
	cmd.Dir = cwd
	cmd.Env = append(os.Environ(), "TERM="+term)
	cmd.Env = append(cmd.Env, opts.Env...)

	f, err := pty.Start(cmd)
	if err != nil {
		return nil, core.E("terminal.NewSession", "pty start "+shell+": "+err.Error(), nil)
	}

	if opts.Cols > 0 && opts.Rows > 0 {
		_ = pty.Setsize(f, &pty.Winsize{Cols: uint16(opts.Cols), Rows: uint16(opts.Rows)})
	}

	host := opts.Host
	if host == "" {
		host = "local"
	}

	return &TerminalSession{
		ID:          opts.ID,
		Host:        host,
		Shell:       shell,
		Cwd:         cwd,
		Env:         opts.Env,
		pty:         f,
		cmd:         cmd,
		ring:        make([]byte, terminalRingCap),
		subscribers: make(map[uint64]TerminalSubscriber),
		done:        make(chan struct{}),
	}, nil
}

// Run pumps PTY output to subscribers + the ring until the shell exits. Blocks
// until cmd.Wait returns. Called once per session by the pool.
func (s *TerminalSession) Run() error {
	defer s.markDone()

	buf := make([]byte, 32*1024)
	for {
		n, err := s.pty.Read(buf)
		if n > 0 {
			chunk := append([]byte(nil), buf[:n]...)
			s.appendRing(chunk)
			s.fanOut(chunk)
		}
		if err != nil {
			break
		}
	}

	if err := s.cmd.Wait(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			_ = exitErr // exit code available via exitErr.ExitCode() when we surface it
		}
	}
	return nil
}

// Write feeds client bytes (xterm.js keystrokes) into the PTY.
func (s *TerminalSession) Write(p []byte) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed || s.pty == nil {
		return 0, io.ErrClosedPipe
	}
	return s.pty.Write(p)
}

// Resize pushes window cols/rows to the PTY (TIOCSWINSZ). Zero dimensions are
// silently ignored.
func (s *TerminalSession) Resize(cols, rows int) {
	if cols <= 0 || rows <= 0 {
		return
	}
	s.mu.RLock()
	f := s.pty
	s.mu.RUnlock()
	if f == nil {
		return
	}
	_ = pty.Setsize(f, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
}

// Subscribe registers a subscriber and returns the current ring snapshot (so a
// new subscriber catches up on scrollback before the live stream) plus an
// unsubscribe func. Safe to call concurrently.
func (s *TerminalSession) Subscribe(sub TerminalSubscriber) ([]byte, func()) {
	s.mu.Lock()
	id := s.nextSubID
	s.nextSubID++
	s.subscribers[id] = sub
	snapshot := s.ringSnapshot()
	s.mu.Unlock()

	return snapshot, func() {
		s.mu.Lock()
		delete(s.subscribers, id)
		s.mu.Unlock()
	}
}

// Done closes when the session's PTY pump exits (shell exit, kill, EOF).
func (s *TerminalSession) Done() <-chan struct{} { return s.done }

// Close kills the shell and closes the PTY. Idempotent; Run returns shortly
// after as the PTY read hits EOF.
func (s *TerminalSession) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	s.mu.Unlock()

	if s.cmd != nil && s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
	}
	if s.pty != nil {
		_ = s.pty.Close()
	}
}

func (s *TerminalSession) appendRing(p []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ringCap := len(s.ring)
	if ringCap == 0 {
		return
	}
	for _, b := range p {
		idx := (s.ringStart + s.ringLen) % ringCap
		s.ring[idx] = b
		if s.ringLen < ringCap {
			s.ringLen++
		} else {
			s.ringStart = (s.ringStart + 1) % ringCap
			s.wrapped = true
		}
	}
}

// ringSnapshot copies the ring's contents into a new slice in chronological
// order. Caller owns the result. Must hold the lock (Subscribe does). Capacity
// is derived from len(s.ring) (always terminalRingCap in production) so the
// wrap path is unit-testable with a small ring.
func (s *TerminalSession) ringSnapshot() []byte {
	if s.ringLen == 0 {
		return nil
	}
	ringCap := len(s.ring)
	out := make([]byte, s.ringLen)
	if !s.wrapped {
		copy(out, s.ring[:s.ringLen])
		return out
	}
	first := ringCap - s.ringStart
	copy(out, s.ring[s.ringStart:])
	copy(out[first:], s.ring[:s.ringStart])
	return out
}

// fanOut delivers a chunk to every subscriber under a read lock. New
// subscribers can't miss bytes — Subscribe atomically grabs the ring snapshot
// while holding the write lock.
func (s *TerminalSession) fanOut(chunk []byte) {
	s.mu.RLock()
	subs := make([]TerminalSubscriber, 0, len(s.subscribers))
	for _, sub := range s.subscribers {
		subs = append(subs, sub)
	}
	s.mu.RUnlock()

	for _, sub := range subs {
		sub(chunk)
	}
}

func (s *TerminalSession) markDone() {
	s.doneOnce.Do(func() { close(s.done) })
}

// --- Session pool ---

// TerminalPool tracks live sessions so the control bridge + event bridge share
// state. One pool per desktop process — sessions belong to the app, not a
// transport.
type TerminalPool struct {
	mu       sync.RWMutex
	sessions map[string]*TerminalSession
}

// NewTerminalPool constructs an empty pool.
func NewTerminalPool() *TerminalPool {
	return &TerminalPool{sessions: make(map[string]*TerminalSession)}
}

var (
	terminalPoolOnce sync.Once
	terminalPoolInst *TerminalPool
)

// terminalPoolSingleton returns the package pool, lazily initialised.
func terminalPoolSingleton() *TerminalPool {
	terminalPoolOnce.Do(func() { terminalPoolInst = NewTerminalPool() })
	return terminalPoolInst
}

// Open allocates a session, starts its Run pump in a goroutine, and returns it
// for immediate Subscribe / Write use.
func (p *TerminalPool) Open(opts SessionOptions) (*TerminalSession, error) {
	if opts.ID == "" {
		opts.ID = newSessionID()
	}
	sess, err := NewTerminalSession(opts)
	if err != nil {
		return nil, err
	}

	p.mu.Lock()
	p.sessions[sess.ID] = sess
	p.mu.Unlock()

	go func() {
		_ = sess.Run()
		p.mu.Lock()
		delete(p.sessions, sess.ID)
		p.mu.Unlock()
	}()

	return sess, nil
}

// Get returns a session by ID, or nil.
func (p *TerminalPool) Get(id string) *TerminalSession {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.sessions[id]
}

// Close terminates a session by ID; no-op if absent.
func (p *TerminalPool) Close(id string) {
	p.mu.RLock()
	sess := p.sessions[id]
	p.mu.RUnlock()
	if sess != nil {
		sess.Close()
	}
}

// Snapshot returns metadata for every live session (for the tab strip).
func (p *TerminalPool) Snapshot() []SessionInfo {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]SessionInfo, 0, len(p.sessions))
	for _, s := range p.sessions {
		s.mu.RLock()
		out = append(out, SessionInfo{ID: s.ID, Host: s.Host, Shell: s.Shell, Cwd: s.Cwd})
		s.mu.RUnlock()
	}
	return out
}

// SessionInfo is the Wails-serialisable view of a session.
type SessionInfo struct {
	ID    string `json:"id"`
	Host  string `json:"host"`
	Shell string `json:"shell"`
	Cwd   string `json:"cwd"`
}

// newSessionID returns 16 random hex chars — enough for a per-process table.
func newSessionID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "sess-" + time.Now().UTC().Format("20060102T150405.000000")
	}
	return hex.EncodeToString(b[:])
}
