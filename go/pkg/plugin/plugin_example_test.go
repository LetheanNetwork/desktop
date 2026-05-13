// SPDX-Licence-Identifier: EUPL-1.2

package plugin_test

import (
	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/plugin"
)

func ExampleNewService() {
	ref := subject.NewService
	_ = core.Sprintf("%T", ref)
}

func ExampleRegister() {
	ref := subject.Register
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

func ExampleService_ProxyGroup() {
	ref := (*subject.Service).ProxyGroup
	_ = core.Sprintf("%T", ref)
}

func TestPlugin_NewService_Good(t *core.T) {
	ref := subject.NewService
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "NewService")
}

func TestPlugin_NewService_Bad(t *core.T) {
	ref := subject.NewService
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:NewService", "NewService")
}

func TestPlugin_NewService_Ugly(t *core.T) {
	ref := subject.NewService
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("NewService"), 0)
}

func TestPlugin_Register_Good(t *core.T) {
	ref := subject.Register
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "Register")
}

func TestPlugin_Register_Bad(t *core.T) {
	ref := subject.Register
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:Register", "Register")
}

func TestPlugin_Register_Ugly(t *core.T) {
	ref := subject.Register
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("Register"), 0)
}

func TestPlugin_Service_OnStartup_Good(t *core.T) {
	ref := (*subject.Service).OnStartup
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "Service_OnStartup")
}

func TestPlugin_Service_OnStartup_Bad(t *core.T) {
	ref := (*subject.Service).OnStartup
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:Service_OnStartup", "Service_OnStartup")
}

func TestPlugin_Service_OnStartup_Ugly(t *core.T) {
	ref := (*subject.Service).OnStartup
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("Service_OnStartup"), 0)
}

func TestPlugin_Service_OnShutdown_Good(t *core.T) {
	ref := (*subject.Service).OnShutdown
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "Service_OnShutdown")
}

func TestPlugin_Service_OnShutdown_Bad(t *core.T) {
	ref := (*subject.Service).OnShutdown
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:Service_OnShutdown", "Service_OnShutdown")
}

func TestPlugin_Service_OnShutdown_Ugly(t *core.T) {
	ref := (*subject.Service).OnShutdown
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("Service_OnShutdown"), 0)
}

func TestPlugin_Service_ProxyGroup_Good(t *core.T) {
	ref := (*subject.Service).ProxyGroup
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "Service_ProxyGroup")
}

func TestPlugin_Service_ProxyGroup_Bad(t *core.T) {
	ref := (*subject.Service).ProxyGroup
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:Service_ProxyGroup", "Service_ProxyGroup")
}

func TestPlugin_Service_ProxyGroup_Ugly(t *core.T) {
	ref := (*subject.Service).ProxyGroup
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("Service_ProxyGroup"), 0)
}
