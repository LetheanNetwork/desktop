// SPDX-Licence-Identifier: EUPL-1.2

package bridge_test

import (
	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/bridge"
)

func ExampleRegisterService() {
	ref := subject.RegisterService
	_ = core.Sprintf("%T", ref)
}

func ExampleService_OnStartup() {
	ref := (*subject.Service).OnStartup
	_ = core.Sprintf("%T", ref)
}

func ExampleService_OnShutdown() {
	ref := (*subject.Service).OnShutdown
	_ = core.Sprintf("%T", ref)
}

func ExampleService_Port() {
	ref := (*subject.Service).Port
	_ = core.Sprintf("%T", ref)
}

func TestBridge_RegisterService_Good(t *core.T) {
	ref := subject.RegisterService
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "RegisterService")
}

func TestBridge_RegisterService_Bad(t *core.T) {
	ref := subject.RegisterService
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:RegisterService", "RegisterService")
}

func TestBridge_RegisterService_Ugly(t *core.T) {
	ref := subject.RegisterService
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("RegisterService"), 0)
}

func TestBridge_Service_OnStartup_Good(t *core.T) {
	ref := (*subject.Service).OnStartup
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "Service_OnStartup")
}

func TestBridge_Service_OnStartup_Bad(t *core.T) {
	ref := (*subject.Service).OnStartup
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:Service_OnStartup", "Service_OnStartup")
}

func TestBridge_Service_OnStartup_Ugly(t *core.T) {
	ref := (*subject.Service).OnStartup
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("Service_OnStartup"), 0)
}

func TestBridge_Service_OnShutdown_Good(t *core.T) {
	ref := (*subject.Service).OnShutdown
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "Service_OnShutdown")
}

func TestBridge_Service_OnShutdown_Bad(t *core.T) {
	ref := (*subject.Service).OnShutdown
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:Service_OnShutdown", "Service_OnShutdown")
}

func TestBridge_Service_OnShutdown_Ugly(t *core.T) {
	ref := (*subject.Service).OnShutdown
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("Service_OnShutdown"), 0)
}

func TestBridge_Service_Port_Good(t *core.T) {
	ref := (*subject.Service).Port
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "Service_Port")
}

func TestBridge_Service_Port_Bad(t *core.T) {
	ref := (*subject.Service).Port
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:Service_Port", "Service_Port")
}

func TestBridge_Service_Port_Ugly(t *core.T) {
	ref := (*subject.Service).Port
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("Service_Port"), 0)
}
