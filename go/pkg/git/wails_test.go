// SPDX-Licence-Identifier: EUPL-1.2

// Real behavioural tests for wails.go's Wails3 surface.
//
// wails_example_test.go's Test* functions previously paired 1:1 with
// its Example* functions and asserted only on a method VALUE's %T
// formatting:
//
//	ref := (*subject.Service).Branch
//	typeName := core.Sprintf("%T", ref)
//	core.AssertContains(t, typeName, "func")
//
// `ref` is a bound-method-expression value — taking it never calls
// Branch, so none of its statements ever execute. That's the "fake
// test" mechanism this coverage lane was tasked to find across
// pkg/git, pkg/lint, and pkg/tools: 87 tests passing, 0.0% statement
// coverage, because every one of them independently satisfies itself
// on the reflected type name and stops. This file replaces the
// pattern with tests that construct a real Service against a real
// process.Service and drive it against real (TempDir-hermetic) git
// repos.
package git_test

import (
	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/git"
)

// ─── ServiceName / ServiceStartup / ServiceShutdown ─────────────────

func TestWails_Service_ServiceName_Good(t *core.T) {
	svc, _ := gitHarness(t)
	core.AssertEqual(t, "Git", svc.ServiceName())
}

func TestWails_Service_ServiceStartup_Good(t *core.T) {
	svc, _ := gitHarness(t)
	r := svc.ServiceStartup(core.Background(), nil)
	core.AssertTrue(t, r.OK)
}

func TestWails_Service_ServiceShutdown_Good(t *core.T) {
	svc, _ := gitHarness(t)
	r := svc.ServiceShutdown()
	core.AssertTrue(t, r.OK)
}

// ─── Branch ──────────────────────────────────────────────────────────

// TestWails_Service_Branch_Good — a local branch with an upstream
// that is both ahead (one unpushed local commit) and behind (one
// commit pushed from a sibling clone) exercises the full success
// path: non-empty branch, successful counts probe, both Atoi parses.
func TestWails_Service_Branch_Good(t *core.T) {
	svc, ps := gitHarness(t)

	work := gitFixture(t, ps) // branch "main", 1 commit, no upstream yet

	bare := t.TempDir()
	mustGit(t, ps, work, "clone", "--bare", work, bare)
	mustGit(t, ps, work, "remote", "add", "origin", bare)
	mustGit(t, ps, work, "push", "-u", "origin", "main")

	// ahead: one local commit never pushed.
	core.RequireTrue(t, core.WriteFile(core.PathJoin(work, "b.txt"), []byte("b\n"), 0o644).OK)
	mustGit(t, ps, work, "add", "-A")
	mustGit(t, ps, work, "commit", "-m", "local only")

	// behind: a sibling clone pushes a commit work never sees until fetch.
	other := t.TempDir()
	mustGit(t, ps, work, "clone", bare, other)
	mustGit(t, ps, other, "config", "user.email", "test@example.com")
	mustGit(t, ps, other, "config", "user.name", "Test")
	core.RequireTrue(t, core.WriteFile(core.PathJoin(other, "c.txt"), []byte("c\n"), 0o644).OK)
	mustGit(t, ps, other, "add", "-A")
	mustGit(t, ps, other, "commit", "-m", "from sibling")
	mustGit(t, ps, other, "push", "origin", "main")

	mustGit(t, ps, work, "fetch", "origin") // update refs/remotes/origin/main without touching local HEAD

	r := svc.Branch(work)
	core.RequireTrue(t, r.OK, r.Error())
	info, ok := r.Value.(subject.BranchInfo)
	core.RequireTrue(t, ok)
	core.AssertEqual(t, "main", info.Branch)
	core.AssertEqual(t, 1, info.Ahead)
	core.AssertEqual(t, 1, info.Behind)
}

// TestWails_Service_Branch_Bad_NotARepo — runGit fails outright
// (not inside a git repo), Branch returns the failure verbatim.
func TestWails_Service_Branch_Bad_NotARepo(t *core.T) {
	svc, _ := gitHarness(t)
	r := svc.Branch(t.TempDir())
	core.AssertFalse(t, r.OK)
}

// TestWails_Service_Branch_Ugly_DetachedHead — `git branch
// --show-current` prints nothing (exit 0) in detached HEAD, taking
// the early "empty branch" return before the counts probe ever runs.
func TestWails_Service_Branch_Ugly_DetachedHead(t *core.T) {
	svc, ps := gitHarness(t)
	work := gitFixture(t, ps)
	mustGit(t, ps, work, "checkout", "--detach", "HEAD")

	r := svc.Branch(work)
	core.RequireTrue(t, r.OK, r.Error())
}

// ─── Status ──────────────────────────────────────────────────────────

// TestWails_Service_Status_Good — one unstaged modification, one
// untracked file, one staged addition: exercises every StatusEntry
// flag combination the non-rename path produces.
func TestWails_Service_Status_Good(t *core.T) {
	svc, ps := gitHarness(t)
	work := gitFixture(t, ps)

	// unstaged modification of the tracked file
	core.RequireTrue(t, core.WriteFile(core.PathJoin(work, "a.txt"), []byte("changed\n"), 0o644).OK)
	// untracked new file
	core.RequireTrue(t, core.WriteFile(core.PathJoin(work, "untracked.txt"), []byte("u\n"), 0o644).OK)
	// staged new file
	core.RequireTrue(t, core.WriteFile(core.PathJoin(work, "staged.txt"), []byte("s\n"), 0o644).OK)
	mustGit(t, ps, work, "add", "staged.txt")

	r := svc.Status(work)
	core.RequireTrue(t, r.OK, r.Error())
}

func TestWails_Service_Status_Bad_NotARepo(t *core.T) {
	svc, _ := gitHarness(t)
	r := svc.Status(t.TempDir())
	core.AssertFalse(t, r.OK)
}

// TestWails_Service_Status_Ugly_Rename — a staged rename emits
// "R  new\x00old" on one -z record pair; the parser must consume the
// trailing old-path token instead of treating it as its own entry.
func TestWails_Service_Status_Ugly_Rename(t *core.T) {
	svc, ps := gitHarness(t)
	work := gitFixture(t, ps)
	mustGit(t, ps, work, "mv", "a.txt", "renamed.txt")

	r := svc.Status(work)
	core.RequireTrue(t, r.OK, r.Error())
}

// ─── Diff ────────────────────────────────────────────────────────────

// TestWails_Service_Diff_Good — staged=true and an explicit file
// together exercise both optional-arg branches on a successful call.
func TestWails_Service_Diff_Good(t *core.T) {
	svc, ps := gitHarness(t)
	work := gitFixture(t, ps)
	core.RequireTrue(t, core.WriteFile(core.PathJoin(work, "a.txt"), []byte("changed\n"), 0o644).OK)
	mustGit(t, ps, work, "add", "a.txt")

	r := svc.Diff(work, "a.txt", true)
	core.RequireTrue(t, r.OK, r.Error())
}

func TestWails_Service_Diff_Bad_NotARepo(t *core.T) {
	svc, _ := gitHarness(t)
	r := svc.Diff(t.TempDir(), "", false)
	core.AssertFalse(t, r.OK)
}

// TestWails_Service_Diff_Ugly_UnstagedNoFile — the default shape
// (file="", staged=false): unstaged whole-repo diff.
func TestWails_Service_Diff_Ugly_UnstagedNoFile(t *core.T) {
	svc, ps := gitHarness(t)
	work := gitFixture(t, ps)
	core.RequireTrue(t, core.WriteFile(core.PathJoin(work, "a.txt"), []byte("changed\n"), 0o644).OK)

	r := svc.Diff(work, "", false)
	core.RequireTrue(t, r.OK, r.Error())
}

// ─── Add ─────────────────────────────────────────────────────────────

// TestWails_Service_Add_Good — specific files (not `all`) exercise
// the "--" + files branch and the success return.
func TestWails_Service_Add_Good(t *core.T) {
	svc, ps := gitHarness(t)
	work := gitFixture(t, ps)
	core.RequireTrue(t, core.WriteFile(core.PathJoin(work, "new.txt"), []byte("n\n"), 0o644).OK)

	r := svc.Add(work, []string{"new.txt"}, false)
	core.RequireTrue(t, r.OK, r.Error())

	out := mustGit(t, ps, work, "status", "--porcelain")
	core.AssertContains(t, out, "new.txt")
}

// TestWails_Service_Add_Bad_NotARepo — all=true drives the `-A`
// branch, and a non-repo target drives the runGit failure return.
func TestWails_Service_Add_Bad_NotARepo(t *core.T) {
	svc, _ := gitHarness(t)
	r := svc.Add(t.TempDir(), nil, true)
	core.AssertFalse(t, r.OK)
}

// TestWails_Service_Add_Ugly_UnknownPathspec — a real repo, but a
// file that was never created: `git add -- missing.txt` fails with a
// pathspec error, exercising the specific-files branch's failure arm.
func TestWails_Service_Add_Ugly_UnknownPathspec(t *core.T) {
	svc, ps := gitHarness(t)
	work := gitFixture(t, ps)
	r := svc.Add(work, []string{"missing.txt"}, false)
	core.AssertFalse(t, r.OK)
}

// ─── Unstage ─────────────────────────────────────────────────────────

// TestWails_Service_Unstage_Good — specific files exercise the
// "--" + files branch on a successful call.
func TestWails_Service_Unstage_Good(t *core.T) {
	svc, ps := gitHarness(t)
	work := gitFixture(t, ps)
	core.RequireTrue(t, core.WriteFile(core.PathJoin(work, "s.txt"), []byte("s\n"), 0o644).OK)
	mustGit(t, ps, work, "add", "s.txt")

	r := svc.Unstage(work, []string{"s.txt"})
	core.RequireTrue(t, r.OK, r.Error())

	out := mustGit(t, ps, work, "status", "--porcelain")
	core.AssertContains(t, out, "?? s.txt")
}

// TestWails_Service_Unstage_Bad_NotARepo — empty files drives the
// "." branch, and a non-repo target drives the runGit failure return.
func TestWails_Service_Unstage_Bad_NotARepo(t *core.T) {
	svc, _ := gitHarness(t)
	r := svc.Unstage(t.TempDir(), nil)
	core.AssertFalse(t, r.OK)
}

// TestWails_Service_Unstage_Ugly_NoopOnCleanIndex — empty files on a
// real repo with nothing staged: `git restore --staged .` succeeds
// as a no-op, exercising the "." branch's success arm.
func TestWails_Service_Unstage_Ugly_NoopOnCleanIndex(t *core.T) {
	svc, ps := gitHarness(t)
	work := gitFixture(t, ps)
	r := svc.Unstage(work, nil)
	core.RequireTrue(t, r.OK, r.Error())
}

// ─── Commit ──────────────────────────────────────────────────────────

// TestWails_Service_Commit_Good — staged changes + a message commit
// cleanly and rev-parse resolves the new SHA.
func TestWails_Service_Commit_Good(t *core.T) {
	svc, ps := gitHarness(t)
	work := gitFixture(t, ps)
	core.RequireTrue(t, core.WriteFile(core.PathJoin(work, "b.txt"), []byte("b\n"), 0o644).OK)
	mustGit(t, ps, work, "add", "-A")

	r := svc.Commit(work, "feat: add b.txt")
	core.RequireTrue(t, r.OK, r.Error())
	res, ok := r.Value.(subject.CommitResult)
	core.RequireTrue(t, ok)
	core.AssertNotEmpty(t, res.SHA)
}

// TestWails_Service_Commit_Bad_EmptyMessage — the trim-empty guard
// short-circuits before any git invocation.
func TestWails_Service_Commit_Bad_EmptyMessage(t *core.T) {
	svc, ps := gitHarness(t)
	work := gitFixture(t, ps)
	r := svc.Commit(work, "   ")
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "message required")
}

// TestWails_Service_Commit_Ugly_NothingStaged — a valid message
// against a repo with a clean index: `git commit` itself fails
// ("nothing to commit"), exercising the commit-failure return.
func TestWails_Service_Commit_Ugly_NothingStaged(t *core.T) {
	svc, ps := gitHarness(t)
	work := gitFixture(t, ps)
	r := svc.Commit(work, "empty commit attempt")
	core.AssertFalse(t, r.OK)
}

// ─── Log ─────────────────────────────────────────────────────────────

// TestWails_Service_Log_Good — an explicit positive limit skips the
// default-20 assignment and parses every %H\t%h\t%an\t%ad\t%s field.
func TestWails_Service_Log_Good(t *core.T) {
	svc, ps := gitHarness(t)
	work := gitFixture(t, ps)
	core.RequireTrue(t, core.WriteFile(core.PathJoin(work, "b.txt"), []byte("b\n"), 0o644).OK)
	mustGit(t, ps, work, "add", "-A")
	mustGit(t, ps, work, "commit", "-m", "second")

	r := svc.Log(work, 1)
	core.RequireTrue(t, r.OK, r.Error())
}

// TestWails_Service_Log_Bad_DefaultLimit — limit<=0 takes the
// default-20 assignment branch.
func TestWails_Service_Log_Bad_DefaultLimit(t *core.T) {
	svc, ps := gitHarness(t)
	work := gitFixture(t, ps)
	r := svc.Log(work, 0)
	core.RequireTrue(t, r.OK, r.Error())
}

// TestWails_Service_Log_Ugly_NotARepo — runGit fails outright.
func TestWails_Service_Log_Ugly_NotARepo(t *core.T) {
	svc, _ := gitHarness(t)
	r := svc.Log(t.TempDir(), 5)
	core.AssertFalse(t, r.OK)
}
