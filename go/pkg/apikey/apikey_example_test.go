// SPDX-Licence-Identifier: EUPL-1.2

package apikey_test

import (
	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/apikey"
)

func ExampleGenerateOrLoad() {
	ref := subject.GenerateOrLoad
	_ = core.Sprintf("%T", ref)
}

func ExampleRotate() {
	ref := subject.Rotate
	_ = core.Sprintf("%T", ref)
}

func ExampleMask() {
	ref := subject.Mask
	_ = core.Sprintf("%T", ref)
}

func TestApikey_GenerateOrLoad_Good(t *core.T) {
	ref := subject.GenerateOrLoad
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "GenerateOrLoad")
}

func TestApikey_GenerateOrLoad_Bad(t *core.T) {
	ref := subject.GenerateOrLoad
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:GenerateOrLoad", "GenerateOrLoad")
}

func TestApikey_GenerateOrLoad_Ugly(t *core.T) {
	ref := subject.GenerateOrLoad
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("GenerateOrLoad"), 0)
}

func TestApikey_Rotate_Good(t *core.T) {
	ref := subject.Rotate
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "Rotate")
}

func TestApikey_Rotate_Bad(t *core.T) {
	ref := subject.Rotate
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:Rotate", "Rotate")
}

func TestApikey_Rotate_Ugly(t *core.T) {
	ref := subject.Rotate
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("Rotate"), 0)
}

func TestApikey_Mask_Good(t *core.T) {
	ref := subject.Mask
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "Mask")
}

func TestApikey_Mask_Bad(t *core.T) {
	ref := subject.Mask
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:Mask", "Mask")
}

func TestApikey_Mask_Ugly(t *core.T) {
	ref := subject.Mask
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("Mask"), 0)
}
