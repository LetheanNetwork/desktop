// SPDX-Licence-Identifier: EUPL-1.2

package terminal

import (
	"encoding/base64"
	"sync"
	"testing"
	"time"

	core "dappco.re/go"
	coreio "dappco.re/go/io"
	officefiles "dappco.re/lthn/desktop/pkg/office/files"
)

type capturedTerminalEvent struct {
	name string
	data any
}

func newTestService() *Service {
	return &Service{
		attached: make(map[string]terminalAttachment),
		emitEvent: func(string, any) {
		},
	}
}

func useTerminalPool(t *testing.T, sessions ...*TerminalSession) {
	t.Helper()
	previous := terminalPoolSingleton()
	pool := NewTerminalPool()
	for _, session := range sessions {
		pool.sessions[session.ID] = session
	}
	terminalPoolOnce = sync.Once{}
	terminalPoolOnce.Do(func() { terminalPoolInst = pool })
	t.Cleanup(func() {
		terminalPoolOnce = sync.Once{}
		terminalPoolOnce.Do(func() { terminalPoolInst = previous })
	})
}

func TestService_ResolveCwd_Good(t *testing.T) {
	// Explicit Cwd wins over everything, with no filesystem lookup.
	if got := newTestService().resolveCwd(OpenInput{Cwd: "/tmp", Repo: "desktop"}); got != "/tmp" {
		t.Errorf("explicit Cwd should win, got %q", got)
	}
}

func TestService_ResolveCwd_Bad(t *testing.T) {
	// No Cwd, no resolvable Repo → empty (the session falls back to $HOME).
	if got := newTestService().resolveCwd(OpenInput{}); got != "" {
		t.Errorf("empty input should not resolve a cwd, got %q", got)
	}
	if got := newTestService().resolveCwd(OpenInput{Repo: "definitely-not-a-real-repo-xyzzy"}); got != "" {
		t.Errorf("unknown repo should not resolve a cwd, got %q", got)
	}
}

func TestService_ResolveRepoPath_Unknown(t *testing.T) {
	if got := resolveRepoPath("definitely-not-a-real-repo-xyzzy"); got != "" {
		t.Errorf("unknown repo should resolve to empty, got %q", got)
	}
	if got := resolveRepoPath(""); got != "" {
		t.Errorf("empty name should resolve to empty, got %q", got)
	}
}

func TestService_ResolveWorkspace_GoodUsesFilesCapability(t *testing.T) {
	c := core.New()
	medium := coreio.NewMemoryMedium()
	if err := medium.EnsureDir("desktop"); err != nil {
		t.Fatalf("create workspace fixture: %v", err)
	}
	filesService := officefiles.NewService(officefiles.Options{
		Core: c,
		Mounts: []officefiles.Mount{{
			ID:                 "projects",
			Name:               "Projects",
			Kind:               "local",
			LocalRoot:          "/trusted/projects",
			Capabilities:       officefiles.ReadWriteCapabilities(),
			Medium:             medium,
			ContainmentAudited: true,
		}},
		Runtime: officefiles.NewMemoryRuntimeMetadata(),
	})
	if result := filesService.Register(c); !result.OK {
		t.Fatalf("register Files service: %s", result.Error())
	}
	if result := c.RegisterService("office-files", filesService); !result.OK {
		t.Fatalf("compose Files service: %s", result.Error())
	}
	service := NewService(c)

	got, err := service.resolveWorkspace(OpenInput{
		MountID: "projects",
		Path:    "desktop",
	})
	if err != nil {
		t.Fatalf("resolve Files workspace: %v", err)
	}
	if got != "/trusted/projects/desktop" {
		t.Fatalf("workspace = %q, want /trusted/projects/desktop", got)
	}
}

func TestService_ResolveWorkspace_BadFailsClosed(t *testing.T) {
	service := NewService(core.New())
	for _, input := range []OpenInput{
		{MountID: "missing"},
		{MountID: "missing", Path: "../escape"},
		{Path: "relative-without-mount"},
	} {
		if _, err := service.resolveWorkspace(input); err == nil {
			t.Fatalf("workspace %#v should fail", input)
		}
	}
}

// TestService_Control_Ugly — the control methods must fail-fast on a missing
// session rather than panic or touch a nil core.
func TestService_Control_Ugly(t *testing.T) {
	s := newTestService()
	if r := s.Attach(AttachInput{ID: ""}); r.OK {
		t.Error("Attach with empty id should fail")
	}
	if r := s.Attach(AttachInput{ID: "ghost"}); r.OK {
		t.Error("Attach to unknown session should fail")
	}
	if r := s.Write(WriteInput{ID: "ghost", Data: "x"}); r.OK {
		t.Error("Write to unknown session should fail")
	}
	if r := s.Resize(ResizeInput{ID: "ghost", Cols: 80, Rows: 24}); r.OK {
		t.Error("Resize on unknown session should fail")
	}
	// Close is a no-op for unknown sessions — idempotent teardown, not an error.
	if r := s.Close(CloseInput{ID: "ghost"}); !r.OK {
		t.Errorf("Close of unknown session should be a no-op OK, got %s", r.Error())
	}
}

func TestService_Attach_CursorGoodRetainedReplay(t *testing.T) {
	session := memSession(8)
	session.ID = "retained"
	session.appendRing([]byte("abcdef"))
	useTerminalPool(t, session)

	var events []capturedTerminalEvent
	service := newTestService()
	service.emitEvent = func(name string, data any) {
		events = append(events, capturedTerminalEvent{name: name, data: data})
	}
	after := uint64(2)

	if result := service.Attach(AttachInput{ID: session.ID, After: &after}); !result.OK {
		t.Fatalf("attach: %s", result.Error())
	}
	if len(events) != 1 {
		t.Fatalf("events = %d, want one replay", len(events))
	}
	if events[0].name != eventOutPrefix+session.ID {
		t.Fatalf("event name = %q, want %q", events[0].name, eventOutPrefix+session.ID)
	}
	assertTerminalChunk(t, events[0].data, 2, 6, "cdef", false)
}

func TestService_Attach_CursorBadFuture(t *testing.T) {
	session := memSession(8)
	session.ID = "future"
	session.appendRing([]byte("abc"))
	useTerminalPool(t, session)

	service := newTestService()
	after := uint64(4)
	if result := service.Attach(AttachInput{ID: session.ID, After: &after}); result.OK {
		t.Fatal("future cursor attach should fail")
	}
	if len(session.subscribers) != 0 {
		t.Fatalf("future cursor registered %d subscribers, want none", len(session.subscribers))
	}
	if len(service.attached) != 0 {
		t.Fatalf("future cursor recorded %d attachments, want none", len(service.attached))
	}
}

func TestService_Attach_ReplacesSubscriber(t *testing.T) {
	session := memSession(8)
	session.ID = "replace"
	useTerminalPool(t, session)

	var events []capturedTerminalEvent
	service := newTestService()
	service.emitEvent = func(name string, data any) {
		events = append(events, capturedTerminalEvent{name: name, data: data})
	}
	zero := uint64(0)
	if result := service.Attach(AttachInput{ID: session.ID, After: &zero}); !result.OK {
		t.Fatalf("first attach: %s", result.Error())
	}
	session.publish([]byte("a"))

	one := uint64(1)
	if result := service.Attach(AttachInput{ID: session.ID, After: &one}); !result.OK {
		t.Fatalf("second attach: %s", result.Error())
	}
	session.publish([]byte("b"))

	if len(session.subscribers) != 1 {
		t.Fatalf("subscribers = %d, want one replacement", len(session.subscribers))
	}
	if len(events) != 2 {
		t.Fatalf("live events = %d, want two", len(events))
	}
	assertTerminalChunk(t, events[0].data, 0, 1, "a", false)
	assertTerminalChunk(t, events[1].data, 1, 2, "b", false)
}

func TestService_Attach_ReplayPrecedesConcurrentLiveOutput(t *testing.T) {
	session := memSession(16)
	session.ID = "ordered"
	session.appendRing([]byte("old"))
	useTerminalPool(t, session)

	var (
		eventsMu sync.Mutex
		events   []capturedTerminalEvent
	)
	replayStarted := make(chan struct{})
	releaseReplay := make(chan struct{})
	service := newTestService()
	service.emitEvent = func(name string, data any) {
		eventsMu.Lock()
		events = append(events, capturedTerminalEvent{name: name, data: data})
		first := len(events) == 1
		eventsMu.Unlock()
		if first {
			close(replayStarted)
			<-releaseReplay
		}
	}

	attachDone := make(chan core.Result)
	go func() {
		attachDone <- service.Attach(AttachInput{ID: session.ID})
	}()
	select {
	case <-replayStarted:
	case <-time.After(time.Second):
		t.Fatal("replay did not start")
	}

	publishDone := make(chan struct{})
	go func() {
		session.publish([]byte("new"))
		close(publishDone)
	}()
	close(releaseReplay)

	select {
	case result := <-attachDone:
		if !result.OK {
			t.Fatalf("attach: %s", result.Error())
		}
	case <-time.After(time.Second):
		t.Fatal("attach did not finish")
	}
	select {
	case <-publishDone:
	case <-time.After(time.Second):
		t.Fatal("live publish did not finish")
	}

	eventsMu.Lock()
	defer eventsMu.Unlock()
	if len(events) != 2 {
		t.Fatalf("events = %d, want replay then live", len(events))
	}
	assertTerminalChunk(t, events[0].data, 0, 3, "old", false)
	assertTerminalChunk(t, events[1].data, 3, 6, "new", false)
}

func TestService_Spawn_Good(t *testing.T) {
	id, err := Spawn(SpawnInput{Command: []string{"/bin/cat"}, Label: "test-agent"})
	if err != nil {
		t.Skipf("spawn: %v", err) // /bin/cat absent in some sandboxes
	}
	if id == "" {
		t.Fatal("Spawn returned an empty id")
	}
	sess := terminalPoolSingleton().Get(id)
	if sess == nil {
		t.Fatal("spawned session is not in the pool")
	}
	if sess.Kind != "agent" {
		t.Errorf("Kind = %q, want agent", sess.Kind)
	}
	if sess.Label != "test-agent" {
		t.Errorf("Label = %q, want test-agent", sess.Label)
	}
	terminalPoolSingleton().Close(id)
}

func TestService_Spawn_Bad(t *testing.T) {
	if _, err := Spawn(SpawnInput{}); err == nil {
		t.Error("Spawn with no command should error")
	}
}

func TestService_WaitKill_Good(t *testing.T) {
	id, err := Spawn(SpawnInput{Command: []string{"/bin/cat"}}) // cat blocks on stdin
	if err != nil {
		t.Skipf("spawn: %v", err)
	}
	done := make(chan struct{})
	go func() { Wait(id); close(done) }()
	Kill(id)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Wait did not return after Kill")
	}
	// Wait on an unknown session returns immediately (already gone).
	Wait("definitely-not-a-session")
}

func TestService_List_Good(t *testing.T) {
	// With no sessions opened, List returns an empty (non-nil) slice.
	r := newTestService().List()
	if !r.OK {
		t.Fatalf("List failed: %s", r.Error())
	}
	out, ok := r.Value.(ListOutput)
	if !ok {
		t.Fatalf("List value type = %T, want ListOutput", r.Value)
	}
	if out.Sessions == nil {
		t.Error("List sessions should be a non-nil slice")
	}
}

func assertTerminalChunk(t *testing.T, value any, start, end uint64, data string, reset bool) {
	t.Helper()
	chunk, ok := value.(TerminalChunk)
	if !ok {
		t.Fatalf("event payload type = %T, want TerminalChunk", value)
	}
	decoded, err := base64.StdEncoding.DecodeString(chunk.Data)
	if err != nil {
		t.Fatalf("decode event data: %v", err)
	}
	if chunk.Start != start || chunk.End != end || string(decoded) != data || chunk.Reset != reset {
		t.Errorf(
			"terminal chunk = {start:%d end:%d data:%q reset:%t}, want {start:%d end:%d data:%q reset:%t}",
			chunk.Start,
			chunk.End,
			string(decoded),
			chunk.Reset,
			start,
			end,
			data,
			reset,
		)
	}
}
