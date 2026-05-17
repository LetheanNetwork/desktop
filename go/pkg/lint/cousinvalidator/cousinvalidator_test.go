// SPDX-Licence-Identifier: EUPL-1.2

// cousinvalidator_test.go — fixture-driven unit tests + the
// live enforcement test that scans the rest of pkg/.
//
// Test naming follows the AX TestFilename_Function_{Good,Bad,Ugly}
// convention. The "Ugly" shape (multi-violation + opt-out mix)
// proves the rule composes correctly across a single file boundary.

package cousinvalidator

import (
	"os"
	"path/filepath"
	"testing"
)

// preparedFixtureDir copies the .go.txt fixtures (which are not
// compiled by `go test` since they don't end in .go) into a temp
// dir as real .go files. Returns the temp dir so the test can
// scan it. Errors fail the test.
//
// Each .go.txt fixture in testdata/ lands as a same-named .go
// file in the temp dir, all in one synthetic package "fixture".
func preparedFixtureDir(t *testing.T, names ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, n := range names {
		src := filepath.Join("testdata", n)
		data, err := os.ReadFile(src)
		if err != nil {
			t.Fatalf("read fixture %s: %s", src, err)
		}
		// Strip the .txt suffix so the fixture parses as a real
		// Go source file.
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

// TestCousinValidator_DetectsViolation_Bad — single forbidden
// function in a single file produces exactly one finding.
func TestCousinValidator_DetectsViolation_Bad(t *testing.T) {
	dir := preparedFixtureDir(t, "violation_isvalidid.go.txt")
	findings, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan: %s", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d: %+v", len(findings), findings)
	}
	if findings[0].FuncName != "isValidID" {
		t.Fatalf("expected FuncName isValidID, got %q", findings[0].FuncName)
	}
}

// TestCousinValidator_DetectsMultipleViolations_Ugly — two
// forbidden functions in the same file produce two findings,
// each with the correct identifier captured.
func TestCousinValidator_DetectsMultipleViolations_Ugly(t *testing.T) {
	dir := preparedFixtureDir(t, "violation_isvalidslug.go.txt")
	findings, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan: %s", err)
	}
	if len(findings) != 2 {
		t.Fatalf("expected 2 findings, got %d: %+v", len(findings), findings)
	}
	got := map[string]bool{}
	for _, f := range findings {
		got[f.FuncName] = true
	}
	if !got["isValidID"] || !got["isValidSlug"] {
		t.Fatalf("expected both isValidID + isValidSlug, got %+v", got)
	}
}

// TestCousinValidator_AllowsNolintPragma_Good — a forbidden name
// preceded by //nolint:cousinvalidator is not reported.
func TestCousinValidator_AllowsNolintPragma_Good(t *testing.T) {
	dir := preparedFixtureDir(t, "allowed_nolint.go.txt")
	findings, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan: %s", err)
	}
	if len(findings) != 0 {
		t.Fatalf("expected 0 findings (nolint pragma), got %d: %+v", len(findings), findings)
	}
}

// TestCousinValidator_AllowsUnrelatedNames_Good — names like
// isValidStage / isValidOutcome / isValidBundleName are NOT in
// ForbiddenNames and must not be reported.
func TestCousinValidator_AllowsUnrelatedNames_Good(t *testing.T) {
	dir := preparedFixtureDir(t, "allowed_unrelated.go.txt")
	findings, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan: %s", err)
	}
	if len(findings) != 0 {
		t.Fatalf("expected 0 findings (unrelated names), got %d: %+v", len(findings), findings)
	}
}

// TestCousinValidator_SkipsTestFiles_Good — fixtures named
// *_test.go are exempt from the rule (tests may use the
// forbidden names as table-driven inputs / fakes).
func TestCousinValidator_SkipsTestFiles_Good(t *testing.T) {
	dir := t.TempDir()
	body := []byte("package fixture\nfunc isValidID(s string) bool { return s != \"\" }\n")
	if err := os.WriteFile(filepath.Join(dir, "thing_test.go"), body, 0o600); err != nil {
		t.Fatalf("write test fixture: %s", err)
	}
	findings, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan: %s", err)
	}
	if len(findings) != 0 {
		t.Fatalf("expected 0 findings (test file), got %d: %+v", len(findings), findings)
	}
}

// TestCousinValidator_SkipsPathsPackage_Good — a forbidden name
// declared inside any directory named "paths" is exempt (that's
// the canonical home for the authoritative validators).
func TestCousinValidator_SkipsPathsPackage_Good(t *testing.T) {
	dir := t.TempDir()
	pathsDir := filepath.Join(dir, "paths")
	if err := os.MkdirAll(pathsDir, 0o700); err != nil {
		t.Fatalf("mkdir paths: %s", err)
	}
	body := []byte("package paths\nfunc isValidID(s string) bool { return s != \"\" }\n")
	if err := os.WriteFile(filepath.Join(pathsDir, "id.go"), body, 0o600); err != nil {
		t.Fatalf("write paths fixture: %s", err)
	}
	findings, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan: %s", err)
	}
	if len(findings) != 0 {
		t.Fatalf("expected 0 findings (paths dir), got %d: %+v", len(findings), findings)
	}
}

// TestCousinValidator_LiveScan_PkgTree_Good — the load-bearing
// enforcement pass. Scans the parent ../ pkg tree (every package
// under go/pkg/) and fails the build if any cousin-validator
// appears in production code without an opt-out pragma.
//
// On failure, the operator's fix path is one of:
//
//  1. Delete the local isValid* helper and call paths.IsValidID
//     (or the matching paths.* surface) directly. Preferred —
//     keeps the patch surface unified for future CVE response.
//  2. Add //nolint:cousinvalidator with a one-line rationale to
//     the doc comment when the local is a thin delegation to
//     the canonical paths.* surface (see Mantis #1580 / the
//     pkg/plugin/manifest.go:isValidPluginCode pattern).
//
// Per the cascade rule (cousinvalidator.go:ForbiddenNames doc),
// any new entry added to ForbiddenNames may surface findings
// that pre-date the rule. Treat those as forward-arc Mantis,
// not in-place fixes through this lane.
func TestCousinValidator_LiveScan_PkgTree_Good(t *testing.T) {
	findings, err := Scan("../..")
	if err != nil {
		t.Fatalf("Scan(../..): %s", err)
	}
	if len(findings) != 0 {
		t.Errorf("cousin-validator findings under go/pkg/ (Mantis #1505):")
		for _, f := range findings {
			t.Errorf("  %s", f.String())
		}
		t.Errorf("Resolution: replace with paths.IsValidID or add //nolint:cousinvalidator with rationale.")
	}
}

// TestCousinValidator_ScanMissingRoot_Bad — a non-existent root
// surfaces as an error (not a silent zero-findings), so a CI
// misconfiguration is caught immediately rather than masquerading
// as a clean run.
func TestCousinValidator_ScanMissingRoot_Bad(t *testing.T) {
	_, err := Scan(filepath.Join(t.TempDir(), "does-not-exist"))
	if err == nil {
		t.Fatalf("expected error for missing root, got nil")
	}
}
