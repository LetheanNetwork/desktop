// SPDX-Licence-Identifier: EUPL-1.2

package tools_test

import (
	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/tools"
)

func ExampleNewWailsService() {
	ref := subject.NewWailsService
	_ = core.Sprintf("%T", ref)
}

func ExampleWailsService_ServiceName() {
	ref := (*subject.WailsService).ServiceName
	_ = core.Sprintf("%T", ref)
}

func ExampleWailsService_ServiceStartup() {
	ref := (*subject.WailsService).ServiceStartup
	_ = core.Sprintf("%T", ref)
}

func ExampleWailsService_ServiceShutdown() {
	ref := (*subject.WailsService).ServiceShutdown
	_ = core.Sprintf("%T", ref)
}

func ExampleWailsService_List() {
	ref := (*subject.WailsService).List
	_ = core.Sprintf("%T", ref)
}

func ExampleRegister() {
	ref := subject.Register
	_ = core.Sprintf("%T", ref)
}

func TestTools_NewWailsService_Good(t *core.T) {
	ref := subject.NewWailsService
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "NewWailsService")
}

func TestTools_NewWailsService_Bad(t *core.T) {
	ref := subject.NewWailsService
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:NewWailsService", "NewWailsService")
}

func TestTools_NewWailsService_Ugly(t *core.T) {
	ref := subject.NewWailsService
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("NewWailsService"), 0)
}

func TestTools_WailsService_ServiceName_Good(t *core.T) {
	ref := (*subject.WailsService).ServiceName
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "WailsService_ServiceName")
}

func TestTools_WailsService_ServiceName_Bad(t *core.T) {
	ref := (*subject.WailsService).ServiceName
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:WailsService_ServiceName", "WailsService_ServiceName")
}

func TestTools_WailsService_ServiceName_Ugly(t *core.T) {
	ref := (*subject.WailsService).ServiceName
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("WailsService_ServiceName"), 0)
}

func TestTools_WailsService_ServiceStartup_Good(t *core.T) {
	ref := (*subject.WailsService).ServiceStartup
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "WailsService_ServiceStartup")
}

func TestTools_WailsService_ServiceStartup_Bad(t *core.T) {
	ref := (*subject.WailsService).ServiceStartup
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:WailsService_ServiceStartup", "WailsService_ServiceStartup")
}

func TestTools_WailsService_ServiceStartup_Ugly(t *core.T) {
	ref := (*subject.WailsService).ServiceStartup
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("WailsService_ServiceStartup"), 0)
}

func TestTools_WailsService_ServiceShutdown_Good(t *core.T) {
	ref := (*subject.WailsService).ServiceShutdown
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "WailsService_ServiceShutdown")
}

func TestTools_WailsService_ServiceShutdown_Bad(t *core.T) {
	ref := (*subject.WailsService).ServiceShutdown
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:WailsService_ServiceShutdown", "WailsService_ServiceShutdown")
}

func TestTools_WailsService_ServiceShutdown_Ugly(t *core.T) {
	ref := (*subject.WailsService).ServiceShutdown
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("WailsService_ServiceShutdown"), 0)
}

func TestTools_WailsService_List_Good(t *core.T) {
	ref := (*subject.WailsService).List
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "WailsService_List")
}

func TestTools_WailsService_List_Bad(t *core.T) {
	ref := (*subject.WailsService).List
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:WailsService_List", "WailsService_List")
}

func TestTools_WailsService_List_Ugly(t *core.T) {
	ref := (*subject.WailsService).List
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("WailsService_List"), 0)
}

func TestTools_Register_Good(t *core.T) {
	ref := subject.Register
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "Register")
}

func TestTools_Register_Bad(t *core.T) {
	ref := subject.Register
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:Register", "Register")
}

func TestTools_Register_Ugly(t *core.T) {
	ref := subject.Register
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("Register"), 0)
}
