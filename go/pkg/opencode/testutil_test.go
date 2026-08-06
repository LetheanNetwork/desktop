// SPDX-Licence-Identifier: EUPL-1.2

// testutil_test.go — shared hermetic test harness for the coverage
// sweep. Three seams make the rest of the Service surface unit-
// testable without docker, DuckDB-on-the-real-home, or a live
// opencode binary:
//
//  1. resetKV rebinds the process-global profile/auth KV singleton
//     (profile.go kvOnce/kvErr/kvInst) to a fresh temp $HOME. The
//     singleton is shared across every test in this binary (sync.Once
//     fires exactly once per process), so every test that touches it
//     — directly or via a Service method — MUST call resetKV first and
//     MUST NOT run in parallel with another such test.
//  2. newTestCore / newTestService wire a *core.Core with
//     process.Service registered (so Service.proc() resolves) and an
//     orm.Memium mounted as "default" (so orm.Of[T] calls succeed
//     in-memory, no DuckDB file).
//  3. fakeRuntime writes a tiny shell script standing in for `docker`
//     (or any Options.Runtime target) so Start/Stop/Reconcile/Upgrade
//     never shell out to a real container runtime. This is the
//     package's OWN pre-existing seam (Options.Runtime / s.runtime())
//     — no production code changes needed to use it.
package opencode

import (
	"os"
	"path/filepath"
	"testing"

	core "dappco.re/go"
	"dappco.re/go/orm"
	"dappco.re/go/process"
)

// resetKV points $HOME at a fresh t.TempDir() and rewinds the
// package-level kv() singleton (kvOnce/kvErr/kvInst — all unexported
// vars in profile.go, reachable because this file is `package
// opencode`) so the next kv() call re-opens a DuckDB file under the
// new temp home instead of reusing whatever a previous test bound.
// Resets again on t.Cleanup so the NEXT test in the binary also gets
// a clean singleton rather than inheriting this test's store.
//
// Every test that calls resetKV touches process-global state and
// must run non-parallel (the default — none of these tests call
// t.Parallel()).
func resetKV(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	kvOnce = core.Once{}
	kvErr = nil
	kvInst = nil
	t.Cleanup(func() {
		kvOnce = core.Once{}
		kvErr = nil
		kvInst = nil
	})
	return home
}

// breakKV forces the NEXT kv() call to fail by pointing $HOME at an
// empty string — core.UserHomeDir()'s unix implementation errors when
// $HOME is unset, so kv() hits its "home dir resolve failed" branch
// with no filesystem fixture needed. Rewinds the kv() singleton so a
// previously-opened store (e.g. from an earlier resetKV in the same
// test) doesn't mask the failure.
func breakKV(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", "")
	kvOnce = core.Once{}
	kvErr = nil
	kvInst = nil
}

// newTestCore returns a *core.Core with process.Service registered
// under "process" (the name Service.proc() looks up) and a fresh
// orm.Memium mounted as "default". Hermetic — no DuckDB, no docker.
func newTestCore(t *testing.T) *core.Core {
	t.Helper()
	c := core.New(
		core.WithName("process", process.NewService(process.Options{})),
	)
	mem := orm.NewMemium()
	if r := orm.Mount(c, "default", mem); !r.OK {
		t.Fatalf("orm.Mount failed: %s", r.Error())
	}
	return c
}

// newTestCoreNoORM is newTestCore without the Memium mount — used by
// the small number of tests that deliberately want orm.Of[T] calls to
// fail (Mantis-style "storage backend absent" fault injection) without
// reaching for capability masking that Memium doesn't model.
func newTestCoreNoORM(t *testing.T) *core.Core {
	t.Helper()
	return core.New(
		core.WithName("process", process.NewService(process.Options{})),
	)
}

// newTestService builds a fully-wired *Service: resetKV (fresh temp
// $HOME + rewound kv() singleton) + newTestCore, then the real
// NewService(opts) factory (which calls SeedDefaultProfile, so the
// "default" profile always exists afterwards). Set opts.Runtime to a
// fakeRuntime(t, ...) path for any test that reaches Start/Stop/
// Reconcile/Upgrade's process.Run calls.
func newTestService(t *testing.T, opts Options) *Service {
	t.Helper()
	resetKV(t)
	c := newTestCore(t)
	r := NewService(opts)(c)
	if !r.OK {
		t.Fatalf("NewService failed: %s", r.Error())
	}
	return r.Value.(*Service)
}

// fakeRuntime writes a small POSIX shell script standing in for
// `docker` (or podman, or whatever Options.Runtime names) and returns
// its absolute path. body is the script logic after the shebang; it
// sees the runtime's argv as "$@" exactly like a real container
// runtime invocation (e.g. "run" "-d" "-p" ... or "ps" "--filter" ...
// or "pull" <ref>). This is the package's own pre-existing
// Options.Runtime testability seam (opencode.go's s.runtime(),
// defaulting to "docker" only when Options.Runtime is empty) — using
// it requires no production code change. Never invokes a real
// container runtime or opencode binary.
func fakeRuntime(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "fake-runtime")
	script := "#!/bin/sh\n" + body + "\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake runtime script: %v", err)
	}
	return path
}
