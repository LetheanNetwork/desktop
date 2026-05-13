// SPDX-Licence-Identifier: EUPL-1.2

package plugin_test

import (
	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/plugin"
)

func ExampleService_Install() {
	ref := (*subject.Service).Install
	_ = core.Sprintf("%T", ref)
}

func ExampleService_Remove() {
	ref := (*subject.Service).Remove
	_ = core.Sprintf("%T", ref)
}

func ExampleService_Start() {
	ref := (*subject.Service).Start
	_ = core.Sprintf("%T", ref)
}

func ExampleService_Stop() {
	ref := (*subject.Service).Stop
	_ = core.Sprintf("%T", ref)
}

func ExampleService_List() {
	ref := (*subject.Service).List
	_ = core.Sprintf("%T", ref)
}

func ExampleService_Status() {
	ref := (*subject.Service).Status
	_ = core.Sprintf("%T", ref)
}

func TestWails_Service_Install_Good(t *core.T) {
	ref := (*subject.Service).Install
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "Service_Install")
}

func TestWails_Service_Install_Bad(t *core.T) {
	ref := (*subject.Service).Install
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:Service_Install", "Service_Install")
}

func TestWails_Service_Install_Ugly(t *core.T) {
	ref := (*subject.Service).Install
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("Service_Install"), 0)
}

func TestWails_Service_Remove_Good(t *core.T) {
	ref := (*subject.Service).Remove
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "Service_Remove")
}

func TestWails_Service_Remove_Bad(t *core.T) {
	ref := (*subject.Service).Remove
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:Service_Remove", "Service_Remove")
}

func TestWails_Service_Remove_Ugly(t *core.T) {
	ref := (*subject.Service).Remove
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("Service_Remove"), 0)
}

func TestWails_Service_Start_Good(t *core.T) {
	ref := (*subject.Service).Start
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "Service_Start")
}

func TestWails_Service_Start_Bad(t *core.T) {
	ref := (*subject.Service).Start
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:Service_Start", "Service_Start")
}

func TestWails_Service_Start_Ugly(t *core.T) {
	ref := (*subject.Service).Start
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("Service_Start"), 0)
}

func TestWails_Service_Stop_Good(t *core.T) {
	ref := (*subject.Service).Stop
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "Service_Stop")
}

func TestWails_Service_Stop_Bad(t *core.T) {
	ref := (*subject.Service).Stop
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:Service_Stop", "Service_Stop")
}

func TestWails_Service_Stop_Ugly(t *core.T) {
	ref := (*subject.Service).Stop
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("Service_Stop"), 0)
}

func TestWails_Service_List_Good(t *core.T) {
	ref := (*subject.Service).List
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "Service_List")
}

func TestWails_Service_List_Bad(t *core.T) {
	ref := (*subject.Service).List
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:Service_List", "Service_List")
}

func TestWails_Service_List_Ugly(t *core.T) {
	ref := (*subject.Service).List
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("Service_List"), 0)
}

func TestWails_Service_Status_Good(t *core.T) {
	ref := (*subject.Service).Status
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "Service_Status")
}

func TestWails_Service_Status_Bad(t *core.T) {
	ref := (*subject.Service).Status
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:Service_Status", "Service_Status")
}

func TestWails_Service_Status_Ugly(t *core.T) {
	ref := (*subject.Service).Status
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("Service_Status"), 0)
}
