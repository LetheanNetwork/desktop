// SPDX-Licence-Identifier: EUPL-1.2

package services

import (
	"go/ast"
	"go/parser"
	"go/token"
	goio "io"

	core "dappco.re/go"
	coreio "dappco.re/go/io"
)

func TestManagedServiceMediumBoundary_GoodHasNoHostFilesystemBypass(t *core.T) {
	medium, err := coreio.NewSandboxed(".")
	core.RequireNoError(t, err)
	t.Cleanup(func() {
		if closer, ok := medium.(goio.Closer); ok {
			core.AssertNoError(t, closer.Close())
		}
	})
	entries, err := medium.List("")
	core.RequireNoError(t, err)

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() ||
			!core.HasSuffix(name, ".go") ||
			core.HasSuffix(name, "_test.go") ||
			name == "manager.go" ||
			name == "registry.go" {
			continue
		}
		source, readErr := medium.Read(name)
		core.RequireNoError(t, readErr)
		file, parseErr := parser.ParseFile(
			token.NewFileSet(),
			name,
			source,
			parser.AllErrors,
		)
		core.RequireNoError(t, parseErr)
		assertManagedServiceMediumBoundary(t, name, file)
	}
}

func assertManagedServiceMediumBoundary(t *core.T, name string, file *ast.File) {
	t.Helper()
	// signals.go is the one file allowed syscall, and only for signal
	// constants — never for filesystem work. It is named here rather than the
	// rule being softened, so that a second file reaching for syscall still
	// fails this test and has to justify itself the same way.
	signalConstants := name == "signals.go"
	coreAliases := make(map[string]bool)
	for _, spec := range file.Imports {
		importPath := core.TrimSuffix(core.TrimPrefix(spec.Path.Value, `"`), `"`)
		switch importPath {
		case "syscall":
			if !signalConstants {
				t.Errorf("%s imports forbidden filesystem package %s", name, importPath)
			}
		case "os", "path/filepath":
			t.Errorf("%s imports forbidden filesystem package %s", name, importPath)
		case "dappco.re/go":
			alias := "core"
			if spec.Name != nil {
				alias = spec.Name.Name
			}
			coreAliases[alias] = true
		}
	}

	forbiddenSelectors := map[string]bool{
		"ReadFile":  true,
		"WriteFile": true,
		"ReadDir":   true,
		"Stat":      true,
		"Remove":    true,
		"RemoveAll": true,
		"Mkdir":     true,
		"MkdirAll":  true,
	}
	ast.Inspect(file, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		identifier, ok := selector.X.(*ast.Ident)
		if ok &&
			coreAliases[identifier.Name] &&
			forbiddenSelectors[selector.Sel.Name] {
			t.Errorf(
				"%s uses forbidden host filesystem selector %s.%s",
				name,
				identifier.Name,
				selector.Sel.Name,
			)
		}
		return true
	})
}
