// SPDX-Licence-Identifier: EUPL-1.2

// serviceingress_test.go — fixture-driven unit tests + the live
// enforcement test that scans the rest of pkg/.
//
// Test naming follows the AX TestFilename_Function_{Good,Bad,Ugly}
// convention.

package serviceingress

import (
	"os"
	"path/filepath"
	"testing"
)

// preparedFixtureDir copies the named .go.txt fixtures (which are
// not compiled by `go test` since they don't end in .go) into a
// temp dir as real .go files. Returns the temp dir so the test
// can scan it. Errors fail the test.
//
// Each fixture's .txt suffix is stripped so the resulting filename
// ends in .go and the lint scanner picks it up.
func preparedFixtureDir(t *testing.T, names ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, n := range names {
		src := filepath.Join("testdata", n)
		data, err := os.ReadFile(src)
		if err != nil {
			t.Fatalf("read fixture %s: %s", src, err)
		}
		out := filepath.Join(dir, trimDotTxt(n))
		if err := os.WriteFile(out, data, 0o600); err != nil {
			t.Fatalf("write fixture %s: %s", out, err)
		}
	}
	return dir
}

func trimDotTxt(name string) string {
	const suf = ".txt"
	if len(name) > len(suf) && name[len(name)-len(suf):] == suf {
		return name[:len(name)-len(suf)]
	}
	return name
}

// TestServiceIngress_DetectsCreateViolation_Bad — an exported
// *Service.Create that reaches PathJoin without IsValidID flags.
func TestServiceIngress_DetectsCreateViolation_Bad(t *testing.T) {
	dir := preparedFixtureDir(t, "violation_create_pathjoin_no_guard.go.txt")
	findings, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan: %s", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d: %+v", len(findings), findings)
	}
	if findings[0].MethodName != "Create" {
		t.Fatalf("expected MethodName Create, got %q", findings[0].MethodName)
	}
}

// TestServiceIngress_DetectsMultiParamUpdate_Bad — an exported
// *Service.Update with multiple params, at least one a literal
// `string`, reaches PathJoin without IsValidID. Proves the rule
// fires on any-param-is-string, not just sole-string-arg shapes.
func TestServiceIngress_DetectsMultiParamUpdate_Bad(t *testing.T) {
	dir := preparedFixtureDir(t, "violation_update_pathjoin_no_guard.go.txt")
	findings, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan: %s", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d: %+v", len(findings), findings)
	}
	if findings[0].MethodName != "Update" {
		t.Fatalf("expected MethodName Update, got %q", findings[0].MethodName)
	}
}

// TestServiceIngress_AllowsStructParamV1Limit_Good — v1 documents
// that struct params are out of scope (resolving field types
// requires the type-checker). The fixture would flag under v2 but
// today must NOT flag — pins the v1 boundary so the contract is
// explicit when a future tightening lands.
func TestServiceIngress_AllowsStructParamV1Limit_Good(t *testing.T) {
	dir := preparedFixtureDir(t, "allowed_struct_param_v1_limit.go.txt")
	findings, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan: %s", err)
	}
	if len(findings) != 0 {
		t.Fatalf("expected 0 findings (struct param v1 limit), got %d: %+v",
			len(findings), findings)
	}
}

// TestServiceIngress_AllowsGuardPresent_Good — IsValidID call in
// the same body suppresses the finding even when PathJoin runs.
func TestServiceIngress_AllowsGuardPresent_Good(t *testing.T) {
	dir := preparedFixtureDir(t, "allowed_guard_present.go.txt")
	findings, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan: %s", err)
	}
	if len(findings) != 0 {
		t.Fatalf("expected 0 findings (IsValidID guard), got %d: %+v",
			len(findings), findings)
	}
}

// TestServiceIngress_AllowsNolintPragma_Good — //nolint pragma in
// the doc comment exempts the method.
func TestServiceIngress_AllowsNolintPragma_Good(t *testing.T) {
	dir := preparedFixtureDir(t, "allowed_nolint.go.txt")
	findings, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan: %s", err)
	}
	if len(findings) != 0 {
		t.Fatalf("expected 0 findings (nolint pragma), got %d: %+v",
			len(findings), findings)
	}
}

// TestServiceIngress_AllowsUnexportedMethod_Good — unexported
// methods are out of scope (cannot be reached via wails / CLI /
// MCP dispatch).
func TestServiceIngress_AllowsUnexportedMethod_Good(t *testing.T) {
	dir := preparedFixtureDir(t, "allowed_unexported_method.go.txt")
	findings, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan: %s", err)
	}
	if len(findings) != 0 {
		t.Fatalf("expected 0 findings (unexported), got %d: %+v",
			len(findings), findings)
	}
}

// TestServiceIngress_AllowsNoParamMethod_Good — exported methods
// with no parameters cannot carry user-supplied IDs into PathJoin.
func TestServiceIngress_AllowsNoParamMethod_Good(t *testing.T) {
	dir := preparedFixtureDir(t, "allowed_no_params.go.txt")
	findings, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan: %s", err)
	}
	if len(findings) != 0 {
		t.Fatalf("expected 0 findings (nullary method), got %d: %+v",
			len(findings), findings)
	}
}

// TestServiceIngress_AllowsOtherReceiver_Good — methods on a
// receiver type other than Service are out of scope.
func TestServiceIngress_AllowsOtherReceiver_Good(t *testing.T) {
	dir := preparedFixtureDir(t, "allowed_other_receiver.go.txt")
	findings, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan: %s", err)
	}
	if len(findings) != 0 {
		t.Fatalf("expected 0 findings (Helper receiver), got %d: %+v",
			len(findings), findings)
	}
}

// TestServiceIngress_SkipsWailsFile_Good — a file literally named
// wails.go is exempt. The same violating body that flags from
// service.go must NOT flag from wails.go.
func TestServiceIngress_SkipsWailsFile_Good(t *testing.T) {
	src := filepath.Join("testdata", "violation_create_pathjoin_no_guard.go.txt")
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read fixture: %s", err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "wails.go"), data, 0o600); err != nil {
		t.Fatalf("write wails fixture: %s", err)
	}
	findings, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan: %s", err)
	}
	if len(findings) != 0 {
		t.Fatalf("expected 0 findings (wails.go exempt), got %d: %+v",
			len(findings), findings)
	}
}

// TestServiceIngress_SkipsTestFiles_Good — *_test.go files are
// exempt (tests may exercise the unguarded path on purpose).
func TestServiceIngress_SkipsTestFiles_Good(t *testing.T) {
	src := filepath.Join("testdata", "violation_create_pathjoin_no_guard.go.txt")
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read fixture: %s", err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "thing_test.go"), data, 0o600); err != nil {
		t.Fatalf("write test fixture: %s", err)
	}
	findings, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan: %s", err)
	}
	if len(findings) != 0 {
		t.Fatalf("expected 0 findings (test file), got %d: %+v",
			len(findings), findings)
	}
}

// TestServiceIngress_SkipsPathsPackage_Good — files under any
// directory named "paths" are exempt (canonical home for the
// authoritative validators that PathJoin without recursing).
func TestServiceIngress_SkipsPathsPackage_Good(t *testing.T) {
	src := filepath.Join("testdata", "violation_create_pathjoin_no_guard.go.txt")
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read fixture: %s", err)
	}
	dir := t.TempDir()
	pathsDir := filepath.Join(dir, "paths")
	if err := os.MkdirAll(pathsDir, 0o700); err != nil {
		t.Fatalf("mkdir paths: %s", err)
	}
	if err := os.WriteFile(filepath.Join(pathsDir, "service.go"), data, 0o600); err != nil {
		t.Fatalf("write paths fixture: %s", err)
	}
	findings, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan: %s", err)
	}
	if len(findings) != 0 {
		t.Fatalf("expected 0 findings (paths dir), got %d: %+v",
			len(findings), findings)
	}
}

// TestServiceIngress_LiveScan_PkgTree_Good — the load-bearing
// enforcement pass. Scans the parent ../ pkg tree (every package
// under go/pkg/) and fails the build if any exported *Service
// method reaches PathJoin without an IsValidID guard from a
// non-wails.go ingress.
//
// On failure, the operator's fix path is one of:
//
//  1. Move the ingress to wails.go (canonical — the wails-layer
//     is the validation boundary). The unguarded body becomes a
//     downstream helper that wails.go calls AFTER IsValidID.
//  2. Add paths.IsValidID(input.ID) (or the matching shape) at
//     the top of the method body, before any PathJoin call.
//  3. Add //nolint:serviceingress with a one-line rationale when
//     the method does NOT actually carry user-supplied IDs into
//     PathJoin (e.g. caller-internal constants only).
func TestServiceIngress_LiveScan_PkgTree_Good(t *testing.T) {
	findings, err := Scan("../..")
	if err != nil {
		t.Fatalf("Scan(../..): %s", err)
	}
	if len(findings) != 0 {
		t.Errorf("serviceingress findings under go/pkg/ (Mantis #1609):")
		for _, f := range findings {
			t.Errorf("  %s", f.String())
		}
		t.Errorf("Resolution: move ingress to wails.go, call paths.IsValidID first, " +
			"or add //nolint:serviceingress with rationale.")
	}
}

// TestServiceIngress_ScanMissingRoot_Bad — a non-existent root
// surfaces as an error rather than a silent zero-findings clean
// pass, catching CI misconfiguration immediately.
func TestServiceIngress_ScanMissingRoot_Bad(t *testing.T) {
	_, err := Scan(filepath.Join(t.TempDir(), "does-not-exist"))
	if err == nil {
		t.Fatalf("expected error for missing root, got nil")
	}
}
