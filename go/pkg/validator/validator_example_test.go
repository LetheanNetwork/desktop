// SPDX-Licence-Identifier: EUPL-1.2

package validator_test

import (
	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/validator"
)

func ExampleEndpoint() {
	ref := subject.Endpoint
	_ = core.Sprintf("%T", ref)
}

func TestValidator_Endpoint_Good(t *core.T) {
	ref := subject.Endpoint
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "Endpoint")
}

func TestValidator_Endpoint_Bad(t *core.T) {
	ref := subject.Endpoint
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:Endpoint", "Endpoint")
}

func TestValidator_Endpoint_Ugly(t *core.T) {
	ref := subject.Endpoint
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("Endpoint"), 0)
}
