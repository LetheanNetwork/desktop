// SPDX-Licence-Identifier: EUPL-1.2

package desktop

import (
	"go/ast"
	"go/parser"
	"go/token"
	goio "io"

	core "dappco.re/go"
	coreio "dappco.re/go/io"
)

func TestDesktop_ModelRuntimeBinding_GoodUsesCoreOwnedServiceOnce(t *core.T) {
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

	imports := 0
	bindings := 0
	for _, spec := range file.Imports {
		path := core.TrimSuffix(core.TrimPrefix(spec.Path.Value, `"`), `"`)
		if path == "dappco.re/lthn/desktop/pkg/modelruntime" {
			imports++
		}
	}
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "NewWailsService" ||
			len(call.Args) != 1 {
			return true
		}
		pkg, ok := selector.X.(*ast.Ident)
		arg, argOK := call.Args[0].(*ast.Ident)
		if ok && argOK &&
			pkg.Name == "modelruntime" &&
			arg.Name == "modelRuntimeSvc" {
			bindings++
		}
		return true
	})

	core.AssertEqual(t, 1, imports)
	core.AssertEqual(t, 1, bindings)
}
