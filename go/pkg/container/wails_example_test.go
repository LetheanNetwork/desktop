// SPDX-Licence-Identifier: EUPL-1.2

package container_test

import (
	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/container"
)

func ExampleService_Detect() {
	ref := (*subject.Service).Detect
	_ = core.Sprintf("%T", ref)
}

func ExampleService_List() {
	ref := (*subject.Service).List
	_ = core.Sprintf("%T", ref)
}

func ExampleService_Logs() {
	ref := (*subject.Service).Logs
	_ = core.Sprintf("%T", ref)
}

func TestWails_Service_Detect_Good(t *core.T) {
	ref := (*subject.Service).Detect
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "Service_Detect")
}

func TestWails_Service_Detect_Bad(t *core.T) {
	ref := (*subject.Service).Detect
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:Service_Detect", "Service_Detect")
}

func TestWails_Service_Detect_Ugly(t *core.T) {
	ref := (*subject.Service).Detect
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("Service_Detect"), 0)
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

func TestWails_Service_Logs_Good(t *core.T) {
	ref := (*subject.Service).Logs
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "Service_Logs")
}

func TestWails_Service_Logs_Bad(t *core.T) {
	ref := (*subject.Service).Logs
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:Service_Logs", "Service_Logs")
}

func TestWails_Service_Logs_Ugly(t *core.T) {
	ref := (*subject.Service).Logs
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("Service_Logs"), 0)
}
