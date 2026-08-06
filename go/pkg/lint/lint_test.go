// SPDX-Licence-Identifier: EUPL-1.2

// Shared test harness for pkg/lint's external test package. Mirrors
// pkg/git's gitHarness — a real, started dappco.re/go/process Service
// registered under "process" on a real *core.Core, and a fake
// `core-lint` executable placed first on $PATH so
// core.App{}.Find(...) resolves it deterministically regardless of
// what's actually installed on the machine running the test.
package lint_test

import (
	core "dappco.re/go"
	"dappco.re/go/process"
	subject "dappco.re/lthn/desktop/pkg/lint"
)

// lintHarness returns a *subject.Service bound to a *core.Core with a
// real, started process.Service registered under "process".
func lintHarness(t *core.T) *subject.Service {
	t.Helper()
	c := core.New()
	r := process.NewService(process.Options{})(c)
	core.RequireTrue(t, r.OK)
	ps := r.Value.(*process.Service)
	core.RequireTrue(t, ps.OnStartup(core.Background()).OK)
	core.RequireTrue(t, c.RegisterService("process", ps).OK)
	return subject.NewService(c)
}

// fakeLintOnPath writes a `core-lint` shell script (the given POSIX
// shell body, after a `#!/bin/sh` line) into a fresh TempDir and puts
// that directory first on $PATH, so findLintBinary's PATH search
// resolves to it deterministically — independent of whatever
// core-lint may or may not be installed on the machine running the
// test (Snider's own dev box has a real one at a hardcoded canonical
// path; see wails_test.go's doc comment for the coverage
// implications of that).
func fakeLintOnPath(t *core.T, body string) {
	t.Helper()
	dir := t.TempDir()
	script := core.PathJoin(dir, "core-lint")
	core.RequireTrue(t, core.WriteFile(script, []byte("#!/bin/sh\n"+body+"\n"), 0o755).OK)
	t.Setenv("PATH", dir)
}

// ─── NewService / Register ──────────────────────────────────────────

func TestLint_NewService_Good(t *core.T) {
	svc := subject.NewService(core.New())
	core.AssertNotNil(t, svc)
	core.AssertEqual(t, "Lint", svc.ServiceName())
}

func TestLint_NewService_Bad(t *core.T) {
	svc := subject.NewService(nil)
	core.AssertNotNil(t, svc)
}

func TestLint_NewService_Ugly(t *core.T) {
	a := subject.NewService(core.New())
	b := subject.NewService(core.New())
	core.AssertTrue(t, a != b, "each call constructs a distinct instance")
}

func TestLint_Register_Good(t *core.T) {
	r := subject.Register(core.New())
	core.AssertTrue(t, r.OK)
	svc, ok := r.Value.(*subject.Service)
	core.AssertTrue(t, ok)
	core.AssertNotNil(t, svc)
}

func TestLint_Register_Bad(t *core.T) {
	r := subject.Register(nil)
	core.AssertTrue(t, r.OK)
	_, ok := r.Value.(*subject.Service)
	core.AssertTrue(t, ok)
}

func TestLint_Register_Ugly(t *core.T) {
	c := core.New()
	r1 := subject.Register(c)
	r2 := subject.Register(c)
	core.AssertTrue(t, r1.OK)
	core.AssertTrue(t, r2.OK)
	core.AssertTrue(t, r1.Value.(*subject.Service) != r2.Value.(*subject.Service))
}
