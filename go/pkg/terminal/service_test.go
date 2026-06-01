// SPDX-Licence-Identifier: EUPL-1.2

package terminal

import "testing"

func newTestService() *Service { return &Service{attached: make(map[string]func())} }

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
