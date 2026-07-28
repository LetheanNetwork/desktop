// SPDX-License-Identifier: EUPL-1.2

package files

import (
	"go/ast"
	"go/parser"
	"go/token"
	goio "io"

	core "dappco.re/go"
	coreio "dappco.re/go/io"
)

func TestFilesMediumBoundary_Good(t *core.T) {
	sources := filesProductionSources(t)
	for name, source := range sources {
		file, err := parser.ParseFile(
			token.NewFileSet(),
			name,
			source,
			parser.AllErrors,
		)
		core.RequireNoError(t, err)
		assertNoFilesBypass(t, name, file)
	}
	assertThinWailsFacade(t, sources["wails.go"])
}

func filesProductionSources(t *core.T) map[string]string {
	t.Helper()
	medium, err := coreio.NewSandboxed(".")
	core.RequireNoError(t, err)
	t.Cleanup(func() {
		if closer, ok := medium.(goio.Closer); ok {
			core.AssertNoError(t, closer.Close())
		}
	})
	entries, err := medium.List("")
	core.RequireNoError(t, err)
	sources := make(map[string]string)
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() ||
			!core.HasSuffix(name, ".go") ||
			core.HasSuffix(name, "_test.go") {
			continue
		}
		source, readErr := medium.Read(name)
		core.RequireNoError(t, readErr)
		sources[name] = source
	}
	return sources
}

func assertNoFilesBypass(t *core.T, name string, file *ast.File) {
	t.Helper()
	coreAliases := make(map[string]bool)
	for _, spec := range file.Imports {
		path := core.TrimSuffix(core.TrimPrefix(spec.Path.Value, `"`), `"`)
		switch path {
		case "os", "path/filepath", "syscall":
			t.Errorf("%s imports forbidden filesystem package %s", name, path)
		case "dappco.re/go":
			alias := "core"
			if spec.Name != nil {
				alias = spec.Name.Name
			}
			coreAliases[alias] = true
		}
	}

	forbiddenSelectors := map[string]bool{
		"ReadFile":          true,
		"WriteFile":         true,
		"ReadDir":           true,
		"DirFS":             true,
		"Stat":              true,
		"Lstat":             true,
		"Open":              true,
		"Create":            true,
		"NewUnrestrictedFS": true,
		"Fs":                true,
	}
	forbiddenInputFields := map[string]bool{
		"Root":         true,
		"AbsolutePath": true,
		"Endpoint":     true,
		"Credential":   true,
		"Secret":       true,
		"Key":          true,
	}

	ast.Inspect(file, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if ok {
			identifier, isIdentifier := selector.X.(*ast.Ident)
			if isIdentifier &&
				coreAliases[identifier.Name] &&
				forbiddenSelectors[selector.Sel.Name] {
				t.Errorf(
					"%s uses forbidden core filesystem selector %s.%s",
					name,
					identifier.Name,
					selector.Sel.Name,
				)
			}
		}

		typeSpec, ok := node.(*ast.TypeSpec)
		if !ok || !core.HasSuffix(typeSpec.Name.Name, "Input") {
			return true
		}
		structType, ok := typeSpec.Type.(*ast.StructType)
		if !ok {
			return true
		}
		for _, field := range structType.Fields.List {
			for _, fieldName := range field.Names {
				if forbiddenInputFields[fieldName.Name] {
					t.Errorf(
						"%s exposes forbidden Wails input field %s.%s",
						name,
						typeSpec.Name.Name,
						fieldName.Name,
					)
				}
			}
		}
		return true
	})
}

func assertThinWailsFacade(t *core.T, source string) {
	t.Helper()
	core.RequireNotEmpty(t, source)
	file, err := parser.ParseFile(
		token.NewFileSet(),
		"wails.go",
		source,
		parser.AllErrors,
	)
	core.RequireNoError(t, err)
	expected := map[string]string{
		"ListMounts":      "listMounts",
		"ListDirectory":   "listDirectory",
		"Preview":         "preview",
		"Open":            "openHost",
		"Reveal":          "revealHost",
		"CreateDirectory": "createDirectory",
		"Rename":          "rename",
		"Copy":            "copy",
		"Move":            "move",
		"Trash":           "trash",
		"ListTrash":       "listTrash",
		"Restore":         "restore",
		"Delete":          "delete",
	}
	seen := make(map[string]bool)

	ast.Inspect(file, func(node ast.Node) bool {
		if selector, ok := node.(*ast.SelectorExpr); ok &&
			selector.Sel.Name == "Medium" {
			t.Errorf("wails.go accesses Medium directly")
		}
		declaration, ok := node.(*ast.FuncDecl)
		if !ok || declaration.Recv == nil || !declaration.Name.IsExported() {
			return true
		}
		helper, exists := expected[declaration.Name.Name]
		if !exists {
			t.Errorf("unexpected public Wails method %s", declaration.Name.Name)
			return false
		}
		seen[declaration.Name.Name] = true
		if declaration.Body == nil || len(declaration.Body.List) != 1 {
			t.Errorf("%s must contain one return delegation", declaration.Name.Name)
			return false
		}
		returnStatement, ok := declaration.Body.List[0].(*ast.ReturnStmt)
		if !ok || len(returnStatement.Results) != 1 {
			t.Errorf("%s must return one helper result", declaration.Name.Name)
			return false
		}
		call, ok := returnStatement.Results[0].(*ast.CallExpr)
		if !ok {
			t.Errorf("%s must return a helper call", declaration.Name.Name)
			return false
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != helper {
			t.Errorf(
				"%s must delegate to %s",
				declaration.Name.Name,
				helper,
			)
		}
		return false
	})

	for method := range expected {
		if !seen[method] {
			t.Errorf("wails.go is missing %s", method)
		}
	}
}
