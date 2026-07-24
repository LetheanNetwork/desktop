// SPDX-Licence-Identifier: EUPL-1.2

// Package cousinvalidator is the lint surface for "no local
// isValidID / isValidSlug cousin-validators outside pkg/paths".
//
// Background — paths.IsValidID (Cerberus pass-9 #1486) is the single
// authoritative shape-guard for user-supplied IDs that land in
// core.PathJoin. The fix has been re-discovered manually 5+ times
// (sales/pipeline, marketing, runbooks, incidents, office/mail,
// office/documents — Cerberus #1486, #1498, #1501 META). Each time
// a new package gains a local isValid* helper duplicating the same
// rules. Local clones do not inherit hardening patches applied to
// paths.IsValidID, so the duplication is a CVE-patch-bypass class.
//
// This package is purely a lint surface. It walks Go source under
// a target root, parses each file with go/parser, and reports any
// top-level function declaration whose name matches a forbidden
// cousin-validator identifier. The rule is name-based, not body-
// based — names like "isValidID" or "isValidSlug" are reserved for
// the canonical paths.* surface, and any unscoped occurrence is a
// signal that a cousin-validator is being reintroduced.
//
// Scope rules:
//
//   - Only files under the scanned root are considered.
//   - Files in pkg/paths/ are exempt (that's the canonical home).
//   - *_test.go files are exempt (tests may declare fixture
//     helpers with matching names).
//   - A function decorated with the //nolint:cousinvalidator
//     pragma in a leading comment is exempt — used when a local
//     wrapper thinly delegates to paths.IsValidID with a documented
//     rationale (e.g. pkg/plugin/manifest.go:isValidPluginCode).
//
// The wired-up enforcement lives in cousinvalidator_test.go which
// scans the parent ../ pkg tree and fails the standard Go test
// suite on any new finding. That keeps the rule load-bearing
// against the existing `task test:go` / `go test ./pkg/...` pass
// without introducing new tooling outside the project's existing
// pipeline.
//
// Usage example (programmatic scan):
//
//	findings, err := cousinvalidator.Scan("../")
//	if err != nil { core.Print(err.Error()) }
//	for _, f := range findings {
//	    core.Print(f.String())
//	}

package cousinvalidator

import (
	"go/ast"
	"go/parser"
	"go/token"

	core "dappco.re/go"
)

// ForbiddenNames is the closed set of function identifiers that must
// not appear at top-level in any non-test file outside pkg/paths/.
//
// Each entry mirrors an exported helper on pkg/paths/ that the
// cousin would silently shadow. Adding a new entry here is a
// deliberate-schema-broadening event — every time pkg/paths/ grows
// a new IsValid* surface that has historically been re-implemented
// in callers, add the cousin name here so future drift fails the
// build immediately.
//
// Current list (Mantis #1505 — derived from #1486, #1498, #1580
// cousin-cleanup history):
//
//   - isValidID   → paths.IsValidID    (Cerberus #1486)
//   - isValidSlug → paths.IsValidID    (the recurring drift class)
//
// NOT in the list (intentionally — these are NOT cousins of paths.IsValidID):
//
//   - isValidPluginCode    → already delegates via paths.IsValidPluginCode (#1580)
//   - isValidStage         → closed-set enum lookup, not shape validation
//   - isValidOutcome       → closed-set enum lookup
//   - isValidBundleName    → bundle-name shape (different rule set)
//   - isValidCustomElementTag → W3C tag shape (different rule set)
//   - isValidPluginViewID  → composite identity rule (plugin-code-prefix)
//   - isValidBinaryPath    → relative-path-shape, not ID-shape
//   - isValidPostMessageOrigin → URL shape, not ID-shape
var ForbiddenNames = []string{
	"isValidID",
	"isValidSlug",
}

// NolintPragma is the leading-comment marker that exempts a single
// function from the rule. Must appear as its own line in the doc
// comment immediately preceding the func decl, alongside a one-line
// rationale (the human-readable why). Example:
//
//	// isValidPluginCode wraps paths.IsValidPluginCode so existing
//	// call sites stay unchanged. See Mantis #1580.
//	//nolint:cousinvalidator — thin delegation to paths surface
//	func isValidPluginCode(code string) bool {
//	    return paths.IsValidPluginCode(code)
//	}
const NolintPragma = "//nolint:cousinvalidator"

// PathsPackageDir is the directory leaf (within the module root)
// that holds the canonical paths.* surface. Files under this dir
// are exempt from the rule — that is where the authoritative
// validators live.
const PathsPackageDir = "paths"

// Finding describes a single rule violation: a forbidden function
// name declared in a non-exempt file.
type Finding struct {
	// File is the path to the offending Go file (as the walker
	// observed it; may be relative to the scan root).
	File string
	// Line is the 1-based line number of the func declaration.
	Line int
	// FuncName is the identifier that matched ForbiddenNames.
	FuncName string
}

// String formats the finding in compiler-style `file:line: message`
// form so a CI log line is one-click jumpable in most editors.
//
// Usage example:
//
//	core.Print(finding.String())
func (f Finding) String() string {
	return core.Sprintf(
		"%s:%d: forbidden cousin-validator %q — use paths.IsValidID instead "+
			"(Mantis #1505; add //nolint:cousinvalidator with rationale to opt out)",
		f.File, f.Line, f.FuncName)
}

// Scan walks every non-exempt Go source file under root and returns
// the set of findings. An empty slice (with nil error) means clean.
//
// Exemptions applied:
//
//   - *_test.go files
//   - files under any directory leaf equal to PathsPackageDir
//   - func decls preceded by NolintPragma
//
// Walk errors surface as a core error — typically a missing root
// or a permission failure. Per-file parse errors are absorbed as
// findings-of-zero (a non-parseable file cannot define a forbidden
// func; the build would fail elsewhere).
//
// Usage example:
//
//	findings, err := cousinvalidator.Scan("/path/to/go/pkg")
//	if err != nil { return core.Fail(err) }
//	for _, f := range findings { core.Print(f.String()) }
func Scan(root string) ([]Finding, error) {
	var out []Finding
	walkR := core.PathWalkDir(root, func(path string, d core.FsDirEntry, err error) error {
		if err != nil {
			// Root-level error (missing scan root, permission
			// denied on the top dir) propagates so a CI mis-
			// configuration is caught immediately; per-entry
			// errors deeper in the tree absorb so a single
			// nested permission-denied doesn't kill the scan.
			if path == root {
				return err
			}
			return nil
		}
		if d == nil {
			return nil
		}
		if d.IsDir() {
			if d.Name() == PathsPackageDir {
				return core.PathSkipDir
			}
			return nil
		}
		name := d.Name()
		if !core.HasSuffix(name, ".go") {
			return nil
		}
		if core.HasSuffix(name, "_test.go") {
			return nil
		}
		findings := scanFile(path)
		out = append(out, findings...)
		return nil
	})
	if !walkR.OK {
		return nil, core.E("cousinvalidator.Scan",
			"PathWalkDir under "+root, walkR.Err())
	}
	return out, nil
}

// scanFile parses a single Go source file and returns any findings
// it produces. Parse errors emit no findings — the build will
// reject the file at compile time so the lint pass doesn't need to
// double-report.
func scanFile(path string) []Finding {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return nil
	}
	var out []Finding
	for _, decl := range f.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		if fd.Name == nil {
			continue
		}
		if !isForbiddenName(fd.Name.Name) {
			continue
		}
		if hasNolintPragma(fd.Doc) {
			continue
		}
		pos := fset.Position(fd.Pos())
		out = append(out, Finding{
			File:     path,
			Line:     pos.Line,
			FuncName: fd.Name.Name,
		})
	}
	return out
}

// isForbiddenName reports whether name appears in ForbiddenNames.
//
// Usage example:
//
//	if isForbiddenName(fd.Name.Name) { ... }
func isForbiddenName(name string) bool {
	for _, n := range ForbiddenNames {
		if name == n {
			return true
		}
	}
	return false
}

// hasNolintPragma reports whether the func's doc comment contains
// the NolintPragma marker on any line. Returns false when doc is
// nil (no comment block above the decl).
//
// Usage example:
//
//	if hasNolintPragma(fd.Doc) { continue }
func hasNolintPragma(doc *ast.CommentGroup) bool {
	if doc == nil {
		return false
	}
	for _, c := range doc.List {
		// Strip the comment slashes; CommentGroup carries the raw
		// "// text" / "/* text */" form. Match on prefix because
		// Go-style would carry "// text" with the marker as part
		// of the text.
		text := core.Trim(core.TrimPrefix(c.Text, "//"))
		if core.HasPrefix(text, "nolint:cousinvalidator") {
			return true
		}
		// Tolerate the slashes-included form as a defensive match
		// in case a contributor types the pragma exactly as the
		// constant defines it.
		if core.Contains(c.Text, NolintPragma) {
			return true
		}
	}
	return false
}
