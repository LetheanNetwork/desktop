// SPDX-Licence-Identifier: EUPL-1.2

// Real behavioural tests for wails.go's Status method.
//
// wails_example_test.go's Test* functions previously paired 1:1 with
// its Example* functions and asserted only on a method VALUE's %T
// formatting — the bound-method-expression never gets called, so
// none of Status's statements ever executed. 87 tests were passing
// across pkg/git + pkg/lint + pkg/tools with 0.0% coverage on the
// strength of that mechanism; pkg/repos's Status was the one method
// caught by it here (RegisterSource/collectSourcePaths were already
// covered honestly by sources_test.go, hence the package's 21.9%
// baseline). This file replaces the pattern with tests that drive
// Status against real (TempDir-hermetic) fixtures.
package repos_test

import (
	core "dappco.re/go"
	"dappco.re/go/process"
	subject "dappco.re/lthn/desktop/pkg/repos"
)

// TestWails_Service_Status_Good_ExplicitPaths — input.Paths bypasses
// the scan entirely; a real one-repo fixture exercises the non-empty
// final-return branch and the statuses() call.
func TestWails_Service_Status_Good_ExplicitPaths(t *core.T) {
	core.RequireTrue(t, process.Init(core.New()).OK)
	svc := subject.NewService(core.New())
	dir := t.TempDir()
	r := process.Run(core.Background(), "git", "-C", dir, "init", "-b", "main")
	core.RequireTrue(t, r.OK, r.Error())

	out := svc.Status(subject.StatusInput{Paths: []string{dir}})
	core.RequireTrue(t, out.OK, out.Error())
	res, ok := out.Value.(subject.StatusOutput)
	core.RequireTrue(t, ok)
	core.AssertEqual(t, 1, res.Scanned)
	core.AssertLen(t, res.Repos, 1)
}

// TestWails_Service_Status_Bad_EmptyEverything — no Paths, no Roots,
// and an isolated $HOME with no canonical Code/* trees under it:
// every path source comes back empty, taking the early
// zero-repos-zero-scanned return.
func TestWails_Service_Status_Bad_EmptyEverything(t *core.T) {
	svc := subject.NewService(core.New())
	t.Setenv("HOME", t.TempDir())

	out := svc.Status(subject.StatusInput{})
	core.RequireTrue(t, out.OK, out.Error())
	res, ok := out.Value.(subject.StatusOutput)
	core.RequireTrue(t, ok)
	core.AssertLen(t, res.Repos, 0)
	core.AssertEqual(t, 0, res.Scanned)
}

// TestWails_Service_Status_Ugly_RootsAndSources — explicit Roots
// (not Paths) drives the scanRoots branch, and a registered
// SourceProvider contributes both a path that duplicates the scan
// result (exercising the dedup "continue") and a genuinely new one
// (exercising the append).
func TestWails_Service_Status_Ugly_RootsAndSources(t *core.T) {
	svc := subject.NewService(core.New())
	root := t.TempDir()
	repoDir := core.PathJoin(root, "hasgit")
	core.RequireTrue(t, core.MkdirAll(core.PathJoin(repoDir, ".git"), 0o755).OK)

	extra := core.PathJoin(t.TempDir(), "extra-source-path")
	svc.RegisterSource("test-source", func(_ core.Context) []string {
		return []string{repoDir, extra} // repoDir dupes the scan; extra is new
	})

	out := svc.Status(subject.StatusInput{Roots: []string{root}})
	core.RequireTrue(t, out.OK, out.Error())
	res, ok := out.Value.(subject.StatusOutput)
	core.RequireTrue(t, ok)
	core.AssertEqual(t, 2, res.Scanned) // repoDir (deduped) + extra
	core.AssertLen(t, res.Repos, 2)
}
