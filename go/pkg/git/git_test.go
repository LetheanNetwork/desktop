// SPDX-Licence-Identifier: EUPL-1.2

// Shared test harness for pkg/git's external test package. Builds a
// real (but hermetic — TempDir only, no network, no fixed ports)
// *core.Core with a genuine dappco.re/go/process Service registered,
// the same construction pkg/bridge/process_test.go uses for the same
// Service. Git fixtures are real repos built with real `git` — the
// `git` binary is a build-time dependency of this whole repo (the
// desktop app shells out to it in production), so this is not a
// fragile external dependency.
package git_test

import (
	core "dappco.re/go"
	"dappco.re/go/process"
	subject "dappco.re/lthn/desktop/pkg/git"
)

// gitHarness returns a *subject.Service bound to a *core.Core with a
// real, started process.Service registered under "process", plus the
// process.Service itself so callers can shell fixture commands
// (`git init`, `git commit`, …) through the identical execution path
// production code uses.
func gitHarness(t *core.T) (*subject.Service, *process.Service) {
	t.Helper()
	c := core.New()
	r := process.NewService(process.Options{})(c)
	core.RequireTrue(t, r.OK)
	ps := r.Value.(*process.Service)
	core.RequireTrue(t, ps.OnStartup(core.Background()).OK)
	core.RequireTrue(t, c.RegisterService("process", ps).OK)
	return subject.NewService(c), ps
}

// mustGit runs `git -C dir <args>` via the harness's process.Service
// and fails the test immediately on error — fixture setup is not
// itself under test.
func mustGit(t *core.T, ps *process.Service, dir string, args ...string) string {
	t.Helper()
	full := append([]string{"-C", dir}, args...)
	r := ps.Run(core.Background(), "git", full...)
	core.RequireTrue(t, r.OK, r.Error())
	out, _ := r.Value.(string)
	return out
}

// gitFixture creates a TempDir repo on branch "main" with one
// committed file ("a.txt"), a throwaway local identity (so `commit`
// never depends on global git config), and no upstream.
func gitFixture(t *core.T, ps *process.Service) string {
	t.Helper()
	dir := t.TempDir()
	mustGit(t, ps, dir, "init", "-b", "main")
	mustGit(t, ps, dir, "config", "user.email", "test@example.com")
	mustGit(t, ps, dir, "config", "user.name", "Test")
	core.RequireTrue(t, core.WriteFile(core.PathJoin(dir, "a.txt"), []byte("hello\n"), 0o644).OK)
	mustGit(t, ps, dir, "add", "-A")
	mustGit(t, ps, dir, "commit", "-m", "initial")
	return dir
}

// ─── runGit guard branches (exercised through Branch, since runGit
// itself is unexported) ─────────────────────────────────────────────

func TestGit_RunGit_Bad_CoreNotBound(t *core.T) {
	var svc subject.Service // zero value: unexported core field is nil
	r := svc.Branch("/any/path")
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "core not bound")
}

func TestGit_RunGit_Bad_ProcessServiceUnavailable(t *core.T) {
	c := core.New() // no process service registered
	svc := subject.NewService(c)
	r := svc.Branch(t.TempDir())
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "process service unavailable")
}

// ─── NewService / Register ──────────────────────────────────────────

func TestGit_NewService_Good(t *core.T) {
	svc := subject.NewService(core.New())
	core.AssertNotNil(t, svc)
	core.AssertEqual(t, "Git", svc.ServiceName())
}

func TestGit_NewService_Bad(t *core.T) {
	// A nil *core.Core must not panic construction — the failure
	// surfaces later, at call time, via runGit's own guard.
	svc := subject.NewService(nil)
	core.AssertNotNil(t, svc)
	r := svc.Branch("/nonexistent")
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "core not bound")
}

func TestGit_NewService_Ugly(t *core.T) {
	a := subject.NewService(core.New())
	b := subject.NewService(core.New())
	core.AssertTrue(t, a != b, "each call constructs a distinct instance")
}

func TestGit_Register_Good(t *core.T) {
	r := subject.Register(core.New())
	core.AssertTrue(t, r.OK)
	svc, ok := r.Value.(*subject.Service)
	core.AssertTrue(t, ok)
	core.AssertNotNil(t, svc)
}

func TestGit_Register_Bad(t *core.T) {
	// Register never inspects its argument — even a nil Core wraps
	// cleanly into a Result whose failure surfaces later at call time.
	r := subject.Register(nil)
	core.AssertTrue(t, r.OK)
	svc, ok := r.Value.(*subject.Service)
	core.AssertTrue(t, ok)
	rb := svc.Branch("/x")
	core.AssertFalse(t, rb.OK)
}

func TestGit_Register_Ugly(t *core.T) {
	c := core.New()
	r1 := subject.Register(c)
	r2 := subject.Register(c)
	core.AssertTrue(t, r1.OK)
	core.AssertTrue(t, r2.OK)
	core.AssertTrue(t, r1.Value.(*subject.Service) != r2.Value.(*subject.Service))
}
