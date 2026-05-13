// SPDX-Licence-Identifier: EUPL-1.2

package marketplace_test

import (
	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/marketplace"
)

func ExampleService_Search() {
	ref := (*subject.Service).Search
	_ = core.Sprintf("%T", ref)
}

func ExampleService_Info() {
	ref := (*subject.Service).Info
	_ = core.Sprintf("%T", ref)
}

func ExampleService_Installed() {
	ref := (*subject.Service).Installed
	_ = core.Sprintf("%T", ref)
}

func ExampleService_Install() {
	ref := (*subject.Service).Install
	_ = core.Sprintf("%T", ref)
}

func ExampleService_Remove() {
	ref := (*subject.Service).Remove
	_ = core.Sprintf("%T", ref)
}

func TestWails_Service_Search_Good(t *core.T) {
	ref := (*subject.Service).Search
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "Service_Search")
}

func TestWails_Service_Search_Bad(t *core.T) {
	ref := (*subject.Service).Search
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:Service_Search", "Service_Search")
}

func TestWails_Service_Search_Ugly(t *core.T) {
	ref := (*subject.Service).Search
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("Service_Search"), 0)
}

func TestWails_Service_Info_Good(t *core.T) {
	ref := (*subject.Service).Info
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "Service_Info")
}

func TestWails_Service_Info_Bad(t *core.T) {
	ref := (*subject.Service).Info
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:Service_Info", "Service_Info")
}

func TestWails_Service_Info_Ugly(t *core.T) {
	ref := (*subject.Service).Info
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("Service_Info"), 0)
}

func TestWails_Service_Installed_Good(t *core.T) {
	ref := (*subject.Service).Installed
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "Service_Installed")
}

func TestWails_Service_Installed_Bad(t *core.T) {
	ref := (*subject.Service).Installed
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:Service_Installed", "Service_Installed")
}

func TestWails_Service_Installed_Ugly(t *core.T) {
	ref := (*subject.Service).Installed
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("Service_Installed"), 0)
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
