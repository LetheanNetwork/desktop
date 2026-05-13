// SPDX-Licence-Identifier: EUPL-1.2

package runner_test

import (
	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/runner"
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

func ExampleService_WGenerate() {
	ref := (*subject.Service).WGenerate
	_ = core.Sprintf("%T", ref)
}

func ExampleService_WChat() {
	ref := (*subject.Service).WChat
	_ = core.Sprintf("%T", ref)
}

func ExampleService_WModels() {
	ref := (*subject.Service).WModels
	_ = core.Sprintf("%T", ref)
}

func ExampleService_WRoutes() {
	ref := (*subject.Service).WRoutes
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

func TestWails_Service_WGenerate_Good(t *core.T) {
	ref := (*subject.Service).WGenerate
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "Service_WGenerate")
}

func TestWails_Service_WGenerate_Bad(t *core.T) {
	ref := (*subject.Service).WGenerate
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:Service_WGenerate", "Service_WGenerate")
}

func TestWails_Service_WGenerate_Ugly(t *core.T) {
	ref := (*subject.Service).WGenerate
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("Service_WGenerate"), 0)
}

func TestWails_Service_WChat_Good(t *core.T) {
	ref := (*subject.Service).WChat
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "Service_WChat")
}

func TestWails_Service_WChat_Bad(t *core.T) {
	ref := (*subject.Service).WChat
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:Service_WChat", "Service_WChat")
}

func TestWails_Service_WChat_Ugly(t *core.T) {
	ref := (*subject.Service).WChat
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("Service_WChat"), 0)
}

func TestWails_Service_WModels_Good(t *core.T) {
	ref := (*subject.Service).WModels
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "Service_WModels")
}

func TestWails_Service_WModels_Bad(t *core.T) {
	ref := (*subject.Service).WModels
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:Service_WModels", "Service_WModels")
}

func TestWails_Service_WModels_Ugly(t *core.T) {
	ref := (*subject.Service).WModels
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("Service_WModels"), 0)
}

func TestWails_Service_WRoutes_Good(t *core.T) {
	ref := (*subject.Service).WRoutes
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "Service_WRoutes")
}

func TestWails_Service_WRoutes_Bad(t *core.T) {
	ref := (*subject.Service).WRoutes
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:Service_WRoutes", "Service_WRoutes")
}

func TestWails_Service_WRoutes_Ugly(t *core.T) {
	ref := (*subject.Service).WRoutes
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("Service_WRoutes"), 0)
}
