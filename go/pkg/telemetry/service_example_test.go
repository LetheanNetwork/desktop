// SPDX-Licence-Identifier: EUPL-1.2

package telemetry_test

import (
	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/telemetry"
)

func ExampleSample() {
	ref := subject.Sample
	_ = core.Sprintf("%T", ref)
}

func ExampleNewService() {
	ref := subject.NewService
	_ = core.Sprintf("%T", ref)
}

func ExampleService_Register() {
	ref := (*subject.Service).Register
	_ = core.Sprintf("%T", ref)
}

func ExampleRegister() {
	ref := subject.Register
	_ = core.Sprintf("%T", ref)
}

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

func ExampleService_CurrentSample() {
	ref := (*subject.Service).CurrentSample
	_ = core.Sprintf("%T", ref)
}

func ExampleService_CurrentHostSnapshot() {
	ref := (*subject.Service).CurrentHostSnapshot
	_ = core.Sprintf("%T", ref)
}

func TestService_Sample_Good(t *core.T) {
	ref := subject.Sample
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "Sample")
}

func TestService_Sample_Bad(t *core.T) {
	ref := subject.Sample
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:Sample", "Sample")
}

func TestService_Sample_Ugly(t *core.T) {
	ref := subject.Sample
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("Sample"), 0)
}

func TestService_NewService_Good(t *core.T) {
	ref := subject.NewService
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "NewService")
}

func TestService_NewService_Bad(t *core.T) {
	ref := subject.NewService
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:NewService", "NewService")
}

func TestService_NewService_Ugly(t *core.T) {
	ref := subject.NewService
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("NewService"), 0)
}

func TestService_Service_Register_Good(t *core.T) {
	ref := (*subject.Service).Register
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "Service_Register")
}

func TestService_Service_Register_Bad(t *core.T) {
	ref := (*subject.Service).Register
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:Service_Register", "Service_Register")
}

func TestService_Service_Register_Ugly(t *core.T) {
	ref := (*subject.Service).Register
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("Service_Register"), 0)
}

func TestService_Register_Good(t *core.T) {
	ref := subject.Register
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "Register")
}

func TestService_Register_Bad(t *core.T) {
	ref := subject.Register
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:Register", "Register")
}

func TestService_Register_Ugly(t *core.T) {
	ref := subject.Register
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("Register"), 0)
}

func TestService_Service_ServiceName_Good(t *core.T) {
	ref := (*subject.Service).ServiceName
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "Service_ServiceName")
}

func TestService_Service_ServiceName_Bad(t *core.T) {
	ref := (*subject.Service).ServiceName
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:Service_ServiceName", "Service_ServiceName")
}

func TestService_Service_ServiceName_Ugly(t *core.T) {
	ref := (*subject.Service).ServiceName
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("Service_ServiceName"), 0)
}

func TestService_Service_ServiceStartup_Good(t *core.T) {
	ref := (*subject.Service).ServiceStartup
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "Service_ServiceStartup")
}

func TestService_Service_ServiceStartup_Bad(t *core.T) {
	ref := (*subject.Service).ServiceStartup
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:Service_ServiceStartup", "Service_ServiceStartup")
}

func TestService_Service_ServiceStartup_Ugly(t *core.T) {
	ref := (*subject.Service).ServiceStartup
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("Service_ServiceStartup"), 0)
}

func TestService_Service_ServiceShutdown_Good(t *core.T) {
	ref := (*subject.Service).ServiceShutdown
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "Service_ServiceShutdown")
}

func TestService_Service_ServiceShutdown_Bad(t *core.T) {
	ref := (*subject.Service).ServiceShutdown
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:Service_ServiceShutdown", "Service_ServiceShutdown")
}

func TestService_Service_ServiceShutdown_Ugly(t *core.T) {
	ref := (*subject.Service).ServiceShutdown
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("Service_ServiceShutdown"), 0)
}

func TestService_Service_CurrentSample_Good(t *core.T) {
	ref := (*subject.Service).CurrentSample
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "Service_CurrentSample")
}

func TestService_Service_CurrentSample_Bad(t *core.T) {
	ref := (*subject.Service).CurrentSample
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:Service_CurrentSample", "Service_CurrentSample")
}

func TestService_Service_CurrentSample_Ugly(t *core.T) {
	ref := (*subject.Service).CurrentSample
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("Service_CurrentSample"), 0)
}

func TestService_Service_CurrentHostSnapshot_Good(t *core.T) {
	ref := (*subject.Service).CurrentHostSnapshot
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "Service_CurrentHostSnapshot")
}

func TestService_Service_CurrentHostSnapshot_Bad(t *core.T) {
	ref := (*subject.Service).CurrentHostSnapshot
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:Service_CurrentHostSnapshot", "Service_CurrentHostSnapshot")
}

func TestService_Service_CurrentHostSnapshot_Ugly(t *core.T) {
	ref := (*subject.Service).CurrentHostSnapshot
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("Service_CurrentHostSnapshot"), 0)
}
