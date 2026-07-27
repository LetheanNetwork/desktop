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

func TestDesktop_DesktopStateBinding_GoodUsesCoreOwnedServiceOnce(t *core.T) {
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
		if path == "dappco.re/lthn/desktop/pkg/desktopstate" {
			imports++
		}
	}
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if selector.Sel.Name != "NewWailsService" || len(call.Args) != 1 {
			return true
		}
		pkg, packageOK := selector.X.(*ast.Ident)
		arg, argumentOK := call.Args[0].(*ast.Ident)
		if packageOK && argumentOK &&
			pkg.Name == "desktopstate" &&
			arg.Name == "desktopStateSvc" {
			bindings++
		}
		return true
	})

	core.AssertEqual(t, 1, imports)
	core.AssertEqual(t, 1, bindings)
	core.AssertContains(
		t,
		source,
		`core.ServiceFor[*desktopstate.Service](s.opts.Core, "desktopstate")`,
	)
}
