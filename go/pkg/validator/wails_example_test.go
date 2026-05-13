// SPDX-Licence-Identifier: EUPL-1.2

package validator_test

import (
	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/validator"
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

func ExampleWailsService_Endpoint() {
	ref := (*subject.WailsService).Endpoint
	_ = core.Sprintf("%T", ref)
}

func TestWails_NewWailsService_Good(t *core.T) {
	ref := subject.NewWailsService
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "NewWailsService")
}

func TestWails_NewWailsService_Bad(t *core.T) {
	ref := subject.NewWailsService
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:NewWailsService", "NewWailsService")
}

func TestWails_NewWailsService_Ugly(t *core.T) {
	ref := subject.NewWailsService
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("NewWailsService"), 0)
}

func TestWails_WailsService_ServiceName_Good(t *core.T) {
	ref := (*subject.WailsService).ServiceName
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "WailsService_ServiceName")
}

func TestWails_WailsService_ServiceName_Bad(t *core.T) {
	ref := (*subject.WailsService).ServiceName
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:WailsService_ServiceName", "WailsService_ServiceName")
}

func TestWails_WailsService_ServiceName_Ugly(t *core.T) {
	ref := (*subject.WailsService).ServiceName
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("WailsService_ServiceName"), 0)
}

func TestWails_WailsService_ServiceStartup_Good(t *core.T) {
	ref := (*subject.WailsService).ServiceStartup
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "WailsService_ServiceStartup")
}

func TestWails_WailsService_ServiceStartup_Bad(t *core.T) {
	ref := (*subject.WailsService).ServiceStartup
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:WailsService_ServiceStartup", "WailsService_ServiceStartup")
}

func TestWails_WailsService_ServiceStartup_Ugly(t *core.T) {
	ref := (*subject.WailsService).ServiceStartup
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("WailsService_ServiceStartup"), 0)
}

func TestWails_WailsService_ServiceShutdown_Good(t *core.T) {
	ref := (*subject.WailsService).ServiceShutdown
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "WailsService_ServiceShutdown")
}

func TestWails_WailsService_ServiceShutdown_Bad(t *core.T) {
	ref := (*subject.WailsService).ServiceShutdown
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:WailsService_ServiceShutdown", "WailsService_ServiceShutdown")
}

func TestWails_WailsService_ServiceShutdown_Ugly(t *core.T) {
	ref := (*subject.WailsService).ServiceShutdown
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("WailsService_ServiceShutdown"), 0)
}

func TestWails_WailsService_Endpoint_Good(t *core.T) {
	ref := (*subject.WailsService).Endpoint
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "WailsService_Endpoint")
}

func TestWails_WailsService_Endpoint_Bad(t *core.T) {
	ref := (*subject.WailsService).Endpoint
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:WailsService_Endpoint", "WailsService_Endpoint")
}

func TestWails_WailsService_Endpoint_Ugly(t *core.T) {
	ref := (*subject.WailsService).Endpoint
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("WailsService_Endpoint"), 0)
}
