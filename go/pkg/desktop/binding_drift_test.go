// SPDX-Licence-Identifier: EUPL-1.2

// Binding drift gate. The renderer names Go receivers as strings and
// nothing in the build couples the two sides, so a receiver rename or
// a deleted method only surfaces when a user clicks the thing. This
// test resolves every binding name the frontend holds against the Go
// source that must expose it, turning that class of breakage into a
// failing test.
//
// Why not render v0.21.0's webkit.ScanCallByName: that helper matches
// string literals passed directly to Call.ByName. This frontend has
// exactly four Call.ByName sites and all four take a `method`
// VARIABLE, funnelled through SurfaceBridgeService.call and friends.
// ScanCallByName therefore resolves zero names here and reports the
// four bridges as dynamic blind spots — a false green. The names are
// real and static, they just live one hop upstream as constants, in
// two shapes:
//
//	const M = 'dappco.re/lthn/desktop/pkg/benchmark.Service.History';
//	const M = `${PERMISSIONS_SERVICE}.Status`;  // prefix + method
//
// Both are resolved below. The Go side is read as source rather than
// reflected over live instances deliberately: constructing the ~60
// bound services means booting composition-root state, and one of
// them is opencode, which must never be started on a developer
// machine. Parsing costs nothing and starts no processes.

package desktop

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	core "dappco.re/go"
	"dappco.re/go/render/display/webkit"
)

// bindingPrefix is the module path in-repo binding names start with.
const bindingPrefix = "dappco.re/lthn/desktop/"

// externalPrefix is the module path for CoreGO bindings the renderer
// calls. These resolve against library types rather than repo source,
// so they are checked by reflection in the companion test below.
const externalPrefix = "dappco.re/go/"

// fullPattern matches a complete binding name written as one literal,
// in single quotes, double quotes or backticks.
func fullPattern(prefix string) *regexp.Regexp {
	return regexp.MustCompile(
		"['\"`]" + regexp.QuoteMeta(prefix) + `([A-Za-z0-9_/]+\.[A-Za-z0-9_.]+)['"` + "`]",
	)
}

// constPattern captures `const NAME = '<binding>'` so a prefix-only
// constant can be resolved when a template literal later appends the
// method.
func constPattern(prefix string) *regexp.Regexp {
	return regexp.MustCompile(
		`(?m)const\s+([A-Za-z0-9_]+)\s*=\s*['"` + "`]" +
			regexp.QuoteMeta(prefix) + `([A-Za-z0-9_/]+\.[A-Za-z0-9_]+)['"` + "`]",
	)
}

// templateBindingPattern captures `${CONST}.Method` — the prefix-plus-
// method shape used by the permissions and services bridges.
var templateBindingPattern = regexp.MustCompile(
	"`\\$\\{([A-Za-z0-9_]+)\\}\\.([A-Za-z0-9_]+)`",
)

// bindingRef is one resolved frontend → Go claim.
type bindingRef struct {
	name   string // full binding name as the renderer sees it
	pkgDir string // repo-relative Go package directory
	typ    string // receiver type name
	method string // method name; empty when the literal named only a type
	file   string // frontend file the claim came from
}

// scanSkipDirs are never walked — generated bindings restate the Go
// side rather than making an independent claim about it, so gating on
// them would only ever assert that codegen agrees with itself.
var scanSkipDirs = map[string]bool{
	"node_modules": true,
	"dist":         true,
	".angular":     true,
	"coverage":     true,
	"bindings":     true,
}

// parseBindingName splits a full binding name into its package
// directory, receiver type and optional method. Returns ok=false when
// the name is too short to carry a type.
func parseBindingName(name string) (pkgDir, typ, method string, ok bool) {
	return parseBindingNameFor(bindingPrefix, name)
}

// parseBindingNameFor is parseBindingName against an arbitrary module
// prefix, so in-repo and CoreGO names share one splitter.
func parseBindingNameFor(prefix, name string) (pkgDir, typ, method string, ok bool) {
	rest := strings.TrimPrefix(name, prefix)
	slash := strings.LastIndex(rest, "/")
	dir, tail := "", rest
	if slash >= 0 {
		dir, tail = rest[:slash], rest[slash+1:]
	}
	parts := strings.Split(tail, ".")
	if len(parts) < 2 {
		return "", "", "", false
	}
	pkgDir = filepath.Join(dir, parts[0])
	typ = parts[1]
	if len(parts) > 2 {
		method = parts[2]
	}
	return pkgDir, typ, method, true
}

// collectBindingRefs walks the frontend source tree and returns every
// binding claim it makes, sorted and de-duplicated.
func collectBindingRefs(t *core.T, frontendDir string) []bindingRef {
	return collectBindingRefsFor(t, frontendDir, bindingPrefix)
}

// collectBindingRefsFor scans for names under one module prefix.
func collectBindingRefsFor(t *core.T, frontendDir, prefix string) []bindingRef {
	t.Helper()
	seen := map[string]bindingRef{}
	fullRE, constRE := fullPattern(prefix), constPattern(prefix)

	walkErr := filepath.Walk(frontendDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if scanSkipDirs[info.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".ts" && ext != ".js" && ext != ".mts" && ext != ".mjs" {
			return nil
		}
		// Spec files assert against harness doubles, not the real
		// receivers, so their strings are fixtures rather than claims.
		if strings.HasSuffix(path, ".spec.ts") {
			return nil
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			// A gate that skips what it cannot read reports a false pass.
			return readErr
		}
		source := string(content)
		rel, _ := filepath.Rel(frontendDir, path)

		add := func(name string) {
			pkgDir, typ, method, ok := parseBindingNameFor(prefix, name)
			if !ok {
				return
			}
			if _, dup := seen[name]; dup {
				return
			}
			seen[name] = bindingRef{
				name: name, pkgDir: pkgDir, typ: typ, method: method, file: rel,
			}
		}

		// Shape 1 — a complete name in one literal.
		for _, m := range fullRE.FindAllStringSubmatch(source, -1) {
			add(prefix + m[1])
		}

		// Shape 2 — `${PREFIX_CONST}.Method`, resolved against the
		// prefix constants declared in the same file.
		prefixes := map[string]string{}
		for _, m := range constRE.FindAllStringSubmatch(source, -1) {
			prefixes[m[1]] = prefix + m[2]
		}
		for _, m := range templateBindingPattern.FindAllStringSubmatch(source, -1) {
			if prefix, found := prefixes[m[1]]; found {
				add(prefix + "." + m[2])
			}
		}
		return nil
	})
	core.RequireNoError(t, walkErr)

	refs := make([]bindingRef, 0, len(seen))
	for _, ref := range seen {
		refs = append(refs, ref)
	}
	sort.Slice(refs, func(i, j int) bool { return refs[i].name < refs[j].name })
	return refs
}

// goPackageMethods parses one Go package directory and returns the set
// of type names declared there plus the methods on each. Test files are
// excluded — the renderer cannot call those.
func goPackageMethods(dir string) (types map[string]bool, methods map[string]map[string]bool, err error) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, parser.AllErrors)
	if err != nil {
		return nil, nil, err
	}
	types = map[string]bool{}
	methods = map[string]map[string]bool{}
	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			for _, decl := range file.Decls {
				switch d := decl.(type) {
				case *ast.GenDecl:
					for _, spec := range d.Specs {
						if ts, ok := spec.(*ast.TypeSpec); ok {
							types[ts.Name.Name] = true
						}
					}
				case *ast.FuncDecl:
					if d.Recv == nil || len(d.Recv.List) == 0 {
						continue
					}
					recv := d.Recv.List[0].Type
					if star, ok := recv.(*ast.StarExpr); ok {
						recv = star.X
					}
					ident, ok := recv.(*ast.Ident)
					if !ok {
						continue
					}
					if methods[ident.Name] == nil {
						methods[ident.Name] = map[string]bool{}
					}
					methods[ident.Name][d.Name.Name] = true
				}
			}
		}
	}
	return types, methods, nil
}

// repoPaths resolves the frontend source dir and the Go module dir
// relative to this test's package directory (go/pkg/desktop).
func repoPaths(t *core.T) (frontendDir, goDir string) {
	t.Helper()
	goDir, err := filepath.Abs(filepath.Join("..", ".."))
	core.RequireNoError(t, err)
	frontendDir = filepath.Join(filepath.Dir(goDir), "frontend", "src")
	return frontendDir, goDir
}

// TestDesktop_BindingDrift_Good_EveryFrontendNameResolves is the gate.
// Every binding name the renderer holds must name a Go type that
// exists and, when the name carries a method, a method that exists on
// it. A rename on either side of the seam fails here instead of
// silently producing a dead button.
func TestDesktop_BindingDrift_Good_EveryFrontendNameResolves(t *core.T) {
	frontendDir, goDir := repoPaths(t)
	refs := collectBindingRefs(t, frontendDir)

	// A gate that finds nothing to check is broken, not passing.
	core.AssertTrue(t, len(refs) > 0)
	t.Logf("resolving %d binding name(s) held by the frontend", len(refs))

	var dead []string
	for _, ref := range refs {
		dir := filepath.Join(goDir, ref.pkgDir)
		if _, statErr := os.Stat(dir); statErr != nil {
			dead = append(dead, ref.name+" — no Go package at "+ref.pkgDir+" (called from "+ref.file+")")
			continue
		}
		types, methods, parseErr := goPackageMethods(dir)
		if parseErr != nil {
			dead = append(dead, ref.name+" — cannot parse "+ref.pkgDir+": "+parseErr.Error())
			continue
		}
		if !types[ref.typ] {
			dead = append(dead, ref.name+" — "+ref.pkgDir+" declares no type "+ref.typ+" (called from "+ref.file+")")
			continue
		}
		if ref.method != "" && !methods[ref.typ][ref.method] {
			dead = append(dead, ref.name+" — "+ref.typ+" has no method "+ref.method+" (called from "+ref.file+")")
		}
	}

	if len(dead) > 0 {
		t.Fatalf("frontend names %d binding(s) the Go side does not expose:\n  %s",
			len(dead), strings.Join(dead, "\n  "))
	}
}

// externalBindingRegistry holds a zero-value instance of every CoreGO
// type the renderer names. Zero values are safe and deliberate: only
// the method set is read, so nothing is constructed against a Core and
// no process starts.
//
// Adding a CoreGO binding to the frontend without adding it here fails
// the gate rather than silently skipping it — an unguarded external
// call is exactly the blind spot this test exists to remove.
func externalBindingRegistry() []any {
	return []any{
		&webkit.WindowBindingService{},
	}
}

// TestDesktop_BindingDrift_Good_EveryCoreGONameResolves guards the
// binding names that point at CoreGO rather than this repo. The tray
// panel names webkit.WindowBindingService.Open / .Hide; nothing in the
// build couples those strings to render's API, so a rename upstream
// would kill the tray panel silently. Reflection over the real types
// is the right instrument here — the receivers live in a module, not
// in source this repo can parse.
func TestDesktop_BindingDrift_Good_EveryCoreGONameResolves(t *core.T) {
	frontendDir, _ := repoPaths(t)
	refs := collectBindingRefsFor(t, frontendDir, externalPrefix)

	registry := externalBindingRegistry()

	// Every distinct external type the frontend names must be present
	// in the registry, otherwise the reflection check below would
	// report a miss it cannot explain.
	registered := map[string]bool{}
	for _, svc := range registry {
		for _, name := range webkit.BindingNames(svc) {
			if _, typ, _, ok := parseBindingNameFor(externalPrefix, name); ok {
				registered[typ] = true
			}
		}
	}
	var unregistered []string
	called := make([]string, 0, len(refs))
	for _, ref := range refs {
		if !registered[ref.typ] {
			unregistered = append(unregistered,
				ref.name+" (called from "+ref.file+")")
			continue
		}
		if ref.method != "" {
			called = append(called, ref.name)
		}
	}
	if len(unregistered) > 0 {
		t.Fatalf("frontend names %d CoreGO binding(s) absent from externalBindingRegistry — "+
			"add the type there so it is guarded:\n  %s",
			len(unregistered), strings.Join(unregistered, "\n  "))
	}

	t.Logf("resolving %d CoreGO binding name(s) held by the frontend", len(called))
	core.AssertTrue(t, len(called) > 0)

	if missing := webkit.UnresolvedBindingNames(called, registry...); len(missing) > 0 {
		t.Fatalf("frontend calls CoreGO bindings that no longer exist:\n  %s",
			strings.Join(missing, "\n  "))
	}
}

// TestDesktop_BindingDrift_Bad_UnparseableNameIsRejected pins the
// splitter: a name with no type segment must be refused rather than
// silently treated as a package-only claim that always passes.
func TestDesktop_BindingDrift_Bad_UnparseableNameIsRejected(t *core.T) {
	if _, _, _, ok := parseBindingName(bindingPrefix + "pkg/runner"); ok {
		t.Error("parseBindingName accepted a name carrying no type")
	}
}

// TestDesktop_BindingDrift_Ugly_NestedPackageAndPrefixOnlyName covers
// the two shapes that broke naive splitting: a nested package path
// (pkg/office/files) and a name that stops at the type.
func TestDesktop_BindingDrift_Ugly_NestedPackageAndPrefixOnlyName(t *core.T) {
	dir, typ, method, ok := parseBindingName(bindingPrefix + "pkg/office/files.Service.List")
	core.AssertTrue(t, ok)
	core.AssertEqual(t, filepath.Join("pkg", "office", "files"), dir)
	core.AssertEqual(t, "Service", typ)
	core.AssertEqual(t, "List", method)

	dir, typ, method, ok = parseBindingName(bindingPrefix + "pkg/permissions.WailsService")
	core.AssertTrue(t, ok)
	core.AssertEqual(t, filepath.Join("pkg", "permissions"), dir)
	core.AssertEqual(t, "WailsService", typ)
	core.AssertEqual(t, "", method)
}
