// SPDX-Licence-Identifier: EUPL-1.2

package api_test

import (
	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/api"
)

func ExampleRegister() {
	ref := subject.Register
	_ = core.Sprintf("%T", ref)
}

func TestRegister_Register_Good(t *core.T) {
	ref := subject.Register
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "Register")
}

func TestRegister_Register_Bad(t *core.T) {
	ref := subject.Register
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:Register", "Register")
}

func TestRegister_Register_Ugly(t *core.T) {
	ref := subject.Register
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("Register"), 0)
}
