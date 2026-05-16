// SPDX-Licence-Identifier: EUPL-1.2

// parity_test.go — RFC.stage-f.md §3.3.1 parity-grep contract.
// Build fails if any auth.* event-name appears in the human-debug
// core.Print stream WITHOUT a matching typed audit.Default().Record
// emission in the same package (or vice versa).
//
// Set P — every `core.Print(...)` call in pkg/serverkey + pkg/account
// containing a string literal whose value is an `event=<name>` token
// where <name> matches `^auth\.[a-z]+\.[a-z_]+$`. Captures the name.
//
// Set T — every audit.Default().Record call in the same packages
// whose Event field references an audit.EventXxx constant defined
// in pkg/audit/types.go. Captures the constant's literal value.
//
// Assertion: Set P == Set T (set equality). The test reads source
// via go/ast over the project's vendored package files; running it
// catches a divergence at build time even for cold paths that
// runtime would never trigger.
//
// When this test fails the fix is one of:
//
//   - Add a core.Print line carrying the missing event=<name> token
//     alongside the typed Record call (preferred — the debug-echo
//     surface is the operator's stderr).
//   - Add an audit.Default().Record call with the new event-name
//     constant alongside the existing core.Print line (preferred
//     when the new event-name represents a new lifecycle moment).
//   - Reserve the missing name in pkg/audit/types.go as an EventXxx
//     constant if the typed surface is what's missing.

package audit

import (
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"strings"
	"testing"

	core "dappco.re/go"
)

// parityScanRoots names the packages the parity-grep walks. Adding a
// package here is a deliberate-schema-broadening event — every new
// auth event-name in the added package becomes part of the contract.
var parityScanRoots = []string{
	"../serverkey",
	"../account",
}

// TestAuthEvents_TypedAndPrintParity_Good asserts Set P == Set T per
// RFC §3.3.1. Source-grep over the two scan roots; failures emit a
// diff-style report so the operator sees exactly which names are
// missing on each side.
func TestAuthEvents_TypedAndPrintParity_Good(t *testing.T) {
	setP := map[string]bool{} // names found in core.Print("event=<name>") lines
	setT := map[string]bool{} // names found in audit.Default().Record(Event{Event: <const>})

	// Resolve every audit.EventAuthXxx constant's literal value once
	// — Set T captures these literals, not the Go identifier, so
	// renaming the const without rebinding to the same literal value
	// surfaces as a divergence here rather than as a runtime drift.
	constLiterals := map[string]string{}
	pkgFiles, err := loadPackageFiles("./")
	if err != nil {
		t.Fatalf("load pkg/audit files: %s", err)
	}
	for _, f := range pkgFiles {
		collectAuditEventConstants(f, constLiterals)
	}

	for _, root := range parityScanRoots {
		files, err := loadPackageFiles(root)
		if err != nil {
			t.Fatalf("load %s: %s", root, err)
		}
		for _, f := range files {
			extractPrintEventNames(f, setP)
			extractRecordEventNames(f, setT, constLiterals)
		}
	}

	// Diff the two sets — names in P but not T (event-name fires
	// core.Print but no typed Record) and names in T but not P
	// (typed Record fires but no core.Print debug-echo) both
	// indicate drift.
	onlyP := setMinus(setP, setT)
	onlyT := setMinus(setT, setP)
	if len(onlyP) > 0 || len(onlyT) > 0 {
		t.Fatalf("RFC §3.3.1 parity drift:\n  only in core.Print (P): %v\n  only in audit.Record (T): %v",
			sortedKeys(onlyP), sortedKeys(onlyT))
	}
}

// loadPackageFiles parses every non-test .go file under dir using
// go/parser. Returns the AST roots so the caller can walk them.
func loadPackageFiles(dir string) ([]*ast.File, error) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, func(fi core.FsFileInfo) bool {
		name := fi.Name()
		if !strings.HasSuffix(name, ".go") {
			return false
		}
		if strings.HasSuffix(name, "_test.go") {
			return false
		}
		return true
	}, parser.AllErrors)
	if err != nil {
		return nil, err
	}
	var out []*ast.File
	for _, p := range pkgs {
		for _, f := range p.Files {
			out = append(out, f)
		}
	}
	return out, nil
}

// collectAuditEventConstants walks an AST file for top-level const
// declarations matching the pattern `EventAuthXxx = "auth.…"` and
// populates the mapping (identifier → literal value).
func collectAuditEventConstants(f *ast.File, out map[string]string) {
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}
		for _, spec := range gd.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok || len(vs.Values) == 0 {
				continue
			}
			for i, name := range vs.Names {
				if !strings.HasPrefix(name.Name, "EventAuth") {
					continue
				}
				if i >= len(vs.Values) {
					continue
				}
				lit, ok := vs.Values[i].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					continue
				}
				value := strings.Trim(lit.Value, `"`)
				if isAuthEventName(value) {
					out[name.Name] = value
				}
			}
		}
	}
}

// extractPrintEventNames walks the AST for core.Print calls whose
// first format-string argument carries an `event=auth.<sub>.<verb>`
// token. The token is parsed out + added to setP.
//
// Pattern matched: `core.Print(<anyExpr>, "event=auth.foo.bar …", …)`
// — the format string is the second arg in core.Print's signature.
func extractPrintEventNames(f *ast.File, setP map[string]bool) {
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if !isCoreSelector(call.Fun, "Print") {
			return true
		}
		if len(call.Args) < 2 {
			return true
		}
		lit, ok := call.Args[1].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		fmtStr := strings.Trim(lit.Value, `"`)
		if name := extractEventEqualsToken(fmtStr); name != "" {
			setP[name] = true
		}
		return true
	})
}

// extractRecordEventNames walks for `audit.Default().Record(audit.Event{
// Event: <ident>, …})` calls (and `RecordSync` / `RecordBatch`
// variants) where <ident> resolves to one of the constants in
// constLiterals. Adds the LITERAL VALUE to setT.
func extractRecordEventNames(f *ast.File, setT map[string]bool, constLiterals map[string]string) {
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if !isRecorderRecordCall(call.Fun) {
			return true
		}
		if len(call.Args) == 0 {
			return true
		}
		composite, ok := call.Args[0].(*ast.CompositeLit)
		if !ok {
			return true
		}
		for _, elt := range composite.Elts {
			kv, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			key, ok := kv.Key.(*ast.Ident)
			if !ok || key.Name != "Event" {
				continue
			}
			sel, ok := kv.Value.(*ast.SelectorExpr)
			if !ok {
				continue
			}
			lit, ok := constLiterals[sel.Sel.Name]
			if !ok {
				continue
			}
			setT[lit] = true
		}
		return true
	})
}

// isCoreSelector returns true when expr is `core.<name>` or `<x>.<name>`
// where x is named core. Conservative match — false on aliased imports.
func isCoreSelector(expr ast.Expr, name string) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	if sel.Sel.Name != name {
		return false
	}
	ident, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	return ident.Name == "core"
}

// isRecorderRecordCall matches `<x>.Record(...)`, `<x>.RecordSync(...)`,
// or `<x>.RecordBatch(...)` where <x> is `audit.Default()` or a chain
// ending in those. Conservative — false on aliased package imports.
func isRecorderRecordCall(expr ast.Expr) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	switch sel.Sel.Name {
	case "Record", "RecordSync", "RecordBatch":
	default:
		return false
	}
	// Receiver must be a call expression — audit.Default().Record
	// fits this shape; an alias variable bound earlier wouldn't.
	// Conservative match means we miss aliased patterns but never
	// false-match a non-audit Record() symbol from elsewhere.
	if _, ok := sel.X.(*ast.CallExpr); ok {
		return true
	}
	return false
}

// extractEventEqualsToken parses an `event=<name> …` token out of a
// space-separated format string. Returns "" when no token is present
// or the value doesn't match the auth event-name shape.
func extractEventEqualsToken(s string) string {
	for _, tok := range strings.Fields(s) {
		if !strings.HasPrefix(tok, "event=") {
			continue
		}
		val := strings.TrimPrefix(tok, "event=")
		if isAuthEventName(val) {
			return val
		}
	}
	return ""
}

// isAuthEventName matches RFC §3.3.1 — `^auth\.[a-z]+\.[a-z_]+$`.
// Conservative regex inlined to keep the parity test free of
// dependency on the package's own regex compile.
func isAuthEventName(s string) bool {
	if !strings.HasPrefix(s, "auth.") {
		return false
	}
	parts := strings.SplitN(s[len("auth."):], ".", 2)
	if len(parts) != 2 {
		return false
	}
	for _, ch := range parts[0] {
		if ch < 'a' || ch > 'z' {
			return false
		}
	}
	if len(parts[1]) == 0 {
		return false
	}
	for _, ch := range parts[1] {
		if (ch < 'a' || ch > 'z') && ch != '_' {
			return false
		}
	}
	return true
}

// setMinus returns the keys in a not present in b.
func setMinus(a, b map[string]bool) map[string]bool {
	out := map[string]bool{}
	for k := range a {
		if !b[k] {
			out[k] = true
		}
	}
	return out
}

// sortedKeys returns the map's keys in deterministic order — used to
// make test failure messages reproducible.
func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
