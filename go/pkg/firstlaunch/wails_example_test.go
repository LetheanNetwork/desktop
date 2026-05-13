// SPDX-Licence-Identifier: EUPL-1.2

package firstlaunch_test

import (
	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/firstlaunch"
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

func ExampleWailsService_Detect() {
	ref := (*subject.WailsService).Detect
	_ = core.Sprintf("%T", ref)
}

func ExampleWailsService_Build() {
	ref := (*subject.WailsService).Build
	_ = core.Sprintf("%T", ref)
}

func ExampleWailsService_Paths() {
	ref := (*subject.WailsService).Paths
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

func TestWails_WailsService_Detect_Good(t *core.T) {
	ref := (*subject.WailsService).Detect
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "WailsService_Detect")
}

func TestWails_WailsService_Detect_Bad(t *core.T) {
	ref := (*subject.WailsService).Detect
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:WailsService_Detect", "WailsService_Detect")
}

func TestWails_WailsService_Detect_Ugly(t *core.T) {
	ref := (*subject.WailsService).Detect
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("WailsService_Detect"), 0)
}

func TestWails_WailsService_Build_Good(t *core.T) {
	ref := (*subject.WailsService).Build
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "WailsService_Build")
}

func TestWails_WailsService_Build_Bad(t *core.T) {
	ref := (*subject.WailsService).Build
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:WailsService_Build", "WailsService_Build")
}

func TestWails_WailsService_Build_Ugly(t *core.T) {
	ref := (*subject.WailsService).Build
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("WailsService_Build"), 0)
}

func TestWails_WailsService_Paths_Good(t *core.T) {
	ref := (*subject.WailsService).Paths
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "WailsService_Paths")
}

func TestWails_WailsService_Paths_Bad(t *core.T) {
	ref := (*subject.WailsService).Paths
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:WailsService_Paths", "WailsService_Paths")
}

func TestWails_WailsService_Paths_Ugly(t *core.T) {
	ref := (*subject.WailsService).Paths
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("WailsService_Paths"), 0)
}
