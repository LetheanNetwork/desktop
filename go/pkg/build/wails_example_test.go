// SPDX-Licence-Identifier: EUPL-1.2

package build_test

import (
	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/build"
)

func ExampleService_ServiceName() {
	ref := (*subject.Service).ServiceName
	_ = core.Sprintf("%T", ref)
}

func ExampleService_ServiceStartup() {
	ref := (*subject.Service).ServiceStartup
	_ = core.Sprintf("%T", ref)
}

func ExampleService_ServiceShutdown() {
	ref := (*subject.Service).ServiceShutdown
	_ = core.Sprintf("%T", ref)
}

func ExampleService_Detect() {
	ref := (*subject.Service).Detect
	_ = core.Sprintf("%T", ref)
}

func ExampleService_Run() {
	ref := (*subject.Service).Run
	_ = core.Sprintf("%T", ref)
}

func ExampleService_ProcessOutput() {
	ref := (*subject.Service).ProcessOutput
	_ = core.Sprintf("%T", ref)
}

func ExampleService_ProcessKill() {
	ref := (*subject.Service).ProcessKill
	_ = core.Sprintf("%T", ref)
}

func ExampleService_ProcessList() {
	ref := (*subject.Service).ProcessList
	_ = core.Sprintf("%T", ref)
}

func TestWails_Service_ServiceName_Good(t *core.T) {
	ref := (*subject.Service).ServiceName
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "Service_ServiceName")
}

func TestWails_Service_ServiceName_Bad(t *core.T) {
	ref := (*subject.Service).ServiceName
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:Service_ServiceName", "Service_ServiceName")
}

func TestWails_Service_ServiceName_Ugly(t *core.T) {
	ref := (*subject.Service).ServiceName
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("Service_ServiceName"), 0)
}

func TestWails_Service_ServiceStartup_Good(t *core.T) {
	ref := (*subject.Service).ServiceStartup
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "Service_ServiceStartup")
}

func TestWails_Service_ServiceStartup_Bad(t *core.T) {
	ref := (*subject.Service).ServiceStartup
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:Service_ServiceStartup", "Service_ServiceStartup")
}

func TestWails_Service_ServiceStartup_Ugly(t *core.T) {
	ref := (*subject.Service).ServiceStartup
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("Service_ServiceStartup"), 0)
}

func TestWails_Service_ServiceShutdown_Good(t *core.T) {
	ref := (*subject.Service).ServiceShutdown
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "Service_ServiceShutdown")
}

func TestWails_Service_ServiceShutdown_Bad(t *core.T) {
	ref := (*subject.Service).ServiceShutdown
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:Service_ServiceShutdown", "Service_ServiceShutdown")
}

func TestWails_Service_ServiceShutdown_Ugly(t *core.T) {
	ref := (*subject.Service).ServiceShutdown
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("Service_ServiceShutdown"), 0)
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

func TestWails_Service_ProcessOutput_Good(t *core.T) {
	ref := (*subject.Service).ProcessOutput
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "Service_ProcessOutput")
}

func TestWails_Service_ProcessOutput_Bad(t *core.T) {
	ref := (*subject.Service).ProcessOutput
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:Service_ProcessOutput", "Service_ProcessOutput")
}

func TestWails_Service_ProcessOutput_Ugly(t *core.T) {
	ref := (*subject.Service).ProcessOutput
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("Service_ProcessOutput"), 0)
}

func TestWails_Service_ProcessKill_Good(t *core.T) {
	ref := (*subject.Service).ProcessKill
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "Service_ProcessKill")
}

func TestWails_Service_ProcessKill_Bad(t *core.T) {
	ref := (*subject.Service).ProcessKill
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:Service_ProcessKill", "Service_ProcessKill")
}

func TestWails_Service_ProcessKill_Ugly(t *core.T) {
	ref := (*subject.Service).ProcessKill
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("Service_ProcessKill"), 0)
}

func TestWails_Service_ProcessList_Good(t *core.T) {
	ref := (*subject.Service).ProcessList
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "Service_ProcessList")
}

func TestWails_Service_ProcessList_Bad(t *core.T) {
	ref := (*subject.Service).ProcessList
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:Service_ProcessList", "Service_ProcessList")
}

func TestWails_Service_ProcessList_Ugly(t *core.T) {
	ref := (*subject.Service).ProcessList
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("Service_ProcessList"), 0)
}
