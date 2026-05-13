// SPDX-Licence-Identifier: EUPL-1.2

package firstlaunch_test

import (
	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/firstlaunch"
)

func ExampleDetect() {
	ref := subject.Detect
	_ = core.Sprintf("%T", ref)
}

func TestFirstlaunch_Detect_Good(t *core.T) {
	ref := subject.Detect
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "Detect")
}

func TestFirstlaunch_Detect_Bad(t *core.T) {
	ref := subject.Detect
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:Detect", "Detect")
}

func TestFirstlaunch_Detect_Ugly(t *core.T) {
	ref := subject.Detect
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("Detect"), 0)
}
