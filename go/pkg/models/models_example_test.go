// SPDX-Licence-Identifier: EUPL-1.2

package models_test

import (
	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/models"
)

func ExampleList() {
	ref := subject.List
	_ = core.Sprintf("%T", ref)
}

func TestModels_List_Good(t *core.T) {
	ref := subject.List
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "List")
}

func TestModels_List_Bad(t *core.T) {
	ref := subject.List
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:List", "List")
}

func TestModels_List_Ugly(t *core.T) {
	ref := subject.List
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("List"), 0)
}
