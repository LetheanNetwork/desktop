// SPDX-Licence-Identifier: EUPL-1.2

package php_test

import (
	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/php"
)

func ExampleService_Detect() {
	ref := (*subject.Service).Detect
	_ = core.Sprintf("%T", ref)
}

func ExampleService_Project() {
	ref := (*subject.Service).Project
	_ = core.Sprintf("%T", ref)
}

func ExampleService_Scripts() {
	ref := (*subject.Service).Scripts
	_ = core.Sprintf("%T", ref)
}

func ExampleService_Run() {
	ref := (*subject.Service).Run
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

func TestWails_Service_Project_Good(t *core.T) {
	ref := (*subject.Service).Project
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "Service_Project")
}

func TestWails_Service_Project_Bad(t *core.T) {
	ref := (*subject.Service).Project
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:Service_Project", "Service_Project")
}

func TestWails_Service_Project_Ugly(t *core.T) {
	ref := (*subject.Service).Project
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("Service_Project"), 0)
}

func TestWails_Service_Scripts_Good(t *core.T) {
	ref := (*subject.Service).Scripts
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "Service_Scripts")
}

func TestWails_Service_Scripts_Bad(t *core.T) {
	ref := (*subject.Service).Scripts
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:Service_Scripts", "Service_Scripts")
}

func TestWails_Service_Scripts_Ugly(t *core.T) {
	ref := (*subject.Service).Scripts
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("Service_Scripts"), 0)
}

func TestWails_Service_Run_Good(t *core.T) {
	ref := (*subject.Service).Run
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "Service_Run")
}

func TestWails_Service_Run_Bad(t *core.T) {
	ref := (*subject.Service).Run
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:Service_Run", "Service_Run")
}

func TestWails_Service_Run_Ugly(t *core.T) {
	ref := (*subject.Service).Run
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("Service_Run"), 0)
}
