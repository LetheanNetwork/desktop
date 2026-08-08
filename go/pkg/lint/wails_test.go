// SPDX-Licence-Identifier: EUPL-1.2

// Real behavioural tests for wails.go's Wails3 surface (ServiceName /
// ServiceStartup / ServiceShutdown / Run / Catalog).
//
// wails_example_test.go's Test* functions previously paired 1:1 with
// its Example* functions and asserted only on a method VALUE's %T
// formatting:
//
//	ref := (*subject.Service).Run
//	typeName := core.Sprintf("%T", ref)
//	core.AssertContains(t, typeName, "func")
//
// `ref` is a bound-method-expression value — taking it never calls
// Run, so none of its statements ever executed. That's the "fake
// test" mechanism this coverage lane was tasked to find: 87 tests
// passing across pkg/git + pkg/lint + pkg/tools with 0.0% statement
// coverage, because every one of them independently satisfies itself
// on the reflected type name and stops. This file replaces the
// pattern with tests that drive Run/Catalog against a real
// process.Service and a fake `core-lint` script placed first on
// $PATH (see lint_test.go's fakeLintOnPath).
package lint_test

import (
	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/lint"
)

// ─── ServiceName / ServiceStartup / ServiceShutdown ─────────────────

func TestWails_Service_ServiceName_Good(t *core.T) {
	svc := lintHarness(t)
	core.AssertEqual(t, "Lint", svc.ServiceName())
}

func TestWails_Service_ServiceStartup_Good(t *core.T) {
	svc := lintHarness(t)
	r := svc.ServiceStartup(core.Background(), nil)
	core.AssertTrue(t, r.OK)
}

func TestWails_Service_ServiceShutdown_Good(t *core.T) {
	svc := lintHarness(t)
	r := svc.ServiceShutdown()
	core.AssertTrue(t, r.OK)
}

// ─── Run ─────────────────────────────────────────────────────────────

// TestWails_Service_Run_Good — a real directory, both optional
// filters set, and a fake core-lint that emits one issue: exercises
// the full success path including the severity-counts map.
func TestWails_Service_Run_Good(t *core.T) {
	svc := lintHarness(t)
	fakeLintOnPath(t, `echo '[{"rule_id":"R1","title":"t","severity":"warn","file":"f.go","line":3,"match":"m","fix":"x"}]'`)

	r := svc.Run(t.TempDir(), "warn", "go")
	core.RequireTrue(t, r.OK, r.Error())
	out, ok := r.Value.(subject.RunOutput)
	core.RequireTrue(t, ok)
	core.AssertEqual(t, 1, out.Total)
	core.AssertEqual(t, 1, out.Counts["warn"])
}

func TestWails_Service_Run_Bad_EmptyPath(t *core.T) {
	svc := lintHarness(t)
	r := svc.Run("   ", "", "")
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "path required")
}

func TestWails_Service_Run_Bad_PathNotFound(t *core.T) {
	svc := lintHarness(t)
	r := svc.Run(core.PathJoin(t.TempDir(), "does-not-exist"), "", "")
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "path is not a directory")
}

// TestWails_Service_Run_Bad_PathIsFile — a real, existing path that
// is a file (not a directory) is rejected by the same message as a
// missing path.
func TestWails_Service_Run_Bad_PathIsFile(t *core.T) {
	svc := lintHarness(t)
	file := core.PathJoin(t.TempDir(), "notadir.txt")
	core.RequireTrue(t, core.WriteFile(file, []byte("x"), 0o644).OK)
	r := svc.Run(file, "", "")
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "path is not a directory")
}

func TestWails_Service_Run_Ugly_InvalidJSON(t *core.T) {
	svc := lintHarness(t)
	fakeLintOnPath(t, "echo 'not json'")
	r := svc.Run(t.TempDir(), "", "")
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "parse json")
}

// TestWails_Service_Run_Ugly_NullIssues — core-lint emitting the
// JSON literal `null` unmarshals to a nil slice; Run must still hand
// back an empty (non-nil) Issues slice.
func TestWails_Service_Run_Ugly_NullIssues(t *core.T) {
	svc := lintHarness(t)
	fakeLintOnPath(t, "echo 'null'")
	r := svc.Run(t.TempDir(), "", "")
	core.RequireTrue(t, r.OK, r.Error())
	out, ok := r.Value.(subject.RunOutput)
	core.RequireTrue(t, ok)
	core.AssertNotNil(t, out.Issues)
	core.AssertLen(t, out.Issues, 0)
}

// TestWails_Service_Run_Ugly_RunLintFailure — core-lint exiting
// non-zero surfaces as a failed Result (process.Service.Run discards
// captured output on a non-zero exit, so there is no "failed but
// still emitted JSON" path to reach through runLint — see
// lint_internal_test.go's TestRunLint_Ugly_CommandFails).
func TestWails_Service_Run_Ugly_RunLintFailure(t *core.T) {
	svc := lintHarness(t)
	fakeLintOnPath(t, "exit 1")
	r := svc.Run(t.TempDir(), "", "")
	core.AssertFalse(t, r.OK)
}

// ─── Catalog ─────────────────────────────────────────────────────────

func TestWails_Service_Catalog_Good(t *core.T) {
	svc := lintHarness(t)
	fakeLintOnPath(t, `echo '[{"rule_id":"R1","title":"t","severity":"warn","description":"d"}]'`)
	r := svc.Catalog()
	core.RequireTrue(t, r.OK, r.Error())
	entries, ok := r.Value.([]subject.CatalogEntry)
	core.RequireTrue(t, ok)
	core.AssertLen(t, entries, 1)
}

func TestWails_Service_Catalog_Bad_RunLintFailure(t *core.T) {
	svc := lintHarness(t)
	fakeLintOnPath(t, "exit 1")
	r := svc.Catalog()
	core.AssertFalse(t, r.OK)
}

func TestWails_Service_Catalog_Ugly_InvalidJSON(t *core.T) {
	svc := lintHarness(t)
	fakeLintOnPath(t, "echo 'not json'")
	r := svc.Catalog()
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "parse json")
}

// TestWails_Service_Catalog_Ugly_NullEntries — same nil-to-empty
// normalisation as Run's issues slice.
func TestWails_Service_Catalog_Ugly_NullEntries(t *core.T) {
	svc := lintHarness(t)
	fakeLintOnPath(t, "echo 'null'")
	r := svc.Catalog()
	core.RequireTrue(t, r.OK, r.Error())
	entries, ok := r.Value.([]subject.CatalogEntry)
	core.RequireTrue(t, ok)
	core.AssertLen(t, entries, 0)
}

// NOTE on findLintBinary's not-found branches (Run/Catalog's
// `binary == ""` checks): the fallback list is $HOME-relative only,
// so a fake $HOME plus an empty $PATH makes "" reachable on any box —
// lint_internal_test.go's TestFindLintBinary_Ugly_* asserts it
// directly.
