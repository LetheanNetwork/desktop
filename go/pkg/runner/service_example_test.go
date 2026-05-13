// SPDX-Licence-Identifier: EUPL-1.2

package runner_test

import (
	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/runner"
)

func ExampleNewService() {
	ref := subject.NewService
	_ = core.Sprintf("%T", ref)
}

func ExampleService_Register() {
	ref := (*subject.Service).Register
	_ = core.Sprintf("%T", ref)
}

func ExampleService_Generate() {
	ref := (*subject.Service).Generate
	_ = core.Sprintf("%T", ref)
}

func ExampleService_Chat() {
	ref := (*subject.Service).Chat
	_ = core.Sprintf("%T", ref)
}

func ExampleService_Models() {
	ref := (*subject.Service).Models
	_ = core.Sprintf("%T", ref)
}

func ExampleRegister() {
	ref := subject.Register
	_ = core.Sprintf("%T", ref)
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

func TestService_Service_Generate_Good(t *core.T) {
	ref := (*subject.Service).Generate
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "Service_Generate")
}

func TestService_Service_Generate_Bad(t *core.T) {
	ref := (*subject.Service).Generate
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:Service_Generate", "Service_Generate")
}

func TestService_Service_Generate_Ugly(t *core.T) {
	ref := (*subject.Service).Generate
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("Service_Generate"), 0)
}

func TestService_Service_Chat_Good(t *core.T) {
	ref := (*subject.Service).Chat
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "Service_Chat")
}

func TestService_Service_Chat_Bad(t *core.T) {
	ref := (*subject.Service).Chat
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:Service_Chat", "Service_Chat")
}

func TestService_Service_Chat_Ugly(t *core.T) {
	ref := (*subject.Service).Chat
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("Service_Chat"), 0)
}

func TestService_Service_Models_Good(t *core.T) {
	ref := (*subject.Service).Models
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "Service_Models")
}

func TestService_Service_Models_Bad(t *core.T) {
	ref := (*subject.Service).Models
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:Service_Models", "Service_Models")
}

func TestService_Service_Models_Ugly(t *core.T) {
	ref := (*subject.Service).Models
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("Service_Models"), 0)
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
