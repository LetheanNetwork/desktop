// SPDX-Licence-Identifier: EUPL-1.2

// Internal-package tests for lint.go's unexported helpers: proc(),
// findLintBinary(), and runLint(). Hermetic: $PATH and $HOME are
// redirected via t.Setenv, and the "binary" is a real (TempDir,
// executable-bit) POSIX shell script — no network, no fixed ports.
package lint

import (
	core "dappco.re/go"
	"dappco.re/go/process"
)

func procHarness(t *core.T) (*Service, *process.Service) {
	t.Helper()
	c := core.New()
	r := process.NewService(process.Options{})(c)
	core.RequireTrue(t, r.OK)
	ps := r.Value.(*process.Service)
	core.RequireTrue(t, ps.OnStartup(core.Background()).OK)
	core.RequireTrue(t, c.RegisterService("process", ps).OK)
	return NewService(c), ps
}

func fakeLint(t *core.T, body string) {
	t.Helper()
	dir := t.TempDir()
	script := core.PathJoin(dir, coreLintBinary)
	core.RequireTrue(t, core.WriteFile(script, []byte("#!/bin/sh\n"+body+"\n"), 0o755).OK)
	t.Setenv("PATH", dir)
}

// ─── proc ────────────────────────────────────────────────────────────

func TestProc_Bad_NoCore(t *core.T) {
	svc := NewService(nil)
	core.AssertNil(t, svc.proc())
}

func TestProc_Good_ProcessRegistered(t *core.T) {
	svc, ps := procHarness(t)
	core.AssertSame(t, ps, svc.proc())
}

// ─── findLintBinary ──────────────────────────────────────────────────

func TestFindLintBinary_Good_OnPath(t *core.T) {
	fakeLint(t, "exit 0")
	got := findLintBinary()
	core.AssertContains(t, got, coreLintBinary)
}

// TestFindLintBinary_Ugly_FallsBackWhenNotOnPath — with nothing on
// $PATH and a fresh, repo-free $HOME, the PATH search and the
// $HOME-derived candidates all miss, so the search reports "not
// found" honestly instead of resolving somebody else's machine
// layout. Every fallback is $HOME-relative, which is what makes this
// assertable on any box.
func TestFindLintBinary_Ugly_FallsBackWhenNotOnPath(t *core.T) {
	t.Setenv("PATH", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	core.AssertEqual(t, "", findLintBinary())
}

// ─── runLint ─────────────────────────────────────────────────────────

func TestRunLint_Bad_ProcUnavailable(t *core.T) {
	svc := NewService(core.New()) // core present, but no "process" service registered
	r := svc.runLint("core-lint", "lint", "catalog", "list")
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "process service unavailable")
}

func TestRunLint_Good_Success(t *core.T) {
	svc, _ := procHarness(t)
	fakeLint(t, "echo '[]'")
	binary := findLintBinary()
	r := svc.runLint(binary, "ignored")
	core.RequireTrue(t, r.OK, r.Error())
	core.AssertEqual(t, "[]\n", r.Value.(string))
}

func TestRunLint_Ugly_CommandFails(t *core.T) {
	svc, _ := procHarness(t)
	fakeLint(t, "echo boom >&2\nexit 1")
	binary := findLintBinary()
	r := svc.runLint(binary, "ignored")
	core.AssertFalse(t, r.OK)
}
