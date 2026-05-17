// SPDX-Licence-Identifier: EUPL-1.2

// Package serviceingress is the lint surface for "exported *Service
// methods that reach core.PathJoin must call paths.IsValidID first,
// OR live in wails.go".
//
// Background — paths.IsValidID is the single authoritative shape-
// guard for user-supplied IDs that land in core.PathJoin (Cerberus
// #1486, sibling rule [[cousinvalidator]]). The wails-layer is the
// boundary where untrusted UI input enters the service; today the
// convention is "IsValidID lives at the wails.go ingress". H#85 +
// Mantis #1505 cleaned the surface; the convention is not enforced.
//
// Cerberus #24 Finding 2 (Mantis #1609 forward-arc) flagged the
// gap: future CLI / MCP / plugin-gateway ingress into a
// *Service.Create / Update / Delete method with raw input.ID would
// bypass IsValidID and reach PathJoin via helpers in service.go.
// This package closes that hole at lint-time.
//
// The rule (v1, syntactic):
//
//   - For each exported method on receiver *Service / Service in
//     a Go file that is NOT named wails.go and NOT a _test.go file
//     and NOT under pkg/paths/, with at least one literal `string`
//     parameter, scan the method body for any call selector ending
//     in PathJoin (core.PathJoin or the package alias form).
//   - If a PathJoin call is present AND no call to IsValidID
//     (paths.IsValidID / IsValidID / any selector ending in
//     IsValidID) appears in the same body, flag the method.
//   - Methods decorated with //nolint:serviceingress in a leading
//     doc comment are exempt — used for methods that genuinely
//     don't take user-supplied IDs (e.g. metric exporters that
//     PathJoin against caller-internal constants).
//
// v1 limits documented in the brief escape-valve (Mantis #1609):
//
//   - No call-tree / SSA analysis. A method that delegates to a
//     same-package helper which does IsValidID + PathJoin still
//     fires. Fix path: either inline the guard, OR add the nolint
//     pragma with a rationale pointing at the helper.
//   - Same-file lexical scan only. Cross-file collaboration in the
//     same package is not modelled.
//   - Literal `string` params only. Struct input shapes (e.g.
//     CreateInput { ID string }) are not introspected — resolving
//     field types requires the type-checker, deferred to v2. Most
//     existing *Service methods take struct inputs, so v1 catches
//     the narrowest, highest-signal class: methods that accept a
//     raw `string` ID alongside other params. Tightening to
//     struct-aware analysis is a forward-arc Mantis (see brief).
//
// The wired-up enforcement lives in serviceingress_test.go which
// scans the parent ../ pkg tree and fails the standard Go test
// suite on any new finding. Mirrors the cousinvalidator pattern —
// no new tooling outside the existing `task test:go` pipeline.
//
// Usage example (programmatic scan):
//
//	findings, err := serviceingress.Scan("../")
//	if err != nil { core.Print(err.Error()) }
//	for _, f := range findings {
//	    core.Print(f.String())
//	}

package serviceingress

import (
	"go/ast"
	"go/parser"
	"go/token"

	core "dappco.re/go"
)

// WailsFile is the filename leaf that exempts a file from the rule.
// The wails-layer IS the validation boundary by convention; a
// method declared in wails.go is expected to call IsValidID itself
// (or delegate to a downstream service-method that does).
const WailsFile = "wails.go"

// PathsPackageDir is the directory leaf that exempts every file
// inside from the rule. The canonical paths.* validators live
// there and need to call PathJoin without recursing into IsValidID.
const PathsPackageDir = "paths"

// NolintPragma is the leading-comment marker that exempts a single
// method from the rule. Must appear in the method's doc comment
// immediately above the func decl, alongside a one-line rationale.
// Example:
//
//	// rotateInternal rebuilds the segment index from an internal
//	// constant prefix; no user input reaches PathJoin.
//	//
//	//nolint:serviceingress — caller-internal prefix, no UI ingress
//	func (s *Service) rotateInternal(seg int) core.Result { ... }
const NolintPragma = "//nolint:serviceingress"

// ServiceReceiverName is the receiver-type name that the rule
// applies to. Methods on other types (helpers, config carriers,
// fixtures) are out of scope — they don't form the wails-bypass
// surface that Cerberus #24 Finding 2 calls out.
const ServiceReceiverName = "Service"

// pathJoinSelector is the call-selector tail that signals
// "filesystem path being assembled from untrusted parts". Matched
// on the selector identifier alone (not the package qualifier) so
// `core.PathJoin`, `c.PathJoin`, and an aliased import like
// `corego "dappco.re/go"` + `corego.PathJoin` all flag identically.
const pathJoinSelector = "PathJoin"

// isValidIDSelector is the call-selector tail that proves the
// method validated its input before reaching PathJoin. Same tail-
// match logic — `paths.IsValidID`, `IsValidID` (dot-imported), or
// any alias all count.
const isValidIDSelector = "IsValidID"

// Finding describes a single rule violation: an exported *Service
// method that calls PathJoin without a matching IsValidID guard
// in a file that is NOT wails.go.
type Finding struct {
	// File is the path to the offending Go file (as the walker
	// observed it; may be relative to the scan root).
	File string
	// Line is the 1-based line number of the func declaration.
	Line int
	// MethodName is the exported method identifier.
	MethodName string
}

// String formats the finding in compiler-style `file:line: message`
// form so a CI log line is one-click jumpable in most editors.
//
// Usage example:
//
//	core.Print(finding.String())
func (f Finding) String() string {
	return core.Sprintf(
		"%s:%d: exported *Service method %q reaches core.PathJoin "+
			"without paths.IsValidID — move the ingress to wails.go OR call "+
			"paths.IsValidID first OR add //nolint:serviceingress with rationale "+
			"(Mantis #1609; Cerberus #24 Finding 2)",
		f.File, f.Line, f.MethodName)
}

// Scan walks every non-exempt Go source file under root and returns
// the set of findings. An empty slice (with nil error) means clean.
//
// Exemptions applied:
//
//   - *_test.go files
//   - files under any directory leaf equal to PathsPackageDir
//   - files named WailsFile (the canonical ingress boundary)
//   - methods preceded by NolintPragma in their doc comment
//
// Walk errors surface as a core error — typically a missing root
// or a permission failure. Per-file parse errors are absorbed as
// findings-of-zero (a non-parseable file cannot define a
// flag-worthy method; the build would fail elsewhere).
//
// Usage example:
//
//	findings, err := serviceingress.Scan("/path/to/go/pkg")
//	if err != nil { return core.Fail(err) }
//	for _, f := range findings { core.Print(f.String()) }
func Scan(root string) ([]Finding, error) {
	var out []Finding
	walkErr := core.PathWalkDir(root, func(path string, d core.FsDirEntry, err error) error {
		if err != nil {
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
		if name == WailsFile {
			return nil
		}
		findings := scanFile(path)
		out = append(out, findings...)
		return nil
	})
	if walkErr != nil {
		return nil, core.E("serviceingress.Scan",
			"PathWalkDir under "+root, walkErr)
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
		if !isExportedServiceMethod(fd) {
			continue
		}
		if !hasStringParam(fd) {
			continue
		}
		if hasNolintPragma(fd.Doc) {
			continue
		}
		if !bodyCallsSelector(fd.Body, pathJoinSelector) {
			continue
		}
		if bodyCallsSelector(fd.Body, isValidIDSelector) {
			continue
		}
		pos := fset.Position(fd.Pos())
		out = append(out, Finding{
			File:       path,
			Line:       pos.Line,
			MethodName: fd.Name.Name,
		})
	}
	return out
}

// isExportedServiceMethod reports whether fd is an exported method
// whose receiver is *Service or Service. Free functions, methods
// on other receiver types, and unexported methods all return false.
//
// Usage example:
//
//	if isExportedServiceMethod(fd) { ... }
func isExportedServiceMethod(fd *ast.FuncDecl) bool {
	if fd.Name == nil {
		return false
	}
	if !fd.Name.IsExported() {
		return false
	}
	if fd.Recv == nil || len(fd.Recv.List) == 0 {
		return false
	}
	return receiverTypeName(fd.Recv.List[0].Type) == ServiceReceiverName
}

// receiverTypeName extracts the underlying type identifier from a
// receiver type expression. Handles both pointer (`*Service`) and
// value (`Service`) receivers; returns the empty string for any
// other shape (slices, maps, generics with non-ident bases).
//
// Usage example:
//
//	if receiverTypeName(field.Type) == ServiceReceiverName { ... }
func receiverTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.StarExpr:
		if id, ok := t.X.(*ast.Ident); ok {
			return id.Name
		}
		// Pointer-to-generic-instantiation: `*Service[T]`.
		if idx, ok := t.X.(*ast.IndexExpr); ok {
			if id, ok := idx.X.(*ast.Ident); ok {
				return id.Name
			}
		}
	case *ast.Ident:
		return t.Name
	case *ast.IndexExpr:
		if id, ok := t.X.(*ast.Ident); ok {
			return id.Name
		}
	}
	return ""
}

// hasStringParam reports whether fd takes at least one parameter
// whose declared type is literally `string`. Struct params (e.g.
// CreateInput) are out of scope for v1 — resolving their field
// types requires the type-checker, deferred per the brief's
// SECURITY-NOTE escape valve.
//
// The narrow `string`-literal scope mirrors the brief's exact
// wording ("any exported method on *Service whose param of
// `string` type reaches core.PathJoin"). The wider struct-param
// surface is a forward-arc tightening — when it lands, swap this
// helper for a go/types-based one and re-run the live scan.
//
// Usage example:
//
//	if !hasStringParam(fd) { continue }
func hasStringParam(fd *ast.FuncDecl) bool {
	if fd.Type == nil || fd.Type.Params == nil {
		return false
	}
	for _, field := range fd.Type.Params.List {
		if id, ok := field.Type.(*ast.Ident); ok && id.Name == "string" {
			return true
		}
	}
	return false
}

// bodyCallsSelector reports whether the func body contains any
// CallExpr whose selector identifier equals selName. Matches both
// `pkg.SelName(...)` (SelectorExpr) and `SelName(...)` (bare Ident,
// e.g. dot-imported) forms.
//
// Returns false for a nil body (interface methods, externally-
// linked stubs).
//
// Usage example:
//
//	if bodyCallsSelector(fd.Body, "PathJoin") { ... }
func bodyCallsSelector(body *ast.BlockStmt, selName string) bool {
	if body == nil {
		return false
	}
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		if found {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch fn := call.Fun.(type) {
		case *ast.SelectorExpr:
			if fn.Sel != nil && fn.Sel.Name == selName {
				found = true
				return false
			}
		case *ast.Ident:
			if fn.Name == selName {
				found = true
				return false
			}
		}
		return true
	})
	return found
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
		text := core.Trim(core.TrimPrefix(c.Text, "//"))
		if core.HasPrefix(text, "nolint:serviceingress") {
			return true
		}
		if core.Contains(c.Text, NolintPragma) {
			return true
		}
	}
	return false
}
