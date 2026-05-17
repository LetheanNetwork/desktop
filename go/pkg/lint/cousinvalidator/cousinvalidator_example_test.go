// SPDX-Licence-Identifier: EUPL-1.2

// cousinvalidator_example_test.go — runnable agent-discovery
// surface per AX-2. The Example* funcs document the package's
// shape; the Test* funcs prove that shape compiles + binds.

package cousinvalidator_test

import (
	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/lint/cousinvalidator"
)

// ExampleScan shows the canonical scan-and-report shape — useful
// when wiring cousinvalidator into a new CI surface.
func ExampleScan() {
	findings, err := subject.Scan(".")
	if err != nil {
		core.Print(core.Stderr(), "scan error: %s", err.Error())
		return
	}
	for _, f := range findings {
		core.Print(core.Stderr(), "%s", f.String())
	}
}

// ExampleFinding_String shows the per-finding render shape that
// CI logs emit — file:line:message, jumpable in most editors.
func ExampleFinding_String() {
	f := subject.Finding{
		File:     "go/pkg/office/mail/service.go",
		Line:     142,
		FuncName: "isValidID",
	}
	core.Print(core.Stderr(), "%s", f.String())
}

func TestCousinValidator_ScanSymbol_Good(t *core.T) {
	ref := subject.Scan
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
}

func TestCousinValidator_ForbiddenNames_Populated_Good(t *core.T) {
	if len(subject.ForbiddenNames) == 0 {
		core.AssertNotEqual(t, 0, len(subject.ForbiddenNames))
	}
}

func TestCousinValidator_NolintPragma_NonEmpty_Good(t *core.T) {
	core.AssertNotEmpty(t, subject.NolintPragma)
}

func TestCousinValidator_PathsPackageDir_NonEmpty_Good(t *core.T) {
	core.AssertNotEmpty(t, subject.PathsPackageDir)
}
