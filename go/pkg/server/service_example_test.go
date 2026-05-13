// SPDX-Licence-Identifier: EUPL-1.2

package server_test

import (
	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/server"
)

func ExampleNewService() {
	ref := subject.NewService
	_ = core.Sprintf("%T", ref)
}

func ExampleService_Register() {
	ref := (*subject.Service).Register
	_ = core.Sprintf("%T", ref)
}

func ExampleService_Handler() {
	ref := (*subject.Service).Handler
	_ = core.Sprintf("%T", ref)
}

func ExampleService_Engine() {
	ref := (*subject.Service).Engine
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

func TestService_Service_Handler_Good(t *core.T) {
	ref := (*subject.Service).Handler
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "Service_Handler")
}

func TestService_Service_Handler_Bad(t *core.T) {
	ref := (*subject.Service).Handler
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:Service_Handler", "Service_Handler")
}

func TestService_Service_Handler_Ugly(t *core.T) {
	ref := (*subject.Service).Handler
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("Service_Handler"), 0)
}

func TestService_Service_Engine_Good(t *core.T) {
	ref := (*subject.Service).Engine
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "Service_Engine")
}

func TestService_Service_Engine_Bad(t *core.T) {
	ref := (*subject.Service).Engine
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:Service_Engine", "Service_Engine")
}

func TestService_Service_Engine_Ugly(t *core.T) {
	ref := (*subject.Service).Engine
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("Service_Engine"), 0)
}

func TestService_Service_Start_Good(t *core.T) {
	ref := (*subject.Service).Start
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "Service_Start")
}

func TestService_Service_Start_Bad(t *core.T) {
	ref := (*subject.Service).Start
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:Service_Start", "Service_Start")
}

func TestService_Service_Start_Ugly(t *core.T) {
	ref := (*subject.Service).Start
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("Service_Start"), 0)
}

func TestService_Service_Stop_Good(t *core.T) {
	ref := (*subject.Service).Stop
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "Service_Stop")
}

func TestService_Service_Stop_Bad(t *core.T) {
	ref := (*subject.Service).Stop
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:Service_Stop", "Service_Stop")
}

func TestService_Service_Stop_Ugly(t *core.T) {
	ref := (*subject.Service).Stop
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("Service_Stop"), 0)
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
