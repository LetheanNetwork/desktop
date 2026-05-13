// SPDX-Licence-Identifier: EUPL-1.2

package desktop_test

import (
	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/desktop"
)

func ExampleNewWindowService() {
	ref := subject.NewWindowService
	_ = core.Sprintf("%T", ref)
}

func ExampleWindowService_ServiceName() {
	ref := (*subject.WindowService).ServiceName
	_ = core.Sprintf("%T", ref)
}

func ExampleWindowService_ServiceStartup() {
	ref := (*subject.WindowService).ServiceStartup
	_ = core.Sprintf("%T", ref)
}

func ExampleWindowService_ServiceShutdown() {
	ref := (*subject.WindowService).ServiceShutdown
	_ = core.Sprintf("%T", ref)
}

func ExampleWindowService_Open() {
	ref := (*subject.WindowService).Open
	_ = core.Sprintf("%T", ref)
}

func ExampleWindowService_Hide() {
	ref := (*subject.WindowService).Hide
	_ = core.Sprintf("%T", ref)
}

func ExampleWindowService_List() {
	ref := (*subject.WindowService).List
	_ = core.Sprintf("%T", ref)
}

func TestBindings_NewWindowService_Good(t *core.T) {
	ref := subject.NewWindowService
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "NewWindowService")
}

func TestBindings_NewWindowService_Bad(t *core.T) {
	ref := subject.NewWindowService
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:NewWindowService", "NewWindowService")
}

func TestBindings_NewWindowService_Ugly(t *core.T) {
	ref := subject.NewWindowService
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("NewWindowService"), 0)
}

func TestBindings_WindowService_ServiceName_Good(t *core.T) {
	ref := (*subject.WindowService).ServiceName
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "WindowService_ServiceName")
}

func TestBindings_WindowService_ServiceName_Bad(t *core.T) {
	ref := (*subject.WindowService).ServiceName
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:WindowService_ServiceName", "WindowService_ServiceName")
}

func TestBindings_WindowService_ServiceName_Ugly(t *core.T) {
	ref := (*subject.WindowService).ServiceName
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("WindowService_ServiceName"), 0)
}

func TestBindings_WindowService_ServiceStartup_Good(t *core.T) {
	ref := (*subject.WindowService).ServiceStartup
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "WindowService_ServiceStartup")
}

func TestBindings_WindowService_ServiceStartup_Bad(t *core.T) {
	ref := (*subject.WindowService).ServiceStartup
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:WindowService_ServiceStartup", "WindowService_ServiceStartup")
}

func TestBindings_WindowService_ServiceStartup_Ugly(t *core.T) {
	ref := (*subject.WindowService).ServiceStartup
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("WindowService_ServiceStartup"), 0)
}

func TestBindings_WindowService_ServiceShutdown_Good(t *core.T) {
	ref := (*subject.WindowService).ServiceShutdown
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "WindowService_ServiceShutdown")
}

func TestBindings_WindowService_ServiceShutdown_Bad(t *core.T) {
	ref := (*subject.WindowService).ServiceShutdown
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:WindowService_ServiceShutdown", "WindowService_ServiceShutdown")
}

func TestBindings_WindowService_ServiceShutdown_Ugly(t *core.T) {
	ref := (*subject.WindowService).ServiceShutdown
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("WindowService_ServiceShutdown"), 0)
}

func TestBindings_WindowService_Open_Good(t *core.T) {
	ref := (*subject.WindowService).Open
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "WindowService_Open")
}

func TestBindings_WindowService_Open_Bad(t *core.T) {
	ref := (*subject.WindowService).Open
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:WindowService_Open", "WindowService_Open")
}

func TestBindings_WindowService_Open_Ugly(t *core.T) {
	ref := (*subject.WindowService).Open
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("WindowService_Open"), 0)
}

func TestBindings_WindowService_Hide_Good(t *core.T) {
	ref := (*subject.WindowService).Hide
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "WindowService_Hide")
}

func TestBindings_WindowService_Hide_Bad(t *core.T) {
	ref := (*subject.WindowService).Hide
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:WindowService_Hide", "WindowService_Hide")
}

func TestBindings_WindowService_Hide_Ugly(t *core.T) {
	ref := (*subject.WindowService).Hide
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("WindowService_Hide"), 0)
}

func TestBindings_WindowService_List_Good(t *core.T) {
	ref := (*subject.WindowService).List
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "WindowService_List")
}

func TestBindings_WindowService_List_Bad(t *core.T) {
	ref := (*subject.WindowService).List
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:WindowService_List", "WindowService_List")
}

func TestBindings_WindowService_List_Ugly(t *core.T) {
	ref := (*subject.WindowService).List
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("WindowService_List"), 0)
}
