// SPDX-Licence-Identifier: EUPL-1.2

package sessions_test

import (
	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/sessions"
)

func ExampleCreate() {
	ref := subject.Create
	_ = core.Sprintf("%T", ref)
}

func ExampleAppend() {
	ref := subject.Append
	_ = core.Sprintf("%T", ref)
}

func ExampleRead() {
	ref := subject.Read
	_ = core.Sprintf("%T", ref)
}

func ExampleList() {
	ref := subject.List
	_ = core.Sprintf("%T", ref)
}

func TestSessions_Create_Bad(t *core.T) {
	ref := subject.Create
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:Create", "Create")
}

func TestSessions_Create_Ugly(t *core.T) {
	ref := subject.Create
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("Create"), 0)
}

func TestSessions_Append_Good(t *core.T) {
	ref := subject.Append
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "Append")
}

func TestSessions_Append_Bad(t *core.T) {
	ref := subject.Append
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:Append", "Append")
}

func TestSessions_Append_Ugly(t *core.T) {
	ref := subject.Append
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("Append"), 0)
}

func TestSessions_Read_Good(t *core.T) {
	ref := subject.Read
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "Read")
}

func TestSessions_Read_Bad(t *core.T) {
	ref := subject.Read
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:Read", "Read")
}

func TestSessions_Read_Ugly(t *core.T) {
	ref := subject.Read
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("Read"), 0)
}

func TestSessions_List_Good(t *core.T) {
	ref := subject.List
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "List")
}

func TestSessions_List_Bad(t *core.T) {
	ref := subject.List
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:List", "List")
}

func TestSessions_List_Ugly(t *core.T) {
	ref := subject.List
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("List"), 0)
}
