// SPDX-Licence-Identifier: EUPL-1.2

package lint_test

import (
	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/lint"
)

func ExampleNewService() {
	ref := subject.NewService
	_ = core.Sprintf("%T", ref)
}

func ExampleRegister() {
	ref := subject.Register
	_ = core.Sprintf("%T", ref)
}

func TestLint_NewService_Good(t *core.T) {
	ref := subject.NewService
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "NewService")
}

func TestLint_NewService_Bad(t *core.T) {
	ref := subject.NewService
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:NewService", "NewService")
}

func TestLint_NewService_Ugly(t *core.T) {
	ref := subject.NewService
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("NewService"), 0)
}

func TestLint_Register_Good(t *core.T) {
	ref := subject.Register
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "Register")
}

func TestLint_Register_Bad(t *core.T) {
	ref := subject.Register
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:Register", "Register")
}

func TestLint_Register_Ugly(t *core.T) {
	ref := subject.Register
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("Register"), 0)
}
