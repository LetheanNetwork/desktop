// SPDX-License-Identifier: EUPL-1.2

package desktop

import (
	"go/ast"
	"go/parser"
	"go/token"
	goio "io"

	core "dappco.re/go"
	coreio "dappco.re/go/io"
)

func TestDesktop_FilesBinding_Good(t *core.T) {
	medium, err := coreio.NewSandboxed(".")
	core.RequireNoError(t, err)
	t.Cleanup(func() {
		if closer, ok := medium.(goio.Closer); ok {
			core.AssertNoError(t, closer.Close())
		}
	})
	source, err := medium.Read("desktop.go")
	core.RequireNoError(t, err)
	file, err := parser.ParseFile(
		token.NewFileSet(),
		"desktop.go",
		source,
		parser.AllErrors,
	)
	core.RequireNoError(t, err)

	officeImports := 0
	for _, spec := range file.Imports {
		path := core.TrimSuffix(core.TrimPrefix(spec.Path.Value, `"`), `"`)
		core.AssertNotEqual(t, "dappco.re/lthn/desktop/pkg/files", path)
		if path == "dappco.re/lthn/desktop/pkg/office/files" {
			officeImports++
		}
	}
	core.AssertEqual(t, 1, officeImports)

	filesBindings := 0
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "Bind" || len(call.Args) != 1 {
			return true
		}
		if identifier, ok := call.Args[0].(*ast.Ident); ok &&
			identifier.Name == "filesSvc" {
			filesBindings++
		}
		return true
	})
	core.AssertEqual(t, 1, filesBindings)
}
